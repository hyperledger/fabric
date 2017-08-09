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

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/pvtdatatxmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgerstorage"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

var logger = flogging.MustGetLogger("kvledger")

// KVLedger provides an implementation of `ledger.PeerLedger`.
// This implementation provides a key-value based data model
type kvLedger struct {
	ledgerID       string
	blockStore     *ledgerstorage.Store
	txtmgmt        txmgr.TxMgr
	historyDB      historydb.HistoryDB
	transientStore transientstore.Store
}

// NewKVLedger constructs new `KVLedger`
func newKVLedger(ledgerID string, blockStore *ledgerstorage.Store,
	versionedDB privacyenabledstate.DB, historyDB historydb.HistoryDB,
	transientStore transientstore.Store) (*kvLedger, error) {

	logger.Debugf("Creating KVLedger ledgerID=%s: ", ledgerID)

	//Initialize transaction manager using state database
	var txmgmt txmgr.TxMgr
	txmgmt = pvtdatatxmgr.NewLockbasedTxMgr(versionedDB, transientStore)

	// Create a kvLedger for this chain/ledger, which encasulates the underlying
	// id store, blockstore, txmgr (state database), history database
	l := &kvLedger{ledgerID, blockStore, txmgmt, historyDB, transientStore}

	//Recover both state DB and history DB if they are out of sync with block storage
	if err := l.recoverDBs(); err != nil {
		panic(fmt.Errorf(`Error during state DB recovery:%s`, err))
	}

	return l, nil
}

//Recover the state database and history database (if exist)
//by recommitting last valid blocks
func (l *kvLedger) recoverDBs() error {
	logger.Debugf("Entering recoverDB()")
	//If there is no block in blockstorage, nothing to recover.
	info, _ := l.blockStore.GetBlockchainInfo()
	if info.Height == 0 {
		logger.Debug("Block storage is empty.")
		return nil
	}
	lastAvailableBlockNum := info.Height - 1
	recoverables := []recoverable{l.txtmgmt, l.historyDB}
	recoverers := []*recoverer{}
	for _, recoverable := range recoverables {
		recoverFlag, firstBlockNum, err := recoverable.ShouldRecover(lastAvailableBlockNum)
		if err != nil {
			return err
		}
		if recoverFlag {
			recoverers = append(recoverers, &recoverer{firstBlockNum, recoverable})
		}
	}
	if len(recoverers) == 0 {
		return nil
	}
	if len(recoverers) == 1 {
		return l.recommitLostBlocks(recoverers[0].firstBlockNum, lastAvailableBlockNum, recoverers[0].recoverable)
	}

	// both dbs need to be recovered
	if recoverers[0].firstBlockNum > recoverers[1].firstBlockNum {
		// swap (put the lagger db at 0 index)
		recoverers[0], recoverers[1] = recoverers[1], recoverers[0]
	}
	if recoverers[0].firstBlockNum != recoverers[1].firstBlockNum {
		// bring the lagger db equal to the other db
		if err := l.recommitLostBlocks(recoverers[0].firstBlockNum, recoverers[1].firstBlockNum-1,
			recoverers[0].recoverable); err != nil {
			return err
		}
	}
	// get both the db upto block storage
	return l.recommitLostBlocks(recoverers[1].firstBlockNum, lastAvailableBlockNum,
		recoverers[0].recoverable, recoverers[1].recoverable)
}

//recommitLostBlocks retrieves blocks in specified range and commit the write set to either
//state DB or history DB or both
func (l *kvLedger) recommitLostBlocks(firstBlockNum uint64, lastBlockNum uint64, recoverables ...recoverable) error {
	var err error
	var block *common.Block
	for blockNumber := firstBlockNum; blockNumber <= lastBlockNum; blockNumber++ {
		if block, err = l.GetBlockByNumber(blockNumber); err != nil {
			return err
		}
		for _, r := range recoverables {
			if err := r.CommitLostBlock(&ledger.BlockAndPvtData{Block: block}); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetTransactionByID retrieves a transaction by id
func (l *kvLedger) GetTransactionByID(txID string) (*peer.ProcessedTransaction, error) {
	tranEnv, err := l.blockStore.RetrieveTxByID(txID)
	if err != nil {
		return nil, err
	}
	txVResult, err := l.blockStore.RetrieveTxValidationCodeByTxID(txID)
	if err != nil {
		return nil, err
	}
	processedTran := &peer.ProcessedTransaction{TransactionEnvelope: tranEnv, ValidationCode: int32(txVResult)}
	return processedTran, nil
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

// GetBlockByTxID returns a block which contains a transaction
func (l *kvLedger) GetBlockByTxID(txID string) (*common.Block, error) {
	return l.blockStore.RetrieveBlockByTxID(txID)
}

func (l *kvLedger) GetTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	return l.blockStore.RetrieveTxValidationCodeByTxID(txID)
}

//Prune prunes the blocks/transactions that satisfy the given policy
func (l *kvLedger) Prune(policy commonledger.PrunePolicy) error {
	return errors.New("Not yet implemented")
}

// NewTxSimulator returns new `ledger.TxSimulator`
func (l *kvLedger) NewTxSimulator(txid string) (ledger.TxSimulator, error) {
	return l.txtmgmt.NewTxSimulator(txid)
}

// NewQueryExecutor gives handle to a query executor.
// A client can obtain more than one 'QueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
func (l *kvLedger) NewQueryExecutor() (ledger.QueryExecutor, error) {
	return l.txtmgmt.NewQueryExecutor(util.GenerateUUID())
}

// NewHistoryQueryExecutor gives handle to a history query executor.
// A client can obtain more than one 'HistoryQueryExecutor's for parallel execution.
// Any synchronization should be performed at the implementation level if required
// Pass the ledger blockstore so that historical values can be looked up from the chain
func (l *kvLedger) NewHistoryQueryExecutor() (ledger.HistoryQueryExecutor, error) {
	return l.historyDB.NewHistoryQueryExecutor(l.blockStore)
}

// Commit commits the valid block (returned in the method RemoveInvalidTransactionsAndPrepare) and related state changes
// TODO when we move the transient store outside the ledger, the commiter would invoke function `CommitWithPvtData` and this
// function will be removed
func (l *kvLedger) Commit(block *common.Block) error {
	pvtdata, err := retrievePrivateData(l.transientStore, block)
	if err != nil {
		return err
	}
	return l.CommitWithPvtData(&ledger.BlockAndPvtData{Block: block, BlockPvtData: pvtdata})
}

// CommitWithPvtData commits the block and the corresponding pvt data in an atomic operation
func (l *kvLedger) CommitWithPvtData(pvtdataAndBlock *ledger.BlockAndPvtData) error {
	var err error
	block := pvtdataAndBlock.Block
	blockNo := pvtdataAndBlock.Block.Header.Number

	logger.Debugf("Channel [%s]: Validating block [%d]", l.ledgerID, blockNo)
	err = l.txtmgmt.ValidateAndPrepare(pvtdataAndBlock, true)
	if err != nil {
		return err
	}

	logger.Debugf("Channel [%s]: Committing block [%d] to storage", l.ledgerID, blockNo)
	if err = l.blockStore.CommitWithPvtData(pvtdataAndBlock); err != nil {
		return err
	}
	logger.Infof("Channel [%s]: Created block [%d] with %d transaction(s)", l.ledgerID, block.Header.Number, len(block.Data.Data))

	logger.Debugf("Channel [%s]: Committing block [%d] transactions to state database", l.ledgerID, blockNo)
	if err = l.txtmgmt.Commit(); err != nil {
		panic(fmt.Errorf(`Error during commit to txmgr:%s`, err))
	}

	// History database could be written in parallel with state and/or async as a future optimization
	if ledgerconfig.IsHistoryDBEnabled() {
		logger.Debugf("Channel [%s]: Committing block [%d] transactions to history database", l.ledgerID, blockNo)
		if err := l.historyDB.Commit(block); err != nil {
			panic(fmt.Errorf(`Error during commit to history db:%s`, err))
		}
	}
	return nil
}

// GetPvtDataAndBlockByNum returns the block and the corresponding pvt data.
// The pvt data is filtered by the list of 'collections' supplied
func (l *kvLedger) GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error) {
	return l.blockStore.GetPvtDataAndBlockByNum(blockNum, filter)
}

// GetPvtDataByNum returns only the pvt data  corresponding to the given block number
// The pvt data is filtered by the list of 'collections' supplied
func (l *kvLedger) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	return l.blockStore.GetPvtDataByNum(blockNum, filter)
}

// Purge removes private read-writes set generated by endorsers at block height lesser than
// a given maxBlockNumToRetain. In other words, Purge only retains private read-write sets
// that were generated at block height of maxBlockNumToRetain or higher.
func (l *kvLedger) PurgePrivateData(maxBlockNumToRetain uint64) error {
	return fmt.Errorf("not yet implemented")
}

// PrivateDataMinBlockNum returns the lowest retained endorsement block height
func (l *kvLedger) PrivateDataMinBlockNum() (uint64, error) {
	return 0, fmt.Errorf("not yet implemented")
}

// Close closes `KVLedger`
func (l *kvLedger) Close() {
	l.blockStore.Shutdown()
	l.txtmgmt.Shutdown()
	l.transientStore.Shutdown()
}

// retrievePrivateData retrieves the pvt data from the transient store for committing it into the
// pvt data store along with block commit.
// KVLedger does this job temporarily for phase-1 and will be moved out to committer
func retrievePrivateData(transientStore transientstore.Store, block *common.Block) (map[uint64]*ledger.TxPvtData, error) {
	pvtdata := make(map[uint64]*ledger.TxPvtData)
	for txIndex, envBytes := range block.Data.Data {
		env, err := utils.GetEnvelopeFromBlock(envBytes)
		if err != nil {
			return nil, err
		}
		payload, err := utils.GetPayload(env)
		if err != nil {
			return nil, err
		}
		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			return nil, err
		}
		pvtEndorsement, err := transientStore.GetSelfSimulatedTxPvtRWSetByTxid(chdr.TxId)
		if err != nil {
			return nil, err
		}
		if pvtEndorsement == nil {
			continue
		}
		txPvtRWSet := &rwset.TxPvtReadWriteSet{}
		if err := proto.Unmarshal(pvtEndorsement.PvtSimulationResults, txPvtRWSet); err != nil {
			return nil, err
		}
		seqInBlock := uint64(txIndex)
		pvtdata[seqInBlock] = &ledger.TxPvtData{SeqInBlock: seqInBlock, WriteSet: txPvtRWSet}
	}
	return pvtdata, nil
}
