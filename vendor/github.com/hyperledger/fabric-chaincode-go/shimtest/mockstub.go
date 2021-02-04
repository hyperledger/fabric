// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package shimtest provides a mock of the ChaincodeStubInterface for
// unit testing chaincode.
//
// Deprecated: ShimTest will be  removed in a future release.
// Future development should make use of the ChaincodeStub Interface
// for generating mocks
package shimtest

import (
	"container/list"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

const (
	minUnicodeRuneValue   = 0 //U+0000
	compositeKeyNamespace = "\x00"
)

// MockStub is an implementation of ChaincodeStubInterface for unit testing chaincode.
// Use this instead of ChaincodeStub in your chaincode's unit test calls to Init or Invoke.
type MockStub struct {
	// arguments the stub was called with
	args [][]byte

	// transientMap
	TransientMap map[string][]byte
	// A pointer back to the chaincode that will invoke this, set by constructor.
	// If a peer calls this stub, the chaincode will be invoked from here.
	cc shim.Chaincode

	// A nice name that can be used for logging
	Name string

	// State keeps name value pairs
	State map[string][]byte

	// Keys stores the list of mapped values in lexical order
	Keys *list.List

	// registered list of other MockStub chaincodes that can be called from this MockStub
	Invokables map[string]*MockStub

	// stores a transaction uuid while being Invoked / Deployed
	// TODO if a chaincode uses recursion this may need to be a stack of TxIDs or possibly a reference counting map
	TxID string

	TxTimestamp *timestamp.Timestamp

	// mocked signedProposal
	signedProposal *pb.SignedProposal

	// stores a channel ID of the proposal
	ChannelID string

	PvtState map[string]map[string][]byte

	// stores per-key endorsement policy, first map index is the collection, second map index is the key
	EndorsementPolicies map[string]map[string][]byte

	// channel to store ChaincodeEvents
	ChaincodeEventsChannel chan *pb.ChaincodeEvent

	Creator []byte

	Decorations map[string][]byte
}

// GetTxID ...
func (stub *MockStub) GetTxID() string {
	return stub.TxID
}

// GetChannelID ...
func (stub *MockStub) GetChannelID() string {
	return stub.ChannelID
}

// GetArgs ...
func (stub *MockStub) GetArgs() [][]byte {
	return stub.args
}

// GetStringArgs ...
func (stub *MockStub) GetStringArgs() []string {
	args := stub.GetArgs()
	strargs := make([]string, 0, len(args))
	for _, barg := range args {
		strargs = append(strargs, string(barg))
	}
	return strargs
}

// GetFunctionAndParameters ...
func (stub *MockStub) GetFunctionAndParameters() (function string, params []string) {
	allargs := stub.GetStringArgs()
	function = ""
	params = []string{}
	if len(allargs) >= 1 {
		function = allargs[0]
		params = allargs[1:]
	}
	return
}

// MockTransactionStart Used to indicate to a chaincode that it is part of a transaction.
// This is important when chaincodes invoke each other.
// MockStub doesn't support concurrent transactions at present.
func (stub *MockStub) MockTransactionStart(txid string) {
	stub.TxID = txid
	stub.setSignedProposal(&pb.SignedProposal{})
	stub.setTxTimestamp(ptypes.TimestampNow())
}

// MockTransactionEnd End a mocked transaction, clearing the UUID.
func (stub *MockStub) MockTransactionEnd(uuid string) {
	stub.signedProposal = nil
	stub.TxID = ""
}

// MockPeerChaincode Register another MockStub chaincode with this MockStub.
// invokableChaincodeName is the name of a chaincode.
// otherStub is a MockStub of the chaincode, already initialized.
// channel is the name of a channel on which another MockStub is called.
func (stub *MockStub) MockPeerChaincode(invokableChaincodeName string, otherStub *MockStub, channel string) {
	// Internally we use chaincode name as a composite name
	if channel != "" {
		invokableChaincodeName = invokableChaincodeName + "/" + channel
	}
	stub.Invokables[invokableChaincodeName] = otherStub
}

// MockInit Initialise this chaincode,  also starts and ends a transaction.
func (stub *MockStub) MockInit(uuid string, args [][]byte) pb.Response {
	stub.args = args
	stub.MockTransactionStart(uuid)
	res := stub.cc.Init(stub)
	stub.MockTransactionEnd(uuid)
	return res
}

// MockInvoke Invoke this chaincode, also starts and ends a transaction.
func (stub *MockStub) MockInvoke(uuid string, args [][]byte) pb.Response {
	stub.args = args
	stub.MockTransactionStart(uuid)
	res := stub.cc.Invoke(stub)
	stub.MockTransactionEnd(uuid)
	return res
}

// GetDecorations ...
func (stub *MockStub) GetDecorations() map[string][]byte {
	return stub.Decorations
}

// MockInvokeWithSignedProposal Invoke this chaincode, also starts and ends a transaction.
func (stub *MockStub) MockInvokeWithSignedProposal(uuid string, args [][]byte, sp *pb.SignedProposal) pb.Response {
	stub.args = args
	stub.MockTransactionStart(uuid)
	stub.signedProposal = sp
	res := stub.cc.Invoke(stub)
	stub.MockTransactionEnd(uuid)
	return res
}

// GetPrivateData ...
func (stub *MockStub) GetPrivateData(collection string, key string) ([]byte, error) {
	m, in := stub.PvtState[collection]

	if !in {
		return nil, nil
	}

	return m[key], nil
}

// GetPrivateDataHash ...
func (stub *MockStub) GetPrivateDataHash(collection, key string) ([]byte, error) {
	return nil, errors.New("Not Implemented")
}

// PutPrivateData ...
func (stub *MockStub) PutPrivateData(collection string, key string, value []byte) error {
	m, in := stub.PvtState[collection]
	if !in {
		stub.PvtState[collection] = make(map[string][]byte)
		m, in = stub.PvtState[collection]
	}

	m[key] = value

	return nil
}

// DelPrivateData ...
func (stub *MockStub) DelPrivateData(collection string, key string) error {
	return errors.New("Not Implemented")
}

// GetPrivateDataByRange ...
func (stub *MockStub) GetPrivateDataByRange(collection, startKey, endKey string) (shim.StateQueryIteratorInterface, error) {
	return nil, errors.New("Not Implemented")
}

// GetPrivateDataByPartialCompositeKey ...
func (stub *MockStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, attributes []string) (shim.StateQueryIteratorInterface, error) {
	return nil, errors.New("Not Implemented")
}

// GetPrivateDataQueryResult ...
func (stub *MockStub) GetPrivateDataQueryResult(collection, query string) (shim.StateQueryIteratorInterface, error) {
	// Not implemented since the mock engine does not have a query engine.
	// However, a very simple query engine that supports string matching
	// could be implemented to test that the framework supports queries
	return nil, errors.New("Not Implemented")
}

// GetState retrieves the value for a given key from the ledger
func (stub *MockStub) GetState(key string) ([]byte, error) {
	value := stub.State[key]
	return value, nil
}

// PutState writes the specified `value` and `key` into the ledger.
func (stub *MockStub) PutState(key string, value []byte) error {
	if stub.TxID == "" {
		err := errors.New("cannot PutState without a transactions - call stub.MockTransactionStart()?")
		return err
	}

	// If the value is nil or empty, delete the key
	if len(value) == 0 {
		return stub.DelState(key)
	}
	stub.State[key] = value

	// insert key into ordered list of keys
	for elem := stub.Keys.Front(); elem != nil; elem = elem.Next() {
		elemValue := elem.Value.(string)
		comp := strings.Compare(key, elemValue)
		if comp < 0 {
			// key < elem, insert it before elem
			stub.Keys.InsertBefore(key, elem)
			break
		} else if comp == 0 {
			// keys exists, no need to change
			break
		} else { // comp > 0
			// key > elem, keep looking unless this is the end of the list
			if elem.Next() == nil {
				stub.Keys.PushBack(key)
				break
			}
		}
	}

	// special case for empty Keys list
	if stub.Keys.Len() == 0 {
		stub.Keys.PushFront(key)
	}

	return nil
}

// DelState removes the specified `key` and its value from the ledger.
func (stub *MockStub) DelState(key string) error {
	delete(stub.State, key)

	for elem := stub.Keys.Front(); elem != nil; elem = elem.Next() {
		if strings.Compare(key, elem.Value.(string)) == 0 {
			stub.Keys.Remove(elem)
		}
	}

	return nil
}

// GetStateByRange ...
func (stub *MockStub) GetStateByRange(startKey, endKey string) (shim.StateQueryIteratorInterface, error) {
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	return NewMockStateRangeQueryIterator(stub, startKey, endKey), nil
}

//To ensure that simple keys do not go into composite key namespace,
//we validate simplekey to check whether the key starts with 0x00 (which
//is the namespace for compositeKey). This helps in avoding simple/composite
//key collisions.
func validateSimpleKeys(simpleKeys ...string) error {
	for _, key := range simpleKeys {
		if len(key) > 0 && key[0] == compositeKeyNamespace[0] {
			return fmt.Errorf(`first character of the key [%s] contains a null character which is not allowed`, key)
		}
	}
	return nil
}

// GetQueryResult function can be invoked by a chaincode to perform a
// rich query against state database.  Only supported by state database implementations
// that support rich query.  The query string is in the syntax of the underlying
// state database. An iterator is returned which can be used to iterate (next) over
// the query result set
func (stub *MockStub) GetQueryResult(query string) (shim.StateQueryIteratorInterface, error) {
	// Not implemented since the mock engine does not have a query engine.
	// However, a very simple query engine that supports string matching
	// could be implemented to test that the framework supports queries
	return nil, errors.New("not implemented")
}

// GetHistoryForKey function can be invoked by a chaincode to return a history of
// key values across time. GetHistoryForKey is intended to be used for read-only queries.
func (stub *MockStub) GetHistoryForKey(key string) (shim.HistoryQueryIteratorInterface, error) {
	return nil, errors.New("not implemented")
}

// GetStateByPartialCompositeKey function can be invoked by a chaincode to query the
// state based on a given partial composite key. This function returns an
// iterator which can be used to iterate over all composite keys whose prefix
// matches the given partial composite key. This function should be used only for
// a partial composite key. For a full composite key, an iter with empty response
// would be returned.
func (stub *MockStub) GetStateByPartialCompositeKey(objectType string, attributes []string) (shim.StateQueryIteratorInterface, error) {
	partialCompositeKey, err := stub.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	return NewMockStateRangeQueryIterator(stub, partialCompositeKey, partialCompositeKey+string(utf8.MaxRune)), nil
}

// CreateCompositeKey combines the list of attributes
// to form a composite key.
func (stub *MockStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return shim.CreateCompositeKey(objectType, attributes)
}

// SplitCompositeKey splits the composite key into attributes
// on which the composite key was formed.
func (stub *MockStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return splitCompositeKey(compositeKey)
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

// GetStateByRangeWithPagination ...
func (stub *MockStub) GetStateByRangeWithPagination(startKey, endKey string, pageSize int32,
	bookmark string) (shim.StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// GetStateByPartialCompositeKeyWithPagination ...
func (stub *MockStub) GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string,
	pageSize int32, bookmark string) (shim.StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// GetQueryResultWithPagination ...
func (stub *MockStub) GetQueryResultWithPagination(query string, pageSize int32,
	bookmark string) (shim.StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// InvokeChaincode locally calls the specified chaincode `Invoke`.
// E.g. stub1.InvokeChaincode("othercc", funcArgs, channel)
// Before calling this make sure to create another MockStub stub2, call shim.NewMockStub("othercc", Chaincode)
// and register it with stub1 by calling stub1.MockPeerChaincode("othercc", stub2, channel)
func (stub *MockStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response {
	// Internally we use chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	// TODO "args" here should possibly be a serialized pb.ChaincodeInput
	otherStub := stub.Invokables[chaincodeName]
	//	function, strings := getFuncArgs(args)
	res := otherStub.MockInvoke(stub.TxID, args)
	return res
}

// GetCreator ...
func (stub *MockStub) GetCreator() ([]byte, error) {
	return stub.Creator, nil
}

// SetTransient set TransientMap to mockStub
func (stub *MockStub) SetTransient(tMap map[string][]byte) error {
	if stub.signedProposal == nil {
		return fmt.Errorf("signedProposal is not initialized")
	}
	payloadByte, err := proto.Marshal(&pb.ChaincodeProposalPayload{
		TransientMap: tMap,
	})
	if err != nil {
		return err
	}
	proposalByte, err := proto.Marshal(&pb.Proposal{
		Payload: payloadByte,
	})
	if err != nil {
		return err
	}
	stub.signedProposal.ProposalBytes = proposalByte
	stub.TransientMap = tMap
	return nil
}

// GetTransient ...
func (stub *MockStub) GetTransient() (map[string][]byte, error) {
	return stub.TransientMap, nil
}

// GetBinding Not implemented ...
func (stub *MockStub) GetBinding() ([]byte, error) {
	return nil, nil
}

// GetSignedProposal Not implemented ...
func (stub *MockStub) GetSignedProposal() (*pb.SignedProposal, error) {
	return stub.signedProposal, nil
}

func (stub *MockStub) setSignedProposal(sp *pb.SignedProposal) {
	stub.signedProposal = sp
}

// GetArgsSlice Not implemented ...
func (stub *MockStub) GetArgsSlice() ([]byte, error) {
	return nil, nil
}

func (stub *MockStub) setTxTimestamp(time *timestamp.Timestamp) {
	stub.TxTimestamp = time
}

// GetTxTimestamp ...
func (stub *MockStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	if stub.TxTimestamp == nil {
		return nil, errors.New("TxTimestamp not set")
	}
	return stub.TxTimestamp, nil
}

// SetEvent ...
func (stub *MockStub) SetEvent(name string, payload []byte) error {
	stub.ChaincodeEventsChannel <- &pb.ChaincodeEvent{EventName: name, Payload: payload}
	return nil
}

// SetStateValidationParameter ...
func (stub *MockStub) SetStateValidationParameter(key string, ep []byte) error {
	return stub.SetPrivateDataValidationParameter("", key, ep)
}

// GetStateValidationParameter ...
func (stub *MockStub) GetStateValidationParameter(key string) ([]byte, error) {
	return stub.GetPrivateDataValidationParameter("", key)
}

// SetPrivateDataValidationParameter ...
func (stub *MockStub) SetPrivateDataValidationParameter(collection, key string, ep []byte) error {
	m, in := stub.EndorsementPolicies[collection]
	if !in {
		stub.EndorsementPolicies[collection] = make(map[string][]byte)
		m, in = stub.EndorsementPolicies[collection]
	}

	m[key] = ep
	return nil
}

// GetPrivateDataValidationParameter ...
func (stub *MockStub) GetPrivateDataValidationParameter(collection, key string) ([]byte, error) {
	m, in := stub.EndorsementPolicies[collection]

	if !in {
		return nil, nil
	}

	return m[key], nil
}

// NewMockStub Constructor to initialise the internal State map
func NewMockStub(name string, cc shim.Chaincode) *MockStub {
	s := new(MockStub)
	s.Name = name
	s.cc = cc
	s.State = make(map[string][]byte)
	s.PvtState = make(map[string]map[string][]byte)
	s.EndorsementPolicies = make(map[string]map[string][]byte)
	s.Invokables = make(map[string]*MockStub)
	s.Keys = list.New()
	s.ChaincodeEventsChannel = make(chan *pb.ChaincodeEvent, 100) //define large capacity for non-blocking setEvent calls.
	s.Decorations = make(map[string][]byte)

	return s
}

/*****************************
 Range Query Iterator
*****************************/

// MockStateRangeQueryIterator ...
type MockStateRangeQueryIterator struct {
	Closed   bool
	Stub     *MockStub
	StartKey string
	EndKey   string
	Current  *list.Element
}

// HasNext returns true if the range query iterator contains additional keys
// and values.
func (iter *MockStateRangeQueryIterator) HasNext() bool {
	if iter.Closed {
		// previously called Close()
		return false
	}

	if iter.Current == nil {
		return false
	}

	current := iter.Current
	for current != nil {
		// if this is an open-ended query for all keys, return true
		if iter.StartKey == "" && iter.EndKey == "" {
			return true
		}
		comp1 := strings.Compare(current.Value.(string), iter.StartKey)
		comp2 := strings.Compare(current.Value.(string), iter.EndKey)
		if comp1 >= 0 {
			if comp2 < 0 {
				return true
			}
			return false
		}
		current = current.Next()
	}
	return false
}

// Next returns the next key and value in the range query iterator.
func (iter *MockStateRangeQueryIterator) Next() (*queryresult.KV, error) {
	if iter.Closed == true {
		err := errors.New("MockStateRangeQueryIterator.Next() called after Close()")
		return nil, err
	}

	if iter.HasNext() == false {
		err := errors.New("MockStateRangeQueryIterator.Next() called when it does not HaveNext()")
		return nil, err
	}

	for iter.Current != nil {
		comp1 := strings.Compare(iter.Current.Value.(string), iter.StartKey)
		comp2 := strings.Compare(iter.Current.Value.(string), iter.EndKey)
		// compare to start and end keys. or, if this is an open-ended query for
		// all keys, it should always return the key and value
		if (comp1 >= 0 && comp2 < 0) || (iter.StartKey == "" && iter.EndKey == "") {
			key := iter.Current.Value.(string)
			value, err := iter.Stub.GetState(key)
			iter.Current = iter.Current.Next()
			return &queryresult.KV{Key: key, Value: value}, err
		}
		iter.Current = iter.Current.Next()
	}
	err := errors.New("MockStateRangeQueryIterator.Next() went past end of range")
	return nil, err
}

// Close closes the range query iterator. This should be called when done
// reading from the iterator to free up resources.
func (iter *MockStateRangeQueryIterator) Close() error {
	if iter.Closed == true {
		err := errors.New("MockStateRangeQueryIterator.Close() called after Close()")
		return err
	}

	iter.Closed = true
	return nil
}

// NewMockStateRangeQueryIterator ...
func NewMockStateRangeQueryIterator(stub *MockStub, startKey string, endKey string) *MockStateRangeQueryIterator {
	iter := new(MockStateRangeQueryIterator)
	iter.Closed = false
	iter.Stub = stub
	iter.StartKey = startKey
	iter.EndKey = endKey
	iter.Current = stub.Keys.Front()
	return iter
}

func getBytes(function string, args []string) [][]byte {
	bytes := make([][]byte, 0, len(args)+1)
	bytes = append(bytes, []byte(function))
	for _, s := range args {
		bytes = append(bytes, []byte(s))
	}
	return bytes
}

func getFuncArgs(bytes [][]byte) (string, []string) {
	function := string(bytes[0])
	args := make([]string, len(bytes)-1)
	for i := 1; i < len(bytes); i++ {
		args[i-1] = string(bytes[i])
	}
	return function, args
}
