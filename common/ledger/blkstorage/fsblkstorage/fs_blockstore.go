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
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
)

// fsBlockStore - filesystem based implementation for `BlockStore`
type fsBlockStore struct {
	id      string
	conf    *Conf
	fileMgr *blockfileMgr
}

// NewFsBlockStore constructs a `FsBlockStore`
func newFsBlockStore(id string, conf *Conf, indexConfig *blkstorage.IndexConfig,
	dbHandle *leveldbhelper.DBHandle) *fsBlockStore {
	return &fsBlockStore{id, conf, newBlockfileMgr(id, conf, indexConfig, dbHandle)}
}

// AddBlock adds a new block
func (store *fsBlockStore) AddBlock(block *common.Block) error {
	return store.fileMgr.addBlock(block)
}

// GetBlockchainInfo returns the current info about blockchain
func (store *fsBlockStore) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	return store.fileMgr.getBlockchainInfo(), nil
}

// RetrieveBlocks returns an iterator that can be used for iterating over a range of blocks
func (store *fsBlockStore) RetrieveBlocks(startNum uint64) (ledger.ResultsIterator, error) {
	var itr *blocksItr
	var err error
	if itr, err = store.fileMgr.retrieveBlocks(startNum); err != nil {
		return nil, err
	}
	return itr, nil
}

// RetrieveBlockByHash returns the block for given block-hash
func (store *fsBlockStore) RetrieveBlockByHash(blockHash []byte) (*common.Block, error) {
	return store.fileMgr.retrieveBlockByHash(blockHash)
}

// RetrieveBlockByNumber returns the block at a given blockchain height
func (store *fsBlockStore) RetrieveBlockByNumber(blockNum uint64) (*common.Block, error) {
	return store.fileMgr.retrieveBlockByNumber(blockNum)
}

// RetrieveTxByID returns a transaction for given transaction id
func (store *fsBlockStore) RetrieveTxByID(txID string) (*common.Envelope, error) {
	return store.fileMgr.retrieveTransactionByID(txID)
}

// RetrieveTxByID returns a transaction for given transaction id
func (store *fsBlockStore) RetrieveTxByBlockNumTranNum(blockNum uint64, tranNum uint64) (*common.Envelope, error) {
	return store.fileMgr.retrieveTransactionByBlockNumTranNum(blockNum, tranNum)
}

func (store *fsBlockStore) RetrieveBlockByTxID(txID string) (*common.Block, error) {
	return store.fileMgr.retrieveBlockByTxID(txID)
}

func (store *fsBlockStore) RetrieveTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	return store.fileMgr.retrieveTxValidationCodeByTxID(txID)
}

// Shutdown shuts down the block store
func (store *fsBlockStore) Shutdown() {
	logger.Debugf("closing fs blockStore:%s", store.id)
	store.fileMgr.close()
}
