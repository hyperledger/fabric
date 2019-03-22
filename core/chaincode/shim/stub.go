/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package shim

import (
	"fmt"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// ChaincodeStub is an object passed to chaincode for shim side handling of
// APIs.
type ChaincodeStub struct {
	TxID                       string
	ChannelId                  string
	chaincodeEvent             *pb.ChaincodeEvent
	args                       [][]byte
	handler                    *Handler
	signedProposal             *pb.SignedProposal
	proposal                   *pb.Proposal
	validationParameterMetakey string

	// Additional fields extracted from the signedProposal
	creator   []byte
	transient map[string][]byte
	binding   []byte

	decorations map[string][]byte
}

// -- init stub ---
// ChaincodeInvocation functionality

func (stub *ChaincodeStub) init(handler *Handler, channelId string, txid string, input *pb.ChaincodeInput, signedProposal *pb.SignedProposal) error {
	stub.TxID = txid
	stub.ChannelId = channelId
	stub.args = input.Args
	stub.handler = handler
	stub.signedProposal = signedProposal
	stub.decorations = input.Decorations
	stub.validationParameterMetakey = pb.MetaDataKeys_VALIDATION_PARAMETER.String()

	// TODO: sanity check: verify that every call to init with a nil
	// signedProposal is a legitimate one, meaning it is an internal call
	// to system chaincodes.
	if signedProposal != nil {
		var err error

		stub.proposal, err = protoutil.GetProposal(signedProposal.ProposalBytes)
		if err != nil {
			return errors.WithMessage(err, "failed extracting signedProposal from signed signedProposal")
		}

		// Extract creator, transient, binding...
		stub.creator, stub.transient, err = protoutil.GetChaincodeProposalContext(stub.proposal)
		if err != nil {
			return errors.WithMessage(err, "failed extracting signedProposal fields")
		}

		stub.binding, err = protoutil.ComputeProposalBinding(stub.proposal)
		if err != nil {
			return errors.WithMessage(err, "failed computing binding from signedProposal")
		}
	}

	return nil
}

// GetTxID returns the transaction ID for the proposal
func (stub *ChaincodeStub) GetTxID() string {
	return stub.TxID
}

// GetChannelID returns the channel for the proposal
func (stub *ChaincodeStub) GetChannelID() string {
	return stub.ChannelId
}

func (stub *ChaincodeStub) GetDecorations() map[string][]byte {
	return stub.decorations
}

// ------------- Call Chaincode functions ---------------

// InvokeChaincode documentation can be found in interfaces.go
func (stub *ChaincodeStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response {
	// Internally we handle chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	return stub.handler.handleInvokeChaincode(chaincodeName, args, stub.ChannelId, stub.TxID)
}

// --------- State functions ----------

// GetState documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetState(key string) ([]byte, error) {
	// Access public data by setting the collection to empty string
	collection := ""
	return stub.handler.handleGetState(collection, key, stub.ChannelId, stub.TxID)
}

// SetStateValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) SetStateValidationParameter(key string, ep []byte) error {
	return stub.handler.handlePutStateMetadataEntry("", key, stub.validationParameterMetakey, ep, stub.ChannelId, stub.TxID)
}

// GetStateValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStateValidationParameter(key string) ([]byte, error) {
	md, err := stub.handler.handleGetStateMetadata("", key, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	if ep, ok := md[stub.validationParameterMetakey]; ok {
		return ep, nil
	}
	return nil, nil
}

// PutState documentation can be found in interfaces.go
func (stub *ChaincodeStub) PutState(key string, value []byte) error {
	if key == "" {
		return errors.New("key must not be an empty string")
	}
	// Access public data by setting the collection to empty string
	collection := ""
	return stub.handler.handlePutState(collection, key, value, stub.ChannelId, stub.TxID)
}

func (stub *ChaincodeStub) createStateQueryIterator(response *pb.QueryResponse) *StateQueryIterator {
	return &StateQueryIterator{CommonIterator: &CommonIterator{
		handler:    stub.handler,
		channelId:  stub.ChannelId,
		txid:       stub.TxID,
		response:   response,
		currentLoc: 0}}
}

// GetQueryResult documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetQueryResult(query string) (StateQueryIteratorInterface, error) {
	// Access public data by setting the collection to empty string
	collection := ""
	// ignore QueryResponseMetadata as it is not applicable for a rich query without pagination
	iterator, _, err := stub.handleGetQueryResult(collection, query, nil)

	return iterator, err
}

// DelState documentation can be found in interfaces.go
func (stub *ChaincodeStub) DelState(key string) error {
	// Access public data by setting the collection to empty string
	collection := ""
	return stub.handler.handleDelState(collection, key, stub.ChannelId, stub.TxID)
}

//  ---------  private state functions  ---------

// GetPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateData(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleGetState(collection, key, stub.ChannelId, stub.TxID)
}

// GetPrivateDataHash documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataHash(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleGetPrivateDataHash(collection, key, stub.ChannelId, stub.TxID)
}

// PutPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) PutPrivateData(collection string, key string, value []byte) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	if key == "" {
		return fmt.Errorf("key must not be an empty string")
	}
	return stub.handler.handlePutState(collection, key, value, stub.ChannelId, stub.TxID)
}

// DelPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) DelPrivateData(collection string, key string) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleDelState(collection, key, stub.ChannelId, stub.TxID)
}

// GetPrivateDataByRange documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataByRange(collection, startKey, endKey string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

func (stub *ChaincodeStub) createRangeKeysForPartialCompositeKey(objectType string, attributes []string) (string, string, error) {
	partialCompositeKey, err := stub.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", "", err
	}
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	return startKey, endKey, nil
}

// GetPrivateDataByPartialCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}

	startKey, endKey, err := stub.createRangeKeysForPartialCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a partial composite key query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

// GetPrivateDataQueryResult documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataQueryResult(collection, query string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := stub.handleGetQueryResult(collection, query, nil)

	return iterator, err
}

// GetPrivateDataValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataValidationParameter(collection, key string) ([]byte, error) {
	md, err := stub.handler.handleGetStateMetadata(collection, key, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	if ep, ok := md[stub.validationParameterMetakey]; ok {
		return ep, nil
	}
	return nil, nil
}

// SetPrivateDataValidationParameter documentation can be found in interfaces.go
func (stub *ChaincodeStub) SetPrivateDataValidationParameter(collection, key string, ep []byte) error {
	return stub.handler.handlePutStateMetadataEntry(collection, key, stub.validationParameterMetakey, ep, stub.ChannelId, stub.TxID)
}

// CommonIterator documentation can be found in interfaces.go
type CommonIterator struct {
	handler    *Handler
	channelId  string
	txid       string
	response   *pb.QueryResponse
	currentLoc int
}

// StateQueryIterator documentation can be found in interfaces.go
type StateQueryIterator struct {
	*CommonIterator
}

// HistoryQueryIterator documentation can be found in interfaces.go
type HistoryQueryIterator struct {
	*CommonIterator
}

type resultType uint8

const (
	STATE_QUERY_RESULT resultType = iota + 1
	HISTORY_QUERY_RESULT
)

func createQueryResponseMetadata(metadataBytes []byte) (*pb.QueryResponseMetadata, error) {
	metadata := &pb.QueryResponseMetadata{}
	err := proto.Unmarshal(metadataBytes, metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func (stub *ChaincodeStub) handleGetStateByRange(collection, startKey, endKey string,
	metadata []byte) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	response, err := stub.handler.handleGetStateByRange(collection, startKey, endKey, metadata, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, nil, err
	}

	iterator := stub.createStateQueryIterator(response)
	responseMetadata, err := createQueryResponseMetadata(response.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return iterator, responseMetadata, nil
}

func (stub *ChaincodeStub) handleGetQueryResult(collection, query string,
	metadata []byte) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	response, err := stub.handler.handleGetQueryResult(collection, query, metadata, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, nil, err
	}

	iterator := stub.createStateQueryIterator(response)
	responseMetadata, err := createQueryResponseMetadata(response.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return iterator, responseMetadata, nil
}

// GetStateByRange documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	collection := ""

	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

// GetHistoryForKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetHistoryForKey(key string) (HistoryQueryIteratorInterface, error) {
	response, err := stub.handler.handleGetHistoryForKey(key, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &HistoryQueryIterator{CommonIterator: &CommonIterator{stub.handler, stub.ChannelId, stub.TxID, response, 0}}, nil
}

//CreateCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return createCompositeKey(objectType, attributes)
}

//SplitCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return splitCompositeKey(compositeKey)
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

func splitCompositeKey(compositeKey string) (string, []string, error) {
	componentIndex := 1
	components := []string{}
	for i := 1; i < len(compositeKey); i++ {
		if compositeKey[i] == minUnicodeRuneValue {
			components = append(components, compositeKey[componentIndex:i])
			componentIndex = i + 1
		}
	}
	return components[0], components[1:], nil
}

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

//To ensure that simple keys do not go into composite key namespace,
//we validate simplekey to check whether the key starts with 0x00 (which
//is the namespace for compositeKey). This helps in avoding simple/composite
//key collisions.
func validateSimpleKeys(simpleKeys ...string) error {
	for _, key := range simpleKeys {
		if len(key) > 0 && key[0] == compositeKeyNamespace[0] {
			return errors.Errorf(`first character of the key [%s] contains a null character which is not allowed`, key)
		}
	}
	return nil
}

//GetStateByPartialCompositeKey function can be invoked by a chaincode to query the
//state based on a given partial composite key. This function returns an
//iterator which can be used to iterate over all composite keys whose prefix
//matches the given partial composite key. This function should be used only for
//a partial composite key. For a full composite key, an iter with empty response
//would be returned.
func (stub *ChaincodeStub) GetStateByPartialCompositeKey(objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	collection := ""
	startKey, endKey, err := stub.createRangeKeysForPartialCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a partial composite key query without pagination
	iterator, _, err := stub.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

func createQueryMetadata(pageSize int32, bookmark string) ([]byte, error) {
	// Construct the QueryMetadata with a page size and a bookmark needed for pagination
	metadata := &pb.QueryMetadata{PageSize: pageSize, Bookmark: bookmark}
	metadataBytes, err := proto.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return metadataBytes, nil
}

func (stub *ChaincodeStub) GetStateByRangeWithPagination(startKey, endKey string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, nil, err
	}

	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}

	return stub.handleGetStateByRange(collection, startKey, endKey, metadata)
}

func (stub *ChaincodeStub) GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string,
	pageSize int32, bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {

	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}

	startKey, endKey, err := stub.createRangeKeysForPartialCompositeKey(objectType, keys)
	if err != nil {
		return nil, nil, err
	}
	return stub.handleGetStateByRange(collection, startKey, endKey, metadata)
}

func (stub *ChaincodeStub) GetQueryResultWithPagination(query string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	// Access public data by setting the collection to empty string
	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}
	return stub.handleGetQueryResult(collection, query, metadata)
}

func (iter *StateQueryIterator) Next() (*queryresult.KV, error) {
	if result, err := iter.nextResult(STATE_QUERY_RESULT); err == nil {
		return result.(*queryresult.KV), err
	} else {
		return nil, err
	}
}

func (iter *HistoryQueryIterator) Next() (*queryresult.KeyModification, error) {
	if result, err := iter.nextResult(HISTORY_QUERY_RESULT); err == nil {
		return result.(*queryresult.KeyModification), err
	} else {
		return nil, err
	}
}

// HasNext documentation can be found in interfaces.go
func (iter *CommonIterator) HasNext() bool {
	if iter.currentLoc < len(iter.response.Results) || iter.response.HasMore {
		return true
	}
	return false
}

// getResultsFromBytes deserializes QueryResult and return either a KV struct
// or KeyModification depending on the result type (i.e., state (range/execute)
// query, history query). Note that commonledger.QueryResult is an empty golang
// interface that can hold values of any type.
func (iter *CommonIterator) getResultFromBytes(queryResultBytes *pb.QueryResultBytes,
	rType resultType) (commonledger.QueryResult, error) {

	if rType == STATE_QUERY_RESULT {
		stateQueryResult := &queryresult.KV{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, stateQueryResult); err != nil {
			return nil, errors.Wrap(err, "error unmarshaling result from bytes")
		}
		return stateQueryResult, nil

	} else if rType == HISTORY_QUERY_RESULT {
		historyQueryResult := &queryresult.KeyModification{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, historyQueryResult); err != nil {
			return nil, err
		}
		return historyQueryResult, nil
	}
	return nil, errors.New("wrong result type")
}

func (iter *CommonIterator) fetchNextQueryResult() error {
	if response, err := iter.handler.handleQueryStateNext(iter.response.Id, iter.channelId, iter.txid); err == nil {
		iter.currentLoc = 0
		iter.response = response
		return nil
	} else {
		return err
	}
}

// nextResult returns the next QueryResult (i.e., either a KV struct or KeyModification)
// from the state or history query iterator. Note that commonledger.QueryResult is an
// empty golang interface that can hold values of any type.
func (iter *CommonIterator) nextResult(rType resultType) (commonledger.QueryResult, error) {
	if iter.currentLoc < len(iter.response.Results) {
		// On valid access of an element from cached results
		queryResult, err := iter.getResultFromBytes(iter.response.Results[iter.currentLoc], rType)
		if err != nil {
			chaincodeLogger.Errorf("Failed to decode query results: %+v", err)
			return nil, err
		}
		iter.currentLoc++

		if iter.currentLoc == len(iter.response.Results) && iter.response.HasMore {
			// On access of last item, pre-fetch to update HasMore flag
			if err = iter.fetchNextQueryResult(); err != nil {
				chaincodeLogger.Errorf("Failed to fetch next results: %+v", err)
				return nil, err
			}
		}

		return queryResult, err
	} else if !iter.response.HasMore {
		// On call to Next() without check of HasMore
		return nil, errors.New("no such key")
	}

	// should not fall through here
	// case: no cached results but HasMore is true.
	return nil, errors.New("invalid iterator state")
}

// Close documentation can be found in interfaces.go
func (iter *CommonIterator) Close() error {
	_, err := iter.handler.handleQueryStateClose(iter.response.Id, iter.channelId, iter.txid)
	return err
}

// GetArgs documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetArgs() [][]byte {
	return stub.args
}

// GetStringArgs documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetStringArgs() []string {
	args := stub.GetArgs()
	strargs := make([]string, 0, len(args))
	for _, barg := range args {
		strargs = append(strargs, string(barg))
	}
	return strargs
}

// GetFunctionAndParameters documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetFunctionAndParameters() (function string, params []string) {
	allargs := stub.GetStringArgs()
	function = ""
	params = []string{}
	if len(allargs) >= 1 {
		function = allargs[0]
		params = allargs[1:]
	}
	return
}

// GetCreator documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetCreator() ([]byte, error) {
	return stub.creator, nil
}

// GetTransient documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetTransient() (map[string][]byte, error) {
	return stub.transient, nil
}

// GetBinding documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetBinding() ([]byte, error) {
	return stub.binding, nil
}

// GetSignedProposal documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetSignedProposal() (*pb.SignedProposal, error) {
	return stub.signedProposal, nil
}

// GetArgsSlice documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetArgsSlice() ([]byte, error) {
	args := stub.GetArgs()
	res := []byte{}
	for _, barg := range args {
		res = append(res, barg...)
	}
	return res, nil
}

// GetTxTimestamp documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	hdr, err := protoutil.GetHeader(stub.proposal.Header)
	if err != nil {
		return nil, err
	}
	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	return chdr.GetTimestamp(), nil
}

// ------------- ChaincodeEvent API ----------------------

// SetEvent documentation can be found in interfaces.go
func (stub *ChaincodeStub) SetEvent(name string, payload []byte) error {
	if name == "" {
		return errors.New("event name can not be nil string")
	}
	stub.chaincodeEvent = &pb.ChaincodeEvent{EventName: name, Payload: payload}
	return nil
}
