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
	"github.com/golang/protobuf/ptypes/timestamp"

	pb "github.com/hyperledger/fabric/protos/peer"
)

// Chaincode interface must be implemented by all chaincodes. The fabric runs
// the transactions by calling these functions as specified.
type Chaincode interface {
	// Init is called during Deploy transaction after the container has been
	// established, allowing the chaincode to initialize its internal data
	Init(stub ChaincodeStubInterface) pb.Response
	// Invoke is called for every Invoke transactions. The chaincode may change
	// its state variables
	Invoke(stub ChaincodeStubInterface) pb.Response
}

// ChaincodeStubInterface is used by deployable chaincode apps to access and modify their ledgers
type ChaincodeStubInterface interface {
	// Get the arguments to the stub call as a 2D byte array
	GetArgs() [][]byte

	// Get the arguments to the stub call as a string array
	GetStringArgs() []string

	// Get the function which is the first argument and the rest of the arguments
	// as parameters
	GetFunctionAndParameters() (string, []string)

	// Get the transaction ID
	GetTxID() string

	// InvokeChaincode locally calls the specified chaincode `Invoke` using the
	// same transaction context; that is, chaincode calling chaincode doesn't
	// create a new transaction message. If the called chaincode is on a different
	// channel, only the Response is returned to the caller; any PutState calls
	// will not have any effect on the ledger of the channel; effectively it is
	// a `Query`. If `channel` is empty, the caller's channel is assumed.
	InvokeChaincode(chaincodeName string, args [][]byte, channel string) pb.Response

	// GetState returns the byte array value specified by the `key`.
	GetState(key string) ([]byte, error)

	// PutState writes the specified `value` and `key` into the ledger.
	PutState(key string, value []byte) error

	// DelState removes the specified `key` and its value from the ledger.
	DelState(key string) error

	// RangeQueryState function can be invoked by a chaincode to query of a range
	// of keys in the state. Assuming the startKey and endKey are in lexical
	// an iterator will be returned that can be used to iterate over all keys
	// between the startKey (inclusive) and endKey (exclusive). The order in which keys are
	// returned by the iterator is random.
	RangeQueryState(startKey, endKey string) (StateQueryIteratorInterface, error)

	// PartialCompositeKeyQuery function can be invoked by a chaincode to query the
	// state based on a given partial composite key. This function returns an
	// iterator which can be used to iterate over all composite keys whose prefix
	// matches the given partial composite key. This function should be used only for
	// a partial composite key. For a full composite key, an iter with empty response
	// would be returned.
	PartialCompositeKeyQuery(objectType string, keys []string) (StateQueryIteratorInterface, error)

	// Given a list of attributes, CreateCompositeKey function combines these attributes
	// to form a composite key.
	CreateCompositeKey(objectType string, attributes []string) (string, error)

	// GetQueryResult function can be invoked by a chaincode to perform a
	// rich query against state database.  Only supported by state database implementations
	// that support rich query.  The query string is in the syntax of the underlying
	// state database. An iterator is returned which can be used to iterate (next) over
	// the query result set
	GetQueryResult(query string) (StateQueryIteratorInterface, error)

	// Given a composite key, SplitCompositeKey function splits the key into attributes
	// on which the composite key was formed.
	SplitCompositeKey(compositeKey string) (string, []string, error)

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

// StateQueryIteratorInterface allows a chaincode to iterate over a set of
// key/value pairs in the state.
type StateQueryIteratorInterface interface {

	// HasNext returns true if the range query iterator contains additional keys
	// and values.
	HasNext() bool

	// Next returns the next key and value in the range query iterator.
	Next() (string, []byte, error)

	// Close closes the range query iterator. This should be called when done
	// reading from the iterator to free up resources.
	Close() error
}
