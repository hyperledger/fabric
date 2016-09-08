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

// Package shim provides APIs for the chaincode to access its state
// variables, transaction context and call other chaincodes.
package shim

import (
	"container/list"
	"errors"
	"strings"

	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/chaincode/shim/crypto/attr"
	"github.com/op/go-logging"
)

// Logger for the shim package.
var mockLogger = logging.MustGetLogger("mock")

// MockStub is an implementation of ChaincodeStubInterface for unit testing chaincode.
// Use this instead of ChaincodeStub in your chaincode's unit test calls to Init, Query or Invoke.
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
}

func (stub *MockStub) GetTxID() string {
	return stub.TxID
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

// Used to indicate to a chaincode that it is part of a transaction.
// This is important when chaincodes invoke each other.
// MockStub doesn't support concurrent transactions at present.
func (stub *MockStub) MockTransactionStart(txid string) {
	stub.TxID = txid
}

// End a mocked transaction, clearing the UUID.
func (stub *MockStub) MockTransactionEnd(uuid string) {
	stub.TxID = ""
}

// Register a peer chaincode with this MockStub
// invokableChaincodeName is the name or hash of the peer
// otherStub is a MockStub of the peer, already intialised
func (stub *MockStub) MockPeerChaincode(invokableChaincodeName string, otherStub *MockStub) {
	stub.Invokables[invokableChaincodeName] = otherStub
}

// Initialise this chaincode,  also starts and ends a transaction.
func (stub *MockStub) MockInit(uuid string, function string, args []string) ([]byte, error) {
	stub.args = getBytes(function, args)
	stub.MockTransactionStart(uuid)
	bytes, err := stub.cc.Init(stub, function, args)
	stub.MockTransactionEnd(uuid)
	return bytes, err
}

// Invoke this chaincode, also starts and ends a transaction.
func (stub *MockStub) MockInvoke(uuid string, function string, args []string) ([]byte, error) {
	stub.args = getBytes(function, args)
	stub.MockTransactionStart(uuid)
	bytes, err := stub.cc.Invoke(stub, function, args)
	stub.MockTransactionEnd(uuid)
	return bytes, err
}

// Query this chaincode
func (stub *MockStub) MockQuery(function string, args []string) ([]byte, error) {
	stub.args = getBytes(function, args)
	// no transaction needed for queries
	bytes, err := stub.cc.Query(stub, function, args)
	return bytes, err
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
		mockLogger.Error("Cannot PutState without a transactions - call stub.MockTransactionStart()?")
		return errors.New("Cannot PutState without a transactions - call stub.MockTransactionStart()?")
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

func (stub *MockStub) RangeQueryState(startKey, endKey string) (StateRangeQueryIteratorInterface, error) {
	return NewMockStateRangeQueryIterator(stub, startKey, endKey), nil
}

// Not implemented
func (stub *MockStub) CreateTable(name string, columnDefinitions []*ColumnDefinition) error {
	return nil
}

// Not implemented
func (stub *MockStub) GetTable(tableName string) (*Table, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) DeleteTable(tableName string) error {
	return nil
}

// Not implemented
func (stub *MockStub) InsertRow(tableName string, row Row) (bool, error) {
	return false, nil
}

// Not implemented
func (stub *MockStub) ReplaceRow(tableName string, row Row) (bool, error) {
	return false, nil
}

// Not implemented
func (stub *MockStub) GetRow(tableName string, key []Column) (Row, error) {
	var r Row
	return r, nil
}

// Not implemented
func (stub *MockStub) GetRows(tableName string, key []Column) (<-chan Row, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) DeleteRow(tableName string, key []Column) error {
	return nil
}

// Invokes a peered chaincode.
// E.g. stub1.InvokeChaincode("stub2Hash", funcArgs)
// Before calling this make sure to create another MockStub stub2, call stub2.MockInit(uuid, func, args)
// and register it with stub1 by calling stub1.MockPeerChaincode("stub2Hash", stub2)
func (stub *MockStub) InvokeChaincode(chaincodeName string, args [][]byte) ([]byte, error) {
	// TODO "args" here should possibly be a serialized pb.ChaincodeInput
	function, params := getFuncArgs(args)
	otherStub := stub.Invokables[chaincodeName]
	mockLogger.Debug("MockStub", stub.Name, "Invoking peer chaincode", otherStub.Name, args)
	//	function, strings := getFuncArgs(args)
	bytes, err := otherStub.MockInvoke(stub.TxID, function, params)
	mockLogger.Debug("MockStub", stub.Name, "Invoked peer chaincode", otherStub.Name, "got", bytes, err)
	return bytes, err
}

func (stub *MockStub) QueryChaincode(chaincodeName string, args [][]byte) ([]byte, error) {
	// TODO "args" here should possibly be a serialized pb.ChaincodeInput
	mockLogger.Debug("MockStub", stub.Name, "Looking for peer chaincode", chaincodeName)
	otherStub := stub.Invokables[chaincodeName]
	if otherStub == nil {
		mockLogger.Error("Could not find peer chaincode to query", chaincodeName)
		return nil, errors.New("Could not find peer chaincode to query")
	}
	mockLogger.Debug("MockStub", stub.Name, "Querying peer chaincode", otherStub.Name, args)
	function, params := getFuncArgs(args)
	bytes, err := otherStub.MockQuery(function, params)
	mockLogger.Debug("MockStub", stub.Name, "Queried peer chaincode", otherStub.Name, "got", bytes, err)
	return bytes, err
}

// Not implemented
func (stub *MockStub) ReadCertAttribute(attributeName string) ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) VerifyAttribute(attributeName string, attributeValue []byte) (bool, error) {
	return false, nil
}

// Not implemented
func (stub *MockStub) VerifyAttributes(attrs ...*attr.Attribute) (bool, error) {
	return false, nil
}

// Not implemented
func (stub *MockStub) VerifySignature(certificate, signature, message []byte) (bool, error) {
	return false, nil
}

// Not implemented
func (stub *MockStub) GetCallerCertificate() ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetCallerMetadata() ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetBinding() ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetPayload() ([]byte, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) GetTxTimestamp() (*timestamp.Timestamp, error) {
	return nil, nil
}

// Not implemented
func (stub *MockStub) SetEvent(name string, payload []byte) error {
	return nil
}

// Constructor to initialise the internal State map
func NewMockStub(name string, cc Chaincode) *MockStub {
	mockLogger.Debug("MockStub(", name, cc, ")")
	s := new(MockStub)
	s.Name = name
	s.cc = cc
	s.State = make(map[string][]byte)
	s.Invokables = make(map[string]*MockStub)
	s.Keys = list.New()

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
		mockLogger.Error("HasNext() but already closed")
		return false
	}

	if iter.Current == nil {
		mockLogger.Error("HasNext() couldn't get Current")
		return false
	}

	if iter.Current.Next() == nil {
		// we've reached the end of the underlying values
		mockLogger.Debug("HasNext() but no next")
		return false
	}

	if iter.EndKey == iter.Current.Value {
		// we've reached the end of the specified range
		mockLogger.Debug("HasNext() at end of specified range")
		return false
	}

	mockLogger.Debug("HasNext() got next")
	return true
}

// Next returns the next key and value in the range query iterator.
func (iter *MockStateRangeQueryIterator) Next() (string, []byte, error) {
	if iter.Closed == true {
		mockLogger.Error("MockStateRangeQueryIterator.Next() called after Close()")
		return "", nil, errors.New("MockStateRangeQueryIterator.Next() called after Close()")
	}

	if iter.HasNext() == false {
		mockLogger.Error("MockStateRangeQueryIterator.Next() called when it does not HaveNext()")
		return "", nil, errors.New("MockStateRangeQueryIterator.Next() called when it does not HaveNext()")
	}

	iter.Current = iter.Current.Next()

	if iter.Current == nil {
		mockLogger.Error("MockStateRangeQueryIterator.Next() went past end of range")
		return "", nil, errors.New("MockStateRangeQueryIterator.Next() went past end of range")
	}
	key := iter.Current.Value.(string)
	value, err := iter.Stub.GetState(key)
	return key, value, err
}

// Close closes the range query iterator. This should be called when done
// reading from the iterator to free up resources.
func (iter *MockStateRangeQueryIterator) Close() error {
	if iter.Closed == true {
		mockLogger.Error("MockStateRangeQueryIterator.Close() called after Close()")
		return errors.New("MockStateRangeQueryIterator.Close() called after Close()")
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
