/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

var chaincodeLogger = flogging.MustGetLogger("chaincode")

// An ACLProvider performs access control checks when invoking
// chaincode.
type ACLProvider interface {
	CheckACL(resName string, channelID string, idinfo any) error
}

// A Registry is responsible for tracking handlers.
type Registry interface {
	Register(*Handler) error
	Ready(string)
	Failed(string, error)
	Deregister(string) error
}

// An Invoker invokes chaincode.
type Invoker interface {
	Invoke(txParams *ccprovider.TransactionParams, chaincodeName string, spec *pb.ChaincodeInput) (*pb.ChaincodeMessage, error)
}

// TransactionRegistry tracks active transactions for each channel.
type TransactionRegistry interface {
	Add(channelID, txID string) bool
	Remove(channelID, txID string)
}

// A ContextRegistry is responsible for managing transaction contexts.
type ContextRegistry interface {
	Create(txParams *ccprovider.TransactionParams) (*TransactionContext, error)
	Get(channelID, txID string) *TransactionContext
	Delete(channelID, txID string)
	Close()
}

// QueryResponseBuilder is responsible for building QueryResponse messages for query
// transactions initiated by chaincode.
type QueryResponseBuilder interface {
	BuildQueryResponse(txContext *TransactionContext, iter commonledger.ResultsIterator,
		iterID string, isPaginated bool, totalReturnLimit int32) (*pb.QueryResponse, error)
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

const (
	ErrorExecutionTimeout = "timeout expired while executing transaction"
	ErrorStreamTerminated = "chaincode stream terminated"
)

// Handler implements the peer side of the chaincode stream.
type Handler struct {
	// Keepalive specifies the interval at which keep-alive messages are sent.
	Keepalive time.Duration
	// TotalQueryLimit specifies the maximum number of results to return for
	// chaincode queries.
	TotalQueryLimit int
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
	// BuiltinSCCs can be used to determine if a name is associated with a system chaincode
	BuiltinSCCs scc.BuiltinSCCs
	// QueryResponseBuilder is used to build query responses
	QueryResponseBuilder QueryResponseBuilder
	// LedgerGetter is used to get the ledger associated with a channel
	LedgerGetter LedgerGetter
	// IDDeserializerFactory is provided to the private data collection store
	IDDeserializerFactory privdata.IdentityDeserializerFactory
	// DeployedCCInfoProvider is used to initialize the Collection Store
	DeployedCCInfoProvider ledger.DeployedChaincodeInfoProvider
	// UUIDGenerator is used to generate UUIDs
	UUIDGenerator UUIDGenerator
	// AppConfig is used to retrieve the application config for a channel
	AppConfig ApplicationConfigRetriever
	// Metrics holds chaincode handler metrics
	Metrics *HandlerMetrics
	// UseWriteBatch an indication that the peer can accept changes from chaincode in batches
	UseWriteBatch bool
	// MaxSizeWriteBatch maximum batch size for the change segment
	MaxSizeWriteBatch uint32
	// UseGetMultipleKeys an indication that the peer can handle get multiple keys
	UseGetMultipleKeys bool
	// MaxSizeGetMultipleKeys maximum size of batches with get multiple keys
	MaxSizeGetMultipleKeys uint32

	// stateLock is used to read and set State.
	stateLock sync.RWMutex
	// state holds the current handler state. It will be created, established, or
	// ready.
	state State
	// chaincodeID holds the ID of the chaincode that registered with the peer.
	chaincodeID string

	// serialLock is used to serialize sends across the grpc chat stream.
	serialLock sync.Mutex
	// chatStream is the bidirectional grpc stream used to communicate with the
	// chaincode instance.
	chatStream ccintf.ChaincodeStream
	// errChan is used to communicate errors from the async send to the receive loop
	errChan chan error
	// mutex is used to serialze the stream closed chan.
	mutex sync.Mutex
	// streamDoneChan is closed when the chaincode stream terminates.
	streamDoneChan chan struct{}
}

// handleMessage is called by ProcessStream to dispatch messages.
func (h *Handler) handleMessage(msg *pb.ChaincodeMessage) error {
	h.stateLock.RLock()
	state := h.state
	h.stateLock.RUnlock()

	chaincodeLogger.Debugf("[%s] Fabric side handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.GetTxid()), msg.GetType(), state)

	if msg.GetType() == pb.ChaincodeMessage_KEEPALIVE {
		return nil
	}

	switch state {
	case Created:
		return h.handleMessageCreatedState(msg)
	case Ready:
		return h.handleMessageReadyState(msg)
	default:
		return errors.Errorf("handle message: invalid state %s for transaction %s", state, msg.GetTxid())
	}
}

func (h *Handler) handleMessageCreatedState(msg *pb.ChaincodeMessage) error {
	switch msg.GetType() {
	case pb.ChaincodeMessage_REGISTER:
		h.HandleRegister(msg)
	default:
		return fmt.Errorf("[%s] Fabric side handler cannot handle message (%s) while in created state", msg.GetTxid(), msg.GetType())
	}
	return nil
}

func (h *Handler) handleMessageReadyState(msg *pb.ChaincodeMessage) error {
	switch msg.GetType() {
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
	case pb.ChaincodeMessage_GET_PRIVATE_DATA_HASH:
		go h.HandleTransaction(msg, h.HandleGetPrivateDataHash)
	case pb.ChaincodeMessage_GET_STATE_METADATA:
		go h.HandleTransaction(msg, h.HandleGetStateMetadata)
	case pb.ChaincodeMessage_PUT_STATE_METADATA:
		go h.HandleTransaction(msg, h.HandlePutStateMetadata)
	case pb.ChaincodeMessage_PURGE_PRIVATE_DATA:
		go h.HandleTransaction(msg, h.HandlePurgePrivateData)
	case pb.ChaincodeMessage_WRITE_BATCH_STATE:
		go h.HandleTransaction(msg, h.HandleWriteBatch)
	case pb.ChaincodeMessage_GET_STATE_MULTIPLE:
		go h.HandleTransaction(msg, h.HandleGetStateMultipleKeys)
	default:
		return fmt.Errorf("[%s] Fabric side handler cannot handle message (%s) while in ready state", msg.GetTxid(), msg.GetType())
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
	startTime := time.Now()
	meterLabels := []string{
		"type", msg.GetType().String(),
		"channel", msg.GetChannelId(),
		"chaincode", h.chaincodeID,
	}

	// Recover from panics that can occur when the transaction context is cleaned up
	// (e.g., iterators closed) due to execution timeout while this goroutine is still
	// actively using those resources. This prevents peer crashes from nil pointer
	// dereferences in the underlying LevelDB iterator.
	// See https://github.com/hyperledger/fabric/issues/5048
	defer func() {
		if r := recover(); r != nil {
			chaincodeLogger.Errorf("[%s] Recovered from panic handling %s: %v", shorttxid(msg.GetTxid()), msg.GetType(), r)
			resp := &pb.ChaincodeMessage{
				Type:      pb.ChaincodeMessage_ERROR,
				Payload:   []byte(fmt.Sprintf("%s failed: transaction ID: %s: panic during execution", msg.GetType(), msg.GetTxid())),
				Txid:      msg.GetTxid(),
				ChannelId: msg.GetChannelId(),
			}
			h.ActiveTransactions.Remove(msg.GetChannelId(), msg.GetTxid())
			h.serialSendAsync(resp)

			meterLabels = append(meterLabels, "success", "false")
			h.Metrics.ShimRequestDuration.With(meterLabels...).Observe(time.Since(startTime).Seconds())
			h.Metrics.ShimRequestsCompleted.With(meterLabels...).Add(1)
		}
	}()

	chaincodeLogger.Debugf("[%s] handling %s from chaincode", shorttxid(msg.GetTxid()), msg.GetType().String())
	if !h.registerTxid(msg) {
		return
	}

	var txContext *TransactionContext
	var err error
	if msg.GetType() == pb.ChaincodeMessage_INVOKE_CHAINCODE {
		txContext, err = h.getTxContextForInvoke(msg.GetChannelId(), msg.GetTxid(), msg.GetPayload(), "")
	} else {
		txContext, err = h.isValidTxSim(msg.GetChannelId(), msg.GetTxid(), "no ledger context")
	}

	h.Metrics.ShimRequestsReceived.With(meterLabels...).Add(1)

	var resp *pb.ChaincodeMessage
	if err == nil {
		resp, err = delegate(msg, txContext)
	}

	if err != nil {
		err = errors.Wrapf(err, "%s failed: transaction ID: %s", msg.GetType(), msg.GetTxid())
		chaincodeLogger.Errorf("[%s] Failed to handle %s. error: %+v", shorttxid(msg.GetTxid()), msg.GetType(), err)
		resp = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(err.Error()), Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}
	}

	chaincodeLogger.Debugf("[%s] Completed %s. Sending %s", shorttxid(msg.GetTxid()), msg.GetType(), resp.GetType())
	h.ActiveTransactions.Remove(msg.GetChannelId(), msg.GetTxid())
	h.serialSendAsync(resp)

	meterLabels = append(meterLabels, "success", strconv.FormatBool(resp.GetType() != pb.ChaincodeMessage_ERROR))
	h.Metrics.ShimRequestDuration.With(meterLabels...).Observe(time.Since(startTime).Seconds())
	h.Metrics.ShimRequestsCompleted.With(meterLabels...).Add(1)
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
		ci.ChannelID = z[1]
	}
	z = strings.SplitN(z[0], ":", 2)
	if len(z) == 2 {
		ci.ChaincodeVersion = z[1]
	}
	ci.ChaincodeName = z[0]

	return ci
}

// serialSend serializes msgs so gRPC will be happy
func (h *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	h.serialLock.Lock()
	defer h.serialLock.Unlock()

	if err := h.chatStream.Send(msg); err != nil {
		err = errors.WithMessagef(err, "[%s] error sending %s", shorttxid(msg.GetTxid()), msg.GetType())
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
				Txid:      msg.GetTxid(),
				ChannelId: msg.GetChannelId(),
			}
			h.Notify(resp)

			// surface send error to stream processing
			h.errChan <- err
		}
	}()
}

// Check if the transactor is allow to call this chaincode on this channel
func (h *Handler) checkACL(signedProp *pb.SignedProposal, proposal *pb.Proposal, ccIns *sysccprovider.ChaincodeInstance) error {
	// if we are here, all we know is that the invoked chaincode is either
	// - a system chaincode that *is* invokable through a cc2cc
	//   (but we may still have to determine whether the invoker can perform this invocation)
	// - an application chaincode
	//   (and we still need to determine whether the invoker can invoke it)

	if h.BuiltinSCCs.IsSysCC(ccIns.ChaincodeName) {
		// Allow this call
		return nil
	}

	// A Nil signedProp will be rejected for non-system chaincodes
	if signedProp == nil {
		return errors.Errorf("signed proposal must not be nil from caller [%s]", ccIns.String())
	}

	return h.ACLProvider.CheckACL(resources.Peer_ChaincodeToChaincode, ccIns.ChannelID, signedProp)
}

func (h *Handler) deregister() {
	h.Registry.Deregister(h.chaincodeID)
}

func (h *Handler) streamDone() <-chan struct{} {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	return h.streamDoneChan
}

func (h *Handler) ProcessStream(stream ccintf.ChaincodeStream) error {
	defer h.deregister()

	h.mutex.Lock()
	h.streamDoneChan = make(chan struct{})
	h.mutex.Unlock()
	defer close(h.streamDoneChan)

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
				err := errors.Wrap(rmsg.err, "receive from chaincode support stream failed")
				chaincodeLogger.Debugf("%+v", err)
				return err
			case rmsg.msg == nil:
				err := errors.New("received nil message, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			default:
				err := h.handleMessage(rmsg.msg)
				if err != nil {
					err = errors.WithMessage(err, "error handling message, ending stream")
					chaincodeLogger.Errorf("[%s] %+v", shorttxid(rmsg.msg.GetTxid()), err)
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
	chaincodeLogger.Debugf("sending READY for chaincode %s", h.chaincodeID)

	chaincodeAdditionalParams := &pb.ChaincodeAdditionalParams{
		UseWriteBatch:          h.UseWriteBatch,
		MaxSizeWriteBatch:      h.MaxSizeWriteBatch,
		UseGetMultipleKeys:     h.UseGetMultipleKeys,
		MaxSizeGetMultipleKeys: h.MaxSizeGetMultipleKeys,
	}
	payloadBytes, err := proto.Marshal(chaincodeAdditionalParams)
	if err != nil {
		return errors.WithStack(err)
	}
	ccMsg := &pb.ChaincodeMessage{
		Type:    pb.ChaincodeMessage_READY,
		Payload: payloadBytes,
	}

	// if error in sending tear down the h
	if err = h.serialSend(ccMsg); err != nil {
		chaincodeLogger.Errorf("error sending READY (%s) for chaincode %s", err, h.chaincodeID)
		return err
	}

	h.stateLock.Lock()
	h.state = Ready
	h.stateLock.Unlock()

	chaincodeLogger.Debugf("Changed to state ready for chaincode %s", h.chaincodeID)

	return nil
}

// notifyRegistry will send ready on registration success and
// update the launch state of the chaincode in the handler registry.
func (h *Handler) notifyRegistry(err error) {
	if err == nil {
		err = h.sendReady()
	}

	if err != nil {
		h.Registry.Failed(h.chaincodeID, err)
		chaincodeLogger.Errorf("failed to start %s -- %s", h.chaincodeID, err)
		return
	}

	h.Registry.Ready(h.chaincodeID)
}

// HandleRegister is invoked when chaincode tries to register.
func (h *Handler) HandleRegister(msg *pb.ChaincodeMessage) {
	h.stateLock.RLock()
	state := h.state
	h.stateLock.RUnlock()

	chaincodeLogger.Debugf("Received %s in state %s", msg.GetType(), state)
	chaincodeID := &pb.ChaincodeID{}
	err := proto.Unmarshal(msg.GetPayload(), chaincodeID)
	if err != nil {
		chaincodeLogger.Errorf("Error in received %s, could NOT unmarshal registration info: %s", pb.ChaincodeMessage_REGISTER, err)
		return
	}

	// Now register with the chaincodeSupport
	// Note: chaincodeID.Name is actually of the form name:version for older chaincodes, and
	// of the form label:hash for newer chaincodes.  Either way, it is the handle by which
	// we track the chaincode's registration.
	if chaincodeID.GetName() == "" {
		h.notifyRegistry(errors.New("error in handling register chaincode, chaincodeID name is empty"))
		return
	}
	h.chaincodeID = chaincodeID.GetName()
	err = h.Registry.Register(h)
	if err != nil {
		h.notifyRegistry(err)
		return
	}

	chaincodeLogger.Debugf("Got %s for chaincodeID = %s, sending back %s", pb.ChaincodeMessage_REGISTER, h.chaincodeID, pb.ChaincodeMessage_REGISTERED)
	if err = h.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}); err != nil {
		chaincodeLogger.Errorf("error sending %s: %s", pb.ChaincodeMessage_REGISTERED, err)
		h.notifyRegistry(err)
		return
	}

	h.stateLock.Lock()
	h.state = Established
	h.stateLock.Unlock()

	chaincodeLogger.Debugf("Changed state to established for %s", h.chaincodeID)

	// for dev mode this will also move to ready automatically
	h.notifyRegistry(nil)
}

func (h *Handler) Notify(msg *pb.ChaincodeMessage) {
	tctx := h.TXContexts.Get(msg.GetChannelId(), msg.GetTxid())
	if tctx == nil {
		chaincodeLogger.Debugf("notifier Txid:%s, channelID:%s does not exist for handling message %s", msg.GetTxid(), msg.GetChannelId(), msg.GetType())
		return
	}

	chaincodeLogger.Debugf("[%s] notifying Txid:%s, channelID:%s", shorttxid(msg.GetTxid()), msg.GetTxid(), msg.GetChannelId())
	tctx.ResponseNotifier <- msg
	tctx.CloseQueryIterators()
}

// is this a txid for which there is a valid txsim
func (h *Handler) isValidTxSim(channelID string, txid string, fmtStr string, args ...any) (*TransactionContext, error) {
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
	if h.ActiveTransactions.Add(msg.GetChannelId(), msg.GetTxid()) {
		return true
	}

	// Log the issue and drop the request
	chaincodeLogger.Errorf("[%s] Another request pending for this CC: %s, Txid: %s, ChannelID: %s. Cannot process.", shorttxid(msg.GetTxid()), h.chaincodeID, msg.GetTxid(), msg.GetChannelId())
	return false
}

func (h *Handler) checkMetadataCap(channelId string) error {
	ac, exists := h.AppConfig.GetApplicationConfig(channelId)
	if !exists {
		return errors.Errorf("application config does not exist for %s", channelId)
	}

	if !ac.Capabilities().KeyLevelEndorsement() {
		return errors.New("key level endorsement is not enabled, channel application capability of V1_3 or later is required")
	}
	return nil
}

func (h *Handler) checkPurgePrivateDataCap(channelId string) error {
	ac, exists := h.AppConfig.GetApplicationConfig(channelId)
	if !exists {
		return errors.Errorf("application config does not exist for %s", channelId)
	}

	if !ac.Capabilities().PurgePvtData() {
		return errors.New("purge private data is not enabled, channel application capability of V2_5 or later is required")
	}
	return nil
}

func errorIfCreatorHasNoReadPermission(chaincodeName, collection string, txContext *TransactionContext) error {
	rwPermission, err := getReadWritePermission(chaincodeName, collection, txContext)
	if err != nil {
		return err
	}
	if !rwPermission.read {
		return errors.Errorf("tx creator does not have read access permission on privatedata in chaincodeName:%s collectionName: %s",
			chaincodeName, collection)
	}
	return nil
}

func errorIfCreatorHasNoWritePermission(chaincodeName, collection string, txContext *TransactionContext) error {
	rwPermission, err := getReadWritePermission(chaincodeName, collection, txContext)
	if err != nil {
		return err
	}
	if !rwPermission.write {
		return errors.Errorf("tx creator does not have write access permission on privatedata in chaincodeName:%s collectionName: %s",
			chaincodeName, collection)
	}
	return nil
}

func getReadWritePermission(chaincodeName, collection string, txContext *TransactionContext) (*readWritePermission, error) {
	// check to see if read access has already been checked in the scope of this chaincode simulation
	if rwPermission := txContext.CollectionACLCache.get(collection); rwPermission != nil {
		return rwPermission, nil
	}

	cc := privdata.CollectionCriteria{
		Channel:    txContext.ChannelID,
		Namespace:  chaincodeName,
		Collection: collection,
	}

	readP, writeP, err := txContext.CollectionStore.RetrieveReadWritePermission(cc, txContext.SignedProp, txContext.TXSimulator)
	if err != nil {
		return nil, err
	}
	rwPermission := &readWritePermission{read: readP, write: writeP}
	txContext.CollectionACLCache.put(collection, rwPermission)

	return rwPermission, nil
}

// Handles query to ledger to get state
func (h *Handler) HandleGetState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	getState := &pb.GetState{}
	err := proto.Unmarshal(msg.GetPayload(), getState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	var res []byte
	namespaceID := txContext.NamespaceID
	collection := getState.GetCollection()
	chaincodeLogger.Debugf("[%s] getting state for chaincode %s, key %s, channel %s", shorttxid(msg.GetTxid()), namespaceID, getState.GetKey(), txContext.ChannelID)

	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return nil, errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoReadPermission(namespaceID, collection, txContext); err != nil {
			return nil, err
		}
		res, err = txContext.TXSimulator.GetPrivateData(namespaceID, collection, getState.GetKey())
	} else {
		res, err = txContext.TXSimulator.GetState(namespaceID, getState.GetKey())
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res == nil {
		chaincodeLogger.Debugf("[%s] No state associated with key: %s. Sending %s with an empty payload", shorttxid(msg.GetTxid()), getState.GetKey(), pb.ChaincodeMessage_RESPONSE)
	}

	// Send response msg back to chaincode. GetState will not trigger event
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// HandleGetStateMultipleKeys query to ledger to get state
func (h *Handler) HandleGetStateMultipleKeys(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	getState := &pb.GetStateMultiple{}
	err := proto.Unmarshal(msg.GetPayload(), getState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	var res [][]byte
	namespaceID := txContext.NamespaceID
	collection := getState.GetCollection()
	chaincodeLogger.Debugf("[%s] getting state for chaincode %s, keys %v, channel %s", shorttxid(msg.GetTxid()), namespaceID, getState.GetKeys(), txContext.ChannelID)

	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return nil, errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoReadPermission(namespaceID, collection, txContext); err != nil {
			return nil, err
		}
		res, err = txContext.TXSimulator.GetPrivateDataMultipleKeys(namespaceID, collection, getState.GetKeys())
	} else {
		res, err = txContext.TXSimulator.GetStateMultipleKeys(namespaceID, getState.GetKeys())
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(res) == 0 {
		chaincodeLogger.Debugf("[%s] No state associated with keys: %v. Sending %s with an empty payload", shorttxid(msg.GetTxid()), getState.GetKeys(), pb.ChaincodeMessage_RESPONSE)
	}

	payloadBytes, err := proto.Marshal(&pb.GetStateMultipleResult{Values: res})
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}

	// Send response msg back to chaincode. GetState will not trigger event
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func (h *Handler) HandleGetPrivateDataHash(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	getState := &pb.GetState{}
	err := proto.Unmarshal(msg.GetPayload(), getState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	var res []byte
	namespaceID := txContext.NamespaceID
	collection := getState.GetCollection()
	chaincodeLogger.Debugf("[%s] getting private data hash for chaincode %s, key %s, channel %s", shorttxid(msg.GetTxid()), namespaceID, getState.GetKey(), txContext.ChannelID)
	if txContext.IsInitTransaction {
		return nil, errors.New("private data APIs are not allowed in chaincode Init()")
	}
	res, err = txContext.TXSimulator.GetPrivateDataHash(namespaceID, collection, getState.GetKey())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res == nil {
		chaincodeLogger.Debugf("[%s] No state associated with key: %s. Sending %s with an empty payload", shorttxid(msg.GetTxid()), getState.GetKey(), pb.ChaincodeMessage_RESPONSE)
	}
	// Send response msg back to chaincode. GetState will not trigger event
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles query to ledger to get state metadata
func (h *Handler) HandleGetStateMetadata(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	err := h.checkMetadataCap(msg.GetChannelId())
	if err != nil {
		return nil, err
	}

	getStateMetadata := &pb.GetStateMetadata{}
	err = proto.Unmarshal(msg.GetPayload(), getStateMetadata)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	namespaceID := txContext.NamespaceID
	collection := getStateMetadata.GetCollection()
	chaincodeLogger.Debugf("[%s] getting state metadata for chaincode %s, key %s, channel %s", shorttxid(msg.GetTxid()), namespaceID, getStateMetadata.GetKey(), txContext.ChannelID)

	var metadata map[string][]byte
	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return nil, errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoReadPermission(namespaceID, collection, txContext); err != nil {
			return nil, err
		}
		metadata, err = txContext.TXSimulator.GetPrivateDataMetadata(namespaceID, collection, getStateMetadata.GetKey())
	} else {
		metadata, err = txContext.TXSimulator.GetStateMetadata(namespaceID, getStateMetadata.GetKey())
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
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles query to ledger to rage query state
func (h *Handler) HandleGetStateByRange(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	getStateByRange := &pb.GetStateByRange{}
	err := proto.Unmarshal(msg.GetPayload(), getStateByRange)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	metadata, err := getQueryMetadataFromBytes(getStateByRange.GetMetadata())
	if err != nil {
		return nil, err
	}

	totalReturnLimit := h.calculateTotalReturnLimit(metadata)
	iterID := h.UUIDGenerator.New()
	var rangeIter commonledger.ResultsIterator
	isPaginated := false
	namespaceID := txContext.NamespaceID
	collection := getStateByRange.GetCollection()
	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return nil, errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoReadPermission(namespaceID, collection, txContext); err != nil {
			return nil, err
		}
		rangeIter, err = txContext.TXSimulator.GetPrivateDataRangeScanIterator(namespaceID, collection,
			getStateByRange.GetStartKey(), getStateByRange.GetEndKey())
	} else if isMetadataSetForPagination(metadata) {
		isPaginated = true
		startKey := getStateByRange.GetStartKey()
		if isMetadataSetForPagination(metadata) {
			if metadata.GetBookmark() != "" {
				startKey = metadata.GetBookmark()
			}
		}
		rangeIter, err = txContext.TXSimulator.GetStateRangeScanIteratorWithPagination(namespaceID,
			startKey, getStateByRange.GetEndKey(), metadata.GetPageSize())
	} else {
		rangeIter, err = txContext.TXSimulator.GetStateRangeScanIterator(namespaceID, getStateByRange.GetStartKey(), getStateByRange.GetEndKey())
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
	if payload == nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.New("marshal failed: proto: Marshal called with nil")
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.Wrap(err, "marshal failed")
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles query to ledger for query state next
func (h *Handler) HandleQueryStateNext(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	queryStateNext := &pb.QueryStateNext{}
	err := proto.Unmarshal(msg.GetPayload(), queryStateNext)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	queryIter := txContext.GetQueryIterator(queryStateNext.GetId())
	if queryIter == nil {
		return nil, errors.New("query iterator not found")
	}

	totalReturnLimit := h.calculateTotalReturnLimit(nil)

	payload, err := h.QueryResponseBuilder.BuildQueryResponse(txContext, queryIter, queryStateNext.GetId(), false, totalReturnLimit)
	if err != nil {
		txContext.CleanupQueryContext(queryStateNext.GetId())
		return nil, errors.WithStack(err)
	}
	if payload == nil {
		txContext.CleanupQueryContext(queryStateNext.GetId())
		return nil, errors.New("marshal failed: proto: Marshal called with nil")
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(queryStateNext.GetId())
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles the closing of a state iterator
func (h *Handler) HandleQueryStateClose(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	queryStateClose := &pb.QueryStateClose{}
	err := proto.Unmarshal(msg.GetPayload(), queryStateClose)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	iter := txContext.GetQueryIterator(queryStateClose.GetId())
	if iter != nil {
		txContext.CleanupQueryContext(queryStateClose.GetId())
	}

	payload := &pb.QueryResponse{HasMore: false, Id: queryStateClose.GetId()}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles query to ledger to execute query state
func (h *Handler) HandleGetQueryResult(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	iterID := h.UUIDGenerator.New()

	getQueryResult := &pb.GetQueryResult{}
	err := proto.Unmarshal(msg.GetPayload(), getQueryResult)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	metadata, err := getQueryMetadataFromBytes(getQueryResult.GetMetadata())
	if err != nil {
		return nil, err
	}

	totalReturnLimit := h.calculateTotalReturnLimit(metadata)
	isPaginated := false
	var executeIter commonledger.ResultsIterator
	namespaceID := txContext.NamespaceID
	collection := getQueryResult.GetCollection()
	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return nil, errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoReadPermission(namespaceID, collection, txContext); err != nil {
			return nil, err
		}
		executeIter, err = txContext.TXSimulator.ExecuteQueryOnPrivateData(namespaceID, collection, getQueryResult.GetQuery())
	} else if isMetadataSetForPagination(metadata) {
		isPaginated = true
		executeIter, err = txContext.TXSimulator.ExecuteQueryWithPagination(namespaceID,
			getQueryResult.GetQuery(), metadata.GetBookmark(), metadata.GetPageSize())

	} else {
		executeIter, err = txContext.TXSimulator.ExecuteQuery(namespaceID, getQueryResult.GetQuery())
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
	if payload == nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.New("marshal failed: proto: Marshal called with nil")
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.Wrap(err, "marshal failed")
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles query to ledger history db
func (h *Handler) HandleGetHistoryForKey(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	if txContext.HistoryQueryExecutor == nil {
		return nil, errors.New("history database is not enabled")
	}
	iterID := h.UUIDGenerator.New()
	namespaceID := txContext.NamespaceID

	getHistoryForKey := &pb.GetHistoryForKey{}
	err := proto.Unmarshal(msg.GetPayload(), getHistoryForKey)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	historyIter, err := txContext.HistoryQueryExecutor.GetHistoryForKey(namespaceID, getHistoryForKey.GetKey())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	totalReturnLimit := h.calculateTotalReturnLimit(nil)

	txContext.InitializeQueryContext(iterID, historyIter)
	payload, err := h.QueryResponseBuilder.BuildQueryResponse(txContext, historyIter, iterID, false, totalReturnLimit)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}
	if payload == nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.New("marshal failed: proto: Marshal called with nil")
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.Wrap(err, "marshal failed")
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func isCollectionSet(collection string) bool {
	return collection != ""
}

func isMetadataSetForPagination(metadata *pb.QueryMetadata) bool {
	if metadata == nil {
		return false
	}

	if metadata.GetPageSize() == 0 && metadata.GetBookmark() == "" {
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

func (h *Handler) calculateTotalReturnLimit(metadata *pb.QueryMetadata) int32 {
	totalReturnLimit := int32(h.TotalQueryLimit)
	if metadata != nil {
		pageSize := metadata.GetPageSize()
		if pageSize > 0 && pageSize < totalReturnLimit {
			totalReturnLimit = pageSize
		}
	}
	return totalReturnLimit
}

func (h *Handler) getTxContextForInvoke(channelID string, txid string, payload []byte, format string, args ...any) (*TransactionContext, error) {
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
	targetInstance := ParseName(chaincodeSpec.GetChaincodeId().GetName())

	// If targetInstance is not an SCC, isValidTxSim should be called which will return an err.
	// We do not want to propagate calls to user CCs when the original call was to a SCC
	// without a channel context (ie, no ledger context).
	if !h.BuiltinSCCs.IsSysCC(targetInstance.ChaincodeName) {
		// normal path - UCC invocation with an empty ("") channel: isValidTxSim will return an error
		return h.isValidTxSim("", txid, "could not get valid transaction")
	}

	// Calling SCC without a ChannelID, then the assumption this is an external SCC called by the client (special case) and no UCC involved,
	// so no Transaction Simulator validation needed as there are no commits to the ledger, get the txContext directly if it is not nil
	txContext := h.TXContexts.Get(channelID, txid)
	if txContext == nil {
		return nil, errors.New("failed to get transaction context")
	}

	return txContext, nil
}

func (h *Handler) HandlePutState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	putState := &pb.PutState{}
	if err := proto.Unmarshal(msg.GetPayload(), putState); err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	if err := h.putState(putState, txContext); err != nil {
		return nil, errors.WithStack(err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func (h *Handler) putState(msg *pb.PutState, txContext *TransactionContext) error {
	var err error

	namespaceID := txContext.NamespaceID
	collection := msg.GetCollection()
	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoWritePermission(namespaceID, collection, txContext); err != nil {
			return err
		}
		err = txContext.TXSimulator.SetPrivateData(namespaceID, collection, msg.GetKey(), msg.GetValue())
	} else {
		err = txContext.TXSimulator.SetState(namespaceID, msg.GetKey(), msg.GetValue())
	}
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *Handler) HandlePutStateMetadata(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	putStateMetadata := &pb.PutStateMetadata{}
	if err := proto.Unmarshal(msg.GetPayload(), putStateMetadata); err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	if err := h.putStateMetadata(putStateMetadata, txContext, msg.GetChannelId()); err != nil {
		return nil, errors.WithStack(err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func (h *Handler) putStateMetadata(msg *pb.PutStateMetadata, txContext *TransactionContext, channelId string) error {
	err := h.checkMetadataCap(channelId)
	if err != nil {
		return err
	}

	metadata := make(map[string][]byte)
	metadata[msg.GetMetadata().GetMetakey()] = msg.GetMetadata().GetValue()

	namespaceID := txContext.NamespaceID
	collection := msg.GetCollection()
	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoWritePermission(namespaceID, collection, txContext); err != nil {
			return err
		}
		err = txContext.TXSimulator.SetPrivateDataMetadata(namespaceID, collection, msg.GetKey(), metadata)
	} else {
		err = txContext.TXSimulator.SetStateMetadata(namespaceID, msg.GetKey(), metadata)
	}
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *Handler) HandleDelState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	delState := &pb.DelState{}
	if err := proto.Unmarshal(msg.GetPayload(), delState); err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	if err := h.delState(delState, txContext); err != nil {
		return nil, errors.WithStack(err)
	}

	// Send response msg back to chaincode.
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func (h *Handler) delState(msg *pb.DelState, txContext *TransactionContext) error {
	var err error

	namespaceID := txContext.NamespaceID
	collection := msg.GetCollection()
	if isCollectionSet(collection) {
		if txContext.IsInitTransaction {
			return errors.New("private data APIs are not allowed in chaincode Init()")
		}
		if err = errorIfCreatorHasNoWritePermission(namespaceID, collection, txContext); err != nil {
			return err
		}
		err = txContext.TXSimulator.DeletePrivateData(namespaceID, collection, msg.GetKey())
	} else {
		err = txContext.TXSimulator.DeleteState(namespaceID, msg.GetKey())
	}
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *Handler) HandlePurgePrivateData(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	delState := &pb.DelState{}
	if err := proto.Unmarshal(msg.GetPayload(), delState); err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	if err := h.purgePrivateData(delState, txContext, msg.GetChannelId()); err != nil {
		return nil, errors.WithStack(err)
	}

	// Send response msg back to chaincode.
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func (h *Handler) purgePrivateData(msg *pb.DelState, txContext *TransactionContext, channelId string) error {
	err := h.checkPurgePrivateDataCap(channelId)
	if err != nil {
		return err
	}

	namespaceID := txContext.NamespaceID
	collection := msg.GetCollection()
	if collection == "" {
		return errors.New("only applicable for private data")
	}

	if txContext.IsInitTransaction {
		return errors.New("private data APIs are not allowed in chaincode Init()")
	}

	if err = errorIfCreatorHasNoWritePermission(namespaceID, collection, txContext); err != nil {
		return err
	}

	if err = txContext.TXSimulator.PurgePrivateData(namespaceID, collection, msg.GetKey()); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *Handler) HandleWriteBatch(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	batch := &pb.WriteBatchState{}
	err := proto.Unmarshal(msg.GetPayload(), batch)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	for _, kv := range batch.GetRec() {
		switch kv.GetType() {
		case pb.WriteRecord_PUT_STATE:
			putState := &pb.PutState{
				Key:        kv.GetKey(),
				Value:      kv.GetValue(),
				Collection: kv.GetCollection(),
			}
			if err = h.putState(putState, txContext); err != nil {
				return nil, errors.WithStack(err)
			}

		case pb.WriteRecord_DEL_STATE:
			delState := &pb.DelState{
				Key:        kv.GetKey(),
				Collection: kv.GetCollection(),
			}
			if err = h.delState(delState, txContext); err != nil {
				return nil, errors.WithStack(err)
			}

		case pb.WriteRecord_PURGE_PRIVATE_DATA:
			delState := &pb.DelState{
				Key:        kv.GetKey(),
				Collection: kv.GetCollection(),
			}
			if err = h.purgePrivateData(delState, txContext, msg.GetChannelId()); err != nil {
				return nil, err
			}

		case pb.WriteRecord_PUT_STATE_METADATA:
			putStateMetadata := &pb.PutStateMetadata{
				Key:        kv.GetKey(),
				Collection: kv.GetCollection(),
				Metadata: &pb.StateMetadata{
					Metakey: kv.GetMetadata().GetMetakey(),
					Value:   kv.GetMetadata().GetValue(),
				},
			}
			if err = h.putStateMetadata(putStateMetadata, txContext, msg.GetChannelId()); err != nil {
				return nil, err
			}

		default:
			return nil, errors.New("unknown operation type")
		}
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

// Handles requests that modify ledger state
func (h *Handler) HandleInvokeChaincode(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("[%s] C-call-C", shorttxid(msg.GetTxid()))

	chaincodeSpec := &pb.ChaincodeSpec{}
	err := proto.Unmarshal(msg.GetPayload(), chaincodeSpec)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	// Get the chaincodeID to invoke. The chaincodeID to be called may
	// contain composite info like "chaincode-name:version/channel-name".
	// We are not using version now but default to the latest.
	targetInstance := ParseName(chaincodeSpec.GetChaincodeId().GetName())
	chaincodeSpec.ChaincodeId.Name = targetInstance.ChaincodeName
	if targetInstance.ChannelID == "" {
		// use caller's channel as the called chaincode is in the same channel
		targetInstance.ChannelID = txContext.ChannelID
	}
	chaincodeLogger.Debugf("[%s] C-call-C %s on channel %s", shorttxid(msg.GetTxid()), targetInstance.ChaincodeName, targetInstance.ChannelID)

	err = h.checkACL(txContext.SignedProp, txContext.Proposal, targetInstance)
	if err != nil {
		chaincodeLogger.Errorf(
			"[%s] C-call-C %s on channel %s failed check ACL [%v]: [%s]",
			shorttxid(msg.GetTxid()),
			targetInstance.ChaincodeName,
			targetInstance.ChannelID,
			txContext.SignedProp,
			err,
		)
		return nil, errors.WithStack(err)
	}

	// Set up a new context for the called chaincode if on a different channel
	// We grab the called channel's ledger simulator to hold the new state
	txParams := &ccprovider.TransactionParams{
		TxID:                 msg.GetTxid(),
		ChannelID:            targetInstance.ChannelID,
		SignedProp:           txContext.SignedProp,
		Proposal:             txContext.Proposal,
		TXSimulator:          txContext.TXSimulator,
		HistoryQueryExecutor: txContext.HistoryQueryExecutor,
	}

	if targetInstance.ChannelID != txContext.ChannelID {
		lgr := h.LedgerGetter.GetLedger(targetInstance.ChannelID)
		if lgr == nil {
			return nil, errors.Errorf("failed to find ledger for channel: %s", targetInstance.ChannelID)
		}

		sim, err := lgr.NewTxSimulator(msg.GetTxid())
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

	// Execute the chaincode... this CANNOT be an init at least for now
	responseMessage, err := h.Invoker.Invoke(txParams, targetInstance.ChaincodeName, chaincodeSpec.GetInput())
	if err != nil {
		return nil, errors.Wrap(err, "execute failed")
	}
	if responseMessage == nil {
		return nil, errors.New("marshal failed: proto: Marshal called with nil")
	}

	// payload is marshalled and sent to the calling chaincode's shim which unmarshals and
	// sends it to chaincode
	res, err := proto.Marshal(responseMessage)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.GetTxid(), ChannelId: msg.GetChannelId()}, nil
}

func (h *Handler) Execute(txParams *ccprovider.TransactionParams, namespace string, msg *pb.ChaincodeMessage, timeout time.Duration) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("Entry")
	defer chaincodeLogger.Debugf("Exit")

	txParams.CollectionStore = h.getCollectionStore(msg.GetChannelId())
	txParams.IsInitTransaction = msg.GetType() == pb.ChaincodeMessage_INIT
	txParams.NamespaceID = namespace

	txctx, err := h.TXContexts.Create(txParams)
	if err != nil {
		return nil, err
	}
	defer h.TXContexts.Delete(msg.GetChannelId(), msg.GetTxid())

	if err = h.setChaincodeProposal(txParams.SignedProp, txParams.Proposal, msg); err != nil {
		return nil, err
	}

	h.serialSendAsync(msg)

	var ccresp *pb.ChaincodeMessage
	select {
	case ccresp = <-txctx.ResponseNotifier:
		// response is sent to user or calling chaincode. ChaincodeMessage_ERROR
		// are typically treated as error
	case <-time.After(timeout):
		err = errors.New(ErrorExecutionTimeout)
		h.Metrics.ExecuteTimeouts.With("chaincode", h.chaincodeID).Add(1)
	case <-h.streamDone():
		err = errors.New(ErrorStreamTerminated)
	}

	return ccresp, err
}

func (h *Handler) setChaincodeProposal(signedProp *pb.SignedProposal, prop *pb.Proposal, msg *pb.ChaincodeMessage) error {
	if prop != nil && signedProp == nil {
		return errors.New("failed getting proposal context. Signed proposal is nil")
	}
	// TODO: This doesn't make a lot of sense. Feels like both are required or
	// neither should be set. Check with a knowledgeable expert.
	if prop != nil {
		msg.Proposal = signedProp
	}
	return nil
}

func (h *Handler) getCollectionStore(channelID string) privdata.CollectionStore {
	return privdata.NewSimpleCollectionStore(
		h.LedgerGetter.GetLedger(channelID),
		h.DeployedCCInfoProvider,
		h.IDDeserializerFactory,
	)
}

func (h *Handler) State() State {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()

	return h.state
}
func (h *Handler) Close() { h.TXContexts.Close() }

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
