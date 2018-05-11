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
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type state int

const (
	created state = iota
	established
	ready
)

func (s state) String() string {
	switch s {
	case created:
		return "created"
	case established:
		return "established"
	case ready:
		return "ready"
	default:
		return "UNKNOWN"
	}
}

var chaincodeLogger = flogging.MustGetLogger("chaincode")

// ACLProvider is responsible for performing access control checks when invoking
// chaincode.
type ACLProvider interface {
	CheckACL(resName string, channelID string, idinfo interface{}) error
}

type Registry interface {
	Register(*Handler) error
	Ready(cname string)
	Deregister(cname string) error
}

// Handler implements the peer side of the chaincode stream.
type Handler struct {
	// Lifecycle is used to access the Lifecycle System Chaincode.
	Lifecycle *Lifecycle
	// Executor is used to invoke chaincode.
	Executor Executor
	// Keepalive specifies the interval at which keep-alive messages are sent.
	Keepalive time.Duration
	// Registry is used to track active handlers.
	Registry Registry
	// ACLProvider is used to check if a chaincode invocation should be allowed.
	ACLProvider ACLProvider
	// TXContexts is a collection of TransactionContext instances
	// that are accessed by channel name and transaction ID.
	TXContexts *TransactionContexts
	// activeTransactions holds active transaction identifiers.
	ActiveTransactions *ActiveTransactions

	sccp sysccprovider.SystemChaincodeProvider

	state       state
	chaincodeID *pb.ChaincodeID
	ccInstance  *sysccprovider.ChaincodeInstance
	serialLock  sync.Mutex // serialLock is used to serialize sends across the grpc chat stream.
	chatStream  ccintf.ChaincodeStream
	errChan     chan error // chan to pass error in sync and nonsync mode
}

// HandleMessage is the entry point Chaincode messages.
func (h *Handler) HandleMessage(msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("[%s]Fabric side Handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.Txid), msg.Type, h.state)

	switch h.state {
	case created:
		return h.handleMessageCreatedState(msg)
	case ready:
		return h.handleMessageReadyState(msg)
	default:
		return errors.Errorf("handle message: invalid state %s for trnansaction %s", h.state, msg.Txid)
	}
}

func (h *Handler) handleMessageCreatedState(msg *pb.ChaincodeMessage) error {
	switch msg.Type {
	case pb.ChaincodeMessage_REGISTER:
		h.handleRegister(msg)
	default:
		return fmt.Errorf("[%s]Fabric side handler cannot handle message (%s) while in created state", msg.Txid, msg.Type)
	}
	return nil
}

func (h *Handler) handleMessageReadyState(msg *pb.ChaincodeMessage) error {
	switch msg.Type {
	case pb.ChaincodeMessage_COMPLETED, pb.ChaincodeMessage_ERROR:
		h.notify(msg)

	case pb.ChaincodeMessage_PUT_STATE:
		go h.invokeHandler(msg, h.handlePutState)
	case pb.ChaincodeMessage_DEL_STATE:
		go h.invokeHandler(msg, h.handleDelState)
	case pb.ChaincodeMessage_INVOKE_CHAINCODE:
		go h.invokeHandler(msg, h.handleInvokeChaincode)

	case pb.ChaincodeMessage_GET_STATE:
		go h.invokeHandler(msg, h.handleGetState)
	case pb.ChaincodeMessage_GET_STATE_BY_RANGE:
		go h.invokeHandler(msg, h.handleGetStateByRange)
	case pb.ChaincodeMessage_GET_QUERY_RESULT:
		go h.invokeHandler(msg, h.handleGetQueryResult)
	case pb.ChaincodeMessage_GET_HISTORY_FOR_KEY:
		go h.invokeHandler(msg, h.handleGetHistoryForKey)
	case pb.ChaincodeMessage_QUERY_STATE_NEXT:
		go h.invokeHandler(msg, h.handleQueryStateNext)
	case pb.ChaincodeMessage_QUERY_STATE_CLOSE:
		go h.invokeHandler(msg, h.handleQueryStateClose)

	default:
		return fmt.Errorf("[%s]Fabric side handler cannot handle message (%s) while in ready state", msg.Txid, msg.Type)
	}

	return nil
}

type handleFunc func(*pb.ChaincodeMessage, *TransactionContext) (*pb.ChaincodeMessage, error)

func (h *Handler) invokeHandler(msg *pb.ChaincodeMessage, delegate handleFunc) {
	chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), msg.Type.String())
	if !h.registerTxid(msg) {
		return
	}

	var txContext *TransactionContext
	var err error
	if msg.Type == pb.ChaincodeMessage_INVOKE_CHAINCODE {
		txContext, err = h.getTxContextForInvoke(msg.ChannelId, msg.Txid, msg.Payload, "")
	} else {
		txContext, err = h.isValidTxSim(msg.ChannelId, msg.Txid, "[%s]No ledger context. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
	}

	var resp *pb.ChaincodeMessage
	if err == nil {
		resp, err = delegate(msg, txContext)
	}

	if err != nil {
		err = errors.Wrapf(err, "%s failed: transaction ID: %s", msg.Type, msg.Txid)
		chaincodeLogger.Errorf("[%s]Failed to handle %s. error: %+v", shorttxid(msg.Txid), msg.Type, err)
		resp = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: []byte(err.Error()), Txid: msg.Txid, ChannelId: msg.ChannelId}
	}

	chaincodeLogger.Debugf("[%s]Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type, resp.Type)
	h.serialSendAsync(resp, false)
	h.ActiveTransactions.Remove(msg.ChannelId, msg.Txid)
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

// The chincodeID should be of the form "chaincode-name:version/channel-name"
// with optional elements.
func getChaincodeInstance(ccName string) *sysccprovider.ChaincodeInstance {
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

func (h *Handler) chaincodeName() string {
	return h.ccInstance.ChaincodeName
}

// serialSend serializes msgs so gRPC will be happy
func (h *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	h.serialLock.Lock()
	defer h.serialLock.Unlock()

	var err error
	if err = h.chatStream.Send(msg); err != nil {
		err = errors.WithMessage(err, fmt.Sprintf("[%s]Error sending %s", shorttxid(msg.Txid), msg.Type))
		chaincodeLogger.Errorf("%+v", err)
	}
	return err
}

// serialSendAsync serves the same purpose as serialSend (serialize msgs so gRPC will
// be happy). In addition, it is also asynchronous so send-remoterecv--localrecv loop
// can be nonblocking. Only errors need to be handled and these are handled by
// communication on supplied error channel. A typical use will be a non-blocking or
// nil channel
func (h *Handler) serialSendAsync(msg *pb.ChaincodeMessage, sendErr bool) {
	go func() {
		if err := h.serialSend(msg); err != nil {
			if sendErr {
				h.errChan <- err
			}
		}
	}()
}

// Check if the transactor is allow to call this chaincode on this channel
func (h *Handler) checkACL(signedProp *pb.SignedProposal, proposal *pb.Proposal, ccIns *sysccprovider.ChaincodeInstance) error {
	// ensure that we don't invoke a system chaincode
	// that is not invokable through a cc2cc invocation
	if h.sccp.IsSysCCAndNotInvokableCC2CC(ccIns.ChaincodeName) {
		return errors.Errorf("system chaincode %s cannot be invoked with a cc2cc invocation", ccIns.ChaincodeName)
	}

	// if we are here, all we know is that the invoked chaincode is either
	// - a system chaincode that *is* invokable through a cc2cc
	//   (but we may still have to determine whether the invoker
	//   can perform this invocation)
	// - an application chaincode (and we still need to determine
	//   whether the invoker can invoke it)

	if h.sccp.IsSysCC(ccIns.ChaincodeName) {
		// Allow this call
		return nil
	}

	// A Nil signedProp will be rejected for non-system chaincodes
	if signedProp == nil {
		return errors.Errorf("signed proposal must not be nil from caller [%s]", ccIns.String())
	}

	return h.ACLProvider.CheckACL(resources.Peer_ChaincodeToChaincode, ccIns.ChainID, signedProp)
}

func (h *Handler) waitForKeepaliveTimer() <-chan time.Time {
	if h.Keepalive > 0 {
		c := time.After(h.Keepalive)
		return c
	}

	// no one will signal this channel, listener blocks forever
	c := make(chan time.Time)
	return c
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

	// holds return values from gRPC Recv below
	type recvMsg struct {
		msg *pb.ChaincodeMessage
		err error
	}

	msgAvail := make(chan *recvMsg)

	var in *pb.ChaincodeMessage
	var err error

	// recv is used to spin Recv routine after previous received msg
	// has been processed
	recv := true

	// catch send errors and bail now that sends aren't synchronous
	for {
		in = nil
		err = nil
		if recv {
			recv = false
			go func() {
				in2, err2 := h.chatStream.Recv()
				msgAvail <- &recvMsg{in2, err2}
			}()
		}
		select {
		case rMsg := <-msgAvail:
			// Defer the deregistering of the this handler.
			if rMsg.err == io.EOF {
				err = errors.Wrapf(err, "received EOF, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", rMsg.err)
				return err
			} else if rMsg.err != nil {
				chaincodeLogger.Errorf("Error handling chaincode support stream: %+v", rMsg.err)
				return err
			} else if rMsg.msg == nil {
				err = errors.New("received nil message, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			}

			in = rMsg.msg

			chaincodeLogger.Debugf("[%s]Received message %s from shim", shorttxid(in.Txid), in.Type)

			// we can spin off another Recv again
			recv = true

			if in.Type == pb.ChaincodeMessage_KEEPALIVE {
				chaincodeLogger.Debug("Received KEEPALIVE Response")
				// Received a keep alive message, we don't do anything with it for now
				// and it does not touch the state machine
				continue
			}
		case sendErr := <-h.errChan:
			err = errors.Wrapf(sendErr, "received error while sending message, ending chaincode support stream")
			chaincodeLogger.Errorf("%s", err)
			return err
		case <-h.waitForKeepaliveTimer():
			// if no error message from serialSend, KEEPALIVE happy, and don't care about error
			// (maybe it'll work later)
			h.serialSendAsync(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE}, false)
			continue
		}

		err = h.HandleMessage(in)
		if err != nil {
			err = errors.WithMessage(err, "error handling message, ending stream")
			chaincodeLogger.Errorf("[%s] %+v", shorttxid(in.Txid), err)
			return err
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

	h.state = ready
	h.Registry.Ready(h.chaincodeID.Name)

	chaincodeLogger.Debugf("Changed to state ready for chaincode %+v", h.chaincodeID)

	return nil
}

// notifyDuringStartup will send ready on registration
func (h *Handler) notifyDuringStartup(val bool) {
	if val {
		// if send failed, notify failure which will initiate
		// tearing down
		if err := h.sendReady(); err != nil {
			chaincodeLogger.Debugf("sendReady failed: %s", err)
		}
	}
}

// handleRegister is invoked when chaincode tries to register.
func (h *Handler) handleRegister(msg *pb.ChaincodeMessage) {
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
		h.notifyDuringStartup(false)
		return
	}

	// get the component parts so we can use the root chaincode
	// name in keys
	h.ccInstance = getChaincodeInstance(h.chaincodeID.Name)

	chaincodeLogger.Debugf("Got %s for chaincodeID = %s, sending back %s", pb.ChaincodeMessage_REGISTER, chaincodeID, pb.ChaincodeMessage_REGISTERED)
	if err := h.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}); err != nil {
		chaincodeLogger.Errorf("Error sending %s: %s", pb.ChaincodeMessage_REGISTERED, err)
		h.notifyDuringStartup(false)
		return
	}

	h.state = established

	chaincodeLogger.Debugf("Changed state to established for %+v", h.chaincodeID)

	// for dev mode this will also move to ready automatically
	h.notifyDuringStartup(true)
}

func (h *Handler) notify(msg *pb.ChaincodeMessage) {
	tctx := h.TXContexts.Get(msg.ChannelId, msg.Txid)
	if tctx == nil {
		chaincodeLogger.Debugf("notifier Txid:%s, channelID:%s does not exist for handleing message %s", msg.Txid, msg.ChannelId, msg.Type)
		return
	}

	chaincodeLogger.Debugf("[%s]notifying Txid:%s, channelID:%s", shorttxid(msg.Txid), msg.Txid, msg.ChannelId)
	tctx.responseNotifier <- msg
	tctx.CloseQueryIterators()
}

// is this a txid for which there is a valid txsim
func (h *Handler) isValidTxSim(channelID string, txid string, fmtStr string, args ...interface{}) (*TransactionContext, error) {
	txContext := h.TXContexts.Get(channelID, txid)
	if txContext == nil || txContext.txsimulator == nil {
		err := errors.Errorf(fmtStr, args...)
		chaincodeLogger.Errorf("%+v", err)
		return nil, err
	}
	return txContext, nil
}

// register Txid to prevent overlapping handle messages from chaincode
func (h *Handler) registerTxid(msg *pb.ChaincodeMessage) bool {
	// Check if this is the unique state request from this chaincode txid
	if !h.ActiveTransactions.Add(msg.ChannelId, msg.Txid) {
		// Drop this request
		chaincodeLogger.Errorf("[%s]Another request pending for this CC: %s, Txid: %s, ChannelID: %s. Cannot process.", shorttxid(msg.Txid), h.chaincodeID.Name, msg.Txid, msg.ChannelId)
		return false
	}
	return true
}

// Handles query to ledger to get state
func (h *Handler) handleGetState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	key := string(msg.Payload)
	getState := &pb.GetState{}
	err := proto.Unmarshal(msg.Payload, getState)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshal")
	}

	chaincodeName := h.chaincodeName()
	chaincodeLogger.Debugf("[%s] getting state for chaincode %s, key %s, channel %s", shorttxid(msg.Txid), chaincodeName, getState.Key, txContext.chainID)

	var res []byte
	if isCollectionSet(getState.Collection) {
		res, err = txContext.txsimulator.GetPrivateData(chaincodeName, getState.Collection, getState.Key)
	} else {
		res, err = txContext.txsimulator.GetState(chaincodeName, getState.Key)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res == nil {
		// The state object being requested does not exist
		chaincodeLogger.Debugf("[%s]No state associated with key: %s. Sending %s with an empty payload", shorttxid(msg.Txid), key, pb.ChaincodeMessage_RESPONSE)
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
	}

	// Send response msg back to chaincode. GetState will not trigger event
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles query to ledger to rage query state
func (h *Handler) handleGetStateByRange(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	getStateByRange := &pb.GetStateByRange{}
	err := proto.Unmarshal(msg.Payload, getStateByRange)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshal")
	}

	iterID := util.GenerateUUID()
	chaincodeName := h.chaincodeName()

	var rangeIter commonledger.ResultsIterator
	if isCollectionSet(getStateByRange.Collection) {
		rangeIter, err = txContext.txsimulator.GetPrivateDataRangeScanIterator(chaincodeName, getStateByRange.Collection, getStateByRange.StartKey, getStateByRange.EndKey)
	} else {
		rangeIter, err = txContext.txsimulator.GetStateRangeScanIterator(chaincodeName, getStateByRange.StartKey, getStateByRange.EndKey)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	txContext.InitializeQueryContext(iterID, rangeIter)
	payload, err := getQueryResponse(txContext, rangeIter, iterID)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

const maxResultLimit = 100

// getQueryResponse takes an iterator and fetch state to construct QueryResponse
func getQueryResponse(txContext *TransactionContext, iter commonledger.ResultsIterator, iterID string) (*pb.QueryResponse, error) {
	pendingQueryResults := txContext.pendingQueryResults[iterID]
	for {
		queryResult, err := iter.Next()
		switch {
		case err != nil:
			chaincodeLogger.Errorf("Failed to get query result from iterator")
			txContext.CleanupQueryContext(iterID)
			return nil, err
		case queryResult == nil:
			// nil response from iterator indicates end of query results
			batch := pendingQueryResults.Cut()
			txContext.CleanupQueryContext(iterID)
			return &pb.QueryResponse{Results: batch, HasMore: false, Id: iterID}, nil
		case pendingQueryResults.Size() == maxResultLimit:
			// max number of results queued up, cut batch, then add current result to pending batch
			batch := pendingQueryResults.Cut()
			if err := pendingQueryResults.Add(queryResult); err != nil {
				txContext.CleanupQueryContext(iterID)
				return nil, err
			}
			return &pb.QueryResponse{Results: batch, HasMore: true, Id: iterID}, nil
		default:
			if err := pendingQueryResults.Add(queryResult); err != nil {
				txContext.CleanupQueryContext(iterID)
				return nil, err
			}
		}
	}
}

// Handles query to ledger for query state next
func (h *Handler) handleQueryStateNext(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	queryStateNext := &pb.QueryStateNext{}
	err := proto.Unmarshal(msg.Payload, queryStateNext)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	queryIter := txContext.GetQueryIterator(queryStateNext.Id)
	if queryIter == nil {
		return nil, errors.New("query iterator not found")
	}

	payload, err := getQueryResponse(txContext, queryIter, queryStateNext.Id)
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
func (h *Handler) handleQueryStateClose(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
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
func (h *Handler) handleGetQueryResult(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	iterID := util.GenerateUUID()
	chaincodeName := h.chaincodeName()

	getQueryResult := &pb.GetQueryResult{}
	err := proto.Unmarshal(msg.Payload, getQueryResult)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	var executeIter commonledger.ResultsIterator
	if isCollectionSet(getQueryResult.Collection) {
		executeIter, err = txContext.txsimulator.ExecuteQueryOnPrivateData(chaincodeName, getQueryResult.Collection, getQueryResult.Query)
	} else {
		executeIter, err = txContext.txsimulator.ExecuteQuery(chaincodeName, getQueryResult.Query)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	txContext.InitializeQueryContext(iterID, executeIter)

	payload, err := getQueryResponse(txContext, executeIter, iterID)
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
func (h *Handler) handleGetHistoryForKey(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	iterID := util.GenerateUUID()
	chaincodeName := h.chaincodeName()

	getHistoryForKey := &pb.GetHistoryForKey{}
	err := proto.Unmarshal(msg.Payload, getHistoryForKey)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	historyIter, err := txContext.historyQueryExecutor.GetHistoryForKey(chaincodeName, getHistoryForKey.Key)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	txContext.InitializeQueryContext(iterID, historyIter)

	payload, err := getQueryResponse(txContext, historyIter, iterID)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		txContext.CleanupQueryContext(iterID)
		return nil, errors.WithStack(err)
	}

	chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func isCollectionSet(collection string) bool {
	if collection == "" {
		return false
	}
	return true
}

func (h *Handler) getTxContextForInvoke(channelID string, txid string, payload []byte, format string, args ...interface{}) (*TransactionContext, error) {
	// if we have a channelID, just get the txsim from isValidTxSim
	// if this is NOT an INVOKE_CHAINCODE, then let isValidTxSim handle retrieving the txContext
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
	targetInstance := getChaincodeInstance(chaincodeSpec.ChaincodeId.Name)
	if targetInstance == nil {
		return nil, errors.Errorf("failed to get chaincode instance for %s", chaincodeSpec.ChaincodeId.Name)
	}

	// If targetInstance is not an SCC, isValidTxSim should be called which will return an err.
	// We do not want to propagate calls to user CCs when the original call was to a SCC
	// without a channel context (ie, no ledger context).
	if isscc := h.sccp.IsSysCC(targetInstance.ChaincodeName); !isscc {
		// normal path - UCC invocation with an empty ("") channel: isValidTxSim will return an error
		return h.isValidTxSim("", txid, "could not get valid transaction")
	}

	// Calling SCC without a  ChainID, then the assumption this is an external SCC called by the client (special case) and no UCC involved,
	// so no Transaction Simulator validation needed as there are no commits to the ledger, get the txContext directly if it is not nil
	txContext := h.TXContexts.Get(channelID, txid)
	if txContext == nil {
		return nil, errors.New("failed to get transaction context")
	}

	return txContext, nil
}

func (h *Handler) handlePutState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	putState := &pb.PutState{}
	err := proto.Unmarshal(msg.Payload, putState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	chaincodeName := h.chaincodeName()
	if isCollectionSet(putState.Collection) {
		err = txContext.txsimulator.SetPrivateData(chaincodeName, putState.Collection, putState.Key, putState.Value)
	} else {
		err = txContext.txsimulator.SetState(chaincodeName, putState.Key, putState.Value)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func (h *Handler) handleDelState(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	delState := &pb.DelState{}
	err := proto.Unmarshal(msg.Payload, delState)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	chaincodeName := h.chaincodeName()
	if isCollectionSet(delState.Collection) {
		err = txContext.txsimulator.DeletePrivateData(chaincodeName, delState.Collection, delState.Key)
	} else {
		err = txContext.txsimulator.DeleteState(chaincodeName, delState.Key)
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Send response msg back to chaincode.
	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

// Handles requests that modify ledger state
func (h *Handler) handleInvokeChaincode(msg *pb.ChaincodeMessage, txContext *TransactionContext) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("[%s] C-call-C", shorttxid(msg.Txid))

	chaincodeSpec := &pb.ChaincodeSpec{}
	err := proto.Unmarshal(msg.Payload, chaincodeSpec)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}

	// Get the chaincodeID to invoke. The chaincodeID to be called may
	// contain composite info like "chaincode-name:version/channel-name"
	// We are not using version now but default to the latest
	targetInstance := getChaincodeInstance(chaincodeSpec.ChaincodeId.Name)
	chaincodeSpec.ChaincodeId.Name = targetInstance.ChaincodeName
	if targetInstance.ChainID == "" {
		// use caller's channel as the called chaincode is in the same channel
		targetInstance.ChainID = txContext.chainID
	}
	chaincodeLogger.Debugf("[%s] C-call-C %s on channel %s", shorttxid(msg.Txid), targetInstance.ChaincodeName, targetInstance.ChainID)

	err = h.checkACL(txContext.signedProp, txContext.proposal, targetInstance)
	if err != nil {
		chaincodeLogger.Errorf(
			"[%s] C-call-C %s on channel %s failed check ACL [%v]: [%s]",
			shorttxid(msg.Txid),
			targetInstance.ChaincodeName,
			targetInstance.ChainID,
			txContext.signedProp,
			err,
		)
		return nil, errors.WithStack(err)
	}

	// Set up a new context for the called chaincode if on a different channel
	// We grab the called channel's ledger simulator to hold the new state
	ctxt := context.Background()
	txsim := txContext.txsimulator
	historyQueryExecutor := txContext.historyQueryExecutor
	if targetInstance.ChainID != txContext.chainID {
		lgr := peer.GetLedger(targetInstance.ChainID)
		if lgr == nil {
			return nil, errors.Errorf("failed to find ledger for channel: %s ", targetInstance.ChainID)
		}

		txsim2, err := lgr.NewTxSimulator(msg.Txid)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer txsim2.Done()

		txsim = txsim2
	}
	ctxt = context.WithValue(ctxt, TXSimulatorKey, txsim)
	ctxt = context.WithValue(ctxt, HistoryQueryExecutorKey, historyQueryExecutor)

	chaincodeLogger.Debugf("[%s] getting chaincode data for %s on channel %s", shorttxid(msg.Txid), targetInstance.ChaincodeName, targetInstance.ChainID)

	// is the chaincode a system chaincode ?
	isscc := h.sccp.IsSysCC(targetInstance.ChaincodeName)

	var version string
	if !isscc {
		// if its a user chaincode, get the details
		cd, err := h.Lifecycle.GetChaincodeDefinition(ctxt, msg.Txid, txContext.signedProp, txContext.proposal, targetInstance.ChainID, targetInstance.ChaincodeName)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		version = cd.CCVersion()

		err = ccprovider.CheckInstantiationPolicy(targetInstance.ChaincodeName, version, cd.(*ccprovider.ChaincodeData))
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		// this is a system cc, just call it directly
		version = util.GetSysCCVersion()
	}

	// Launch the new chaincode if not already running
	chaincodeLogger.Debugf("[%s] launching chaincode %s on channel %s", shorttxid(msg.Txid), targetInstance.ChaincodeName, targetInstance.ChainID)

	cccid := ccprovider.NewCCContext(targetInstance.ChainID, targetInstance.ChaincodeName, version, msg.Txid, false, txContext.signedProp, txContext.proposal)
	cciSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: chaincodeSpec}

	// Execute the chaincode... this CANNOT be an init at least for now
	response, _, err := h.Executor.Execute(ctxt, cccid, cciSpec)
	if err != nil {
		return nil, errors.Wrap(err, "execute failed")
	}

	// payload is marshalled and send to the calling chaincode's shim which unmarshals and
	// sends it to chaincode
	res, err := proto.Marshal(response)
	if err != nil {
		return nil, errors.Wrap(err, "marshal failed")
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}, nil
}

func (h *Handler) setChaincodeProposal(signedProp *pb.SignedProposal, prop *pb.Proposal, msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debug("Setting chaincode proposal context...")
	if prop != nil {
		chaincodeLogger.Debug("Proposal different from nil. Creating chaincode proposal context...")

		// Check that also signedProp is different from nil
		if signedProp == nil {
			return errors.New("failed getting proposal context. Signed proposal is nil")
		}

		msg.Proposal = signedProp
	}
	return nil
}

func (h *Handler) Execute(ctxt context.Context, cccid *ccprovider.CCContext, msg *pb.ChaincodeMessage, timeout time.Duration) (*pb.ChaincodeMessage, error) {
	chaincodeLogger.Debugf("Entry")
	defer chaincodeLogger.Debugf("Exit")

	notfy, err := h.sendExecuteMessage(ctxt, cccid.ChainID, msg, cccid.SignedProposal, cccid.Proposal)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("error sending"))
	}

	var ccresp *pb.ChaincodeMessage
	select {
	case ccresp = <-notfy:
		// response is sent to user or calling chaincode. ChaincodeMessage_ERROR
		// are typically treated as error
	case <-time.After(timeout):
		err = errors.New("timeout expired while executing transaction")
	}

	// our responsibility to delete transaction context if sendExecuteMessage succeeded
	h.TXContexts.Delete(msg.ChannelId, msg.Txid)

	return ccresp, err
}

func (h *Handler) sendExecuteMessage(ctxt context.Context, chainID string, msg *pb.ChaincodeMessage, signedProp *pb.SignedProposal, prop *pb.Proposal) (chan *pb.ChaincodeMessage, error) {
	txctx, err := h.TXContexts.Create(ctxt, chainID, msg.Txid, signedProp, prop)
	if err != nil {
		return nil, err
	}
	chaincodeLogger.Debugf("[%s]Inside sendExecuteMessage. Message %s", shorttxid(msg.Txid), msg.Type)

	// if security is disabled the context elements will just be nil
	if err = h.setChaincodeProposal(signedProp, prop, msg); err != nil {
		return nil, err
	}

	chaincodeLogger.Debugf("[%s]sendExecuteMsg trigger event %s", shorttxid(msg.Txid), msg.Type)
	h.serialSendAsync(msg, true)

	return txctx.responseNotifier, nil
}

func (h *Handler) Close() {
	h.TXContexts.Close()
}
