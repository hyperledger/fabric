/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgerstorage"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	lutil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("kvledger")

// KVLedger provides an implementation of `ledger.PeerLedger`.
// This implementation provides a key-value based data model
type kvLedger struct {
	ledgerID               string
	blockStore             *ledgerstorage.Store
	txtmgmt                txmgr.TxMgr
	historyDB              historydb.HistoryDB
	configHistoryRetriever ledger.ConfigHistoryRetriever
	blockAPIsRWLock        *sync.RWMutex
	stats                  *ledgerStats
	commitHash             []byte
}

// NewKVLedger constructs new `KVLedger`
func newKVLedger(
	ledgerID string,
	blockStore *ledgerstorage.Store,
	versionedDB privacyenabledstate.DB,
	historyDB historydb.HistoryDB,
	configHistoryMgr confighistory.Mgr,
	stateListeners []ledger.StateListener,
	bookkeeperProvider bookkeeping.Provider,
	ccInfoProvider ledger.DeployedChaincodeInfoProvider,
	stats *ledgerStats,
) (*kvLedger, error) {
	logger.Debugf("Creating KVLedger ledgerID=%s: ", ledgerID)
	// Create a kvLedger for this chain/ledger, which encasulates the underlying
	// id store, blockstore, txmgr (state database), history database
	l := &kvLedger{ledgerID: ledgerID, blockStore: blockStore, historyDB: historyDB, blockAPIsRWLock: &sync.RWMutex{}}

	// Retrieves the current commit hash from the blockstore
	var err error
	l.commitHash, err = l.lastPersistedCommitHash()
	if err != nil {
		return nil, err
	}

	// TODO Move the function `GetChaincodeEventListener` to ledger interface and
	// this functionality of regiserting for events to ledgermgmt package so that this
	// is reused across other future ledger implementations
	ccEventListener := versionedDB.GetChaincodeEventListener()
	logger.Debugf("Register state db for chaincode lifecycle events: %t", ccEventListener != nil)
	if ccEventListener != nil {
		cceventmgmt.GetMgr().Register(ledgerID, ccEventListener)
	}
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(&collectionInfoRetriever{l, ccInfoProvider})
	if err := l.initTxMgr(versionedDB, stateListeners, btlPolicy, bookkeeperProvider, ccInfoProvider); err != nil {
		return nil, err
	}
	l.initBlockStore(btlPolicy)
	//Recover both state DB and history DB if they are out of sync with block storage
	if err := l.recoverDBs(); err != nil {
		return nil, err
	}
	l.configHistoryRetriever = configHistoryMgr.GetRetriever(ledgerID, l)

	l.stats = stats
	return l, nil
}

func (l *kvLedger) initTxMgr(versionedDB privacyenabledstate.DB, stateListeners []ledger.StateListener,
	btlPolicy pvtdatapolicy.BTLPolicy, bookkeeperProvider bookkeeping.Provider, ccInfoProvider ledger.DeployedChaincodeInfoProvider) error {
	var err error
	l.txtmgmt, err = lockbasedtxmgr.NewLockBasedTxMgr(l.ledgerID, versionedDB, stateListeners, btlPolicy, bookkeeperProvider, ccInfoProvider)
	return err
}

func (l *kvLedger) initBlockStore(btlPolicy pvtdatapolicy.BTLPolicy) {
	l.blockStore.Init(btlPolicy)
}

func (l *kvLedger) lastPersistedCommitHash() ([]byte, error) {
	bcInfo, err := l.GetBlockchainInfo()
	if err != nil {
		return nil, err
	}
	if bcInfo.Height == 0 {
		logger.Debugf("Chain is empty")
		return nil, nil
	}

	logger.Debugf("Fetching block [%d] to retrieve the currentCommitHash", bcInfo.Height-1)
	block, err := l.GetBlockByNumber(bcInfo.Height - 1)
	if err != nil {
		return nil, err
	}

	if len(block.Metadata.Metadata) < int(common.BlockMetadataIndex_COMMIT_HASH+1) {
		logger.Debugf("Last block metadata does not contain commit hash")
		return nil, nil
	}

	commitHash := &common.Metadata{}
	err = proto.Unmarshal(block.Metadata.Metadata[common.BlockMetadataIndex_COMMIT_HASH], commitHash)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling last persisted commit hash")
	}
	return commitHash.Value, nil
}

//Recover the state database and history database (if exist)
//by recommitting last valid blocks
func (l *kvLedger) recoverDBs() error {
	logger.Debugf("Entering recoverDB()")
	if err := l.syncStateAndHistoryDBWithBlockstore(); err != nil {
		return err
	}
	if err := l.syncStateDBWithPvtdatastore(); err != nil {
		return err
	}
	return nil
}

func (l *kvLedger) syncStateAndHistoryDBWithBlockstore() error {
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

		// During ledger reset/rollback, the state database must be dropped. If the state database
		// uses goleveldb, the reset/rollback code itself drop the DB. If it uses couchDB, the
		// DB must be dropped manually. Hence, we compare (only for the stateDB) the height
		// of the state DB and block store to ensure that the state DB is dropped.

		// firstBlockNum is nothing but the nextBlockNum expected by the state DB.
		// In otherwords, the firstBlockNum is nothing but the height of stateDB.
		if firstBlockNum > lastAvailableBlockNum+1 {
			dbName := recoverable.Name()
			return fmt.Errorf("the %s database [height=%d] is ahead of the block store [height=%d]. "+
				"This is possible when the %s database is not dropped after a ledger reset/rollback. "+
				"The %s database can safely be dropped and will be rebuilt up to block store height upon the next peer start.",
				dbName, firstBlockNum, lastAvailableBlockNum+1, dbName, dbName)
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

func (l *kvLedger) syncStateDBWithPvtdatastore() error {
	// TODO: So far, the design philosophy was that the scope of block storage is
	// limited to storing and retrieving blocks data with certain guarantees and statedb is
	// for the state management. The higher layer, 'kvledger', coordinates the acts between
	// the two. However, with maintaining the state of the consumption of blocks (i.e,
	// lastUpdatedOldBlockList for pvtstore reconciliation) within private data block storage
	// breaks that assumption. The knowledge of what blocks have been consumed for the purpose
	// of state update should not lie with the source (i.e., pvtdatastorage). A potential fix
	// is mentioned in FAB-12731

	blocksPvtData, err := l.blockStore.GetLastUpdatedOldBlocksPvtData()
	if err != nil {
		return err
	}

	// as the pvtdataStore can contain pvtData of yet to be committed blocks,
	// we need to filter them before passing it to the transaction manager for
	// stateDB updates.
	if err := l.filterYetToCommitBlocks(blocksPvtData); err != nil {
		return err
	}

	if err = l.applyValidTxPvtDataOfOldBlocks(blocksPvtData); err != nil {
		return err
	}

	return nil
}

func (l *kvLedger) filterYetToCommitBlocks(blocksPvtData map[uint64][]*ledger.TxPvtData) error {
	info, err := l.blockStore.GetBlockchainInfo()
	if err != nil {
		return err
	}
	for blkNum := range blocksPvtData {
		if blkNum > info.Height-1 {
			logger.Infof("found pvtdata associated with yet to be committed block [%d]", blkNum)
			delete(blocksPvtData, blkNum)
		}
	}
	return nil
}

//recommitLostBlocks retrieves blocks in specified range and commit the write set to either
//state DB or history DB or both
func (l *kvLedger) recommitLostBlocks(firstBlockNum uint64, lastBlockNum uint64, recoverables ...recoverable) error {
	logger.Infof("Recommitting lost blocks - firstBlockNum=%d, lastBlockNum=%d, recoverables=%#v", firstBlockNum, lastBlockNum, recoverables)
	var err error
	var blockAndPvtdata *ledger.BlockAndPvtData
	for blockNumber := firstBlockNum; blockNumber <= lastBlockNum; blockNumber++ {
		if blockAndPvtdata, err = l.GetPvtDataAndBlockByNum(blockNumber, nil); err != nil {
			return err
		}
		for _, r := range recoverables {
			if err := r.CommitLostBlock(blockAndPvtdata); err != nil {
				return err
			}
		}
	}
	logger.Infof("Recommitted lost blocks - firstBlockNum=%d, lastBlockNum=%d, recoverables=%#v", firstBlockNum, lastBlockNum, recoverables)
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
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return processedTran, nil
}

// GetBlockchainInfo returns basic info about blockchain
func (l *kvLedger) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	bcInfo, err := l.blockStore.GetBlockchainInfo()
	l.blockAPIsRWLock.RLock()
	defer l.blockAPIsRWLock.RUnlock()
	return bcInfo, err
}

// GetBlockByNumber returns block at a given height
// blockNumber of  math.MaxUint64 will return last block
func (l *kvLedger) GetBlockByNumber(blockNumber uint64) (*common.Block, error) {
	block, err := l.blockStore.RetrieveBlockByNumber(blockNumber)
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return block, err
}

// GetBlocksIterator returns an iterator that starts from `startBlockNumber`(inclusive).
// The iterator is a blocking iterator i.e., it blocks till the next block gets available in the ledger
// ResultsIterator contains type BlockHolder
func (l *kvLedger) GetBlocksIterator(startBlockNumber uint64) (commonledger.ResultsIterator, error) {
	blkItr, err := l.blockStore.RetrieveBlocks(startBlockNumber)
	if err != nil {
		return nil, err
	}
	return &blocksItr{l.blockAPIsRWLock, blkItr}, nil
}

// GetBlockByHash returns a block given it's hash
func (l *kvLedger) GetBlockByHash(blockHash []byte) (*common.Block, error) {
	block, err := l.blockStore.RetrieveBlockByHash(blockHash)
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return block, err
}

// GetBlockByTxID returns a block which contains a transaction
func (l *kvLedger) GetBlockByTxID(txID string) (*common.Block, error) {
	block, err := l.blockStore.RetrieveBlockByTxID(txID)
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return block, err
}

func (l *kvLedger) GetTxValidationCodeByTxID(txID string) (peer.TxValidationCode, error) {
	txValidationCode, err := l.blockStore.RetrieveTxValidationCodeByTxID(txID)
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return txValidationCode, err
}

//Prune prunes the blocks/transactions that satisfy the given policy
func (l *kvLedger) Prune(policy commonledger.PrunePolicy) error {
	return errors.New("not yet implemented")
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

// CommitWithPvtData commits the block and the corresponding pvt data in an atomic operation
func (l *kvLedger) CommitWithPvtData(pvtdataAndBlock *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error {
	var err error
	block := pvtdataAndBlock.Block
	blockNo := pvtdataAndBlock.Block.Header.Number

	startBlockProcessing := time.Now()
	if commitOpts.FetchPvtDataFromLedger {
		// when we reach here, it means that the pvtdata store has the
		// pvtdata associated with this block but the stateDB might not
		// have it. During the commit of this block, no update would
		// happen in the pvtdata store as it already has the required data.

		// if there is any missing pvtData, reconciler will fetch them
		// and update both the pvtdataStore and stateDB. Hence, we can
		// fetch what is available in the pvtDataStore. If any or
		// all of the pvtdata associated with the block got expired
		// and no longer available in pvtdataStore, eventually these
		// pvtdata would get expired in the stateDB as well (though it
		// would miss the pvtData until then)
		txPvtData, err := l.blockStore.GetPvtDataByNum(blockNo, nil)
		if err != nil {
			return err
		}
		pvtdataAndBlock.PvtData = convertTxPvtDataArrayToMap(txPvtData)
	}

	logger.Debugf("[%s] Validating state for block [%d]", l.ledgerID, blockNo)
	txstatsInfo, updateBatchBytes, err := l.txtmgmt.ValidateAndPrepare(pvtdataAndBlock, true)
	if err != nil {
		return err
	}
	elapsedBlockProcessing := time.Since(startBlockProcessing)

	startBlockstorageAndPvtdataCommit := time.Now()
	logger.Debugf("[%s] Adding CommitHash to the block [%d]", l.ledgerID, blockNo)
	// we need to ensure that only after a gensis block, commitHash is computed
	// and added to the block. In other words, only after joining a new channel
	// or peer reset, the commitHash would be added to the block
	if block.Header.Number == 1 || l.commitHash != nil {
		l.addBlockCommitHash(pvtdataAndBlock.Block, updateBatchBytes)
	}

	logger.Debugf("[%s] Committing block [%d] to storage", l.ledgerID, blockNo)
	l.blockAPIsRWLock.Lock()
	defer l.blockAPIsRWLock.Unlock()
	if err = l.blockStore.CommitWithPvtData(pvtdataAndBlock); err != nil {
		return err
	}
	elapsedBlockstorageAndPvtdataCommit := time.Since(startBlockstorageAndPvtdataCommit)

	startCommitState := time.Now()
	logger.Debugf("[%s] Committing block [%d] transactions to state database", l.ledgerID, blockNo)
	if err = l.txtmgmt.Commit(); err != nil {
		panic(errors.WithMessage(err, "error during commit to txmgr"))
	}
	elapsedCommitState := time.Since(startCommitState)

	// History database could be written in parallel with state and/or async as a future optimization,
	// although it has not been a bottleneck...no need to clutter the log with elapsed duration.
	if ledgerconfig.IsHistoryDBEnabled() {
		logger.Debugf("[%s] Committing block [%d] transactions to history database", l.ledgerID, blockNo)
		if err := l.historyDB.Commit(block); err != nil {
			panic(errors.WithMessage(err, "Error during commit to history db"))
		}
	}

	logger.Infof("[%s] Committed block [%d] with %d transaction(s) in %dms (state_validation=%dms block_and_pvtdata_commit=%dms state_commit=%dms)"+
		" commitHash=[%x]",
		l.ledgerID, block.Header.Number, len(block.Data.Data),
		time.Since(startBlockProcessing)/time.Millisecond,
		elapsedBlockProcessing/time.Millisecond,
		elapsedBlockstorageAndPvtdataCommit/time.Millisecond,
		elapsedCommitState/time.Millisecond,
		l.commitHash,
	)
	l.updateBlockStats(
		elapsedBlockProcessing,
		elapsedBlockstorageAndPvtdataCommit,
		elapsedCommitState,
		txstatsInfo,
	)
	return nil
}

func convertTxPvtDataArrayToMap(txPvtData []*ledger.TxPvtData) ledger.TxPvtDataMap {
	txPvtDataMap := make(ledger.TxPvtDataMap)
	for _, pvtData := range txPvtData {
		txPvtDataMap[pvtData.SeqInBlock] = pvtData
	}
	return txPvtDataMap
}

func (l *kvLedger) updateBlockStats(
	blockProcessingTime time.Duration,
	blockstorageAndPvtdataCommitTime time.Duration,
	statedbCommitTime time.Duration,
	txstatsInfo []*txmgr.TxStatInfo,
) {
	l.stats.updateBlockProcessingTime(blockProcessingTime)
	l.stats.updateBlockstorageAndPvtdataCommitTime(blockstorageAndPvtdataCommitTime)
	l.stats.updateStatedbCommitTime(statedbCommitTime)
	l.stats.updateTransactionsStats(txstatsInfo)
}

// GetMissingPvtDataInfoForMostRecentBlocks returns the missing private data information for the
// most recent `maxBlock` blocks which miss at least a private data of a eligible collection.
func (l *kvLedger) GetMissingPvtDataInfoForMostRecentBlocks(maxBlock int) (ledger.MissingPvtDataInfo, error) {
	// the missing pvtData info in the pvtdataStore could belong to a block which is yet
	// to be processed and committed to the blockStore and stateDB.
	// In such cases, we cannot return missing pvtData info. Otherwise, we would end up in
	// an inconsistent state database.
	if l.blockStore.IsPvtStoreAheadOfBlockStore() {
		return nil, nil
	}
	return l.blockStore.GetMissingPvtDataInfoForMostRecentBlocks(maxBlock)
}

func (l *kvLedger) addBlockCommitHash(block *common.Block, updateBatchBytes []byte) {
	var valueBytes []byte

	txValidationCode := block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]
	valueBytes = append(valueBytes, proto.EncodeVarint(uint64(len(txValidationCode)))...)
	valueBytes = append(valueBytes, txValidationCode...)
	valueBytes = append(valueBytes, updateBatchBytes...)
	valueBytes = append(valueBytes, l.commitHash...)

	l.commitHash = util.ComputeSHA256(valueBytes)
	block.Metadata.Metadata[common.BlockMetadataIndex_COMMIT_HASH] = utils.MarshalOrPanic(&common.Metadata{Value: l.commitHash})
}

// GetPvtDataAndBlockByNum returns the block and the corresponding pvt data.
// The pvt data is filtered by the list of 'collections' supplied
func (l *kvLedger) GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error) {
	blockAndPvtdata, err := l.blockStore.GetPvtDataAndBlockByNum(blockNum, filter)
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return blockAndPvtdata, err
}

// GetPvtDataByNum returns only the pvt data  corresponding to the given block number
// The pvt data is filtered by the list of 'collections' supplied
func (l *kvLedger) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	pvtdata, err := l.blockStore.GetPvtDataByNum(blockNum, filter)
	l.blockAPIsRWLock.RLock()
	l.blockAPIsRWLock.RUnlock()
	return pvtdata, err
}

// DoesPvtDataInfoExist returns true when
// (1) the ledger has pvtdata associated with the given block number (or)
// (2) a few or all pvtdata associated with the given block number is missing but the
//     missing info is recorded in the ledger (or)
// (3) the block is committed does not contain any pvtData.
func (l *kvLedger) DoesPvtDataInfoExist(blockNum uint64) (bool, error) {
	return l.blockStore.DoesPvtDataInfoExist(blockNum)
}

// Purge removes private read-writes set generated by endorsers at block height lesser than
// a given maxBlockNumToRetain. In other words, Purge only retains private read-write sets
// that were generated at block height of maxBlockNumToRetain or higher.
func (l *kvLedger) PurgePrivateData(maxBlockNumToRetain uint64) error {
	return errors.New("not yet implemented")
}

// PrivateDataMinBlockNum returns the lowest retained endorsement block height
func (l *kvLedger) PrivateDataMinBlockNum() (uint64, error) {
	return 0, errors.New("not yet implemented")
}

func (l *kvLedger) GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error) {
	return l.configHistoryRetriever, nil
}

func (l *kvLedger) CommitPvtDataOfOldBlocks(pvtData []*ledger.BlockPvtData) ([]*ledger.PvtdataHashMismatch, error) {
	logger.Debugf("[%s:] Comparing pvtData of [%d] old blocks against the hashes in transaction's rwset to find valid and invalid data",
		l.ledgerID, len(pvtData))

	hashVerifiedPvtData, hashMismatches, err := constructValidAndInvalidPvtData(pvtData, l.blockStore)
	if err != nil {
		return nil, err
	}

	logger.Debugf("[%s:] Committing pvtData of [%d] old blocks to the pvtdatastore", l.ledgerID, len(pvtData))
	err = l.blockStore.CommitPvtDataOfOldBlocks(hashVerifiedPvtData)
	if err != nil {
		return nil, err
	}

	err = l.applyValidTxPvtDataOfOldBlocks(hashVerifiedPvtData)
	if err != nil {
		return nil, err
	}

	return hashMismatches, nil
}

func (l *kvLedger) applyValidTxPvtDataOfOldBlocks(hashVerifiedPvtData map[uint64][]*ledger.TxPvtData) error {
	logger.Debugf("[%s:] Filtering pvtData of invalidation transactions", l.ledgerID)
	committedPvtData, err := filterPvtDataOfInvalidTx(hashVerifiedPvtData, l.blockStore)
	if err != nil {
		return err
	}

	logger.Debugf("[%s:] Committing pvtData of [%d] old blocks to the stateDB", l.ledgerID, len(hashVerifiedPvtData))
	err = l.txtmgmt.RemoveStaleAndCommitPvtDataOfOldBlocks(committedPvtData)
	if err != nil {
		return err
	}

	logger.Debugf("[%s:] Clearing the bookkeeping information from pvtdatastore", l.ledgerID)
	if err := l.blockStore.ResetLastUpdatedOldBlocksList(); err != nil {
		return err
	}
	return nil
}

func (l *kvLedger) GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error) {
	return l, nil
}

// Close closes `KVLedger`
func (l *kvLedger) Close() {
	l.blockStore.Shutdown()
	l.txtmgmt.Shutdown()
}

type blocksItr struct {
	blockAPIsRWLock *sync.RWMutex
	blocksItr       commonledger.ResultsIterator
}

func (itr *blocksItr) Next() (commonledger.QueryResult, error) {
	block, err := itr.blocksItr.Next()
	if err != nil {
		return nil, err
	}
	itr.blockAPIsRWLock.RLock()
	itr.blockAPIsRWLock.RUnlock()
	return block, nil
}

func (itr *blocksItr) Close() {
	itr.blocksItr.Close()
}

type collectionInfoRetriever struct {
	ledger       ledger.PeerLedger
	infoProvider ledger.DeployedChaincodeInfoProvider
}

func (r *collectionInfoRetriever) CollectionInfo(chaincodeName, collectionName string) (*common.StaticCollectionConfig, error) {
	qe, err := r.ledger.NewQueryExecutor()
	if err != nil {
		return nil, err
	}
	defer qe.Done()
	return r.infoProvider.CollectionInfo(chaincodeName, collectionName, qe)
}

func filterPvtDataOfInvalidTx(hashVerifiedPvtData map[uint64][]*ledger.TxPvtData, blockStore *ledgerstorage.Store) (map[uint64][]*ledger.TxPvtData, error) {
	committedPvtData := make(map[uint64][]*ledger.TxPvtData)
	for blkNum, txsPvtData := range hashVerifiedPvtData {

		// TODO: Instead of retrieving the whole block, we need to retireve only
		// the TxValidationFlags from the block metadata. For that, we would need
		// to add a new index for the block metadata. FAB- FAB-15808
		block, err := blockStore.RetrieveBlockByNumber(blkNum)
		if err != nil {
			return nil, err
		}
		blockValidationFlags := lutil.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

		var blksPvtData []*ledger.TxPvtData
		for _, pvtData := range txsPvtData {
			if blockValidationFlags.IsValid(int(pvtData.SeqInBlock)) {
				blksPvtData = append(blksPvtData, pvtData)
			}
		}
		committedPvtData[blkNum] = blksPvtData
	}
	return committedPvtData, nil
}
