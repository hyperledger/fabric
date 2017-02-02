/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package chaincode

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	ccintf "github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/looplab/fsm"
	logging "github.com/op/go-logging"
	"golang.org/x/net/context"
)

const (
	createdstate     = "created"     //start state
	establishedstate = "established" //in: CREATED, rcv:  REGISTER, send: REGISTERED, INIT
	initstate        = "init"        //in:ESTABLISHED, rcv:-, send: INIT
	readystate       = "ready"       //in:ESTABLISHED,TRANSACTION, rcv:COMPLETED
	endstate         = "end"         //in:INIT,ESTABLISHED, rcv: error, terminate container

)

var chaincodeLogger = logging.MustGetLogger("chaincode")

// MessageHandler interface for handling chaincode messages (common between Peer chaincode support and chaincode)
type MessageHandler interface {
	HandleMessage(msg *pb.ChaincodeMessage) error
	SendMessage(msg *pb.ChaincodeMessage) error
}

type transactionContext struct {
	chainID          string
	proposal         *pb.Proposal
	responseNotifier chan *pb.ChaincodeMessage

	// tracks open iterators used for range queries
	queryIteratorMap map[string]commonledger.ResultsIterator

	txsimulator ledger.TxSimulator
}

type nextStateInfo struct {
	msg      *pb.ChaincodeMessage
	sendToCC bool
}

//chaincode registered name is of the form
//    <name>:<version>/<suffix>
type ccParts struct {
	name    string //the main name of the chaincode
	version string //the version param if any (used for upgrade)
	suffix  string //for now just the chain name
}

// Handler responsbile for management of Peer's side of chaincode stream
type Handler struct {
	sync.RWMutex
	//peer to shim grpc serializer. User only in serialSend
	serialLock  sync.Mutex
	ChatStream  ccintf.ChaincodeStream
	FSM         *fsm.FSM
	ChaincodeID *pb.ChaincodeID
	ccCompParts *ccParts

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

//gets component parts from the canonical name of the chaincode.
//Called exactly once per chaincode when registering chaincode.
//This is needed for the "one-instance-per-chain" model when
//starting up the chaincode for each chain. It will still
//work for the "one-instance-for-all-chains" as the version
//and suffix will just be absent (also note that LCCC reserves
//"/:[]${}" as special chars mainly for such namespace uses)
func (handler *Handler) decomposeRegisteredName(cid *pb.ChaincodeID) {
	handler.ccCompParts = chaincodeIDParts(cid.Name)
}

func chaincodeIDParts(ccName string) *ccParts {
	b := []byte(ccName)
	p := &ccParts{}

	//compute suffix (ie, chain name)
	i := bytes.IndexByte(b, '/')
	if i >= 0 {
		if i < len(b)-1 {
			p.suffix = string(b[i+1:])
		}
		b = b[:i]
	}

	//compute version
	i = bytes.IndexByte(b, ':')
	if i >= 0 {
		if i < len(b)-1 {
			p.version = string(b[i+1:])
		}
		b = b[:i]
	}
	// remaining is the chaincode name
	p.name = string(b)

	return p
}

func (handler *Handler) getCCRootName() string {
	return handler.ccCompParts.name
}

//serialSend serializes msgs so gRPC will be happy
func (handler *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	handler.serialLock.Lock()
	defer handler.serialLock.Unlock()

	var err error
	if err = handler.ChatStream.Send(msg); err != nil {
		err = fmt.Errorf("[%s]Error sending %s: %s", shorttxid(msg.Txid), msg.Type.String(), err)
		chaincodeLogger.Errorf("%s", err)
	}
	return err
}

//serialSendAsync serves the same purpose as serialSend (serializ msgs so gRPC will
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

func (handler *Handler) createTxContext(ctxt context.Context, chainID string, txid string, prop *pb.Proposal) (*transactionContext, error) {
	if handler.txCtxs == nil {
		return nil, fmt.Errorf("cannot create notifier for txid:%s", txid)
	}
	handler.Lock()
	defer handler.Unlock()
	if handler.txCtxs[txid] != nil {
		return nil, fmt.Errorf("txid:%s exists", txid)
	}
	txctx := &transactionContext{chainID: chainID, proposal: prop, responseNotifier: make(chan *pb.ChaincodeMessage, 1),
		queryIteratorMap: make(map[string]commonledger.ResultsIterator)}
	handler.txCtxs[txid] = txctx
	txctx.txsimulator = getTxSimulator(ctxt)

	return txctx, nil
}

func (handler *Handler) getTxContext(txid string) *transactionContext {
	handler.Lock()
	defer handler.Unlock()
	return handler.txCtxs[txid]
}

func (handler *Handler) deleteTxContext(txid string) {
	handler.Lock()
	defer handler.Unlock()
	if handler.txCtxs != nil {
		delete(handler.txCtxs, txid)
	}
}

func (handler *Handler) putQueryIterator(txContext *transactionContext, txid string,
	queryIterator commonledger.ResultsIterator) {
	handler.Lock()
	defer handler.Unlock()
	txContext.queryIteratorMap[txid] = queryIterator
}

func (handler *Handler) getQueryIterator(txContext *transactionContext, txid string) commonledger.ResultsIterator {
	handler.Lock()
	defer handler.Unlock()
	return txContext.queryIteratorMap[txid]
}

func (handler *Handler) deleteQueryIterator(txContext *transactionContext, txid string) {
	handler.Lock()
	defer handler.Unlock()
	delete(txContext.queryIteratorMap, txid)
}

// Check if the transactor is allow to call this chaincode on this channel
func (handler *Handler) checkACL(proposal *pb.Proposal, calledCC *ccParts) *pb.ChaincodeMessage {
	// TODO: Decide what to pass in to verify that this transactor can access this
	// channel (chID) and chaincode (ccID). Very likely we need the signedProposal
	// which contains the sig and creator cert

	// If error, return ChaincodeMessage with type ChaincodeMessage_ERROR
	return nil
}

//THIS CAN BE REMOVED ONCE WE FULL SUPPORT (Invoke) CONFIDENTIALITY WITH CC-CALLING-CC
//Only invocation are allowed
func (handler *Handler) canCallChaincode(txid string, isQuery bool) *pb.ChaincodeMessage {
	var errMsg string
	txctx := handler.getTxContext(txid)
	if txctx == nil {
		errMsg = fmt.Sprintf("[%s]Error no context while checking for confidentiality. Sending %s", shorttxid(txid), pb.ChaincodeMessage_ERROR)
	}

	if errMsg != "" {
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(errMsg), Txid: txid}
	}

	//not CONFIDENTIAL transaction, OK to call CC
	return nil
}

func (handler *Handler) deregister() error {
	if handler.registered {
		handler.chaincodeSupport.deregisterHandler(handler)
	}
	return nil
}

func (handler *Handler) triggerNextState(msg *pb.ChaincodeMessage, send bool) {
	handler.nextState <- &nextStateInfo{msg, send}
}

func (handler *Handler) waitForKeepaliveTimer() <-chan time.Time {
	if handler.chaincodeSupport.keepalive > 0 {
		c := time.After(handler.chaincodeSupport.keepalive)
		return c
	}
	//no one will signal this channel, listner blocks forever
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
				chaincodeLogger.Debugf("Received EOF, ending chaincode support stream, %s", err)
				return err
			} else if err != nil {
				chaincodeLogger.Errorf("Error handling chaincode support stream: %s", err)
				return err
			} else if in == nil {
				err = fmt.Errorf("Received nil message, ending chaincode support stream")
				chaincodeLogger.Debug("Received nil message, ending chaincode support stream")
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
				err = fmt.Errorf("Next state nil message, ending chaincode support stream")
				chaincodeLogger.Debug("Next state nil message, ending chaincode support stream")
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

		err = handler.HandleMessage(in)
		if err != nil {
			chaincodeLogger.Errorf("[%s]Error handling message, ending stream: %s", shorttxid(in.Txid), err)
			return fmt.Errorf("Error handling message, ending stream: %s", err)
		}

		if nsInfo != nil && nsInfo.sendToCC {
			chaincodeLogger.Debugf("[%s]sending state message %s", shorttxid(in.Txid), in.Type.String())
			//if error bail in select
			handler.serialSendAsync(in, errc)
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
			{Name: pb.ChaincodeMessage_INIT.String(), Src: []string{establishedstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_READY.String(), Src: []string{establishedstate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_PUT_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_PUT_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_DEL_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_DEL_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_COMPLETED.String(), Src: []string{initstate, readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_EXECUTE_QUERY_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_EXECUTE_QUERY_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_QUERY_STATE_NEXT.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_QUERY_STATE_NEXT.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_QUERY_STATE_CLOSE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_QUERY_STATE_CLOSE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{initstate}, Dst: endstate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_TRANSACTION.String(), Src: []string{readystate}, Dst: readystate},
		},
		fsm.Callbacks{
			"before_" + pb.ChaincodeMessage_REGISTER.String():           func(e *fsm.Event) { v.beforeRegisterEvent(e, v.FSM.Current()) },
			"before_" + pb.ChaincodeMessage_COMPLETED.String():          func(e *fsm.Event) { v.beforeCompletedEvent(e, v.FSM.Current()) },
			"before_" + pb.ChaincodeMessage_INIT.String():               func(e *fsm.Event) { v.beforeInitState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_GET_STATE.String():           func(e *fsm.Event) { v.afterGetState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_RANGE_QUERY_STATE.String():   func(e *fsm.Event) { v.afterRangeQueryState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_EXECUTE_QUERY_STATE.String(): func(e *fsm.Event) { v.afterExecuteQueryState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_QUERY_STATE_NEXT.String():    func(e *fsm.Event) { v.afterQueryStateNext(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_QUERY_STATE_CLOSE.String():   func(e *fsm.Event) { v.afterQueryStateClose(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_PUT_STATE.String():           func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_DEL_STATE.String():           func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_INVOKE_CHAINCODE.String():    func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"enter_" + establishedstate:                                 func(e *fsm.Event) { v.enterEstablishedState(e, v.FSM.Current()) },
			"enter_" + initstate:                                        func(e *fsm.Event) { v.enterInitState(e, v.FSM.Current()) },
			"enter_" + readystate:                                       func(e *fsm.Event) { v.enterReadyState(e, v.FSM.Current()) },
			"enter_" + endstate:                                         func(e *fsm.Event) { v.enterEndState(e, v.FSM.Current()) },
		},
	)

	return v
}

func (handler *Handler) createTXIDEntry(txid string) bool {
	if handler.txidMap == nil {
		return false
	}
	handler.Lock()
	defer handler.Unlock()
	if handler.txidMap[txid] {
		return false
	}
	handler.txidMap[txid] = true
	return handler.txidMap[txid]
}

func (handler *Handler) deleteTXIDEntry(txid string) {
	handler.Lock()
	defer handler.Unlock()
	if handler.txidMap != nil {
		delete(handler.txidMap, txid)
	} else {
		chaincodeLogger.Warningf("TXID %s not found!", txid)
	}
}

func (handler *Handler) notifyDuringStartup(val bool) {
	//if USER_RUNS_CC readyNotify will be nil
	if handler.readyNotify != nil {
		chaincodeLogger.Debug("Notifying during startup")
		handler.readyNotify <- val
	} else {
		chaincodeLogger.Debug("nothing to notify (dev mode ?)")
	}
}

// beforeRegisterEvent is invoked when chaincode tries to register.
func (handler *Handler) beforeRegisterEvent(e *fsm.Event, state string) {
	chaincodeLogger.Debugf("Received %s in state %s", e.Event, state)
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeID := &pb.ChaincodeID{}
	err := proto.Unmarshal(msg.Payload, chaincodeID)
	if err != nil {
		e.Cancel(fmt.Errorf("Error in received %s, could NOT unmarshal registration info: %s", pb.ChaincodeMessage_REGISTER, err))
		return
	}

	// Now register with the chaincodeSupport
	handler.ChaincodeID = chaincodeID
	err = handler.chaincodeSupport.registerHandler(handler)
	if err != nil {
		e.Cancel(err)
		handler.notifyDuringStartup(false)
		return
	}

	//get the component parts so we can use the root chaincode
	//name in keys
	handler.decomposeRegisteredName(handler.ChaincodeID)

	chaincodeLogger.Debugf("Got %s for chaincodeID = %s, sending back %s", e.Event, chaincodeID, pb.ChaincodeMessage_REGISTERED)
	if err := handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_REGISTERED}); err != nil {
		e.Cancel(fmt.Errorf("Error sending %s: %s", pb.ChaincodeMessage_REGISTERED, err))
		handler.notifyDuringStartup(false)
		return
	}
}

func (handler *Handler) notify(msg *pb.ChaincodeMessage) {
	handler.Lock()
	defer handler.Unlock()
	tctx := handler.txCtxs[msg.Txid]
	if tctx == nil {
		chaincodeLogger.Debugf("notifier Txid:%s does not exist", msg.Txid)
	} else {
		chaincodeLogger.Debugf("notifying Txid:%s", msg.Txid)
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
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	// Notify on channel once into READY state
	chaincodeLogger.Debugf("[%s]beforeCompleted - not in ready state will notify when in readystate", shorttxid(msg.Txid))
	return
}

// beforeInitState is invoked before an init message is sent to the chaincode.
func (handler *Handler) beforeInitState(e *fsm.Event, state string) {
	chaincodeLogger.Debugf("Before state %s.. notifying waiter that we are up", state)
	handler.notifyDuringStartup(true)
}

// afterGetState handles a GET_STATE request from the chaincode.
func (handler *Handler) afterGetState(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Received %s, invoking get state from ledger", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE)

	// Query ledger for state
	handler.handleGetState(msg)
}

// is this a txid for which there is a valid txsim
func (handler *Handler) isValidTxSim(txid string, fmtStr string, args ...interface{}) (*transactionContext, *pb.ChaincodeMessage) {
	txContext := handler.getTxContext(txid)
	if txContext == nil || txContext.txsimulator == nil {
		// Send error msg back to chaincode. No ledger context
		errStr := fmt.Sprintf(fmtStr, args...)
		chaincodeLogger.Errorf(errStr)
		return nil, &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(errStr), Txid: txid}
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
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage
		var txContext *transactionContext
		txContext, serialSendMsg = handler.isValidTxSim(msg.Txid,
			"[%s]No ledger context for GetState. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s]handleGetState serial send %s",
					shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			}
			handler.serialSendAsync(serialSendMsg, nil)
		}()

		if txContext == nil {
			return
		}

		key := string(msg.Payload)
		chaincodeID := handler.getCCRootName()
		if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
			chaincodeLogger.Debugf("[%s] getting state for chaincode %s, key %s, channel %s",
				shorttxid(msg.Txid), chaincodeID, key, txContext.chainID)
		}

		var res []byte
		var err error
		res, err = txContext.txsimulator.GetState(chaincodeID, key)

		if err != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("[%s]Failed to get chaincode state(%s). Sending %s",
				shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
		} else if res == nil {
			//The state object being requested does not exist
			chaincodeLogger.Debugf("[%s]No state associated with key: %s. Sending %s with an empty payload",
				shorttxid(msg.Txid), key, pb.ChaincodeMessage_RESPONSE)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid}
		} else {
			// Send response msg back to chaincode. GetState will not trigger event
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s]Got state. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_RESPONSE)
			}
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid}
		}

	}()
}

const maxRangeQueryStateLimit = 100

// afterRangeQueryState handles a RANGE_QUERY_STATE request from the chaincode.
func (handler *Handler) afterRangeQueryState(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_RANGE_QUERY_STATE)

	// Query ledger for state
	handler.handleRangeQueryState(msg)
	chaincodeLogger.Debug("Exiting GET_STATE")
}

// Handles query to ledger to rage query state
func (handler *Handler) handleRangeQueryState(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterRangeQueryState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			chaincodeLogger.Debugf("[%s]handleRangeQueryState serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
		}()

		rangeQueryState := &pb.RangeQueryState{}
		unmarshalErr := proto.Unmarshal(msg.Payload, rangeQueryState)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall range query request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		iterID := util.GenerateUUID()

		var txContext *transactionContext

		txContext, serialSendMsg = handler.isValidTxSim(msg.Txid, "[%s]No ledger context for RangeQueryState. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
		if txContext == nil {
			return
		}
		chaincodeID := handler.getCCRootName()

		rangeIter, err := txContext.txsimulator.GetStateRangeScanIterator(chaincodeID, rangeQueryState.StartKey, rangeQueryState.EndKey)
		if err != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed to get ledger scan iterator. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		handler.putQueryIterator(txContext, iterID, rangeIter)

		var keysAndValues []*pb.QueryStateKeyValue
		var i = uint32(0)
		var qresult commonledger.QueryResult
		for ; i < maxRangeQueryStateLimit; i++ {
			qresult, err = rangeIter.Next()
			if err != nil {
				chaincodeLogger.Errorf("Failed to get query result from iterator. Sending %s", pb.ChaincodeMessage_ERROR)
				return
			}
			if qresult == nil {
				break
			}
			kv := qresult.(*ledger.KV)
			keyAndValue := pb.QueryStateKeyValue{Key: kv.Key, Value: kv.Value}
			keysAndValues = append(keysAndValues, &keyAndValue)
		}

		if qresult != nil {
			rangeIter.Close()
			handler.deleteQueryIterator(txContext, iterID)
		}

		payload := &pb.QueryStateResponse{KeysAndValues: keysAndValues, HasMore: qresult != nil, ID: iterID}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {
			rangeIter.Close()
			handler.deleteQueryIterator(txContext, iterID)

			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed marshall resopnse. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid}

	}()
}

// afterQueryStateNext handles a QUERY_STATE_NEXT request from the chaincode.
func (handler *Handler) afterQueryStateNext(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_RANGE_QUERY_STATE)

	// Query ledger for state
	handler.handleQueryStateNext(msg)
	chaincodeLogger.Debug("Exiting QUERY_STATE_NEXT")
}

// Handles query to ledger for query state next
func (handler *Handler) handleQueryStateNext(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterRangeQueryState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Debug("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			chaincodeLogger.Debugf("[%s]handleRangeQueryState serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
		}()

		queryStateNext := &pb.QueryStateNext{}
		unmarshalErr := proto.Unmarshal(msg.Payload, queryStateNext)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall state next query request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		txContext := handler.getTxContext(msg.Txid)
		queryIter := handler.getQueryIterator(txContext, queryStateNext.ID)

		if queryIter == nil {
			payload := []byte("query iterator not found")
			chaincodeLogger.Errorf("query iterator not found. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		var keysAndValues []*pb.QueryStateKeyValue
		var i = uint32(0)

		var qresult commonledger.QueryResult
		var err error
		for ; i < maxRangeQueryStateLimit; i++ {
			qresult, err = queryIter.Next()
			if err != nil {
				chaincodeLogger.Errorf("Failed to get query result from iterator. Sending %s", pb.ChaincodeMessage_ERROR)
				return
			}
			if qresult != nil {
				break
			}
			kv := qresult.(*ledger.KV)
			keyAndValue := pb.QueryStateKeyValue{Key: kv.Key, Value: kv.Value}
			keysAndValues = append(keysAndValues, &keyAndValue)
		}

		if qresult != nil {
			queryIter.Close()
			handler.deleteQueryIterator(txContext, queryStateNext.ID)
		}

		payload := &pb.QueryStateResponse{KeysAndValues: keysAndValues, HasMore: qresult != nil, ID: queryStateNext.ID}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {
			queryIter.Close()
			handler.deleteQueryIterator(txContext, queryStateNext.ID)

			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed marshall resopnse. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid}

	}()
}

// afterRangeQueryState handles a QUERY_STATE_CLOSE request from the chaincode.
func (handler *Handler) afterQueryStateClose(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_RANGE_QUERY_STATE)

	// Query ledger for state
	handler.handleQueryStateClose(msg)
	chaincodeLogger.Debug("Exiting QUERY_STATE_CLOSE")
}

// Handles the closing of a state iterator
func (handler *Handler) handleQueryStateClose(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterRangeQueryState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			chaincodeLogger.Debugf("[%s]handleRangeQueryState serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
		}()

		queryStateClose := &pb.QueryStateClose{}
		unmarshalErr := proto.Unmarshal(msg.Payload, queryStateClose)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall state query close request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		txContext := handler.getTxContext(msg.Txid)
		iter := handler.getQueryIterator(txContext, queryStateClose.ID)
		if iter != nil {
			iter.Close()
			handler.deleteQueryIterator(txContext, queryStateClose.ID)
		}

		payload := &pb.QueryStateResponse{HasMore: false, ID: queryStateClose.ID}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {

			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed marshall resopnse. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		chaincodeLogger.Debugf("Closed. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid}

	}()
}

const maxExecuteQueryStateLimit = 100

// afterExecuteQueryState handles a EXECUTE_QUERY_STATE request from the chaincode.
func (handler *Handler) afterExecuteQueryState(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_EXECUTE_QUERY_STATE)

	// Query ledger for state
	handler.handleExecuteQueryState(msg)
	chaincodeLogger.Debug("Exiting EXECUTE_QUERY_STATE")
}

// Handles query to ledger to execute query state
func (handler *Handler) handleExecuteQueryState(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the afterQueryState function is exited. Interesting bug fix!!
	go func() {
		// Check if this is the unique state request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Error("Another state request pending for this Txid. Cannot process.")
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			chaincodeLogger.Debugf("[%s]handleExecuteQueryState serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSendAsync(serialSendMsg, nil)
		}()

		executeQueryState := &pb.ExecuteQueryState{}
		unmarshalErr := proto.Unmarshal(msg.Payload, executeQueryState)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall query request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		iterID := util.GenerateUUID()

		var txContext *transactionContext

		txContext, serialSendMsg = handler.isValidTxSim(msg.Txid, "[%s]No ledger context for ExecuteQueryState. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
		if txContext == nil {
			return
		}

		executeIter, err := txContext.txsimulator.ExecuteQuery(executeQueryState.Query)
		if err != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed to get ledger scan iterator. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		handler.putQueryIterator(txContext, iterID, executeIter)

		var keysAndValues []*pb.QueryStateKeyValue
		var i = uint32(0)
		var qresult commonledger.QueryResult
		for ; i < maxExecuteQueryStateLimit; i++ {
			qresult, err = executeIter.Next()
			if err != nil {
				chaincodeLogger.Errorf("Failed to get query result from iterator. Sending %s", pb.ChaincodeMessage_ERROR)
				return
			}
			if qresult == nil {
				break
			}
			queryRecord := qresult.(*ledger.QueryRecord)
			keyAndValue := pb.QueryStateKeyValue{Key: queryRecord.Key, Value: queryRecord.Record}
			keysAndValues = append(keysAndValues, &keyAndValue)
		}

		if qresult != nil {
			executeIter.Close()
			handler.deleteQueryIterator(txContext, iterID)
		}

		var payloadBytes []byte
		payload := &pb.QueryStateResponse{KeysAndValues: keysAndValues, HasMore: qresult != nil, ID: iterID}
		payloadBytes, err = proto.Marshal(payload)
		if err != nil {
			executeIter.Close()
			handler.deleteQueryIterator(txContext, iterID)

			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed marshall resopnse. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		chaincodeLogger.Debugf("Got keys and values. Sending %s", pb.ChaincodeMessage_RESPONSE)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: payloadBytes, Txid: msg.Txid}

	}()
}

// afterPutState handles a PUT_STATE request from the chaincode.
func (handler *Handler) afterPutState(e *fsm.Event, state string) {
	_, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s in state %s, invoking put state to ledger", pb.ChaincodeMessage_PUT_STATE, state)

	// Put state into ledger handled within enterBusyState
}

// afterDelState handles a DEL_STATE request from the chaincode.
func (handler *Handler) afterDelState(e *fsm.Event, state string) {
	_, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking delete state from ledger", pb.ChaincodeMessage_DEL_STATE)

	// Delete state from ledger handled within enterBusyState
}

// afterInvokeChaincode handles an INVOKE_CHAINCODE request from the chaincode.
func (handler *Handler) afterInvokeChaincode(e *fsm.Event, state string) {
	_, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s in state %s, invoking another chaincode", pb.ChaincodeMessage_INVOKE_CHAINCODE, state)

	// Invoke another chaincode handled within enterBusyState
}

// Handles request to ledger to put state
func (handler *Handler) enterBusyState(e *fsm.Event, state string) {
	go func() {
		msg, _ := e.Args[0].(*pb.ChaincodeMessage)
		if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
			chaincodeLogger.Debugf("[%s]state is %s", shorttxid(msg.Txid), state)
		}
		// Check if this is the unique request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Debug("Another request pending for this Txid. Cannot process.")
			return
		}

		var triggerNextStateMsg *pb.ChaincodeMessage
		var txContext *transactionContext
		txContext, triggerNextStateMsg = handler.isValidTxSim(msg.Txid, "[%s]No ledger context for %s. Sending %s",
			shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s]enterBusyState trigger event %s",
					shorttxid(triggerNextStateMsg.Txid), triggerNextStateMsg.Type)
			}
			handler.triggerNextState(triggerNextStateMsg, true)
		}()

		if txContext == nil {
			return
		}

		chaincodeID := handler.getCCRootName()
		var err error
		var res []byte

		if msg.Type.String() == pb.ChaincodeMessage_PUT_STATE.String() {
			putStateInfo := &pb.PutStateInfo{}
			unmarshalErr := proto.Unmarshal(msg.Payload, putStateInfo)
			if unmarshalErr != nil {
				payload := []byte(unmarshalErr.Error())
				chaincodeLogger.Debugf("[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				return
			}

			err = txContext.txsimulator.SetState(chaincodeID, putStateInfo.Key, putStateInfo.Value)
		} else if msg.Type.String() == pb.ChaincodeMessage_DEL_STATE.String() {
			// Invoke ledger to delete state
			key := string(msg.Payload)
			err = txContext.txsimulator.DeleteState(chaincodeID, key)
		} else if msg.Type.String() == pb.ChaincodeMessage_INVOKE_CHAINCODE.String() {
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s] C-call-C", shorttxid(msg.Txid))
			}
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
			calledCcParts := chaincodeIDParts(chaincodeSpec.ChaincodeID.Name)
			chaincodeSpec.ChaincodeID.Name = calledCcParts.name
			if calledCcParts.suffix == "" {
				// use caller's channel as the called chaincode is in the same channel
				calledCcParts.suffix = txContext.chainID
			}
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s] C-call-C %s on channel %s",
					shorttxid(msg.Txid), calledCcParts.name, calledCcParts.suffix)
			}

			triggerNextStateMsg = handler.checkACL(txContext.proposal, calledCcParts)
			if triggerNextStateMsg != nil {
				return
			}

			// Set up a new context for the called chaincode if on a different channel
			// We grab the called channel's ledger simulator to hold the new state
			ctxt := context.Background()
			txsim := txContext.txsimulator
			if calledCcParts.suffix != txContext.chainID {
				lgr := peer.GetLedger(calledCcParts.suffix)
				if lgr == nil {
					payload := "Failed to find ledger for called channel " + calledCcParts.suffix
					triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR,
						Payload: []byte(payload), Txid: msg.Txid}
					return
				}
				txsim2, err2 := lgr.NewTxSimulator()
				if err2 != nil {
					triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR,
						Payload: []byte(err2.Error()), Txid: msg.Txid}
					return
				}
				defer txsim2.Done()
				txsim = txsim2
			}
			ctxt = context.WithValue(ctxt, TXSimulatorKey, txsim)

			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s] calling lccc to get chaincode data for %s on channel %s",
					shorttxid(msg.Txid), calledCcParts.name, calledCcParts.suffix)
			}

			//Call LCCC to get the called chaincode artifacts
			var cd *ccprovider.ChaincodeData
			cd, err = GetChaincodeDataFromLCCC(ctxt, msg.Txid, txContext.proposal, calledCcParts.suffix, calledCcParts.name)
			if err != nil {
				payload := []byte(err.Error())
				chaincodeLogger.Debugf("[%s]Failed to get chaincoed data (%s) for invoked chaincode. Sending %s",
					shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				return
			}
			cccid := ccprovider.NewCCContext(calledCcParts.suffix, calledCcParts.name, cd.Version, msg.Txid, false, txContext.proposal)

			// Launch the new chaincode if not already running
			if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
				chaincodeLogger.Debugf("[%s] launching chaincode %s on channel %s",
					shorttxid(msg.Txid), calledCcParts.name, calledCcParts.suffix)
			}
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

			ccMsg, _ := createTransactionMessage(msg.Txid, chaincodeInput)

			// Execute the chaincode
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
			chaincodeLogger.Errorf("[%s]Failed to handle %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)
			triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		// Send response msg back to chaincode.
		if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
			chaincodeLogger.Debugf("[%s]Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_RESPONSE)
		}
		triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid}
	}()
}

func (handler *Handler) enterEstablishedState(e *fsm.Event, state string) {
	handler.notifyDuringStartup(true)
}

func (handler *Handler) enterInitState(e *fsm.Event, state string) {
	ccMsg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Entered state %s", shorttxid(ccMsg.Txid), state)
	//very first time entering init state from established, send message to chaincode
	if ccMsg.Type == pb.ChaincodeMessage_INIT {
		if err := handler.serialSend(ccMsg); err != nil {
			errMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(fmt.Sprintf("Error sending %s: %s", pb.ChaincodeMessage_INIT, err)), Txid: ccMsg.Txid}
			handler.notify(errMsg)
		}
	}
}

func (handler *Handler) enterReadyState(e *fsm.Event, state string) {
	// Now notify
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
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
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Entered state %s", shorttxid(msg.Txid), state)
	handler.notify(msg)
	e.Cancel(fmt.Errorf("Entered end state"))
}

func (handler *Handler) setChaincodeProposal(prop *pb.Proposal, msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debug("Setting chaincode proposal context...")
	if prop != nil {
		chaincodeLogger.Debug("Proposal different from nil. Creating chaincode proposal context...")

		proposalContext, err := utils.GetChaincodeProposalContext(prop)
		if err != nil {
			chaincodeLogger.Debug("Failed getting proposal context from proposal [%s]", err)
			return fmt.Errorf("Failed getting proposal context from proposal [%s]", err)
		}

		msg.ProposalContext = proposalContext
	}
	return nil
}

//if initArgs is set (should be for "deploy" only) move to Init
//else move to ready
func (handler *Handler) initOrReady(ctxt context.Context, chainID string, txid string, prop *pb.Proposal, initArgs [][]byte) (chan *pb.ChaincodeMessage, error) {
	var ccMsg *pb.ChaincodeMessage
	var send bool

	txctx, funcErr := handler.createTxContext(ctxt, chainID, txid, prop)
	if funcErr != nil {
		return nil, funcErr
	}

	notfy := txctx.responseNotifier

	if initArgs != nil {
		chaincodeLogger.Debug("sending INIT")
		funcArgsMsg := &pb.ChaincodeInput{Args: initArgs}
		var payload []byte
		if payload, funcErr = proto.Marshal(funcArgsMsg); funcErr != nil {
			handler.deleteTxContext(txid)
			return nil, fmt.Errorf("Failed to marshall %s : %s\n", ccMsg.Type.String(), funcErr)
		}
		ccMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INIT, Payload: payload, Txid: txid}
		send = false
	} else {
		chaincodeLogger.Debug("sending READY")
		ccMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_READY, Txid: txid}
		send = true
	}

	//if security is disabled the context elements will just be nil
	if err := handler.setChaincodeProposal(prop, ccMsg); err != nil {
		return nil, err
	}

	handler.triggerNextState(ccMsg, send)

	return notfy, nil
}

// HandleMessage implementation of MessageHandler interface.  Peer's handling of Chaincode messages.
func (handler *Handler) HandleMessage(msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("[%s]Handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.Txid), msg.Type, handler.FSM.Current())

	if (msg.Type == pb.ChaincodeMessage_COMPLETED || msg.Type == pb.ChaincodeMessage_ERROR) && handler.FSM.Current() == "ready" {
		chaincodeLogger.Debugf("[%s]HandleMessage- COMPLETED. Notify", msg.Txid)
		handler.notify(msg)
		return nil
	}
	if handler.FSM.Cannot(msg.Type.String()) {
		// Other errors
		return fmt.Errorf("[%s]Chaincode handler validator FSM cannot handle message (%s) with payload size (%d) while in state: %s", msg.Txid, msg.Type.String(), len(msg.Payload), handler.FSM.Current())
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

func (handler *Handler) sendExecuteMessage(ctxt context.Context, chainID string, msg *pb.ChaincodeMessage, prop *pb.Proposal) (chan *pb.ChaincodeMessage, error) {
	txctx, err := handler.createTxContext(ctxt, chainID, msg.Txid, prop)
	if err != nil {
		return nil, err
	}
	if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
		chaincodeLogger.Debugf("[%s]Inside sendExecuteMessage. Message %s", shorttxid(msg.Txid), msg.Type.String())
	}

	//if security is disabled the context elements will just be nil
	if err = handler.setChaincodeProposal(prop, msg); err != nil {
		return nil, err
	}

	// Trigger FSM event if it is a transaction
	if msg.Type.String() == pb.ChaincodeMessage_TRANSACTION.String() {
		chaincodeLogger.Debugf("[%s]sendExecuteMsg trigger event %s", shorttxid(msg.Txid), msg.Type)
		handler.triggerNextState(msg, true)
	} else {
		// Send the message to shim
		chaincodeLogger.Debugf("[%s]sending query", shorttxid(msg.Txid))
		if err = handler.serialSend(msg); err != nil {
			handler.deleteTxContext(msg.Txid)
			return nil, fmt.Errorf("[%s]SendMessage error sending (%s)", shorttxid(msg.Txid), err)
		}
	}

	return txctx.responseNotifier, nil
}

func (handler *Handler) isRunning() bool {
	switch handler.FSM.Current() {
	case createdstate:
		fallthrough
	case establishedstate:
		fallthrough
	case initstate:
		return false
	default:
		return true
	}
}
