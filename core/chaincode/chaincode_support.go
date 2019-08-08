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
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	pb "github.com/hyperledger/fabric/protos/peer"
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
	Start(ccid ccintf.CCID) error
	Stop(ccid ccintf.CCID) error
	Wait(ccid ccintf.CCID) (int, error)
}

// Launcher is used to launch chaincode runtimes.
type Launcher interface {
	Launch(ccid ccintf.CCID) error
}

// Lifecycle provides a way to retrieve chaincode definitions and the packages necessary to run them
type Lifecycle interface {
	// ChaincodeDefinition returns the details for a chaincode by name
	ChaincodeDefinition(channelID, chaincodeName string, qe ledger.SimpleQueryExecutor) (ccprovider.ChaincodeDefinition, error)
}

// LegacyChaincodeDefinitions need to perform security checks whenever a chaincode definition
// is looked up.  These security checks include checking the instantiation policy, as well
// as performing the chaincode fingerprint matching, and other checks implemented in the legacy
// packaging.
type LegacyChaincodeDefinition interface {
	ExecuteLegacySecurityChecks() error
}

// ChaincodeSupport responsible for providing interfacing with chaincodes from the Peer.
type ChaincodeSupport struct {
	ACLProvider            ACLProvider
	AppConfig              ApplicationConfigRetriever
	BuiltinSCCs            scc.BuiltinSCCs
	DeployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	ExecuteTimeout         time.Duration
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
func (cs *ChaincodeSupport) Launch(ccid ccintf.CCID) (*Handler, error) {
	if h := cs.HandlerRegistry.Handler(ccid); h != nil {
		return h, nil
	}

	if err := cs.Launcher.Launch(ccid); err != nil {
		return nil, errors.Wrapf(err, "could not launch chaincode %s", ccid)
	}

	h := cs.HandlerRegistry.Handler(ccid)
	if h == nil {
		return nil, errors.Errorf("claimed to start chaincode container for %s but could not find handler", ccid)
	}

	return h, nil
}

// LaunchInProc is a stopgap solution to be called by the inproccontroller to allow system chaincodes to register
func (cs *ChaincodeSupport) LaunchInProc(ccid ccintf.CCID) <-chan struct{} {
	launchStatus, ok := cs.HandlerRegistry.Launching(ccid)
	if ok {
		chaincodeLogger.Panicf("attempted to launch a system chaincode which has already been launched")
	}

	return launchStatus.Done()
}

// HandleChaincodeStream implements ccintf.HandleChaincodeStream for all vms to call with appropriate stream
func (cs *ChaincodeSupport) HandleChaincodeStream(stream ccintf.ChaincodeStream) error {
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
func (cs *ChaincodeSupport) ExecuteLegacyInit(txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, spec *pb.ChaincodeDeploymentSpec) (*pb.Response, *pb.ChaincodeEvent, error) {
	// FIXME: this is a hack, we shouldn't construct the
	// packageID manually but rather let lifecycle construct it
	// for us. However this is legacy code that will disappear
	// so it is acceptable for now (FAB-14627)
	ccid := ccintf.CCID(cccid.Name + ":" + cccid.Version)

	h, err := cs.Launch(ccid)
	if err != nil {
		return nil, nil, err
	}

	resp, err := cs.execute(pb.ChaincodeMessage_INIT, txParams, cccid, spec.GetChaincodeSpec().Input, h)
	return processChaincodeExecutionResult(txParams.TxID, cccid.Name, resp, err)
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
	chaincodeLogger.Debugf("[%s] getting chaincode data for %s on channel %s", shorttxid(txParams.TxID), chaincodeName, txParams.ChannelID)
	ccid, ccContext, err := cs.CheckInvocation(txParams.ChannelID, chaincodeName, txParams.TXSimulator)
	if err != nil {
		return nil, errors.WithMessage(err, "invalid invocation")
	}

	h, err := cs.Launch(ccintf.CCID(ccid))
	if err != nil {
		return nil, err
	}

	isInit, err := cs.CheckInit(txParams, ccContext, input)
	if err != nil {
		return nil, err
	}

	cctype := pb.ChaincodeMessage_TRANSACTION
	if isInit {
		cctype = pb.ChaincodeMessage_INIT
	}

	return cs.execute(cctype, txParams, ccContext, input, h)
}

func (cs *ChaincodeSupport) CheckInvocation(channelID, chaincodeName string, txSim ledger.SimpleQueryExecutor) (ccid string, ccContext *ccprovider.CCContext, err error) {
	cd, err := cs.Lifecycle.ChaincodeDefinition(channelID, chaincodeName, txSim)
	if err != nil {
		logDevModeError(cs.UserRunsCC)
		return "", nil, errors.Wrapf(err, "[channel %s] failed to get chaincode container info for %s", channelID, chaincodeName)
	}

	if legacyDefinition, ok := cd.(LegacyChaincodeDefinition); ok {
		err = legacyDefinition.ExecuteLegacySecurityChecks()
		if err != nil {
			return "", nil, errors.WithMessagef(err, "[channel %s] failed the chaincode security checks for %s", channelID, chaincodeName)
		}
	}

	return cd.CCID(), &ccprovider.CCContext{
		Name:         chaincodeName,
		Version:      cd.CCVersion(),
		InitRequired: cd.RequiresInit(),
		ID:           cd.Hash(),
	}, nil
}

func (cs *ChaincodeSupport) CheckInit(txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, input *pb.ChaincodeInput) (bool, error) {
	if txParams.ChannelID == "" {
		// Channel-less invocations must be for SCCs, so, we ignore them for now
		return false, nil
	}

	ac, ok := cs.AppConfig.GetApplicationConfig(txParams.ChannelID)
	if !ok {
		return false, errors.Errorf("could not retrieve application config for channel '%s'", txParams.ChannelID)
	}

	if !ac.Capabilities().LifecycleV20() {
		return false, nil
	}

	if !cccid.InitRequired {
		// If Init is not required, treat this as a normal invocation
		// i.e. execute Invoke with 'init' as the function name
		return false, nil
	}

	// At this point, we know we must enforce init exactly once semantics

	value, err := txParams.TXSimulator.GetState(cccid.Name, InitializedKeyName)
	if err != nil {
		return false, errors.WithMessage(err, "could not get 'initialized' key")
	}

	needsInitialization := !bytes.Equal(value, []byte(cccid.Version))

	switch {
	case !input.IsInit && !needsInitialization:
		return false, nil
	case !input.IsInit && needsInitialization:
		return false, errors.Errorf("chaincode '%s' has not been initialized for this version, must call as init first", cccid.Name)
	case input.IsInit && !needsInitialization:
		return false, errors.Errorf("chaincode '%s' is already initialized but called as init", cccid.Name)
	default:
		// input.IsInit && needsInitialization:
		err = txParams.TXSimulator.SetState(cccid.Name, InitializedKeyName, []byte(cccid.Version))
		if err != nil {
			return false, errors.WithMessage(err, "could not set 'initialized' key")
		}
		return true, nil
	}
}

// execute executes a transaction and waits for it to complete until a timeout value.
func (cs *ChaincodeSupport) execute(cctyp pb.ChaincodeMessage_Type, txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, input *pb.ChaincodeInput, h *Handler) (*pb.ChaincodeMessage, error) {
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

	ccresp, err := h.Execute(txParams, cccid, ccMsg, cs.ExecuteTimeout)
	if err != nil {
		return nil, errors.WithMessage(err, "error sending")
	}

	return ccresp, nil
}

func logDevModeError(userRunsCC bool) {
	if userRunsCC {
		chaincodeLogger.Error("You are attempting to perform an action other than Deploy on Chaincode that is not ready and you are in developer mode. Did you forget to Deploy your chaincode?")
	}
}
