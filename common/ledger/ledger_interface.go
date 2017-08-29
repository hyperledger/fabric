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
)

// Ledger captures the methods that are common across the 'PeerLedger', 'OrdererLedger', and 'ValidatedLedger'
type Ledger interface {
	// GetBlockchainInfo returns basic info about blockchain
	GetBlockchainInfo() (*common.BlockchainInfo, error)
	// GetBlockByNumber returns block at a given height
	// blockNumber of  math.MaxUint64 will return last block
	GetBlockByNumber(blockNumber uint64) (*common.Block, error)
	// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
	// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
	// ResultsIterator contains type BlockHolder
	GetBlocksIterator(startBlockNumber uint64) (ResultsIterator, error)
	// Close closes the ledger
	Close()
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

// PrunePolicy - a general interface for supporting different pruning policies
type PrunePolicy interface{}
