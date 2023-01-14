/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"bytes"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/pvtstatepurgemgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/queryutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validation"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("lockbasedtxmgr")

// LockBasedTxMgr a simple implementation of interface `txmgmt.TxMgr`.
// This implementation uses a read-write lock to prevent conflicts between transaction simulation and committing
type LockBasedTxMgr struct {
	ledgerid            string
	db                  *privacyenabledstate.DB
	pvtdataPurgeMgr     *pvtdataPurgeMgr
	commitBatchPreparer *validation.CommitBatchPreparer
	stateListeners      []ledger.StateListener
	ccInfoProvider      ledger.DeployedChaincodeInfoProvider
	commitRWLock        sync.RWMutex
	oldBlockCommit      sync.Mutex
	currentUpdates      *currentUpdates
	hashFunc            rwsetutil.HashFunc
}

// pvtdataPurgeMgr wraps the actual purge manager and an additional flag 'usedOnce'
// for usage of this additional flag, see the relevant comments in the txmgr.Commit() function above
type pvtdataPurgeMgr struct {
	*pvtstatepurgemgmt.PurgeMgr
	usedOnce bool
}

type currentUpdates struct {
	block     *common.Block
	batch     *privacyenabledstate.UpdateBatch
	listeners []ledger.StateListener
}

func (c *currentUpdates) blockNum() uint64 {
	return c.block.Header.Number
}

func (c *currentUpdates) maxTxNumber() uint64 {
	return uint64(len(c.block.Data.Data)) - 1
}

func (c *currentUpdates) purgeAppInitiatedPvtKeys(keys map[privacyenabledstate.PvtdataCompositeKey]*version.Height) {
	for ck, ver := range keys {
		pvtValVer := c.batch.PvtUpdates.Get(ck.Namespace, ck.CollectionName, ck.Key)
		if pvtValVer == nil || pvtValVer.Version.Compare(ver) < 0 {
			// delete the purged pvtkey from the statedb if the pvtkey is not present in the private updates
			// (this could be because, either this peer is ineligible now, or the pvtWS is not supplied with the block)
			// Don't delete if the pvtkey of higher version is present in the private updates
			c.batch.PvtUpdates.Delete(ck.Namespace, ck.CollectionName, ck.Key, ver)
		}
	}
}

// Initializer captures the dependencies for tx manager
type Initializer struct {
	LedgerID            string
	DB                  *privacyenabledstate.DB
	StateListeners      []ledger.StateListener
	BtlPolicy           pvtdatapolicy.BTLPolicy
	BookkeepingProvider *bookkeeping.Provider
	CCInfoProvider      ledger.DeployedChaincodeInfoProvider
	CustomTxProcessors  map[common.HeaderType]ledger.CustomTxProcessor
	HashFunc            rwsetutil.HashFunc
}

// NewLockBasedTxMgr constructs a new instance of NewLockBasedTxMgr
func NewLockBasedTxMgr(initializer *Initializer) (*LockBasedTxMgr, error) {
	if initializer.HashFunc == nil {
		return nil, errors.New("create new lock based TxMgr failed: passed in nil ledger hasher")
	}

	if err := initializer.DB.Open(); err != nil {
		return nil, err
	}
	txmgr := &LockBasedTxMgr{
		ledgerid:       initializer.LedgerID,
		db:             initializer.DB,
		stateListeners: initializer.StateListeners,
		ccInfoProvider: initializer.CCInfoProvider,
		hashFunc:       initializer.HashFunc,
	}
	pvtstatePurgeMgr, err := pvtstatepurgemgmt.InstantiatePurgeMgr(
		initializer.LedgerID,
		initializer.DB,
		initializer.BtlPolicy,
		initializer.BookkeepingProvider)
	if err != nil {
		return nil, err
	}
	txmgr.pvtdataPurgeMgr = &pvtdataPurgeMgr{pvtstatePurgeMgr, false}
	txmgr.commitBatchPreparer = validation.NewCommitBatchPreparer(
		txmgr,
		initializer.DB,
		initializer.CustomTxProcessors,
		initializer.HashFunc)
	return txmgr, nil
}

// GetLastSavepoint returns the block num recorded in savepoint,
// returns 0 if NO savepoint is found
func (txmgr *LockBasedTxMgr) GetLastSavepoint() (*version.Height, error) {
	return txmgr.db.GetLatestSavePoint()
}

// NewQueryExecutor implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) NewQueryExecutor(txid string) (ledger.QueryExecutor, error) {
	qe := newQueryExecutor(txmgr, txid, nil, true, txmgr.hashFunc)
	txmgr.commitRWLock.RLock()
	return qe, nil
}

// NewQueryExecutorNoCollChecks is a workaround to make the initilization of lifecycle cache
// work. The issue is that in the current lifecycle code the cache is initialized via Initialize
// function of a statelistener which gets invoked during ledger opening. This invovation eventually
// leads to a call to DeployedChaincodeInfoProvider which inturn needs the channel config in order
// to varify the name of the implicit collection. And the channelconfig is loaded only after the
// ledger is opened. So, as a workaround, we skip the check of collection name in this function
// by supplying a relaxed query executor - This is perfectly safe otherwise.
// As a proper fix, the initialization of other components should take place outside ledger by explicit
// querying the ledger state so that the sequence of initialization is explicitly controlled.
// However that needs a bigger refactoring of code.
func (txmgr *LockBasedTxMgr) NewQueryExecutorNoCollChecks() (ledger.QueryExecutor, error) {
	qe := newQueryExecutor(txmgr, "", nil, false, txmgr.hashFunc)
	txmgr.commitRWLock.RLock()
	return qe, nil
}

// NewTxSimulator implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) NewTxSimulator(txid string) (ledger.TxSimulator, error) {
	logger.Debugf("constructing new tx simulator")
	s, err := newTxSimulator(txmgr, txid, txmgr.hashFunc)
	if err != nil {
		return nil, err
	}
	txmgr.commitRWLock.RLock()
	return s, nil
}

// ValidateAndPrepare implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) ValidateAndPrepare(blockAndPvtdata *ledger.BlockAndPvtData, doMVCCValidation bool) (
	[]*validation.AppInitiatedPurgeUpdate, []*validation.TxStatInfo, []byte, error,
) {
	// Among ValidateAndPrepare(), PrepareForExpiringKeys(), and
	// RemoveStaleAndCommitPvtDataOfOldBlocks(), we can allow only one
	// function to execute at a time. The reason is that each function calls
	// LoadCommittedVersions() which would clear the existing entries in the
	// transient buffer and load new entries (such a transient buffer is not
	// applicable for the golevelDB). As a result, these three functions can
	// interleave and nullify the optimization provided by the bulk read API.
	// Once the ledger cache (FAB-103) is introduced and existing
	// LoadCommittedVersions() is refactored to return a map, we can allow
	// these three functions to execute parallelly.
	logger.Debugf("Waiting for purge mgr to finish the background job of computing expirying keys for the block")
	txmgr.pvtdataPurgeMgr.WaitForPrepareToFinish()
	txmgr.oldBlockCommit.Lock()
	defer txmgr.oldBlockCommit.Unlock()
	logger.Debug("lock acquired on oldBlockCommit for validating read set version against the committed version")

	block := blockAndPvtdata.Block
	logger.Debugf("Validating new block with num trans = [%d]", len(block.Data.Data))
	batch, appPurgeUpdates, txstatsInfo, err := txmgr.commitBatchPreparer.ValidateAndPrepareBatch(blockAndPvtdata, doMVCCValidation)
	if err != nil {
		txmgr.reset()
		return nil, nil, nil, err
	}
	txmgr.currentUpdates = &currentUpdates{block: block, batch: batch}
	if err := txmgr.invokeNamespaceListeners(); err != nil {
		txmgr.reset()
		return nil, nil, nil, err
	}

	updateBytes, err := deterministicBytesForPubAndHashUpdates(batch)
	return appPurgeUpdates, txstatsInfo, updateBytes, err
}

// RemoveStaleAndCommitPvtDataOfOldBlocks implements method in interface `txmgmt.TxMgr`
// The following six operations are performed:
// (1) constructs the unique pvt data from the passed reconciledPvtdata
// (2) acquire a lock on oldBlockCommit
// (3) checks for stale pvtData by comparing [version, valueHash] and removes stale data
// (4) creates update batch from the non-stale pvtData
// (5) update the BTL bookkeeping managed by the purge manager and update expiring keys.
// (6) commit the non-stale pvt data to the stateDB
// This function assumes that the passed input contains only transactions that had been
// marked "Valid". In the current design, kvledger (a single consumer of this function),
// filters out the data of "invalid" transactions and supplies the data for "valid" transactions only.
func (txmgr *LockBasedTxMgr) RemoveStaleAndCommitPvtDataOfOldBlocks(reconciledPvtdata map[uint64][]*ledger.TxPvtData) error {
	// (0) Among ValidateAndPrepare(), PrepareForExpiringKeys(), and
	// RemoveStaleAndCommitPvtDataOfOldBlocks(), we can allow only one
	// function to execute at a time. The reason is that each function calls
	// LoadCommittedVersions() which would clear the existing entries in the
	// transient buffer and load new entries (such a transient buffer is not
	// applicable for the golevelDB). As a result, these three functions can
	// interleave and nullify the optimization provided by the bulk read API.
	// Once the ledger cache (FAB-103) is introduced and existing
	// LoadCommittedVersions() is refactored to return a map, we can allow
	// these three functions to execute parallelly. However, we cannot remove
	// the lock on oldBlockCommit as it is also used to avoid interleaving
	// between Commit() and execution of this function for the correctness.
	logger.Debug("Waiting for purge mgr to finish the background job of computing expirying keys for the block")
	txmgr.pvtdataPurgeMgr.WaitForPrepareToFinish()
	txmgr.oldBlockCommit.Lock()
	defer txmgr.oldBlockCommit.Unlock()
	logger.Debug("lock acquired on oldBlockCommit for committing pvtData of old blocks to state database")

	// (1) as the reconciledPvtdata can contain multiple versions of pvtData for
	// a given <ns, coll, key>, we need to find duplicate tuples with different
	// versions and use the one with the higher version
	logger.Debug("Constructing unique pvtData by removing duplicate entries")
	uniquePvtData, err := constructUniquePvtData(reconciledPvtdata)
	if len(uniquePvtData) == 0 || err != nil {
		return err
	}

	// (3) remove the pvt data which does not matches the hashed
	// value stored in the public state
	logger.Debug("Finding and removing stale pvtData")
	if err := uniquePvtData.findAndRemoveStalePvtData(txmgr.db); err != nil {
		return err
	}

	// (4) create the update batch from the uniquePvtData
	batch := uniquePvtData.transformToUpdateBatch()

	// (5) update bookkeeping in the purge manager and update toPurgeList
	// (i.e., the list of expiry keys). As the expiring keys would have
	// been constructed during last PrepareForExpiringKeys from commit, we need
	// to update the list. This is because RemoveStaleAndCommitPvtDataOfOldBlocks
	// may have added new data which might be eligible for expiry during the
	// next regular block commit.
	logger.Debug("Updating expiry info in the purge manager")
	if err := txmgr.pvtdataPurgeMgr.UpdateExpiryInfoOfPvtDataOfOldBlocks(batch.PvtUpdates); err != nil {
		return err
	}

	// (6) commit the pvt data to the stateDB
	logger.Debug("Committing updates to state database")
	return txmgr.db.ApplyPrivacyAwareUpdates(batch, nil)
}

type uniquePvtDataMap map[privacyenabledstate.HashedCompositeKey]*privacyenabledstate.PvtKVWrite

func constructUniquePvtData(reconciledPvtdata map[uint64][]*ledger.TxPvtData) (uniquePvtDataMap, error) {
	uniquePvtData := make(uniquePvtDataMap)
	// go over the reconciledPvtdata to find duplicate <ns, coll, key>
	// in the pvtWrites and use the one with the higher version number
	for blkNum, blockPvtData := range reconciledPvtdata {
		if err := uniquePvtData.updateUsingBlockPvtdata(blockPvtData, blkNum); err != nil {
			return nil, err
		}
	} // for each block
	return uniquePvtData, nil
}

func (uniquePvtData uniquePvtDataMap) updateUsingBlockPvtdata(blockPvtData []*ledger.TxPvtData, blkNum uint64) error {
	for _, txPvtData := range blockPvtData {
		ver := version.NewHeight(blkNum, txPvtData.SeqInBlock)
		if err := uniquePvtData.updateUsingTxPvtData(txPvtData, ver); err != nil {
			return err
		}
	} // for each tx
	return nil
}

func (uniquePvtData uniquePvtDataMap) updateUsingTxPvtData(txPvtData *ledger.TxPvtData, ver *version.Height) error {
	for _, nsPvtData := range txPvtData.WriteSet.NsPvtRwset {
		if err := uniquePvtData.updateUsingNsPvtData(nsPvtData, ver); err != nil {
			return err
		}
	} // for each ns
	return nil
}

func (uniquePvtData uniquePvtDataMap) updateUsingNsPvtData(nsPvtData *rwset.NsPvtReadWriteSet, ver *version.Height) error {
	for _, collPvtData := range nsPvtData.CollectionPvtRwset {
		if err := uniquePvtData.updateUsingCollPvtData(collPvtData, nsPvtData.Namespace, ver); err != nil {
			return err
		}
	} // for each coll
	return nil
}

func (uniquePvtData uniquePvtDataMap) updateUsingCollPvtData(collPvtData *rwset.CollectionPvtReadWriteSet,
	ns string, ver *version.Height) error {
	kvRWSet := &kvrwset.KVRWSet{}
	if err := proto.Unmarshal(collPvtData.Rwset, kvRWSet); err != nil {
		return err
	}

	hashedCompositeKey := privacyenabledstate.HashedCompositeKey{
		Namespace:      ns,
		CollectionName: collPvtData.CollectionName,
	}

	for _, kvWrite := range kvRWSet.Writes { // for each kv pair
		hashedCompositeKey.KeyHash = string(util.ComputeStringHash(kvWrite.Key))
		uniquePvtData.updateUsingPvtWrite(kvWrite, hashedCompositeKey, ver)
	} // for each kv pair

	return nil
}

func (uniquePvtData uniquePvtDataMap) updateUsingPvtWrite(pvtWrite *kvrwset.KVWrite,
	hashedCompositeKey privacyenabledstate.HashedCompositeKey, ver *version.Height) {
	pvtData, ok := uniquePvtData[hashedCompositeKey]
	if !ok || pvtData.Version.Compare(ver) < 0 {
		uniquePvtData[hashedCompositeKey] =
			&privacyenabledstate.PvtKVWrite{
				Key:      pvtWrite.Key,
				IsDelete: rwsetutil.IsKVWriteDelete(pvtWrite),
				Value:    pvtWrite.Value,
				Version:  ver,
			}
	}
}

func (uniquePvtData uniquePvtDataMap) findAndRemoveStalePvtData(db *privacyenabledstate.DB) error {
	// (1) load all committed versions
	if err := uniquePvtData.loadCommittedVersionIntoCache(db); err != nil {
		return err
	}

	// (2) find and remove the stale data
	for hashedCompositeKey, pvtWrite := range uniquePvtData {
		isStale, err := checkIfPvtWriteIsStale(&hashedCompositeKey, pvtWrite, db)
		if err != nil {
			return err
		}
		if isStale {
			delete(uniquePvtData, hashedCompositeKey)
		}
	}
	return nil
}

func (uniquePvtData uniquePvtDataMap) loadCommittedVersionIntoCache(db *privacyenabledstate.DB) error {
	// Note that ClearCachedVersions would not be called till we validate and commit these
	// pvt data of old blocks. This is because only during the exclusive lock duration, we
	// clear the cache and we have already acquired one before reaching here.
	var hashedCompositeKeys []*privacyenabledstate.HashedCompositeKey
	for hashedCompositeKey := range uniquePvtData {
		// tempKey ensures a different pointer is added to the slice for each key
		tempKey := hashedCompositeKey
		hashedCompositeKeys = append(hashedCompositeKeys, &tempKey)
	}

	err := db.LoadCommittedVersionsOfPubAndHashedKeys(nil, hashedCompositeKeys)
	if err != nil {
		return err
	}
	return nil
}

func checkIfPvtWriteIsStale(hashedKey *privacyenabledstate.HashedCompositeKey,
	kvWrite *privacyenabledstate.PvtKVWrite, db *privacyenabledstate.DB) (bool, error) {
	ns := hashedKey.Namespace
	coll := hashedKey.CollectionName
	keyHashBytes := []byte(hashedKey.KeyHash)
	committedVersion, err := db.GetKeyHashVersion(ns, coll, keyHashBytes)
	if err != nil {
		return true, err
	}

	// for a deleted hashedKey, we would get a nil committed version. Note that
	// the hashedKey was deleted because either it got expired or was deleted by
	// the chaincode itself.
	if committedVersion == nil {
		return !kvWrite.IsDelete, nil
	}

	/*
		TODO: FAB-12922
		In the first round, we need to the check version of passed pvtData
		against the version of pvtdata stored in the stateDB. In second round,
		for the remaining pvtData, we need to check for staleness using hashed
		version. In the third round, for the still remaining pvtdata, we need
		to check against hashed values. In each phase we would require to
		perform bulkload of relevant data from the stateDB.
		committedPvtData, err := db.GetPrivateData(ns, coll, kvWrite.Key)
		if err != nil {
			return false, err
		}
		if committedPvtData.Version.Compare(kvWrite.Version) > 0 {
			return false, nil
		}
	*/
	if version.AreSame(committedVersion, kvWrite.Version) {
		return false, nil
	}

	// due to metadata updates, we could get a version
	// mismatch between pvt kv write and the committed
	// hashedKey. In this case, we must compare the hash
	// of the value. If the hash matches, we should update
	// the version number in the pvt kv write and return
	// true as the validation result
	vv, err := db.GetValueHash(ns, coll, keyHashBytes)
	if err != nil {
		return true, err
	}
	if bytes.Equal(vv.Value, util.ComputeHash(kvWrite.Value)) {
		// if hash of value matches, update version
		// and return true
		kvWrite.Version = vv.Version // side effect
		// (checkIfPvtWriteIsStale should not be updating the state)
		return false, nil
	}
	return true, nil
}

func (uniquePvtData uniquePvtDataMap) transformToUpdateBatch() *privacyenabledstate.UpdateBatch {
	batch := privacyenabledstate.NewUpdateBatch()
	for hashedCompositeKey, pvtWrite := range uniquePvtData {
		ns := hashedCompositeKey.Namespace
		coll := hashedCompositeKey.CollectionName
		if pvtWrite.IsDelete {
			batch.PvtUpdates.Delete(ns, coll, pvtWrite.Key, pvtWrite.Version)
		} else {
			batch.PvtUpdates.Put(ns, coll, pvtWrite.Key, pvtWrite.Value, pvtWrite.Version)
		}
	}
	return batch
}

func (txmgr *LockBasedTxMgr) invokeNamespaceListeners() error {
	for _, listener := range txmgr.stateListeners {
		stateUpdatesForListener := extractStateUpdates(txmgr.currentUpdates.batch, listener.InterestedInNamespaces())
		if len(stateUpdatesForListener) == 0 {
			continue
		}
		txmgr.currentUpdates.listeners = append(txmgr.currentUpdates.listeners, listener)

		committedStateQueryExecuter := &queryutil.QECombiner{
			QueryExecuters: []queryutil.QueryExecuter{txmgr.db},
		}

		postCommitQueryExecuter := &queryutil.QECombiner{
			QueryExecuters: []queryutil.QueryExecuter{
				&queryutil.UpdateBatchBackedQueryExecuter{
					UpdateBatch:      txmgr.currentUpdates.batch.PubUpdates.UpdateBatch,
					HashUpdatesBatch: txmgr.currentUpdates.batch.HashUpdates,
				},
				txmgr.db,
			},
		}

		trigger := &ledger.StateUpdateTrigger{
			LedgerID:                    txmgr.ledgerid,
			StateUpdates:                stateUpdatesForListener,
			CommittingBlockNum:          txmgr.currentUpdates.blockNum(),
			CommittedStateQueryExecutor: committedStateQueryExecuter,
			PostCommitQueryExecutor:     postCommitQueryExecuter,
		}
		if err := listener.HandleStateUpdates(trigger); err != nil {
			return err
		}
		logger.Debugf("Invoking listener for state changes:%s", listener.Name())
	}
	return nil
}

// Shutdown implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) Shutdown() {
	// wait for background go routine to finish else the timing issue causes a nil pointer inside goleveldb code
	// see FAB-11974
	txmgr.pvtdataPurgeMgr.WaitForPrepareToFinish()
	txmgr.db.Close()
}

// UpdateBatchWithAppInitiatedPvtKeysToPurge adds delete markers in the state update batch for the private data keys that
// should be deleted from the state as a result of app initiated purge operations.
func (txmgr *LockBasedTxMgr) UpdateBatchWithAppInitiatedPvtKeysToPurge(keys map[privacyenabledstate.PvtdataCompositeKey]*version.Height) {
	txmgr.currentUpdates.purgeAppInitiatedPvtKeys(keys)
}

// Commit implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) Commit() error {
	// we need to acquire a lock on oldBlockCommit. The following are the two reasons:
	// (1) the DeleteExpiredAndUpdateBookkeeping() would perform incorrect operation if
	//        toPurgeList is updated by RemoveStaleAndCommitPvtDataOfOldBlocks().
	// (2) RemoveStaleAndCommitPvtDataOfOldBlocks computes the update
	//     batch based on the current state and if we allow regular block commits at the
	//     same time, the former may overwrite the newer versions of the data and we may
	//     end up with an incorrect update batch.
	txmgr.oldBlockCommit.Lock()
	defer txmgr.oldBlockCommit.Unlock()
	logger.Debug("lock acquired on oldBlockCommit for committing regular updates to state database")

	// When using the purge manager for the first block commit after peer start, the asynchronous function
	// 'PrepareForExpiringKeys' is invoked in-line. However, for the subsequent blocks commits, this function is invoked
	// in advance for the next block
	if !txmgr.pvtdataPurgeMgr.usedOnce {
		txmgr.pvtdataPurgeMgr.PrepareForExpiringKeys(txmgr.currentUpdates.blockNum())
		txmgr.pvtdataPurgeMgr.usedOnce = true
	}
	defer func() {
		txmgr.pvtdataPurgeMgr.PrepareForExpiringKeys(txmgr.currentUpdates.blockNum() + 1)
		logger.Debugf("launched the background routine for preparing keys to purge with the next block")
		txmgr.reset()
	}()

	logger.Debugf("Committing updates to state database")
	if txmgr.currentUpdates == nil {
		panic("validateAndPrepare() method should have been called before calling commit()")
	}

	if err := txmgr.pvtdataPurgeMgr.UpdateExpiryInfo(
		txmgr.currentUpdates.batch.PvtUpdates, txmgr.currentUpdates.batch.HashUpdates); err != nil {
		return err
	}

	if err := txmgr.pvtdataPurgeMgr.AddExpiredEntriesToUpdateBatch(
		txmgr.currentUpdates.batch.PvtUpdates, txmgr.currentUpdates.batch.HashUpdates); err != nil {
		return err
	}

	commitHeight := version.NewHeight(txmgr.currentUpdates.blockNum(), txmgr.currentUpdates.maxTxNumber())
	txmgr.commitRWLock.Lock()
	logger.Debugf("Write lock acquired for committing updates to state database")
	if err := txmgr.db.ApplyPrivacyAwareUpdates(txmgr.currentUpdates.batch, commitHeight); err != nil {
		txmgr.commitRWLock.Unlock()
		return err
	}
	txmgr.commitRWLock.Unlock()
	// only while holding a lock on oldBlockCommit, we should clear the cache as the
	// cache is being used by the old pvtData committer to load the version of
	// hashedKeys. Also, note that the PrepareForExpiringKeys uses the cache.
	txmgr.clearCache()
	logger.Debugf("Updates committed to state database and the write lock is released")

	// purge manager should be called (in this call the purge mgr removes the expiry entries from schedules) after committing to statedb
	if err := txmgr.pvtdataPurgeMgr.BlockCommitDone(); err != nil {
		return err
	}
	// In the case of error state listeners will not receive this call - instead a peer panic is caused by the ledger upon receiving
	// an error from this function
	txmgr.updateStateListeners()
	return nil
}

// Rollback implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) Rollback() {
	txmgr.reset()
}

// clearCache empty the cache maintained by the statedb implementation
func (txmgr *LockBasedTxMgr) clearCache() {
	if txmgr.db.IsBulkOptimizable() {
		txmgr.db.ClearCachedVersions()
	}
}

// ShouldRecover implements method in interface kvledger.Recoverer
func (txmgr *LockBasedTxMgr) ShouldRecover(lastAvailableBlock uint64) (bool, uint64, error) {
	savepoint, err := txmgr.GetLastSavepoint()
	if err != nil {
		return false, 0, err
	}
	if savepoint == nil {
		return true, 0, nil
	}
	return savepoint.BlockNum != lastAvailableBlock, savepoint.BlockNum + 1, nil
}

// Name returns the name of the database that manages all active states.
func (txmgr *LockBasedTxMgr) Name() string {
	return "state"
}

// CommitLostBlock implements method in interface kvledger.Recoverer
func (txmgr *LockBasedTxMgr) CommitLostBlock(blockAndPvtdata *ledger.BlockAndPvtData) error {
	block := blockAndPvtdata.Block
	logger.Debugf("Constructing updateSet for the block %d", block.Header.Number)
	if _, _, _, err := txmgr.ValidateAndPrepare(blockAndPvtdata, false); err != nil {
		return err
	}

	// log every 1000th block at Info level so that statedb rebuild progress can be tracked in production envs.
	if block.Header.Number%1000 == 0 {
		logger.Infof("Recommitting block [%d] to state database", block.Header.Number)
	} else {
		logger.Debugf("Recommitting block [%d] to state database", block.Header.Number)
	}

	return txmgr.Commit()
}

// ExportPubStateAndPvtStateHashes simply delegates the call to the statedb for exporting the data for a snapshot.
// It is assumed that the consumer would invoke this function when the commits are paused
func (txmgr *LockBasedTxMgr) ExportPubStateAndPvtStateHashes(dir string, newHashFunc snapshot.NewHashFunc) (map[string][]byte, error) {
	// no need to acuqire any lock in this function, as the commits would be paused
	return txmgr.db.ExportPubStateAndPvtStateHashes(dir, newHashFunc)
}

func extractStateUpdates(batch *privacyenabledstate.UpdateBatch, namespaces []string) ledger.StateUpdates {
	su := make(ledger.StateUpdates)
	for _, namespace := range namespaces {
		nsu := &ledger.KVStateUpdates{}
		// include public updates
		for key, versionedValue := range batch.PubUpdates.GetUpdates(namespace) {
			nsu.PublicUpdates = append(nsu.PublicUpdates,
				&kvrwset.KVWrite{
					Key:      key,
					IsDelete: versionedValue.Value == nil,
					Value:    versionedValue.Value,
				},
			)
		}
		// include colls hashes updates
		if hashUpdates, ok := batch.HashUpdates.UpdateMap[namespace]; ok {
			nsu.CollHashUpdates = make(map[string][]*kvrwset.KVWriteHash)
			for _, collName := range hashUpdates.GetCollectionNames() {
				for key, vv := range hashUpdates.GetUpdates(collName) {
					nsu.CollHashUpdates[collName] = append(
						nsu.CollHashUpdates[collName],
						&kvrwset.KVWriteHash{
							KeyHash:   []byte(key),
							IsDelete:  vv.Value == nil,
							ValueHash: vv.Value,
						},
					)
				}
			}
		}
		if len(nsu.PublicUpdates)+len(nsu.CollHashUpdates) > 0 {
			su[namespace] = nsu
		}
	}
	return su
}

func (txmgr *LockBasedTxMgr) updateStateListeners() {
	for _, l := range txmgr.currentUpdates.listeners {
		l.StateCommitDone(txmgr.ledgerid)
	}
}

func (txmgr *LockBasedTxMgr) reset() {
	txmgr.currentUpdates = nil
}
