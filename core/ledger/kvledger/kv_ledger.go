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

package kvledger

import (
	"errors"
	"fmt"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"

	logging "github.com/op/go-logging"

	"github.com/hyperledger/fabric/protos/common"
)

var logger = logging.MustGetLogger("kvledger")

// KVLedger provides an implementation of `ledger.PeerLedger`.
// This implementation provides a key-value based data model
type kvLedger struct {
	ledgerID   string
	blockStore blkstorage.BlockStore
	txtmgmt    txmgr.TxMgr
	historyDB  historydb.HistoryDB
}

// NewKVLedger constructs new `KVLedger`
func newKVLedger(ledgerID string, blockStore blkstorage.BlockStore,
	versionedDB statedb.VersionedDB, historyDB historydb.HistoryDB) (*kvLedger, error) {

	logger.Debugf("Creating KVLedger ledgerID=%s: ", ledgerID)

	//Initialize transaction manager using state database
	var txmgmt txmgr.TxMgr
	txmgmt = lockbasedtxmgr.NewLockBasedTxMgr(versionedDB)

	// Create a kvLedger for this chain/ledger, which encasulates the underlying
	// id store, blockstore, txmgr (state database), history database
	l := &kvLedger{ledgerID, blockStore, txmgmt, historyDB}

	//Recover both state DB and history DB if they are out of sync with block storage
	if err := recoverDB(l); err != nil {
		panic(fmt.Errorf(`Error during state DB recovery:%s`, err))
	}

	return l, nil
}

//Recover the state database and history database (if exist)
//by recommitting last valid blocks
func recoverDB(l *kvLedger) error {
	//If there is no block in blockstorage, nothing to recover.
	info, _ := l.blockStore.GetBlockchainInfo()
	if info.Height == 0 {
		logger.Debugf("Block storage is empty.")
		return nil
	}

	var err error
	var stateDBSavepoint, historyDBSavepoint uint64
	//Default value for bool is false
	var recoverStateDB, recoverHistoryDB bool

	//Getting savepointValue stored in the state DB
	if stateDBSavepoint, err = l.txtmgmt.GetBlockNumFromSavepoint(); err != nil {
		return err
	}

	//Check whether the state DB is in sync with block storage
	if recoverStateDB, err = isRecoveryNeeded(stateDBSavepoint, info.Height); err != nil {
		return err
	}

	if ledgerconfig.IsHistoryDBEnabled() == true {
		//Getting savepointValue stored in the history DB
		if historyDBSavepoint, err = l.historyDB.GetBlockNumFromSavepoint(); err != nil {
			return err
		}
		//Check whether the history DB is in sync with block storage
		if recoverHistoryDB, err = isRecoveryNeeded(historyDBSavepoint, info.Height); err != nil {
			return err
		}
	}

	if recoverHistoryDB == false && recoverStateDB == false {
		//If nothing needs recovery, return
		if ledgerconfig.IsHistoryDBEnabled() == true {
			logger.Debugf("Both state database and history database are in sync with the block storage. No need to perform recovery operation.")
		} else {
			logger.Debugf("State database is in sync with the block storage.")
		}
		return nil
	} else if recoverHistoryDB == false && recoverStateDB == true {
		logger.Debugf("State database is behind block storage by %d blocks. Recovering state database.", info.Height-stateDBSavepoint)
		if err = recommitLostBlocks(l, stateDBSavepoint, info.Height, true, false); err != nil {
			return err
		}
	} else if recoverHistoryDB == true && recoverStateDB == false {
		logger.Debugf("History database is behind block storage by %d blocks. Recovering history database.", info.Height-historyDBSavepoint)
		if err = recommitLostBlocks(l, historyDBSavepoint, info.Height, false, true); err != nil {
			return err
		}
	} else if recoverHistoryDB == true && recoverStateDB == true {
		logger.Debugf("State database is behind block storage by %d blocks, and history database is behind block storage by %d blocks. Recovering both state and history database.", info.Height-stateDBSavepoint, info.Height-historyDBSavepoint)
		//If both state DB and history DB need to be recovered, first
		//we need to ensure that the state DB and history DB are in same state
		//before recommitting lost blocks.
		if stateDBSavepoint > historyDBSavepoint {
			logger.Debugf("History database is behind the state database by %d blocks", stateDBSavepoint-historyDBSavepoint)
			logger.Debugf("Making the history DB in sync with state DB")
			if err = recommitLostBlocks(l, historyDBSavepoint, stateDBSavepoint, false, true); err != nil {
				return err
			}
			logger.Debugf("Making both history DB and state DB in sync with the block storage")
			if err = recommitLostBlocks(l, stateDBSavepoint, info.Height, true, true); err != nil {
				return err
			}
		} else if stateDBSavepoint < historyDBSavepoint {
			logger.Debugf("State database is behind the history database by %d blocks", historyDBSavepoint-stateDBSavepoint)
			logger.Debugf("Making the state DB in sync with history DB")
			if err = recommitLostBlocks(l, stateDBSavepoint, historyDBSavepoint, true, false); err != nil {
				return err
			}
			logger.Debugf("Making both state DB and history DB in sync with the block storage")
			if err = recommitLostBlocks(l, historyDBSavepoint, info.Height, true, true); err != nil {
				return err
			}
		} else {
			logger.Debugf("State and history database are in same state but behind block storage")
			logger.Debugf("Making both state DB and history DB in sync with the block storage")
			if err = recommitLostBlocks(l, stateDBSavepoint, info.Height, true, true); err != nil {
				return err
			}
		}
	}
	return nil
}

//isRecoveryNeeded compares savepoint and current block height to decide whether
//to initiate recovery process
func isRecoveryNeeded(savepoint uint64, blockHeight uint64) (bool, error) {
	if savepoint > blockHeight {
		return false, errors.New("BlockStorage height is behind savepoint by %d blocks. Recovery the BlockStore first")
	} else if savepoint == blockHeight {
		return false, nil
	} else {
		return true, nil
	}
}

//recommitLostBlocks retrieves blocks in specified range and commit the write set to either
//state DB or history DB or both
func recommitLostBlocks(l *kvLedger, savepoint uint64, blockHeight uint64, recoverStateDB bool, recoverHistoryDB bool) error {
	//Compute updateSet for each missing savepoint and commit to state DB
	var err error
	var block *common.Block
	for blockNumber := savepoint + 1; blockNumber <= blockHeight; blockNumber++ {
		if block, err = l.GetBlockByNumber(blockNumber); err != nil {
			return err
		}
		if recoverStateDB == true {
			logger.Debugf("Constructing updateSet for the block %d", blockNumber)
			if err = l.txtmgmt.ValidateAndPrepare(block, false); err != nil {
				return err
			}
			logger.Debugf("Committing block %d to state database", blockNumber)
			if err = l.txtmgmt.Commit(); err != nil {
				return err
			}
		}
		if ledgerconfig.IsHistoryDBEnabled() == true && recoverHistoryDB == true {
			if err = l.historyDB.Commit(block); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetTransactionByID retrieves a transaction by id
func (l *kvLedger) GetTransactionByID(txID string) (*common.Envelope, error) {
	return l.blockStore.RetrieveTxByID(txID)
}

// GetBlockchainInfo returns basic info about blockchain
func (l *kvLedger) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	return l.blockStore.GetBlockchainInfo()
}

// GetBlockByNumber returns block at a given height
// blockNumber of  math.MaxUint64 will return last block
func (l *kvLedger) GetBlockByNumber(blockNumber uint64) (*common.Block, error) {
	return l.blockStore.RetrieveBlockByNumber(blockNumber)

}

// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
// ResultsIterator contains type BlockHolder
func (l *kvLedger) GetBlocksIterator(startBlockNumber uint64) (commonledger.ResultsIterator, error) {
	return l.blockStore.RetrieveBlocks(startBlockNumber)

}

// GetBlockByHash returns a block given it's hash
func (l *kvLedger) GetBlockByHash(blockHash []byte) (*common.Block, error) {
	return l.blockStore.RetrieveBlockByHash(blockHash)
}

//Prune prunes the blocks/transactions that satisfy the given policy
func (l *kvLedger) Prune(policy commonledger.PrunePolicy) error {
	return errors.New("Not yet implemented")
}

// NewTxSimulator returns new `ledger.TxSimulator`
func (l *kvLedger) NewTxSimulator() (ledger.TxSimulator, error) {
	return l.txtmgmt.NewTxSimulator()
}

// NewQueryExecutor gives handle to a query executor.
// A client can obtain more than one 'QueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (l *kvLedger) NewQueryExecutor() (ledger.QueryExecutor, error) {
	return l.txtmgmt.NewQueryExecutor()
}

// NewHistoryQueryExecutor gives handle to a history query executor.
// A client can obtain more than one 'HistoryQueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (l *kvLedger) NewHistoryQueryExecutor() (ledger.HistoryQueryExecutor, error) {
	return l.historyDB.NewHistoryQueryExecutor()
}

// Commit commits the valid block (returned in the method RemoveInvalidTransactionsAndPrepare) and related state changes
func (l *kvLedger) Commit(block *common.Block) error {
	var err error

	logger.Debugf("Validating block")
	err = l.txtmgmt.ValidateAndPrepare(block, true)
	if err != nil {
		return err
	}

	logger.Debugf("Committing block to storage")
	if err = l.blockStore.AddBlock(block); err != nil {
		return err
	}

	logger.Debugf("Committing block transactions to state database")
	if err = l.txtmgmt.Commit(); err != nil {
		panic(fmt.Errorf(`Error during commit to txmgr:%s`, err))
	}

	// History database could be written in parallel with state and/or async as a future optimization
	if ledgerconfig.IsHistoryDBEnabled() == true {
		logger.Debugf("Committing block transactions to history database")
		if err := l.historyDB.Commit(block); err != nil {
			panic(fmt.Errorf(`Error during commit to history db:%s`, err))
		}
	}

	return nil
}

// Close closes `KVLedger`
func (l *kvLedger) Close() {
	l.blockStore.Shutdown()
	l.txtmgmt.Shutdown()
}
