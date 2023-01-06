/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bits-and-blooms/bitset"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("pvtdatastorage")

// Provider provides handle to specific 'Store' that in turn manages
// private write sets for a ledger
type Provider struct {
	dbProvider *leveldbhelper.Provider
	pvtData    *PrivateDataConfig
}

// PrivateDataConfig encapsulates the configuration for private data storage on the ledger
type PrivateDataConfig struct {
	// PrivateDataConfig is used to configure a private data storage provider
	*ledger.PrivateDataConfig
	// StorePath is the filesystem path for private data storage.
	// It is internally computed by the ledger component,
	// so it is not in ledger.PrivateDataConfig and not exposed to other components.
	StorePath string
}

// Store manages the permanent storage of private write sets for a ledger
type Store struct {
	db                    *leveldbhelper.DBHandle
	ledgerid              string
	btlPolicy             pvtdatapolicy.BTLPolicy
	batchesInterval       int
	maxBatchSize          int
	purgeInterval         uint64
	purgedKeyAuditLogging bool

	isEmpty            bool
	lastCommittedBlock uint64
	bootsnapshotInfo   *bootsnapshotInfo

	purgerLock      sync.Mutex
	collElgProcSync *collElgProcSync
	// After committing the pvtdata of old blocks,
	// the `isLastUpdatedOldBlocksSet` is set to true.
	// Once the stateDB is updated with these pvtdata,
	// the `isLastUpdatedOldBlocksSet` is set to false.
	// isLastUpdatedOldBlocksSet is mainly used during the
	// recovery process. During the peer startup, if the
	// isLastUpdatedOldBlocksSet is set to true, the pvtdata
	// in the stateDB needs to be updated before finishing the
	// recovery operation.
	isLastUpdatedOldBlocksSet bool

	deprioritizedDataReconcilerInterval time.Duration
	accessDeprioMissingDataAfter        time.Time
}

type bootsnapshotInfo struct {
	createdFromSnapshot bool
	lastBlockInSnapshot uint64
}

type blkTranNumKey []byte

type PurgeMarker struct {
	Ns, Coll   string
	PvtkeyHash []byte
	TxNum      uint64
}

type dataEntry struct {
	key   *dataKey
	value *rwset.CollectionPvtReadWriteSet
}

type hashedIndexEntry struct {
	key   *hashedIndexKey
	value string
}

type purgeMarkerEntry struct {
	key   *purgeMarkerKey
	value *purgeMarkerVal
}

type purgeMarkerCollEntry struct {
	key   *purgeMarkerCollKey
	value *purgeMarkerVal
}

type expiryEntry struct {
	key   *expiryKey
	value *ExpiryData
}

type expiryKey struct {
	expiringBlk   uint64
	committingBlk uint64
}

type nsCollBlk struct {
	ns, coll string
	blkNum   uint64
}

type dataKey struct {
	nsCollBlk
	txNum uint64
}

type missingDataKey struct {
	nsCollBlk
}

type bootKVHashesKey struct {
	blkNum uint64
	txNum  uint64
	ns     string
	coll   string
}

type hashedIndexKey struct {
	ns, coll      string
	pvtkeyHash    []byte
	blkNum, txNum uint64
}

type purgeMarkerKey struct {
	ns, coll   string
	pvtkeyHash []byte
}

type purgeMarkerVal struct {
	blkNum, txNum uint64
}

type purgeMarkerCollKey struct {
	ns, coll string
}

type storeEntries struct {
	dataEntries             []*dataEntry
	hashedIndexEntries      []*hashedIndexEntry
	purgeMarkerEntries      []*purgeMarkerEntry
	purgeMarkerCollEntries  []*purgeMarkerCollEntry
	expiryEntries           []*expiryEntry
	elgMissingDataEntries   map[missingDataKey]*bitset.BitSet
	inelgMissingDataEntries map[missingDataKey]*bitset.BitSet
}

// lastUpdatedOldBlocksList keeps the list of last updated blocks
// and is stored as the value of lastUpdatedOldBlocksKey (defined in kv_encoding.go)
type lastUpdatedOldBlocksList []uint64

//////// Provider functions  /////////////
//////////////////////////////////////////

// NewProvider instantiates a StoreProvider
func NewProvider(conf *PrivateDataConfig) (*Provider, error) {
	dbProvider, err := leveldbhelper.NewProvider(
		&leveldbhelper.Conf{
			DBPath:         conf.StorePath,
			ExpectedFormat: currentDataVersion,
		})
	if err != nil {
		return nil, err
	}
	return &Provider{
		dbProvider: dbProvider,
		pvtData:    conf,
	}, nil
}

// SnapshotDataImporterFor returns an implementation of interface privacyenabledstate.SnapshotPvtdataHashesConsumer
// The returned struct is expected to be registered for receiving the pvtdata hashes from snapshot and loads the data
// into pvtdata store.
func (p *Provider) SnapshotDataImporterFor(
	ledgerID string,
	lastBlockInSnapshot uint64,
	membershipProvider ledger.MembershipInfoProvider,
	configHistoryRetriever *confighistory.Retriever,
	tempDirRoot string,
) (*SnapshotDataImporter, error) {
	db := p.dbProvider.GetDBHandle(ledgerID)
	batch := db.NewUpdateBatch()
	batch.Put(lastBlockInBootSnapshotKey, encodeLastBlockInBootSnapshotVal(lastBlockInSnapshot))
	batch.Put(lastCommittedBlkkey, encodeLastCommittedBlockVal(lastBlockInSnapshot))
	if err := db.WriteBatch(batch, true); err != nil {
		return nil, errors.WithMessage(err, "error while writing snapshot info to db")
	}

	return newSnapshotDataImporter(
		ledgerID,
		p.dbProvider.GetDBHandle(ledgerID),
		membershipProvider,
		configHistoryRetriever,
		tempDirRoot,
	)
}

// OpenStore returns a handle to a store
func (p *Provider) OpenStore(ledgerid string) (*Store, error) {
	dbHandle := p.dbProvider.GetDBHandle(ledgerid)
	s := &Store{
		db:                                  dbHandle,
		ledgerid:                            ledgerid,
		batchesInterval:                     p.pvtData.BatchesInterval,
		maxBatchSize:                        p.pvtData.MaxBatchSize,
		purgeInterval:                       uint64(p.pvtData.PurgeInterval),
		purgedKeyAuditLogging:               p.pvtData.PurgedKeyAuditLogging,
		deprioritizedDataReconcilerInterval: p.pvtData.DeprioritizedDataReconcilerInterval,
		accessDeprioMissingDataAfter:        time.Now().Add(p.pvtData.DeprioritizedDataReconcilerInterval),
		collElgProcSync: &collElgProcSync{
			notification: make(chan bool, 1),
			procComplete: make(chan bool, 1),
		},
	}
	if err := s.initState(); err != nil {
		return nil, err
	}
	s.launchCollElgProc()
	logger.Debugf("Pvtdata store opened. Initial state: isEmpty [%t], lastCommittedBlock [%d]",
		s.isEmpty, s.lastCommittedBlock)
	return s, nil
}

// Close closes the store
func (p *Provider) Close() {
	p.dbProvider.Close()
}

// Drop drops channel-specific data from the pvtdata store
func (p *Provider) Drop(ledgerid string) error {
	return p.dbProvider.Drop(ledgerid)
}

//////// store functions  ////////////////
//////////////////////////////////////////

func (s *Store) initState() error {
	var err error
	if s.isEmpty, s.lastCommittedBlock, err = s.getLastCommittedBlockNum(); err != nil {
		return err
	}

	if s.bootsnapshotInfo, err = s.fetchBootSnapshotInfo(); err != nil {
		return err
	}

	// TODO: FAB-16298 -- the concept of pendingBatch is no longer valid
	// for pvtdataStore. We can remove it v2.1. We retain the concept in
	// v2.0 to allow rolling upgrade from v142 to v2.0
	batchPending, err := s.hasPendingCommit()
	if err != nil {
		return err
	}

	if batchPending {
		committingBlockNum := s.nextBlockNum()
		batch := s.db.NewUpdateBatch()
		batch.Put(lastCommittedBlkkey, encodeLastCommittedBlockVal(committingBlockNum))
		batch.Delete(pendingCommitKey)
		if err := s.db.WriteBatch(batch, true); err != nil {
			return err
		}
		s.isEmpty = false
		s.lastCommittedBlock = committingBlockNum
	}

	var blist lastUpdatedOldBlocksList
	if blist, err = s.getLastUpdatedOldBlocksList(); err != nil {
		return err
	}
	if len(blist) > 0 {
		s.isLastUpdatedOldBlocksSet = true
	} // false if not set

	return nil
}

// Init initializes the store. This function is expected to be invoked before using the store
func (s *Store) Init(btlPolicy pvtdatapolicy.BTLPolicy) {
	s.btlPolicy = btlPolicy
}

// Commit commits the pvt data as well as both the eligible and ineligible
// missing private data --- `eligible` denotes that the missing private data belongs to a collection
// for which this peer is a member; `ineligible` denotes that the missing private data belong to a
// collection for which this peer is not a member.
func (s *Store) Commit(blockNum uint64, pvtData []*ledger.TxPvtData, missingPvtData ledger.TxMissingPvtData, purgeMarkers []*PurgeMarker) error {
	expectedBlockNum := s.nextBlockNum()
	if expectedBlockNum != blockNum {
		return errors.Errorf("expected block number=%d, received block number=%d", expectedBlockNum, blockNum)
	}

	batch := s.db.NewUpdateBatch()
	var err error
	var key, val []byte

	storeEntries, err := prepareStoreEntries(blockNum, pvtData, s.btlPolicy, missingPvtData, purgeMarkers)
	if err != nil {
		return err
	}

	for _, dataEntry := range storeEntries.dataEntries {
		key = encodeDataKey(dataEntry.key)
		if val, err = encodeDataValue(dataEntry.value); err != nil {
			return err
		}
		batch.Put(key, val)
	}

	for _, hashedIndexEntry := range storeEntries.hashedIndexEntries {
		key := encodeHashedIndexKey(hashedIndexEntry.key)
		batch.Put(key, []byte(hashedIndexEntry.value))
	}

	for _, purgeMarkerEntry := range storeEntries.purgeMarkerEntries {
		batch.Put(
			encodePurgeMarkerKey(purgeMarkerEntry.key),
			encodePurgeMarkerVal(purgeMarkerEntry.value),
		)
		batch.Put(
			encodePurgeMarkerForReconKey(purgeMarkerEntry.key),
			encodePurgeMarkerVal(purgeMarkerEntry.value),
		)
	}

	for _, purgeMarkerCollEntry := range storeEntries.purgeMarkerCollEntries {
		batch.Put(
			encodePurgeMarkerCollKey(purgeMarkerCollEntry.key),
			encodePurgeMarkerVal(purgeMarkerCollEntry.value),
		)
	}

	for _, expiryEntry := range storeEntries.expiryEntries {
		key = encodeExpiryKey(expiryEntry.key)
		if val, err = encodeExpiryValue(expiryEntry.value); err != nil {
			return err
		}
		batch.Put(key, val)
	}

	for missingDataKey, missingDataValue := range storeEntries.elgMissingDataEntries {
		key = encodeElgPrioMissingDataKey(&missingDataKey)

		if val, err = encodeMissingDataValue(missingDataValue); err != nil {
			return err
		}
		batch.Put(key, val)
	}

	for missingDataKey, missingDataValue := range storeEntries.inelgMissingDataEntries {
		key = encodeInelgMissingDataKey(&missingDataKey)

		if val, err = encodeMissingDataValue(missingDataValue); err != nil {
			return err
		}
		batch.Put(key, val)
	}

	committingBlockNum := s.nextBlockNum()
	logger.Debugf("Committing private data for block [%d]", committingBlockNum)
	batch.Put(lastCommittedBlkkey, encodeLastCommittedBlockVal(committingBlockNum))
	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}

	s.isEmpty = false
	atomic.StoreUint64(&s.lastCommittedBlock, committingBlockNum)
	logger.Debugf("Committed private data for block [%d]", committingBlockNum)
	s.performPurgeIfScheduled(committingBlockNum)
	return nil
}

// GetLastUpdatedOldBlocksPvtData returns the pvtdata of blocks listed in `lastUpdatedOldBlocksList`
// TODO FAB-16293 -- GetLastUpdatedOldBlocksPvtData() can be removed either in v2.0 or in v2.1.
// If we decide to rebuild stateDB in v2.0, by default, the rebuild logic would take
// care of synching stateDB with pvtdataStore without calling GetLastUpdatedOldBlocksPvtData().
// Hence, it can be safely removed. Suppose if we decide not to rebuild stateDB in v2.0,
// we can remove this function in v2.1.
func (s *Store) GetLastUpdatedOldBlocksPvtData() (map[uint64][]*ledger.TxPvtData, error) {
	if !s.isLastUpdatedOldBlocksSet {
		return nil, nil
	}

	updatedBlksList, err := s.getLastUpdatedOldBlocksList()
	if err != nil {
		return nil, err
	}

	blksPvtData := make(map[uint64][]*ledger.TxPvtData)
	for _, blkNum := range updatedBlksList {
		if blksPvtData[blkNum], err = s.GetPvtDataByBlockNum(blkNum, nil); err != nil {
			return nil, err
		}
	}
	return blksPvtData, nil
}

func (s *Store) getLastUpdatedOldBlocksList() ([]uint64, error) {
	var v []byte
	var err error
	if v, err = s.db.Get(lastUpdatedOldBlocksKey); err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}

	var updatedBlksList []uint64
	buf := proto.NewBuffer(v)
	numBlks, err := buf.DecodeVarint()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(numBlks); i++ {
		blkNum, err := buf.DecodeVarint()
		if err != nil {
			return nil, err
		}
		updatedBlksList = append(updatedBlksList, blkNum)
	}
	return updatedBlksList, nil
}

// TODO FAB-16294 -- ResetLastUpdatedOldBlocksList() can be removed in v2.1.
// From v2.0 onwards, we do not store the last updatedBlksList. Only to support
// the rolling upgrade from v142 to v2.0, we retain the ResetLastUpdatedOldBlocksList()
// in v2.0.

// ResetLastUpdatedOldBlocksList removes the `lastUpdatedOldBlocksList` entry from the store
func (s *Store) ResetLastUpdatedOldBlocksList() error {
	batch := s.db.NewUpdateBatch()
	batch.Delete(lastUpdatedOldBlocksKey)
	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	s.isLastUpdatedOldBlocksSet = false
	return nil
}

// GetPvtDataByBlockNum returns only the pvt data  corresponding to the given block number
// The pvt data is filtered by the list of 'ns/collections' supplied in the filter
// A nil filter does not filter any results
func (s *Store) GetPvtDataByBlockNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	logger.Debugf("Get private data for block [%d], filter=%#v", blockNum, filter)
	if s.isEmpty {
		return nil, errors.New("the store is empty")
	}
	lastCommittedBlock := atomic.LoadUint64(&s.lastCommittedBlock)
	if blockNum > lastCommittedBlock {
		return nil, errors.Errorf("last committed block number [%d] smaller than the requested block number [%d]", lastCommittedBlock, blockNum)
	}
	startKey, endKey := getDataKeysForRangeScanByBlockNum(blockNum)
	logger.Debugf("Querying private data storage for write sets using startKey=%#v, endKey=%#v", startKey, endKey)
	itr, err := s.db.GetIterator(startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer itr.Release()

	var blockPvtdata []*ledger.TxPvtData
	var currentTxNum uint64
	var currentTxWsetAssember *txPvtdataAssembler
	firstItr := true

	for itr.Next() {
		dataKeyBytes := itr.Key()
		dataValueBytes := itr.Value()
		dataKey, err := decodeDatakey(dataKeyBytes)
		if err != nil {
			return nil, err
		}
		expired, err := isExpired(dataKey.nsCollBlk, s.btlPolicy, lastCommittedBlock)
		if err != nil {
			return nil, err
		}
		if expired || !passesFilter(dataKey, filter) {
			continue
		}

		if firstItr {
			currentTxNum = dataKey.txNum
			currentTxWsetAssember = newTxPvtdataAssembler(blockNum, currentTxNum)
			firstItr = false
		}

		if dataKey.txNum != currentTxNum {
			blockPvtdata = append(blockPvtdata, currentTxWsetAssember.getTxPvtdata())
			currentTxNum = dataKey.txNum
			currentTxWsetAssember = newTxPvtdataAssembler(blockNum, currentTxNum)
		}

		dataValue, err := decodeDataValue(dataValueBytes)
		if err != nil {
			return nil, err
		}

		if err := s.removePurgedDataFromCollPvtRWset(dataKey, dataValue); err != nil {
			return nil, err
		}

		currentTxWsetAssember.add(dataKey.ns, dataValue)
	}
	if currentTxWsetAssember != nil {
		blockPvtdata = append(blockPvtdata, currentTxWsetAssember.getTxPvtdata())
	}
	return blockPvtdata, nil
}

func (s *Store) retrieveLatestPurgeKeyCollMarkerHt(ns, coll string) (*version.Height, error) {
	encVal, err := s.db.Get(
		encodePurgeMarkerCollKey(
			&purgeMarkerCollKey{
				ns:   ns,
				coll: coll,
			},
		),
	)
	if err != nil {
		return nil, err
	}
	if encVal == nil {
		return nil, nil
	}
	return decodePurgeMarkerVal(encVal)
}

// keyPotentiallyPurged returns false if `purgeMarkerCollKey` does not exists (which means never any key is purged from the given collection)
// or the height of `purgeMarkerCollKey` is lower than the <ns, coll> in the data key (which means that the last purge of any key from the collection
// was prior to the given key commit). The main purpose of this function is to optimize while filtering the purge data by avoiding computing hashes
// of individual keys, all the time
func (s *Store) keyPotentiallyPurged(k *dataKey) (bool, error) {
	purgeKeyCollMarkerHt, err := s.retrieveLatestPurgeKeyCollMarkerHt(k.ns, k.coll)
	if purgeKeyCollMarkerHt == nil || err != nil {
		return false, err
	}

	keyHt := &version.Height{
		BlockNum: k.blkNum,
		TxNum:    k.txNum,
	}

	return keyHt.Compare(purgeKeyCollMarkerHt) <= 0, nil
}

func (s *Store) RemoveAppInitiatedPurgesUsingReconMarker(
	kvHashes map[string][]byte, ns, coll string, blkNum, txNum uint64,
) (map[string][]byte, error) {
	trimmedKVHashes := map[string][]byte{}
	keyHt := &version.Height{
		BlockNum: blkNum,
		TxNum:    txNum,
	}

	for k, v := range kvHashes {
		potentialPurgeMarker := encodePurgeMarkerForReconKey(
			&purgeMarkerKey{
				ns:         ns,
				coll:       coll,
				pvtkeyHash: []byte(k),
			},
		)

		encPurgeMarkerVal, err := s.db.Get(potentialPurgeMarker)
		if err != nil {
			return nil, err
		}

		if encPurgeMarkerVal == nil {
			trimmedKVHashes[k] = v
			continue
		}

		purgeMarkerHt, err := decodePurgeMarkerVal(encPurgeMarkerVal)
		if err != nil {
			return nil, err
		}
		if keyHt.Compare(purgeMarkerHt) > 0 {
			trimmedKVHashes[k] = v
		}
	}
	return trimmedKVHashes, nil
}

func (s *Store) removePurgedDataFromCollPvtRWset(k *dataKey, v *rwset.CollectionPvtReadWriteSet) error {
	purgePossible, err := s.keyPotentiallyPurged(k)
	if !purgePossible || err != nil {
		return err
	}

	collRWSet, err := rwsetutil.CollPvtRwSetFromProtoMsg(v)
	if err != nil {
		return err
	}

	keyHt := &version.Height{
		BlockNum: k.blkNum,
		TxNum:    k.txNum,
	}

	filterInKVWrites := []*kvrwset.KVWrite{}
	for _, w := range collRWSet.KvRwSet.Writes {
		potentialPurgeMarker := encodePurgeMarkerKey(&purgeMarkerKey{
			ns:         k.ns,
			coll:       k.coll,
			pvtkeyHash: util.ComputeStringHash(w.Key),
		})

		encPurgeMarkerVal, err := s.db.Get(potentialPurgeMarker)
		if err != nil {
			return err
		}

		if encPurgeMarkerVal == nil {
			filterInKVWrites = append(filterInKVWrites, w)
			continue
		}

		purgeMarkerHt, err := decodePurgeMarkerVal(encPurgeMarkerVal)
		if err != nil {
			return err
		}

		if keyHt.Compare(purgeMarkerHt) > 0 {
			filterInKVWrites = append(filterInKVWrites, w)
			continue
		}
	}

	collRWSet.KvRwSet.Writes = filterInKVWrites
	if v.Rwset, err = proto.Marshal(collRWSet.KvRwSet); err != nil {
		return err
	}
	return nil
}

// GetMissingPvtDataInfoForMostRecentBlocks returns the missing private data information for the
// most recent `maxBlock` blocks which miss at least a private data of a eligible collection.
func (s *Store) GetMissingPvtDataInfoForMostRecentBlocks(startingBlockNum uint64, maxBlock int) (ledger.MissingPvtDataInfo, error) {
	// we assume that this function would be called by the gossip only after processing the
	// last retrieved missing pvtdata info and committing the same.
	if maxBlock < 1 {
		return nil, nil
	}

	if time.Now().After(s.accessDeprioMissingDataAfter) {
		s.accessDeprioMissingDataAfter = time.Now().Add(s.deprioritizedDataReconcilerInterval)
		logger.Debug("fetching missing pvtdata entries from the deprioritized list")
		return s.getMissingData(elgDeprioritizedMissingDataGroup, startingBlockNum, maxBlock)
	}

	logger.Debug("fetching missing pvtdata entries from the prioritized list")
	return s.getMissingData(elgPrioritizedMissingDataGroup, startingBlockNum, maxBlock)
}

func (s *Store) getMissingData(group []byte, startingBlockNum uint64, maxBlock int) (ledger.MissingPvtDataInfo, error) {
	missingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	numberOfBlockProcessed := 0
	lastProcessedBlock := uint64(0)
	isMaxBlockLimitReached := false

	startKey, endKey := createRangeScanKeysForElgMissingData(startingBlockNum, group)
	dbItr, err := s.db.GetIterator(startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer dbItr.Release()

	for dbItr.Next() {
		missingDataKeyBytes := dbItr.Key()
		missingDataKey := decodeElgMissingDataKey(missingDataKeyBytes)

		if isMaxBlockLimitReached && (missingDataKey.blkNum != lastProcessedBlock) {
			// ensures that exactly maxBlock number
			// of blocks' entries are processed
			break
		}

		// check whether the entry is expired. If so, move to the next item.
		// As we may use the old lastCommittedBlock value, there is a possibility that
		// this missing data is actually expired but we may get the stale information.
		// Though it may leads to extra work of pulling the expired data, it will not
		// affect the correctness. Further, as we try to fetch the most recent missing
		// data (less possibility of expiring now), such scenario would be rare. In the
		// best case, we can load the latest lastCommittedBlock value here atomically to
		// make this scenario very rare.
		lastCommittedBlock := atomic.LoadUint64(&s.lastCommittedBlock)
		expired, err := isExpired(missingDataKey.nsCollBlk, s.btlPolicy, lastCommittedBlock)
		if err != nil {
			return nil, err
		}
		if expired {
			continue
		}

		// check for an existing entry for the blkNum in the MissingPvtDataInfo.
		// If no such entry exists, create one. Also, keep track of the number of
		// processed block due to maxBlock limit.
		if _, ok := missingPvtDataInfo[missingDataKey.blkNum]; !ok {
			numberOfBlockProcessed++
			if numberOfBlockProcessed == maxBlock {
				isMaxBlockLimitReached = true
				// as there can be more than one entry for this block,
				// we cannot `break` here
				lastProcessedBlock = missingDataKey.blkNum
			}
		}

		valueBytes := dbItr.Value()
		bitmap, err := decodeMissingDataValue(valueBytes)
		if err != nil {
			return nil, err
		}

		// for each transaction which misses private data, make an entry in missingBlockPvtDataInfo
		for index, isSet := bitmap.NextSet(0); isSet; index, isSet = bitmap.NextSet(index + 1) {
			txNum := uint64(index)
			missingPvtDataInfo.Add(missingDataKey.blkNum, txNum, missingDataKey.ns, missingDataKey.coll)
		}
	}

	return missingPvtDataInfo, nil
}

// FetchBootKVHashes returns the KVHashes from the data that was loaded from a snapshot at the time of
// bootstrapping. This function returns an error if the supplied blkNum is greater than the last block
// number in the booting snapshot
func (s *Store) FetchBootKVHashes(blkNum, txNum uint64, ns, coll string) (map[string][]byte, error) {
	if s.bootsnapshotInfo.createdFromSnapshot && blkNum > s.bootsnapshotInfo.lastBlockInSnapshot {
		return nil, errors.New(
			"unexpected call. Boot KV Hashes are persisted only for the data imported from snapshot",
		)
	}
	encVal, err := s.db.Get(
		encodeBootKVHashesKey(
			&bootKVHashesKey{
				blkNum: blkNum,
				txNum:  txNum,
				ns:     ns,
				coll:   coll,
			},
		),
	)
	if err != nil || encVal == nil {
		return nil, err
	}
	bootKVHashes, err := decodeBootKVHashesVal(encVal)
	if err != nil {
		return nil, err
	}
	return bootKVHashes.toMap(), nil
}

// ProcessCollsEligibilityEnabled notifies the store when the peer becomes eligible to receive data for an
// existing collection. Parameter 'committingBlk' refers to the block number that contains the corresponding
// collection upgrade transaction and the parameter 'nsCollMap' contains the collections for which the peer
// is now eligible to receive pvt data
func (s *Store) ProcessCollsEligibilityEnabled(committingBlk uint64, nsCollMap map[string][]string) error {
	key := encodeCollElgKey(committingBlk)
	m := newCollElgInfo(nsCollMap)
	val, err := encodeCollElgVal(m)
	if err != nil {
		return err
	}
	batch := s.db.NewUpdateBatch()
	batch.Put(key, val)
	if err = s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	s.collElgProcSync.notify()
	return nil
}

func (s *Store) performPurgeIfScheduled(latestCommittedBlk uint64) {
	if latestCommittedBlk%s.purgeInterval != 0 {
		return
	}
	go func() {
		s.purgerLock.Lock()
		logger.Debugf("Purger started: Removing expired and purged private data till block number [%d]", latestCommittedBlk)
		defer s.purgerLock.Unlock()

		if err := s.purgeExpiredData(0, latestCommittedBlk); err != nil {
			logger.Warningf("Could not purge expired data from pvtdata store:%s", err)
		}

		if err := s.deleteDataMarkedForPurge(); err != nil {
			logger.Warningf("Could not purge data marked for purge from pvtdata store:%s", err)
		}
		logger.Debug("Purger finished")
	}()
}

func (s *Store) purgeExpiredData(minBlkNum, maxBlkNum uint64) error {
	expiryEntries, err := s.retrieveExpiryEntries(minBlkNum, maxBlkNum)
	if err != nil || len(expiryEntries) == 0 {
		return err
	}

	batch := s.db.NewUpdateBatch()
	for _, expiryEntry := range expiryEntries {
		batch.Delete(encodeExpiryKey(expiryEntry.key))
		dataKeys, missingDataKeys, bootKVHashesKeys := deriveKeys(expiryEntry)

		for _, dataKey := range dataKeys {
			batch.Delete(encodeDataKey(dataKey))
		}

		for _, missingDataKey := range missingDataKeys {
			batch.Delete(
				encodeElgPrioMissingDataKey(missingDataKey),
			)
			batch.Delete(
				encodeElgDeprioMissingDataKey(missingDataKey),
			)
			batch.Delete(
				encodeInelgMissingDataKey(missingDataKey),
			)
		}

		for _, bootKVHashesKey := range bootKVHashesKeys {
			batch.Delete(encodeBootKVHashesKey(bootKVHashesKey))
		}

		dataEntries, err := s.retrieveDataEntries(dataKeys)
		if err != nil {
			return err
		}
		hashedIndexEntries, err := prepareHashedIndexEntries(dataEntries)
		if err != nil {
			return err
		}
		for _, hashedIndexEntry := range hashedIndexEntries {
			batch.Delete(encodeHashedIndexKey(hashedIndexEntry.key))
		}

		if err := s.db.WriteBatch(batch, false); err != nil {
			return err
		}
		batch.Reset()
	}

	logger.Infow("Expired keys removed from private data storage", "channel", s.ledgerid, "numExpired", len(expiryEntries), "blockNum", maxBlkNum)
	return nil
}

func (s *Store) deleteDataMarkedForPurge() error {
	maxBatchSize := 4 * 1024 * 1024 // 4Mb
	purgeMarkerCounter := 0
	hashedIndexCounter := 0
	p := newPurgeUpdatesProcessor(s.ledgerid, s.db, s.purgedKeyAuditLogging, maxBatchSize)
	pStart, pEnd := rangeScanKeysForPurgeMarkers()

	// get the purge markers that need to be processed at this block height
	purgeMarkerIter, err := s.db.GetIterator(pStart, pEnd)
	if err != nil {
		return err
	}

	for purgeMarkerIter.Next() {
		if err := purgeMarkerIter.Error(); err != nil {
			return err
		}

		encPurgeMarkerKey := purgeMarkerIter.Key()
		encPurgeMarkerVal := purgeMarkerIter.Value()

		// for each purge marker to be processed, get the hashed index entries that need to be purged on this peer
		hStart, hEnd, err := driveHashedIndexKeyRangeFromPurgeMarker(encPurgeMarkerKey, encPurgeMarkerVal)
		if err != nil {
			return err
		}
		hashedIndexIter, err := s.db.GetIterator(hStart, hEnd)
		if err != nil {
			return err
		}
		for hashedIndexIter.Next() {
			if err := hashedIndexIter.Error(); err != nil {
				return err
			}

			// for each hashed index entry, process the purge from private data store and delete the hashed index
			if err := p.process(hashedIndexIter.Key(), hashedIndexIter.Value()); err != nil {
				return err
			}
			hashedIndexCounter++
		}

		// also delete the purge marker itself
		p.addProcessedPurgeMarkerForDeletion(encPurgeMarkerKey)
		purgeMarkerCounter++
	}

	// commit the private data purges and index deletions to the private data store
	err = p.commitPendingChanges()
	if err != nil {
		return err
	}

	if purgeMarkerCounter > 0 {
		logger.Infow("Purged private data from private data storage", "channel", s.ledgerid, "numKeysPurged", purgeMarkerCounter, "numPrivateDataStoreRecordsPurged", hashedIndexCounter)
	}

	return nil
}

func (s *Store) retrieveExpiryEntries(minBlkNum, maxBlkNum uint64) ([]*expiryEntry, error) {
	startKey, endKey := getExpiryKeysForRangeScan(minBlkNum, maxBlkNum)
	logger.Debugf("retrieveExpiryEntries(): startKey=%#v, endKey=%#v", startKey, endKey)
	itr, err := s.db.GetIterator(startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer itr.Release()

	var expiryEntries []*expiryEntry
	for itr.Next() {
		expiryKeyBytes := itr.Key()
		expiryValueBytes := itr.Value()
		expiryKey, err := decodeExpiryKey(expiryKeyBytes)
		if err != nil {
			return nil, err
		}
		expiryValue, err := decodeExpiryValue(expiryValueBytes)
		if err != nil {
			return nil, err
		}
		expiryEntries = append(expiryEntries, &expiryEntry{key: expiryKey, value: expiryValue})
	}
	return expiryEntries, nil
}

func (s *Store) retrieveDataEntries(dataKeys []*dataKey) ([]*dataEntry, error) {
	dataEntries := []*dataEntry{}
	for _, k := range dataKeys {
		v, err := s.db.Get(encodeDataKey(k))
		if err != nil {
			return nil, err
		}

		collWS, err := decodeDataValue(v)
		if err != nil {
			return nil, err
		}

		dataEntries = append(dataEntries,
			&dataEntry{
				key:   k,
				value: collWS,
			})
	}
	return dataEntries, nil
}

func (s *Store) launchCollElgProc() {
	go func() {
		if err := s.processCollElgEvents(); err != nil {
			// process collection eligibility events when store is opened -
			// in case there is an unprocessed events from previous run
			logger.Errorw("failed to process collection eligibility events", "err", err)
		}
		for {
			logger.Debugf("Waiting for collection eligibility event")
			s.collElgProcSync.waitForNotification()
			if err := s.processCollElgEvents(); err != nil {
				logger.Errorw("failed to process collection eligibility events", "err", err)
			}
			s.collElgProcSync.done()
		}
	}()
}

func (s *Store) processCollElgEvents() error {
	logger.Debugf("Starting to process collection eligibility events")
	s.purgerLock.Lock()
	defer s.purgerLock.Unlock()
	collElgStartKey, collElgEndKey := createRangeScanKeysForCollElg()
	eventItr, err := s.db.GetIterator(collElgStartKey, collElgEndKey)
	if err != nil {
		return err
	}
	defer eventItr.Release()
	batch := s.db.NewUpdateBatch()
	totalEntriesConverted := 0

	for eventItr.Next() {
		collElgKey, collElgVal := eventItr.Key(), eventItr.Value()
		blkNum := decodeCollElgKey(collElgKey)
		CollElgInfo, err := decodeCollElgVal(collElgVal)
		logger.Debugf("Processing collection eligibility event [blkNum=%d], CollElgInfo=%s", blkNum, CollElgInfo)
		if err != nil {
			logger.Errorf("This error is not expected %s", err)
			continue
		}
		for ns, colls := range CollElgInfo.NsCollMap {
			var coll string
			for _, coll = range colls.Entries {
				logger.Infof("Converting missing data entries from ineligible to eligible for [ns=%s, coll=%s]", ns, coll)
				startKey, endKey := createRangeScanKeysForInelgMissingData(blkNum, ns, coll)
				collItr, err := s.db.GetIterator(startKey, endKey)
				if err != nil {
					return err
				}
				collEntriesConverted := 0

				for collItr.Next() { // each entry
					originalKey, originalVal := collItr.Key(), collItr.Value()
					modifiedKey := decodeInelgMissingDataKey(originalKey)
					batch.Delete(originalKey)
					copyVal := make([]byte, len(originalVal))
					copy(copyVal, originalVal)
					batch.Put(
						encodeElgPrioMissingDataKey(modifiedKey),
						copyVal,
					)
					collEntriesConverted++
					if batch.Len() > s.maxBatchSize {
						if err := s.db.WriteBatch(batch, true); err != nil {
							return err
						}
						batch.Reset()
						sleepTime := time.Duration(s.batchesInterval)
						logger.Infof("Going to sleep for %d milliseconds between batches. Entries for [ns=%s, coll=%s] converted so far = %d",
							sleepTime, ns, coll, collEntriesConverted)
						s.purgerLock.Unlock()
						time.Sleep(sleepTime * time.Millisecond)
						s.purgerLock.Lock()
					}
				} // entry loop

				collItr.Release()
				logger.Infof("Converted all [%d] entries for [ns=%s, coll=%s]", collEntriesConverted, ns, coll)
				totalEntriesConverted += collEntriesConverted
			} // coll loop
		} // ns loop
		batch.Delete(collElgKey) // delete the collection eligibility event key as well
	} // event loop

	if err := s.db.WriteBatch(batch, true); err != nil {
		return err
	}
	logger.Debugf("Converted [%d] ineligible missing data entries to eligible", totalEntriesConverted)
	return nil
}

// LastCommittedBlockHeight returns the height of the last committed block
func (s *Store) LastCommittedBlockHeight() (uint64, error) {
	if s.isEmpty {
		return 0, nil
	}
	return atomic.LoadUint64(&s.lastCommittedBlock) + 1, nil
}

func (s *Store) nextBlockNum() uint64 {
	if s.isEmpty {
		return 0
	}
	return atomic.LoadUint64(&s.lastCommittedBlock) + 1
}

// TODO: FAB-16298 -- the concept of pendingBatch is no longer valid
// for pvtdataStore. We can remove it v2.1. We retain the concept in
// v2.0 to allow rolling upgrade from v142 to v2.0
func (s *Store) hasPendingCommit() (bool, error) {
	var v []byte
	var err error
	if v, err = s.db.Get(pendingCommitKey); err != nil {
		return false, err
	}
	return v != nil, nil
}

func (s *Store) getLastCommittedBlockNum() (bool, uint64, error) {
	var v []byte
	var err error
	if v, err = s.db.Get(lastCommittedBlkkey); v == nil || err != nil {
		return true, 0, err
	}
	return false, decodeLastCommittedBlockVal(v), nil
}

func (s *Store) fetchBootSnapshotInfo() (*bootsnapshotInfo, error) {
	v, err := s.db.Get(lastBlockInBootSnapshotKey)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return &bootsnapshotInfo{}, nil
	}

	lastBlkInSnapshot, err := decodeLastBlockInBootSnapshotVal(v)
	if err != nil {
		return nil, err
	}

	return &bootsnapshotInfo{
		createdFromSnapshot: true,
		lastBlockInSnapshot: lastBlkInSnapshot,
	}, nil
}

func (s *Store) FetchPrivateDataRawKey(ns, coll string, keyHash []byte) (string, error) {
	startKey, endKey := rangeScanKeysForHashedIndexKey(ns, coll, keyHash)
	dbItr, err := s.db.GetIterator(startKey, endKey)
	if err != nil {
		return "", err
	}
	defer dbItr.Release()

	if !dbItr.Next() {
		return "", dbItr.Error()
	}

	encVal := dbItr.Value()
	return string(encVal), nil
}

type collElgProcSync struct {
	notification, procComplete chan bool
}

func (c *collElgProcSync) notify() {
	select {
	case c.notification <- true:
		logger.Debugf("Signaled to collection eligibility processing routine")
	default: // noop
		logger.Debugf("Previous signal still pending. Skipping new signal")
	}
}

func (c *collElgProcSync) waitForNotification() {
	<-c.notification
}

func (c *collElgProcSync) done() {
	select {
	case c.procComplete <- true:
	default:
	}
}

func (c *collElgProcSync) waitForDone() {
	<-c.procComplete
}

type purgeUpdatesProcessor struct {
	ledgerid     string
	db           *leveldbhelper.DBHandle
	batch        *leveldbhelper.UpdateBatch
	maxBatchSize int

	pvtWrites map[string]*rwsetutil.CollPvtRwSet

	currentSize           int
	purgedKeyAuditLogging bool
}

// newPurgeUpdatesProcessor is used for processing the purge markers - i.e., delete the private data versions that are marked for purge from
// the pvtdata store.
func newPurgeUpdatesProcessor(ledgerid string, db *leveldbhelper.DBHandle, purgedKeyAuditLogging bool, maxBatchSize int) *purgeUpdatesProcessor {
	return &purgeUpdatesProcessor{
		ledgerid:              ledgerid,
		db:                    db,
		purgedKeyAuditLogging: purgedKeyAuditLogging,
		maxBatchSize:          maxBatchSize,
		pvtWrites:             map[string]*rwsetutil.CollPvtRwSet{},
		batch:                 db.NewUpdateBatch(),
	}
}

// process takes one hashedIndex Key, value (that points to a private key in a particular writeset) at a time and retrieves the
// corresponding writeset from the store. It then deletes the intended key from the writeset and write the trimmed writset back.
// Note that one writeset may contain more than one keys and this function may get invoked mutilple time for different keys in the same
// writeset, hence in every invocation, we should not fetch the writeset from pvtdata store - this is required for correctness reasons not for the performance reasons.
// Otherwise, previously performed delete operations on the same writeset will become void, as fetching from store will always give the full writeset (as the trimmed one is not yet committed).
// The deletion of hashedIndexKey is also included in the same batch
func (p *purgeUpdatesProcessor) process(hashedIndexKey, hashedIndexVal []byte) error {
	dataKey, err := deriveDataKeyFromEncodedHashedIndexKey(hashedIndexKey)
	if err != nil {
		return err
	}

	if _, ok := p.pvtWrites[string(dataKey)]; !ok {
		dataValue, err := p.db.Get(dataKey)
		if err != nil {
			return err
		}
		collPvtRWSetProto, err := decodeDataValue(dataValue)
		if err != nil {
			return err
		}
		collPvtRWSet, err := rwsetutil.CollPvtRwSetFromProtoMsg(collPvtRWSetProto)
		if err != nil {
			return err
		}
		p.pvtWrites[string(dataKey)] = collPvtRWSet
		p.currentSize += len(dataKey) + len(dataValue)
	}

	collWS := p.pvtWrites[string(dataKey)]
	writes := collWS.KvRwSet.Writes
	for i, w := range writes {
		// hashedIndexVal represents the raw private data key
		if w.Key == string(hashedIndexVal) {
			collWS.KvRwSet.Writes = append(writes[:i], writes[i+1:]...)
			p.currentSize -= len(w.Key) + len(w.Value)
			break
		}
	}

	// If configured, log the purged key for audit purpose
	if p.purgedKeyAuditLogging {
		decodedDataKey, err := decodeDatakey(dataKey)
		if err != nil {
			return err
		}
		logger.Infow("Purging private data from private data storage", "channel", p.ledgerid, "chaincode", decodedDataKey.nsCollBlk.ns, "collection", decodedDataKey.nsCollBlk.coll, "key", string(hashedIndexVal), "blockNum", decodedDataKey.nsCollBlk.blkNum, "tranNum", decodedDataKey.txNum)
	}

	p.batch.Delete(hashedIndexKey)
	if p.currentSize+p.batch.Size() > p.maxBatchSize {
		if err := p.commitPendingChanges(); err != nil {
			return err
		}
	}
	return nil
}

func (p *purgeUpdatesProcessor) addProcessedPurgeMarkerForDeletion(purgeMarkerKey []byte) {
	p.batch.Delete(purgeMarkerKey)
}

func (p *purgeUpdatesProcessor) commitPendingChanges() error {
	for k, w := range p.pvtWrites {
		pvtWSProto, err := w.ToProtoMsg()
		if err != nil {
			return err
		}
		encDataValue, err := encodeDataValue(pvtWSProto)
		if err != nil {
			return err
		}
		p.batch.Put([]byte(k), encDataValue)
	}
	if err := p.db.WriteBatch(p.batch, true); err != nil {
		return err
	}

	p.pvtWrites = map[string]*rwsetutil.CollPvtRwSet{}
	p.batch.Reset()
	return nil
}
