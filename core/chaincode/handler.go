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
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	ccintf "github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/looplab/fsm"
	"github.com/op/go-logging"
	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/ledger"
)

const (
	createdstate     = "created"     //start state
	establishedstate = "established" //in: CREATED, rcv:  REGISTER, send: REGISTERED, INIT
	initstate        = "init"        //in:ESTABLISHED, rcv:-, send: INIT
	readystate       = "ready"       //in:ESTABLISHED,TRANSACTION, rcv:COMPLETED
	transactionstate = "transaction" //in:READY, rcv: xact from consensus, send: TRANSACTION
	busyinitstate    = "busyinit"    //in:INIT, rcv: PUT_STATE, DEL_STATE, INVOKE_CHAINCODE
	busyxactstate    = "busyxact"    //in:TRANSACION, rcv: PUT_STATE, DEL_STATE, INVOKE_CHAINCODE
	endstate         = "end"         //in:INIT,ESTABLISHED, rcv: error, terminate container

)

var chaincodeLogger = logging.MustGetLogger("chaincode")

// MessageHandler interface for handling chaincode messages (common between Peer chaincode support and chaincode)
type MessageHandler interface {
	HandleMessage(msg *pb.ChaincodeMessage) error
	SendMessage(msg *pb.ChaincodeMessage) error
}

type transactionContext struct {
	transactionSecContext *pb.Transaction
	responseNotifier      chan *pb.ChaincodeMessage

	// tracks open iterators used for range queries
	rangeQueryIteratorMap map[string]statemgmt.RangeScanIterator
}

type nextStateInfo struct {
	msg      *pb.ChaincodeMessage
	sendToCC bool
}

// Handler responsbile for management of Peer's side of chaincode stream
type Handler struct {
	sync.RWMutex
	//peer to shim grpc serializer. User only in serialSend
	serialLock  sync.Mutex
	ChatStream  ccintf.ChaincodeStream
	FSM         *fsm.FSM
	ChaincodeID *pb.ChaincodeID

	// A copy of decrypted deploy tx this handler manages, no code
	deployTXSecContext *pb.Transaction

	chaincodeSupport *ChaincodeSupport
	registered       bool
	readyNotify      chan bool
	// Map of tx txid to either invoke or query tx (decrypted). Each tx will be
	// added prior to execute and remove when done execute
	txCtxs map[string]*transactionContext

	txidMap map[string]bool

	// Track which TXIDs are queries; Although the shim maintains this, it cannot be trusted.
	isTransaction map[string]bool

	// used to do Send after making sure the state transition is complete
	nextState chan *nextStateInfo
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

func (handler *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	handler.serialLock.Lock()
	defer handler.serialLock.Unlock()
	if err := handler.ChatStream.Send(msg); err != nil {
		chaincodeLogger.Errorf("Error sending %s: %s", msg.Type.String(), err)
		return fmt.Errorf("Error sending %s: %s", msg.Type.String(), err)
	}
	return nil
}

func (handler *Handler) createTxContext(txid string, tx *pb.Transaction) (*transactionContext, error) {
	if handler.txCtxs == nil {
		return nil, fmt.Errorf("cannot create notifier for txid:%s", txid)
	}
	handler.Lock()
	defer handler.Unlock()
	if handler.txCtxs[txid] != nil {
		return nil, fmt.Errorf("txid:%s exists", txid)
	}
	txctx := &transactionContext{transactionSecContext: tx, responseNotifier: make(chan *pb.ChaincodeMessage, 1),
		rangeQueryIteratorMap: make(map[string]statemgmt.RangeScanIterator)}
	handler.txCtxs[txid] = txctx
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

func (handler *Handler) putRangeQueryIterator(txContext *transactionContext, txid string,
	rangeScanIterator statemgmt.RangeScanIterator) {
	handler.Lock()
	defer handler.Unlock()
	txContext.rangeQueryIteratorMap[txid] = rangeScanIterator
}

func (handler *Handler) getRangeQueryIterator(txContext *transactionContext, txid string) statemgmt.RangeScanIterator {
	handler.Lock()
	defer handler.Unlock()
	return txContext.rangeQueryIteratorMap[txid]
}

func (handler *Handler) deleteRangeQueryIterator(txContext *transactionContext, txid string) {
	handler.Lock()
	defer handler.Unlock()
	delete(txContext.rangeQueryIteratorMap, txid)
}

//THIS CAN BE REMOVED ONCE WE SUPPORT CONFIDENTIALITY WITH CC-CALLING-CC
//we dissallow chaincode-chaincode interactions till confidentiality implications are understood
func (handler *Handler) canCallChaincode(txid string) *pb.ChaincodeMessage {
	secHelper := handler.chaincodeSupport.getSecHelper()
	if secHelper == nil {
		return nil
	}

	var errMsg string
	txctx := handler.getTxContext(txid)
	if txctx == nil {
		errMsg = fmt.Sprintf("[%s]Error no context while checking for confidentiality. Sending %s", shorttxid(txid), pb.ChaincodeMessage_ERROR)
	} else if txctx.transactionSecContext == nil {
		errMsg = fmt.Sprintf("[%s]Error transaction context is nil while checking for confidentiality. Sending %s", shorttxid(txid), pb.ChaincodeMessage_ERROR)
	} else if txctx.transactionSecContext.ConfidentialityLevel != pb.ConfidentialityLevel_PUBLIC {
		errMsg = fmt.Sprintf("[%s]Error chaincode-chaincode interactions not supported for with privacy enabled. Sending %s", shorttxid(txid), pb.ChaincodeMessage_ERROR)
	}

	if errMsg != "" {
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(errMsg), Txid: txid}
	}

	//not CONFIDENTIAL transaction, OK to call CC
	return nil
}

func (handler *Handler) encryptOrDecrypt(encrypt bool, txid string, payload []byte) ([]byte, error) {
	secHelper := handler.chaincodeSupport.getSecHelper()
	if secHelper == nil {
		return payload, nil
	}

	txctx := handler.getTxContext(txid)
	if txctx == nil {
		return nil, fmt.Errorf("[%s]No context for txid %s", shorttxid(txid), txid)
	}
	if txctx.transactionSecContext == nil {
		return nil, fmt.Errorf("[%s]transaction context is nil for txid %s", shorttxid(txid), txid)
	}
	// TODO: this must be removed
	if txctx.transactionSecContext.ConfidentialityLevel == pb.ConfidentialityLevel_PUBLIC {
		return payload, nil
	}

	var enc crypto.StateEncryptor
	var err error
	if txctx.transactionSecContext.Type == pb.Transaction_CHAINCODE_DEPLOY {
		if enc, err = secHelper.GetStateEncryptor(handler.deployTXSecContext, handler.deployTXSecContext); err != nil {
			return nil, fmt.Errorf("error getting crypto encryptor for deploy tx :%s", err)
		}
	} else if txctx.transactionSecContext.Type == pb.Transaction_CHAINCODE_INVOKE || txctx.transactionSecContext.Type == pb.Transaction_CHAINCODE_QUERY {
		if enc, err = secHelper.GetStateEncryptor(handler.deployTXSecContext, txctx.transactionSecContext); err != nil {
			return nil, fmt.Errorf("error getting crypto encryptor %s", err)
		}
	} else {
		return nil, fmt.Errorf("invalid transaction type %s", txctx.transactionSecContext.Type.String())
	}
	if enc == nil {
		return nil, fmt.Errorf("secure context returns nil encryptor for tx %s", txid)
	}
	if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
		chaincodeLogger.Debugf("[%s]Payload before encrypt/decrypt: %v", shorttxid(txid), payload)
	}
	if encrypt {
		payload, err = enc.Encrypt(payload)
	} else {
		payload, err = enc.Decrypt(payload)
	}
	if chaincodeLogger.IsEnabledFor(logging.DEBUG) {
		chaincodeLogger.Debugf("[%s]Payload after encrypt/decrypt: %v", shorttxid(txid), payload)
	}

	return payload, err
}

func (handler *Handler) decrypt(txid string, payload []byte) ([]byte, error) {
	return handler.encryptOrDecrypt(false, txid, payload)
}

func (handler *Handler) encrypt(txid string, payload []byte) ([]byte, error) {
	return handler.encryptOrDecrypt(true, txid, payload)
}

func (handler *Handler) getSecurityBinding(tx *pb.Transaction) ([]byte, error) {
	secHelper := handler.chaincodeSupport.getSecHelper()
	if secHelper == nil {
		return nil, nil
	}

	return secHelper.GetTransactionBinding(tx)
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

			//TODO we could use this to hook into container lifecycle (kill the chaincode if not in use, etc)
			kaerr := handler.serialSend(&pb.ChaincodeMessage{Type: pb.ChaincodeMessage_KEEPALIVE})
			if kaerr != nil {
				chaincodeLogger.Errorf("Error sending keepalive, err=%s", kaerr)
			} else {
				chaincodeLogger.Debug("Sent KEEPALIVE request")
			}
			//keepalive message kicked in. just continue
			continue
		}

		err = handler.HandleMessage(in)
		if err != nil {
			chaincodeLogger.Errorf("[%s]Error handling message, ending stream: %s", shorttxid(in.Txid), err)
			return fmt.Errorf("Error handling message, ending stream: %s", err)
		}

		if nsInfo != nil && nsInfo.sendToCC {
			chaincodeLogger.Debugf("[%s]sending state message %s", shorttxid(in.Txid), in.Type.String())
			if err = handler.serialSend(in); err != nil {
				chaincodeLogger.Debugf("[%s]serial sending received error %s", shorttxid(in.Txid), err)
				return fmt.Errorf("[%s]serial sending received error %s", shorttxid(in.Txid), err)
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
			{Name: pb.ChaincodeMessage_INIT.String(), Src: []string{establishedstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_READY.String(), Src: []string{establishedstate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_TRANSACTION.String(), Src: []string{readystate}, Dst: transactionstate},
			{Name: pb.ChaincodeMessage_PUT_STATE.String(), Src: []string{transactionstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_DEL_STATE.String(), Src: []string{transactionstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), Src: []string{transactionstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_PUT_STATE.String(), Src: []string{initstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_DEL_STATE.String(), Src: []string{initstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_INVOKE_CHAINCODE.String(), Src: []string{initstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_COMPLETED.String(), Src: []string{initstate, readystate, transactionstate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{busyinitstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{transactionstate}, Dst: transactionstate},
			{Name: pb.ChaincodeMessage_GET_STATE.String(), Src: []string{busyxactstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{busyinitstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{transactionstate}, Dst: transactionstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE.String(), Src: []string{busyxactstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT.String(), Src: []string{busyinitstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT.String(), Src: []string{transactionstate}, Dst: transactionstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT.String(), Src: []string{busyxactstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE.String(), Src: []string{readystate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE.String(), Src: []string{initstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE.String(), Src: []string{busyinitstate}, Dst: busyinitstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE.String(), Src: []string{transactionstate}, Dst: transactionstate},
			{Name: pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE.String(), Src: []string{busyxactstate}, Dst: busyxactstate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{initstate}, Dst: endstate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{transactionstate}, Dst: readystate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{busyinitstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{busyxactstate}, Dst: transactionstate},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{busyinitstate}, Dst: initstate},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{busyxactstate}, Dst: transactionstate},
		},
		fsm.Callbacks{
			"before_" + pb.ChaincodeMessage_REGISTER.String():               func(e *fsm.Event) { v.beforeRegisterEvent(e, v.FSM.Current()) },
			"before_" + pb.ChaincodeMessage_COMPLETED.String():              func(e *fsm.Event) { v.beforeCompletedEvent(e, v.FSM.Current()) },
			"before_" + pb.ChaincodeMessage_INIT.String():                   func(e *fsm.Event) { v.beforeInitState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_GET_STATE.String():               func(e *fsm.Event) { v.afterGetState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_RANGE_QUERY_STATE.String():       func(e *fsm.Event) { v.afterRangeQueryState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT.String():  func(e *fsm.Event) { v.afterRangeQueryStateNext(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE.String(): func(e *fsm.Event) { v.afterRangeQueryStateClose(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_PUT_STATE.String():               func(e *fsm.Event) { v.afterPutState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_DEL_STATE.String():               func(e *fsm.Event) { v.afterDelState(e, v.FSM.Current()) },
			"after_" + pb.ChaincodeMessage_INVOKE_CHAINCODE.String():        func(e *fsm.Event) { v.afterInvokeChaincode(e, v.FSM.Current()) },
			"enter_" + establishedstate:                                     func(e *fsm.Event) { v.enterEstablishedState(e, v.FSM.Current()) },
			"enter_" + initstate:                                            func(e *fsm.Event) { v.enterInitState(e, v.FSM.Current()) },
			"enter_" + readystate:                                           func(e *fsm.Event) { v.enterReadyState(e, v.FSM.Current()) },
			"enter_" + busyinitstate:                                        func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"enter_" + busyxactstate:                                        func(e *fsm.Event) { v.enterBusyState(e, v.FSM.Current()) },
			"enter_" + endstate:                                             func(e *fsm.Event) { v.enterEndState(e, v.FSM.Current()) },
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

// markIsTransaction marks a TXID as a transaction or a query; true = transaction, false = query
func (handler *Handler) markIsTransaction(txid string, isTrans bool) bool {
	handler.Lock()
	defer handler.Unlock()
	if handler.isTransaction == nil {
		return false
	}
	handler.isTransaction[txid] = isTrans
	return true
}

func (handler *Handler) getIsTransaction(txid string) bool {
	handler.Lock()
	defer handler.Unlock()
	if handler.isTransaction == nil {
		return false
	}
	return handler.isTransaction[txid]
}

func (handler *Handler) deleteIsTransaction(txid string) {
	handler.Lock()
	defer handler.Unlock()
	if handler.isTransaction != nil {
		delete(handler.isTransaction, txid)
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

		// clean up rangeQueryIteratorMap
		for _, v := range tctx.rangeQueryIteratorMap {
			v.Close()
		}
	}
}

// beforeCompletedEvent is invoked when chaincode has completed execution of init, invoke or query.
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

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			chaincodeLogger.Debugf("[%s]handleGetState serial send %s", shorttxid(serialSendMsg.Txid), serialSendMsg.Type)
			handler.serialSend(serialSendMsg)
		}()

		key := string(msg.Payload)
		ledgerObj, ledgerErr := ledger.GetLedger()
		if ledgerErr != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(ledgerErr.Error())
			chaincodeLogger.Errorf("Failed to get chaincode state(%s). Sending %s", ledgerErr, pb.ChaincodeMessage_ERROR)
			// Remove txid from current set
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		// Invoke ledger to get state
		chaincodeID := handler.ChaincodeID.Name

		readCommittedState := !handler.getIsTransaction(msg.Txid)
		res, err := ledgerObj.GetState(chaincodeID, key, readCommittedState)
		if err != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("[%s]Failed to get chaincode state(%s). Sending %s", shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
		} else if res == nil {
			//The state object being requested does not exist, so don't attempt to decrypt it
			chaincodeLogger.Debugf("[%s]No state associated with key: %s. Sending %s with an empty payload", shorttxid(msg.Txid), key, pb.ChaincodeMessage_RESPONSE)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid}
		} else {
			// Decrypt the data if the confidential is enabled
			if res, err = handler.decrypt(msg.Txid, res); err == nil {
				// Send response msg back to chaincode. GetState will not trigger event
				chaincodeLogger.Debugf("[%s]Got state. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_RESPONSE)
				serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: res, Txid: msg.Txid}
			} else {
				// Send err msg back to chaincode.
				chaincodeLogger.Errorf("[%s]Got error (%s) while decrypting. Sending %s", shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
				errBytes := []byte(err.Error())
				serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: errBytes, Txid: msg.Txid}
			}

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
			handler.serialSend(serialSendMsg)
		}()

		rangeQueryState := &pb.RangeQueryState{}
		unmarshalErr := proto.Unmarshal(msg.Payload, rangeQueryState)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall range query request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		hasNext := true

		ledger, ledgerErr := ledger.GetLedger()
		if ledgerErr != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(ledgerErr.Error())
			chaincodeLogger.Errorf("Failed to get ledger. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		chaincodeID := handler.ChaincodeID.Name

		readCommittedState := !handler.getIsTransaction(msg.Txid)
		rangeIter, err := ledger.GetStateRangeScanIterator(chaincodeID, rangeQueryState.StartKey, rangeQueryState.EndKey, readCommittedState)
		if err != nil {
			// Send error msg back to chaincode. GetState will not trigger event
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("Failed to get ledger scan iterator. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		iterID := util.GenerateUUID()
		txContext := handler.getTxContext(msg.Txid)
		handler.putRangeQueryIterator(txContext, iterID, rangeIter)

		hasNext = rangeIter.Next()

		var keysAndValues []*pb.RangeQueryStateKeyValue
		var i = uint32(0)
		for ; hasNext && i < maxRangeQueryStateLimit; i++ {
			key, value := rangeIter.GetKeyValue()
			// Decrypt the data if the confidential is enabled
			decryptedValue, decryptErr := handler.decrypt(msg.Txid, value)
			if decryptErr != nil {
				payload := []byte(decryptErr.Error())
				chaincodeLogger.Errorf("Failed decrypt value. Sending %s", pb.ChaincodeMessage_ERROR)
				serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}

				rangeIter.Close()
				handler.deleteRangeQueryIterator(txContext, iterID)

				return
			}
			keyAndValue := pb.RangeQueryStateKeyValue{Key: key, Value: decryptedValue}
			keysAndValues = append(keysAndValues, &keyAndValue)

			hasNext = rangeIter.Next()
		}

		if !hasNext {
			rangeIter.Close()
			handler.deleteRangeQueryIterator(txContext, iterID)
		}

		payload := &pb.RangeQueryStateResponse{KeysAndValues: keysAndValues, HasMore: hasNext, ID: iterID}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {
			rangeIter.Close()
			handler.deleteRangeQueryIterator(txContext, iterID)

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

// afterRangeQueryState handles a RANGE_QUERY_STATE_NEXT request from the chaincode.
func (handler *Handler) afterRangeQueryStateNext(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_RANGE_QUERY_STATE)

	// Query ledger for state
	handler.handleRangeQueryStateNext(msg)
	chaincodeLogger.Debug("Exiting RANGE_QUERY_STATE_NEXT")
}

// Handles query to ledger to rage query state next
func (handler *Handler) handleRangeQueryStateNext(msg *pb.ChaincodeMessage) {
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
			handler.serialSend(serialSendMsg)
		}()

		rangeQueryStateNext := &pb.RangeQueryStateNext{}
		unmarshalErr := proto.Unmarshal(msg.Payload, rangeQueryStateNext)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall state range next query request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		txContext := handler.getTxContext(msg.Txid)
		rangeIter := handler.getRangeQueryIterator(txContext, rangeQueryStateNext.ID)

		if rangeIter == nil {
			payload := []byte("Range query iterator not found")
			chaincodeLogger.Errorf("Range query iterator not found. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		var keysAndValues []*pb.RangeQueryStateKeyValue
		var i = uint32(0)
		hasNext := true
		for ; hasNext && i < maxRangeQueryStateLimit; i++ {
			key, value := rangeIter.GetKeyValue()
			// Decrypt the data if the confidential is enabled
			decryptedValue, decryptErr := handler.decrypt(msg.Txid, value)
			if decryptErr != nil {
				payload := []byte(decryptErr.Error())
				chaincodeLogger.Errorf("Failed decrypt value. Sending %s", pb.ChaincodeMessage_ERROR)
				serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}

				rangeIter.Close()
				handler.deleteRangeQueryIterator(txContext, rangeQueryStateNext.ID)

				return
			}
			keyAndValue := pb.RangeQueryStateKeyValue{Key: key, Value: decryptedValue}
			keysAndValues = append(keysAndValues, &keyAndValue)

			hasNext = rangeIter.Next()
		}

		if !hasNext {
			rangeIter.Close()
			handler.deleteRangeQueryIterator(txContext, rangeQueryStateNext.ID)
		}

		payload := &pb.RangeQueryStateResponse{KeysAndValues: keysAndValues, HasMore: hasNext, ID: rangeQueryStateNext.ID}
		payloadBytes, err := proto.Marshal(payload)
		if err != nil {
			rangeIter.Close()
			handler.deleteRangeQueryIterator(txContext, rangeQueryStateNext.ID)

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

// afterRangeQueryState handles a RANGE_QUERY_STATE_CLOSE request from the chaincode.
func (handler *Handler) afterRangeQueryStateClose(e *fsm.Event, state string) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, invoking get state from ledger", pb.ChaincodeMessage_RANGE_QUERY_STATE)

	// Query ledger for state
	handler.handleRangeQueryStateClose(msg)
	chaincodeLogger.Debug("Exiting RANGE_QUERY_STATE_CLOSE")
}

// Handles the closing of a state iterator
func (handler *Handler) handleRangeQueryStateClose(msg *pb.ChaincodeMessage) {
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
			handler.serialSend(serialSendMsg)
		}()

		rangeQueryStateClose := &pb.RangeQueryStateClose{}
		unmarshalErr := proto.Unmarshal(msg.Payload, rangeQueryStateClose)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Errorf("Failed to unmarshall state range query close request. Sending %s", pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		txContext := handler.getTxContext(msg.Txid)
		iter := handler.getRangeQueryIterator(txContext, rangeQueryStateClose.ID)
		if iter != nil {
			iter.Close()
			handler.deleteRangeQueryIterator(txContext, rangeQueryStateClose.ID)
		}

		payload := &pb.RangeQueryStateResponse{HasMore: false, ID: rangeQueryStateClose.ID}
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
		// First check if this TXID is a transaction; error otherwise
		if !handler.getIsTransaction(msg.Txid) {
			payload := []byte(fmt.Sprintf("Cannot handle %s in query context", msg.Type.String()))
			chaincodeLogger.Debugf("[%s]Cannot handle %s in query context. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)
			errMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			handler.triggerNextState(errMsg, true)
			return
		}

		chaincodeLogger.Debugf("[%s]state is %s", shorttxid(msg.Txid), state)
		// Check if this is the unique request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Debug("Another request pending for this Txid. Cannot process.")
			return
		}

		var triggerNextStateMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			chaincodeLogger.Debugf("[%s]enterBusyState trigger event %s", shorttxid(triggerNextStateMsg.Txid), triggerNextStateMsg.Type)
			handler.triggerNextState(triggerNextStateMsg, true)
		}()

		ledgerObj, ledgerErr := ledger.GetLedger()
		if ledgerErr != nil {
			// Send error msg back to chaincode and trigger event
			payload := []byte(ledgerErr.Error())
			chaincodeLogger.Debugf("[%s]Failed to handle %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)
			triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		chaincodeID := handler.ChaincodeID.Name
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

			var pVal []byte
			// Encrypt the data if the confidential is enabled
			if pVal, err = handler.encrypt(msg.Txid, putStateInfo.Value); err == nil {
				// Invoke ledger to put state
				err = ledgerObj.SetState(chaincodeID, putStateInfo.Key, pVal)
			}
		} else if msg.Type.String() == pb.ChaincodeMessage_DEL_STATE.String() {
			// Invoke ledger to delete state
			key := string(msg.Payload)
			err = ledgerObj.DeleteState(chaincodeID, key)
		} else if msg.Type.String() == pb.ChaincodeMessage_INVOKE_CHAINCODE.String() {
			//check and prohibit C-call-C for CONFIDENTIAL txs
			if triggerNextStateMsg = handler.canCallChaincode(msg.Txid); triggerNextStateMsg != nil {
				return
			}
			chaincodeSpec := &pb.ChaincodeSpec{}
			unmarshalErr := proto.Unmarshal(msg.Payload, chaincodeSpec)
			if unmarshalErr != nil {
				payload := []byte(unmarshalErr.Error())
				chaincodeLogger.Debugf("[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				return
			}

			// Get the chaincodeID to invoke
			newChaincodeID := chaincodeSpec.ChaincodeID.Name

			// Create the transaction object
			chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: chaincodeSpec}
			transaction, _ := pb.NewChaincodeExecute(chaincodeInvocationSpec, msg.Txid, pb.Transaction_CHAINCODE_INVOKE)

			// Launch the new chaincode if not already running
			_, chaincodeInput, launchErr := handler.chaincodeSupport.Launch(context.Background(), transaction)
			if launchErr != nil {
				payload := []byte(launchErr.Error())
				chaincodeLogger.Debugf("[%s]Failed to launch invoked chaincode. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
				triggerNextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				return
			}

			// TODO: Need to handle timeout correctly
			timeout := time.Duration(30000) * time.Millisecond

			ccMsg, _ := createTransactionMessage(transaction.Txid, chaincodeInput)

			// Execute the chaincode
			//NOTE: when confidential C-call-C is understood, transaction should have the correct sec context for enc/dec
			response, execErr := handler.chaincodeSupport.Execute(context.Background(), newChaincodeID, ccMsg, timeout, transaction)

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
		chaincodeLogger.Debugf("[%s]Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_RESPONSE)
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
		// Mark isTransaction to allow put/del state and invoke other chaincodes
		handler.markIsTransaction(ccMsg.Txid, true)
		if err := handler.serialSend(ccMsg); err != nil {
			errMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(fmt.Sprintf("Error sending %s: %s", pb.ChaincodeMessage_INIT, err)), Txid: ccMsg.Txid}
			handler.notify(errMsg)
		}
	}
}

func (handler *Handler) enterReadyState(e *fsm.Event, state string) {
	// Now notify
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	//we have to encrypt chaincode event payload. We cannot encrypt event type as
	//it is needed by the event system to filter clients by
	if ok && msg.ChaincodeEvent != nil && msg.ChaincodeEvent.Payload != nil {
		var err error
		if msg.Payload, err = handler.encrypt(msg.Txid, msg.Payload); nil != err {
			chaincodeLogger.Errorf("[%s]Failed to encrypt chaincode event payload", msg.Txid)
			msg.Payload = []byte(fmt.Sprintf("Failed to encrypt chaincode event payload %s", err.Error()))
			msg.Type = pb.ChaincodeMessage_ERROR
		}
	}
	handler.deleteIsTransaction(msg.Txid)
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
	handler.deleteIsTransaction(msg.Txid)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Entered state %s", shorttxid(msg.Txid), state)
	handler.notify(msg)
	e.Cancel(fmt.Errorf("Entered end state"))
}

func (handler *Handler) cloneTx(tx *pb.Transaction) (*pb.Transaction, error) {
	raw, err := proto.Marshal(tx)
	if err != nil {
		chaincodeLogger.Errorf("Failed marshalling transaction [%s].", err.Error())
		return nil, err
	}

	clone := &pb.Transaction{}
	err = proto.Unmarshal(raw, clone)
	if err != nil {
		chaincodeLogger.Errorf("Failed unmarshalling transaction [%s].", err.Error())
		return nil, err
	}

	return clone, nil
}

func (handler *Handler) initializeSecContext(tx, depTx *pb.Transaction) error {
	//set deploy transaction on the handler
	if depTx != nil {
		//we are given a deep clone of depTx.. Just use it
		handler.deployTXSecContext = depTx
	} else {
		//nil depTx => tx is a deploy transaction, clone it
		var err error
		handler.deployTXSecContext, err = handler.cloneTx(tx)
		if err != nil {
			return fmt.Errorf("Failed to clone transaction: %s\n", err)
		}
	}

	//don't need the payload which is not useful and rather large
	handler.deployTXSecContext.Payload = nil

	//we need to null out path from depTx as invoke or queries don't have it
	cID := &pb.ChaincodeID{}
	err := proto.Unmarshal(handler.deployTXSecContext.ChaincodeID, cID)
	if err != nil {
		return fmt.Errorf("Failed to unmarshall : %s\n", err)
	}

	cID.Path = ""
	data, err := proto.Marshal(cID)
	if err != nil {
		return fmt.Errorf("Failed to marshall : %s\n", err)
	}

	handler.deployTXSecContext.ChaincodeID = data

	return nil
}

func (handler *Handler) setChaincodeSecurityContext(tx *pb.Transaction, msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debug("setting chaincode security context...")
	if msg.SecurityContext == nil {
		msg.SecurityContext = &pb.ChaincodeSecurityContext{}
	}
	if tx != nil {
		chaincodeLogger.Debug("setting chaincode security context. Transaction different from nil")
		chaincodeLogger.Debugf("setting chaincode security context. Metadata [% x]", tx.Metadata)

		msg.SecurityContext.CallerCert = tx.Cert
		msg.SecurityContext.CallerSign = tx.Signature
		binding, err := handler.getSecurityBinding(tx)
		if err != nil {
			chaincodeLogger.Errorf("Failed getting binding [%s]", err)
			return err
		}
		msg.SecurityContext.Binding = binding
		msg.SecurityContext.Metadata = tx.Metadata

		cis := &pb.ChaincodeInvocationSpec{}
		if err := proto.Unmarshal(tx.Payload, cis); err != nil {
			chaincodeLogger.Errorf("Failed getting payload [%s]", err)
			return err
		}

		ctorMsgRaw, err := proto.Marshal(cis.ChaincodeSpec.GetCtorMsg())
		if err != nil {
			chaincodeLogger.Errorf("Failed getting ctorMsgRaw [%s]", err)
			return err
		}

		msg.SecurityContext.Payload = ctorMsgRaw
		msg.SecurityContext.TxTimestamp = tx.Timestamp
	}
	return nil
}

//if initArgs is set (should be for "deploy" only) move to Init
//else move to ready
func (handler *Handler) initOrReady(txid string, initArgs [][]byte, tx *pb.Transaction, depTx *pb.Transaction) (chan *pb.ChaincodeMessage, error) {
	var ccMsg *pb.ChaincodeMessage
	var send bool

	txctx, funcErr := handler.createTxContext(txid, tx)
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

	if err := handler.initializeSecContext(tx, depTx); err != nil {
		handler.deleteTxContext(txid)
		return nil, err
	}

	//if security is disabled the context elements will just be nil
	if err := handler.setChaincodeSecurityContext(tx, ccMsg); err != nil {
		return nil, err
	}

	handler.triggerNextState(ccMsg, send)

	return notfy, nil
}

// Handles request to query another chaincode
func (handler *Handler) handleQueryChaincode(msg *pb.ChaincodeMessage) {
	go func() {
		// Check if this is the unique request from this chaincode txid
		uniqueReq := handler.createTXIDEntry(msg.Txid)
		if !uniqueReq {
			// Drop this request
			chaincodeLogger.Debugf("[%s]Another request pending for this Txid. Cannot process.", shorttxid(msg.Txid))
			return
		}

		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.deleteTXIDEntry(msg.Txid)
			handler.serialSend(serialSendMsg)
		}()

		//check and prohibit C-call-C for CONFIDENTIAL txs
		if serialSendMsg = handler.canCallChaincode(msg.Txid); serialSendMsg != nil {
			return
		}

		chaincodeSpec := &pb.ChaincodeSpec{}
		unmarshalErr := proto.Unmarshal(msg.Payload, chaincodeSpec)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			chaincodeLogger.Debugf("[%s]Unable to decipher payload. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		// Get the chaincodeID to invoke
		newChaincodeID := chaincodeSpec.ChaincodeID.Name

		// Create the transaction object
		chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: chaincodeSpec}
		transaction, _ := pb.NewChaincodeExecute(chaincodeInvocationSpec, msg.Txid, pb.Transaction_CHAINCODE_QUERY)

		// Launch the new chaincode if not already running
		_, chaincodeInput, launchErr := handler.chaincodeSupport.Launch(context.Background(), transaction)
		if launchErr != nil {
			payload := []byte(launchErr.Error())
			chaincodeLogger.Debugf("[%s]Failed to launch invoked chaincode. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		// TODO: Need to handle timeout correctly
		timeout := time.Duration(30000) * time.Millisecond

		ccMsg, _ := createQueryMessage(transaction.Txid, chaincodeInput)

		// Query the chaincode
		//NOTE: when confidential C-call-C is understood, transaction should have the correct sec context for enc/dec
		response, execErr := handler.chaincodeSupport.Execute(context.Background(), newChaincodeID, ccMsg, timeout, transaction)

		if execErr != nil {
			// Send error msg back to chaincode and trigger event
			payload := []byte(execErr.Error())
			chaincodeLogger.Debugf("[%s]Failed to handle %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
			return
		}

		// Send response msg back to chaincode.

		//this is need to send the payload directly to calling chaincode without
		//interpreting (in particular, don't look for errors)
		if respBytes, err := proto.Marshal(response); err != nil {
			chaincodeLogger.Errorf("[%s]Error marshaling response. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR)
			payload := []byte(execErr.Error())
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
		} else {
			chaincodeLogger.Debugf("[%s]Completed %s. Sending %s", shorttxid(msg.Txid), msg.Type.String(), pb.ChaincodeMessage_RESPONSE)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RESPONSE, Payload: respBytes, Txid: msg.Txid}
		}
	}()
}

// HandleMessage implementation of MessageHandler interface.  Peer's handling of Chaincode messages.
func (handler *Handler) HandleMessage(msg *pb.ChaincodeMessage) error {
	chaincodeLogger.Debugf("[%s]Handling ChaincodeMessage of type: %s in state %s", shorttxid(msg.Txid), msg.Type, handler.FSM.Current())

	//QUERY_COMPLETED message can happen ONLY for Transaction_QUERY (stateless)
	if msg.Type == pb.ChaincodeMessage_QUERY_COMPLETED {
		chaincodeLogger.Debugf("[%s]HandleMessage- QUERY_COMPLETED. Notify", msg.Txid)
		handler.deleteIsTransaction(msg.Txid)
		var err error
		if msg.Payload, err = handler.encrypt(msg.Txid, msg.Payload); nil != err {
			chaincodeLogger.Errorf("[%s]Failed to encrypt query result %s", msg.Txid, string(msg.Payload))
			msg.Payload = []byte(fmt.Sprintf("Failed to encrypt query result %s", err.Error()))
			msg.Type = pb.ChaincodeMessage_QUERY_ERROR
		}
		handler.notify(msg)
		return nil
	} else if msg.Type == pb.ChaincodeMessage_QUERY_ERROR {
		chaincodeLogger.Debugf("[%s]HandleMessage- QUERY_ERROR (%s). Notify", msg.Txid, string(msg.Payload))
		handler.deleteIsTransaction(msg.Txid)
		handler.notify(msg)
		return nil
	} else if msg.Type == pb.ChaincodeMessage_INVOKE_QUERY {
		// Received request to query another chaincode from shim
		chaincodeLogger.Debugf("[%s]HandleMessage- Received request to query another chaincode", msg.Txid)
		handler.handleQueryChaincode(msg)
		return nil
	}
	if handler.FSM.Cannot(msg.Type.String()) {
		// Check if this is a request from validator in query context
		if msg.Type.String() == pb.ChaincodeMessage_PUT_STATE.String() || msg.Type.String() == pb.ChaincodeMessage_DEL_STATE.String() || msg.Type.String() == pb.ChaincodeMessage_INVOKE_CHAINCODE.String() {
			// Check if this TXID is a transaction
			if !handler.getIsTransaction(msg.Txid) {
				payload := []byte(fmt.Sprintf("[%s]Cannot handle %s in query context", msg.Txid, msg.Type.String()))
				chaincodeLogger.Errorf("[%s]Cannot handle %s in query context. Sending %s", msg.Txid, msg.Type.String(), pb.ChaincodeMessage_ERROR)
				errMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
				handler.serialSend(errMsg)
				return fmt.Errorf("Cannot handle %s in query context", msg.Type.String())
			}
		}

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

func (handler *Handler) sendExecuteMessage(msg *pb.ChaincodeMessage, tx *pb.Transaction) (chan *pb.ChaincodeMessage, error) {
	txctx, err := handler.createTxContext(msg.Txid, tx)
	if err != nil {
		return nil, err
	}

	// Mark TXID as either transaction or query
	chaincodeLogger.Debugf("[%s]Inside sendExecuteMessage. Message %s", shorttxid(msg.Txid), msg.Type.String())
	if msg.Type.String() == pb.ChaincodeMessage_QUERY.String() {
		handler.markIsTransaction(msg.Txid, false)
	} else {
		handler.markIsTransaction(msg.Txid, true)
	}

	//if security is disabled the context elements will just be nil
	if err := handler.setChaincodeSecurityContext(tx, msg); err != nil {
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

/****************
func (handler *Handler) initEvent() (chan *pb.ChaincodeMessage, error) {
	if handler.responseNotifiers == nil {
		return nil,fmt.Errorf("SendMessage called before registration for Txid:%s", msg.Txid)
	}
	var notfy chan *pb.ChaincodeMessage
	handler.Lock()
	if handler.responseNotifiers[msg.Txid] != nil {
		handler.Unlock()
		return nil, fmt.Errorf("SendMessage Txid:%s exists", msg.Txid)
	}
	//note the explicit use of buffer 1. We won't block if the receiver times outi and does not wait
	//for our response
	handler.responseNotifiers[msg.Txid] = make(chan *pb.ChaincodeMessage, 1)
	handler.Unlock()

	if err := c.serialSend(msg); err != nil {
		deleteNotifier(msg.Txid)
		return nil, fmt.Errorf("SendMessage error sending %s(%s)", msg.Txid, err)
	}
	return notfy, nil
}
*******************/
