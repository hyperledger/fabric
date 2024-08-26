// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package shim

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ChaincodeStub is an object passed to chaincode for shim side handling of
// APIs.
type ChaincodeStub struct {
	TxID                       string
	ChannelID                  string
	chaincodeEvent             *peer.ChaincodeEvent
	args                       [][]byte
	handler                    *Handler
	signedProposal             *peer.SignedProposal
	proposal                   *peer.Proposal
	validationParameterMetakey string

	// Additional fields extracted from the signedProposal
	creator   []byte
	transient map[string][]byte
	binding   []byte

	decorations map[string][]byte
}

// ChaincodeInvocation functionality

func newChaincodeStub(handler *Handler, channelID, txid string, input *peer.ChaincodeInput, signedProposal *peer.SignedProposal) (*ChaincodeStub, error) {
	stub := &ChaincodeStub{
		TxID:                       txid,
		ChannelID:                  channelID,
		args:                       input.Args,
		handler:                    handler,
		signedProposal:             signedProposal,
		decorations:                input.Decorations,
		validationParameterMetakey: peer.MetaDataKeys_VALIDATION_PARAMETER.String(),
	}

	// TODO: sanity check: verify that every call to init with a nil
	// signedProposal is a legitimate one, meaning it is an internal call
	// to system chaincodes.
	if signedProposal != nil {
		var err error

		stub.proposal = &peer.Proposal{}
		err = proto.Unmarshal(signedProposal.ProposalBytes, stub.proposal)
		if err != nil {

			return nil, fmt.Errorf("failed to extract Proposal from SignedProposal: %s", err)
		}

		// check for header
		if len(stub.proposal.GetHeader()) == 0 {
			return nil, errors.New("failed to extract Proposal fields: proposal header is nil")
		}

		// Extract creator, transient, binding...
		hdr := &common.Header{}
		if err := proto.Unmarshal(stub.proposal.GetHeader(), hdr); err != nil {
			return nil, fmt.Errorf("failed to extract proposal header: %s", err)
		}

		// extract and validate channel header
		chdr := &common.ChannelHeader{}
		if err := proto.Unmarshal(hdr.ChannelHeader, chdr); err != nil {
			return nil, fmt.Errorf("failed to extract channel header: %s", err)
		}
		validTypes := map[common.HeaderType]bool{
			common.HeaderType_ENDORSER_TRANSACTION: true,
			common.HeaderType_CONFIG:               true,
		}
		if !validTypes[common.HeaderType(chdr.GetType())] {
			return nil, fmt.Errorf(
				"invalid channel header type. Expected %s or %s, received %s",
				common.HeaderType_ENDORSER_TRANSACTION,
				common.HeaderType_CONFIG,
				common.HeaderType(chdr.GetType()),
			)
		}

		// extract creator from signature header
		shdr := &common.SignatureHeader{}
		if err := proto.Unmarshal(hdr.GetSignatureHeader(), shdr); err != nil {
			return nil, fmt.Errorf("failed to extract signature header: %s", err)
		}
		stub.creator = shdr.GetCreator()

		// extract transient data from proposal payload
		payload := &peer.ChaincodeProposalPayload{}
		if err := proto.Unmarshal(stub.proposal.GetPayload(), payload); err != nil {
			return nil, fmt.Errorf("failed to extract proposal payload: %s", err)
		}
		stub.transient = payload.GetTransientMap()

		// compute the proposal binding from the nonce, creator and epoch
		epoch := make([]byte, 8)
		binary.LittleEndian.PutUint64(epoch, chdr.GetEpoch())
		digest := sha256.Sum256(append(append(shdr.GetNonce(), stub.creator...), epoch...))
		stub.binding = digest[:]

	}

	return stub, nil
}

// GetTxID returns the transaction ID for the proposal
func (s *ChaincodeStub) GetTxID() string {
	return s.TxID
}

// GetChannelID returns the channel for the proposal
func (s *ChaincodeStub) GetChannelID() string {
	return s.ChannelID
}

// GetDecorations ...
func (s *ChaincodeStub) GetDecorations() map[string][]byte {
	return s.decorations
}

// GetMSPID returns the local mspid of the peer by checking the CORE_PEER_LOCALMSPID
// env var and returns an error if the env var is not set
func GetMSPID() (string, error) {
	mspid := os.Getenv("CORE_PEER_LOCALMSPID")

	if mspid == "" {
		return "", errors.New("'CORE_PEER_LOCALMSPID' is not set")
	}

	return mspid, nil
}

// ------------- Call Chaincode functions ---------------

// InvokeChaincode documentation can be found in interfaces.go
func (s *ChaincodeStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) *peer.Response {
	// Internally we handle chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	return s.handler.handleInvokeChaincode(chaincodeName, args, s.ChannelID, s.TxID)
}

// --------- State functions ----------

// GetState documentation can be found in interfaces.go
func (s *ChaincodeStub) GetState(key string) ([]byte, error) {
	// Access public data by setting the collection to empty string
	collection := ""
	return s.handler.handleGetState(collection, key, s.ChannelID, s.TxID)
}

// SetStateValidationParameter documentation can be found in interfaces.go
func (s *ChaincodeStub) SetStateValidationParameter(key string, ep []byte) error {
	return s.handler.handlePutStateMetadataEntry("", key, s.validationParameterMetakey, ep, s.ChannelID, s.TxID)
}

// GetStateValidationParameter documentation can be found in interfaces.go
func (s *ChaincodeStub) GetStateValidationParameter(key string) ([]byte, error) {
	md, err := s.handler.handleGetStateMetadata("", key, s.ChannelID, s.TxID)
	if err != nil {
		return nil, err
	}
	if ep, ok := md[s.validationParameterMetakey]; ok {
		return ep, nil
	}
	return nil, nil
}

// PutState documentation can be found in interfaces.go
func (s *ChaincodeStub) PutState(key string, value []byte) error {
	if key == "" {
		return errors.New("key must not be an empty string")
	}
	// Access public data by setting the collection to empty string
	collection := ""
	return s.handler.handlePutState(collection, key, value, s.ChannelID, s.TxID)
}

func (s *ChaincodeStub) createStateQueryIterator(response *peer.QueryResponse) *StateQueryIterator {
	return &StateQueryIterator{
		CommonIterator: &CommonIterator{
			handler:    s.handler,
			channelID:  s.ChannelID,
			txid:       s.TxID,
			response:   response,
			currentLoc: 0,
		},
	}
}

// GetQueryResult documentation can be found in interfaces.go
func (s *ChaincodeStub) GetQueryResult(query string) (StateQueryIteratorInterface, error) {
	// Access public data by setting the collection to empty string
	collection := ""
	// ignore QueryResponseMetadata as it is not applicable for a rich query without pagination
	iterator, _, err := s.handleGetQueryResult(collection, query, nil)

	return iterator, err
}

// DelState documentation can be found in interfaces.go
func (s *ChaincodeStub) DelState(key string) error {
	// Access public data by setting the collection to empty string
	collection := ""
	return s.handler.handleDelState(collection, key, s.ChannelID, s.TxID)
}

//  ---------  private state functions  ---------

// GetPrivateData documentation can be found in interfaces.go
func (s *ChaincodeStub) GetPrivateData(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return s.handler.handleGetState(collection, key, s.ChannelID, s.TxID)
}

// GetPrivateDataHash documentation can be found in interfaces.go
func (s *ChaincodeStub) GetPrivateDataHash(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return s.handler.handleGetPrivateDataHash(collection, key, s.ChannelID, s.TxID)
}

// PutPrivateData documentation can be found in interfaces.go
func (s *ChaincodeStub) PutPrivateData(collection string, key string, value []byte) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	if key == "" {
		return fmt.Errorf("key must not be an empty string")
	}
	return s.handler.handlePutState(collection, key, value, s.ChannelID, s.TxID)
}

// DelPrivateData documentation can be found in interfaces.go
func (s *ChaincodeStub) DelPrivateData(collection string, key string) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	return s.handler.handleDelState(collection, key, s.ChannelID, s.TxID)
}

// PurgePrivateData documentation can be found in interfaces.go
func (s *ChaincodeStub) PurgePrivateData(collection string, key string) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	return s.handler.handlePurgeState(collection, key, s.ChannelID, s.TxID)
}

// GetPrivateDataByRange documentation can be found in interfaces.go
func (s *ChaincodeStub) GetPrivateDataByRange(collection, startKey, endKey string) (StateQueryIteratorInterface, error) {
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
	iterator, _, err := s.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

func (s *ChaincodeStub) createRangeKeysForPartialCompositeKey(objectType string, attributes []string) (string, string, error) {
	partialCompositeKey, err := s.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", "", err
	}
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	return startKey, endKey, nil
}

// GetPrivateDataByPartialCompositeKey documentation can be found in interfaces.go
func (s *ChaincodeStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}

	startKey, endKey, err := s.createRangeKeysForPartialCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a partial composite key query without pagination
	iterator, _, err := s.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

// GetPrivateDataQueryResult documentation can be found in interfaces.go
func (s *ChaincodeStub) GetPrivateDataQueryResult(collection, query string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := s.handleGetQueryResult(collection, query, nil)

	return iterator, err
}

// GetPrivateDataValidationParameter documentation can be found in interfaces.go
func (s *ChaincodeStub) GetPrivateDataValidationParameter(collection, key string) ([]byte, error) {
	md, err := s.handler.handleGetStateMetadata(collection, key, s.ChannelID, s.TxID)
	if err != nil {
		return nil, err
	}
	if ep, ok := md[s.validationParameterMetakey]; ok {
		return ep, nil
	}
	return nil, nil
}

// SetPrivateDataValidationParameter documentation can be found in interfaces.go
func (s *ChaincodeStub) SetPrivateDataValidationParameter(collection, key string, ep []byte) error {
	return s.handler.handlePutStateMetadataEntry(collection, key, s.validationParameterMetakey, ep, s.ChannelID, s.TxID)
}

// CommonIterator documentation can be found in interfaces.go
type CommonIterator struct {
	handler    *Handler
	channelID  string
	txid       string
	response   *peer.QueryResponse
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

// General interface for supporting different types of query results.
// Actual types differ for different queries
type queryResult interface{}

type resultType uint8

// TODO: Document constants
/*
	Constants ...
*/
const (
	StateQueryResult resultType = iota + 1
	HistoryQueryResult
)

func createQueryResponseMetadata(metadataBytes []byte) (*peer.QueryResponseMetadata, error) {
	metadata := &peer.QueryResponseMetadata{}
	err := proto.Unmarshal(metadataBytes, metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func (s *ChaincodeStub) handleGetStateByRange(collection, startKey, endKey string,
	metadata []byte) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {

	response, err := s.handler.handleGetStateByRange(collection, startKey, endKey, metadata, s.ChannelID, s.TxID)
	if err != nil {
		return nil, nil, err
	}

	iterator := s.createStateQueryIterator(response)
	responseMetadata, err := createQueryResponseMetadata(response.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return iterator, responseMetadata, nil
}

func (s *ChaincodeStub) handleGetQueryResult(collection, query string,
	metadata []byte) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {

	response, err := s.handler.handleGetQueryResult(collection, query, metadata, s.ChannelID, s.TxID)
	if err != nil {
		return nil, nil, err
	}

	iterator := s.createStateQueryIterator(response)
	responseMetadata, err := createQueryResponseMetadata(response.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return iterator, responseMetadata, nil
}

// GetStateByRange documentation can be found in interfaces.go
func (s *ChaincodeStub) GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	collection := ""

	// ignore QueryResponseMetadata as it is not applicable for a range query without pagination
	iterator, _, err := s.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

// GetHistoryForKey documentation can be found in interfaces.go
func (s *ChaincodeStub) GetHistoryForKey(key string) (HistoryQueryIteratorInterface, error) {
	response, err := s.handler.handleGetHistoryForKey(key, s.ChannelID, s.TxID)
	if err != nil {
		return nil, err
	}
	return &HistoryQueryIterator{CommonIterator: &CommonIterator{s.handler, s.ChannelID, s.TxID, response, 0}}, nil
}

// CreateCompositeKey documentation can be found in interfaces.go
func (s *ChaincodeStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return CreateCompositeKey(objectType, attributes)
}

// SplitCompositeKey documentation can be found in interfaces.go
func (s *ChaincodeStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return splitCompositeKey(compositeKey)
}

// CreateCompositeKey ...
func CreateCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + string(rune(minUnicodeRuneValue))
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(rune(minUnicodeRuneValue))
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
		return fmt.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return fmt.Errorf(`input contains unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

// To ensure that simple keys do not go into composite key namespace,
// we validate simplekey to check whether the key starts with 0x00 (which
// is the namespace for compositeKey). This helps in avoiding simple/composite
// key collisions.
func validateSimpleKeys(simpleKeys ...string) error {
	for _, key := range simpleKeys {
		if len(key) > 0 && key[0] == compositeKeyNamespace[0] {
			return fmt.Errorf(`first character of the key [%s] contains a null character which is not allowed`, key)
		}
	}
	return nil
}

// GetStateByPartialCompositeKey documentation can be found in interfaces.go
func (s *ChaincodeStub) GetStateByPartialCompositeKey(objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	collection := ""
	startKey, endKey, err := s.createRangeKeysForPartialCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	// ignore QueryResponseMetadata as it is not applicable for a partial composite key query without pagination
	iterator, _, err := s.handleGetStateByRange(collection, startKey, endKey, nil)

	return iterator, err
}

func createQueryMetadata(pageSize int32, bookmark string) ([]byte, error) {
	// Construct the QueryMetadata with a page size and a bookmark needed for pagination
	metadata := &peer.QueryMetadata{PageSize: pageSize, Bookmark: bookmark}
	metadataBytes, err := proto.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return metadataBytes, nil
}

// GetStateByRangeWithPagination ...
func (s *ChaincodeStub) GetStateByRangeWithPagination(startKey, endKey string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {

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

	return s.handleGetStateByRange(collection, startKey, endKey, metadata)
}

// GetStateByPartialCompositeKeyWithPagination ...
func (s *ChaincodeStub) GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string,
	pageSize int32, bookmark string) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {

	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}

	startKey, endKey, err := s.createRangeKeysForPartialCompositeKey(objectType, keys)
	if err != nil {
		return nil, nil, err
	}
	return s.handleGetStateByRange(collection, startKey, endKey, metadata)
}

// GetQueryResultWithPagination ...
func (s *ChaincodeStub) GetQueryResultWithPagination(query string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *peer.QueryResponseMetadata, error) {
	// Access public data by setting the collection to empty string
	collection := ""

	metadata, err := createQueryMetadata(pageSize, bookmark)
	if err != nil {
		return nil, nil, err
	}
	return s.handleGetQueryResult(collection, query, metadata)
}

// Next ...
func (iter *StateQueryIterator) Next() (*queryresult.KV, error) {
	result, err := iter.nextResult(StateQueryResult)
	if err != nil {
		return nil, err
	}
	return result.(*queryresult.KV), err
}

// Next ...
func (iter *HistoryQueryIterator) Next() (*queryresult.KeyModification, error) {
	result, err := iter.nextResult(HistoryQueryResult)
	if err != nil {
		return nil, err
	}
	return result.(*queryresult.KeyModification), err
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
// query, history query). Note that queryResult is an empty golang
// interface that can hold values of any type.
func (iter *CommonIterator) getResultFromBytes(queryResultBytes *peer.QueryResultBytes,
	rType resultType) (queryResult, error) {

	if rType == StateQueryResult {
		stateQueryResult := &queryresult.KV{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, stateQueryResult); err != nil {
			return nil, fmt.Errorf("error unmarshaling result from bytes: %s", err)
		}
		return stateQueryResult, nil

	} else if rType == HistoryQueryResult {
		historyQueryResult := &queryresult.KeyModification{}
		if err := proto.Unmarshal(queryResultBytes.ResultBytes, historyQueryResult); err != nil {
			return nil, err
		}
		return historyQueryResult, nil
	}
	return nil, errors.New("wrong result type")
}

func (iter *CommonIterator) fetchNextQueryResult() error {
	response, err := iter.handler.handleQueryStateNext(iter.response.Id, iter.channelID, iter.txid)
	if err != nil {
		return err
	}
	iter.currentLoc = 0
	iter.response = response
	return nil
}

// nextResult returns the next QueryResult (i.e., either a KV struct or KeyModification)
// from the state or history query iterator. Note that queryResult is an
// empty golang interface that can hold values of any type.
func (iter *CommonIterator) nextResult(rType resultType) (queryResult, error) {
	if iter.currentLoc < len(iter.response.Results) {
		// On valid access of an element from cached results
		queryResult, err := iter.getResultFromBytes(iter.response.Results[iter.currentLoc], rType)
		if err != nil {
			return nil, err
		}
		iter.currentLoc++

		if iter.currentLoc == len(iter.response.Results) && iter.response.HasMore {
			// On access of last item, pre-fetch to update HasMore flag
			if err = iter.fetchNextQueryResult(); err != nil {
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
	_, err := iter.handler.handleQueryStateClose(iter.response.Id, iter.channelID, iter.txid)
	return err
}

// GetArgs documentation can be found in interfaces.go
func (s *ChaincodeStub) GetArgs() [][]byte {
	return s.args
}

// GetStringArgs documentation can be found in interfaces.go
func (s *ChaincodeStub) GetStringArgs() []string {
	args := s.GetArgs()
	strargs := make([]string, 0, len(args))
	for _, barg := range args {
		strargs = append(strargs, string(barg))
	}
	return strargs
}

// GetFunctionAndParameters documentation can be found in interfaces.go
func (s *ChaincodeStub) GetFunctionAndParameters() (function string, params []string) {
	allargs := s.GetStringArgs()
	function = ""
	params = []string{}
	if len(allargs) >= 1 {
		function = allargs[0]
		params = allargs[1:]
	}
	return
}

// GetCreator documentation can be found in interfaces.go
func (s *ChaincodeStub) GetCreator() ([]byte, error) {
	return s.creator, nil
}

// GetTransient documentation can be found in interfaces.go
func (s *ChaincodeStub) GetTransient() (map[string][]byte, error) {
	return s.transient, nil
}

// GetBinding documentation can be found in interfaces.go
func (s *ChaincodeStub) GetBinding() ([]byte, error) {
	return s.binding, nil
}

// GetSignedProposal documentation can be found in interfaces.go
func (s *ChaincodeStub) GetSignedProposal() (*peer.SignedProposal, error) {
	return s.signedProposal, nil
}

// GetArgsSlice documentation can be found in interfaces.go
func (s *ChaincodeStub) GetArgsSlice() ([]byte, error) {
	args := s.GetArgs()
	res := []byte{}
	for _, barg := range args {
		res = append(res, barg...)
	}
	return res, nil
}

// GetTxTimestamp documentation can be found in interfaces.go
func (s *ChaincodeStub) GetTxTimestamp() (*timestamppb.Timestamp, error) {
	hdr := &common.Header{}
	if err := proto.Unmarshal(s.proposal.Header, hdr); err != nil {
		return nil, fmt.Errorf("error unmarshaling Header: %s", err)
	}

	chdr := &common.ChannelHeader{}
	if err := proto.Unmarshal(hdr.ChannelHeader, chdr); err != nil {
		return nil, fmt.Errorf("error unmarshaling ChannelHeader: %s", err)
	}

	return chdr.GetTimestamp(), nil
}

// ------------- ChaincodeEvent API ----------------------

// SetEvent documentation can be found in interfaces.go
func (s *ChaincodeStub) SetEvent(name string, payload []byte) error {
	if name == "" {
		return errors.New("event name can not be empty string")
	}
	s.chaincodeEvent = &peer.ChaincodeEvent{EventName: name, Payload: payload}
	return nil
}
