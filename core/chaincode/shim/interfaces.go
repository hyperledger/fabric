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

// Interfaces to allow testing of chaincode apps with mocked up stubs
package shim

import (
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric/core/chaincode/shim/crypto/attr"
)

// Chaincode interface must be implemented by all chaincodes. The fabric runs
// the transactions by calling these functions as specified.
type Chaincode interface {
	// Init is called during Deploy transaction after the container has been
	// established, allowing the chaincode to initialize its internal data
	Init(stub ChaincodeStubInterface, function string, args []string) ([]byte, error)

	// Invoke is called for every Invoke transactions. The chaincode may change
	// its state variables
	Invoke(stub ChaincodeStubInterface, function string, args []string) ([]byte, error)

	// Query is called for Query transactions. The chaincode may only read
	// (but not modify) its state variables and return the result
	Query(stub ChaincodeStubInterface, function string, args []string) ([]byte, error)
}

// ChaincodeStubInterface is used by deployable chaincode apps to access and modify their ledgers
type ChaincodeStubInterface interface {
	// Get the arguments to the stub call as a 2D byte array
	GetArgs() [][]byte

	// Get the arguments to the stub call as a string array
	GetStringArgs() []string

	// Get the transaction ID
	GetTxID() string

	// InvokeChaincode locally calls the specified chaincode `Invoke` using the
	// same transaction context; that is, chaincode calling chaincode doesn't
	// create a new transaction message.
	InvokeChaincode(chaincodeName string, args [][]byte) ([]byte, error)

	// QueryChaincode locally calls the specified chaincode `Query` using the
	// same transaction context; that is, chaincode calling chaincode doesn't
	// create a new transaction message.
	QueryChaincode(chaincodeName string, args [][]byte) ([]byte, error)

	// GetState returns the byte array value specified by the `key`.
	GetState(key string) ([]byte, error)

	// PutState writes the specified `value` and `key` into the ledger.
	PutState(key string, value []byte) error

	// DelState removes the specified `key` and its value from the ledger.
	DelState(key string) error

	// RangeQueryState function can be invoked by a chaincode to query of a range
	// of keys in the state. Assuming the startKey and endKey are in lexical
	// an iterator will be returned that can be used to iterate over all keys
	// between the startKey and endKey, inclusive. The order in which keys are
	// returned by the iterator is random.
	RangeQueryState(startKey, endKey string) (StateRangeQueryIteratorInterface, error)

	// CreateTable creates a new table given the table name and column definitions
	CreateTable(name string, columnDefinitions []*ColumnDefinition) error

	// GetTable returns the table for the specified table name or ErrTableNotFound
	// if the table does not exist.
	GetTable(tableName string) (*Table, error)

	// DeleteTable deletes an entire table and all associated rows.
	DeleteTable(tableName string) error

	// InsertRow inserts a new row into the specified table.
	// Returns -
	// true and no error if the row is successfully inserted.
	// false and no error if a row already exists for the given key.
	// false and a TableNotFoundError if the specified table name does not exist.
	// false and an error if there is an unexpected error condition.
	InsertRow(tableName string, row Row) (bool, error)

	// ReplaceRow updates the row in the specified table.
	// Returns -
	// true and no error if the row is successfully updated.
	// false and no error if a row does not exist the given key.
	// flase and a TableNotFoundError if the specified table name does not exist.
	// false and an error if there is an unexpected error condition.
	ReplaceRow(tableName string, row Row) (bool, error)

	// GetRow fetches a row from the specified table for the given key.
	GetRow(tableName string, key []Column) (Row, error)

	// GetRows returns multiple rows based on a partial key. For example, given table
	// | A | B | C | D |
	// where A, C and D are keys, GetRows can be called with [A, C] to return
	// all rows that have A, C and any value for D as their key. GetRows could
	// also be called with A only to return all rows that have A and any value
	// for C and D as their key.
	GetRows(tableName string, key []Column) (<-chan Row, error)

	// DeleteRow deletes the row for the given key from the specified table.
	DeleteRow(tableName string, key []Column) error

	// ReadCertAttribute is used to read an specific attribute from the transaction certificate,
	// *attributeName* is passed as input parameter to this function.
	// Example:
	//  attrValue,error:=stub.ReadCertAttribute("position")
	ReadCertAttribute(attributeName string) ([]byte, error)

	// VerifyAttribute is used to verify if the transaction certificate has an attribute
	// with name *attributeName* and value *attributeValue* which are the input parameters
	// received by this function.
	// Example:
	//    containsAttr, error := stub.VerifyAttribute("position", "Software Engineer")
	VerifyAttribute(attributeName string, attributeValue []byte) (bool, error)

	// VerifyAttributes does the same as VerifyAttribute but it checks for a list of
	// attributes and their respective values instead of a single attribute/value pair
	// Example:
	//    containsAttrs, error:= stub.VerifyAttributes(&attr.Attribute{"position",  "Software Engineer"}, &attr.Attribute{"company", "ACompany"})
	VerifyAttributes(attrs ...*attr.Attribute) (bool, error)

	// VerifySignature verifies the transaction signature and returns `true` if
	// correct and `false` otherwise
	VerifySignature(certificate, signature, message []byte) (bool, error)

	// GetCallerCertificate returns caller certificate
	GetCallerCertificate() ([]byte, error)

	// GetCallerMetadata returns caller metadata
	GetCallerMetadata() ([]byte, error)

	// GetBinding returns the transaction binding
	GetBinding() ([]byte, error)

	// GetPayload returns transaction payload, which is a `ChaincodeSpec` defined
	// in fabric/protos/chaincode.proto
	GetPayload() ([]byte, error)

	// GetTxTimestamp returns transaction created timestamp, which is currently
	// taken from the peer receiving the transaction. Note that this timestamp
	// may not be the same with the other peers' time.
	GetTxTimestamp() (*timestamp.Timestamp, error)

	// SetEvent saves the event to be sent when a transaction is made part of a block
	SetEvent(name string, payload []byte) error
}

// StateRangeQueryIteratorInterface allows a chaincode to iterate over a range of
// key/value pairs in the state.
type StateRangeQueryIteratorInterface interface {

	// HasNext returns true if the range query iterator contains additional keys
	// and values.
	HasNext() bool

	// Next returns the next key and value in the range query iterator.
	Next() (string, []byte, error)

	// Close closes the range query iterator. This should be called when done
	// reading from the iterator to free up resources.
	Close() error
}
