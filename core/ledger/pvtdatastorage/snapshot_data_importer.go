/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"bytes"
	"math"
	"os"

	"github.com/bits-and-blooms/bitset"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/pkg/errors"
)

var (
	// batch used for sorting the data
	maxSnapshotRowSortBatchSize = 1 * 1024 * 1024
	// batch used for writing the final data,
	// (64 bytes for a single kvhash + additional KVs and encoding overheads) * 10000 will roughly lead to batch size between 1MB and 2MB
	maxBatchLenForSnapshotImport = 10000
)

type SnapshotDataImporter struct {
	namespacesVisited      map[string]struct{}
	eligibilityAndBTLCache *eligibilityAndBTLCache

	rowsSorter *snapshotRowsSorter
	db         *leveldbhelper.DBHandle
}

func newSnapshotDataImporter(
	ledgerID string,
	dbHandle *leveldbhelper.DBHandle,
	membershipProvider ledger.MembershipInfoProvider,
	configHistoryRetriever *confighistory.Retriever,
	tempDirRoot string,
) (*SnapshotDataImporter, error) {
	rowsSorter, err := newSnapshotRowsSorter(tempDirRoot)
	if err != nil {
		return nil, err
	}
	return &SnapshotDataImporter{
		namespacesVisited:      map[string]struct{}{},
		eligibilityAndBTLCache: newEligibilityAndBTLCache(ledgerID, membershipProvider, configHistoryRetriever),
		rowsSorter:             rowsSorter,
		db:                     dbHandle,
	}, nil
}

func (i *SnapshotDataImporter) ConsumeSnapshotData(
	namespace, collection string,
	keyHash, valueHash []byte,
	version *version.Height,
) error {
	return i.rowsSorter.add(
		&snapshotRow{
			version:   version,
			ns:        namespace,
			coll:      collection,
			keyHash:   keyHash,
			valueHash: valueHash,
		},
	)
}

func (i *SnapshotDataImporter) Done() error {
	defer i.rowsSorter.cleanup()
	if err := i.rowsSorter.addingDone(); err != nil {
		return err
	}
	iter, err := i.rowsSorter.iterator()
	if err != nil {
		return err
	}
	defer iter.close()

	dbUpdates := newDBUpdates()
	currentBlockNum := uint64(0)

	for {
		row, err := iter.next()
		if err != nil {
			return err
		}
		if row == nil {
			// iterator exhausted. Commit all pending writes
			return dbUpdates.commitToDB(i.db)
		}

		namespace := row.ns
		collection := row.coll
		blkNum := row.version.BlockNum
		txNum := row.version.TxNum
		keyHash := row.keyHash
		valueHash := row.valueHash

		if blkNum != currentBlockNum {
			currentBlockNum = blkNum
			// commit is to be invoked only on block boundaries because we write data for one block only once
			if dbUpdates.numKVHashesEntries() >= maxBatchLenForSnapshotImport {
				if err := dbUpdates.commitToDB(i.db); err != nil {
					return err
				}
				dbUpdates = newDBUpdates()
			}
		}

		if _, ok := i.namespacesVisited[namespace]; !ok {
			if err := i.eligibilityAndBTLCache.loadDataFor(namespace); err != nil {
				return err
			}
			i.namespacesVisited[namespace] = struct{}{}
		}

		isEligible, err := i.eligibilityAndBTLCache.isEligibile(namespace, collection, blkNum)
		if err != nil {
			return err
		}

		if isEligible {
			dbUpdates.upsertElgMissingDataEntry(namespace, collection, blkNum, txNum)
		} else {
			dbUpdates.upsertInelgMissingDataEntry(namespace, collection, blkNum, txNum)
		}

		dbUpdates.upsertBootKVHashes(namespace, collection, blkNum, txNum, keyHash, valueHash)

		if hasExpiry, expiringBlk := i.eligibilityAndBTLCache.hasExpiry(namespace, collection, blkNum); hasExpiry {
			dbUpdates.upsertExpiryEntry(expiringBlk, blkNum, namespace, collection, txNum)
		}
	}
}

type nsColl struct {
	ns, coll string
}

type eligibility struct {
	configBlockNum uint64
	isEligible     bool
}

type eligibilityAndBTLCache struct {
	ledgerID               string
	membershipProvider     ledger.MembershipInfoProvider
	configHistoryRetriever *confighistory.Retriever

	eligibilityHistory map[nsColl][]*eligibility
	btl                map[nsColl]uint64
}

func newEligibilityAndBTLCache(
	ledgerID string,
	membershipProvider ledger.MembershipInfoProvider,
	configHistoryRetriever *confighistory.Retriever) *eligibilityAndBTLCache {
	return &eligibilityAndBTLCache{
		ledgerID:               ledgerID,
		membershipProvider:     membershipProvider,
		configHistoryRetriever: configHistoryRetriever,
		eligibilityHistory:     map[nsColl][]*eligibility{},
		btl:                    map[nsColl]uint64{},
	}
}

func (i *eligibilityAndBTLCache) loadDataFor(namespace string) error {
	var queryBlkNum uint64 = math.MaxUint64
	for {
		configInfo, err := i.configHistoryRetriever.MostRecentCollectionConfigBelow(queryBlkNum, namespace)
		if err != nil || configInfo == nil {
			return err
		}

		committingBlkNum := configInfo.CommittingBlockNum
		collections := configInfo.CollectionConfig.GetConfig()

		for _, collection := range collections {
			staticCollection := collection.GetStaticCollectionConfig()
			eligible, err := i.membershipProvider.AmMemberOf(i.ledgerID, staticCollection.MemberOrgsPolicy)
			if err != nil {
				return err
			}
			key := nsColl{
				ns:   namespace,
				coll: staticCollection.Name,
			}
			i.eligibilityHistory[key] = append(i.eligibilityHistory[key],
				&eligibility{
					configBlockNum: committingBlkNum,
					isEligible:     eligible,
				},
			)
			if staticCollection.BlockToLive > 0 {
				i.btl[key] = staticCollection.BlockToLive
			}
		}
		queryBlkNum = committingBlkNum
	}
}

func (i *eligibilityAndBTLCache) isEligibile(namespace, collection string, dataBlockNum uint64) (bool, error) {
	if implicitcollection.IsImplicitCollection(collection) {
		return collection == i.membershipProvider.MyImplicitCollectionName(), nil
	}

	key := nsColl{
		ns:   namespace,
		coll: collection,
	}
	history := i.eligibilityHistory[key]

	if len(history) == 0 {
		return false,
			errors.Errorf(
				"unexpected error - no collection config history for <namespace=%s, collection=%s>",
				namespace, collection,
			)
	}

	if dataBlockNum <= history[len(history)-1].configBlockNum {
		return false,
			errors.Errorf(
				"unexpected error - no collection config found below block number [%d] for <namespace=%s, collection=%s>",
				dataBlockNum, namespace, collection,
			)
	}

	for _, h := range history {
		if h.configBlockNum >= dataBlockNum {
			if h.isEligible {
				return true, nil
			}
			continue
		}
		return h.isEligible, nil
	}

	return false, errors.Errorf("unexpected code path - potential bug")
}

func (i *eligibilityAndBTLCache) hasExpiry(namespace, collection string, committingBlk uint64) (bool, uint64) {
	var expiringBlk uint64 = math.MaxUint64
	btl, ok := i.btl[nsColl{
		ns:   namespace,
		coll: collection,
	}]
	if ok {
		expiringBlk = pvtdatapolicy.ComputeExpiringBlock(namespace, collection, committingBlk, btl)
	}
	return expiringBlk < math.MaxUint64, expiringBlk
}

type dbUpdates struct {
	elgMissingDataEntries   map[missingDataKey]*bitset.BitSet
	inelgMissingDataEntries map[missingDataKey]*bitset.BitSet
	bootKVHashes            map[bootKVHashesKey]*BootKVHashes
	expiryEntries           map[expiryKey]*ExpiryData
}

func newDBUpdates() *dbUpdates {
	return &dbUpdates{
		elgMissingDataEntries:   map[missingDataKey]*bitset.BitSet{},
		inelgMissingDataEntries: map[missingDataKey]*bitset.BitSet{},
		bootKVHashes:            map[bootKVHashesKey]*BootKVHashes{},
		expiryEntries:           map[expiryKey]*ExpiryData{},
	}
}

func (u *dbUpdates) upsertElgMissingDataEntry(ns, coll string, blkNum, txNum uint64) {
	key := missingDataKey{
		nsCollBlk{
			ns:     ns,
			coll:   coll,
			blkNum: blkNum,
		},
	}
	missingData, ok := u.elgMissingDataEntries[key]
	if !ok {
		missingData = &bitset.BitSet{}
		u.elgMissingDataEntries[key] = missingData
	}
	missingData.Set(uint(txNum))
}

func (u *dbUpdates) upsertInelgMissingDataEntry(ns, coll string, blkNum, txNum uint64) {
	key := missingDataKey{
		nsCollBlk{
			ns:     ns,
			coll:   coll,
			blkNum: blkNum,
		},
	}
	missingData, ok := u.inelgMissingDataEntries[key]
	if !ok {
		missingData = &bitset.BitSet{}
		u.inelgMissingDataEntries[key] = missingData
	}
	missingData.Set(uint(txNum))
}

func (u *dbUpdates) upsertBootKVHashes(ns, coll string, blkNum, txNum uint64, keyHash, valueHash []byte) {
	key := bootKVHashesKey{
		blkNum: blkNum,
		txNum:  txNum,
		ns:     ns,
		coll:   coll,
	}
	bootKVHashes, ok := u.bootKVHashes[key]
	if !ok {
		bootKVHashes = &BootKVHashes{}
		u.bootKVHashes[key] = bootKVHashes
	}
	bootKVHashes.List = append(bootKVHashes.List,
		&BootKVHash{
			KeyHash:   keyHash,
			ValueHash: valueHash,
		},
	)
}

func (u *dbUpdates) upsertExpiryEntry(expiringBlk, committingBlk uint64, namespace, collection string, txNum uint64) {
	key := expiryKey{
		expiringBlk:   expiringBlk,
		committingBlk: committingBlk,
	}
	expiryData, ok := u.expiryEntries[key]
	if !ok {
		expiryData = newExpiryData()
		u.expiryEntries[key] = expiryData
	}
	expiryData.addMissingData(namespace, collection)
	expiryData.addBootKVHash(namespace, collection, txNum)
}

func (u *dbUpdates) numKVHashesEntries() int {
	return len(u.bootKVHashes)
}

func (u *dbUpdates) commitToDB(db *leveldbhelper.DBHandle) error {
	batch := db.NewUpdateBatch()
	for k, v := range u.elgMissingDataEntries {
		encKey := encodeElgPrioMissingDataKey(&k)
		encVal, err := encodeMissingDataValue(v)
		if err != nil {
			return err
		}
		batch.Put(encKey, encVal)
	}

	for k, v := range u.inelgMissingDataEntries {
		encKey := encodeInelgMissingDataKey(&k)
		encVal, err := encodeMissingDataValue(v)
		if err != nil {
			return err
		}
		batch.Put(encKey, encVal)
	}

	for k, v := range u.bootKVHashes {
		encKey := encodeBootKVHashesKey(&k)
		encVal, err := encodeBootKVHashesVal(v)
		if err != nil {
			return err
		}
		batch.Put(encKey, encVal)
	}

	for k, v := range u.expiryEntries {
		encKey := encodeExpiryKey(&k)
		encVal, err := encodeExpiryValue(v)
		if err != nil {
			return err
		}
		batch.Put(encKey, encVal)
	}
	return db.WriteBatch(batch, true)
}

type snapshotRowsSorter struct {
	tempDir    string
	dbProvider *leveldbhelper.Provider
	db         *leveldbhelper.DBHandle
	batch      *leveldbhelper.UpdateBatch
	batchSize  int
}

func newSnapshotRowsSorter(tempDirRoot string) (*snapshotRowsSorter, error) {
	tempDir, err := os.MkdirTemp(tempDirRoot, "pvtdatastore-snapshotdatainporter-")
	if err != nil {
		return nil, errors.Wrap(err, "error while creating temp dir for sorting rows")
	}
	dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{
		DBPath: tempDir,
	})
	if err != nil {
		return nil, err
	}
	db := dbProvider.GetDBHandle("")
	batch := db.NewUpdateBatch()
	return &snapshotRowsSorter{
		tempDir:    tempDir,
		dbProvider: dbProvider,
		db:         db,
		batch:      batch,
	}, nil
}

func (s *snapshotRowsSorter) add(k *snapshotRow) error {
	encKey := encodeSnapshotRowForSorting(k)
	s.batch.Put(encKey, []byte{})
	s.batchSize += len(encKey)

	if s.batchSize >= maxSnapshotRowSortBatchSize {
		if err := s.db.WriteBatch(s.batch, false); err != nil {
			return err
		}
		s.batch.Reset()
		s.batchSize = 0
	}
	return nil
}

func (s *snapshotRowsSorter) addingDone() error {
	return s.db.WriteBatch(s.batch, false)
}

func (s *snapshotRowsSorter) iterator() (*sortedSnapshotRowsIterator, error) {
	dbIter, err := s.db.GetIterator(nil, nil)
	if err != nil {
		return nil, err
	}
	return &sortedSnapshotRowsIterator{
		dbIter: dbIter,
	}, nil
}

func (s *snapshotRowsSorter) cleanup() {
	s.dbProvider.Close()
	if err := os.RemoveAll(s.tempDir); err != nil {
		logger.Errorw("Error while deleting temp dir [%s]", s.tempDir)
	}
}

type sortedSnapshotRowsIterator struct {
	dbIter *leveldbhelper.Iterator
}

func (i *sortedSnapshotRowsIterator) next() (*snapshotRow, error) {
	hasMore := i.dbIter.Next()
	if err := i.dbIter.Error(); err != nil {
		return nil, err
	}
	if !hasMore {
		return nil, nil
	}
	encKey := i.dbIter.Key()
	encKeyCopy := make([]byte, len(encKey))
	copy(encKeyCopy, encKey)
	row, err := decodeSnapshotRowFromSortEncoding(encKeyCopy)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (i *sortedSnapshotRowsIterator) close() {
	i.dbIter.Release()
}

type snapshotRow struct {
	version            *version.Height
	ns, coll           string
	keyHash, valueHash []byte
}

func encodeSnapshotRowForSorting(k *snapshotRow) []byte {
	encKey := k.version.ToBytes()
	encKey = append(encKey, []byte(k.ns)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, []byte(k.coll)...)
	encKey = append(encKey, nilByte)
	encKey = append(encKey, util.EncodeOrderPreservingVarUint64(uint64(len(k.keyHash)))...)
	encKey = append(encKey, k.keyHash...)
	encKey = append(encKey, k.valueHash...)
	return encKey
}

func decodeSnapshotRowFromSortEncoding(encKey []byte) (*snapshotRow, error) {
	version, bytesConsumed, err := version.NewHeightFromBytes(encKey)
	if err != nil {
		return nil, err
	}

	remainingBytes := encKey[bytesConsumed:]
	nsCollKVHash := bytes.SplitN(remainingBytes, []byte{nilByte}, 3)
	ns := nsCollKVHash[0]
	coll := nsCollKVHash[1]
	kvHashes := nsCollKVHash[2]

	keyHashLen, bytesConsumed, err := util.DecodeOrderPreservingVarUint64(kvHashes)
	if err != nil {
		return nil, err
	}
	keyHash := kvHashes[bytesConsumed : bytesConsumed+int(keyHashLen)]
	valueHash := kvHashes[bytesConsumed+int(keyHashLen):]
	return &snapshotRow{
		version:   version,
		ns:        string(ns),
		coll:      string(coll),
		keyHash:   keyHash,
		valueHash: valueHash,
	}, nil
}
