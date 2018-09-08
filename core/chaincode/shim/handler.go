/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package shim

import (
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/pkg/errors"
)

type state string

const (
	created     state = "created"     //start state
	established state = "established" //connection established
	ready       state = "ready"       //ready for requests

)

// PeerChaincodeStream interface for stream between Peer and chaincode instance.
type PeerChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
	CloseSend() error
}

func (handler *Handler) triggerNextState(msg *pb.ChaincodeMessage, errc chan error) {
	chaincodeLogger.Debugf("[%s] send state message %s", shorttxid(msg.Txid), msg.Type)
	handler.serialSendAsync(msg, errc)
}

// Handler handler implementation for shim side of chaincode.
type Handler struct {
	//need lock to protect chaincode from attempting
	//concurrent requests to the peer
	sync.Mutex

	//shim to peer grpc serializer. User only in serialSend
	serialLock sync.Mutex

	To         string
	ChatStream PeerChaincodeStream
	cc         Chaincode
	state      state
	// Multiple queries (and one transaction) with different txids can be executing in parallel for this chaincode
	// responseChannel is the channel on which responses are communicated by the shim to the chaincodeStub.
	responseChannel map[string]chan pb.ChaincodeMessage
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

//serialSend serializes msgs so gRPC will be happy
func (handler *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	handler.serialLock.Lock()
	defer handler.serialLock.Unlock()

	err := handler.ChatStream.Send(msg)

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
//with the same TXID to a CC on two multiple channels
func (handler *Handler) getTxCtxId(chainID string, txid string) string {
	return chainID + txid
}

func (handler *Handler) createChannel(channelID, txid string) (chan pb.ChaincodeMessage, error) {
	handler.Lock()
	defer handler.Unlock()
	if handler.responseChannel == nil {
		return nil, errors.Errorf("[%s] cannot create response channel", shorttxid(txid))
	}
	txCtxID := handler.getTxCtxId(channelID, txid)
	if handler.responseChannel[txCtxID] != nil {
		return nil, errors.Errorf("[%s] channel exists", shorttxid(txCtxID))
	}
	c := make(chan pb.ChaincodeMessage)
	handler.responseChannel[txCtxID] = c
	return c, nil
}

func (handler *Handler) sendChannel(msg *pb.ChaincodeMessage) error {
	handler.Lock()
	defer handler.Unlock()
	if handler.responseChannel == nil {
		return errors.Errorf("[%s] Cannot send message response channel", shorttxid(msg.Txid))
	}
	txCtxID := handler.getTxCtxId(msg.ChannelId, msg.Txid)
	if handler.responseChannel[txCtxID] == nil {
		return errors.Errorf("[%s] sendChannel does not exist", shorttxid(msg.Txid))
	}

	chaincodeLogger.Debugf("[%s] before send", shorttxid(msg.Txid))
	handler.responseChannel[txCtxID] <- *msg
	chaincodeLogger.Debugf("[%s] after send", shorttxid(msg.Txid))

	return nil
}

//sends a message and selects
func (handler *Handler) sendReceive(msg *pb.ChaincodeMessage, c chan pb.ChaincodeMessage) (pb.ChaincodeMessage, error) {
	errc := make(chan error, 1)
	handler.serialSendAsync(msg, errc)

	//the serialsend above will send an err or nil
	//the select filters that first error(or nil)
	//and continues to wait for the response
	//it is possible that the response triggers first
	//in which case the errc obviously worked and is
	//ignored
	for {
		select {
		case err := <-errc:
			if err == nil {
				continue
			}
			//would have been logged, return false
			return pb.ChaincodeMessage{}, err
		case outmsg, val := <-c:
			if !val {
				return pb.ChaincodeMessage{}, errors.New("unexpected failure on receive")
			}
			return outmsg, nil
		}
	}
}

func (handler *Handler) deleteChannel(channelID, txid string) {
	handler.Lock()
	defer handler.Unlock()
	if handler.responseChannel != nil {
		txCtxID := handler.getTxCtxId(channelID, txid)
		delete(handler.responseChannel, txCtxID)
	}
}

// NewChaincodeHandler returns a new instance of the shim side handler.
func newChaincodeHandler(peerChatStream PeerChaincodeStream, chaincode Chaincode) *Handler {
	v := &Handler{
		ChatStream: peerChatStream,
		cc:         chaincode,
	}
	v.responseChannel = make(map[string]chan pb.ChaincodeMessage)
	v.state = created
	return v
}

// handleInit handles request to initialize chaincode.
func (handler *Handler) handleInit(msg *pb.ChaincodeMessage, errc chan error) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the beforeInit function is exited. Interesting bug fix!!
	go func() {
		var nextStateMsg *pb.ChaincodeMessage

		defer func() {
			handler.triggerNextState(nextStateMsg, errc)
		}()

		errFunc := func(err error, payload []byte, ce *pb.ChaincodeEvent, errFmt string, args ...interface{}) *pb.ChaincodeMessage {
			if err != nil {
				// Send ERROR message to chaincode support and change state
				if payload == nil {
					payload = []byte(err.Error())
				}
				chaincodeLogger.Errorf(errFmt, args...)
				return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChaincodeEvent: ce, ChannelId: msg.ChannelId}
			}
			return nil
		}
		// Get the function and args from Payload
		input := &pb.ChaincodeInput{}
		unmarshalErr := proto.Unmarshal(msg.Payload, input)
		if nextStateMsg = errFunc(unmarshalErr, nil, nil, "[%s] Incorrect payload format. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR.String()); nextStateMsg != nil {
			return
		}

		// Call chaincode's Run
		// Create the ChaincodeStub which the chaincode can use to callback
		stub := new(ChaincodeStub)
		err := stub.init(handler, msg.ChannelId, msg.Txid, input, msg.Proposal)
		if nextStateMsg = errFunc(err, nil, stub.chaincodeEvent, "[%s] Init get error response. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR.String()); nextStateMsg != nil {
			return
		}
		res := handler.cc.Init(stub)
		chaincodeLogger.Debugf("[%s] Init get response status: %d", shorttxid(msg.Txid), res.Status)

		if res.Status >= ERROR {
			err = errors.New(res.Message)
			if nextStateMsg = errFunc(err, []byte(res.Message), stub.chaincodeEvent, "[%s] Init get error response. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR.String()); nextStateMsg != nil {
				return
			}
		}

		resBytes, err := proto.Marshal(&res)
		if err != nil {
			payload := []byte(err.Error())
			chaincodeLogger.Errorf("[%s] Init marshal response error [%s]. Sending %s", shorttxid(msg.Txid), err, pb.ChaincodeMessage_ERROR)
			nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChaincodeEvent: stub.chaincodeEvent}
			return
		}

		// Send COMPLETED message to chaincode support and change state
		nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: resBytes, Txid: msg.Txid, ChaincodeEvent: stub.chaincodeEvent, ChannelId: stub.ChannelId}
		chaincodeLogger.Debugf("[%s] Init succeeded. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_COMPLETED)
	}()
}

// handleTransaction Handles request to execute a transaction.
func (handler *Handler) handleTransaction(msg *pb.ChaincodeMessage, errc chan error) {
	// The defer followed by triggering a go routine dance is needed to ensure that the previous state transition
	// is completed before the next one is triggered. The previous state transition is deemed complete only when
	// the beforeInit function is exited. Interesting bug fix!!
	go func() {
		//better not be nil
		var nextStateMsg *pb.ChaincodeMessage

		defer func() {
			handler.triggerNextState(nextStateMsg, errc)
		}()

		errFunc := func(err error, ce *pb.ChaincodeEvent, errStr string, args ...interface{}) *pb.ChaincodeMessage {
			if err != nil {
				payload := []byte(err.Error())
				chaincodeLogger.Errorf(errStr, args...)
				return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid, ChaincodeEvent: ce, ChannelId: msg.ChannelId}
			}
			return nil
		}

		// Get the function and args from Payload
		input := &pb.ChaincodeInput{}
		unmarshalErr := proto.Unmarshal(msg.Payload, input)
		if nextStateMsg = errFunc(unmarshalErr, nil, "[%s] Incorrect payload format. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR.String()); nextStateMsg != nil {
			return
		}

		// Call chaincode's Run
		// Create the ChaincodeStub which the chaincode can use to callback
		stub := new(ChaincodeStub)
		err := stub.init(handler, msg.ChannelId, msg.Txid, input, msg.Proposal)
		if nextStateMsg = errFunc(err, stub.chaincodeEvent, "[%s] Transaction execution failed. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR.String()); nextStateMsg != nil {
			return
		}
		res := handler.cc.Invoke(stub)

		// Endorser will handle error contained in Response.
		resBytes, err := proto.Marshal(&res)
		if nextStateMsg = errFunc(err, stub.chaincodeEvent, "[%s] Transaction execution failed. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_ERROR.String()); nextStateMsg != nil {
			return
		}

		// Send COMPLETED message to chaincode support and change state
		chaincodeLogger.Debugf("[%s] Transaction completed. Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_COMPLETED)
		nextStateMsg = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: resBytes, Txid: msg.Txid, ChaincodeEvent: stub.chaincodeEvent, ChannelId: stub.ChannelId}
	}()
}

// callPeerWithChaincodeMsg sends a chaincode message (for e.g., GetState along with the key) to the peer for a given txid
// and receives the response.
func (handler *Handler) callPeerWithChaincodeMsg(msg *pb.ChaincodeMessage, channelID, txid string) (pb.ChaincodeMessage, error) {
	// Create the channel on which to communicate the response from the peer
	var respChan chan pb.ChaincodeMessage
	var err error
	if respChan, err = handler.createChannel(channelID, txid); err != nil {
		return pb.ChaincodeMessage{}, err
	}

	defer handler.deleteChannel(channelID, txid)

	return handler.sendReceive(msg, respChan)
}

// TODO: Implement a method to get multiple keys at a time [FAB-1244]
// handleGetState communicates with the peer to fetch the requested state information from the ledger.
func (handler *Handler) handleGetState(collection string, key string, channelId string, txid string) ([]byte, error) {
	// Construct payload for GET_STATE
	payloadBytes, _ := proto.Marshal(&pb.GetState{Collection: collection, Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE)

	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelId, txid)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("[%s] error sending GET_STATE", shorttxid(txid)))
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] GetState received payload %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)
		return responseMsg.Payload, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] GetState received error %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s] Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handleGetStateMetadata(collection string, key string, channelID string, txID string) (map[string][]byte, error) {
	// Construct payload for GET_STATE_METADATA
	payloadBytes, _ := proto.Marshal(&pb.GetStateMetadata{Collection: collection, Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_METADATA, Payload: payloadBytes, Txid: txID, ChannelId: channelID}
	chaincodeLogger.Debugf("[%s]Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE_METADATA)

	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelID, txID)
	if err != nil {
		return nil, errors.WithMessage(err, fmt.Sprintf("[%s]error sending GET_STATE_METADATA", shorttxid(txID)))
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]GetStateMetadata received payload %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)
		var mdResult pb.StateMetadataResult
		err := proto.Unmarshal(responseMsg.Payload, &mdResult)
		if err != nil {
			chaincodeLogger.Errorf("[%s]GetStateMetadata could not unmarshal result", shorttxid(responseMsg.Txid))
			return nil, errors.New("Could not unmarshal metadata response")
		}
		metadata := make(map[string][]byte)
		for _, md := range mdResult.Entries {
			metadata[md.Metakey] = md.Value
		}

		return metadata, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]GetStateMetadata received error %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s]Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("[%s]incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

// TODO: Implement a method to set multiple keys at a time [FAB-1244]
// handlePutState communicates with the peer to put state information into the ledger.
func (handler *Handler) handlePutState(collection string, key string, value []byte, channelId string, txid string) error {
	// Construct payload for PUT_STATE
	payloadBytes, _ := proto.Marshal(&pb.PutState{Collection: collection, Key: key, Value: value})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_PUT_STATE)

	// Execute the request and get response
	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelId, txid)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("[%s] error sending PUT_STATE", msg.Txid))
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully updated state", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)
		return nil
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s. Payload: %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR, responseMsg.Payload)
		return errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s] Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return errors.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handlePutStateMetadataEntry(collection string, key string, metakey string, metadata []byte, channelID string, txID string) error {
	// Construct payload for PUT_STATE_METADATA
	md := &pb.StateMetadata{Metakey: metakey, Value: metadata}
	payloadBytes, _ := proto.Marshal(&pb.PutStateMetadata{Collection: collection, Key: key, Metadata: md})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE_METADATA, Payload: payloadBytes, Txid: txID, ChannelId: channelID}
	chaincodeLogger.Debugf("[%s]Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_PUT_STATE_METADATA)

	// Execute the request and get response
	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelID, txID)
	if err != nil {
		return errors.WithMessage(err, fmt.Sprintf("[%s]error sending PUT_STATE_METADATA", msg.Txid))
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s]Received %s. Successfully updated state metadata", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)
		return nil
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s]Received %s. Payload: %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR, responseMsg.Payload)
		return errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s]Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return errors.Errorf("[%s]incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

// handleDelState communicates with the peer to delete a key from the state in the ledger.
func (handler *Handler) handleDelState(collection string, key string, channelId string, txid string) error {
	//payloadBytes, _ := proto.Marshal(&pb.GetState{Collection: collection, Key: key})
	payloadBytes, _ := proto.Marshal(&pb.DelState{Collection: collection, Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE)

	// Execute the request and get response
	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelId, txid)
	if err != nil {
		return errors.Errorf("[%s] error sending DEL_STATE %s", shorttxid(msg.Txid), pb.ChaincodeMessage_DEL_STATE)
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully deleted state", msg.Txid, pb.ChaincodeMessage_RESPONSE)
		return nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s. Payload: %s", msg.Txid, pb.ChaincodeMessage_ERROR, responseMsg.Payload)
		return errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s] Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return errors.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handleGetStateByRange(collection, startKey, endKey string, metadata []byte,
	channelId string, txid string) (*pb.QueryResponse, error) {
	// Send GET_STATE_BY_RANGE message to peer chaincode support
	//we constructed a valid object. No need to check for error
	payloadBytes, _ := proto.Marshal(&pb.GetStateByRange{Collection: collection, StartKey: startKey, EndKey: endKey, Metadata: metadata})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE_BY_RANGE)

	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelId, txid)
	if err != nil {
		return nil, errors.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE_BY_RANGE)
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully got range", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)

		rangeQueryResponse := &pb.QueryResponse{}
		err = proto.Unmarshal(responseMsg.Payload, rangeQueryResponse)
		if err != nil {
			chaincodeLogger.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
			return nil, errors.Errorf("[%s] GetStateByRangeResponse unmarshall error", shorttxid(responseMsg.Txid))
		}

		return rangeQueryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handleQueryStateNext(id, channelId, txid string) (*pb.QueryResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	var respChan chan pb.ChaincodeMessage
	var err error
	if respChan, err = handler.createChannel(channelId, txid); err != nil {
		chaincodeLogger.Errorf("[%s] Another state request pending for this Txid. Cannot process.", shorttxid(txid))
		return nil, err
	}

	defer handler.deleteChannel(channelId, txid)

	// Send QUERY_STATE_NEXT message to peer chaincode support
	//we constructed a valid object. No need to check for error
	payloadBytes, _ := proto.Marshal(&pb.QueryStateNext{Id: id})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_NEXT)

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = handler.sendReceive(msg, respChan); err != nil {
		chaincodeLogger.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_NEXT)
		return nil, errors.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_NEXT)
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully got range", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)

		queryResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, queryResponse); err != nil {
			chaincodeLogger.Errorf("[%s] unmarshall error", shorttxid(responseMsg.Txid))
			return nil, errors.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return queryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handleQueryStateClose(id, channelId, txid string) (*pb.QueryResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	var respChan chan pb.ChaincodeMessage
	var err error
	if respChan, err = handler.createChannel(channelId, txid); err != nil {
		chaincodeLogger.Errorf("[%s] Another state request pending for this Txid. Cannot process.", shorttxid(txid))
		return nil, err
	}

	defer handler.deleteChannel(channelId, txid)

	// Send QUERY_STATE_CLOSE message to peer chaincode support
	//we constructed a valid object. No need to check for error
	payloadBytes, _ := proto.Marshal(&pb.QueryStateClose{Id: id})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_CLOSE)

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = handler.sendReceive(msg, respChan); err != nil {
		chaincodeLogger.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_CLOSE)
		return nil, errors.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_CLOSE)
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully got range", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)

		queryResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, queryResponse); err != nil {
			chaincodeLogger.Errorf("[%s] unmarshall error", shorttxid(responseMsg.Txid))
			return nil, errors.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return queryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handleGetQueryResult(collection string, query string, metadata []byte,
	channelId string, txid string) (*pb.QueryResponse, error) {
	// Send GET_QUERY_RESULT message to peer chaincode support
	//we constructed a valid object. No need to check for error
	payloadBytes, _ := proto.Marshal(&pb.GetQueryResult{Collection: collection, Query: query, Metadata: metadata})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_QUERY_RESULT)

	responseMsg, err := handler.callPeerWithChaincodeMsg(msg, channelId, txid)
	if err != nil {
		return nil, errors.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_QUERY_RESULT)
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully got range", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)

		executeQueryResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, executeQueryResponse); err != nil {
			chaincodeLogger.Errorf("[%s] unmarshall error", shorttxid(responseMsg.Txid))
			return nil, errors.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return executeQueryResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) handleGetHistoryForKey(key string, channelId string, txid string) (*pb.QueryResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	var respChan chan pb.ChaincodeMessage
	var err error
	if respChan, err = handler.createChannel(channelId, txid); err != nil {
		chaincodeLogger.Errorf("[%s] Another state request pending for this Txid. Cannot process.", shorttxid(txid))
		return nil, err
	}

	defer handler.deleteChannel(channelId, txid)

	// Send GET_HISTORY_FOR_KEY message to peer chaincode support
	//we constructed a valid object. No need to check for error
	payloadBytes, _ := proto.Marshal(&pb.GetHistoryForKey{Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_HISTORY_FOR_KEY)

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = handler.sendReceive(msg, respChan); err != nil {
		chaincodeLogger.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_HISTORY_FOR_KEY)
		return nil, errors.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_HISTORY_FOR_KEY)
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully got range", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)

		getHistoryForKeyResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, getHistoryForKeyResponse); err != nil {
			chaincodeLogger.Errorf("[%s] unmarshall error", shorttxid(responseMsg.Txid))
			return nil, errors.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return getHistoryForKeyResponse, nil
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return nil, errors.New(string(responseMsg.Payload[:]))
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("Incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return nil, errors.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (handler *Handler) createResponse(status int32, payload []byte) pb.Response {
	return pb.Response{Status: status, Payload: payload}
}

// handleInvokeChaincode communicates with the peer to invoke another chaincode.
func (handler *Handler) handleInvokeChaincode(chaincodeName string, args [][]byte, channelId string, txid string) pb.Response {
	//we constructed a valid object. No need to check for error
	payloadBytes, _ := proto.Marshal(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: chaincodeName}, Input: &pb.ChaincodeInput{Args: args}})

	// Create the channel on which to communicate the response from validating peer
	var respChan chan pb.ChaincodeMessage
	var err error
	if respChan, err = handler.createChannel(channelId, txid); err != nil {
		return handler.createResponse(ERROR, []byte(err.Error()))
	}

	defer handler.deleteChannel(channelId, txid)

	// Send INVOKE_CHAINCODE message to peer chaincode support
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: payloadBytes, Txid: txid, ChannelId: channelId}
	chaincodeLogger.Debugf("[%s] Sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_INVOKE_CHAINCODE)

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = handler.sendReceive(msg, respChan); err != nil {
		errStr := fmt.Sprintf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_INVOKE_CHAINCODE)
		chaincodeLogger.Error(errStr)
		return handler.createResponse(ERROR, []byte(errStr))
	}

	if responseMsg.Type.String() == pb.ChaincodeMessage_RESPONSE.String() {
		// Success response
		chaincodeLogger.Debugf("[%s] Received %s. Successfully invoked chaincode", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)
		respMsg := &pb.ChaincodeMessage{}
		if err := proto.Unmarshal(responseMsg.Payload, respMsg); err != nil {
			chaincodeLogger.Errorf("[%s] Error unmarshaling called chaincode response: %s", shorttxid(responseMsg.Txid), err)
			return handler.createResponse(ERROR, []byte(err.Error()))
		}
		if respMsg.Type == pb.ChaincodeMessage_COMPLETED {
			// Success response
			chaincodeLogger.Debugf("[%s] Received %s. Successfully invoked chaincode", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_RESPONSE)
			res := &pb.Response{}
			if err = proto.Unmarshal(respMsg.Payload, res); err != nil {
				chaincodeLogger.Errorf("[%s] Error unmarshaling payload of response: %s", shorttxid(responseMsg.Txid), err)
				return handler.createResponse(ERROR, []byte(err.Error()))
			}
			return *res
		}
		chaincodeLogger.Errorf("[%s] Received %s. Error from chaincode", shorttxid(responseMsg.Txid), respMsg.Type)
		return handler.createResponse(ERROR, responseMsg.Payload)
	}
	if responseMsg.Type.String() == pb.ChaincodeMessage_ERROR.String() {
		// Error response
		chaincodeLogger.Errorf("[%s] Received %s.", shorttxid(responseMsg.Txid), pb.ChaincodeMessage_ERROR)
		return handler.createResponse(ERROR, responseMsg.Payload)
	}

	// Incorrect chaincode message received
	chaincodeLogger.Errorf("[%s] Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
	return handler.createResponse(ERROR, []byte(fmt.Sprintf("[%s] Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)))
}

//handle ready state
func (handler *Handler) handleReady(msg *pb.ChaincodeMessage, errc chan error) error {
	switch msg.Type {
	case pb.ChaincodeMessage_RESPONSE:
		if err := handler.sendChannel(msg); err != nil {
			chaincodeLogger.Errorf("[%s] error sending %s (state:%s): %s", shorttxid(msg.Txid), msg.Type, handler.state, err)
			return err
		}
		chaincodeLogger.Debugf("[%s] Received %s, communicated (state:%s)", shorttxid(msg.Txid), msg.Type, handler.state)
		return nil

	case pb.ChaincodeMessage_ERROR:
		if err := handler.sendChannel(msg); err != nil {
			chaincodeLogger.Errorf("[%s] error sending %s (state:%s): %s", shorttxid(msg.Txid), msg.Type, handler.state, err)
		}

		chaincodeLogger.Debugf("[%s] Error Received %s, communicated (state:%s)", shorttxid(msg.Txid), msg.Type, handler.state)

		//we don't return error on ERROR
		return nil

	case pb.ChaincodeMessage_INIT:
		chaincodeLogger.Debugf("[%s] Received %s, initializing chaincode", shorttxid(msg.Txid), msg.Type)
		// Call the chaincode's Run function to initialize
		handler.handleInit(msg, errc)
		return nil

	case pb.ChaincodeMessage_TRANSACTION:
		chaincodeLogger.Debugf("[%s] Received %s, invoking transaction on chaincode(state:%s)", shorttxid(msg.Txid), msg.Type, handler.state)
		// Call the chaincode's Run function to invoke transaction
		handler.handleTransaction(msg, errc)
		return nil
	}

	return errors.Errorf("[%s] Chaincode handler cannot handle message (%s) with payload size (%d) while in state: %s", msg.Txid, msg.Type, len(msg.Payload), handler.state)
}

//handle established state
func (handler *Handler) handleEstablished(msg *pb.ChaincodeMessage, errc chan error) error {
	if msg.Type == pb.ChaincodeMessage_READY {
		handler.state = ready
		return nil
	}
	return errors.Errorf("[%s] Chaincode handler cannot handle message (%s) with payload size (%d) while in state: %s", msg.Txid, msg.Type, len(msg.Payload), handler.state)
}

//handle created state
func (handler *Handler) handleCreated(msg *pb.ChaincodeMessage, errc chan error) error {
	if msg.Type == pb.ChaincodeMessage_REGISTERED {
		handler.state = established
		return nil
	}
	return errors.Errorf("[%s] Chaincode handler cannot handle message (%s) with payload size (%d) while in state: %s", msg.Txid, msg.Type, len(msg.Payload), handler.state)
}

// handleMessage message handles loop for shim side of chaincode/peer stream.
func (handler *Handler) handleMessage(msg *pb.ChaincodeMessage, errc chan error) error {
	if msg.Type == pb.ChaincodeMessage_KEEPALIVE {
		chaincodeLogger.Debug("Sending KEEPALIVE response")
		handler.serialSendAsync(msg, nil) // ignore errors, maybe next KEEPALIVE will work
		return nil
	}
	chaincodeLogger.Debugf("[%s] Handling ChaincodeMessage of type: %s(state:%s)", shorttxid(msg.Txid), msg.Type, handler.state)

	var err error

	switch handler.state {
	case ready:
		err = handler.handleReady(msg, errc)
	case established:
		err = handler.handleEstablished(msg, errc)
	case created:
		err = handler.handleCreated(msg, errc)
	default:
		err = errors.Errorf("[%s] Chaincode handler cannot handle message (%s) with payload size (%d) while in state: %s", msg.Txid, msg.Type, len(msg.Payload), handler.state)
	}

	if err != nil {
		payload := []byte(err.Error())
		errorMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
		handler.serialSend(errorMsg)
		return err
	}

	return nil
}
