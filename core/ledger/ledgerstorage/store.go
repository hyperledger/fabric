/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerstorage

import (
	"sync"
	"sync/atomic"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
	"github.com/hyperledger/fabric/protos/common"
)

var logger = flogging.MustGetLogger("ledgerstorage")

// Provider encapusaltes two providers 1) block store provider and 2) and pvt data store provider
type Provider struct {
	blkStoreProvider     blkstorage.BlockStoreProvider
	pvtdataStoreProvider pvtdatastorage.Provider
}

// Store encapsulates two stores 1) block store and pvt data store
type Store struct {
	blkstorage.BlockStore
	pvtdataStore                pvtdatastorage.Store
	rwlock                      sync.RWMutex
	isPvtstoreAheadOfBlockstore atomic.Value
}

var attrsToIndex = []blkstorage.IndexableAttr{
	blkstorage.IndexableAttrBlockHash,
	blkstorage.IndexableAttrBlockNum,
	blkstorage.IndexableAttrTxID,
	blkstorage.IndexableAttrBlockNumTranNum,
	//BlockTxID index is necessary to detect duplicateTxID during rollback
	blkstorage.IndexableAttrBlockTxID,
	blkstorage.IndexableAttrTxValidationCode,
}

// NewProvider returns the handle to the provider
func NewProvider(metricsProvider metrics.Provider) *Provider {
	// Initialize the block storage
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	blockStoreProvider := fsblkstorage.NewProvider(
		fsblkstorage.NewConf(ledgerconfig.GetBlockStorePath(), ledgerconfig.GetMaxBlockfileSize()),
		indexConfig,
		metricsProvider)

	pvtStoreProvider := pvtdatastorage.NewProvider()
	return &Provider{blockStoreProvider, pvtStoreProvider}
}

// Open opens the store
func (p *Provider) Open(ledgerid string) (*Store, error) {
	var blockStore blkstorage.BlockStore
	var pvtdataStore pvtdatastorage.Store
	var err error

	if blockStore, err = p.blkStoreProvider.OpenBlockStore(ledgerid); err != nil {
		return nil, err
	}
	if pvtdataStore, err = p.pvtdataStoreProvider.OpenStore(ledgerid); err != nil {
		return nil, err
	}
	store := &Store{
		BlockStore:   blockStore,
		pvtdataStore: pvtdataStore,
	}
	if err := store.init(); err != nil {
		return nil, err
	}

	info, err := blockStore.GetBlockchainInfo()
	if err != nil {
		return nil, err
	}
	pvtstoreHeight, err := pvtdataStore.LastCommittedBlockHeight()
	if err != nil {
		return nil, err
	}
	store.isPvtstoreAheadOfBlockstore.Store(pvtstoreHeight > info.Height)

	return store, nil
}

// Close closes the provider
func (p *Provider) Close() {
	p.blkStoreProvider.Close()
	p.pvtdataStoreProvider.Close()
}

// Exists checks whether the ledgerID already presents
func (p *Provider) Exists(ledgerID string) (bool, error) {
	return p.blkStoreProvider.Exists(ledgerID)
}

// Init initializes store with essential configurations
func (s *Store) Init(btlPolicy pvtdatapolicy.BTLPolicy) {
	s.pvtdataStore.Init(btlPolicy)
}

// CommitWithPvtData commits the block and the corresponding pvt data in an atomic operation
func (s *Store) CommitWithPvtData(blockAndPvtdata *ledger.BlockAndPvtData) error {
	blockNum := blockAndPvtdata.Block.Header.Number
	s.rwlock.Lock()
	defer s.rwlock.Unlock()

	pvtBlkStoreHt, err := s.pvtdataStore.LastCommittedBlockHeight()
	if err != nil {
		return err
	}

	writtenToPvtStore := false
	if pvtBlkStoreHt < blockNum+1 { // The pvt data store sanity check does not allow rewriting the pvt data.
		// when re-processing blocks (rejoin the channel or re-fetching last few block),
		// skip the pvt data commit to the pvtdata blockstore
		logger.Debugf("Writing block [%d] to pvt block store", blockNum)
		// If a state fork occurs during a regular block commit,
		// we have a mechanism to drop all blocks followed by refetching of blocks
		// and re-processing them. In the current way of doing this, we only drop
		// the block files (and related artifacts) but we do not drop/overwrite the
		// pvtdatastorage as it might leads to data loss.
		// During block reprocessing, as there is a possibility of an invalid pvtdata
		// transaction to become valid, we store the pvtdata of invalid transactions
		// too in the pvtdataStore as we do for the publicdata in the case of blockStore.
		pvtData, missingPvtData := constructPvtDataAndMissingData(blockAndPvtdata)
		if err := s.pvtdataStore.Prepare(blockAndPvtdata.Block.Header.Number, pvtData, missingPvtData); err != nil {
			return err
		}
		writtenToPvtStore = true
	} else {
		logger.Debugf("Skipping writing block [%d] to pvt block store as the store height is [%d]", blockNum, pvtBlkStoreHt)
	}

	if err := s.AddBlock(blockAndPvtdata.Block); err != nil {
		return err
	}

	if pvtBlkStoreHt == blockNum+1 {
		// we reach here only when the pvtdataStore was ahead
		// of blockStore during the store opening time (would
		// occur after a peer rollback/reset).
		s.isPvtstoreAheadOfBlockstore.Store(false)
	}

	if writtenToPvtStore {
		return s.pvtdataStore.Commit()
	}
	return nil
}

func constructPvtDataAndMissingData(blockAndPvtData *ledger.BlockAndPvtData) ([]*ledger.TxPvtData,
	ledger.TxMissingPvtDataMap) {

	var pvtData []*ledger.TxPvtData
	missingPvtData := make(ledger.TxMissingPvtDataMap)

	numTxs := uint64(len(blockAndPvtData.Block.Data.Data))

	// for all tx, construct pvtdata and missing pvtdata list
	for txNum := uint64(0); txNum < numTxs; txNum++ {
		if pvtdata, ok := blockAndPvtData.PvtData[txNum]; ok {
			pvtData = append(pvtData, pvtdata)
		}

		if missingData, ok := blockAndPvtData.MissingPvtData[txNum]; ok {
			for _, missing := range missingData {
				missingPvtData.Add(txNum, missing.Namespace,
					missing.Collection, missing.IsEligible)
			}
		}
	}
	return pvtData, missingPvtData
}

// CommitPvtDataOfOldBlocks commits the pvtData of old blocks
func (s *Store) CommitPvtDataOfOldBlocks(blocksPvtData map[uint64][]*ledger.TxPvtData) error {
	err := s.pvtdataStore.CommitPvtDataOfOldBlocks(blocksPvtData)
	if err != nil {
		return err
	}
	return nil
}

// GetPvtDataAndBlockByNum returns the block and the corresponding pvt data.
// The pvt data is filtered by the list of 'collections' supplied
func (s *Store) GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error) {
	s.rwlock.RLock()
	defer s.rwlock.RUnlock()

	var block *common.Block
	var pvtdata []*ledger.TxPvtData
	var err error
	if block, err = s.RetrieveBlockByNumber(blockNum); err != nil {
		return nil, err
	}
	if pvtdata, err = s.getPvtDataByNumWithoutLock(blockNum, filter); err != nil {
		return nil, err
	}
	return &ledger.BlockAndPvtData{Block: block, PvtData: constructPvtdataMap(pvtdata)}, nil
}

// GetPvtDataByNum returns only the pvt data  corresponding to the given block number
// The pvt data is filtered by the list of 'ns/collections' supplied in the filter
// A nil filter does not filter any results
func (s *Store) GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	s.rwlock.RLock()
	defer s.rwlock.RUnlock()
	return s.getPvtDataByNumWithoutLock(blockNum, filter)
}

// getPvtDataByNumWithoutLock returns only the pvt data  corresponding to the given block number.
// This function does not acquire a readlock and it is expected that in most of the circumstances, the caller
// possesses a read lock on `s.rwlock`
func (s *Store) getPvtDataByNumWithoutLock(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	var pvtdata []*ledger.TxPvtData
	var err error
	if pvtdata, err = s.pvtdataStore.GetPvtDataByBlockNum(blockNum, filter); err != nil {
		return nil, err
	}
	return pvtdata, nil
}

// DoesPvtDataInfoExist returns true when
// (1) the ledger has pvtdata associated with the given block number (or)
// (2) a few or all pvtdata associated with the given block number is missing but the
//     missing info is recorded in the ledger (or)
// (3) the block is committed does not contain any pvtData.
func (s *Store) DoesPvtDataInfoExist(blockNum uint64) (bool, error) {
	pvtStoreHt, err := s.pvtdataStore.LastCommittedBlockHeight()
	if err != nil {
		return false, err
	}
	return blockNum+1 <= pvtStoreHt, nil
}

// GetMissingPvtDataInfoForMostRecentBlocks invokes the function on underlying pvtdata store
func (s *Store) GetMissingPvtDataInfoForMostRecentBlocks(maxBlock int) (ledger.MissingPvtDataInfo, error) {
	// it is safe to not acquire a read lock on s.rwlock. Without a lock, the value of
	// lastCommittedBlock can change due to a new block commit. As a result, we may not
	// be able to fetch the missing data info of truly the most recent blocks. This
	// decision was made to ensure that the regular block commit rate is not affected.
	return s.pvtdataStore.GetMissingPvtDataInfoForMostRecentBlocks(maxBlock)
}

// ProcessCollsEligibilityEnabled invokes the function on underlying pvtdata store
func (s *Store) ProcessCollsEligibilityEnabled(committingBlk uint64, nsCollMap map[string][]string) error {
	return s.pvtdataStore.ProcessCollsEligibilityEnabled(committingBlk, nsCollMap)
}

// GetLastUpdatedOldBlocksPvtData invokes the function on underlying pvtdata store
func (s *Store) GetLastUpdatedOldBlocksPvtData() (map[uint64][]*ledger.TxPvtData, error) {
	return s.pvtdataStore.GetLastUpdatedOldBlocksPvtData()
}

// ResetLastUpdatedOldBlocksList invokes the function on underlying pvtdata store
func (s *Store) ResetLastUpdatedOldBlocksList() error {
	return s.pvtdataStore.ResetLastUpdatedOldBlocksList()
}

// IsPvtStoreAheadOfBlockStore returns true when the pvtStore height is
// greater than the blockstore height. Otherwise, it returns false.
func (s *Store) IsPvtStoreAheadOfBlockStore() bool {
	return s.isPvtstoreAheadOfBlockstore.Load().(bool)
}

// init first invokes function `initFromExistingBlockchain`
// in order to check whether the pvtdata store is present because of an upgrade
// of peer from 1.0 and need to be updated with the existing blockchain. If, this is
// not the case then this init will invoke function `syncPvtdataStoreWithBlockStore`
// to follow the normal course
func (s *Store) init() error {
	var initialized bool
	var err error
	if initialized, err = s.initPvtdataStoreFromExistingBlockchain(); err != nil || initialized {
		return err
	}
	return s.commitPendingBatchInPvtdataStore()
}

// initPvtdataStoreFromExistingBlockchain updates the initial state of the pvtdata store
// if an existing block store has a blockchain and the pvtdata store is empty.
// This situation is expected to happen when a peer is upgrated from version 1.0
// and an existing blockchain is present that was generated with version 1.0.
// Under this scenario, the pvtdata store is brought upto the point as if it has
// processed existing blocks with no pvt data. This function returns true if the
// above mentioned condition is found to be true and pvtdata store is successfully updated
func (s *Store) initPvtdataStoreFromExistingBlockchain() (bool, error) {
	var bcInfo *common.BlockchainInfo
	var pvtdataStoreEmpty bool
	var err error

	if bcInfo, err = s.BlockStore.GetBlockchainInfo(); err != nil {
		return false, err
	}
	if pvtdataStoreEmpty, err = s.pvtdataStore.IsEmpty(); err != nil {
		return false, err
	}
	if pvtdataStoreEmpty && bcInfo.Height > 0 {
		if err = s.pvtdataStore.InitLastCommittedBlock(bcInfo.Height - 1); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// commitPendingBatchInPvtdataStore checks whether there is a pending batch
// (possibly from a previous system crash) of pvt data that was not committed.
// If a pending batch exists, the batch is committed.
func (s *Store) commitPendingBatchInPvtdataStore() error {
	var pendingPvtbatch bool
	var err error
	if pendingPvtbatch, err = s.pvtdataStore.HasPendingBatch(); err != nil {
		return err
	}
	if !pendingPvtbatch {
		return nil
	}

	// we can safetly commit the pending batch as gossip would avoid
	// fetching pvtData if already exist in the local pvtdataStore.
	// when the pvtdataStore height is greater than the blockstore,
	// pvtdata reconciler will not fetch any missing pvtData.
	return s.pvtdataStore.Commit()
}

func constructPvtdataMap(pvtdata []*ledger.TxPvtData) map[uint64]*ledger.TxPvtData {
	if pvtdata == nil {
		return nil
	}
	m := make(map[uint64]*ledger.TxPvtData)
	for _, pvtdatum := range pvtdata {
		m[pvtdatum.SeqInBlock] = pvtdatum
	}
	return m
}

// LoadPreResetHeight returns the pre reset height.
func LoadPreResetHeight(blockstorePath string) (map[string]uint64, error) {
	return fsblkstorage.LoadPreResetHeight(blockstorePath)
}

// ResetBlockStore resets all ledgers to the genesis block.
func ResetBlockStore(blockstorePath string) error {
	return fsblkstorage.ResetBlockStore(blockstorePath)
}

// ValidateRollbackParams performs necessary validation on the input given for
// the rollback operation.
func ValidateRollbackParams(blockstorePath, ledgerID string, blockNum uint64) error {
	return fsblkstorage.ValidateRollbackParams(blockstorePath, ledgerID, blockNum)
}

// Rollback reverts changes made to the block store beyond a given block number.
func Rollback(blockstorePath, ledgerID string, blockNum uint64) error {
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	return fsblkstorage.Rollback(blockstorePath, ledgerID, blockNum, indexConfig)
}
