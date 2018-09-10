/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

var chaincodeLogger = flogging.MustGetLogger("chaincode")

// An ACLProvider performs access control checks when invoking
// chaincode.
type ACLProvider interface {
	CheckACL(resName string, channelID string, idinfo interface{}) error
}

// A Registry is responsible for tracking handlers.
type Registry interface {
	Register(*Handler) error
	Ready(cname string)
	Failed(cname string, err error)
	Deregister(cname string) error
}

// An Invoker invokes chaincode.
type Invoker interface {
	Invoke(txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, spec *pb.ChaincodeInput) (*pb.ChaincodeMessage, error)
}

// SystemCCProvider provides system chaincode metadata.
type SystemCCProvider interface {
	IsSysCC(name string) bool
	IsSysCCAndNotInvokableCC2CC(name string) bool
}

// TransactionRegistry tracks active transactions for each channel.
type TransactionRegistry interface {
	Add(channelID, txID string) bool
	Remove(channelID, txID string)
}

// A ContextRegistry is responsible for managing transaction contexts.
type ContextRegistry interface {
	Create(txParams *ccprovider.TransactionParams) (*TransactionContext, error)
	Get(chainID, txID string) *TransactionContext
	Delete(chainID, txID string)
	Close()
}

// InstantiationPolicyChecker is used to evaluate instantiation policies.
type InstantiationPolicyChecker interface {
	CheckInstantiationPolicy(name, version string, cd *ccprovider.ChaincodeData) error
}

// Adapter from function to InstantiationPolicyChecker interface.
type CheckInstantiationPolicyFunc func(name, version string, cd *ccprovider.ChaincodeData) error

func (c CheckInstantiationPolicyFunc) CheckInstantiationPolicy(name, version string, cd *ccprovider.ChaincodeData) error {
	return c(name, version, cd)
}

// QueryResponseBuilder is responsible for building QueryResponse messages for query
// transactions initiated by chaincode.
type QueryResponseBuilder interface {
	BuildQueryResponse(txContext *TransactionContext, iter commonledger.ResultsIterator,
		iterID string, isPaginated bool, totalReturnLimit int32) (*pb.QueryResponse, error)
}

// ChaincodeDefinitionGetter is responsible for retrieving a chaincode definition
// from the system. The definition is used by the InstantiationPolicyChecker.
type ChaincodeDefinitionGetter interface {
	ChaincodeDefinition(chaincodeName string, txSim ledger.QueryExecutor) (ccprovider.ChaincodeDefinition, error)
}

// LedgerGetter is used to get ledgers for chaincode.
type LedgerGetter interface {
	GetLedger(cid string) ledger.PeerLedger
}

// UUIDGenerator is responsible for creating unique query identifiers.
type UUIDGenerator interface {
	New() string
}
type UUIDGeneratorFunc func() string

func (u UUIDGeneratorFunc) New() string { return u() }

// ApplicationConfigRetriever to retrieve the application configuration for a channel
type ApplicationConfigRetriever interface {
	// GetApplicationConfig returns the channelconfig.Application for the channel
	// and whether the Application config exists
	GetApplicationConfig(cid string) (channelconfig.Application, bool)
}

// Handler implements the peer side of the chaincode stream.
type Handler struct {
	// Keepalive specifies the interval at which keep-alive messages are sent.
	Keepalive time.Duration
	// SystemCCVersion specifies the current system chaincode version
	SystemCCVersion string
	// DefinitionGetter is used to retrieve the chaincode definition from the
	// Lifecycle System Chaincode.
	DefinitionGetter ChaincodeDefinitionGetter
	// Invoker is used to invoke chaincode.
	Invoker Invoker
	// Registry is used to track active handlers.
	Registry Registry
	// ACLProvider is used to check if a chaincode invocation should be allowed.
	ACLProvider ACLProvider
	// TXContexts is a collection of TransactionContext instances
	// that are accessed by channel name and transaction ID.
	TXContexts ContextRegistry
	// activeTransactions holds active transaction identifiers.
	ActiveTransactions TransactionRegistry
	// SystemCCProvider provides access to system chaincode metadata
	SystemCCProvider SystemCCProvider
	// InstantiationPolicyChecker is used to evaluate the chaincode instantiation policies.
	InstantiationPolicyChecker InstantiationPolicyChecker
	// QueryResponeBuilder is used to build query responses
	QueryResponseBuilder QueryResponseBuilder
	// LedgerGetter is used to get the ledger associated with a channel
	LedgerGetter LedgerGetter
	// UUIDGenerator is used to generate UUIDs
	UUIDGenerator UUIDGenerator
	// AppConfig is used to retrieve the application config for a channel
	AppConfig ApplicationConfigRetriever

	// state holds the current handler state. It will be created, established, or
	// ready.
	state State
	// chaincodeID holds the ID of the chaincode that registered with the peer.
	chaincodeID *pb.ChaincodeID
	// ccInstances holds information about the chaincode instance associated with
	// the peer.
	ccInstance *sysccprovider.ChaincodeInstance

	// serialLock is used to serialize sends across the grpc chat stream.
	serialLock sync.Mutex
	// chatStream is the bidirectional grpc stream used to communicate with the
	// chaincode instance.
	chatStream ccintf.ChaincodeStream
	// errChan is used to communicate errors from the async send to the receive loop
	errChan chan error
}

// handleMessage is called by ProcessStream to dispatch messages.
func (h *Handler) handleMessage(msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("[%s] Fabric side handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.Txid), msg.Type, h.state)

	if msg.Type == pb.ChaincodeMessage_KEEPALIVE {
		return nil
	}

	switch h.state {
	case Created:
		return h.handleMessageCreatedState(msg)
	case Ready:
		return h.handleMessageReadyState(msg)
	default:
		return errors.Errorf("handle message: invalid state %s for transaction %s", h.state, msg.Txid)
	}
}

func (h *Handler) handleMessageCreatedState(msg *pb.ChaincodeMessage) error {
	switch msg.Type {
	case pb.ChaincodeMessage_REGISTER:
		h.HandleRegister(msg)
	default:
		return fmt.Errorf("[%s] Fabric side handler cannot handle message (%s) while in created state", msg.Txid, msg.Type)
	}
	return nil
}

func (h *Handler) handleMessageReadyState(msg *pb.ChaincodeMessage) error {
	switch msg.Type {
	case pb.ChaincodeMessage_COMPLETED, pb.ChaincodeMessage_ERROR:
		h.Notify(msg)

	case pb.ChaincodeMessage_PUT_STATE:
		go h.HandleTransaction(msg, h.HandlePutState)
	case pb.ChaincodeMessage_DEL_STATE:
		go h.HandleTransaction(msg, h.HandleDelState)
	case pb.ChaincodeMessage_INVOKE_CHAINCODE:
		go h.HandleTransaction(msg, h.HandleInvokeChaincode)

	case pb.ChaincodeMessage_GET_STATE:
		go h.HandleTransaction(msg, h.HandleGetState)
	case pb.ChaincodeMessage_GET_STATE_BY_RANGE:
		go h.HandleTransaction(msg, h.HandleGetStateByRange)
	case pb.ChaincodeMessage_GET_QUERY_RESULT:
		go h.HandleTransaction(msg, h.HandleGetQueryResult)
	case pb.ChaincodeMessage_GET_HISTORY_FOR_KEY:
		go h.HandleTransaction(msg, h.HandleGetHistoryForKey)
	case pb.ChaincodeMessage_QUERY_STATE_NEXT:
		go h.HandleTransaction(msg, h.HandleQueryStateNext)
	case pb.ChaincodeMessage_QUERY_STATE_CLOSE:
		go h.HandleTransaction(msg, h.HandleQueryStateClose)

	case pb.ChaincodeMessage_GET_STATE_METADATA:
		go h.HandleTransaction(msg, h.HandleGetStateMetadata)
	case pb.ChaincodeMessage_PUT_STATE_METADATA:
		go h.HandleTransaction(msg, h.HandlePutStateMetadata)
	default:
		return fmt.Errorf("[%s] Fabric side handler cannot handle message (%s) while in ready state", msg.Txid, msg.Type)
	}

	return nil
}

type MessageHandler interface {
	Handle(*pb.ChaincodeMessage, *TransactionContext) (*pb.ChaincodeMessage, error)
}

type handleFunc func(*pb.ChaincodeMessage, *TransactionContext) (*pb.ChaincodeMessage, error)

// HandleTransaction is a middleware function that obtains and verifies a transaction
// context prior to forwarding the message to the provided delegate. Response messages
// returned by the delegate are sent to the chat stream. Any errors returned by the
// delegate are packaged as chaincode error messages.
func (h *Handler) HandleTransaction(msg *pb.ChaincodeMessage, delegate handleFunc) {
	chaincodeLogger.Debugf("[%s] handling %s from chaincode", shorttxid(msg.Txid), msg.Type.String())
	if !h.registerTxid(msg) {
		return
	}

	var txContext *TransactionContext
	var err error
	if msg.Type == pb.ChaincodeMessage_INVOKE_CHAINCODE {
		txContext, err = h.getTxContextForInvoke(msg.ChannelId, msg.Txid, msg.Payload, "")
	} else {
		txContext, err = h.isValidTxSim(msg.ChannelId, msg.Txid, "no ledger context")
	}

	var resp *pb.ChaincodeMessage
	if err == nil {
		resp, err = delegate(msg, txContext)
	}

	if err != nil {
		err = errors.Wrapf(err, "%s failed: transaction ID: %s", msg.Type, msg.Txid)
		chaincodeLogger.Errorf("[%s] Failed to handle %s. error: %+v", shorttxid(msg.Txid), msg.Type, err)
		resp = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(err.Error()), Txid: msg.Txid, ChannelId: msg.ChannelId}
	}

	chaincodeLogger.Debugf("[%s] Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type, resp.Type)
	h.ActiveTransactions.Remove(msg.ChannelId, msg.Txid)
	h.serialSendAsync(resp)
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

// ParseName parses a chaincode name into a ChaincodeInstance. The name should
// be of the form "chaincode-name:version/channel-name" with optional elements.
func ParseName(ccName string) *sysccprovider.ChaincodeInstance {
	ci := &sysccprovider.ChaincodeInstance{}

	z := strings.SplitN(ccName, "/", 2)
	if len(z) == 2 {
		ci.ChainID = z[1]
	}
	z = strings.SplitN(z[0], ":", 2)
	if len(z) == 2 {
		ci.ChaincodeVersion = z[1]
	}
	ci.ChaincodeName = z[0]

	return ci
}

func (h *Handler) ChaincodeName() string {
	if h.ccInstance == nil {
		return ""
	}
	return h.ccInstance.ChaincodeName
}

// serialSend serializes msgs so gRPC will be happy
func (h *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	h.serialLock.Lock()
	defer h.serialLock.Unlock()

	if err := h.chatStream.Send(msg); err != nil {
		err = errors.WithMessage(err, fmt.Sprintf("[%s] error sending %s", shorttxid(msg.Txid), msg.Type))
		chaincodeLogger.Errorf("%+v", err)
		return err
	}

	return nil
}

// serialSendAsync serves the same purpose as serialSend (serialize msgs so gRPC will
// be happy). In addition, it is also asynchronous so send-remoterecv--localrecv loop
// can be nonblocking. Only errors need to be handled and these are handled by
// communication on supplied error channel. A typical use will be a non-blocking or
// nil channel
func (h *Handler) serialSendAsync(msg *pb.ChaincodeMessage) {
	go func() {
		if err := h.serialSend(msg); err != nil {
			// provide an error response to the caller
			resp := &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_ERROR,
				Payload:   []byte(err.Error()),
				Txid:      msg.Txid,
				ChannelId: msg.ChannelId,
			}
			h.Notify(resp)

			// surface send error to stream processing
			h.errChan <- err
		}
	}()
}

// Check if the transactor is allow to call this chaincode on this channel
func (h *Handler) checkACL(signedProp *pb.SignedProposal, proposal *pb.Proposal, ccIns *sysccprovider.ChaincodeInstance) error {
	// ensure that we don't invoke a system chaincode
	// that is not invokable through a cc2cc invocation
	if h.SystemCCProvider.IsSysCCAndNotInvokableCC2CC(ccIns.ChaincodeName) {
		return errors.Errorf("system chaincode %s cannot be invoked with a cc2cc invocation", ccIns.ChaincodeName)
	}

	// if we are here, all we know is that the invoked chaincode is either
	// - a system chaincode that *is* invokable through a cc2cc
	//   (but we may still have to determine whether the invoker can perform this invocation)
	// - an application chaincode
	//   (and we still need to determine whether the invoker can invoke it)

	if h.SystemCCProvider.IsSysCC(ccIns.ChaincodeName) {
		// Allow this call
		return nil
	}

	// A Nil signedProp will be rejected for non-system chaincodes
	if signedProp == nil {
		return errors.Errorf("signed proposal must not be nil from caller [%s]", ccIns.String())
	}

	return h.ACLProvider.CheckACL(resources.Peer_ChaincodeToChaincode, ccIns.ChainID, signedProp)
}

func (h *Handler) deregister() {
	if h.chaincodeID != nil {
		h.Registry.Deregister(h.chaincodeID.Name)
	}
}

func (h *Handler) ProcessStream(stream ccintf.ChaincodeStream) error {
	defer h.deregister()

	h.chatStream = stream
	h.errChan = make(chan error, 1)

	var keepaliveCh <-chan time.Time
	if h.Keepalive != 0 {
		ticker := time.NewTicker(h.Keepalive)
		defer ticker.Stop()
		keepaliveCh = ticker.C
	}

	// holds return values from gRPC Recv below
	type recvMsg struct {
		msg *pb.ChaincodeMessage
		err error
	}
	msgAvail := make(chan *recvMsg, 1)

	receiveMessage := func() {
		in, err := h.chatStream.Recv()
		msgAvail <- &recvMsg{in, err}
	}

	go receiveMessage()
	for {
		select {
		case rmsg := <-msgAvail:
			switch {
			// Defer the deregistering of the this handler.
			case rmsg.err == io.EOF:
				chaincodeLogger.Debugf("received EOF, ending chaincode support stream: %s", rmsg.err)
				return rmsg.err
			case rmsg.err != nil:
				err := errors.Wrap(rmsg.err, "receive failed")
				chaincodeLogger.Errorf("handling chaincode support stream: %+v", err)
				return err
			case rmsg.msg == nil:
				err := errors.New("received nil message, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			default:
				err := h.handleMessage(rmsg.msg)
				if err != nil {
					err = errors.WithMessage(err, "error handling message, ending stream")
					chaincodeLogger.Errorf("[%s] %+v", shorttxid(rmsg.msg.Txid), err)
					return err
				}

				go receiveMessage()
			}

		case sendErr := <-h.errChan:
			err := errors.Wrapf(sendErr, "received error while sending message, ending chaincode support stream")
			chaincodeLogger.Errorf("%s", err)
			return err
		case <-keepaliveCh:
			// if no error message from serialSend, KEEPALIVE happy, and don't care about error
			// (maybe it'll work later)
			h.serialSendAsync(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE})
			continue
		}
	}
}

// sendReady sends READY to chaincode serially (just like REGISTER)
func (h *Handler) sendReady() error {
	chaincodeLogger.Debugf("sending READY for chaincode %+v", h.chaincodeID)
	ccMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY}

	// if error in sending tear down the h
	if err := h.serialSend(ccMsg); err != nil {
		chaincodeLogger.Errorf("error sending READY (%s) for chaincode %+v", err, h.chaincodeID)
		return err
	}

	h.state = Ready

	chaincodeLogger.Debugf("Changed to state ready for chaincode %+v", h.chaincodeID)

	return nil
}

// notifyRegistry will send ready on registration success and
// update the launch state of the chaincode in the handler registry.
func (h *Handler) notifyRegistry(err error) {
	if err == nil {
		err = h.sendReady()
	}

	if err != nil {
		h.Registry.Failed(h.chaincodeID.Name, err)
		chaincodeLogger.Errorf("failed to start %s", h.chaincodeID)
		return
	}

	h.Registry.Ready(h.chaincodeID.Name)
}

// handleRegister is invoked when chaincode tries to register.
func (h *Handler) HandleRegister(msg *pb.ChaincodeMessage) {
	chaincodeLogger.Debugf("Received %s in state %s", msg.Type, h.state)
	chaincodeID := &pb.ChaincodeID{}
	err := proto.Unmarshal(msg.Payload, chaincodeID)
	if err != nil {
		chaincodeLogger.Errorf("Error in received %s, could NOT unmarshal registration info: %s", pb.ChaincodeMessage_REGISTER, err)
		return
	}

	// Now register with the chaincodeSupport
	h.chaincodeID = chaincodeID
	err = h.Registry.Register(h)
	if err != nil {
		h.notifyRegistry(err)
		return
	}

	// get the component parts so we can use the root chaincode
	// name in keys
	h.ccInstance = ParseName(h.chaincodeID.Name)

	chaincodeLogger.Debugf("Got %s for chaincodeID = %s, sending back %s", pb.ChaincodeMessage_REGISTER, chaincodeID, pb.ChaincodeMessage_REGISTERED)
	if err := h.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}); err != nil {
		chaincodeLogger.Errorf("error sending %s: %s", pb.ChaincodeMessage_REGISTERED, err)
		h.notifyRegistry(err)
		return
	}

	h.state = Established

	chaincodeLogger.Debugf("Changed state to established for %+v", h.chaincodeID)

	// for dev mode this will also move to ready automatically
	h.notifyRegistry(nil)
}

func (h *Handler) Notify(msg *pb.ChaincodeMessage) {
	tctx := h.TXContexts.Get(msg.ChannelId, msg.Txid)
	if tctx == nil {
		chaincodeLogger.Debugf("notifier Txid:%s, channelID:%s does not exist for handling message %s", msg.Txid, msg.ChannelId, msg.Type)
		return
	}

	chaincodeLogger.Debugf("[%s] notifying Txid:%s, channelID:%s", shorttxid(msg.Txid), msg.Txid, msg.ChannelId)
	tctx.ResponseNotifier <- msg
	tctx.CloseQueryIterators()
}

// is this a txid for which there is a valid txsim
func (h *Handler) isValidTxSim(channelID string, txid string, fmtStr string, args ...interface{}) (*TransactionContext, error) {
	txContext := h.TXContexts.Get(channelID, txid)
	if txContext == nil || txContext.TXSimulator == nil {
		err := errors.Errorf(fmtStr, args...)
		chaincodeLogger.Errorf("no ledger context: %s %s\n\n %+v", channelID, txid, err)
		return nil, err
	}
	return txContext, nil
}

// register Txid to prevent overlapping handle messages from chaincode
func (h *Handler) registerTxid(msg *pb.ChaincodeMessage) bool {
	// Check if this is the unique state request from this chaincode txid
	if h.ActiveTransactions.Add(msg.ChannelId, msg.Txid) {
		return true
	}

	// Log the issue and drop the request
	chaincodeName := "unknown"
	if h.chaincodeID != nil {
		chaincodeName = h.chaincodeID.Name
	}
	chaincodeLogger.Errorf("[%s] Another request pending for this CC: %s, Txid: %s, ChannelID: %s. Cannot process.", shorttxid(msg.Txid), chaincodeName, msg.Txid, msg.ChannelId)
	return false
}

func (h *Handler) checkMetadataCap(msg *pb.ChaincodeMessage) error {
	ac, exists := h.AppConfig.GetApplicationConfig(msg.ChannelId)
	if !exists {
		return errors.Errorf("application config does not exist for %s", msg.ChannelId)
	}

	if !ac.Capabilities().KeyLevelEndorsement() {
		return errors.New("key level endorsement is not enabled")
	}
	return nil
}

// Handles query to ledger to get state
func (h *Handler) HandleGetState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	key := string(msg.Payload)
	getState := &pb.GetState{}
	err := proto.Unmarshal(msg.Payload, getState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	chaincodeName := h.ChaincodeName()
	chaincodeLogger.Debugf("[%s] getting state for chaincode %s, key %s, channel %s", shorttxid(msg.Txid), chaincodeName, getState.Key, txContext.ChainID)

	var res []byte
	if isCollectionSet(getState.Collection) {
		res, err = txContext.TXSimulator.GetPrivateData(chaincodeName, getState.Collection, getState.Key)
	} else {
		res, err = txContext.TXSimulator.GetState(chaincodeName, getState.Key)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res == nil {
		chaincodeLogger.Debugf("[%s] No state associated with key: %s. Sending %s with an empty payload", shorttxid(msg.Txid), key, pb.ChaincodeMessage_RESPONSE)
	}

	// Send response msg back to chaincode. GetState will not trigger event
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles query to ledger to get state metadata
func (h *Handler) HandleGetStateMetadata(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	err := h.checkMetadataCap(msg)
	if err != nil {
		return nil, err
	}

	getStateMetadata := &pb.GetStateMetadata{}
	err = proto.Unmarshal(msg.Payload, getStateMetadata)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	chaincodeName := h.ChaincodeName()
	chaincodeLogger.Debugf("[%s] getting state metadata for chaincode %s, key %s, channel %s", shorttxid(msg.Txid), chaincodeName, getStateMetadata.Key, txContext.ChainID)

	var metadata map[string][]byte
	if isCollectionSet(getStateMetadata.Collection) {
		metadata, err = txContext.TXSimulator.GetPrivateDataMetadata(chaincodeName, getStateMetadata.Collection, getStateMetadata.Key)
	} else {
		metadata, err = txContext.TXSimulator.GetStateMetadata(chaincodeName, getStateMetadata.Key)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var metadataResult pb.StateMetadataResult
	for metakey := range metadata {
		md := &pb.StateMetadata{Metakey: metakey, Value: metadata[metakey]}
		metadataResult.Entries = append(metadataResult.Entries, md)
	}
	res, err := proto.Marshal(&metadataResult)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Send response msg back to chaincode. GetState will not trigger event
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles query to ledger to rage query state
func (h *Handler) HandleGetStateByRange(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	getStateByRange := &pb.GetStateByRange{}
	err := proto.Unmarshal(msg.Payload, getStateByRange)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	metadata, err := getQueryMetadataFromBytes(getStateByRange.Metadata)
	if err != nil {
		return nil, err
	}

	totalReturnLimit := calculateTotalReturnLimit(metadata)

	iterID := h.UUIDGenerator.New()
	chaincodeName := h.ChaincodeName()

	var rangeIter commonledger.ResultsIterator
	var paginationInfo map[string]interface{}

	isPaginated := false

	if isCollectionSet(getStateByRange.Collection) {
		rangeIter, err = txContext.TXSimulator.GetPrivateDataRangeScanIterator(chaincodeName, getStateByRange.Collection,
			getStateByRange.StartKey, getStateByRange.EndKey)
	} else if isMetadataSetForPagination(metadata) {
		paginationInfo, err = createPaginationInfoFromMetadata(metadata, totalReturnLimit, pb.ChaincodeMessage_GET_STATE_BY_RANGE)
		if err != nil {
			return nil, err
		}
		isPaginated = true

		startKey := getStateByRange.StartKey

		if isMetadataSetForPagination(metadata) {
			if metadata.Bookmark != "" {
				startKey = metadata.Bookmark
			}
		}
		rangeIter, err = txContext.TXSimulator.GetStateRangeScanIteratorWithMetadata(chaincodeName,
			startKey, getStateByRange.EndKey, paginationInfo)
	} else {
		rangeIter, err = txContext.TXSimulator.GetStateRangeScanIterator(chaincodeName, getStateByRange.StartKey, getStateByRange.EndKey)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	txContext.InitializeQueryContext(iterID, rangeIter)

	payload, err := h.QueryResponseBuilder.BuildQueryResponse(txContext, rangeIter, iterID, isPaginated, totalReturnLimit)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.Wrap(err, "marshal failed")
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles query to ledger for query state next
func (h *Handler) HandleQueryStateNext(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	queryStateNext := &pb.QueryStateNext{}
	err := proto.Unmarshal(msg.Payload, queryStateNext)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	queryIter := txContext.GetQueryIterator(queryStateNext.Id)
	if queryIter == nil {
		return nil, errors.New("query iterator not found")
	}

	totalReturnLimit := calculateTotalReturnLimit(nil)

	payload, err := h.QueryResponseBuilder.BuildQueryResponse(txContext, queryIter, queryStateNext.Id, false, totalReturnLimit)
	if err != nil {
		txContext.CleanupQueryContext(queryStateNext.Id)
		return nil, errors.WithStack(err)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(queryStateNext.Id)
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles the closing of a state iterator
func (h *Handler) HandleQueryStateClose(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	queryStateClose := &pb.QueryStateClose{}
	err := proto.Unmarshal(msg.Payload, queryStateClose)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	iter := txContext.GetQueryIterator(queryStateClose.Id)
	if iter != nil {
		txContext.CleanupQueryContext(queryStateClose.Id)
	}

	payload := &pb.QueryResponse{HasMore: false, Id: queryStateClose.Id}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles query to ledger to execute query state
func (h *Handler) HandleGetQueryResult(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	iterID := h.UUIDGenerator.New()
	chaincodeName := h.ChaincodeName()

	getQueryResult := &pb.GetQueryResult{}
	err := proto.Unmarshal(msg.Payload, getQueryResult)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	metadata, err := getQueryMetadataFromBytes(getQueryResult.Metadata)
	if err != nil {
		return nil, err
	}

	totalReturnLimit := calculateTotalReturnLimit(metadata)
	isPaginated := false

	var executeIter commonledger.ResultsIterator
	var paginationInfo map[string]interface{}

	if isCollectionSet(getQueryResult.Collection) {
		executeIter, err = txContext.TXSimulator.ExecuteQueryOnPrivateData(chaincodeName, getQueryResult.Collection, getQueryResult.Query)
	} else if isMetadataSetForPagination(metadata) {
		paginationInfo, err = createPaginationInfoFromMetadata(metadata, totalReturnLimit, pb.ChaincodeMessage_GET_QUERY_RESULT)
		if err != nil {
			return nil, err
		}
		isPaginated = true
		executeIter, err = txContext.TXSimulator.ExecuteQueryWithMetadata(chaincodeName,
			getQueryResult.Query, paginationInfo)

	} else {
		executeIter, err = txContext.TXSimulator.ExecuteQuery(chaincodeName, getQueryResult.Query)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	txContext.InitializeQueryContext(iterID, executeIter)

	payload, err := h.QueryResponseBuilder.BuildQueryResponse(txContext, executeIter, iterID, isPaginated, totalReturnLimit)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.Wrap(err, "marshal failed")
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles query to ledger history db
func (h *Handler) HandleGetHistoryForKey(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	iterID := h.UUIDGenerator.New()
	chaincodeName := h.ChaincodeName()

	getHistoryForKey := &pb.GetHistoryForKey{}
	err := proto.Unmarshal(msg.Payload, getHistoryForKey)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	historyIter, err := txContext.HistoryQueryExecutor.GetHistoryForKey(chaincodeName, getHistoryForKey.Key)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	totalReturnLimit := calculateTotalReturnLimit(nil)

	txContext.InitializeQueryContext(iterID, historyIter)
	payload, err := h.QueryResponseBuilder.BuildQueryResponse(txContext, historyIter, iterID, false, totalReturnLimit)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.Wrap(err, "marshal failed")
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func isCollectionSet(collection string) bool {
	return collection != ""
}

func isMetadataSetForPagination(metadata *pb.QueryMetadata) bool {
	if metadata == nil {
		return false
	}

	if metadata.PageSize == 0 && metadata.Bookmark == "" {
		return false
	}

	return true
}

func getQueryMetadataFromBytes(metadataBytes []byte) (*pb.QueryMetadata, error) {
	if metadataBytes != nil {
		metadata := &pb.QueryMetadata{}
		err := proto.Unmarshal(metadataBytes, metadata)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal failed")
		}
		return metadata, nil
	}
	return nil, nil
}

func createPaginationInfoFromMetadata(metadata *pb.QueryMetadata, totalReturnLimit int32, queryType pb.ChaincodeMessage_Type) (map[string]interface{}, error) {
	paginationInfoMap := make(map[string]interface{})

	switch queryType {
	case pb.ChaincodeMessage_GET_QUERY_RESULT:
		paginationInfoMap["bookmark"] = metadata.Bookmark
	case pb.ChaincodeMessage_GET_STATE_BY_RANGE:
		// this is a no-op for range query
	default:
		return nil, errors.New("query type must be either GetQueryResult or GetStateByRange")
	}

	paginationInfoMap["limit"] = totalReturnLimit
	return paginationInfoMap, nil
}

func calculateTotalReturnLimit(metadata *pb.QueryMetadata) int32 {
	totalReturnLimit := int32(ledgerconfig.GetTotalQueryLimit())
	if metadata != nil {
		pageSize := int32(metadata.PageSize)
		if pageSize > 0 && pageSize < totalReturnLimit {
			totalReturnLimit = pageSize
		}
	}
	return totalReturnLimit
}

func (h *Handler) getTxContextForInvoke(channelID string, txid string, payload []byte, format string, args ...interface{}) (*TransactionContext, error) {
	// if we have a channelID, just get the txsim from isValidTxSim
	if channelID != "" {
		return h.isValidTxSim(channelID, txid, "could not get valid transaction")
	}

	chaincodeSpec := &pb.ChaincodeSpec{}
	err := proto.Unmarshal(payload, chaincodeSpec)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	// Get the chaincodeID to invoke. The chaincodeID to be called may
	// contain composite info like "chaincode-name:version/channel-name"
	// We are not using version now but default to the latest
	targetInstance := ParseName(chaincodeSpec.ChaincodeId.Name)

	// If targetInstance is not an SCC, isValidTxSim should be called which will return an err.
	// We do not want to propagate calls to user CCs when the original call was to a SCC
	// without a channel context (ie, no ledger context).
	if !h.SystemCCProvider.IsSysCC(targetInstance.ChaincodeName) {
		// normal path - UCC invocation with an empty ("") channel: isValidTxSim will return an error
		return h.isValidTxSim("", txid, "could not get valid transaction")
	}

	// Calling SCC without a ChainID, then the assumption this is an external SCC called by the client (special case) and no UCC involved,
	// so no Transaction Simulator validation needed as there are no commits to the ledger, get the txContext directly if it is not nil
	txContext := h.TXContexts.Get(channelID, txid)
	if txContext == nil {
		return nil, errors.New("failed to get transaction context")
	}

	return txContext, nil
}

func (h *Handler) HandlePutState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	putState := &pb.PutState{}
	err := proto.Unmarshal(msg.Payload, putState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	chaincodeName := h.ChaincodeName()
	if isCollectionSet(putState.Collection) {
		err = txContext.TXSimulator.SetPrivateData(chaincodeName, putState.Collection, putState.Key, putState.Value)
	} else {
		err = txContext.TXSimulator.SetState(chaincodeName, putState.Key, putState.Value)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func (h *Handler) HandlePutStateMetadata(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	err := h.checkMetadataCap(msg)
	if err != nil {
		return nil, err
	}

	putStateMetadata := &pb.PutStateMetadata{}
	err = proto.Unmarshal(msg.Payload, putStateMetadata)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	metadata := make(map[string][]byte)
	metadata[putStateMetadata.Metadata.Metakey] = putStateMetadata.Metadata.Value

	chaincodeName := h.ChaincodeName()
	if isCollectionSet(putStateMetadata.Collection) {
		err = txContext.TXSimulator.SetPrivateDataMetadata(chaincodeName, putStateMetadata.Collection, putStateMetadata.Key, metadata)
	} else {
		err = txContext.TXSimulator.SetStateMetadata(chaincodeName, putStateMetadata.Key, metadata)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func (h *Handler) HandleDelState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	delState := &pb.DelState{}
	err := proto.Unmarshal(msg.Payload, delState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	chaincodeName := h.ChaincodeName()
	if isCollectionSet(delState.Collection) {
		err = txContext.TXSimulator.DeletePrivateData(chaincodeName, delState.Collection, delState.Key)
	} else {
		err = txContext.TXSimulator.DeleteState(chaincodeName, delState.Key)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Send response msg back to chaincode.
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles requests that modify ledger state
func (h *Handler) HandleInvokeChaincode(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("[%s] C-call-C", shorttxid(msg.Txid))

	chaincodeSpec := &pb.ChaincodeSpec{}
	err := proto.Unmarshal(msg.Payload, chaincodeSpec)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	// Get the chaincodeID to invoke. The chaincodeID to be called may
	// contain composite info like "chaincode-name:version/channel-name".
	// We are not using version now but default to the latest.
	targetInstance := ParseName(chaincodeSpec.ChaincodeId.Name)
	chaincodeSpec.ChaincodeId.Name = targetInstance.ChaincodeName
	if targetInstance.ChainID == "" {
		// use caller's channel as the called chaincode is in the same channel
		targetInstance.ChainID = txContext.ChainID
	}
	chaincodeLogger.Debugf("[%s] C-call-C %s on channel %s", shorttxid(msg.Txid), targetInstance.ChaincodeName, targetInstance.ChainID)

	err = h.checkACL(txContext.SignedProp, txContext.Proposal, targetInstance)
	if err != nil {
		chaincodeLogger.Errorf(
			"[%s] C-call-C %s on channel %s failed check ACL [%v]: [%s]",
			shorttxid(msg.Txid),
			targetInstance.ChaincodeName,
			targetInstance.ChainID,
			txContext.SignedProp,
			err,
		)
		return nil, errors.WithStack(err)
	}

	// Set up a new context for the called chaincode if on a different channel
	// We grab the called channel's ledger simulator to hold the new state
	txParams := &ccprovider.TransactionParams{
		TxID:                 msg.Txid,
		ChannelID:            targetInstance.ChainID,
		SignedProp:           txContext.SignedProp,
		Proposal:             txContext.Proposal,
		TXSimulator:          txContext.TXSimulator,
		HistoryQueryExecutor: txContext.HistoryQueryExecutor,
	}

	if targetInstance.ChainID != txContext.ChainID {
		lgr := h.LedgerGetter.GetLedger(targetInstance.ChainID)
		if lgr == nil {
			return nil, errors.Errorf("failed to find ledger for channel: %s", targetInstance.ChainID)
		}

		sim, err := lgr.NewTxSimulator(msg.Txid)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer sim.Done()

		hqe, err := lgr.NewHistoryQueryExecutor()
		if err != nil {
			return nil, errors.WithStack(err)
		}

		txParams.TXSimulator = sim
		txParams.HistoryQueryExecutor = hqe
	}

	chaincodeLogger.Debugf("[%s] getting chaincode data for %s on channel %s", shorttxid(msg.Txid), targetInstance.ChaincodeName, targetInstance.ChainID)

	version := h.SystemCCVersion
	if !h.SystemCCProvider.IsSysCC(targetInstance.ChaincodeName) {
		// if its a user chaincode, get the details
		cd, err := h.DefinitionGetter.ChaincodeDefinition(targetInstance.ChaincodeName, txParams.TXSimulator)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		version = cd.CCVersion()

		err = h.InstantiationPolicyChecker.CheckInstantiationPolicy(targetInstance.ChaincodeName, version, cd.(*ccprovider.ChaincodeData))
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// Launch the new chaincode if not already running
	chaincodeLogger.Debugf("[%s] launching chaincode %s on channel %s", shorttxid(msg.Txid), targetInstance.ChaincodeName, targetInstance.ChainID)

	cccid := &ccprovider.CCContext{
		Name:    targetInstance.ChaincodeName,
		Version: version,
	}

	// Execute the chaincode... this CANNOT be an init at least for now
	responseMessage, err := h.Invoker.Invoke(txParams, cccid, chaincodeSpec.Input)
	if err != nil {
		return nil, errors.Wrap(err, "execute failed")
	}

	// payload is marshalled and sent to the calling chaincode's shim which unmarshals and
	// sends it to chaincode
	res, err := proto.Marshal(responseMessage)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func (h *Handler) Execute(txParams *ccprovider.TransactionParams, cccid *ccprovider.CCContext, msg *pb.ChaincodeMessage, timeout time.Duration) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("Entry")
	defer chaincodeLogger.Debugf("Exit")

	txctx, err := h.TXContexts.Create(txParams)
	if err != nil {
		return nil, err
	}
	defer h.TXContexts.Delete(msg.ChannelId, msg.Txid)

	if err := h.setChaincodeProposal(txParams.SignedProp, txParams.Proposal, msg); err != nil {
		return nil, err
	}

	h.serialSendAsync(msg)

	var ccresp *pb.ChaincodeMessage
	select {
	case ccresp = <-txctx.ResponseNotifier:
		// response is sent to user or calling chaincode. ChaincodeMessage_ERROR
		// are typically treated as error
	case <-time.After(timeout):
		err = errors.New("timeout expired while executing transaction")
	}

	return ccresp, err
}

func (h *Handler) setChaincodeProposal(signedProp *pb.SignedProposal, prop *pb.Proposal, msg *pb.ChaincodeMessage) error {
	if prop != nil && signedProp == nil {
		return errors.New("failed getting proposal context. Signed proposal is nil")
	}
	// TODO: This doesn't make a lot of sense. Feels like both are required or
	// neither should be set. Check with a knowledable expert.
	if prop != nil {
		msg.Proposal = signedProp
	}
	return nil
}

func (h *Handler) State() State { return h.state }
func (h *Handler) Close()       { h.TXContexts.Close() }

type State int

const (
	Created State = iota
	Established
	Ready
)

func (s State) String() string {
	switch s {
	case Created:
		return "created"
	case Established:
		return "established"
	case Ready:
		return "ready"
	default:
		return "UNKNOWN"
	}
}
