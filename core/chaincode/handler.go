/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type state string

const (
	created     state = "created"     //start state
	established state = "established" //in: CREATED, rcv:  REGISTER, send: REGISTERED
	ready       state = "ready"       //in:ESTABLISHED, rcv:COMPLETED

)

var chaincodeLogger = flogging.MustGetLogger("chaincode")

type transactionContext struct {
	chainID          string
	signedProp       *pb.SignedProposal
	proposal         *pb.Proposal
	responseNotifier chan *pb.ChaincodeMessage

	// tracks open iterators used for range queries
	queryIteratorMap    map[string]commonledger.ResultsIterator
	pendingQueryResults map[string]*pendingQueryResult

	txsimulator          ledger.TxSimulator
	historyQueryExecutor ledger.HistoryQueryExecutor
}

type pendingQueryResult struct {
	batch []*pb.QueryResultBytes
	count int
}

type stateHandlers map[pb.ChaincodeMessage_Type]func(*pb.ChaincodeMessage)

// Handler responsible for management of Peer's side of chaincode stream
type Handler struct {
	sync.Mutex
	//peer to shim grpc serializer. User only in serialSend
	serialLock  sync.Mutex
	ChatStream  ccintf.ChaincodeStream
	state       state
	ChaincodeID *pb.ChaincodeID
	ccInstance  *sysccprovider.ChaincodeInstance

	chaincodeSupport *ChaincodeSupport
	registered       bool
	readyNotify      chan bool

	//chan to pass error in sync and nonsync mode
	errChan chan error
	// Map of tx txid to either invoke tx. Each tx will be
	// added prior to execute and remove when done execute
	txCtxs map[string]*transactionContext

	txidMap map[string]bool

	//handlers for each state of the handler
	readyStateHandlers  stateHandlers
	createStateHandlers stateHandlers
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

//gets chaincode instance from the canonical name of the chaincode.
//Called exactly once per chaincode when registering chaincode.
//This is needed for the "one-instance-per-chain" model when
//starting up the chaincode for each chain. It will still
//work for the "one-instance-for-all-chains" as the version
//and suffix will just be absent (also note that LSCC reserves
//"/:[]${}" as special chars mainly for such namespace uses)
func (handler *Handler) decomposeRegisteredName(cid *pb.ChaincodeID) {
	handler.ccInstance = getChaincodeInstance(cid.Name)
}

func getChaincodeInstance(ccName string) *sysccprovider.ChaincodeInstance {
	b := []byte(ccName)
	ci := &sysccprovider.ChaincodeInstance{}

	//compute suffix (ie, chain name)
	i := bytes.IndexByte(b, '/')
	if i >= 0 {
		if i < len(b)-1 {
			ci.ChainID = string(b[i+1:])
		}
		b = b[:i]
	}

	//compute version
	i = bytes.IndexByte(b, ':')
	if i >= 0 {
		if i < len(b)-1 {
			ci.ChaincodeVersion = string(b[i+1:])
		}
		b = b[:i]
	}
	// remaining is the chaincode name
	ci.ChaincodeName = string(b)

	return ci
}

func (handler *Handler) getCCRootName() string {
	return handler.ccInstance.ChaincodeName
}

//serialSend serializes msgs so gRPC will be happy
func (handler *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	handler.serialLock.Lock()
	defer handler.serialLock.Unlock()

	var err error
	if err = handler.ChatStream.Send(msg); err != nil {
		err = errors.WithMessage(err, fmt.Sprintf("[%s]Error sending %s", shorttxid(msg.Txid), msg.Type))
		chaincodeLogger.Errorf("%+v", err)
	}
	return err
}

//serialSendAsync serves the same purpose as serialSend (serialize msgs so gRPC will
//be happy). In addition, it is also asynchronous so send-remoterecv--localrecv loop
//can be nonblocking. Only errors need to be handled and these are handled by
//communication on supplied error channel. A typical use will be a non-blocking or
//nil channel
func (handler *Handler) serialSendAsync(msg *pb.ChaincodeMessage, sendErr bool) {
	go func() {
		if err := handler.serialSend(msg); err != nil {
			if sendErr {
				handler.errChan <- err
			}
		}
	}()
}

//transaction context id should be composed of chainID and txid. While
//needed for CC-2-CC, it also allows users to concurrently send proposals
//with the same TXID to the SAME CC on multiple channels
func (handler *Handler) getTxCtxId(chainID string, txid string) string {
	return chainID + txid
}

func (handler *Handler) createTxContext(ctxt context.Context, chainID string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal) (*transactionContext, error) {
	if handler.txCtxs == nil {
		return nil, errors.Errorf("cannot create notifier for txid: %s", txid)
	}
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(chainID, txid)
	if handler.txCtxs[txCtxID] != nil {
		return nil, errors.Errorf("txid: %s(%s) exists", txid, chainID)
	}
	txctx := &transactionContext{chainID: chainID, signedProp: signedProp,
		proposal: prop, responseNotifier: make(chan *pb.ChaincodeMessage, 1),
		queryIteratorMap:    make(map[string]commonledger.ResultsIterator),
		pendingQueryResults: make(map[string]*pendingQueryResult)}
	handler.txCtxs[txCtxID] = txctx
	txctx.txsimulator = getTxSimulator(ctxt)
	txctx.historyQueryExecutor = getHistoryQueryExecutor(ctxt)

	return txctx, nil
}

func (handler *Handler) getTxContext(chainID, txid string) *transactionContext {
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(chainID, txid)
	return handler.txCtxs[txCtxID]
}

func (handler *Handler) deleteTxContext(chainID, txid string) {
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(chainID, txid)
	if handler.txCtxs != nil {
		delete(handler.txCtxs, txCtxID)
	}
}

func (handler *Handler) initializeQueryContext(txContext *transactionContext, queryID string,
	queryIterator commonledger.ResultsIterator) {
	handler.Lock()
	defer handler.Unlock()
	txContext.queryIteratorMap[queryID] = queryIterator
	txContext.pendingQueryResults[queryID] = &pendingQueryResult{batch: make([]*pb.QueryResultBytes, 0)}
}

func (handler *Handler) getQueryIterator(txContext *transactionContext, queryID string) commonledger.ResultsIterator {
	handler.Lock()
	defer handler.Unlock()
	return txContext.queryIteratorMap[queryID]
}

func (handler *Handler) cleanupQueryContext(txContext *transactionContext, queryID string) {
	handler.Lock()
	defer handler.Unlock()
	txContext.queryIteratorMap[queryID].Close()
	delete(txContext.queryIteratorMap, queryID)
	delete(txContext.pendingQueryResults, queryID)
}

// Check if the transactor is allow to call this chaincode on this channel
func (handler *Handler) checkACL(signedProp *pb.SignedProposal, proposal *pb.Proposal, ccIns *sysccprovider.ChaincodeInstance) error {
	// ensure that we don't invoke a system chaincode
	// that is not invokable through a cc2cc invocation
	if sysccprovider.GetSystemChaincodeProvider().IsSysCCAndNotInvokableCC2CC(ccIns.ChaincodeName) {
		return errors.Errorf("system chaincode %s cannot be invoked with a cc2cc invocation", ccIns.ChaincodeName)
	}

	// if we are here, all we know is that the invoked chaincode is either
	// - a system chaincode that *is* invokable through a cc2cc
	//   (but we may still have to determine whether the invoker
	//   can perform this invocation)
	// - an application chaincode (and we still need to determine
	//   whether the invoker can invoke it)

	if sysccprovider.GetSystemChaincodeProvider().IsSysCC(ccIns.ChaincodeName) {
		// Allow this call
		return nil
	}

	// A Nil signedProp will be rejected for non-system chaincodes
	if signedProp == nil {
		return errors.Errorf("signed proposal must not be nil from caller [%s]", ccIns.String())
	}

	return aclmgmt.GetACLProvider().CheckACL(resources.CC2CC, ccIns.ChainID, signedProp)
}

func (handler *Handler) deregister() error {
	if handler.registered {
		handler.chaincodeSupport.deregisterHandler(handler)
	}
	return nil
}

func (handler *Handler) waitForKeepaliveTimer() <-chan time.Time {
	if handler.chaincodeSupport.keepalive > 0 {
		c := time.After(handler.chaincodeSupport.keepalive)
		return c
	}
	//no one will signal this channel, listener blocks forever
	c := make(chan time.Time, 1)
	return c
}

func (handler *Handler) processStream() error {
	defer handler.deregister()

	//holds return values from gRPC Recv below
	type recvMsg struct {
		msg *pb.ChaincodeMessage
		err error
	}

	msgAvail := make(chan *recvMsg)

	var in *pb.ChaincodeMessage
	var err error

	//recv is used to spin Recv routine after previous received msg
	//has been processed
	recv := true

	//catch send errors and bail now that sends aren't synchronous
	for {
		in = nil
		err = nil
		if recv {
			recv = false
			go func() {
				in2, err2 := handler.ChatStream.Recv()
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
		case sendErr := <-handler.errChan:
			err = errors.Wrapf(sendErr, "received error while sending message, ending chaincode support stream")
			chaincodeLogger.Errorf("%s", err)
			return err
		case <-handler.waitForKeepaliveTimer():
			if handler.chaincodeSupport.keepalive <= 0 {
				chaincodeLogger.Errorf("Invalid select: keepalive not on (keepalive=%d)", handler.chaincodeSupport.keepalive)
				continue
			}

			//if no error message from serialSend, KEEPALIVE happy, and don't care about error
			//(maybe it'll work later)
			handler.serialSendAsync(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE}, false)
			continue
		}

		err = handler.handleMessage(in)
		if err != nil {
			err = errors.WithMessage(err, "error handling message, ending stream")
			chaincodeLogger.Errorf("[%s] %+v", shorttxid(in.Txid), err)
			return err
		}
	}
}

// HandleChaincodeStream Main loop for handling the associated Chaincode stream
func HandleChaincodeStream(chaincodeSupport *ChaincodeSupport, ctxt context.Context, stream ccintf.ChaincodeStream) error {
	deadline, ok := ctxt.Deadline()
	chaincodeLogger.Debugf("Current context deadline = %s, ok = %v", deadline, ok)
	handler := newChaincodeSupportHandler(chaincodeSupport, stream)
	return handler.processStream()
}

func newChaincodeSupportHandler(chaincodeSupport *ChaincodeSupport, peerChatStream ccintf.ChaincodeStream) *Handler {
	v := &Handler{
		ChatStream:       peerChatStream,
		chaincodeSupport: chaincodeSupport,
		state:            created,
		errChan:          make(chan error, 1),
	}

	v.readyStateHandlers = stateHandlers{
		//events from CC at the end of a TX  that require notification
		pb.ChaincodeMessage_COMPLETED: v.notify,
		pb.ChaincodeMessage_ERROR:     v.notify,

		//state requests from CC that require processing
		pb.ChaincodeMessage_GET_STATE:           v.handleGetState,
		pb.ChaincodeMessage_GET_STATE_BY_RANGE:  v.handleGetStateByRange,
		pb.ChaincodeMessage_GET_QUERY_RESULT:    v.handleGetQueryResult,
		pb.ChaincodeMessage_GET_HISTORY_FOR_KEY: v.handleGetHistoryForKey,
		pb.ChaincodeMessage_QUERY_STATE_NEXT:    v.handleQueryStateNext,
		pb.ChaincodeMessage_QUERY_STATE_CLOSE:   v.handleQueryStateClose,
		pb.ChaincodeMessage_PUT_STATE:           v.handleModState,
		pb.ChaincodeMessage_DEL_STATE:           v.handleModState,
		pb.ChaincodeMessage_INVOKE_CHAINCODE:    v.handleModState,
	}

	v.createStateHandlers = stateHandlers{
		//move from created to established via REGISTER
		pb.ChaincodeMessage_REGISTER: v.handleRegister,
	}

	return v
}

func (handler *Handler) createTXIDEntry(channelID, txid string) bool {
	if handler.txidMap == nil {
		return false
	}
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(channelID, txid)
	if handler.txidMap[txCtxID] {
		return false
	}
	handler.txidMap[txCtxID] = true
	return handler.txidMap[txCtxID]
}

func (handler *Handler) deleteTXIDEntry(channelID, txid string) {
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(channelID, txid)
	if handler.txidMap != nil {
		delete(handler.txidMap, txCtxID)
	} else {
		chaincodeLogger.Warningf("TXID %s not found!", txCtxID)
	}
}

//sendReady sends READY to chaincode serially (just like REGISTER)
func (handler *Handler) sendReady() error {
	chaincodeLogger.Debugf("sending READY for chaincode %+v", handler.ChaincodeID)
	ccMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY}

	//if error in sending tear down the handler
	if err := handler.serialSend(ccMsg); err != nil {
		chaincodeLogger.Errorf("error sending READY (%s) for chaincode %+v", err, handler.ChaincodeID)
		return err
	}

	handler.state = ready

	chaincodeLogger.Debugf("Changed to state ready for chaincode %+v", handler.ChaincodeID)

	return nil
}

//notifyDuringStartup will send ready on registration
func (handler *Handler) notifyDuringStartup(val bool) {
	//if USER_RUNS_CC readyNotify will be nil
	if handler.readyNotify != nil {
		if val {
			//if send failed, notify failure which will initiate
			//tearing down
			if err := handler.sendReady(); err != nil {
				val = false
			}
		}
		handler.readyNotify <- val
		return
	}

	chaincodeLogger.Debug("nothing to notify (dev mode ?)")

	//In theory, we don't even need a devmode flag in the peer anymore
	//as the chaincode is brought up without any context (ledger context
	//in particular). What this means is we can have - in theory - a nondev
	//environment where we can attach a chaincode manually. This could be
	//useful .... but for now lets just be conservative and allow manual
	//chaincode only in dev mode (ie, peer started with --peer-chaincodedev=true)
	if handler.chaincodeSupport.userRunsCC {
		if val {
			handler.sendReady()
		} else {
			chaincodeLogger.Errorf("(dev mode) Error during startup .. not sending READY")
		}
	} else {
		chaincodeLogger.Warningf("trying to manually run chaincode when not in devmode ?")
	}
}

// handleRegister is invoked when chaincode tries to register.
func (handler *Handler) handleRegister(msg *pb.ChaincodeMessage) {
	chaincodeLogger.Debugf("Received %s in state %s", msg.Type, handler.state)
	chaincodeID := &pb.ChaincodeID{}
	err := proto.Unmarshal(msg.Payload, chaincodeID)
	if err != nil {
		chaincodeLogger.Errorf("Error in received %s, could NOT unmarshal registration info: %s", pb.ChaincodeMessage_REGISTER, err)
		return
	}

	// Now register with the chaincodeSupport
	handler.ChaincodeID = chaincodeID
	err = handler.chaincodeSupport.registerHandler(handler)
	if err != nil {
		handler.notifyDuringStartup(false)
		return
	}

	//get the component parts so we can use the root chaincode
	//name in keys
	handler.decomposeRegisteredName(handler.ChaincodeID)

	chaincodeLogger.Debugf("Got %s for chaincodeID = %s, sending back %s", pb.ChaincodeMessage_REGISTER, chaincodeID, pb.ChaincodeMessage_REGISTERED)
	if err := handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}); err != nil {
		chaincodeLogger.Errorf("Error sending %s: %s", pb.ChaincodeMessage_REGISTERED, err)
		handler.notifyDuringStartup(false)
		return
	}

	handler.state = established

	chaincodeLogger.Debugf("Changed state to established for %+v", handler.ChaincodeID)

	//for dev mode this will also move to ready automatically
	handler.notifyDuringStartup(true)
}

func (handler *Handler) notify(msg *pb.ChaincodeMessage) {
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(msg.ChannelId, msg.Txid)
	tctx := handler.txCtxs[txCtxID]
	if tctx == nil {
		chaincodeLogger.Debugf("notifier Txid:%s, channelID:%s does not exist for handleing message %s", msg.Txid, msg.ChannelId, msg.Type)
	} else {
		chaincodeLogger.Debugf("[%s]notifying Txid:%s, channelID:%s", shorttxid(msg.Txid), msg.Txid, msg.ChannelId)
		tctx.responseNotifier <- msg

		// clean up queryIteratorMap
		for _, v := range tctx.queryIteratorMap {
			v.Close()
		}
	}
}

// is this a txid for which there is a valid txsim
func (handler *Handler) isValidTxSim(channelID string, txid string, fmtStr string, args ...interface{}) (*transactionContext, *pb.ChaincodeMessage) {
	txContext := handler.getTxContext(channelID, txid)
	if txContext == nil || txContext.txsimulator == nil {
		// Send error msg back to chaincode. No ledger context
		errStr := fmt.Sprintf(fmtStr, args...)
		chaincodeLogger.Errorf(errStr)
		return nil, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(errStr), Txid: txid, ChannelId: channelID}
	}
	return txContext, nil
}

//register Txid to prevent overlapping handle messages from chaincode
func (handler *Handler) registerTxid(msg *pb.ChaincodeMessage) bool {
	// Check if this is the unique state request from this chaincode txid
	if uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid); !uniqueReq {
		// Drop this request
		chaincodeLogger.Errorf("[%s]Another request pending for this CC: %s, Txid: %s, ChannelID: %s. Cannot process.", shorttxid(msg.Txid), handler.ChaincodeID.Name, msg.Txid, msg.ChannelId)
		return false
	}
	return true
}

//deregister current txid on completion
func (handler *Handler) deRegisterTxid(msg, serialSendMsg *pb.ChaincodeMessage, serial bool) {
	handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
	chaincodeLogger.Debugf("[%s]send %s(serial-%t)", shorttxid(serialSendMsg.Txid), serialSendMsg.Type, serial)
	handler.serialSendAsync(serialSendMsg, serial)
}

// Handles query to ledger to get state
func (handler *Handler) handleGetState(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE)
		if !handler.registerTxid(msg) {
			return
		}

		var serialSendMsg *pb.ChaincodeMessage
		var txContext *transactionContext
		txContext, serialSendMsg = handler.isValidTxSim(msg.ChannelId, msg.Txid,
			"[%s]No ledger context for GetState. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)

		defer func() {
			handler.deRegisterTxid(msg, serialSendMsg, false)
		}()

		if txContext == nil {
			return
		}

		key := string(msg.Payload)
		getState := &pb.GetState{}
		unmarshalErr := proto.Unmarshal(msg.Payload, getState)
		if unmarshalErr != nil {
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(unmarshalErr.Error()), Txid: msg.Txid, ChannelId: msg.ChannelId}
		}
		chaincodeID := handler.getCCRootName()
		chaincodeLogger.Debugf("[%s] getting state for chaincode %s, key %s, channel %s",
			shorttxid(msg.Txid), chaincodeID, getState.Key, txContext.chainID)

		var res []byte
		var err error
		if isCollectionSet(getState.Collection) {
			res, err = txContext.txsimulator.GetPrivateData(chaincodeID, getState.Collection, getState.Key)
		} else {
			res, err = txContext.txsimulator.GetState(chaincodeID, getState.Key)
		}

		if err != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("[%s]Failed to get chaincode state(%s). Sending %s",
				shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		} else if res == nil {
			//The state object being requested does not exist
			chaincodeLogger.Debugf("[%s]No state associated with key: %s. Sending %s with an empty payload",
				shorttxid(msg.Txid), key, pb.ChaincodeMessage_RESPONSE)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}
		} else {
			// Send response msg back to chaincode. GetState will not trigger event
			chaincodeLogger.Debugf("[%s]Got state. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_RESPONSE)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

	}()
}

// Handles query to ledger to rage query state
func (handler *Handler) handleGetStateByRange(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE_BY_RANGE)

		if !handler.registerTxid(msg) {
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deRegisterTxid(msg, serialSendMsg, false)
		}()

		getStateByRange := &pb.GetStateByRange{}
		unmarshalErr := proto.Unmarshal(msg.Payload, getStateByRange)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall range query request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
			return
		}

		iterID := util.GenerateUUID()

		var txContext *transactionContext

		txContext, serialSendMsg = handler.isValidTxSim(msg.ChannelId, msg.Txid, "[%s]No ledger context for GetStateByRange. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
		if txContext == nil {
			return
		}
		chaincodeID := handler.getCCRootName()

		errHandler := func(err error, iter commonledger.ResultsIterator, errFmt string, errArgs ...interface{}) {
			if iter != nil {
				handler.cleanupQueryContext(txContext, iterID)
			}
			payload := []byte(err.Error())
			chaincodeLogger.Errorf(errFmt, errArgs...)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

		var rangeIter commonledger.ResultsIterator
		var err error

		if isCollectionSet(getStateByRange.Collection) {
			rangeIter, err = txContext.txsimulator.GetPrivateDataRangeScanIterator(chaincodeID, getStateByRange.Collection, getStateByRange.StartKey, getStateByRange.EndKey)
		} else {
			rangeIter, err = txContext.txsimulator.GetStateRangeScanIterator(chaincodeID, getStateByRange.StartKey, getStateByRange.EndKey)
		}
		if err != nil {
			errHandler(err, nil, "Failed to get ledger scan iterator. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		handler.initializeQueryContext(txContext, iterID, rangeIter)

		var payload *pb.QueryResponse
		payload, err = getQueryResponse(handler, txContext, rangeIter, iterID)
		if err != nil {
			errHandler(err, rangeIter, "Failed to get query result. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		var payloadBytes []byte
		payloadBytes, err = proto.Marshal(payload)
		if err != nil {
			errHandler(err, rangeIter, "Failed to marshal response. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}
		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}

	}()
}

const maxResultLimit = 100

//getQueryResponse takes an iterator and fetch state to construct QueryResponse
func getQueryResponse(handler *Handler, txContext *transactionContext, iter commonledger.ResultsIterator,
	iterID string) (*pb.QueryResponse, error) {
	pendingQueryResults := txContext.pendingQueryResults[iterID]
	for {
		queryResult, err := iter.Next()
		switch {
		case err != nil:
			chaincodeLogger.Errorf("Failed to get query result from iterator")
			handler.cleanupQueryContext(txContext, iterID)
			return nil, err
		case queryResult == nil:
			// nil response from iterator indicates end of query results
			batch := pendingQueryResults.cut()
			handler.cleanupQueryContext(txContext, iterID)
			return &pb.QueryResponse{Results: batch, HasMore: false, Id: iterID}, nil
		case pendingQueryResults.count == maxResultLimit:
			// max number of results queued up, cut batch, then add current result to pending batch
			batch := pendingQueryResults.cut()
			if err := pendingQueryResults.add(queryResult); err != nil {
				handler.cleanupQueryContext(txContext, iterID)
				return nil, err
			}
			return &pb.QueryResponse{Results: batch, HasMore: true, Id: iterID}, nil
		default:
			if err := pendingQueryResults.add(queryResult); err != nil {
				handler.cleanupQueryContext(txContext, iterID)
				return nil, err
			}
		}
	}
}

func (p *pendingQueryResult) cut() []*pb.QueryResultBytes {
	batch := p.batch
	p.batch = nil
	p.count = 0
	return batch
}

func (p *pendingQueryResult) add(queryResult commonledger.QueryResult) error {
	queryResultBytes, err := proto.Marshal(queryResult.(proto.Message))
	if err != nil {
		chaincodeLogger.Errorf("Failed to get encode query result as bytes")
		return err
	}
	p.batch = append(p.batch, &pb.QueryResultBytes{ResultBytes: queryResultBytes})
	p.count = len(p.batch)
	return nil
}

// Handles query to ledger for query state next
func (handler *Handler) handleQueryStateNext(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_NEXT)

		if !handler.registerTxid(msg) {
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deRegisterTxid(msg, serialSendMsg, false)
		}()

		var txContext *transactionContext
		var queryStateNext *pb.QueryStateNext

		errHandler := func(payload []byte, iter commonledger.ResultsIterator, errFmt string, errArgs ...interface{}) {
			if iter != nil {
				handler.cleanupQueryContext(txContext, queryStateNext.Id)
			}
			chaincodeLogger.Errorf(errFmt, errArgs...)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

		queryStateNext = &pb.QueryStateNext{}

		unmarshalErr := proto.Unmarshal(msg.Payload, queryStateNext)
		if unmarshalErr != nil {
			errHandler([]byte(unmarshalErr.Error()), nil, "Failed to unmarshall state next query request. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		txContext = handler.getTxContext(msg.ChannelId, msg.Txid)
		if txContext == nil {
			errHandler([]byte("transaction context not found (timed out ?)"), nil, "[%s]Failed to get transaction context. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
			return
		}

		queryIter := handler.getQueryIterator(txContext, queryStateNext.Id)

		if queryIter == nil {
			errHandler([]byte("query iterator not found"), nil, "query iterator not found. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		payload, err := getQueryResponse(handler, txContext, queryIter, queryStateNext.Id)
		if err != nil {
			errHandler([]byte(err.Error()), queryIter, "Failed to get query result. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {
			errHandler([]byte(err.Error()), queryIter, "Failed to marshal response. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}
		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}

	}()
}

// Handles the closing of a state iterator
func (handler *Handler) handleQueryStateClose(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_CLOSE)

		if !handler.registerTxid(msg) {
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deRegisterTxid(msg, serialSendMsg, false)
		}()

		errHandler := func(payload []byte, errFmt string, errArgs ...interface{}) {
			chaincodeLogger.Errorf(errFmt, errArgs...)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

		queryStateClose := &pb.QueryStateClose{}
		unmarshalErr := proto.Unmarshal(msg.Payload, queryStateClose)
		if unmarshalErr != nil {
			errHandler([]byte(unmarshalErr.Error()), "Failed to unmarshall state query close request. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		txContext := handler.getTxContext(msg.ChannelId, msg.Txid)
		if txContext == nil {
			errHandler([]byte("transaction context not found (timed out ?)"), "[%s]Failed to get transaction context. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
			return
		}

		iter := handler.getQueryIterator(txContext, queryStateClose.Id)
		if iter != nil {
			handler.cleanupQueryContext(txContext, queryStateClose.Id)
		}

		payload := &pb.QueryResponse{HasMore: false, Id: queryStateClose.Id}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {
			errHandler([]byte(err.Error()), "Failed marshall resopnse. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		chaincodeLogger.Debugf("Closed. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}

	}()
}

// Handles query to ledger to execute query state
func (handler *Handler) handleGetQueryResult(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_QUERY_RESULT)

		if !handler.registerTxid(msg) {
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deRegisterTxid(msg, serialSendMsg, false)
		}()

		var txContext *transactionContext
		var iterID string

		errHandler := func(payload []byte, iter commonledger.ResultsIterator, errFmt string, errArgs ...interface{}) {
			if iter != nil {
				handler.cleanupQueryContext(txContext, iterID)
			}
			chaincodeLogger.Errorf(errFmt, errArgs...)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

		getQueryResult := &pb.GetQueryResult{}
		unmarshalErr := proto.Unmarshal(msg.Payload, getQueryResult)
		if unmarshalErr != nil {
			errHandler([]byte(unmarshalErr.Error()), nil, "Failed to unmarshall query request. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		iterID = util.GenerateUUID()

		txContext, serialSendMsg = handler.isValidTxSim(msg.ChannelId, msg.Txid, "[%s]No ledger context for GetQueryResult. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
		if txContext == nil {
			return
		}

		chaincodeID := handler.getCCRootName()

		var err error
		var executeIter commonledger.ResultsIterator
		if isCollectionSet(getQueryResult.Collection) {
			executeIter, err = txContext.txsimulator.ExecuteQueryOnPrivateData(chaincodeID, getQueryResult.Collection, getQueryResult.Query)
		} else {
			executeIter, err = txContext.txsimulator.ExecuteQuery(chaincodeID, getQueryResult.Query)
		}

		if err != nil {
			errHandler([]byte(err.Error()), nil, "Failed to get ledger query iterator. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		handler.initializeQueryContext(txContext, iterID, executeIter)

		var payload *pb.QueryResponse
		payload, err = getQueryResponse(handler, txContext, executeIter, iterID)
		if err != nil {
			errHandler([]byte(err.Error()), executeIter, "Failed to get query result. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		var payloadBytes []byte
		payloadBytes, err = proto.Marshal(payload)
		if err != nil {
			errHandler([]byte(err.Error()), executeIter, "Failed marshall response. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}

	}()
}

// Handles query to ledger history db
func (handler *Handler) handleGetHistoryForKey(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", pb.ChaincodeMessage_GET_HISTORY_FOR_KEY)

		if !handler.registerTxid(msg) {
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deRegisterTxid(msg, serialSendMsg, false)
		}()

		var iterID string
		var txContext *transactionContext

		errHandler := func(payload []byte, iter commonledger.ResultsIterator, errFmt string, errArgs ...interface{}) {
			if iter != nil {
				handler.cleanupQueryContext(txContext, iterID)
			}
			chaincodeLogger.Errorf(errFmt, errArgs...)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

		getHistoryForKey := &pb.GetHistoryForKey{}
		unmarshalErr := proto.Unmarshal(msg.Payload, getHistoryForKey)
		if unmarshalErr != nil {
			errHandler([]byte(unmarshalErr.Error()), nil, "Failed to unmarshall query request. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		iterID = util.GenerateUUID()

		txContext, serialSendMsg = handler.isValidTxSim(msg.ChannelId, msg.Txid, "[%s]No ledger context for GetHistoryForKey. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
		if txContext == nil {
			return
		}
		chaincodeID := handler.getCCRootName()

		historyIter, err := txContext.historyQueryExecutor.GetHistoryForKey(chaincodeID, getHistoryForKey.Key)
		if err != nil {
			errHandler([]byte(err.Error()), nil, "Failed to get ledger history iterator. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		handler.initializeQueryContext(txContext, iterID, historyIter)

		var payload *pb.QueryResponse
		payload, err = getQueryResponse(handler, txContext, historyIter, iterID)

		if err != nil {
			errHandler([]byte(err.Error()), historyIter, "Failed to get query result. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		var payloadBytes []byte
		payloadBytes, err = proto.Marshal(payload)
		if err != nil {
			errHandler([]byte(err.Error()), historyIter, "Failed marshal response. Sending %s", pb.ChaincodeMessage_ERROR)
			return
		}

		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid, ChannelId: msg.ChannelId}

	}()
}

func isCollectionSet(collection string) bool {
	if collection == "" {
		return false
	}
	return true
}

func (handler *Handler) getTxContextForMessage(channelID string, txid string, msgType string, payload []byte, fmtStr string, args ...interface{}) (*transactionContext, *pb.ChaincodeMessage) {
	//if we have a channelID, just get the txsim from isValidTxSim
	//if this is NOT an INVOKE_CHAINCODE, then let isValidTxSim handle retrieving the txContext
	if channelID != "" || msgType != pb.ChaincodeMessage_INVOKE_CHAINCODE.String() {
		return handler.isValidTxSim(channelID, txid, fmtStr, args)
	}

	var calledCcIns *sysccprovider.ChaincodeInstance
	var txContext *transactionContext
	var triggerNextStateMsg *pb.ChaincodeMessage

	// prepare to get isscc (only for INVOKE_CHAINCODE, any other msgType will always call isValidTxSim to get the tx context)

	chaincodeSpec := &pb.ChaincodeSpec{}
	unmarshalErr := proto.Unmarshal(payload, chaincodeSpec)
	if unmarshalErr != nil {
		errStr := fmt.Sprintf("[%s]Unable to decipher payload. Sending %s", shorttxid(txid), pb.ChaincodeMessage_ERROR)
		triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(errStr), Txid: txid}
		return nil, triggerNextStateMsg
	}
	// Get the chaincodeID to invoke. The chaincodeID to be called may
	// contain composite info like "chaincode-name:version/channel-name"
	// We are not using version now but default to the latest
	if calledCcIns = getChaincodeInstance(chaincodeSpec.ChaincodeId.Name); calledCcIns == nil {
		triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte("could not get chaincode name for INVOKE_CHAINCODE"), Txid: txid}
		return nil, triggerNextStateMsg
	}

	//   If calledCcIns is not an SCC, isValidTxSim should be called which will return an err.
	//   We do not want to propagate calls to user CCs when the original call was to a SCC
	//   without a channel context (ie, no ledger context).
	if isscc := sysccprovider.GetSystemChaincodeProvider().IsSysCC(calledCcIns.ChaincodeName); !isscc {
		// normal path - UCC invocation with an empty ("") channel: isValidTxSim will return an error
		return handler.isValidTxSim("", txid, fmtStr, args)
	}

	// Calling SCC without a  ChainID, then the assumption this is an external SCC called by the client (special case) and no UCC involved,
	// so no Transaction Simulator validation needed as there are no commits to the ledger, get the txContext directly if it is not nil
	if txContext = handler.getTxContext(channelID, txid); txContext == nil {
		errStr := fmt.Sprintf(fmtStr, args)
		triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(errStr), Txid: txid}
		return nil, triggerNextStateMsg
	}

	return txContext, nil
}

// Handles requests that modify ledger state
func (handler *Handler) handleModState(msg *pb.ChaincodeMessage) {
	go func() {
		chaincodeLogger.Debugf("[%s]handling %s from chaincode", shorttxid(msg.Txid), msg.Type)

		if !handler.registerTxid(msg) {
			return
		}

		var triggerNextStateMsg *pb.ChaincodeMessage
		var txContext *transactionContext
		txContext, triggerNextStateMsg = handler.getTxContextForMessage(msg.ChannelId, msg.Txid, msg.Type.String(), msg.Payload,
			"[%s]No ledger context for %s. Sending %s", shorttxid(msg.Txid), msg.Type, pb.ChaincodeMessage_ERROR)

		defer func() {
			handler.deRegisterTxid(msg, triggerNextStateMsg, false)
		}()

		if txContext == nil {
			return
		}

		errHandler := func(payload []byte, errFmt string, errArgs ...interface{}) {
			chaincodeLogger.Errorf(errFmt, errArgs...)
			triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChannelId: msg.ChannelId}
		}

		chaincodeID := handler.getCCRootName()
		var err error
		var res []byte

		if msg.Type.String() == pb.ChaincodeMessage_PUT_STATE.String() {
			putState := &pb.PutState{}
			unmarshalErr := proto.Unmarshal(msg.Payload, putState)
			if unmarshalErr != nil {
				errHandler([]byte(unmarshalErr.Error()), "[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				return
			}

			if isCollectionSet(putState.Collection) {
				err = txContext.txsimulator.SetPrivateData(chaincodeID, putState.Collection, putState.Key, putState.Value)
			} else {
				err = txContext.txsimulator.SetState(chaincodeID, putState.Key, putState.Value)
			}

		} else if msg.Type.String() == pb.ChaincodeMessage_DEL_STATE.String() {
			// Invoke ledger to delete state
			delState := &pb.DelState{}
			unmarshalErr := proto.Unmarshal(msg.Payload, delState)
			if unmarshalErr != nil {
				errHandler([]byte(unmarshalErr.Error()), "[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				return
			}

			if isCollectionSet(delState.Collection) {
				err = txContext.txsimulator.DeletePrivateData(chaincodeID, delState.Collection, delState.Key)
			} else {
				err = txContext.txsimulator.DeleteState(chaincodeID, delState.Key)
			}
		} else if msg.Type.String() == pb.ChaincodeMessage_INVOKE_CHAINCODE.String() {
			chaincodeLogger.Debugf("[%s] C-call-C", shorttxid(msg.Txid))
			chaincodeSpec := &pb.ChaincodeSpec{}
			unmarshalErr := proto.Unmarshal(msg.Payload, chaincodeSpec)
			if unmarshalErr != nil {
				payload := []byte(unmarshalErr.Error())
				chaincodeLogger.Debugf("[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				return
			}

			// Get the chaincodeID to invoke. The chaincodeID to be called may
			// contain composite info like "chaincode-name:version/channel-name"
			// We are not using version now but default to the latest
			calledCcIns := getChaincodeInstance(chaincodeSpec.ChaincodeId.Name)
			chaincodeSpec.ChaincodeId.Name = calledCcIns.ChaincodeName
			if calledCcIns.ChainID == "" {
				// use caller's channel as the called chaincode is in the same channel
				calledCcIns.ChainID = txContext.chainID
			}
			chaincodeLogger.Debugf("[%s] C-call-C %s on channel %s",
				shorttxid(msg.Txid), calledCcIns.ChaincodeName, calledCcIns.ChainID)

			err := handler.checkACL(txContext.signedProp, txContext.proposal, calledCcIns)
			if err != nil {
				chaincodeLogger.Errorf("[%s] C-call-C %s on channel %s failed check ACL [%v]: [%s]",
					shorttxid(msg.Txid), calledCcIns.ChaincodeName, calledCcIns.ChainID, txContext.signedProp, err)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR,
					Payload: []byte(err.Error()), Txid: msg.Txid}
				return
			}

			// Set up a new context for the called chaincode if on a different channel
			// We grab the called channel's ledger simulator to hold the new state
			ctxt := context.Background()
			txsim := txContext.txsimulator
			historyQueryExecutor := txContext.historyQueryExecutor
			if calledCcIns.ChainID != txContext.chainID {
				lgr := peer.GetLedger(calledCcIns.ChainID)
				if lgr == nil {
					payload := "Failed to find ledger for called channel " + calledCcIns.ChainID
					triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR,
						Payload: []byte(payload), Txid: msg.Txid, ChannelId: msg.ChannelId}
					return
				}
				txsim2, err2 := lgr.NewTxSimulator(msg.Txid)
				if err2 != nil {
					triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR,
						Payload: []byte(err2.Error()), Txid: msg.Txid, ChannelId: msg.ChannelId}
					return
				}
				defer txsim2.Done()
				txsim = txsim2
			}
			ctxt = context.WithValue(ctxt, TXSimulatorKey, txsim)
			ctxt = context.WithValue(ctxt, HistoryQueryExecutorKey, historyQueryExecutor)

			chaincodeLogger.Debugf("[%s] getting chaincode data for %s on channel %s",
				shorttxid(msg.Txid), calledCcIns.ChaincodeName, calledCcIns.ChainID)

			//is the chaincode a system chaincode ?
			isscc := sysccprovider.GetSystemChaincodeProvider().IsSysCC(calledCcIns.ChaincodeName)

			var version string
			if !isscc {
				//if its a user chaincode, get the details
				cd, err := GetChaincodeDefinition(ctxt, msg.Txid, txContext.signedProp, txContext.proposal, calledCcIns.ChainID, calledCcIns.ChaincodeName)
				if err != nil {
					errHandler([]byte(err.Error()), "[%s]Failed to get chaincode data (%s) for invoked chaincode. Sending %s", shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
					return
				}

				version = cd.CCVersion()

				err = ccprovider.CheckInstantiationPolicy(calledCcIns.ChaincodeName, version, cd.(*ccprovider.ChaincodeData))
				if err != nil {
					errHandler([]byte(err.Error()), "[%s]CheckInstantiationPolicy, error %s. Sending %s", shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
					return
				}
			} else {
				//this is a system cc, just call it directly
				version = util.GetSysCCVersion()
			}

			cccid := ccprovider.NewCCContext(calledCcIns.ChainID, calledCcIns.ChaincodeName, version, msg.Txid, false, txContext.signedProp, txContext.proposal)

			// Launch the new chaincode if not already running
			chaincodeLogger.Debugf("[%s] launching chaincode %s on channel %s",
				shorttxid(msg.Txid), calledCcIns.ChaincodeName, calledCcIns.ChainID)
			cciSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: chaincodeSpec}
			_, chaincodeInput, launchErr := handler.chaincodeSupport.Launch(ctxt, cccid, cciSpec)
			if launchErr != nil {
				payload := []byte(launchErr.Error())
				chaincodeLogger.Debugf("[%s]Failed to launch invoked chaincode. Sending %s",
					shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				return
			}

			// TODO: Need to handle timeout correctly
			timeout := time.Duration(30000) * time.Millisecond

			ccMsg, _ := createCCMessage(pb.ChaincodeMessage_TRANSACTION, calledCcIns.ChainID, msg.Txid, chaincodeInput)

			// Execute the chaincode... this CANNOT be an init at least for now
			response, execErr := handler.chaincodeSupport.Execute(ctxt, cccid, ccMsg, timeout)

			//payload is marshalled and send to the calling chaincode's shim which unmarshals and
			//sends it to chaincode
			res = nil
			if execErr != nil {
				err = execErr
			} else {
				res, err = proto.Marshal(response)
			}
		}

		if err != nil {
			// Send error msg back to chaincode and trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("[%s]Failed to handle %s. Sending %s", shorttxid(msg.Txid), msg.Type, pb.ChaincodeMessage_ERROR)
			triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		// Send response msg back to chaincode.
		chaincodeLogger.Debugf("[%s]Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type, pb.ChaincodeMessage_RESPONSE)
		triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}
	}()
}

func (handler *Handler) setChaincodeProposal(signedProp *pb.SignedProposal, prop *pb.Proposal, msg *pb.ChaincodeMessage) error {
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

// handleMessage is the entrance method for Peer's handling of Chaincode messages.
func (handler *Handler) handleMessage(msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("[%s]Fabric side Handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.Txid), msg.Type, handler.state)

	var hFn func(*pb.ChaincodeMessage)
	switch handler.state {
	case created:
		//chaincode connects and puts into established
		hFn = handler.createStateHandlers[msg.Type]
	case ready:
		//chaincode state requests handled in ready
		hFn = handler.readyStateHandlers[msg.Type]
	default:
		return fmt.Errorf("[%s]handleMessage-invalid state %s", msg.Txid, handler.state)
	}

	if hFn == nil {
		return fmt.Errorf("[%s]Fabric side handler cannot handle message (%s) while in state: %s", msg.Txid, msg.Type, handler.state)
	}

	hFn(msg)

	return nil
}

func (handler *Handler) sendExecuteMessage(ctxt context.Context, chainID string, msg *pb.ChaincodeMessage, signedProp *pb.SignedProposal, prop *pb.Proposal) (chan *pb.ChaincodeMessage, error) {
	txctx, err := handler.createTxContext(ctxt, chainID, msg.Txid, signedProp, prop)
	if err != nil {
		return nil, err
	}
	chaincodeLogger.Debugf("[%s]Inside sendExecuteMessage. Message %s", shorttxid(msg.Txid), msg.Type)

	//if security is disabled the context elements will just be nil
	if err = handler.setChaincodeProposal(signedProp, prop, msg); err != nil {
		return nil, err
	}

	chaincodeLogger.Debugf("[%s]sendExecuteMsg trigger event %s", shorttxid(msg.Txid), msg.Type)
	handler.serialSendAsync(msg, true)

	return txctx.responseNotifier, nil
}
