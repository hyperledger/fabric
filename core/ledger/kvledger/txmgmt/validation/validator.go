/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

// validator validates a tx against the latest committed state
// and preceding valid transactions with in the same block
type validator struct {
	db       *privacyenabledstate.DB
	hashFunc rwsetutil.HashFunc
}

// preLoadCommittedVersionOfRSet loads committed version of all keys in each
// transaction's read set into a cache.
func (v *validator) preLoadCommittedVersionOfRSet(blk *block) error {
	// Collect both public and hashed keys in read sets of all transactions in a given block
	var pubKeys []*statedb.CompositeKey
	var hashedKeys []*privacyenabledstate.HashedCompositeKey

	// pubKeysMap and hashedKeysMap are used to avoid duplicate entries in the
	// pubKeys and hashedKeys. Though map alone can be used to collect keys in
	// read sets and pass as an argument in LoadCommittedVersionOfPubAndHashedKeys(),
	// array is used for better code readability. On the negative side, this approach
	// might use some extra memory.
	pubKeysMap := make(map[statedb.CompositeKey]interface{})
	hashedKeysMap := make(map[privacyenabledstate.HashedCompositeKey]interface{})

	for _, tx := range blk.txs {
		for _, nsRWSet := range tx.rwset.NsRwSets {
			for _, kvRead := range nsRWSet.KvRwSet.Reads {
				compositeKey := statedb.CompositeKey{
					Namespace: nsRWSet.NameSpace,
					Key:       kvRead.Key,
				}
				if _, ok := pubKeysMap[compositeKey]; !ok {
					pubKeysMap[compositeKey] = nil
					pubKeys = append(pubKeys, &compositeKey)
				}

			}
			for _, colHashedRwSet := range nsRWSet.CollHashedRwSets {
				for _, kvHashedRead := range colHashedRwSet.HashedRwSet.HashedReads {
					hashedCompositeKey := privacyenabledstate.HashedCompositeKey{
						Namespace:      nsRWSet.NameSpace,
						CollectionName: colHashedRwSet.CollectionName,
						KeyHash:        string(kvHashedRead.KeyHash),
					}
					if _, ok := hashedKeysMap[hashedCompositeKey]; !ok {
						hashedKeysMap[hashedCompositeKey] = nil
						hashedKeys = append(hashedKeys, &hashedCompositeKey)
					}
				}
			}
		}
	}

	// Load committed version of all keys into a cache
	if len(pubKeys) > 0 || len(hashedKeys) > 0 {
		err := v.db.LoadCommittedVersionsOfPubAndHashedKeys(pubKeys, hashedKeys)
		if err != nil {
			return err
		}
	}

	return nil
}

// validateAndPrepareBatch performs validation and prepares the batch for final writes
func (v *validator) validateAndPrepareBatch(blk *block, doMVCCValidation bool) (*publicAndHashUpdates, []*AppInitiatedPurgeUpdate, error) {
	// Check whether statedb implements BulkOptimizable interface. For now,
	// only CouchDB implements BulkOptimizable to reduce the number of REST
	// API calls from peer to CouchDB instance.
	if v.db.IsBulkOptimizable() {
		err := v.preLoadCommittedVersionOfRSet(blk)
		if err != nil {
			return nil, nil, err
		}
	}

	updates := newPubAndHashUpdates()
	purgeTracker := newPvtdataPurgeTracker()

	for _, tx := range blk.txs {
		var validationCode peer.TxValidationCode
		var err error
		if validationCode, err = v.validateEndorserTX(tx.rwset, doMVCCValidation, updates); err != nil {
			return nil, nil, err
		}

		tx.validationCode = validationCode
		if validationCode == peer.TxValidationCode_VALID {
			logger.Debugf("Block [%d] Transaction index [%d] TxId [%s] marked as valid by state validator. ContainsPostOrderWrites [%t]", blk.num, tx.indexInBlock, tx.id, tx.containsPostOrderWrites)

			committingTxHeight := version.NewHeight(blk.num, uint64(tx.indexInBlock))
			if err := updates.applyWriteSet(tx.rwset, committingTxHeight, v.db, tx.containsPostOrderWrites); err != nil {
				return nil, nil, err
			}

			purgeTracker.update(tx.rwset, committingTxHeight)
		} else {
			logger.Warningf("Block [%d] Transaction index [%d] TxId [%s] marked as invalid by state validator. Reason code [%s]",
				blk.num, tx.indexInBlock, tx.id, validationCode.String())
		}
	}
	return updates, purgeTracker.getUpdates(), nil
}

// validateEndorserTX validates endorser transaction
func (v *validator) validateEndorserTX(
	txRWSet *rwsetutil.TxRwSet,
	doMVCCValidation bool,
	updates *publicAndHashUpdates) (peer.TxValidationCode, error) {
	validationCode := peer.TxValidationCode_VALID
	var err error
	// mvcc validation, may invalidate transaction
	if doMVCCValidation {
		validationCode, err = v.validateTx(txRWSet, updates)
	}
	return validationCode, err
}

func (v *validator) validateTx(txRWSet *rwsetutil.TxRwSet, updates *publicAndHashUpdates) (peer.TxValidationCode, error) {
	// Uncomment the following only for local debugging. Don't want to print data in the logs in production
	// logger.Debugf("validateTx - validating txRWSet: %s", spew.Sdump(txRWSet))
	for _, nsRWSet := range txRWSet.NsRwSets {
		ns := nsRWSet.NameSpace
		// Validate public reads
		if valid, err := v.validateReadSet(ns, nsRWSet.KvRwSet.Reads, updates.publicUpdates); !valid || err != nil {
			if err != nil {
				return peer.TxValidationCode(-1), err
			}
			return peer.TxValidationCode_MVCC_READ_CONFLICT, nil
		}
		// Validate range queries for phantom items
		if valid, err := v.validateRangeQueries(ns, nsRWSet.KvRwSet.RangeQueriesInfo, updates.publicUpdates); !valid || err != nil {
			if err != nil {
				return peer.TxValidationCode(-1), err
			}
			return peer.TxValidationCode_PHANTOM_READ_CONFLICT, nil
		}
		// Validate hashes for private reads
		if valid, err := v.validateNsHashedReadSets(ns, nsRWSet.CollHashedRwSets, updates.hashUpdates); !valid || err != nil {
			if err != nil {
				return peer.TxValidationCode(-1), err
			}
			return peer.TxValidationCode_MVCC_READ_CONFLICT, nil
		}
	}
	return peer.TxValidationCode_VALID, nil
}

// //////////////////////////////////////////////////////////////////////////////
// ///                 Validation of public read-set
// //////////////////////////////////////////////////////////////////////////////
func (v *validator) validateReadSet(ns string, kvReads []*kvrwset.KVRead, updates *privacyenabledstate.PubUpdateBatch) (bool, error) {
	for _, kvRead := range kvReads {
		if valid, err := v.validateKVRead(ns, kvRead, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// validateKVRead performs mvcc check for a key read during transaction simulation.
// i.e., it checks whether a key/version combination is already updated in the statedb (by an already committed block)
// or in the updates (by a preceding valid transaction in the current block)
func (v *validator) validateKVRead(ns string, kvRead *kvrwset.KVRead, updates *privacyenabledstate.PubUpdateBatch) (bool, error) {
	readVersion := rwsetutil.NewVersion(kvRead.Version)
	if updates.Exists(ns, kvRead.Key) {
		logger.Warnw("Transaction invalidation due to version mismatch, key in readset has been updated in a prior transaction in this block",
			"namespace", ns, "key", kvRead.Key, "readVersion", readVersion)
		return false, nil
	}
	committedVersion, err := v.db.GetVersion(ns, kvRead.Key)
	if err != nil {
		return false, err
	}

	logger.Debugw("Comparing readset version to committed version",
		"namespace", ns, "key", kvRead.Key, "readVersion", readVersion, "committedVersion", committedVersion)

	if !version.AreSame(committedVersion, readVersion) {
		logger.Warnw("Transaction invalidation due to version mismatch, readset version does not match committed version",
			"namespace", ns, "key", kvRead.Key, "readVersion", readVersion, "committedVersion", committedVersion)
		return false, nil
	}
	return true, nil
}

// //////////////////////////////////////////////////////////////////////////////
// ///                 Validation of range queries
// //////////////////////////////////////////////////////////////////////////////
func (v *validator) validateRangeQueries(ns string, rangeQueriesInfo []*kvrwset.RangeQueryInfo, updates *privacyenabledstate.PubUpdateBatch) (bool, error) {
	for _, rqi := range rangeQueriesInfo {
		if valid, err := v.validateRangeQuery(ns, rqi, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// validateRangeQuery performs a phantom read check i.e., it
// checks whether the results of the range query are still the same when executed on the
// statedb (latest state as of last committed block) + updates (prepared by the writes of preceding valid transactions
// in the current block and yet to be committed as part of group commit at the end of the validation of the block)
func (v *validator) validateRangeQuery(ns string, rangeQueryInfo *kvrwset.RangeQueryInfo, updates *privacyenabledstate.PubUpdateBatch) (bool, error) {
	logger.Debugf("validateRangeQuery: ns=%s, rangeQueryInfo=%s", ns, rangeQueryInfo)

	// If during simulation, the caller had not exhausted the iterator so
	// rangeQueryInfo.EndKey is not actual endKey given by the caller in the range query
	// but rather it is the last key seen by the caller and hence the combinedItr should include the endKey in the results.
	includeEndKey := !rangeQueryInfo.ItrExhausted

	combinedItr, err := newCombinedIterator(v.db, updates.UpdateBatch,
		ns, rangeQueryInfo.StartKey, rangeQueryInfo.EndKey, includeEndKey)
	if err != nil {
		return false, err
	}
	defer combinedItr.Close()
	var qv rangeQueryValidator
	if rangeQueryInfo.GetReadsMerkleHashes() != nil {
		logger.Debug(`Hashing results are present in the range query info hence, initiating hashing based validation`)
		qv = &rangeQueryHashValidator{hashFunc: v.hashFunc}
	} else {
		logger.Debug(`Hashing results are not present in the range query info hence, initiating raw KVReads based validation`)
		qv = &rangeQueryResultsValidator{}
	}
	if err := qv.init(rangeQueryInfo, combinedItr); err != nil {
		return false, err
	}
	return qv.validate()
}

// //////////////////////////////////////////////////////////////////////////////
// ///                 Validation of hashed read-set
// //////////////////////////////////////////////////////////////////////////////
func (v *validator) validateNsHashedReadSets(ns string, collHashedRWSets []*rwsetutil.CollHashedRwSet,
	updates *privacyenabledstate.HashedUpdateBatch) (bool, error) {
	for _, collHashedRWSet := range collHashedRWSets {
		if valid, err := v.validateCollHashedReadSet(ns, collHashedRWSet.CollectionName, collHashedRWSet.HashedRwSet.HashedReads, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

func (v *validator) validateCollHashedReadSet(ns, coll string, kvReadHashes []*kvrwset.KVReadHash,
	updates *privacyenabledstate.HashedUpdateBatch) (bool, error) {
	for _, kvReadHash := range kvReadHashes {
		if valid, err := v.validateKVReadHash(ns, coll, kvReadHash, updates); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// validateKVReadHash performs mvcc check for a hash of a key that is present in the private data space
// i.e., it checks whether a key/version combination is already updated in the statedb (by an already committed block)
// or in the updates (by a preceding valid transaction in the current block)
func (v *validator) validateKVReadHash(ns, coll string, kvReadHash *kvrwset.KVReadHash, updates *privacyenabledstate.HashedUpdateBatch) (bool, error) {
	readHashVersion := rwsetutil.NewVersion(kvReadHash.Version)
	if updates.Contains(ns, coll, kvReadHash.KeyHash) {
		logger.Warnw("Transaction invalidation due to hash version mismatch, hash key in readset has been updated in a prior transaction in this block",
			"namespace", ns, "collection", coll, "keyHash", kvReadHash.KeyHash, "readHashVersion", readHashVersion)
		return false, nil
	}
	committedVersion, err := v.db.GetKeyHashVersion(ns, coll, kvReadHash.KeyHash)
	if err != nil {
		return false, err
	}

	logger.Debugw("Comparing hash readset version to committed version",
		"namespace", ns, "collection", coll, "keyHash", kvReadHash.KeyHash, "readVersion", readHashVersion, "committedVersion", committedVersion)

	if !version.AreSame(committedVersion, readHashVersion) {
		logger.Warnw("Transaction invalidation due to hash version mismatch, readset version does not match committed version",
			"namespace", ns, "collection", coll, "keyHash", kvReadHash.KeyHash, "readVersion", readHashVersion, "committedVersion", committedVersion)
		return false, nil
	}
	return true, nil
}

type AppInitiatedPurgeUpdate struct {
	CompositeKey *privacyenabledstate.HashedCompositeKey
	Version      *version.Height
}

type pvtdataPurgeTracker struct {
	m map[privacyenabledstate.HashedCompositeKey]*AppInitiatedPurgeUpdate
}

func newPvtdataPurgeTracker() *pvtdataPurgeTracker {
	return &pvtdataPurgeTracker{
		m: map[privacyenabledstate.HashedCompositeKey]*AppInitiatedPurgeUpdate{},
	}
}

func (p *pvtdataPurgeTracker) update(rwset *rwsetutil.TxRwSet, version *version.Height) {
	for _, nsRwSets := range rwset.NsRwSets {
		for _, collHashedRwSet := range nsRwSets.CollHashedRwSets {
			for _, hashedWrite := range collHashedRwSet.HashedRwSet.GetHashedWrites() {

				ck := privacyenabledstate.HashedCompositeKey{
					Namespace:      nsRwSets.NameSpace,
					CollectionName: collHashedRwSet.CollectionName,
					KeyHash:        string(hashedWrite.GetKeyHash()),
				}

				if hashedWrite.IsPurge {
					p.m[ck] = &AppInitiatedPurgeUpdate{
						CompositeKey: &ck,
						Version:      version,
					}
				}
			}
		}
	}
}

func (p *pvtdataPurgeTracker) getUpdates() []*AppInitiatedPurgeUpdate {
	updates := []*AppInitiatedPurgeUpdate{}
	for _, update := range p.m {
		updates = append(updates, update)
	}
	return updates
}
