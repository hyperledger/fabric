/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package shim provides APIs for the chaincode to access its state
// variables, transaction context and call other chaincodes.
package shim

import (
	"container/list"
	"fmt"
	"strings"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

// Logger for the shim package.
var mockLogger = logging.MustGetLogger("mock")

// MockStub is an implementation of ChaincodeStubInterface for unit testing chaincode.
// Use this instead of ChaincodeStub in your chaincode's unit test calls to Init or Invoke.
type MockStub struct {
	// arguments the stub was called with
	args [][]byte

	// A pointer back to the chaincode that will invoke this, set by constructor.
	// If a peer calls this stub, the chaincode will be invoked from here.
	cc Chaincode

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

	Decorations map[string][]byte
}

func (stub *MockStub) GetTxID() string {
	return stub.TxID
}

func (stub *MockStub) GetChannelID() string {
	return stub.ChannelID
}

func (stub *MockStub) GetArgs() [][]byte {
	return stub.args
}

func (stub *MockStub) GetStringArgs() []string {
	args := stub.GetArgs()
	strargs := make([]string, 0, len(args))
	for _, barg := range args {
		strargs = append(strargs, string(barg))
	}
	return strargs
}

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

// Used to indicate to a chaincode that it is part of a transaction.
// This is important when chaincodes invoke each other.
// MockStub doesn't support concurrent transactions at present.
func (stub *MockStub) MockTransactionStart(txid string) {
	stub.TxID = txid
	stub.setSignedProposal(&pb.SignedProposal{})
	stub.setTxTimestamp(util.CreateUtcTimestamp())
}

// End a mocked transaction, clearing the UUID.
func (stub *MockStub) MockTransactionEnd(uuid string) {
	stub.signedProposal = nil
	stub.TxID = ""
}

// Register a peer chaincode with this MockStub
// invokableChaincodeName is the name or hash of the peer
// otherStub is a MockStub of the peer, already intialised
func (stub *MockStub) MockPeerChaincode(invokableChaincodeName string, otherStub *MockStub) {
	stub.Invokables[invokableChaincodeName] = otherStub
}

// Initialise this chaincode,  also starts and ends a transaction.
func (stub *MockStub) MockInit(uuid string, args [][]byte) pb.Response {
	stub.args = args
	stub.MockTransactionStart(uuid)
	res := stub.cc.Init(stub)
	stub.MockTransactionEnd(uuid)
	return res
}

// Invoke this chaincode, also starts and ends a transaction.
func (stub *MockStub) MockInvoke(uuid string, args [][]byte) pb.Response {
	stub.args = args
	stub.MockTransactionStart(uuid)
	res := stub.cc.Invoke(stub)
	stub.MockTransactionEnd(uuid)
	return res
}

func (stub *MockStub) GetDecorations() map[string][]byte {
	return stub.Decorations
}

// Invoke this chaincode, also starts and ends a transaction.
func (stub *MockStub) MockInvokeWithSignedProposal(uuid string, args [][]byte, sp *pb.SignedProposal) pb.Response {
	stub.args = args
	stub.MockTransactionStart(uuid)
	stub.signedProposal = sp
	res := stub.cc.Invoke(stub)
	stub.MockTransactionEnd(uuid)
	return res
}

func (stub *MockStub) GetPrivateData(collection string, key string) ([]byte, error) {
	m, in := stub.PvtState[collection]

	if !in {
		return nil, nil
	}

	return m[key], nil
}

func (stub *MockStub) GetPrivateDataHash(collection, key string) ([]byte, error) {
	return nil, errors.New("Not Implemented")
}

func (stub *MockStub) PutPrivateData(collection string, key string, value []byte) error {
	m, in := stub.PvtState[collection]
	if !in {
		stub.PvtState[collection] = make(map[string][]byte)
		m, in = stub.PvtState[collection]
	}

	m[key] = value

	return nil
}

func (stub *MockStub) DelPrivateData(collection string, key string) error {
	return errors.New("Not Implemented")
}

func (stub *MockStub) GetPrivateDataByRange(collection, startKey, endKey string) (StateQueryIteratorInterface, error) {
	return nil, errors.New("Not Implemented")
}

func (stub *MockStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	return nil, errors.New("Not Implemented")
}

func (stub *MockStub) GetPrivateDataQueryResult(collection, query string) (StateQueryIteratorInterface, error) {
	// Not implemented since the mock engine does not have a query engine.
	// However, a very simple query engine that supports string matching
	// could be implemented to test that the framework supports queries
	return nil, errors.New("Not Implemented")
}

// GetState retrieves the value for a given key from the ledger
func (stub *MockStub) GetState(key string) ([]byte, error) {
	value := stub.State[key]
	mockLogger.Debug("MockStub", stub.Name, "Getting", key, value)
	return value, nil
}

// PutState writes the specified `value` and `key` into the ledger.
func (stub *MockStub) PutState(key string, value []byte) error {
	if stub.TxID == "" {
		err := errors.New("cannot PutState without a transactions - call stub.MockTransactionStart()?")
		mockLogger.Errorf("%+v", err)
		return err
	}

	// If the value is nil or empty, delete the key
	if len(value) == 0 {
		mockLogger.Debug("MockStub", stub.Name, "PutState called, but value is nil or empty. Delete ", key)
		return stub.DelState(key)
	}

	mockLogger.Debug("MockStub", stub.Name, "Putting", key, value)
	stub.State[key] = value

	// insert key into ordered list of keys
	for elem := stub.Keys.Front(); elem != nil; elem = elem.Next() {
		elemValue := elem.Value.(string)
		comp := strings.Compare(key, elemValue)
		mockLogger.Debug("MockStub", stub.Name, "Compared", key, elemValue, " and got ", comp)
		if comp < 0 {
			// key < elem, insert it before elem
			stub.Keys.InsertBefore(key, elem)
			mockLogger.Debug("MockStub", stub.Name, "Key", key, " inserted before", elem.Value)
			break
		} else if comp == 0 {
			// keys exists, no need to change
			mockLogger.Debug("MockStub", stub.Name, "Key", key, "already in State")
			break
		} else { // comp > 0
			// key > elem, keep looking unless this is the end of the list
			if elem.Next() == nil {
				stub.Keys.PushBack(key)
				mockLogger.Debug("MockStub", stub.Name, "Key", key, "appended")
				break
			}
		}
	}

	// special case for empty Keys list
	if stub.Keys.Len() == 0 {
		stub.Keys.PushFront(key)
		mockLogger.Debug("MockStub", stub.Name, "Key", key, "is first element in list")
	}

	return nil
}

// DelState removes the specified `key` and its value from the ledger.
func (stub *MockStub) DelState(key string) error {
	mockLogger.Debug("MockStub", stub.Name, "Deleting", key, stub.State[key])
	delete(stub.State, key)

	for elem := stub.Keys.Front(); elem != nil; elem = elem.Next() {
		if strings.Compare(key, elem.Value.(string)) == 0 {
			stub.Keys.Remove(elem)
		}
	}

	return nil
}

func (stub *MockStub) GetStateByRange(startKey, endKey string) (StateQueryIteratorInterface, error) {
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	return NewMockStateRangeQueryIterator(stub, startKey, endKey), nil
}

// GetQueryResult function can be invoked by a chaincode to perform a
// rich query against state database.  Only supported by state database implementations
// that support rich query.  The query string is in the syntax of the underlying
// state database. An iterator is returned which can be used to iterate (next) over
// the query result set
func (stub *MockStub) GetQueryResult(query string) (StateQueryIteratorInterface, error) {
	// Not implemented since the mock engine does not have a query engine.
	// However, a very simple query engine that supports string matching
	// could be implemented to test that the framework supports queries
	return nil, errors.New("not implemented")
}

// GetHistoryForKey function can be invoked by a chaincode to return a history of
// key values across time. GetHistoryForKey is intended to be used for read-only queries.
func (stub *MockStub) GetHistoryForKey(key string) (HistoryQueryIteratorInterface, error) {
	return nil, errors.New("not implemented")
}

//GetStateByPartialCompositeKey function can be invoked by a chaincode to query the
//state based on a given partial composite key. This function returns an
//iterator which can be used to iterate over all composite keys whose prefix
//matches the given partial composite key. This function should be used only for
//a partial composite key. For a full composite key, an iter with empty response
//would be returned.
func (stub *MockStub) GetStateByPartialCompositeKey(objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	partialCompositeKey, err := stub.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return nil, err
	}
	return NewMockStateRangeQueryIterator(stub, partialCompositeKey, partialCompositeKey+string(maxUnicodeRuneValue)), nil
}

// CreateCompositeKey combines the list of attributes
//to form a composite key.
func (stub *MockStub) CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return createCompositeKey(objectType, attributes)
}

// SplitCompositeKey splits the composite key into attributes
// on which the composite key was formed.
func (stub *MockStub) SplitCompositeKey(compositeKey string) (string, []string, error) {
	return splitCompositeKey(compositeKey)
}

func (stub *MockStub) GetStateByRangeWithPagination(startKey, endKey string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	return nil, nil, nil
}

func (stub *MockStub) GetStateByPartialCompositeKeyWithPagination(objectType string, keys []string,
	pageSize int32, bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	return nil, nil, nil
}

func (stub *MockStub) GetQueryResultWithPagination(query string, pageSize int32,
	bookmark string) (StateQueryIteratorInterface, *pb.QueryResponseMetadata, error) {
	return nil, nil, nil
}

// InvokeChaincode calls a peered chaincode.
// E.g. stub1.InvokeChaincode("stub2Hash", funcArgs, channel)
// Before calling this make sure to create another MockStub stub2, call stub2.MockInit(uuid, func, args)
// and register it with stub1 by calling stub1.MockPeerChaincode("stub2Hash", stub2)
func (stub *MockStub) InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response {
	// Internally we use chaincode name as a composite name
	if channel != "" {
		chaincodeName = chaincodeName + "/" + channel
	}
	// TODO "args" here should possibly be a serialized pb.ChaincodeInput
	otherStub := stub.Invokables[chaincodeName]
	mockLogger.Debug("MockStub", stub.Name, "Invoking peer chaincode", otherStub.Name, args)
	//	function, strings := getFuncArgs(args)
	res := otherStub.MockInvoke(stub.TxID, args)
	mockLogger.Debug("MockStub", stub.Name, "Invoked peer chaincode", otherStub.Name, "got", fmt.Sprintf("%+v", res))
	return res
}

// Not implemented
func (stub *MockStub) GetCreator() ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetTransient() (map[string][]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetBinding() ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetSignedProposal() (*pb.SignedProposal, error) {
	return stub.signedProposal, nil
}

func (stub *MockStub) setSignedProposal(sp *pb.SignedProposal) {
	stub.signedProposal = sp
}

// Not implemented
func (stub *MockStub) GetArgsSlice() ([]byte, error) {
	return nil, nil
}

func (stub *MockStub) setTxTimestamp(time *timestamp.Timestamp) {
	stub.TxTimestamp = time
}

func (stub *MockStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	if stub.TxTimestamp == nil {
		return nil, errors.New("TxTimestamp not set.")
	}
	return stub.TxTimestamp, nil
}

func (stub *MockStub) SetEvent(name string, payload []byte) error {
	stub.ChaincodeEventsChannel <- &pb.ChaincodeEvent{EventName: name, Payload: payload}
	return nil
}

func (stub *MockStub) SetStateValidationParameter(key string, ep []byte) error {
	return stub.SetPrivateDataValidationParameter("", key, ep)
}

func (stub *MockStub) GetStateValidationParameter(key string) ([]byte, error) {
	return stub.GetPrivateDataValidationParameter("", key)
}

func (stub *MockStub) SetPrivateDataValidationParameter(collection, key string, ep []byte) error {
	m, in := stub.EndorsementPolicies[collection]
	if !in {
		stub.EndorsementPolicies[collection] = make(map[string][]byte)
		m, in = stub.EndorsementPolicies[collection]
	}

	m[key] = ep
	return nil
}

func (stub *MockStub) GetPrivateDataValidationParameter(collection, key string) ([]byte, error) {
	m, in := stub.EndorsementPolicies[collection]

	if !in {
		return nil, nil
	}

	return m[key], nil
}

// Constructor to initialise the internal State map
func NewMockStub(name string, cc Chaincode) *MockStub {
	mockLogger.Debug("MockStub(", name, cc, ")")
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
		mockLogger.Debug("HasNext() but already closed")
		return false
	}

	if iter.Current == nil {
		mockLogger.Error("HasNext() couldn't get Current")
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
				mockLogger.Debug("HasNext() got next")
				return true
			} else {
				mockLogger.Debug("HasNext() but no next")
				return false

			}
		}
		current = current.Next()
	}

	// we've reached the end of the underlying values
	mockLogger.Debug("HasNext() but no next")
	return false
}

// Next returns the next key and value in the range query iterator.
func (iter *MockStateRangeQueryIterator) Next() (*queryresult.KV, error) {
	if iter.Closed == true {
		err := errors.New("MockStateRangeQueryIterator.Next() called after Close()")
		mockLogger.Errorf("%+v", err)
		return nil, err
	}

	if iter.HasNext() == false {
		err := errors.New("MockStateRangeQueryIterator.Next() called when it does not HaveNext()")
		mockLogger.Errorf("%+v", err)
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
	mockLogger.Errorf("%+v", err)
	return nil, err
}

// Close closes the range query iterator. This should be called when done
// reading from the iterator to free up resources.
func (iter *MockStateRangeQueryIterator) Close() error {
	if iter.Closed == true {
		err := errors.New("MockStateRangeQueryIterator.Close() called after Close()")
		mockLogger.Errorf("%+v", err)
		return err
	}

	iter.Closed = true
	return nil
}

func (iter *MockStateRangeQueryIterator) Print() {
	mockLogger.Debug("MockStateRangeQueryIterator {")
	mockLogger.Debug("Closed?", iter.Closed)
	mockLogger.Debug("Stub", iter.Stub)
	mockLogger.Debug("StartKey", iter.StartKey)
	mockLogger.Debug("EndKey", iter.EndKey)
	mockLogger.Debug("Current", iter.Current)
	mockLogger.Debug("HasNext?", iter.HasNext())
	mockLogger.Debug("}")
}

func NewMockStateRangeQueryIterator(stub *MockStub, startKey string, endKey string) *MockStateRangeQueryIterator {
	mockLogger.Debug("NewMockStateRangeQueryIterator(", stub, startKey, endKey, ")")
	iter := new(MockStateRangeQueryIterator)
	iter.Closed = false
	iter.Stub = stub
	iter.StartKey = startKey
	iter.EndKey = endKey
	iter.Current = stub.Keys.Front()

	iter.Print()

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
	mockLogger.Debugf("getFuncArgs(%x)", bytes)
	function := string(bytes[0])
	args := make([]string, len(bytes)-1)
	for i := 1; i < len(bytes); i++ {
		mockLogger.Debugf("getFuncArgs - i:%x, len(bytes):%x", i, len(bytes))
		args[i-1] = string(bytes[i])
	}
	return function, args
}
