/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"math"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/confighistory"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/pkg/errors"
	"github.com/willf/bitset"
)

type SnapshotDataImporter struct {
	namespacesVisited      map[string]struct{}
	eligibilityAndBTLCache *eligibilityAndBTLCache

	db *leveldbhelper.DBHandle
}

func newSnapshotDataImporter(
	ledgerID string,
	dbHandle *leveldbhelper.DBHandle,
	membershipProvider ledger.MembershipInfoProvider,
	configHistoryRetriever *confighistory.Retriever,
) *SnapshotDataImporter {
	return &SnapshotDataImporter{
		namespacesVisited:      map[string]struct{}{},
		eligibilityAndBTLCache: newEligibilityAndBTLCache(ledgerID, membershipProvider, configHistoryRetriever),
		db:                     dbHandle,
	}
}

func (i *SnapshotDataImporter) ConsumeSnapshotData(
	namespace, collection string,
	keyHash, valueHash []byte,
	version *version.Height) error {

	if _, ok := i.namespacesVisited[namespace]; !ok {
		if err := i.eligibilityAndBTLCache.loadDataFor(namespace); err != nil {
			return err
		}
		i.namespacesVisited[namespace] = struct{}{}
	}

	blkNum := version.BlockNum
	txNum := version.TxNum

	isEligible, err := i.eligibilityAndBTLCache.isEligibile(namespace, collection, blkNum)
	if err != nil {
		return err
	}

	dbUpdater := &dbUpdater{
		db:    i.db,
		batch: i.db.NewUpdateBatch(),
	}

	err = dbUpdater.upsertMissingDataEntry(
		&missingDataKey{
			nsCollBlk{
				ns:     namespace,
				coll:   collection,
				blkNum: blkNum,
			},
		},
		txNum,
		isEligible,
	)
	if err != nil {
		return err
	}

	err = dbUpdater.upsertBootKVHashes(
		&bootKVHashesKey{
			ns:     namespace,
			coll:   collection,
			blkNum: blkNum,
			txNum:  txNum,
		},
		keyHash,
		valueHash,
	)
	if err != nil {
		return err
	}

	hasExpiry, expiringBlk := i.eligibilityAndBTLCache.hasExpiry(namespace, collection, blkNum)
	if hasExpiry {
		err := dbUpdater.upsertExpiryEntry(
			&expiryKey{
				committingBlk: blkNum,
				expiringBlk:   expiringBlk,
			},
			namespace, collection, txNum,
		)
		if err != nil {
			return err
		}
	}

	return dbUpdater.commitBatch()
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

type dbUpdater struct {
	db    *leveldbhelper.DBHandle
	batch *leveldbhelper.UpdateBatch
}

func (u *dbUpdater) upsertMissingDataEntry(key *missingDataKey, committingTxNum uint64, isEligible bool) error {
	var encKey []byte
	if isEligible {
		encKey = encodeElgPrioMissingDataKey(key)
	} else {
		encKey = encodeInelgMissingDataKey(key)
	}

	encVal, err := u.db.Get(encKey)
	if err != nil {
		return errors.WithMessage(err, "error while getting missing data bitmap from the store")
	}

	var missingData *bitset.BitSet
	if encVal != nil {
		if missingData, err = decodeMissingDataValue(encVal); err != nil {
			return err
		}
	} else {
		missingData = &bitset.BitSet{}
	}

	missingData.Set(uint(committingTxNum))
	encVal, err = encodeMissingDataValue(missingData)
	if err != nil {
		return err
	}
	u.batch.Put(encKey, encVal)
	return nil
}

func (u *dbUpdater) upsertBootKVHashes(key *bootKVHashesKey, keyHash, valueHash []byte) error {
	encKey := encodeBootKVHashesKey(key)
	encVal, err := u.db.Get(encKey)
	if err != nil {
		return err
	}

	var val *BootKVHashes
	if encVal != nil {
		if val, err = decodeBootKVHashesVal(encVal); err != nil {
			return err
		}
	} else {
		val = &BootKVHashes{}
	}

	val.List = append(val.List,
		&BootKVHash{
			KeyHash:   keyHash,
			ValueHash: valueHash,
		},
	)
	if encVal, err = encodeBootKVHashesVal(val); err != nil {
		return errors.Wrap(err, "error while marshalling BootKVHashes")
	}
	u.batch.Put(encKey, encVal)
	return nil
}

func (u *dbUpdater) upsertExpiryEntry(
	key *expiryKey,
	namesapce, collection string,
	txNum uint64,
) error {
	encKey := encodeExpiryKey(key)
	encVal, err := u.db.Get(encKey)
	if err != nil {
		return err
	}

	var val *ExpiryData
	if encVal != nil {
		if val, err = decodeExpiryValue(encVal); err != nil {
			return err
		}
	} else {
		val = newExpiryData()
	}

	val.addMissingData(namesapce, collection)
	val.addBootKVHash(namesapce, collection, txNum)
	encVal, err = encodeExpiryValue(val)
	if err != nil {
		return err
	}
	u.batch.Put(encKey, encVal)
	return nil
}

func (u *dbUpdater) commitBatch() error {
	return u.db.WriteBatch(u.batch, true)
}
