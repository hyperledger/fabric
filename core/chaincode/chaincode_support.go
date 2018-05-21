/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Runtime is used to manage chaincode runtime instances.
type Runtime interface {
	Start(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error
	Stop(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error
}

// Launcher is used to launch chaincode runtimes.
type Launcher interface {
	Launch(context context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) error
}

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	Keepalive       time.Duration
	ExecuteTimeout  time.Duration
	UserRunsCC      bool
	Runtime         Runtime
	ACLProvider     ACLProvider
	HandlerRegistry *HandlerRegistry
	Launcher        Launcher
	sccp            sysccprovider.SystemChaincodeProvider
}

// NewChaincodeSupport creates a new ChaincodeSupport instance.
func NewChaincodeSupport(
	config *Config,
	peerAddress string,
	userRunsCC bool,
	caCert []byte,
	certGenerator CertGenerator,
	packageProvider PackageProvider,
	aclProvider ACLProvider,
	processor Processor,
	sccp sysccprovider.SystemChaincodeProvider,
) *ChaincodeSupport {
	cs := &ChaincodeSupport{
		UserRunsCC:      userRunsCC,
		Keepalive:       config.Keepalive,
		ExecuteTimeout:  config.ExecuteTimeout,
		HandlerRegistry: NewHandlerRegistry(userRunsCC),
		ACLProvider:     aclProvider,
		sccp:            sccp,
	}

	// Keep TestQueries working
	if !config.TLSEnabled {
		certGenerator = nil
	}

	cs.Runtime = &ContainerRuntime{
		CertGenerator: certGenerator,
		Processor:     processor,
		CACert:        caCert,
		PeerAddress:   peerAddress,
		CommonEnv: []string{
			"CORE_CHAINCODE_LOGGING_LEVEL=" + config.LogLevel,
			"CORE_CHAINCODE_LOGGING_SHIM=" + config.ShimLogLevel,
			"CORE_CHAINCODE_LOGGING_FORMAT=" + config.LogFormat,
		},
	}

	cs.Launcher = &RuntimeLauncher{
		Runtime:         cs.Runtime,
		Registry:        cs.HandlerRegistry,
		PackageProvider: packageProvider,
		Lifecycle:       &Lifecycle{Executor: cs},
		StartupTimeout:  config.StartupTimeout,
	}

	return cs
}

// Launch starts executing chaincode if it is not already running. This method
// blocks until the peer side handler gets into ready state or encounters a fatal
// error. If the chaincode is already running, it simply returns.
func (cs *ChaincodeSupport) Launch(ctx context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) error {
	cname := cccid.GetCanonicalName()
	if cs.HandlerRegistry.Handler(cname) != nil {
		return nil
	}

	// TODO: There has to be a better way to do this...
	if cs.UserRunsCC && !cccid.Syscc {
		chaincodeLogger.Error(
			"You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?",
		)
	}

	// This is hacky. The only user of this context value is the in-process controller
	// used to support system chaincode. It should really be instantiated with the
	// appropriate reference to ChaincodeSupport.
	ctx = context.WithValue(ctx, ccintf.GetCCHandlerKey(), cs)

	return cs.Launcher.Launch(ctx, cccid, spec)
}

// Stop stops a chaincode if running.
func (cs *ChaincodeSupport) Stop(ctx context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	cname := cccid.GetCanonicalName()
	defer cs.HandlerRegistry.Deregister(cname)

	err := cs.Runtime.Stop(ctx, cccid, cds)
	if err != nil {
		return err
	}

	return nil
}

// HandleChaincodeStream implements ccintf.HandleChaincodeStream for all vms to call with appropriate stream
func (cs *ChaincodeSupport) HandleChaincodeStream(ctxt context.Context, stream ccintf.ChaincodeStream) error {
	deadline, ok := ctxt.Deadline()
	chaincodeLogger.Debugf("Current context deadline = %s, ok = %v", deadline, ok)

	handler := &Handler{
		Invoker:                    cs,
		DefinitionGetter:           &Lifecycle{Executor: cs},
		Keepalive:                  cs.Keepalive,
		Registry:                   cs.HandlerRegistry,
		ACLProvider:                cs.ACLProvider,
		TXContexts:                 NewTransactionContexts(),
		ActiveTransactions:         NewActiveTransactions(),
		SystemCCProvider:           cs.sccp,
		SystemCCVersion:            util.GetSysCCVersion(),
		InstantiationPolicyChecker: CheckInstantiationPolicyFunc(ccprovider.CheckInstantiationPolicy),
		QueryResponseBuilder:       &QueryResponseGenerator{MaxResultLimit: 100},
		UUIDGenerator:              UUIDGeneratorFunc(util.GenerateUUID),
		LedgerGetter:               peer.Default,
	}

	return handler.ProcessStream(stream)
}

// Register the bidi stream entry point called by chaincode to register with the Peer.
func (cs *ChaincodeSupport) Register(stream pb.ChaincodeSupport_RegisterServer) error {
	return cs.HandleChaincodeStream(stream.Context(), stream)
}

// createCCMessage creates a transaction message.
func createCCMessage(messageType pb.ChaincodeMessage_Type, cid string, txid string, cMsg *pb.ChaincodeInput) (*pb.ChaincodeMessage, error) {
	payload, err := proto.Marshal(cMsg)
	if err != nil {
		return nil, err
	}
	ccmsg := &pb.ChaincodeMessage{
		Type:      messageType,
		Payload:   payload,
		Txid:      txid,
		ChannelId: cid,
	}
	return ccmsg, nil
}

// Execute invokes chaincode and returns the original response.
func (cs *ChaincodeSupport) Execute(ctxt context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) (*pb.Response, *pb.ChaincodeEvent, error) {
	resp, err := cs.Invoke(ctxt, cccid, spec)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to execute transaction %s", cccid.TxID)
	}
	if resp == nil {
		return nil, nil, errors.Errorf("nil response from transaction %s", cccid.TxID)
	}

	if resp.ChaincodeEvent != nil {
		resp.ChaincodeEvent.ChaincodeId = cccid.Name
		resp.ChaincodeEvent.TxId = cccid.TxID
	}

	switch resp.Type {
	case pb.ChaincodeMessage_COMPLETED:
		res := &pb.Response{}
		err := proto.Unmarshal(resp.Payload, res)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to unmarshal response for transaction %s", cccid.TxID)
		}
		return res, resp.ChaincodeEvent, nil

	case pb.ChaincodeMessage_ERROR:
		return nil, resp.ChaincodeEvent, errors.Errorf("transaction returned with failure: %s", resp.Payload)

	default:
		return nil, nil, errors.Errorf("unexpected response type %d for transaction %s", resp.Type, cccid.TxID)
	}
}

// Invoke will invoke chaincode and return the message containing the response.
// The chaincode will be launched if it is not already running.
func (cs *ChaincodeSupport) Invoke(ctxt context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) (*pb.ChaincodeMessage, error) {
	var cctyp pb.ChaincodeMessage_Type
	switch spec.(type) {
	case *pb.ChaincodeDeploymentSpec:
		cctyp = pb.ChaincodeMessage_INIT
	case *pb.ChaincodeInvocationSpec:
		cctyp = pb.ChaincodeMessage_TRANSACTION
	default:
		return nil, errors.New("a deployment or invocation spec is required")
	}

	chaincodeSpec := spec.GetChaincodeSpec()
	if chaincodeSpec == nil {
		return nil, errors.New("chaincode spec is nil")
	}

	err := cs.Launch(ctxt, cccid, spec)
	if err != nil {
		return nil, err
	}

	input := chaincodeSpec.Input
	input.Decorations = cccid.ProposalDecorations
	ccMsg, err := createCCMessage(cctyp, cccid.ChainID, cccid.TxID, input)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create chaincode message")
	}

	return cs.execute(ctxt, cccid, ccMsg)
}

// execute executes a transaction and waits for it to complete until a timeout value.
func (cs *ChaincodeSupport) execute(ctxt context.Context, cccid *ccprovider.CCContext, msg *pb.ChaincodeMessage) (*pb.ChaincodeMessage, error) {
	cname := cccid.GetCanonicalName()
	chaincodeLogger.Debugf("canonical name: %s", cname)

	handler := cs.HandlerRegistry.Handler(cname)
	if handler == nil {
		chaincodeLogger.Debugf("chaincode is not running: %s", cname)
		return nil, errors.Errorf("unable to invoke chaincode %s", cname)
	}

	ccresp, err := handler.Execute(ctxt, cccid, msg, cs.ExecuteTimeout)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error sending"))
	}

	return ccresp, nil
}
