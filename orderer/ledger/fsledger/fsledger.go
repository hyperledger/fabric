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

package fsledger

import (
	"errors"

	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"

	"github.com/hyperledger/fabric/protos/common"
)

const (
	fileSegmentSize = 64 * 1024 * 1024
)

// fsLedger - an orderer ledger implementation that persists blocks on filesystem based store
type fsLedger struct {
	blockStore blkstorage.BlockStore
}

// GetBlockchainInfo returns basic info about blockchain
func (l *fsLedger) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	return l.blockStore.GetBlockchainInfo()
}

// GetBlockByNumber returns block at a given height
func (l *fsLedger) GetBlockByNumber(blockNumber uint64) (*common.Block, error) {
	return l.blockStore.RetrieveBlockByNumber(blockNumber)
}

// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
// ResultsIterator contains type BlockHolder
func (l *fsLedger) GetBlocksIterator(startBlockNumber uint64) (ledger.ResultsIterator, error) {
	return l.blockStore.RetrieveBlocks(startBlockNumber)
}

//Prune prunes the blocks/transactions that satisfy the given policy
func (l *fsLedger) Prune(policy ledger.PrunePolicy) error {
	return errors.New("Not yet implemented")
}

// Close closes the ledger
func (l *fsLedger) Close() {
	l.blockStore.Shutdown()
}

// Commit adds a new block
func (l *fsLedger) Commit(block *common.Block) error {
	return l.blockStore.AddBlock(block)
}
