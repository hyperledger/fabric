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

package ordererledger

import (
	"errors"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/blkstorage"

	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
)

const (
	fileSegmentSize = 64 * 1024 * 1024
)

// fsBasedOrdererLedger - an orderer ledger implementation that persists blocks on filesystem based store
type fsBasedOrdererLedger struct {
	blockStore blkstorage.BlockStore
}

// GetBlockchainInfo returns basic info about blockchain
func (rl *fsBasedOrdererLedger) GetBlockchainInfo() (*pb.BlockchainInfo, error) {
	return rl.blockStore.GetBlockchainInfo()
}

// GetBlockByNumber returns block at a given height
func (rl *fsBasedOrdererLedger) GetBlockByNumber(blockNumber uint64) (*common.Block, error) {
	return rl.blockStore.RetrieveBlockByNumber(blockNumber)
}

// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
// ResultsIterator contains type BlockHolder
func (rl *fsBasedOrdererLedger) GetBlocksIterator(startBlockNumber uint64) (ledger.ResultsIterator, error) {
	return rl.blockStore.RetrieveBlocks(startBlockNumber)
}

//Prune prunes the blocks/transactions that satisfy the given policy
func (rl *fsBasedOrdererLedger) Prune(policy ledger.PrunePolicy) error {
	return errors.New("Not yet implemented")
}

// Close closes the ledger
func (rl *fsBasedOrdererLedger) Close() {
	rl.blockStore.Shutdown()
}

// Commit adds a new block
func (rl *fsBasedOrdererLedger) Commit(block *common.Block) error {
	return rl.blockStore.AddBlock(block)
}
