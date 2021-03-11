/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/extcc"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
)

const (
	// InitializedKeyName is the reserved key in a chaincode's namespace which
	// records the ID of the chaincode which initialized the namespace.
	// In this way, we can enforce Init exactly once semantics, whenever
	// the backing chaincode bytes change (but not be required to re-initialize
	// the chaincode say, when endorsement policy changes).
	InitializedKeyName = "\x00" + string(utf8.MaxRune) + "initialized"
)

// Runtime is used to manage chaincode runtime instances.
type Runtime interface {
	Build(ccid string) (*ccintf.ChaincodeServerInfo, error)
	Start(ccid string, ccinfo *ccintf.PeerConnection) error
	Stop(ccid string) error
	Wait(ccid string) (int, error)
}

// Launcher is used to launch chaincode runtimes.
type Launcher interface {
	Launch(ccid string, streamHandler extcc.StreamHandler) error
	Stop(ccid string) error
}

// Lifecycle provides a way to retrieve chaincode definitions and the packages necessary to run them
type Lifecycle interface {
	// ChaincodeEndorsementInfo looks up the chaincode info in the given channel.  It is the responsibility
	// of the implementation to add appropriate read dependencies for the information returned.
	ChaincodeEndorsementInfo(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (*lifecycle.ChaincodeEndorsementInfo, error)
}

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	ACLProvider            ACLProvider
	AppConfig              ApplicationConfigRetriever
	BuiltinSCCs            scc.BuiltinSCCs
	DeployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	ExecuteTimeout         time.Duration
	InstallTimeout         time.Duration
	HandlerMetrics         *HandlerMetrics
	HandlerRegistry        *HandlerRegistry
	Keepalive              time.Duration
	Launcher               Launcher
	Lifecycle              Lifecycle
	Peer                   *peer.Peer
	Runtime                Runtime
	TotalQueryLimit        int
	UserRunsCC             bool
}

// Launch starts executing chaincode if it is not already running. This method
// blocks until the peer side handler gets into ready state or encounters a fatal
// error. If the chaincode is already running, it simply returns.
func (cs *ChaincodeSupport) Launch(ccid string) (*Handler, error) {
	if h := cs.HandlerRegistry.Handler(ccid); h != nil {
		return h, nil
	}

	if err := cs.Launcher.Launch(ccid, cs); err != nil {
		return nil, errors.Wrapf(err, "could not launch chaincode %s", ccid)
	}

	h := cs.HandlerRegistry.Handler(ccid)
	if h == nil {
		return nil, errors.Errorf("claimed to start chaincode container for %s but could not find handler", ccid)
	}

	return h, nil
}

// LaunchInProc is a stopgap solution to be called by the inproccontroller to allow system chaincodes to register
func (cs *ChaincodeSupport) LaunchInProc(ccid string) <-chan struct{} {
	launchStatus, ok := cs.HandlerRegistry.Launching(ccid)
	if ok {
		chaincodeLogger.Panicf("attempted to launch a system chaincode which has already been launched")
	}

	return launchStatus.Done()
}

// HandleChaincodeStream implements ccintf.HandleChaincodeStream for all vms to call with appropriate stream
func (cs *ChaincodeSupport) HandleChaincodeStream(stream ccintf.ChaincodeStream) error {
	var deserializerFactory privdata.IdentityDeserializerFactoryFunc = func(channelID string) msp.IdentityDeserializer {
		return cs.Peer.Channel(channelID).MSPManager()
	}
	handler := &Handler{
		Invoker:                cs,
		Keepalive:              cs.Keepalive,
		Registry:               cs.HandlerRegistry,
		ACLProvider:            cs.ACLProvider,
		TXContexts:             NewTransactionContexts(),
		ActiveTransactions:     NewActiveTransactions(),
		BuiltinSCCs:            cs.BuiltinSCCs,
		QueryResponseBuilder:   &QueryResponseGenerator{MaxResultLimit: 100},
		UUIDGenerator:          UUIDGeneratorFunc(util.GenerateUUID),
		LedgerGetter:           cs.Peer,
		IDDeserializerFactory:  deserializerFactory,
		DeployedCCInfoProvider: cs.DeployedCCInfoProvider,
		AppConfig:              cs.AppConfig,
		Metrics:                cs.HandlerMetrics,
		TotalQueryLimit:        cs.TotalQueryLimit,
	}

	return handler.ProcessStream(stream)
}

// Register the bidi stream entry point called by chaincode to register with the Peer.
func (cs *ChaincodeSupport) Register(stream pb.ChaincodeSupport_RegisterServer) error {
	return cs.HandleChaincodeStream(stream)
}

// ExecuteLegacyInit is a temporary method which should be removed once the old style lifecycle
// is entirely deprecated.  Ideally one release after the introduction of the new lifecycle.
// It does not attempt to start the chaincode based on the information from lifecycle, but instead
// accepts the container information directly in the form of a ChaincodeDeploymentSpec.
func (cs *ChaincodeSupport) ExecuteLegacyInit(txParams *ccprovider.TransactionParams, ccName, ccVersion string, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error) {
	// FIXME: this is a hack, we shouldn't construct the
	// ccid manually but rather let lifecycle construct it
	// for us. However this is legacy code that will disappear
	// so it is acceptable for now (FAB-14627)
	ccid := ccName + ":" + ccVersion

	h, err := cs.Launch(ccid)
	if err != nil {
		return nil, nil, err
	}

	resp, err := cs.execute(pb.ChaincodeMessage_INIT, txParams, ccName, input, h)
	return processChaincodeExecutionResult(txParams.TxID, ccName, resp, err)
}

// Execute invokes chaincode and returns the original response.
func (cs *ChaincodeSupport) Execute(txParams *ccprovider.TransactionParams, chaincodeName string, input *pb.ChaincodeInput) (*pb.Response, *pb.ChaincodeEvent, error) {
	resp, err := cs.Invoke(txParams, chaincodeName, input)
	return processChaincodeExecutionResult(txParams.TxID, chaincodeName, resp, err)
}

func processChaincodeExecutionResult(txid, ccName string, resp *pb.ChaincodeMessage, err error) (*pb.Response, *pb.ChaincodeEvent, error) {
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to execute transaction %s", txid)
	}
	if resp == nil {
		return nil, nil, errors.Errorf("nil response from transaction %s", txid)
	}

	if resp.ChaincodeEvent != nil {
		resp.ChaincodeEvent.ChaincodeId = ccName
		resp.ChaincodeEvent.TxId = txid
	}

	switch resp.Type {
	case pb.ChaincodeMessage_COMPLETED:
		res := &pb.Response{}
		err := proto.Unmarshal(resp.Payload, res)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to unmarshal response for transaction %s", txid)
		}
		return res, resp.ChaincodeEvent, nil

	case pb.ChaincodeMessage_ERROR:
		return nil, resp.ChaincodeEvent, errors.Errorf("transaction returned with failure: %s", resp.Payload)

	default:
		return nil, nil, errors.Errorf("unexpected response type %d for transaction %s", resp.Type, txid)
	}
}

// Invoke will invoke chaincode and return the message containing the response.
// The chaincode will be launched if it is not already running.
func (cs *ChaincodeSupport) Invoke(txParams *ccprovider.TransactionParams, chaincodeName string, input *pb.ChaincodeInput) (*pb.ChaincodeMessage, error) {
	ccid, cctype, err := cs.CheckInvocation(txParams, chaincodeName, input)
	if err != nil {
		return nil, errors.WithMessage(err, "invalid invocation")
	}

	h, err := cs.Launch(ccid)
	if err != nil {
		return nil, err
	}

	return cs.execute(cctype, txParams, chaincodeName, input, h)
}

// CheckInvocation inspects the parameters of an invocation and determines if, how, and to where a that invocation should be routed.
// First, we ensure that the target namespace is defined on the channel and invokable on this peer, according to the lifecycle implementation.
// Then, if the chaincode definition requires it, this function enforces 'init exactly once' semantics.
// Finally, it returns the chaincode ID to route to and the message type of the request (normal transaction, or init).
func (cs *ChaincodeSupport) CheckInvocation(txParams *ccprovider.TransactionParams, chaincodeName string, input *pb.ChaincodeInput) (ccid string, cctype pb.ChaincodeMessage_Type, err error) {
	chaincodeLogger.Debugf("[%s] getting chaincode data for %s on channel %s", shorttxid(txParams.TxID), chaincodeName, txParams.ChannelID)
	cii, err := cs.Lifecycle.ChaincodeEndorsementInfo(txParams.ChannelID, chaincodeName, txParams.TXSimulator)
	if err != nil {
		logDevModeError(cs.UserRunsCC)
		return "", 0, errors.Wrapf(err, "[channel %s] failed to get chaincode container info for %s", txParams.ChannelID, chaincodeName)
	}

	needsInitialization := false
	if cii.EnforceInit {

		value, err := txParams.TXSimulator.GetState(chaincodeName, InitializedKeyName)
		if err != nil {
			return "", 0, errors.WithMessage(err, "could not get 'initialized' key")
		}

		needsInitialization = !bytes.Equal(value, []byte(cii.Version))
	}

	// Note, IsInit is a new field for v2.0 and should only be set for invocations of non-legacy chaincodes.
	// Any invocation of a legacy chaincode with IsInit set will fail.  This is desirable, as the old
	// InstantiationPolicy contract enforces which users may call init.
	if input.IsInit {
		if !cii.EnforceInit {
			return "", 0, errors.Errorf("chaincode '%s' does not require initialization but called as init", chaincodeName)
		}

		if !needsInitialization {
			return "", 0, errors.Errorf("chaincode '%s' is already initialized but called as init", chaincodeName)
		}

		err = txParams.TXSimulator.SetState(chaincodeName, InitializedKeyName, []byte(cii.Version))
		if err != nil {
			return "", 0, errors.WithMessage(err, "could not set 'initialized' key")
		}

		return cii.ChaincodeID, pb.ChaincodeMessage_INIT, nil
	}

	if needsInitialization {
		return "", 0, errors.Errorf("chaincode '%s' has not been initialized for this version, must call as init first", chaincodeName)
	}

	return cii.ChaincodeID, pb.ChaincodeMessage_TRANSACTION, nil
}

// execute executes a transaction and waits for it to complete until a timeout value.
func (cs *ChaincodeSupport) execute(cctyp pb.ChaincodeMessage_Type, txParams *ccprovider.TransactionParams, namespace string, input *pb.ChaincodeInput, h *Handler) (*pb.ChaincodeMessage, error) {
	input.Decorations = txParams.ProposalDecorations

	payload, err := proto.Marshal(input)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create chaincode message")
	}

	ccMsg := &pb.ChaincodeMessage{
		Type:      cctyp,
		Payload:   payload,
		Txid:      txParams.TxID,
		ChannelId: txParams.ChannelID,
	}

	timeout := cs.executeTimeout(namespace, input)
	ccresp, err := h.Execute(txParams, namespace, ccMsg, timeout)
	if err != nil {
		return nil, errors.WithMessage(err, "error sending")
	}

	return ccresp, nil
}

func (cs *ChaincodeSupport) executeTimeout(namespace string, input *pb.ChaincodeInput) time.Duration {
	operation := chaincodeOperation(input.Args)
	switch {
	case namespace == "lscc" && operation == "install":
		return maxDuration(cs.InstallTimeout, cs.ExecuteTimeout)
	case namespace == lifecycle.LifecycleNamespace && operation == lifecycle.InstallChaincodeFuncName:
		return maxDuration(cs.InstallTimeout, cs.ExecuteTimeout)
	default:
		return cs.ExecuteTimeout
	}
}

func maxDuration(durations ...time.Duration) time.Duration {
	var result time.Duration
	for _, d := range durations {
		if d > result {
			result = d
		}
	}
	return result
}

func chaincodeOperation(args [][]byte) string {
	if len(args) == 0 {
		return ""
	}
	return string(args[0])
}

func logDevModeError(userRunsCC bool) {
	if userRunsCC {
		chaincodeLogger.Error("You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?")
	}
}
