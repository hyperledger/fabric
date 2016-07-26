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

package shim

import (
	"errors"
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/looplab/fsm"
)

// PeerChaincodeStream interface for stream between Peer and chaincode instance.
type PeerChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
	CloseSend() error
}

type nextStateInfo struct {
	msg      *pb.ChaincodeMessage
	sendToCC bool
}

func (handler *Handler) triggerNextState(msg *pb.ChaincodeMessage, send bool) {
	handler.nextState <- &nextStateInfo{msg, send}
}

// Handler handler implementation for shim side of chaincode.
type Handler struct {
	sync.RWMutex
	//shim to peer grpc serializer. User only in serialSend
	serialLock sync.Mutex
	To         string
	ChatStream PeerChaincodeStream
	FSM        *fsm.FSM
	cc         Chaincode
	// Multiple queries (and one transaction) with different Uuids can be executing in parallel for this chaincode
	// responseChannel is the channel on which responses are communicated by the shim to the chaincodeStub.
	responseChannel map[string]chan pb.ChaincodeMessage
	// Track which UUIDs are transactions and which are queries, to decide whether get/put state and invoke chaincode are allowed.
	isTransaction map[string]bool
	nextState     chan *nextStateInfo
}

func shortuuid(uuid string) string {
	if len(uuid) < 8 {
		return uuid
	}
	return uuid[0:8]
}

func (handler *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	handler.serialLock.Lock()
	defer handler.serialLock.Unlock()
	if err := handler.ChatStream.Send(msg); err != nil {
		chaincodeLogger.Errorf("[%s]Error sending %s: %s", shortuuid(msg.Uuid), msg.Type.String(), err)
		return fmt.Errorf("Error sending %s: %s", msg.Type.String(), err)
	}
	return nil
}

func (handler *Handler) createChannel(uuid string) (chan pb.ChaincodeMessage, error) {
	handler.Lock()
	defer handler.Unlock()
	if handler.responseChannel == nil {
		return nil, fmt.Errorf("[%s]Cannot create response channel", shortuuid(uuid))
	}
	if handler.responseChannel[uuid] != nil {
		return nil, fmt.Errorf("[%s]Channel exists", shortuuid(uuid))
	}
	c := make(chan pb.ChaincodeMessage)
	handler.responseChannel[uuid] = c
	return c, nil
}

func (handler *Handler) sendChannel(msg *pb.ChaincodeMessage) error {
	handler.Lock()
	defer handler.Unlock()
	if handler.responseChannel == nil {
		return fmt.Errorf("[%s]Cannot send message response channel", shortuuid(msg.Uuid))
	}
	if handler.responseChannel[msg.Uuid] == nil {
		return fmt.Errorf("[%s]sendChannel does not exist", shortuuid(msg.Uuid))
	}

	chaincodeLogger.Debugf("[%s]before send", shortuuid(msg.Uuid))
	handler.responseChannel[msg.Uuid] <- *msg
	chaincodeLogger.Debugf("[%s]after send", shortuuid(msg.Uuid))

	return nil
}

func (handler *Handler) receiveChannel(c chan pb.ChaincodeMessage) (pb.ChaincodeMessage, bool) {
	msg, val := <-c
	return msg, val
}

func (handler *Handler) deleteChannel(uuid string) {
	handler.Lock()
	defer handler.Unlock()
	if handler.responseChannel != nil {
		delete(handler.responseChannel, uuid)
	}
}

// markIsTransaction marks a UUID as a transaction or a query; true = transaction, false = query
func (handler *Handler) markIsTransaction(uuid string, isTrans bool) bool {
	if handler.isTransaction == nil {
		return false
	}
	handler.Lock()
	defer handler.Unlock()
	handler.isTransaction[uuid] = isTrans
	return true
}

func (handler *Handler) deleteIsTransaction(uuid string) {
	handler.Lock()
	if handler.isTransaction != nil {
		delete(handler.isTransaction, uuid)
	}
	handler.Unlock()
}

// NewChaincodeHandler returns a new instance of the shim side handler.
func newChaincodeHandler(peerChatStream PeerChaincodeStream, chaincode Chaincode) *Handler {
	v := &Handler{
		ChatStream: peerChatStream,
		cc:         chaincode,
	}
	v.responseChannel = make(map[string]chan pb.ChaincodeMessage)
	v.isTransaction = make(map[string]bool)
	v.nextState = make(chan *nextStateInfo)

	// Create the shim side FSM
	v.FSM = fsm.NewFSM(
		"created",
		fsm.Events{
			{Name: pb.ChaincodeMessage_REGISTERED.String(), Src: []string{"created"}, Dst: "established"},
			{Name: pb.ChaincodeMessage_INIT.String(), Src: []string{"established"}, Dst: "init"},
			{Name: pb.ChaincodeMessage_READY.String(), Src: []string{"established"}, Dst: "ready"},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{"init"}, Dst: "established"},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{"init"}, Dst: "init"},
			{Name: pb.ChaincodeMessage_COMPLETED.String(), Src: []string{"init"}, Dst: "ready"},
			{Name: pb.ChaincodeMessage_TRANSACTION.String(), Src: []string{"ready"}, Dst: "transaction"},
			{Name: pb.ChaincodeMessage_COMPLETED.String(), Src: []string{"transaction"}, Dst: "ready"},
			{Name: pb.ChaincodeMessage_ERROR.String(), Src: []string{"transaction"}, Dst: "ready"},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{"transaction"}, Dst: "transaction"},
			{Name: pb.ChaincodeMessage_QUERY.String(), Src: []string{"transaction"}, Dst: "transaction"},
			{Name: pb.ChaincodeMessage_QUERY.String(), Src: []string{"ready"}, Dst: "ready"},
			{Name: pb.ChaincodeMessage_RESPONSE.String(), Src: []string{"ready"}, Dst: "ready"},
		},
		fsm.Callbacks{
			"before_" + pb.ChaincodeMessage_REGISTERED.String(): func(e *fsm.Event) { v.beforeRegistered(e) },
			//"after_" + pb.ChaincodeMessage_INIT.String(): func(e *fsm.Event) { v.beforeInit(e) },
			//"after_" + pb.ChaincodeMessage_TRANSACTION.String(): func(e *fsm.Event) { v.beforeTransaction(e) },
			"after_" + pb.ChaincodeMessage_RESPONSE.String(): func(e *fsm.Event) { v.afterResponse(e) },
			"after_" + pb.ChaincodeMessage_ERROR.String():    func(e *fsm.Event) { v.afterError(e) },
			"enter_init":                                     func(e *fsm.Event) { v.enterInitState(e) },
			"enter_transaction":                              func(e *fsm.Event) { v.enterTransactionState(e) },
			//"enter_ready":                                     func(e *fsm.Event) { v.enterReadyState(e) },
			"before_" + pb.ChaincodeMessage_QUERY.String(): func(e *fsm.Event) { v.beforeQuery(e) }, //only checks for QUERY
		},
	)
	return v
}

// beforeRegistered is called to handle the REGISTERED message.
func (handler *Handler) beforeRegistered(e *fsm.Event) {
	if _, ok := e.Args[0].(*pb.ChaincodeMessage); !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("Received %s, ready for invocations", pb.ChaincodeMessage_REGISTERED)
}

// handleInit handles request to initialize chaincode.
func (handler *Handler) handleInit(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the beforeInit function is exited. Interesting bug fix!!
	go func() {
		var nextStateMsg *pb.ChaincodeMessage

		send := true

		defer func() {
			handler.triggerNextState(nextStateMsg, send)
		}()

		// Get the function and args from Payload
		input := &pb.ChaincodeInput{}
		unmarshalErr := proto.Unmarshal(msg.Payload, input)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			// Send ERROR message to chaincode support and change state
			chaincodeLogger.Debugf("[%s]Incorrect payload format. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_ERROR)
			nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Uuid: msg.Uuid}
			return
		}

		// Mark as a transaction (allow put/del state)
		handler.markIsTransaction(msg.Uuid, true)

		// Call chaincode's Run
		// Create the ChaincodeStub which the chaincode can use to callback
		stub := new(ChaincodeStub)
		stub.init(msg.Uuid, msg.SecurityContext)
		res, err := handler.cc.Init(stub, input.Function, input.Args)

		// delete isTransaction entry
		handler.deleteIsTransaction(msg.Uuid)

		if err != nil {
			payload := []byte(err.Error())
			// Send ERROR message to chaincode support and change state
			chaincodeLogger.Errorf("[%s]Init failed. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_ERROR)
			nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Uuid: msg.Uuid, ChaincodeEvent: stub.chaincodeEvent}
			return
		}

		// Send COMPLETED message to chaincode support and change state
		nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: res, Uuid: msg.Uuid, ChaincodeEvent: stub.chaincodeEvent}
		chaincodeLogger.Debugf("[%s]Init succeeded. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_COMPLETED)
	}()
}

// enterInitState will initialize the chaincode if entering init from established.
func (handler *Handler) enterInitState(e *fsm.Event) {
	chaincodeLogger.Debugf("Entered state %s", handler.FSM.Current())
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Received %s, initializing chaincode", shortuuid(msg.Uuid), msg.Type.String())
	if msg.Type.String() == pb.ChaincodeMessage_INIT.String() {
		// Call the chaincode's Run function to initialize
		handler.handleInit(msg)
	}
}

// handleTransaction Handles request to execute a transaction.
func (handler *Handler) handleTransaction(msg *pb.ChaincodeMessage) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the beforeInit function is exited. Interesting bug fix!!
	go func() {
		//better not be nil
		var nextStateMsg *pb.ChaincodeMessage

		send := true

		defer func() {
			handler.triggerNextState(nextStateMsg, send)
		}()

		// Get the function and args from Payload
		input := &pb.ChaincodeInput{}
		unmarshalErr := proto.Unmarshal(msg.Payload, input)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			// Send ERROR message to chaincode support and change state
			chaincodeLogger.Debugf("[%s]Incorrect payload format. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_ERROR)
			nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Uuid: msg.Uuid}
			return
		}

		// Mark as a transaction (allow put/del state)
		handler.markIsTransaction(msg.Uuid, true)

		// Call chaincode's Run
		// Create the ChaincodeStub which the chaincode can use to callback
		stub := new(ChaincodeStub)
		stub.init(msg.Uuid, msg.SecurityContext)
		res, err := handler.cc.Invoke(stub, input.Function, input.Args)

		// delete isTransaction entry
		handler.deleteIsTransaction(msg.Uuid)

		if err != nil {
			payload := []byte(err.Error())
			// Send ERROR message to chaincode support and change state
			chaincodeLogger.Errorf("[%s]Transaction execution failed. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_ERROR)
			nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Uuid: msg.Uuid, ChaincodeEvent: stub.chaincodeEvent}
			return
		}

		// Send COMPLETED message to chaincode support and change state
		chaincodeLogger.Debugf("[%s]Transaction completed. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_COMPLETED)
		nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: res, Uuid: msg.Uuid, ChaincodeEvent: stub.chaincodeEvent}
	}()
}

// handleQuery handles request to execute a query.
func (handler *Handler) handleQuery(msg *pb.ChaincodeMessage) {
	// Query does not transition state. It can happen anytime after Ready
	go func() {
		var serialSendMsg *pb.ChaincodeMessage

		defer func() {
			handler.serialSend(serialSendMsg)
		}()

		// Get the function and args from Payload
		input := &pb.ChaincodeInput{}
		unmarshalErr := proto.Unmarshal(msg.Payload, input)
		if unmarshalErr != nil {
			payload := []byte(unmarshalErr.Error())
			// Send ERROR message to chaincode support and change state
			chaincodeLogger.Debugf("[%s]Incorrect payload format. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_QUERY_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_ERROR, Payload: payload, Uuid: msg.Uuid}
			return
		}

		// Mark as a query (do not allow put/del state)
		handler.markIsTransaction(msg.Uuid, false)

		// Call chaincode's Query
		// Create the ChaincodeStub which the chaincode can use to callback
		stub := new(ChaincodeStub)
		stub.init(msg.Uuid, msg.SecurityContext)
		res, err := handler.cc.Query(stub, input.Function, input.Args)

		// delete isTransaction entry
		handler.deleteIsTransaction(msg.Uuid)

		if err != nil {
			payload := []byte(err.Error())
			// Send ERROR message to chaincode support and change state
			chaincodeLogger.Errorf("[%s]Query execution failed. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_QUERY_ERROR)
			serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_ERROR, Payload: payload, Uuid: msg.Uuid}
			return
		}

		// Send COMPLETED message to chaincode support
		chaincodeLogger.Debugf("[%s]Query completed. Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_QUERY_COMPLETED)
		serialSendMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_COMPLETED, Payload: res, Uuid: msg.Uuid}
	}()
}

// enterTransactionState will execute chaincode's Run if coming from a TRANSACTION event.
func (handler *Handler) enterTransactionState(e *fsm.Event) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]Received %s, invoking transaction on chaincode(Src:%s, Dst:%s)", shortuuid(msg.Uuid), msg.Type.String(), e.Src, e.Dst)
	if msg.Type.String() == pb.ChaincodeMessage_TRANSACTION.String() {
		// Call the chaincode's Run function to invoke transaction
		handler.handleTransaction(msg)
	}
}

// enterReadyState will need to handle COMPLETED event by sending message to the peer
//func (handler *Handler) enterReadyState(e *fsm.Event) {

// afterCompleted will need to handle COMPLETED event by sending message to the peer
func (handler *Handler) afterCompleted(e *fsm.Event) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}
	chaincodeLogger.Debugf("[%s]sending COMPLETED to validator for tid", shortuuid(msg.Uuid))
	if err := handler.serialSend(msg); err != nil {
		e.Cancel(fmt.Errorf("send COMPLETED failed %s", err))
	}
}

// beforeQuery is invoked when a query message is received from the validator
func (handler *Handler) beforeQuery(e *fsm.Event) {
	if e.Args != nil {
		msg, ok := e.Args[0].(*pb.ChaincodeMessage)
		if !ok {
			e.Cancel(fmt.Errorf("Received unexpected message type"))
			return
		}
		handler.handleQuery(msg)
	}
}

// afterResponse is called to deliver a response or error to the chaincode stub.
func (handler *Handler) afterResponse(e *fsm.Event) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}

	if err := handler.sendChannel(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending %s (state:%s): %s", shortuuid(msg.Uuid), msg.Type, handler.FSM.Current(), err)
	} else {
		chaincodeLogger.Debugf("[%s]Received %s, communicated (state:%s)", shortuuid(msg.Uuid), msg.Type, handler.FSM.Current())
	}
}

func (handler *Handler) afterError(e *fsm.Event) {
	msg, ok := e.Args[0].(*pb.ChaincodeMessage)
	if !ok {
		e.Cancel(fmt.Errorf("Received unexpected message type"))
		return
	}

	/* TODO- revisit. This may no longer be needed with the serialized/streamlined messaging model
	 * There are two situations in which the ERROR event can be triggered:
	 * 1. When an error is encountered within handleInit or handleTransaction - some issue at the chaincode side; In this case there will be no responseChannel and the message has been sent to the validator.
	 * 2. The chaincode has initiated a request (get/put/del state) to the validator and is expecting a response on the responseChannel; If ERROR is received from validator, this needs to be notified on the responseChannel.
	 */
	if err := handler.sendChannel(msg); err == nil {
		chaincodeLogger.Debugf("[%s]Error received from validator %s, communicated(state:%s)", shortuuid(msg.Uuid), msg.Type, handler.FSM.Current())
	}
}

// TODO: Implement method to get and put entire state map and not one key at a time?
// handleGetState communicates with the validator to fetch the requested state information from the ledger.
func (handler *Handler) handleGetState(key string, uuid string) ([]byte, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Debug("Another state request pending for this Uuid. Cannot process.")
		return nil, uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send GET_STATE message to validator chaincode support
	payload := []byte(key)
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: payload, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_GET_STATE)
	if err := handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending GET_STATE %s", shortuuid(uuid), err)
		return nil, errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", shortuuid(responseMsg.Uuid))
		return nil, errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]GetState received payload %s", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)
		return responseMsg.Payload, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]GetState received error %s", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s]Incorrect chaincode message %s received. Expecting %s or %s", shortuuid(responseMsg.Uuid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.New("Incorrect chaincode message received")
}

// handlePutState communicates with the validator to put state information into the ledger.
func (handler *Handler) handlePutState(key string, value []byte, uuid string) error {
	// Check if this is a transaction
	chaincodeLogger.Debugf("[%s]Inside putstate, isTransaction = %t", shortuuid(uuid), handler.isTransaction[uuid])
	if !handler.isTransaction[uuid] {
		return errors.New("Cannot put state in query context")
	}

	payload := &pb.PutStateInfo{Key: key, Value: value}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return errors.New("Failed to process put state request")
	}

	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Errorf("[%s]Another state request pending for this Uuid. Cannot process.", shortuuid(uuid))
		return uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send PUT_STATE message to validator chaincode support
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: payloadBytes, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_PUT_STATE)
	if err = handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending PUT_STATE %s", msg.Uuid, err)
		return errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", msg.Uuid)
		return errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully updated state", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)
		return nil
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s. Payload: %s", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR, responseMsg.Payload)
		return errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s]Incorrect chaincode message %s received. Expecting %s or %s", shortuuid(responseMsg.Uuid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return errors.New("Incorrect chaincode message received")
}

// handleDelState communicates with the validator to delete a key from the state in the ledger.
func (handler *Handler) handleDelState(key string, uuid string) error {
	// Check if this is a transaction
	if !handler.isTransaction[uuid] {
		return errors.New("Cannot del state in query context")
	}

	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Errorf("[%s]Another state request pending for this Uuid. Cannot process create createChannel.", shortuuid(uuid))
		return uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send DEL_STATE message to validator chaincode support
	payload := []byte(key)
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: payload, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_DEL_STATE)
	if err := handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending DEL_STATE %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_DEL_STATE)
		return errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", shortuuid(msg.Uuid))
		return errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully deleted state", msg.Uuid, pb.ChaincodeMessage_RESPONSE)
		return nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s. Payload: %s", msg.Uuid, pb.ChaincodeMessage_ERROR, responseMsg.Payload)
		return errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s]Incorrect chaincode message %s received. Expecting %s or %s", shortuuid(responseMsg.Uuid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return errors.New("Incorrect chaincode message received")
}

func (handler *Handler) handleRangeQueryState(startKey, endKey string, uuid string) (*pb.RangeQueryStateResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Debugf("[%s]Another state request pending for this Uuid. Cannot process.", shortuuid(uuid))
		return nil, uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send RANGE_QUERY_STATE message to validator chaincode support
	payload := &pb.RangeQueryState{StartKey: startKey, EndKey: endKey}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.New("Failed to process range query state request")
	}
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RANGE_QUERY_STATE, Payload: payloadBytes, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_RANGE_QUERY_STATE)
	if err = handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_RANGE_QUERY_STATE)
		return nil, errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", uuid)
		return nil, errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully got range", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)

		rangeQueryResponse := &pb.RangeQueryStateResponse{}
		unmarshalErr := proto.Unmarshal(responseMsg.Payload, rangeQueryResponse)
		if unmarshalErr != nil {
			chaincodeLogger.Errorf("[%s]unmarshall error", shortuuid(responseMsg.Uuid))
			return nil, errors.New("Error unmarshalling RangeQueryStateResponse.")
		}

		return rangeQueryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s recieved. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.New("Incorrect chaincode message received")
}

func (handler *Handler) handleRangeQueryStateNext(id, uuid string) (*pb.RangeQueryStateResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Debugf("[%s]Another state request pending for this Uuid. Cannot process.", shortuuid(uuid))
		return nil, uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send RANGE_QUERY_STATE_NEXT message to validator chaincode support
	payload := &pb.RangeQueryStateNext{ID: id}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.New("Failed to process range query state next request")
	}
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT, Payload: payloadBytes, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT)
	if err = handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_RANGE_QUERY_STATE_NEXT)
		return nil, errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", uuid)
		return nil, errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully got range", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)

		rangeQueryResponse := &pb.RangeQueryStateResponse{}
		unmarshalErr := proto.Unmarshal(responseMsg.Payload, rangeQueryResponse)
		if unmarshalErr != nil {
			chaincodeLogger.Errorf("[%s]unmarshall error", shortuuid(responseMsg.Uuid))
			return nil, errors.New("Error unmarshalling RangeQueryStateResponse.")
		}

		return rangeQueryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s recieved. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.New("Incorrect chaincode message received")
}

func (handler *Handler) handleRangeQueryStateClose(id, uuid string) (*pb.RangeQueryStateResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Debugf("[%s]Another state request pending for this Uuid. Cannot process.", shortuuid(uuid))
		return nil, uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send RANGE_QUERY_STATE_CLOSE message to validator chaincode support
	payload := &pb.RangeQueryStateClose{ID: id}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.New("Failed to process range query state close request")
	}
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE, Payload: payloadBytes, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE)
	if err = handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_RANGE_QUERY_STATE_CLOSE)
		return nil, errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", uuid)
		return nil, errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully got range", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)

		rangeQueryResponse := &pb.RangeQueryStateResponse{}
		unmarshalErr := proto.Unmarshal(responseMsg.Payload, rangeQueryResponse)
		if unmarshalErr != nil {
			chaincodeLogger.Errorf("[%s]unmarshall error", shortuuid(responseMsg.Uuid))
			return nil, errors.New("Error unmarshalling RangeQueryStateResponse.")
		}

		return rangeQueryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s recieved. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.New("Incorrect chaincode message received")
}

// handleInvokeChaincode communicates with the validator to invoke another chaincode.
func (handler *Handler) handleInvokeChaincode(chaincodeName string, function string, args []string, uuid string) ([]byte, error) {
	// Check if this is a transaction
	if !handler.isTransaction[uuid] {
		return nil, errors.New("Cannot invoke chaincode in query context")
	}

	chaincodeID := &pb.ChaincodeID{Name: chaincodeName}
	input := &pb.ChaincodeInput{Function: function, Args: args}
	payload := &pb.ChaincodeSpec{ChaincodeID: chaincodeID, CtorMsg: input}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.New("Failed to process invoke chaincode request")
	}

	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Errorf("[%s]Another request pending for this Uuid. Cannot process.", uuid)
		return nil, uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send INVOKE_CHAINCODE message to validator chaincode support
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: payloadBytes, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_INVOKE_CHAINCODE)
	if err = handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_INVOKE_CHAINCODE)
		return nil, errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", shortuuid(msg.Uuid))
		return nil, errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully invoked chaincode", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)
		respMsg := &pb.ChaincodeMessage{}
		if err := proto.Unmarshal(responseMsg.Payload, respMsg); err != nil {
			chaincodeLogger.Errorf("[%s]Error unmarshaling called chaincode response: %s", shortuuid(responseMsg.Uuid), err)
			return nil, err
		}
		if respMsg.Type == pb.ChaincodeMessage_COMPLETED {
			// Success response
			chaincodeLogger.Debugf("[%s]Received %s. Successfully invoed chaincode", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)
			return respMsg.Payload, nil
		}
		chaincodeLogger.Errorf("[%s]Received %s. Error from chaincode", shortuuid(responseMsg.Uuid), respMsg.Type.String())
		return nil, errors.New(string(respMsg.Payload[:]))
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s.", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Debugf("[%s]Incorrect chaincode message %s received. Expecting %s or %s", shortuuid(responseMsg.Uuid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.New("Incorrect chaincode message received")
}

// handleQueryChaincode communicates with the validator to query another chaincode.
func (handler *Handler) handleQueryChaincode(chaincodeName string, function string, args []string, uuid string) ([]byte, error) {
	chaincodeID := &pb.ChaincodeID{Name: chaincodeName}
	input := &pb.ChaincodeInput{Function: function, Args: args}
	payload := &pb.ChaincodeSpec{ChaincodeID: chaincodeID, CtorMsg: input}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.New("Failed to process query chaincode request")
	}

	// Create the channel on which to communicate the response from validating peer
	respChan, uniqueReqErr := handler.createChannel(uuid)
	if uniqueReqErr != nil {
		chaincodeLogger.Debug("Another request pending for this Uuid. Cannot process.")
		return nil, uniqueReqErr
	}

	defer handler.deleteChannel(uuid)

	// Send INVOKE_QUERY message to validator chaincode support
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_QUERY, Payload: payloadBytes, Uuid: uuid}
	chaincodeLogger.Debugf("[%s]Sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_INVOKE_QUERY)
	if err = handler.serialSend(msg); err != nil {
		chaincodeLogger.Errorf("[%s]error sending %s", shortuuid(msg.Uuid), pb.ChaincodeMessage_INVOKE_QUERY)
		return nil, errors.New("could not send msg")
	}

	// Wait on responseChannel for response
	responseMsg, ok := handler.receiveChannel(respChan)
	if !ok {
		chaincodeLogger.Errorf("[%s]Received unexpected message type", shortuuid(msg.Uuid))
		return nil, errors.New("Received unexpected message type")
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		respMsg := &pb.ChaincodeMessage{}
		if err := proto.Unmarshal(responseMsg.Payload, respMsg); err != nil {
			chaincodeLogger.Errorf("[%s]Error unmarshaling called chaincode responseP: %s", shortuuid(responseMsg.Uuid), err)
			return nil, err
		}
		if respMsg.Type == pb.ChaincodeMessage_QUERY_COMPLETED {
			// Success response
			chaincodeLogger.Debugf("[%s]Received %s. Successfully queried chaincode", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_RESPONSE)
			return respMsg.Payload, nil
		}
		chaincodeLogger.Errorf("[%s]Error from chaincode: %s", shortuuid(responseMsg.Uuid), string(respMsg.Payload[:]))
		return nil, errors.New(string(respMsg.Payload[:]))
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s.", shortuuid(responseMsg.Uuid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s]Incorrect chaincode message %s recieved. Expecting %s or %s", shortuuid(responseMsg.Uuid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.New("Incorrect chaincode message received")
}

// handleMessage message handles loop for shim side of chaincode/validator stream.
func (handler *Handler) handleMessage(msg *pb.ChaincodeMessage) error {
	if msg.Type == pb.ChaincodeMessage_KEEPALIVE {
		// Received a keep alive message, we don't do anything with it for now
		// and it does not touch the state machine
		return nil
	}
	chaincodeLogger.Debugf("[%s]Handling ChaincodeMessage of type: %s(state:%s)", shortuuid(msg.Uuid), msg.Type, handler.FSM.Current())
	if handler.FSM.Cannot(msg.Type.String()) {
		errStr := fmt.Sprintf("[%s]Chaincode handler FSM cannot handle message (%s) with payload size (%d) while in state: %s", msg.Uuid, msg.Type.String(), len(msg.Payload), handler.FSM.Current())
		err := errors.New(errStr)
		payload := []byte(err.Error())
		errorMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Uuid: msg.Uuid}
		handler.serialSend(errorMsg)
		return err
	}
	err := handler.FSM.Event(msg.Type.String(), msg)
	return filterError(err)
}

// filterError filters the errors to allow NoTransitionError and CanceledError to not propagate for cases where embedded Err == nil.
func filterError(errFromFSMEvent error) error {
	if errFromFSMEvent != nil {
		if noTransitionErr, ok := errFromFSMEvent.(*fsm.NoTransitionError); ok {
			if noTransitionErr.Err != nil {
				// Only allow NoTransitionError's, all others are considered true error.
				return errFromFSMEvent
			}
		}
		if canceledErr, ok := errFromFSMEvent.(*fsm.CanceledError); ok {
			if canceledErr.Err != nil {
				// Only allow NoTransitionError's, all others are considered true error.
				return canceledErr
				//t.Error("expected only 'NoTransitionError'")
			}
			chaincodeLogger.Debugf("Ignoring CanceledError: %s", canceledErr)
		}
	}
	return nil
}
