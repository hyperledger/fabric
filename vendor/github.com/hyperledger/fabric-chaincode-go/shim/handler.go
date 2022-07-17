// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package shim

import (
	"errors"
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

type state string

const (
	created     state = "created"     // start state
	established state = "established" // connection established
	ready       state = "ready"       // ready for requests
)

// PeerChaincodeStream is the common stream interface for Peer - chaincode communication.
// Both chaincode-as-server and chaincode-as-client patterns need to support this
type PeerChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
}

// ClientStream supports the (original) chaincode-as-client interaction pattern
type ClientStream interface {
	PeerChaincodeStream
	CloseSend() error
}

// Handler handler implementation for shim side of chaincode.
type Handler struct {
	// serialLock is used to prevent concurrent calls to Send on the
	// PeerChaincodeStream. This is required by gRPC.
	serialLock sync.Mutex
	// chatStream is the client used to access the chaincode support server on
	// the peer.
	chatStream PeerChaincodeStream

	// cc is the chaincode associated with this handler.
	cc Chaincode
	// state holds the current state of this handler.
	state state

	// Multiple queries (and one transaction) with different txids can be executing in parallel for this chaincode
	// responseChannels is the channel on which responses are communicated by the shim to the chaincodeStub.
	// need lock to protect chaincode from attempting
	// concurrent requests to the peer
	responseChannelsMutex sync.Mutex
	responseChannels      map[string]chan pb.ChaincodeMessage
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}

// serialSend serializes calls to Send on the gRPC client.
func (h *Handler) serialSend(msg *pb.ChaincodeMessage) error {
	h.serialLock.Lock()
	defer h.serialLock.Unlock()

	return h.chatStream.Send(msg)
}

// serialSendAsync sends the provided message asynchronously in a separate
// goroutine. The result of the send is communicated back to the caller via
// errc.
func (h *Handler) serialSendAsync(msg *pb.ChaincodeMessage, errc chan<- error) {
	go func() {
		errc <- h.serialSend(msg)
	}()
}

// transactionContextID builds a transaction context identifier by
// concatenating a channel ID and a transaction ID.
func transactionContextID(chainID, txid string) string {
	return chainID + txid
}

func (h *Handler) createResponseChannel(channelID, txid string) (<-chan pb.ChaincodeMessage, error) {
	h.responseChannelsMutex.Lock()
	defer h.responseChannelsMutex.Unlock()

	if h.responseChannels == nil {
		return nil, fmt.Errorf("[%s] cannot create response channel", shorttxid(txid))
	}

	txCtxID := transactionContextID(channelID, txid)
	if h.responseChannels[txCtxID] != nil {
		return nil, fmt.Errorf("[%s] channel exists", shorttxid(txCtxID))
	}

	responseChan := make(chan pb.ChaincodeMessage)
	h.responseChannels[txCtxID] = responseChan
	return responseChan, nil
}

func (h *Handler) deleteResponseChannel(channelID, txid string) {
	h.responseChannelsMutex.Lock()
	defer h.responseChannelsMutex.Unlock()
	if h.responseChannels != nil {
		txCtxID := transactionContextID(channelID, txid)
		delete(h.responseChannels, txCtxID)
	}
}

func (h *Handler) handleResponse(msg *pb.ChaincodeMessage) error {
	h.responseChannelsMutex.Lock()
	defer h.responseChannelsMutex.Unlock()

	if h.responseChannels == nil {
		return fmt.Errorf("[%s] Cannot send message response channel", shorttxid(msg.Txid))
	}

	txCtxID := transactionContextID(msg.ChannelId, msg.Txid)
	responseCh := h.responseChannels[txCtxID]
	if responseCh == nil {
		return fmt.Errorf("[%s] responseChannel does not exist", shorttxid(msg.Txid))
	}
	responseCh <- *msg
	return nil
}

// sendReceive sends msg to the peer and waits for the response to arrive on
// the provided responseChan. On success, the response message will be
// returned. An error will be returned msg was not successfully sent to the
// peer.
func (h *Handler) sendReceive(msg *pb.ChaincodeMessage, responseChan <-chan pb.ChaincodeMessage) (pb.ChaincodeMessage, error) {
	err := h.serialSend(msg)
	if err != nil {
		return pb.ChaincodeMessage{}, err
	}

	outmsg := <-responseChan
	return outmsg, nil
}

// NewChaincodeHandler returns a new instance of the shim side handler.
func newChaincodeHandler(peerChatStream PeerChaincodeStream, chaincode Chaincode) *Handler {
	return &Handler{
		chatStream:       peerChatStream,
		cc:               chaincode,
		responseChannels: map[string]chan pb.ChaincodeMessage{},
		state:            created,
	}
}

type stubHandlerFunc func(*pb.ChaincodeMessage) (*pb.ChaincodeMessage, error)

func (h *Handler) handleStubInteraction(handler stubHandlerFunc, msg *pb.ChaincodeMessage, errc chan<- error) {
	resp, err := handler(msg)
	if err != nil {
		resp = &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(err.Error()), Txid: msg.Txid, ChannelId: msg.ChannelId}
	}
	h.serialSendAsync(resp, errc)
}

// handleInit calls the Init function of the associated chaincode.
func (h *Handler) handleInit(msg *pb.ChaincodeMessage) (*pb.ChaincodeMessage, error) {
	// Get the function and args from Payload
	input := &pb.ChaincodeInput{}
	err := proto.Unmarshal(msg.Payload, input)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %s", err)
	}

	// Create the ChaincodeStub which the chaincode can use to callback
	stub, err := newChaincodeStub(h, msg.ChannelId, msg.Txid, input, msg.Proposal)
	if err != nil {
		return nil, fmt.Errorf("failed to create new ChaincodeStub: %s", err)
	}

	res := h.cc.Init(stub)
	if res.Status >= ERROR {
		return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: []byte(res.Message), Txid: msg.Txid, ChaincodeEvent: stub.chaincodeEvent, ChannelId: msg.ChannelId}, nil
	}

	resBytes, err := proto.Marshal(&res)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %s", err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: resBytes, Txid: msg.Txid, ChaincodeEvent: stub.chaincodeEvent, ChannelId: stub.ChannelID}, nil
}

// handleTransaction calls Invoke on the associated chaincode.
func (h *Handler) handleTransaction(msg *pb.ChaincodeMessage) (*pb.ChaincodeMessage, error) {
	// Get the function and args from Payload
	input := &pb.ChaincodeInput{}
	err := proto.Unmarshal(msg.Payload, input)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %s", err)
	}

	// Create the ChaincodeStub which the chaincode can use to callback
	stub, err := newChaincodeStub(h, msg.ChannelId, msg.Txid, input, msg.Proposal)
	if err != nil {
		return nil, fmt.Errorf("failed to create new ChaincodeStub: %s", err)
	}

	res := h.cc.Invoke(stub)

	// Endorser will handle error contained in Response.
	resBytes, err := proto.Marshal(&res)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %s", err)
	}

	return &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_COMPLETED, Payload: resBytes, Txid: msg.Txid, ChaincodeEvent: stub.chaincodeEvent, ChannelId: stub.ChannelID}, nil
}

// callPeerWithChaincodeMsg sends a chaincode message to the peer for the given
// txid and channel and receives the response.
func (h *Handler) callPeerWithChaincodeMsg(msg *pb.ChaincodeMessage, channelID, txid string) (pb.ChaincodeMessage, error) {
	// Create the channel on which to communicate the response from the peer
	respChan, err := h.createResponseChannel(channelID, txid)
	if err != nil {
		return pb.ChaincodeMessage{}, err
	}
	defer h.deleteResponseChannel(channelID, txid)

	return h.sendReceive(msg, respChan)
}

// handleGetState communicates with the peer to fetch the requested state information from the ledger.
func (h *Handler) handleGetState(collection string, key string, channelID string, txid string) ([]byte, error) {
	// Construct payload for GET_STATE
	payloadBytes := marshalOrPanic(&pb.GetState{Collection: collection, Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return nil, fmt.Errorf("[%s] error sending %s: %s", shorttxid(txid), pb.ChaincodeMessage_GET_STATE, err)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		return responseMsg.Payload, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleGetPrivateDataHash(collection string, key string, channelID string, txid string) ([]byte, error) {
	// Construct payload for GET_PRIVATE_DATA_HASH
	payloadBytes := marshalOrPanic(&pb.GetState{Collection: collection, Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_PRIVATE_DATA_HASH, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return nil, fmt.Errorf("[%s] error sending %s: %s", shorttxid(txid), pb.ChaincodeMessage_GET_PRIVATE_DATA_HASH, err)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		return responseMsg.Payload, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleGetStateMetadata(collection string, key string, channelID string, txID string) (map[string][]byte, error) {
	// Construct payload for GET_STATE_METADATA
	payloadBytes := marshalOrPanic(&pb.GetStateMetadata{Collection: collection, Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_METADATA, Payload: payloadBytes, Txid: txID, ChannelId: channelID}
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txID)
	if err != nil {
		return nil, fmt.Errorf("[%s] error sending %s: %s", shorttxid(txID), pb.ChaincodeMessage_GET_STATE_METADATA, err)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		var mdResult pb.StateMetadataResult
		err := proto.Unmarshal(responseMsg.Payload, &mdResult)
		if err != nil {
			return nil, errors.New("Could not unmarshal metadata response")
		}
		metadata := make(map[string][]byte)
		for _, md := range mdResult.Entries {
			metadata[md.Metakey] = md.Value
		}

		return metadata, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

// handlePutState communicates with the peer to put state information into the ledger.
func (h *Handler) handlePutState(collection string, key string, value []byte, channelID string, txid string) error {
	// Construct payload for PUT_STATE
	payloadBytes := marshalOrPanic(&pb.PutState{Collection: collection, Key: key, Value: value})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE, Payload: payloadBytes, Txid: txid, ChannelId: channelID}

	// Execute the request and get response
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return fmt.Errorf("[%s] error sending %s: %s", msg.Txid, pb.ChaincodeMessage_PUT_STATE, err)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		return nil
	}

	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return fmt.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handlePutStateMetadataEntry(collection string, key string, metakey string, metadata []byte, channelID string, txID string) error {
	// Construct payload for PUT_STATE_METADATA
	md := &pb.StateMetadata{Metakey: metakey, Value: metadata}
	payloadBytes := marshalOrPanic(&pb.PutStateMetadata{Collection: collection, Key: key, Metadata: md})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PUT_STATE_METADATA, Payload: payloadBytes, Txid: txID, ChannelId: channelID}
	// Execute the request and get response
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txID)
	if err != nil {
		return fmt.Errorf("[%s] error sending %s: %s", msg.Txid, pb.ChaincodeMessage_PUT_STATE_METADATA, err)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		return nil
	}

	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return fmt.Errorf("[%s]incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

// handleDelState communicates with the peer to delete a key from the state in the ledger.
func (h *Handler) handleDelState(collection string, key string, channelID string, txid string) error {
	payloadBytes := marshalOrPanic(&pb.DelState{Collection: collection, Key: key})
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_DEL_STATE, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	// Execute the request and get response
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_DEL_STATE)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		return nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return fmt.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

// handlerPurgeState communicates with the peer to purge a state from private data
func (h *Handler) handlePurgeState(collection string, key string, channelID string, txid string) error {
	payloadBytes := marshalOrPanic(&pb.DelState{Collection: collection, Key: key})
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_PURGE_PRIVATE_DATA, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	// Execute the request and get response
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_DEL_STATE)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		return nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return fmt.Errorf("[%s] incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleGetStateByRange(collection, startKey, endKey string, metadata []byte,
	channelID string, txid string) (*pb.QueryResponse, error) {
	// Send GET_STATE_BY_RANGE message to peer chaincode support
	payloadBytes := marshalOrPanic(&pb.GetStateByRange{Collection: collection, StartKey: startKey, EndKey: endKey, Metadata: metadata})
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_STATE_BY_RANGE, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return nil, fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_STATE_BY_RANGE)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		rangeQueryResponse := &pb.QueryResponse{}
		err = proto.Unmarshal(responseMsg.Payload, rangeQueryResponse)
		if err != nil {
			return nil, fmt.Errorf("[%s] GetStateByRangeResponse unmarshall error", shorttxid(responseMsg.Txid))
		}

		return rangeQueryResponse, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleQueryStateNext(id, channelID, txid string) (*pb.QueryResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, err := h.createResponseChannel(channelID, txid)
	if err != nil {
		return nil, err
	}
	defer h.deleteResponseChannel(channelID, txid)

	// Send QUERY_STATE_NEXT message to peer chaincode support
	payloadBytes := marshalOrPanic(&pb.QueryStateNext{Id: id})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_NEXT, Payload: payloadBytes, Txid: txid, ChannelId: channelID}

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = h.sendReceive(msg, respChan); err != nil {
		return nil, fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_NEXT)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		queryResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, queryResponse); err != nil {
			return nil, fmt.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return queryResponse, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleQueryStateClose(id, channelID, txid string) (*pb.QueryResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, err := h.createResponseChannel(channelID, txid)
	if err != nil {
		return nil, err
	}
	defer h.deleteResponseChannel(channelID, txid)

	// Send QUERY_STATE_CLOSE message to peer chaincode support
	payloadBytes := marshalOrPanic(&pb.QueryStateClose{Id: id})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_QUERY_STATE_CLOSE, Payload: payloadBytes, Txid: txid, ChannelId: channelID}

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = h.sendReceive(msg, respChan); err != nil {
		return nil, fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_QUERY_STATE_CLOSE)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		queryResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, queryResponse); err != nil {
			return nil, fmt.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return queryResponse, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleGetQueryResult(collection string, query string, metadata []byte,
	channelID string, txid string) (*pb.QueryResponse, error) {
	// Send GET_QUERY_RESULT message to peer chaincode support
	payloadBytes := marshalOrPanic(&pb.GetQueryResult{Collection: collection, Query: query, Metadata: metadata})
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_QUERY_RESULT, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	responseMsg, err := h.callPeerWithChaincodeMsg(msg, channelID, txid)
	if err != nil {
		return nil, fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_QUERY_RESULT)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		executeQueryResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, executeQueryResponse); err != nil {
			return nil, fmt.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return executeQueryResponse, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) handleGetHistoryForKey(key string, channelID string, txid string) (*pb.QueryResponse, error) {
	// Create the channel on which to communicate the response from validating peer
	respChan, err := h.createResponseChannel(channelID, txid)
	if err != nil {
		return nil, err
	}
	defer h.deleteResponseChannel(channelID, txid)

	// Send GET_HISTORY_FOR_KEY message to peer chaincode support
	payloadBytes := marshalOrPanic(&pb.GetHistoryForKey{Key: key})

	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_GET_HISTORY_FOR_KEY, Payload: payloadBytes, Txid: txid, ChannelId: channelID}
	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = h.sendReceive(msg, respChan); err != nil {
		return nil, fmt.Errorf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_GET_HISTORY_FOR_KEY)
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		getHistoryForKeyResponse := &pb.QueryResponse{}
		if err = proto.Unmarshal(responseMsg.Payload, getHistoryForKeyResponse); err != nil {
			return nil, fmt.Errorf("[%s] unmarshal error", shorttxid(responseMsg.Txid))
		}

		return getHistoryForKeyResponse, nil
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return nil, fmt.Errorf("%s", responseMsg.Payload[:])
	}

	// Incorrect chaincode message received
	return nil, fmt.Errorf("incorrect chaincode message %s received. Expecting %s or %s", responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)
}

func (h *Handler) createResponse(status int32, payload []byte) pb.Response {
	return pb.Response{Status: status, Payload: payload}
}

// handleInvokeChaincode communicates with the peer to invoke another chaincode.
func (h *Handler) handleInvokeChaincode(chaincodeName string, args [][]byte, channelID string, txid string) pb.Response {
	payloadBytes := marshalOrPanic(&pb.ChaincodeSpec{ChaincodeId: &pb.ChaincodeID{Name: chaincodeName}, Input: &pb.ChaincodeInput{Args: args}})

	// Create the channel on which to communicate the response from validating peer
	respChan, err := h.createResponseChannel(channelID, txid)
	if err != nil {
		return h.createResponse(ERROR, []byte(err.Error()))
	}
	defer h.deleteResponseChannel(channelID, txid)

	// Send INVOKE_CHAINCODE message to peer chaincode support
	msg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_INVOKE_CHAINCODE, Payload: payloadBytes, Txid: txid, ChannelId: channelID}

	var responseMsg pb.ChaincodeMessage

	if responseMsg, err = h.sendReceive(msg, respChan); err != nil {
		errStr := fmt.Sprintf("[%s] error sending %s", shorttxid(msg.Txid), pb.ChaincodeMessage_INVOKE_CHAINCODE)
		return h.createResponse(ERROR, []byte(errStr))
	}

	if responseMsg.Type == pb.ChaincodeMessage_RESPONSE {
		// Success response
		respMsg := &pb.ChaincodeMessage{}
		if err := proto.Unmarshal(responseMsg.Payload, respMsg); err != nil {
			return h.createResponse(ERROR, []byte(err.Error()))
		}
		if respMsg.Type == pb.ChaincodeMessage_COMPLETED {
			// Success response
			res := &pb.Response{}
			if err = proto.Unmarshal(respMsg.Payload, res); err != nil {
				return h.createResponse(ERROR, []byte(err.Error()))
			}
			return *res
		}
		return h.createResponse(ERROR, responseMsg.Payload)
	}
	if responseMsg.Type == pb.ChaincodeMessage_ERROR {
		// Error response
		return h.createResponse(ERROR, responseMsg.Payload)
	}

	// Incorrect chaincode message received
	return h.createResponse(ERROR, []byte(fmt.Sprintf("[%s] Incorrect chaincode message %s received. Expecting %s or %s", shorttxid(responseMsg.Txid), responseMsg.Type, pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR)))
}

// handleReady handles messages received from the peer when the handler is in the "ready" state.
func (h *Handler) handleReady(msg *pb.ChaincodeMessage, errc chan error) error {
	switch msg.Type {
	case pb.ChaincodeMessage_RESPONSE, pb.ChaincodeMessage_ERROR:
		if err := h.handleResponse(msg); err != nil {
			return err
		}
		return nil

	case pb.ChaincodeMessage_INIT:
		go h.handleStubInteraction(h.handleInit, msg, errc)
		return nil

	case pb.ChaincodeMessage_TRANSACTION:
		go h.handleStubInteraction(h.handleTransaction, msg, errc)
		return nil

	default:
		return fmt.Errorf("[%s] Chaincode h cannot handle message (%s) while in state: %s", msg.Txid, msg.Type, h.state)
	}
}

// handleEstablished handles messages received from the peer when the handler is in the "established" state.
func (h *Handler) handleEstablished(msg *pb.ChaincodeMessage, errc chan error) error {
	if msg.Type != pb.ChaincodeMessage_READY {
		return fmt.Errorf("[%s] Chaincode h cannot handle message (%s) while in state: %s", msg.Txid, msg.Type, h.state)
	}

	h.state = ready
	return nil
}

// hanndleCreated handles messages received from the peer when the handler is in the "created" state.
func (h *Handler) handleCreated(msg *pb.ChaincodeMessage, errc chan error) error {
	if msg.Type != pb.ChaincodeMessage_REGISTERED {
		return fmt.Errorf("[%s] Chaincode h cannot handle message (%s) while in state: %s", msg.Txid, msg.Type, h.state)
	}

	h.state = established
	return nil
}

// handleMessage message handles loop for shim side of chaincode/peer stream.
func (h *Handler) handleMessage(msg *pb.ChaincodeMessage, errc chan error) error {
	if msg.Type == pb.ChaincodeMessage_KEEPALIVE {
		h.serialSendAsync(msg, errc)
		return nil
	}
	var err error

	switch h.state {
	case ready:
		err = h.handleReady(msg, errc)
	case established:
		err = h.handleEstablished(msg, errc)
	case created:
		err = h.handleCreated(msg, errc)
	default:
		panic(fmt.Sprintf("invalid handler state: %s", h.state))
	}

	if err != nil {
		payload := []byte(err.Error())
		errorMsg := &pb.ChaincodeMessage{Type: pb.ChaincodeMessage_ERROR, Payload: payload, Txid: msg.Txid}
		h.serialSend(errorMsg)
		return err
	}

	return nil
}

// marshalOrPanic attempts to marshal the provided protobbuf message but will panic
// when marshaling fails instead of returning an error.
func marshalOrPanic(msg proto.Message) []byte {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal message: %s", err))
	}
	return bytes
}
