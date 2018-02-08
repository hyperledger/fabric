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
	"github.com/looplab/fsm"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

const (
	createdstate     = "created"     //start state
	establishedstate = "established" //in: CREATED, rcv:  REGISTER, send: REGISTERED, INIT
	readystate       = "ready"       //in:ESTABLISHED,TRANSACTION, rcv:COMPLETED
	endstate         = "end"         //in:INIT,ESTABLISHED, rcv: error, terminate container

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

type nextStateInfo struct {
	msg      *pb.ChaincodeMessage
	sendToCC bool

	//the only time we need to send synchronously is
	//when launching the chaincode to take it to ready
	//state (look for the panic when sending serial)
	sendSync bool
}

// Handler responsible for management of Peer's side of chaincode stream
type Handler struct {
	sync.RWMutex
	//peer to shim grpc serializer. User only in serialSend
	serialLock  sync.Mutex
	ChatStream  ccintf.ChaincodeStream
	FSM         *fsm.FSM
	ChaincodeID *pb.ChaincodeID
	ccInstance  *sysccprovider.ChaincodeInstance

	chaincodeSupport *ChaincodeSupport
	registered       bool
	readyNotify      chan bool
	// Map of tx txid to either invoke tx. Each tx will be
	// added prior to execute and remove when done execute
	txCtxs map[string]*transactionContext

	txidMap map[string]bool

	// used to do Send after making sure the state transition is complete
	nextState chan *nextStateInfo
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
		err = errors.WithMessage(err, fmt.Sprintf("[%s]Error sending %s", shorttxid(msg.Txid), msg.Type.String()))
		chaincodeLogger.Errorf("%+v", err)
	}
	return err
}

//serialSendAsync serves the same purpose as serialSend (serialize msgs so gRPC will
//be happy). In addition, it is also asynchronous so send-remoterecv--localrecv loop
//can be nonblocking. Only errors need to be handled and these are handled by
//communication on supplied error channel. A typical use will be a non-blocking or
//nil channel
func (handler *Handler) serialSendAsync(msg *pb.ChaincodeMessage, errc chan error) {
	go func() {
		err := handler.serialSend(msg)
		if errc != nil {
			errc <- err
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

func (handler *Handler) triggerNextState(msg *pb.ChaincodeMessage, send bool) {
	//this will send Async
	handler.nextState <- &nextStateInfo{msg: msg, sendToCC: send, sendSync: false}
}

func (handler *Handler) triggerNextStateSync(msg *pb.ChaincodeMessage) {
	//this will send sync
	handler.nextState <- &nextStateInfo{msg: msg, sendToCC: true, sendSync: true}
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
	msgAvail := make(chan *pb.ChaincodeMessage)
	var nsInfo *nextStateInfo
	var in *pb.ChaincodeMessage
	var err error

	//recv is used to spin Recv routine after previous received msg
	//has been processed
	recv := true

	//catch send errors and bail now that sends aren't synchronous
	errc := make(chan error, 1)
	for {
		in = nil
		err = nil
		nsInfo = nil
		if recv {
			recv = false
			go func() {
				var in2 *pb.ChaincodeMessage
				in2, err = handler.ChatStream.Recv()
				msgAvail <- in2
			}()
		}
		select {
		case sendErr := <-errc:
			if sendErr != nil {
				return sendErr
			}
			//send was successful, just continue
			continue
		case in = <-msgAvail:
			// Defer the deregistering of the this handler.
			if err == io.EOF {
				err = errors.Wrapf(err, "received EOF, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			} else if err != nil {
				chaincodeLogger.Errorf("Error handling chaincode support stream: %+v", err)
				return err
			} else if in == nil {
				err = errors.New("received nil message, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			}
			chaincodeLogger.Debugf("[%s]Received message %s from shim", shorttxid(in.Txid), in.Type.String())
			if in.Type.String() == pb.ChaincodeMessage_ERROR.String() {
				chaincodeLogger.Errorf("Got error: %s", string(in.Payload))
			}

			// we can spin off another Recv again
			recv = true

			if in.Type == pb.ChaincodeMessage_KEEPALIVE {
				chaincodeLogger.Debug("Received KEEPALIVE Response")
				// Received a keep alive message, we don't do anything with it for now
				// and it does not touch the state machine
				continue
			}
		case nsInfo = <-handler.nextState:
			in = nsInfo.msg
			if in == nil {
				err = errors.New("next state nil message, ending chaincode support stream")
				chaincodeLogger.Debugf("%+v", err)
				return err
			}
			chaincodeLogger.Debugf("[%s]Move state message %s", shorttxid(in.Txid), in.Type.String())
		case <-handler.waitForKeepaliveTimer():
			if handler.chaincodeSupport.keepalive <= 0 {
				chaincodeLogger.Errorf("Invalid select: keepalive not on (keepalive=%d)", handler.chaincodeSupport.keepalive)
				continue
			}

			//if no error message from serialSend, KEEPALIVE happy, and don't care about error
			//(maybe it'll work later)
			handler.serialSendAsync(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE}, nil)
			continue
		}

		err = handler.handleMessage(in)
		if err != nil {
			err = errors.WithMessage(err, "error handling message, ending stream")
			chaincodeLogger.Errorf("[%s] %+v", shorttxid(in.Txid), err)
			return err
		}

		if nsInfo != nil && nsInfo.sendToCC {
			chaincodeLogger.Debugf("[%s]sending state message %s", shorttxid(in.Txid), in.Type.String())
			//ready messages are sent sync
			if nsInfo.sendSync {
				if in.Type.String() != pb.ChaincodeMessage_READY.String() {
					panic(fmt.Sprintf("[%s]Sync send can only be for READY state %s\n", shorttxid(in.Txid), in.Type.String()))
				}
				if err = handler.serialSend(in); err != nil {
					return errors.WithMessage(err, fmt.Sprintf("[%s]error sending ready  message, ending stream:", shorttxid(in.Txid)))
				}
			} else {
				//if error bail in select
				handler.serialSendAsync(in, errc)
			}
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
		ChatStream: peerChatStream,
	}
	v.chaincodeSupport = chaincodeSupport
	//we want this to block
	v.nextState = make(chan *nextStateInfo)

	v.FSM = fsm.NewFSM(
		createdstate,
		fsm.Events{
			//Send REGISTERED, then, if deploy { trigger INIT(via INIT) } else { trigger READY(via COMPLETED) }
			{Name: pb.ChaincodeMessage_REGISTER.String(), Src: []string{createdstate}, Dst: establishedstate},
			{Name: pb.ChaincodeMessage_READY.String(), Src: []string{establishedstate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_PUT_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_DEL_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_COMPLETED.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_STATE_BY_RANGE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_QUERY_RESULT.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_QUERY_STATE_NEXT.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_QUERY_STATE_CLOSE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_INIT.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_TRANSACTION.String(), Src: []string{readystate}, Dst: readystate},
		},
		fsm.Callbacks{
			"before_" + pb.ChaincodeMessage_REGISTER.String():           func(e *fsm.Event) { v.beforeRegisterEvent(e, v.FSM.Current()) },
			"before_" + pb.ChaincodeMessage_COMPLETED.String():          func(e *fsm.Event) { v.beforeCompletedEvent(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_GET_STATE.String():           func(e *fsm.Event) { v.afterGetState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_GET_STATE_BY_RANGE.String():  func(e *fsm.Event) { v.afterGetStateByRange(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_GET_QUERY_RESULT.String():    func(e *fsm.Event) { v.afterGetQueryResult(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_GET_HISTORY_FOR_KEY.String(): func(e *fsm.Event) { v.afterGetHistoryForKey(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_QUERY_STATE_NEXT.String():    func(e *fsm.Event) { v.afterQueryStateNext(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_QUERY_STATE_CLOSE.String():   func(e *fsm.Event) { v.afterQueryStateClose(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_PUT_STATE.String():           func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_DEL_STATE.String():           func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_INVOKE_CHAINCODE.String():    func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"enter_" + establishedstate:                                 func(e *fsm.Event) { v.enterEstablishedState(e, v.FSM.Current()) },
			"enter_" + readystate:                                       func(e *fsm.Event) { v.enterReadyState(e, v.FSM.Current()) },
			"enter_" + endstate:                                         func(e *fsm.Event) { v.enterEndState(e, v.FSM.Current()) },
		},
	)

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

func (handler *Handler) notifyDuringStartup(val bool) {
	//if USER_RUNS_CC readyNotify will be nil
	if handler.readyNotify != nil {
		chaincodeLogger.Debug("Notifying during startup")
		handler.readyNotify <- val
	} else {
		chaincodeLogger.Debug("nothing to notify (dev mode ?)")
		//In theory, we don't even need a devmode flag in the peer anymore
		//as the chaincode is brought up without any context (ledger context
		//in particular). What this means is we can have - in theory - a nondev
		//environment where we can attach a chaincode manually. This could be
		//useful .... but for now lets just be conservative and allow manual
		//chaincode only in dev mode (ie, peer started with --peer-chaincodedev=true)
		if handler.chaincodeSupport.userRunsCC {
			if val {
				chaincodeLogger.Debug("sending READY")
				ccMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY}
				go handler.triggerNextState(ccMsg, true)
			} else {
				chaincodeLogger.Errorf("Error during startup .. not sending READY")
			}
		} else {
			chaincodeLogger.Warningf("trying to manually run chaincode when not in devmode ?")
		}
	}
}

// beforeRegisterEvent is invoked when chaincode tries to register.
func (handler *Handler) beforeRegisterEvent(e *fsm.Event, state string) {
	chaincodeLogger.Debugf("Received %s in state %s", e.Event, state)
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeID := &pb.ChaincodeID{}
	err := proto.Unmarshal(msg.Payload, chaincodeID)
	if err != nil {
		e.Cancel(errors.Wrap(err, fmt.Sprintf("error in received %s, could NOT unmarshal registration info", pb.ChaincodeMessage_REGISTER)))
		return
	}

	// Now register with the chaincodeSupport
	handler.ChaincodeID = chaincodeID
	err = handler.chaincodeSupport.registerHandler(handler)
	if err != nil {
		e.Cancel(errors.New(err.Error()))
		handler.notifyDuringStartup(false)
		return
	}

	//get the component parts so we can use the root chaincode
	//name in keys
	handler.decomposeRegisteredName(handler.ChaincodeID)

	chaincodeLogger.Debugf("Got %s for chaincodeID = %s, sending back %s", e.Event, chaincodeID, pb.ChaincodeMessage_REGISTERED)
	if err := handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}); err != nil {
		e.Cancel(errors.WithMessage(err, fmt.Sprintf("error sending %s", pb.ChaincodeMessage_REGISTERED)))
		handler.notifyDuringStartup(false)
		return
	}
}

func (handler *Handler) notify(msg *pb.ChaincodeMessage) {
	handler.Lock()
	defer handler.Unlock()
	txCtxID := handler.getTxCtxId(msg.ChannelId, msg.Txid)
	tctx := handler.txCtxs[txCtxID]
	if tctx == nil {
		chaincodeLogger.Debugf("notifier Txid:%s, channelID:%s does not exist", msg.Txid, msg.ChannelId)
	} else {
		chaincodeLogger.Debugf("notifying Txid:%s, channelID:%s", msg.Txid, msg.ChannelId)
		tctx.responseNotifier <- msg

		// clean up queryIteratorMap
		for _, v := range tctx.queryIteratorMap {
			v.Close()
		}
	}
}

// beforeCompletedEvent is invoked when chaincode has completed execution of init, invoke.
func (handler *Handler) beforeCompletedEvent(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	// Notify on channel once into READY state
	chaincodeLogger.Debugf("[%s]beforeCompleted - not in ready state will notify when in readystate", shorttxid(msg.Txid))
	return
}

// afterGetState handles a GET_STATE request from the chaincode.
func (handler *Handler) afterGetState(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Received %s, invoking get state from ledger", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE)

	// Query ledger for state
	handler.handleGetState(msg)
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

// Handles query to ledger to get state
func (handler *Handler) handleGetState(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterGetState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage
		var txContext *transactionContext
		txContext, serialSendMsg = handler.isValidTxSim(msg.ChannelId, msg.Txid,
			"[%s]No ledger context for GetState. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]handleGetState serial send %s",
				shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
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

// afterGetStateByRange handles a GET_STATE_BY_RANGE request from the chaincode.
func (handler *Handler) afterGetStateByRange(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_GET_STATE_BY_RANGE)

	// Query ledger for state
	handler.handleGetStateByRange(msg)
	chaincodeLogger.Debug("Exiting GET_STATE_BY_RANGE")
}

// Handles query to ledger to rage query state
func (handler *Handler) handleGetStateByRange(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterGetStateByRange function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]handleGetStateByRange serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
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

// afterQueryStateNext handles a QUERY_STATE_NEXT request from the chaincode.
func (handler *Handler) afterQueryStateNext(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking query state next from ledger", pb.ChaincodeMessage_QUERY_STATE_NEXT)

	// Query ledger for state
	handler.handleQueryStateNext(msg)
	chaincodeLogger.Debug("Exiting QUERY_STATE_NEXT")
}

// Handles query to ledger for query state next
func (handler *Handler) handleQueryStateNext(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterGetStateByRange function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Debug("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]handleQueryStateNext serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
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

// afterQueryStateClose handles a QUERY_STATE_CLOSE request from the chaincode.
func (handler *Handler) afterQueryStateClose(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking query state close from ledger", pb.ChaincodeMessage_QUERY_STATE_CLOSE)

	// Query ledger for state
	handler.handleQueryStateClose(msg)
	chaincodeLogger.Debug("Exiting QUERY_STATE_CLOSE")
}

// Handles the closing of a state iterator
func (handler *Handler) handleQueryStateClose(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterGetStateByRange function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]handleQueryStateClose serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
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

// afterGetQueryResult handles a GET_QUERY_RESULT request from the chaincode.
func (handler *Handler) afterGetQueryResult(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_GET_QUERY_RESULT)

	// Query ledger for state
	handler.handleGetQueryResult(msg)
	chaincodeLogger.Debug("Exiting GET_QUERY_RESULT")
}

// Handles query to ledger to execute query state
func (handler *Handler) handleGetQueryResult(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterQueryState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]handleGetQueryResult serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
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

// afterGetHistoryForKey handles a GET_HISTORY_FOR_KEY request from the chaincode.
func (handler *Handler) afterGetHistoryForKey(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_GET_HISTORY_FOR_KEY)

	// Query ledger history db
	handler.handleGetHistoryForKey(msg)
	chaincodeLogger.Debug("Exiting GET_HISTORY_FOR_KEY")
}

// Handles query to ledger history db
func (handler *Handler) handleGetHistoryForKey(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterQueryState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]handleGetHistoryForKey serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
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

// Handles request to ledger to put state
func (handler *Handler) enterBusyState(e *fsm.Event, state string) {
	go func() {
		msg, _ := e.Args[0].(*pb.ChaincodeMessage)
		chaincodeLogger.Debugf("[%s]state is %s", shorttxid(msg.Txid), state)
		// Check if this is the unique request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.ChannelId, msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Debugf("Another request pending for this CC: %s, Txid: %s, ChannelID: %s. Cannot process.", handler.ChaincodeID.Name, msg.Txid, msg.ChannelId)
			return
		}

		var triggerNextStateMsg *pb.ChaincodeMessage
		var txContext *transactionContext
		txContext, triggerNextStateMsg = handler.getTxContextForMessage(msg.ChannelId, msg.Txid, msg.Type.String(), msg.Payload,
			"[%s]No ledger context for %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)

		defer func() {
			handler.deleteTXIDEntry(msg.ChannelId, msg.Txid)
			chaincodeLogger.Debugf("[%s]enterBusyState trigger event %s",
				shorttxid(triggerNextStateMsg.Txid), triggerNextStateMsg.Type)
			handler.triggerNextState(triggerNextStateMsg, true)
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
				errHandler([]byte(unmarshalErr.Error()), "[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
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
				errHandler([]byte(err.Error()), "[%s] C-call-C %s on channel %s failed check ACL [%v]: [%s]", shorttxid(msg.Txid), calledCcIns.ChaincodeName, calledCcIns.ChainID, txContext.signedProp, err)
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
				errHandler([]byte(launchErr.Error()), "[%s]Failed to launch invoked chaincode. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
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
			errHandler([]byte(err.Error()), "[%s]Failed to handle %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)
			return
		}

		// Send response msg back to chaincode.
		chaincodeLogger.Debugf("[%s]Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_RESPONSE)
		triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid, ChannelId: msg.ChannelId}
	}()
}

func (handler *Handler) enterEstablishedState(e *fsm.Event, state string) {
	handler.notifyDuringStartup(true)
}

func (handler *Handler) enterReadyState(e *fsm.Event, state string) {
	// Now notify
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Entered state %s", shorttxid(msg.Txid), state)
	handler.notify(msg)
}

func (handler *Handler) enterEndState(e *fsm.Event, state string) {
	defer handler.deregister()
	// Now notify
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(errors.New("received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Entered state %s", shorttxid(msg.Txid), state)
	handler.notify(msg)
	e.Cancel(errors.New("entered end state"))
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

//move to ready
func (handler *Handler) ready(ctxt context.Context, chainID string, txid string, signedProp *pb.SignedProposal, prop *pb.Proposal) (chan *pb.ChaincodeMessage, error) {
	txctx, funcErr := handler.createTxContext(ctxt, chainID, txid, signedProp, prop)
	if funcErr != nil {
		return nil, funcErr
	}

	chaincodeLogger.Debug("sending READY")
	ccMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY, Txid: txid, ChannelId: chainID}

	//if security is disabled the context elements will just be nil
	if err := handler.setChaincodeProposal(signedProp, prop, ccMsg); err != nil {
		return nil, err
	}

	//send the ready synchronously as the
	//ready message is during launch and needs
	//to happen before any init/invokes can sneak in
	handler.triggerNextStateSync(ccMsg)

	return txctx.responseNotifier, nil
}

// handleMessage is the entrance method for Peer's handling of Chaincode messages.
func (handler *Handler) handleMessage(msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("[%s]Fabric side Handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.Txid), msg.Type, handler.FSM.Current())

	if (msg.Type == pb.ChaincodeMessage_COMPLETED || msg.Type == pb.ChaincodeMessage_ERROR) && handler.FSM.Current() == "ready" {
		chaincodeLogger.Debugf("[%s]HandleMessage- COMPLETED. Notify", msg.Txid)
		handler.notify(msg)
		return nil
	}
	if handler.FSM.Cannot(msg.Type.String()) {
		// Other errors
		return errors.Errorf("[%s]Chaincode handler validator FSM cannot handle message (%s) with payload size (%d) while in state: %s", msg.Txid, msg.Type.String(), len(msg.Payload), handler.FSM.Current())
	}
	eventErr := handler.FSM.Event(msg.Type.String(), msg)
	filteredErr := filterError(eventErr)
	if filteredErr != nil {
		chaincodeLogger.Errorf("[%s]Failed to trigger FSM event %s: %s", msg.Txid, msg.Type.String(), filteredErr)
	}

	return filteredErr
}

// Filter the Errors to allow NoTransitionError and CanceledError to not propagate for cases where embedded Err == nil
func filterError(errFromFSMEvent error) error {
	if errFromFSMEvent != nil {
		if noTransitionErr, ok := errFromFSMEvent.(*fsm.NoTransitionError); ok {
			if noTransitionErr.Err != nil {
				// Squash the NoTransitionError
				return errFromFSMEvent
			}
			chaincodeLogger.Debugf("Ignoring NoTransitionError: %s", noTransitionErr)
		}
		if canceledErr, ok := errFromFSMEvent.(*fsm.CanceledError); ok {
			if canceledErr.Err != nil {
				// Squash the CanceledError
				return canceledErr
			}
			chaincodeLogger.Debugf("Ignoring CanceledError: %s", canceledErr)
		}
	}
	return nil
}

func (handler *Handler) sendExecuteMessage(ctxt context.Context, chainID string, msg *pb.ChaincodeMessage, signedProp *pb.SignedProposal, prop *pb.Proposal) (chan *pb.ChaincodeMessage, error) {
	txctx, err := handler.createTxContext(ctxt, chainID, msg.Txid, signedProp, prop)
	if err != nil {
		return nil, err
	}
	chaincodeLogger.Debugf("[%s]Inside sendExecuteMessage. Message %s", shorttxid(msg.Txid), msg.Type.String())

	//if security is disabled the context elements will just be nil
	if err = handler.setChaincodeProposal(signedProp, prop, msg); err != nil {
		return nil, err
	}

	chaincodeLogger.Debugf("[%s]sendExecuteMsg trigger event %s", shorttxid(msg.Txid), msg.Type)
	handler.triggerNextState(msg, true)

	return txctx.responseNotifier, nil
}

func (handler *Handler) isRunning() bool {
	switch handler.FSM.Current() {
	case createdstate:
		fallthrough
	case establishedstate:
		fallthrough
	default:
		return true
	}
}
