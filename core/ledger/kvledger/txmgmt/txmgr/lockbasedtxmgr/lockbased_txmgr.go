/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"bytes"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/pvtstatepurgemgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/queryutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validator"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/validator/valimpl"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
)

var logger = flogging.MustGetLogger("lockbasedtxmgr")

// LockBasedTxMgr a simple implementation of interface `txmgmt.TxMgr`.
// This implementation uses a read-write lock to prevent conflicts between transaction simulation and committing
type LockBasedTxMgr struct {
	ledgerid        string
	db              privacyenabledstate.DB
	pvtdataPurgeMgr *pvtdataPurgeMgr
	validator       validator.Validator
	stateListeners  []ledger.StateListener
	ccInfoProvider  ledger.DeployedChaincodeInfoProvider
	commitRWLock    sync.RWMutex
	oldBlockCommit  sync.Mutex
	current         *current
}

type current struct {
	block     *common.Block
	batch     *privacyenabledstate.UpdateBatch
	listeners []ledger.StateListener
}

func (c *current) blockNum() uint64 {
	return c.block.Header.Number
}

func (c *current) maxTxNumber() uint64 {
	return uint64(len(c.block.Data.Data)) - 1
}

// NewLockBasedTxMgr constructs a new instance of NewLockBasedTxMgr
func NewLockBasedTxMgr(ledgerid string, db privacyenabledstate.DB, stateListeners []ledger.StateListener,
	btlPolicy pvtdatapolicy.BTLPolicy, bookkeepingProvider bookkeeping.Provider, ccInfoProvider ledger.DeployedChaincodeInfoProvider) (*LockBasedTxMgr, error) {
	db.Open()
	txmgr := &LockBasedTxMgr{
		ledgerid:       ledgerid,
		db:             db,
		stateListeners: stateListeners,
		ccInfoProvider: ccInfoProvider,
	}
	pvtstatePurgeMgr, err := pvtstatepurgemgmt.InstantiatePurgeMgr(ledgerid, db, btlPolicy, bookkeepingProvider)
	if err != nil {
		return nil, err
	}
	txmgr.pvtdataPurgeMgr = &pvtdataPurgeMgr{pvtstatePurgeMgr, false}
	txmgr.validator = valimpl.NewStatebasedValidator(txmgr, db)
	return txmgr, nil
}

// GetLastSavepoint returns the block num recorded in savepoint,
// returns 0 if NO savepoint is found
func (txmgr *LockBasedTxMgr) GetLastSavepoint() (*version.Height, error) {
	return txmgr.db.GetLatestSavePoint()
}

// NewQueryExecutor implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) NewQueryExecutor(txid string) (ledger.QueryExecutor, error) {
	qe := newQueryExecutor(txmgr, txid)
	txmgr.commitRWLock.RLock()
	return qe, nil
}

// NewTxSimulator implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) NewTxSimulator(txid string) (ledger.TxSimulator, error) {
	logger.Debugf("constructing new tx simulator")
	s, err := newLockBasedTxSimulator(txmgr, txid)
	if err != nil {
		return nil, err
	}
	txmgr.commitRWLock.RLock()
	return s, nil
}

// ValidateAndPrepare implements method in interface `txmgmt.TxMgr`
func (txmgr *LockBasedTxMgr) ValidateAndPrepare(blockAndPvtdata *ledger.BlockAndPvtData, doMVCCValidation bool) (
	[]*txmgr.TxStatInfo, []byte, error,
) {
	// Among ValidateAndPrepare(), PrepareExpiringKeys(), and
	// RemoveStaleAndCommitPvtDataOfOldBlocks(), we can allow only one
	// function to execute at a time. The reason is that each function calls
	// LoadCommittedVersions() which would clear the existing entries in the
	// transient buffer and load new entries (such a transient buffer is not
	// applicable for the golevelDB). As a result, these three functions can
	// interleave and nullify the optimization provided by the bulk read API.
	// Once the ledger cache (FAB-103) is introduced and existing
	// LoadCommittedVersions() is refactored to return a map, we can allow
	// these three functions to execute parallely.
	logger.Debugf("Waiting for purge mgr to finish the background job of computing expirying keys for the block")
	txmgr.pvtdataPurgeMgr.WaitForPrepareToFinish()
	txmgr.oldBlockCommit.Lock()
	defer txmgr.oldBlockCommit.Unlock()
	logger.Debug("lock acquired on oldBlockCommit for validating read set version against the committed version")

	block := blockAndPvtdata.Block
	logger.Debugf("Validating new block with num trans = [%d]", len(block.Data.Data))
	batch, txstatsInfo, err := txmgr.validator.ValidateAndPrepareBatch(blockAndPvtdata, doMVCCValidation)
	if err != nil {
		txmgr.reset()
		return nil, nil, err
	}
	txmgr.current = &current{block: block, batch: batch}
	if err := txmgr.invokeNamespaceListeners(); err != nil {
		txmgr.reset()
		return nil, nil, err
	}

	updateBytesBuilder := &privacyenabledstate.UpdatesBytesBuilder{}
	updateBytes, err := updateBytesBuilder.DeterministicBytesForPubAndHashUpdates(batch)
	return txstatsInfo, updateBytes, err
}

// RemoveStaleAndCommitPvtDataOfOldBlocks implements method in interface `txmgmt.TxMgr`
// The following six operations are performed:
// (1) contructs the unique pvt data from the passed blocksPvtData
// (2) acquire a lock on oldBlockCommit
// (3) checks for stale pvtData by comparing [version, valueHash] and removes stale data
// (4) creates update batch from the the non-stale pvtData
// (5) update the BTL bookkeeping managed by the purge manager and update expiring keys.
// (6) commit the non-stale pvt data to the stateDB
// This function assumes that the passed input contains only transactions that had been
// marked "Valid". In the current design, kvledger (a single consumer of this function),
// filters out the data of "invalid" transactions and supplies the data for "valid" transactions only.
func (txmgr *LockBasedTxMgr) RemoveStaleAndCommitPvtDataOfOldBlocks(blocksPvtData map[uint64][]*ledger.TxPvtData) error {
	// (0) Among ValidateAndPrepare(), PrepareExpiringKeys(), and
	// RemoveStaleAndCommitPvtDataOfOldBlocks(), we can allow only one
	// function to execute at a time. The reason is that each function calls
	// LoadCommittedVersions() which would clear the existing entries in the
	// transient buffer and load new entries (such a transient buffer is not
	// applicable for the golevelDB). As a result, these three functions can
	// interleave and nullify the optimization provided by the bulk read API.
	// Once the ledger cache (FAB-103) is introduced and existing
	// LoadCommittedVersions() is refactored to return a map, we can allow
	// these three functions to execute parallely. However, we cannot remove
	// the lock on oldBlockCommit as it is also used to avoid interleaving
	// between Commit() and execution of this function for the correctness.
	logger.Debug("Waiting for purge mgr to finish the background job of computing expirying keys for the block")
	txmgr.pvtdataPurgeMgr.WaitForPrepareToFinish()
	txmgr.oldBlockCommit.Lock()
	defer txmgr.oldBlockCommit.Unlock()
	logger.Debug("lock acquired on oldBlockCommit for committing pvtData of old blocks to state database")

	// (1) as the blocksPvtData can contain multiple versions of pvtData for
	// a given <ns, coll, key>, we need to find duplicate tuples with different
	// versions and use the one with the higher version
	logger.Debug("Constructing unique pvtData by removing duplicate entries")
	uniquePvtData, err := constructUniquePvtData(blocksPvtData)
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
	// been constructed during last PrepareExpiringKeys from commit, we need
	// to update the list. This is because RemoveStaleAndCommitPvtDataOfOldBlocks
	// may have added new data which might be eligible for expiry during the
	// next regular block commit.
	logger.Debug("Updating bookkeeping info in the purge manager")
	if err := txmgr.pvtdataPurgeMgr.UpdateBookkeepingForPvtDataOfOldBlocks(batch.PvtUpdates); err != nil {
		return err
	}

	// (6) commit the pvt data to the stateDB
	logger.Debug("Committing updates to state database")
	if err := txmgr.db.ApplyPrivacyAwareUpdates(batch, nil); err != nil {
		return err
	}
	return nil
}

type uniquePvtDataMap map[privacyenabledstate.HashedCompositeKey]*privacyenabledstate.PvtKVWrite

func constructUniquePvtData(blocksPvtData map[uint64][]*ledger.TxPvtData) (uniquePvtDataMap, error) {
	uniquePvtData := make(uniquePvtDataMap)
	// go over the blocksPvtData to find duplicate <ns, coll, key>
	// in the pvtWrites and use the one with the higher version number
	for blkNum, blockPvtData := range blocksPvtData {
		if err := uniquePvtData.updateUsingBlockPvtData(blockPvtData, blkNum); err != nil {
			return nil, err
		}
	} // for each block
	return uniquePvtData, nil
}

func (uniquePvtData uniquePvtDataMap) updateUsingBlockPvtData(blockPvtData []*ledger.TxPvtData, blkNum uint64) error {
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
				IsDelete: pvtWrite.IsDelete,
				Value:    pvtWrite.Value,
				Version:  ver,
			}
	}
}

func (uniquePvtData uniquePvtDataMap) findAndRemoveStalePvtData(db privacyenabledstate.DB) error {
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

func (uniquePvtData uniquePvtDataMap) loadCommittedVersionIntoCache(db privacyenabledstate.DB) error {
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
	kvWrite *privacyenabledstate.PvtKVWrite, db privacyenabledstate.DB) (bool, error) {

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
		for the remaining pvtData, we need to check for stalenss using hashed
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
		stateUpdatesForListener := extractStateUpdates(txmgr.current.batch, listener.InterestedInNamespaces())
		if len(stateUpdatesForListener) == 0 {
			continue
		}
		txmgr.current.listeners = append(txmgr.current.listeners, listener)

		committedStateQueryExecuter := &queryutil.QECombiner{
			QueryExecuters: []queryutil.QueryExecuter{txmgr.db}}

		postCommitQueryExecuter := &queryutil.QECombiner{
			QueryExecuters: []queryutil.QueryExecuter{
				&queryutil.UpdateBatchBackedQueryExecuter{UpdateBatch: txmgr.current.batch.PubUpdates.UpdateBatch},
				txmgr.db,
			},
		}

		trigger := &ledger.StateUpdateTrigger{
			LedgerID:                    txmgr.ledgerid,
			StateUpdates:                stateUpdatesForListener,
			CommittingBlockNum:          txmgr.current.blockNum(),
			CommittedStateQueryExecutor: committedStateQueryExecuter,
			PostCommitQueryExecutor:     postCommitQueryExecuter,
		}
		if err := listener.HandleStateUpdates(trigger); err != nil {
			return err
		}
		logger.Debugf("Invoking listener for state changes:%s", listener)
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
		txmgr.pvtdataPurgeMgr.PrepareForExpiringKeys(txmgr.current.blockNum())
		txmgr.pvtdataPurgeMgr.usedOnce = true
	}
	defer func() {
		txmgr.pvtdataPurgeMgr.PrepareForExpiringKeys(txmgr.current.blockNum() + 1)
		logger.Debugf("launched the background routine for preparing keys to purge with the next block")
		txmgr.reset()
	}()

	logger.Debugf("Committing updates to state database")
	if txmgr.current == nil {
		panic("validateAndPrepare() method should have been called before calling commit()")
	}

	if err := txmgr.pvtdataPurgeMgr.DeleteExpiredAndUpdateBookkeeping(
		txmgr.current.batch.PvtUpdates, txmgr.current.batch.HashUpdates); err != nil {
		return err
	}

	commitHeight := version.NewHeight(txmgr.current.blockNum(), txmgr.current.maxTxNumber())
	txmgr.commitRWLock.Lock()
	logger.Debugf("Write lock acquired for committing updates to state database")
	if err := txmgr.db.ApplyPrivacyAwareUpdates(txmgr.current.batch, commitHeight); err != nil {
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
	// In the case of error state listeners will not recieve this call - instead a peer panic is caused by the ledger upon receiveing
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
	if _, _, err := txmgr.ValidateAndPrepare(blockAndPvtdata, false); err != nil {
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

func extractStateUpdates(batch *privacyenabledstate.UpdateBatch, namespaces []string) ledger.StateUpdates {
	stateupdates := make(ledger.StateUpdates)
	for _, namespace := range namespaces {
		updatesMap := batch.PubUpdates.GetUpdates(namespace)
		var kvwrites []*kvrwset.KVWrite
		for key, versionedValue := range updatesMap {
			kvwrites = append(kvwrites, &kvrwset.KVWrite{Key: key, IsDelete: versionedValue.Value == nil, Value: versionedValue.Value})
			if len(kvwrites) > 0 {
				stateupdates[namespace] = kvwrites
			}
		}
	}
	return stateupdates
}

func (txmgr *LockBasedTxMgr) updateStateListeners() {
	for _, l := range txmgr.current.listeners {
		l.StateCommitDone(txmgr.ledgerid)
	}
}

func (txmgr *LockBasedTxMgr) reset() {
	txmgr.current = nil
}

// pvtdataPurgeMgr wraps the actual purge manager and an additional flag 'usedOnce'
// for usage of this additional flag, see the relevant comments in the txmgr.Commit() function above
type pvtdataPurgeMgr struct {
	pvtstatepurgemgmt.PurgeMgr
	usedOnce bool
}
