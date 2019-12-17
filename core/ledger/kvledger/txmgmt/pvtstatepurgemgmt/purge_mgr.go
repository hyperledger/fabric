/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	"math"
	"sync"

	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/util"
)

// PurgeMgr manages purging of the expired pvtdata
type PurgeMgr interface {
	// PrepareForExpiringKeys gives a chance to the PurgeMgr to do background work in advance if any
	PrepareForExpiringKeys(expiringAtBlk uint64)
	// WaitForPrepareToFinish holds the caller till the background goroutine launched by 'PrepareForExpiringKeys' is finished
	WaitForPrepareToFinish()
	// DeleteExpiredAndUpdateBookkeeping updates the bookkeeping and modifies the update batch by adding the deletes for the expired pvtdata
	DeleteExpiredAndUpdateBookkeeping(
		pvtUpdates *privacyenabledstate.PvtUpdateBatch,
		hashedUpdates *privacyenabledstate.HashedUpdateBatch) error
	// UpdateBookkeepingForPvtDataOfOldBlocks updates the existing expiry entries in the bookkeeper with the given pvtUpdates
	UpdateBookkeepingForPvtDataOfOldBlocks(pvtUpdates *privacyenabledstate.PvtUpdateBatch) error
	// BlockCommitDone is a callback to the PurgeMgr when the block is committed to the ledger
	BlockCommitDone() error
}

type keyAndVersion struct {
	key             string
	committingBlock uint64
	purgeKeyOnly    bool
}

type expiryInfoMap map[privacyenabledstate.HashedCompositeKey]*keyAndVersion

type workingset struct {
	toPurge             expiryInfoMap
	toClearFromSchedule []*expiryInfoKey
	expiringBlk         uint64
	err                 error
}

type purgeMgr struct {
	btlPolicy pvtdatapolicy.BTLPolicy
	db        privacyenabledstate.DB
	expKeeper expiryKeeper

	lock    *sync.Mutex
	waitGrp *sync.WaitGroup

	workingset *workingset
}

// InstantiatePurgeMgr instantiates a PurgeMgr.
func InstantiatePurgeMgr(ledgerid string, db privacyenabledstate.DB, btlPolicy pvtdatapolicy.BTLPolicy, bookkeepingProvider bookkeeping.Provider) (PurgeMgr, error) {
	return &purgeMgr{
		btlPolicy: btlPolicy,
		db:        db,
		expKeeper: newExpiryKeeper(ledgerid, bookkeepingProvider),
		lock:      &sync.Mutex{},
		waitGrp:   &sync.WaitGroup{},
	}, nil
}

// PrepareForExpiringKeys implements function in the interface 'PurgeMgr'
func (p *purgeMgr) PrepareForExpiringKeys(expiringAtBlk uint64) {
	p.waitGrp.Add(1)
	go func() {
		p.lock.Lock()
		p.waitGrp.Done()
		defer p.lock.Unlock()
		p.workingset = p.prepareWorkingsetFor(expiringAtBlk)
	}()
	p.waitGrp.Wait()
}

// WaitForPrepareToFinish implements function in the interface 'PurgeMgr'
func (p *purgeMgr) WaitForPrepareToFinish() {
	p.lock.Lock()
	p.lock.Unlock()
}

func (p *purgeMgr) UpdateBookkeepingForPvtDataOfOldBlocks(pvtUpdates *privacyenabledstate.PvtUpdateBatch) error {
	builder := newExpiryScheduleBuilder(p.btlPolicy)
	pvtUpdateCompositeKeyMap := pvtUpdates.ToCompositeKeyMap()
	for k, vv := range pvtUpdateCompositeKeyMap {
		builder.add(k.Namespace, k.CollectionName, k.Key, util.ComputeStringHash(k.Key), vv)
	}

	var updatedList []*expiryInfo
	for _, toAdd := range builder.getExpiryInfo() {
		toUpdate, err := p.expKeeper.retrieveByExpiryKey(toAdd.expiryInfoKey)
		if err != nil {
			return err
		}
		// Though we could update the existing entry (as there should be one due
		// to only the keyHash of this pvtUpdateKey), for simplicity and to be less
		// expensive, we append a new entry
		toUpdate.pvtdataKeys.addAll(toAdd.pvtdataKeys)
		updatedList = append(updatedList, toUpdate)
	}

	// As the expiring keys list might have been constructed after the last
	// regular block commit, we need to update the list. This is because,
	// some of the old pvtData which are being committed might get expired
	// during the next regular block commit. As a result, the corresponding
	// hashedKey in the expiring keys list would be missing the pvtData.
	p.addMissingPvtDataToWorkingSet(pvtUpdateCompositeKeyMap)

	return p.expKeeper.updateBookkeeping(updatedList, nil)
}

func (p *purgeMgr) addMissingPvtDataToWorkingSet(pvtKeys privacyenabledstate.PvtdataCompositeKeyMap) {
	if p.workingset == nil || len(p.workingset.toPurge) == 0 {
		return
	}

	for k := range pvtKeys {
		hashedCompositeKey := privacyenabledstate.HashedCompositeKey{
			Namespace:      k.Namespace,
			CollectionName: k.CollectionName,
			KeyHash:        string(util.ComputeStringHash(k.Key))}

		toPurgeKey, ok := p.workingset.toPurge[hashedCompositeKey]
		if !ok {
			// corresponding hashedKey is not present in the
			// expiring keys list
			continue
		}

		// if the purgeKeyOnly is set, it means that the version of the pvtKey
		// stored in the stateDB is older than the version of the hashedKey.
		// As a result, only the pvtKey needs to be purged (expiring block height
		// for the recent hashedKey would be higher). If the recent
		// pvtKey of the corresponding hashedKey is being committed, we need to
		// remove the purgeKeyOnly entries from the toPurgeList it is going to be
		// updated by the commit of missing pvtData
		if toPurgeKey.purgeKeyOnly {
			delete(p.workingset.toPurge, hashedCompositeKey)
		} else {
			toPurgeKey.key = k.Key
		}
	}
}

// DeleteExpiredAndUpdateBookkeeping implements function in the interface 'PurgeMgr'
func (p *purgeMgr) DeleteExpiredAndUpdateBookkeeping(
	pvtUpdates *privacyenabledstate.PvtUpdateBatch,
	hashedUpdates *privacyenabledstate.HashedUpdateBatch) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.workingset.err != nil {
		return p.workingset.err
	}

	listExpiryInfo, err := buildExpirySchedule(p.btlPolicy, pvtUpdates, hashedUpdates)
	if err != nil {
		return err
	}

	// For each key selected for purging, check if the key is not getting updated in the current block,
	// add its deletion in the update batches for pvt and hashed updates
	for compositeHashedKey, keyAndVersion := range p.workingset.toPurge {
		ns := compositeHashedKey.Namespace
		coll := compositeHashedKey.CollectionName
		keyHash := []byte(compositeHashedKey.KeyHash)
		key := keyAndVersion.key
		purgeKeyOnly := keyAndVersion.purgeKeyOnly
		hashUpdated := hashedUpdates.Contains(ns, coll, keyHash)
		pvtKeyUpdated := pvtUpdates.Contains(ns, coll, key)

		logger.Debugf("Checking whether the key [ns=%s, coll=%s, keyHash=%x, purgeKeyOnly=%t] "+
			"is updated in the update batch for the committing block - hashUpdated=%t, and pvtKeyUpdated=%t",
			ns, coll, keyHash, purgeKeyOnly, hashUpdated, pvtKeyUpdated)

		expiringTxVersion := version.NewHeight(p.workingset.expiringBlk, math.MaxUint64)
		if !hashUpdated && !purgeKeyOnly {
			logger.Debugf("Adding the hashed key to be purged to the delete list in the update batch")
			hashedUpdates.Delete(ns, coll, keyHash, expiringTxVersion)
		}
		if key != "" && !pvtKeyUpdated {
			logger.Debugf("Adding the pvt key to be purged to the delete list in the update batch")
			pvtUpdates.Delete(ns, coll, key, expiringTxVersion)
		}
	}
	return p.expKeeper.updateBookkeeping(listExpiryInfo, nil)
}

// BlockCommitDone implements function in the interface 'PurgeMgr'
// These orphan entries for purge-schedule can be cleared off in bulk in a separate background routine as well
// If we maintain the following logic (i.e., clear off entries just after block commit), we need a TODO -
// We need to perform a check in the start, because there could be a crash between the block commit and
// invocation to this function resulting in the orphan entry for the deletes scheduled for the last block
// Also, the another way is to club the delete of these entries in the same batch that adds entries for the future expirations -
// however, that requires updating the expiry store by replaying the last block from blockchain in order to sustain a crash between
// entries updates and block commit
func (p *purgeMgr) BlockCommitDone() error {
	defer func() { p.workingset = nil }()
	return p.expKeeper.updateBookkeeping(nil, p.workingset.toClearFromSchedule)
}

// prepareWorkingsetFor returns a working set for a given expiring block 'expiringAtBlk'.
// This working set contains the pvt data keys that will expire with the commit of block 'expiringAtBlk'.
func (p *purgeMgr) prepareWorkingsetFor(expiringAtBlk uint64) *workingset {
	logger.Debugf("Preparing potential purge list working-set for expiringAtBlk [%d]", expiringAtBlk)
	workingset := &workingset{expiringBlk: expiringAtBlk}
	// Retrieve the keys from bookkeeper
	expiryInfo, err := p.expKeeper.retrieve(expiringAtBlk)
	if err != nil {
		workingset.err = err
		return workingset
	}
	// Transform the keys into the form such that for each hashed key that is eligible for purge appears in 'toPurge'
	toPurge := transformToExpiryInfoMap(expiryInfo)
	// Load the latest versions of the hashed keys
	p.preloadCommittedVersionsInCache(toPurge)
	var expiryInfoKeysToClear []*expiryInfoKey

	if len(toPurge) == 0 {
		logger.Debugf("No expiry entry found for expiringAtBlk [%d]", expiringAtBlk)
		return workingset
	}
	logger.Debugf("Total [%d] expiring entries found. Evaluating whether some of these keys have been overwritten in later blocks...", len(toPurge))

	for purgeEntryK, purgeEntryV := range toPurge {
		logger.Debugf("Evaluating for hashedKey [%s]", purgeEntryK)
		expiryInfoKeysToClear = append(expiryInfoKeysToClear, &expiryInfoKey{committingBlk: purgeEntryV.committingBlock, expiryBlk: expiringAtBlk})
		currentVersion, err := p.db.GetKeyHashVersion(purgeEntryK.Namespace, purgeEntryK.CollectionName, []byte(purgeEntryK.KeyHash))
		if err != nil {
			workingset.err = err
			return workingset
		}

		if sameVersion(currentVersion, purgeEntryV.committingBlock) {
			logger.Debugf(
				"The version of the hashed key in the committed state and in the expiry entry is same " +
					"hence, keeping the entry in the purge list")
			continue
		}

		logger.Debugf("The version of the hashed key in the committed state and in the expiry entry is different")
		if purgeEntryV.key != "" {
			logger.Debugf("The expiry entry also contains the raw key along with the key hash")
			committedPvtVerVal, err := p.db.GetPrivateData(purgeEntryK.Namespace, purgeEntryK.CollectionName, purgeEntryV.key)
			if err != nil {
				workingset.err = err
				return workingset
			}

			if sameVersionFromVal(committedPvtVerVal, purgeEntryV.committingBlock) {
				logger.Debugf(
					"The version of the pvt key in the committed state and in the expiry entry is same" +
						"Including only key in the purge list and not the hashed key")
				purgeEntryV.purgeKeyOnly = true
				continue
			}
		}

		// If we reached here, the keyhash and private key (if present, in the expiry entry) have been updated in a later block, therefore remove from current purge list
		logger.Debugf("Removing from purge list - the key hash and key (if present, in the expiry entry)")
		delete(toPurge, purgeEntryK)
	}
	// Final keys to purge from state
	workingset.toPurge = toPurge
	// Keys to clear from bookkeeper
	workingset.toClearFromSchedule = expiryInfoKeysToClear
	return workingset
}

func (p *purgeMgr) preloadCommittedVersionsInCache(expInfoMap expiryInfoMap) {
	if !p.db.IsBulkOptimizable() {
		return
	}
	var hashedKeys []*privacyenabledstate.HashedCompositeKey
	for k := range expInfoMap {
		hashedKeys = append(hashedKeys, &k)
	}
	p.db.LoadCommittedVersionsOfPubAndHashedKeys(nil, hashedKeys)
}

func transformToExpiryInfoMap(expiryInfo []*expiryInfo) expiryInfoMap {
	expinfoMap := make(expiryInfoMap)
	for _, expinfo := range expiryInfo {
		for ns, colls := range expinfo.pvtdataKeys.Map {
			for coll, keysAndHashes := range colls.Map {
				for _, keyAndHash := range keysAndHashes.List {
					compositeKey := privacyenabledstate.HashedCompositeKey{Namespace: ns, CollectionName: coll, KeyHash: string(keyAndHash.Hash)}
					expinfoMap[compositeKey] = &keyAndVersion{key: keyAndHash.Key, committingBlock: expinfo.expiryInfoKey.committingBlk}
				}
			}
		}
	}
	return expinfoMap
}

func sameVersion(version *version.Height, blockNum uint64) bool {
	return version != nil && version.BlockNum == blockNum
}

func sameVersionFromVal(vv *statedb.VersionedValue, blockNum uint64) bool {
	return vv != nil && sameVersion(vv.Version, blockNum)
}
