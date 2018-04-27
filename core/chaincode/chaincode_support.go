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
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// CertGenerator generate client certificates for chaincode
type CertGenerator interface {
	// Generate returns a certificate and private key and associates
	// the hash of the certificates with the given chaincode name
	Generate(ccName string) (*accesscontrol.CertAndPrivKeyPair, error)
}

// Runtime is used to manage chaincode runtime instances.
type Runtime interface {
	Start(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error
	Stop(ctxt context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error
}

// PackageProvider is responsible for getting the chaincode package from
// the filesystem.
type PackageProvider interface {
	GetChaincode(ccname string, ccversion string) (ccprovider.CCPackage, error)
}

// NewChaincodeSupport creates a new ChaincodeSupport instance.
func NewChaincodeSupport(
	config *Config,
	peerAddress string,
	userrunsCC bool,
	ccstartuptimeout time.Duration,
	caCert []byte,
	certGenerator CertGenerator,
	packageProvider PackageProvider,
	aclProvider ACLProvider,
) *ChaincodeSupport {
	cs := &ChaincodeSupport{
		caCert:           caCert,
		peerNetworkID:    config.PeerNetworkID,
		peerID:           config.PeerID,
		userRunsCC:       userrunsCC,
		ccStartupTimeout: ccstartuptimeout,
		keepalive:        config.Keepalive,
		executetimeout:   config.ExecuteTimeout,
		HandlerRegistry:  NewHandlerRegistry(userrunsCC),
		PackageProvider:  packageProvider,
		ACLProvider:      aclProvider,
	}

	// Keep TestQueries working
	if !config.TLSEnabled {
		certGenerator = nil
	}

	cs.ContainerRuntime = &ContainerRuntime{
		CertGenerator: certGenerator,
		Processor:     ProcessFunc(container.VMCProcess),
		CACert:        caCert,
		PeerAddress:   peerAddress,
		PeerID:        config.PeerID,
		PeerNetworkID: config.PeerNetworkID,
		CommonEnv: []string{
			"CORE_CHAINCODE_LOGGING_LEVEL=" + config.LogLevel,
			"CORE_CHAINCODE_LOGGING_SHIM=" + config.ShimLogLevel,
			"CORE_CHAINCODE_LOGGING_FORMAT=" + config.LogFormat,
		},
	}

	return cs
}

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	caCert           []byte
	peerAddress      string
	ccStartupTimeout time.Duration
	peerNetworkID    string
	peerID           string
	keepalive        time.Duration
	executetimeout   time.Duration
	userRunsCC       bool
	ContainerRuntime Runtime
	PackageProvider  PackageProvider
	ACLProvider      ACLProvider
	HandlerRegistry  *HandlerRegistry
	sccp             sysccprovider.SystemChaincodeProvider
}

// SetSysCCProvider is a bit of a hack to make a latent dependency of ChaincodeSupport
// be an explicit dependency.  Because the chaincode support must be registered before
// the sysccprovider implementation can be created, we cannot make the sccp part of the
// constructor for ChaincodeSupport.
func (cs *ChaincodeSupport) SetSysCCProvider(sccp sysccprovider.SystemChaincodeProvider) {
	cs.sccp = sccp
}

// launchAndWaitForReady launches a container for the specified chaincode
// context if one is not already running. It then waits for the chaincode
// registration to complete or for the process to time out.
func (cs *ChaincodeSupport) launchAndWaitForReady(ctx context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	cname := cccid.GetCanonicalName()
	ready, err := cs.HandlerRegistry.Launching(cname)
	if err != nil {
		return err
	}

	// This is hacky. The only user of this context value is the in-process controller
	// used to support system chaincode. It should really be instantiated with the
	// appropriate reference to ChaincodeSupport.
	launchCtx := context.WithValue(ctx, ccintf.GetCCHandlerKey(), cs)

	launchFail := make(chan error, 1)
	go func() {
		chaincodeLogger.Debugf("chaincode %s is being launched", cname)
		err := cs.ContainerRuntime.Start(launchCtx, cccid, cds)
		if err != nil {
			launchFail <- errors.WithMessage(err, "error starting container")
		}
	}()

	select {
	case <-ready:
	case err = <-launchFail:
	case <-time.After(cs.ccStartupTimeout):
		err = errors.Errorf("timeout expired while starting chaincode %s for transaction %s", cname, cccid.TxID)
	}

	if err != nil {
		chaincodeLogger.Debugf("stopping due to error while launching: %+v", err)
		if err := cs.Stop(ctx, cccid, cds); err != nil {
			chaincodeLogger.Debugf("stop failed: %+v", err)
		}
		return err
	}

	return nil
}

// Stop stops a chaincode if running.
func (cs *ChaincodeSupport) Stop(ctx context.Context, cccid *ccprovider.CCContext, cds *pb.ChaincodeDeploymentSpec) error {
	cname := cccid.GetCanonicalName()
	defer cs.HandlerRegistry.Deregister(cname)

	err := cs.ContainerRuntime.Stop(ctx, cccid, cds)
	if err != nil {
		return err
	}

	return nil
}

// Launch will launch the chaincode if not running (if running return nil) and will wait for handler of the chaincode to get into ready state.
func (cs *ChaincodeSupport) Launch(context context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) (*pb.ChaincodeInput, error) {
	cname := cccid.GetCanonicalName()
	cID := spec.GetChaincodeSpec().ChaincodeId
	cMsg := spec.GetChaincodeSpec().Input

	if cs.HandlerRegistry.Handler(cname) != nil {
		return cMsg, nil
	}

	cds, _ := spec.(*pb.ChaincodeDeploymentSpec)
	if cds == nil {
		if cccid.Syscc {
			return cMsg, errors.Errorf("a syscc should be running (it cannot be launched) %s", cname)
		}

		if cs.userRunsCC {
			chaincodeLogger.Error("You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?")
		}

		//hopefully we are restarting from existing image and the deployed transaction exists
		//(this will also validate the ID from the LSCC if we're not using the config-tree approach)
		depPayload, err := cs.GetCDS(context, cccid.TxID, cccid.SignedProposal, cccid.Proposal, cccid.ChainID, cID.Name)
		if err != nil {
			return cMsg, errors.WithMessage(err, fmt.Sprintf("could not get ChaincodeDeploymentSpec for %s", cname))
		}
		if depPayload == nil {
			return cMsg, errors.WithMessage(err, fmt.Sprintf("nil ChaincodeDeploymentSpec for %s", cname))
		}

		cds = &pb.ChaincodeDeploymentSpec{}

		//Get lang from original deployment
		err = proto.Unmarshal(depPayload, cds)
		if err != nil {
			return cMsg, errors.Wrap(err, fmt.Sprintf("failed to unmarshal deployment transactions for %s", cname))
		}
	}

	//from here on : if we launch the container and get an error, we need to stop the container

	//launch container if it is a System container or not in dev mode
	if !cs.userRunsCC || cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM {
		//NOTE-We need to streamline code a bit so the data from LSCC gets passed to this thus
		//avoiding the need to go to the FS. In particular, we should use cdsfs completely. It is
		//just a vestige of old protocol that we continue to use ChaincodeDeploymentSpec for
		//anything other than Install. In particular, instantiate, invoke, upgrade should be using
		//just some form of ChaincodeInvocationSpec.
		//
		//But for now, if we are invoking we have gone through the LSCC path above. If  instantiating
		//or upgrading currently we send a CDS with nil CodePackage. In this case the codepath
		//in the endorser has gone through LSCC validation. Just get the code from the FS.
		if cds.CodePackage == nil {
			//no code bytes for these situations
			if !(cs.userRunsCC || cds.ExecEnv == pb.ChaincodeDeploymentSpec_SYSTEM) {
				ccpack, err := cs.PackageProvider.GetChaincode(cID.Name, cID.Version)
				if err != nil {
					return cMsg, err
				}

				cds = ccpack.GetDepSpec()
				chaincodeLogger.Debugf("launchAndWaitForReady fetched %d bytes from file system", len(cds.CodePackage))
			}
		}

		err := cs.launchAndWaitForReady(context, cccid, cds)
		if err != nil {
			chaincodeLogger.Errorf("launchAndWaitForReady failed: %+v", err)
			return cMsg, err
		}
	}

	chaincodeLogger.Debug("launch complete")

	return cMsg, nil
}

// HandleChaincodeStream implements ccintf.HandleChaincodeStream for all vms to call with appropriate stream
func (cs *ChaincodeSupport) HandleChaincodeStream(ctxt context.Context, stream ccintf.ChaincodeStream) error {
	return HandleChaincodeStream(cs, ctxt, stream)
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

// execute executes a transaction and waits for it to complete until a timeout value.
func (cs *ChaincodeSupport) execute(ctxt context.Context, cccid *ccprovider.CCContext, msg *pb.ChaincodeMessage, timeout time.Duration) (*pb.ChaincodeMessage, error) {
	cname := cccid.GetCanonicalName()
	chaincodeLogger.Debugf("canonical name: %s", cname)

	handler := cs.HandlerRegistry.Handler(cname)
	if handler == nil {
		chaincodeLogger.Debugf("chaincode is not running: %s", cname)
		return nil, errors.Errorf("unable to invoke chaincode %s", cname)
	}

	ccresp, err := handler.Execute(ctxt, cccid, msg, timeout)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error sending"))
	}

	return ccresp, nil
}

//Execute - execute proposal, return original response of chaincode
func (cs *ChaincodeSupport) ExecuteSpec(ctxt context.Context, cccid *ccprovider.CCContext, spec ccprovider.ChaincodeSpecGetter) (*pb.Response, *pb.ChaincodeEvent, error) {
	var cctyp pb.ChaincodeMessage_Type

	switch spec.(type) {
	case *pb.ChaincodeDeploymentSpec: // init
		cctyp = pb.ChaincodeMessage_INIT
	case *pb.ChaincodeInvocationSpec: // invoke
		cctyp = pb.ChaincodeMessage_TRANSACTION
	default:
		return nil, nil, errors.New("a deployment or invocation spec is required")
	}

	cMsg, err := cs.Launch(ctxt, cccid, spec)
	if err != nil {
		return nil, nil, err
	}

	cMsg.Decorations = cccid.ProposalDecorations
	ccMsg, err := createCCMessage(cctyp, cccid.ChainID, cccid.TxID, cMsg)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed to create chaincode message")
	}

	resp, err := cs.execute(ctxt, cccid, ccMsg, cs.executetimeout)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed to execute transaction %s")
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

// GetCDS retrieves a chaincode deployment spec for the required chaincode
func (cs *ChaincodeSupport) GetCDS(ctxt context.Context, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chainID string, chaincodeID string) ([]byte, error) {
	version := util.GetSysCCVersion()
	cccid := ccprovider.NewCCContext(chainID, "lscc", version, txid, true, signedProp, prop)

	args := util.ToChaincodeArgs("getdepspec", chainID, chaincodeID)
	res, _, err := cs.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		return nil, errors.Wrapf(err, "getdepspec %s/%s", chainID, chaincodeID)
	}
	if res.Status != shim.OK {
		return nil, errors.Errorf("getdepspec %s/%s: %s", chainID, chaincodeID, res.Message)
	}

	return res.Payload, nil
}

// GetChaincodeDefinition returns a resourcesconfig.ChaincodeDefinition for the chaincode
// associated with the provided channel and name.
func (cs *ChaincodeSupport) GetChaincodeDefinition(ctxt context.Context, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal, chainID string, chaincodeID string) (ccprovider.ChaincodeDefinition, error) {
	version := util.GetSysCCVersion()
	cccid := ccprovider.NewCCContext(chainID, "lscc", version, txid, true, signedProp, prop)

	args := util.ToChaincodeArgs("getccdata", chainID, chaincodeID)
	res, _, err := cs.ExecuteChaincode(ctxt, cccid, args)
	if err != nil {
		return nil, errors.Wrapf(err, "getccdata %s/%s", chainID, chaincodeID)
	}
	if res.Status != shim.OK {
		return nil, errors.Errorf("getccdata %s/%s: %s", chainID, chaincodeID, res.Message)
	}

	cd := &ccprovider.ChaincodeData{}
	err = proto.Unmarshal(res.Payload, cd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal chaincode definition")
	}

	return cd, nil
}

// ExecuteChaincode invokes chaincode with the provided arguments.
func (cs *ChaincodeSupport) ExecuteChaincode(ctxt context.Context, cccid *ccprovider.CCContext, args [][]byte) (*pb.Response, *pb.ChaincodeEvent, error) {
	invocationSpec := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			Type:        pb.ChaincodeSpec_GOLANG,
			ChaincodeId: &pb.ChaincodeID{Name: cccid.Name},
			Input:       &pb.ChaincodeInput{Args: args},
		},
	}

	res, ccevent, err := cs.ExecuteSpec(ctxt, cccid, invocationSpec)
	if err != nil {
		err = errors.WithMessage(err, "error invoking chaincode")
		chaincodeLogger.Errorf("%+v", err)
		return nil, nil, err
	}

	return res, ccevent, err
}
