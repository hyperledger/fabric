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

package ledger

import (
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Ledger captures the methods that are common across the 'raw ledger' and the 'final ledger'
type Ledger interface {
	// GetBlockchainInfo returns basic info about blockchain
	GetBlockchainInfo() (*pb.BlockchainInfo, error)
	// GetBlockchainInfo returns block at a given height
	GetBlockByNumber(blockNumber uint64) (*common.Block, error)
	// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
	// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
	// ResultsIterator contains type BlockHolder
	GetBlocksIterator(startBlockNumber uint64) (ResultsIterator, error)
	//Prune prunes the blocks/transactions that satisfy the given policy
	Prune(policy PrunePolicy) error
	// Close closes the ledger
	Close()
}

// RawLedger implements methods required by 'raw ledger'
type RawLedger interface {
	Ledger
	// CommitBlock adds a new block
	CommitBlock(block *common.Block) error
}

// ValidatedLedger represents the 'final ledger'. In addition to implement the methods inherited from the Ledger,
// it provides the handle to objects for querying the state and executing transactions.
type ValidatedLedger interface {
	Ledger
	// GetTransactionByID retrieves a transaction by id
	GetTransactionByID(txID string) (*pb.Transaction, error)
	// GetBlockByHash returns a block given it's hash
	GetBlockByHash(blockHash []byte) (*common.Block, error)
	// NewTxSimulator gives handle to a transaction simulator.
	// A client can obtain more than one 'TxSimulator's for parallel execution.
	// Any snapshoting/synchronization should be performed at the implementation level if required
	NewTxSimulator() (TxSimulator, error)
	// NewQueryExecuter gives handle to a query executer.
	// A client can obtain more than one 'QueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewQueryExecutor() (QueryExecutor, error)
	// RemoveInvalidTransactions validates all the transactions in the given block
	// and returns a block that contains only valid transactions and a list of transactions that are invalid
	RemoveInvalidTransactionsAndPrepare(block *common.Block) (*common.Block, []*pb.InvalidTransaction, error)
	// Commit commits the changes prepared in the method RemoveInvalidTransactionsAndPrepare.
	// Commits both the valid block and related state changes
	Commit() error
	// Rollback rollbacks the changes prepared in the method RemoveInvalidTransactionsAndPrepare
	Rollback()
}

// QueryExecutor executes the queries
// Get* methods are for supporting KV-based data model. ExecuteQuery method is for supporting a rich datamodel and query support
//
// ExecuteQuery method in the case of a rich data model is expected to support queries on
// latest state, historical state and on the intersection of state and transactions
type QueryExecutor interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) ([]byte, error)
	// GetStateMultipleKeys gets the values for multiple keys in a single call
	GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error)
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// The returned ResultsIterator contains results of type *KV
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)
	// GetTransactionsForKey returns an iterator that contains all the transactions that modified the given key.
	// The returned ResultsIterator contains results of type *msgs.Transaction
	GetTransactionsForKey(namespace string, key string) (ResultsIterator, error)
	// ExecuteQuery executes the given query and returns an iterator that contains results of type specific to the underlying data store.
	ExecuteQuery(query string) (ResultsIterator, error)
	// Done releases resources occupied by the QueryExecutor
	Done()
}

// TxSimulator simulates a transaction on a consistent snapshot of the 'as recent state as possible'
// Set* methods are for supporting KV-based data model. ExecuteUpdate method is for supporting a rich datamodel and query support
type TxSimulator interface {
	QueryExecutor
	// SetState sets the given value for the given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	SetState(namespace string, key string, value []byte) error
	// DeleteState deletes the given namespace and key
	DeleteState(namespace string, key string) error
	// SetMultipleKeys sets the values for multiple keys in a single call
	SetStateMultipleKeys(namespace string, kvs map[string][]byte) error
	// ExecuteUpdate for supporting rich data model (see comments on QueryExecutor above)
	ExecuteUpdate(query string) error
	// GetTxSimulationResults encapsulates the results of the transaction simulation.
	// This should contain enough detail for
	// - The update in the state that would be caused if the transaction is to be committed
	// - The environment in which the transaction is executed so as to be able to decide the validity of the enviroment
	//   (at a later time on a different peer) during committing the transactions
	// Different ledger implementation (or configurations of a single implementation) may want to represent the above two pieces
	// of information in different way in order to support different data-models or optimize the information representations.
	// TODO detailed illustration of a couple of representations.
	GetTxSimulationResults() ([]byte, error)
}

// ResultsIterator - an iterator for query result set
type ResultsIterator interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (QueryResult, error)
	// Close releases resources occupied by the iterator
	Close()
}

// QueryResult - a general interface for supporting different types of query results. Actual types differ for different queries
type QueryResult interface{}

// KV - QueryResult for KV-based datamodel. Holds a key and corresponding value. A nil value indicates a non-existent key.
type KV struct {
	Key   string
	Value []byte
}

// BlockHolder holds block returned by the iterator in GetBlocksIterator.
// The sole purpose of this holder is to avoid desrialization if block is desired in raw bytes form (e.g., for transfer)
type BlockHolder interface {
	GetBlock() *common.Block
	GetBlockBytes() []byte
}

// PrunePolicy - a general interface for supporting different pruning policies
type PrunePolicy interface{}
