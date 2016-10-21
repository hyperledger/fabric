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

package fsblkstorage

import (
	"github.com/hyperledger/fabric/core/ledgernext"
	"github.com/hyperledger/fabric/protos"
)

// FsBlockStore - filesystem based implementation for `BlockStore`
type FsBlockStore struct {
	fileMgr *blockfileMgr
}

// NewFsBlockStore constructs a `FsBlockStore`
func NewFsBlockStore(conf *Conf) *FsBlockStore {
	return &FsBlockStore{newBlockfileMgr(conf)}
}

// AddBlock adds a new block
func (store *FsBlockStore) AddBlock(block *protos.Block2) error {
	return store.fileMgr.addBlock(block)
}

// GetBlockchainInfo returns the current info about blockchain
func (store *FsBlockStore) GetBlockchainInfo() (*protos.BlockchainInfo, error) {
	return store.fileMgr.getBlockchainInfo(), nil
}

// RetrieveBlocks returns an iterator that can be used for iterating over a range of blocks
func (store *FsBlockStore) RetrieveBlocks(startNum uint64, endNum uint64) (ledger.ResultsIterator, error) {
	var itr *BlocksItr
	var err error
	if itr, err = store.fileMgr.retrieveBlocks(startNum, endNum); err != nil {
		return nil, err
	}
	return itr, nil
}

// RetrieveBlockByHash returns the block for given block-hash
func (store *FsBlockStore) RetrieveBlockByHash(blockHash []byte) (*protos.Block2, error) {
	return store.fileMgr.retrieveBlockByHash(blockHash)
}

// RetrieveBlockByNumber returns the block at a given blockchain height
func (store *FsBlockStore) RetrieveBlockByNumber(blockNum uint64) (*protos.Block2, error) {
	return store.fileMgr.retrieveBlockByNumber(blockNum)
}

// RetrieveTxByID returns a transaction for given transaction id
func (store *FsBlockStore) RetrieveTxByID(txID string) (*protos.Transaction2, error) {
	return store.fileMgr.retrieveTransactionByID(txID)
}

// Shutdown shuts down the block store
func (store *FsBlockStore) Shutdown() {
	store.fileMgr.close()
}
