/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"time"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/pkg/errors"
)

const (
	previousDataVersion = ""
	currentDataVersion  = "2.5"
	maxUpgradeBatchSize = 4 * 1024 * 1024 // 4 MB
)

func CheckAndConstructHashedIndex(storePath string, ledgerIDs []string) error {
	info, err := leveldbhelper.RetrieveDataFormatInfo(storePath)
	if err != nil {
		return err
	}

	if info.IsDBEmpty || info.FormatVerison == currentDataVersion {
		return nil
	}

	if info.FormatVerison == previousDataVersion {
		if err := constructHashedIndex(storePath, ledgerIDs); err != nil {
			return err
		}
		return nil
	}

	return errors.Errorf("unexpected data version - cannot upgrade data format for pvtdatastore from %s to %s", info.FormatVerison, currentDataVersion)
}

// constructHashedIndex creates the HashedIndex entries for the private data keys and at the end sets the
// data format version to the current version (2.5)
func constructHashedIndex(storePath string, ledgerIDs []string) error {
	p, err := leveldbhelper.NewProvider(
		&leveldbhelper.Conf{
			DBPath:         storePath,
			ExpectedFormat: previousDataVersion,
		})
	if err != nil {
		return err
	}

	defer p.Close()

	for _, l := range ledgerIDs {
		db := p.GetDBHandle(l)
		if err := constructHashedIndexFor(l, db); err != nil {
			return err
		}
	}
	return p.SetDataFormat(currentDataVersion)
}

// constructHashedIndexFor creates the HashedIndex entries for a given ledger.
// In this function we also piggyback to upgrade any private data key from format V11 to V12
func constructHashedIndexFor(ledgerID string, db *leveldbhelper.DBHandle) error {
	startKey, endKey := entireDatakeyRange()
	itr, err := db.GetIterator(startKey, endKey)
	if err != nil {
		return err
	}
	defer itr.Release()

	batch := db.NewUpdateBatch()

	initialTime := time.Now()
	numEntriesCreated := 0
	logger.Infow("Starting creation of HashedIndex entries retroactively", "ledgerID", ledgerID)
	for itr.Next() {
		k := itr.Key()
		v := itr.Value()

		// In pvtdatastore, we used to persist the data of entire transaction as a single KV in V11.
		// Later, when we introduced BlockToLive, we started storing each collection data as a KV.
		// Here we leverage this opportunity (when we create HashedIndexKeys retroactively) to convert
		// the private data in V11 format, if any so in the next available opportunity,
		// we will remove the code where we have to detect this difference while processing queries on pvtdatastore.
		v11Fmt, err := v11Format(k)
		if err != nil {
			return err
		}

		if v11Fmt {
			blockNum, _, err := v11DecodePK(k)
			if err != nil {
				return err
			}

			txPvtData, err := v11DecodeKV(k, v)
			if err != nil {
				return err
			}

			dataEntries := prepareDataEntries(blockNum, []*ledger.TxPvtData{txPvtData})
			for _, dataEntry := range dataEntries {
				var dataKey, dataVal []byte
				dataKey = encodeDataKey(dataEntry.key)
				if dataVal, err = encodeDataValue(dataEntry.value); err != nil {
					return err
				}
				batch.Put(dataKey, dataVal)
				if err := addHashedIndexEntriesInto(batch, dataEntry.key, dataEntry.value); err != nil {
					return err
				}
				numEntriesCreated++
			}
			batch.Delete(k)
		} else {
			dataKey, err := decodeDatakey(k)
			if err != nil {
				return err
			}

			dataVal, err := decodeDataValue(v)
			if err != nil {
				return err
			}
			if err := addHashedIndexEntriesInto(batch, dataKey, dataVal); err != nil {
				return err
			}
			numEntriesCreated++
		}

		if batch.Size() >= maxUpgradeBatchSize {
			if err := db.WriteBatch(batch, true); err != nil {
				return err
			}
			if (time.Since(initialTime) / time.Second) > 5 {
				initialTime = time.Now()
				logger.Infow("Creating HashedIndex entries retroactively...", "total entries created", numEntriesCreated, "ledgerID", ledgerID)
			}
			batch.Reset()
		}
	}

	if err := db.WriteBatch(batch, true); err != nil {
		return err
	}
	logger.Infow("Creation of HashedIndex entries retroactively done", "total entries created", numEntriesCreated, "ledgerID", ledgerID)
	return nil
}

func addHashedIndexEntriesInto(batch *leveldbhelper.UpdateBatch, dataKey *dataKey, dataValue *rwset.CollectionPvtReadWriteSet) error {
	collPvtRWSet, err := rwsetutil.CollPvtRwSetFromProtoMsg(dataValue)
	if err != nil {
		return err
	}
	for _, kvWrite := range collPvtRWSet.KvRwSet.Writes {
		k := encodeHashedIndexKey(
			&hashedIndexKey{
				ns:         dataKey.ns,
				coll:       dataKey.coll,
				pvtkeyHash: util.ComputeStringHash(kvWrite.Key),
				blkNum:     dataKey.blkNum,
				txNum:      dataKey.txNum,
			},
		)
		batch.Put(k, []byte(kvWrite.Key))
	}
	return nil
}
