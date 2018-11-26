/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerstorage

import (
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
	lutil "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
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
	pvtdataStore pvtdatastorage.Store
	rwlock       *sync.RWMutex
}

// NewProvider returns the handle to the provider
func NewProvider() *Provider {
	// Initialize the block storage
	attrsToIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockHash,
		blkstorage.IndexableAttrBlockNum,
		blkstorage.IndexableAttrTxID,
		blkstorage.IndexableAttrBlockNumTranNum,
		blkstorage.IndexableAttrBlockTxID,
		blkstorage.IndexableAttrTxValidationCode,
	}
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	blockStoreProvider := fsblkstorage.NewProvider(
		fsblkstorage.NewConf(ledgerconfig.GetBlockStorePath(), ledgerconfig.GetMaxBlockfileSize()),
		indexConfig)

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
	store := &Store{blockStore, pvtdataStore, &sync.RWMutex{}}
	if err := store.init(); err != nil {
		return nil, err
	}
	return store, nil
}

// Close closes the provider
func (p *Provider) Close() {
	p.blkStoreProvider.Close()
	p.pvtdataStoreProvider.Close()
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
		// as the ledger has already validated all txs in this block, we need to
		// use the validated info to commit only the pvtData of valid tx.
		// TODO: FAB-12924 Having said the above, there is a corner case that we
		// need to think about. If a state fork occurs during a regular block commit,
		// we have a mechanism to drop all blocks followed by refetching of blocks
		// and re-processing them. In the current way of doing this, we only drop
		// the block files (and related artifacts) but we do not drop/overwrite the
		// pvtdatastorage - because the assumption so far was to store full data
		// (for valid and invalid transactions). Now, we will have to allow dropping
		// of pvtdatastorage as well. However, the issue is that its shared across
		// channels (unlike block files).
		// The side effect of not dropping pvtdatastorage is that we may actually
		// have some missing data entries sitting in the pvtdatastore for the invalid
		// transactions which break our goal of storing only the pvtdata of valid tx.
		// We might also miss pvtData of a valid transaction. Note that the
		// RemoveStaleAndCommitPvtDataOfOldBlocks() in stateDB txmgr expects only
		// valid transactions' pvtdata. Hence, it is necessary to rebuild pvtdatastore
		// along with the blockstore to keep only valid tx data in the pvtdatastore.
		validTxPvtData, validTxMissingPvtData := constructValidTxPvtDataAndMissingData(blockAndPvtdata)
		if err := s.pvtdataStore.Prepare(blockAndPvtdata.Block.Header.Number, validTxPvtData, validTxMissingPvtData); err != nil {
			return err
		}
		writtenToPvtStore = true
	} else {
		logger.Debugf("Skipping writing block [%d] to pvt block store as the store height is [%d]", blockNum, pvtBlkStoreHt)
	}

	if err := s.AddBlock(blockAndPvtdata.Block); err != nil {
		s.pvtdataStore.Rollback()
		return err
	}

	if writtenToPvtStore {
		return s.pvtdataStore.Commit()
	}
	return nil
}

func constructValidTxPvtDataAndMissingData(blockAndPvtData *ledger.BlockAndPvtData) ([]*ledger.TxPvtData,
	ledger.TxMissingPvtDataMap) {

	var validTxPvtData []*ledger.TxPvtData
	validTxMissingPvtData := make(ledger.TxMissingPvtDataMap)

	txsFilter := lutil.TxValidationFlags(blockAndPvtData.Block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	numTxs := uint64(len(blockAndPvtData.Block.Data.Data))

	// for all valid tx, construct pvtdata and missing pvtdata list
	for txNum := uint64(0); txNum < numTxs; txNum++ {
		if txsFilter.IsInvalid(int(txNum)) {
			continue
		}

		if pvtdata, ok := blockAndPvtData.PvtData[txNum]; ok {
			validTxPvtData = append(validTxPvtData, pvtdata)
		}

		if missingPvtData, ok := blockAndPvtData.MissingPvtData[txNum]; ok {
			for _, missing := range missingPvtData {
				validTxMissingPvtData.Add(txNum, missing.Namespace,
					missing.Collection, missing.IsEligible)
			}
		}
	}
	return validTxPvtData, validTxMissingPvtData
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
	return s.syncPvtdataStoreWithBlockStore()
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

// syncPvtdataStoreWithBlockStore checks whether the block storage and pvt data store are in sync
// this is called when the store instance is constructed and handed over for the use.
// this check whether there is a pending batch (possibly from a previous system crash)
// of pvt data that was not committed. If a pending batch exists, the check is made
// whether the associated block was successfully committed in the block storage (before the crash)
// or not. If the block was committed, the private data batch is committed
// otherwise, the pvt data batch is rolledback
func (s *Store) syncPvtdataStoreWithBlockStore() error {
	var pendingPvtbatch bool
	var err error
	if pendingPvtbatch, err = s.pvtdataStore.HasPendingBatch(); err != nil {
		return err
	}
	if !pendingPvtbatch {
		return nil
	}
	var bcInfo *common.BlockchainInfo
	var pvtdataStoreHt uint64

	if bcInfo, err = s.GetBlockchainInfo(); err != nil {
		return err
	}
	if pvtdataStoreHt, err = s.pvtdataStore.LastCommittedBlockHeight(); err != nil {
		return err
	}

	if bcInfo.Height == pvtdataStoreHt {
		return s.pvtdataStore.Rollback()
	}

	if bcInfo.Height == pvtdataStoreHt+1 {
		return s.pvtdataStore.Commit()
	}

	return errors.Errorf("This is not expected. blockStoreHeight=%d, pvtdataStoreHeight=%d", bcInfo.Height, pvtdataStoreHt)
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
