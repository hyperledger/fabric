/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/willf/bitset"
)

// CommitPvtDataOfOldBlocks commits the pvtData (i.e., previously missing data) of old blockp.
// The parameter `blocksPvtData` refers a list of old block's pvtdata which are missing in the pvtstore.
// Given a list of old block's pvtData, `CommitPvtDataOfOldBlocks` performs the following three
// operations
// (1) construct update entries (i.e., dataEntries, expiryEntries, missingDataEntries)
//     from the above created data entries
// (2) create a db update batch from the update entries
// (3) commit the update batch to the pvtStore
func (s *Store) CommitPvtDataOfOldBlocks(blocksPvtData map[uint64][]*ledger.TxPvtData) error {
	if s.isLastUpdatedOldBlocksSet {
		return &ErrIllegalCall{`The lastUpdatedOldBlocksList is set. It means that the
		stateDB may not be in sync with the pvtStore`}
	}

	oldBlkDataProccessor := &oldBlockDataProcessor{
		Store: s,
	}

	logger.Debugf("Constructing pvtdatastore entries for pvtData of [%d] old blocks", len(blocksPvtData))
	entries, err := oldBlkDataProccessor.constructEntries(blocksPvtData)
	if err != nil {
		return err
	}

	logger.Debug("Constructing update batch from pvtdatastore entries")
	batch := s.db.NewUpdateBatch()
	if err := entries.addToUpdateBatch(batch); err != nil {
		return err
	}

	logger.Debug("Committing the update batch to pvtdatastore")
	return s.db.WriteBatch(batch, true)
}

type oldBlockDataProcessor struct {
	*Store
}

func (p *oldBlockDataProcessor) constructEntries(blocksPvtData map[uint64][]*ledger.TxPvtData) (*entriesForPvtDataOfOldBlocks, error) {
	var dataEntries []*dataEntry
	for blkNum, pvtData := range blocksPvtData {
		dataEntries = append(dataEntries, prepareDataEntries(blkNum, pvtData)...)
	}

	entries := &entriesForPvtDataOfOldBlocks{
		dataEntries:        make(map[dataKey]*rwset.CollectionPvtReadWriteSet),
		expiryEntries:      make(map[expiryKey]*ExpiryData),
		missingDataEntries: make(map[nsCollBlk]*bitset.BitSet),
	}

	for _, dataEntry := range dataEntries {
		var expData *ExpiryData
		nsCollBlk := dataEntry.key.nsCollBlk
		txNum := dataEntry.key.txNum

		expKey, err := p.constructExpiryKeyFromDataEntry(dataEntry)
		if err != nil {
			return nil, err
		}
		if !neverExpires(expKey.expiringBlk) {
			if expData, err = p.getExpiryDataFromEntriesOrStore(entries, expKey); err != nil {
				return nil, err
			}
			if expData == nil {
				// data entry is already expired
				// and purged (a rare scenario)
				continue
			}
			expData.addPresentData(nsCollBlk.ns, nsCollBlk.coll, txNum)
		}

		var missingData *bitset.BitSet
		if missingData, err = p.getMissingDataFromEntriesOrStore(entries, nsCollBlk); err != nil {
			return nil, err
		}
		if missingData == nil {
			// data entry is already expired
			// and purged (a rare scenario)
			continue
		}
		missingData.Clear(uint(txNum))

		entries.add(dataEntry, expKey, expData, missingData)
	}
	return entries, nil
}

func (p *oldBlockDataProcessor) constructExpiryKeyFromDataEntry(dataEntry *dataEntry) (expiryKey, error) {
	// get the expiryBlk number to construct the expiryKey
	nsCollBlk := dataEntry.key.nsCollBlk
	expiringBlk, err := p.btlPolicy.GetExpiringBlock(nsCollBlk.ns, nsCollBlk.coll, nsCollBlk.blkNum)
	if err != nil {
		return expiryKey{}, err
	}

	return expiryKey{
		expiringBlk:   expiringBlk,
		committingBlk: nsCollBlk.blkNum,
	}, nil
}

func (p *oldBlockDataProcessor) getExpiryDataFromEntriesOrStore(entries *entriesForPvtDataOfOldBlocks, expiryKey expiryKey) (*ExpiryData, error) {
	if expiryData, ok := entries.expiryEntries[expiryKey]; ok {
		return expiryData, nil
	}

	expiryData, err := p.getExpiryDataOfExpiryKey(&expiryKey)
	if err != nil {
		return nil, err
	}
	return expiryData, nil
}

func (p *oldBlockDataProcessor) getMissingDataFromEntriesOrStore(entries *entriesForPvtDataOfOldBlocks, nsCollBlk nsCollBlk) (*bitset.BitSet, error) {
	if missingData, ok := entries.missingDataEntries[nsCollBlk]; ok {
		return missingData, nil
	}

	missingDataKey := &missingDataKey{
		nsCollBlk:  nsCollBlk,
		isEligible: true,
	}
	missingData, err := p.getBitmapOfMissingDataKey(missingDataKey)
	if err != nil {
		return nil, err
	}
	return missingData, nil
}

type entriesForPvtDataOfOldBlocks struct {
	dataEntries        map[dataKey]*rwset.CollectionPvtReadWriteSet
	expiryEntries      map[expiryKey]*ExpiryData
	missingDataEntries map[nsCollBlk]*bitset.BitSet
}

func (e *entriesForPvtDataOfOldBlocks) add(datEntry *dataEntry, expKey expiryKey, expData *ExpiryData, missingData *bitset.BitSet) {
	dataKey := dataKey{
		nsCollBlk: datEntry.key.nsCollBlk,
		txNum:     datEntry.key.txNum,
	}
	e.dataEntries[dataKey] = datEntry.value

	if expData != nil {
		e.expiryEntries[expKey] = expData
	}

	e.missingDataEntries[dataKey.nsCollBlk] = missingData
}

func (e *entriesForPvtDataOfOldBlocks) addToUpdateBatch(batch *leveldbhelper.UpdateBatch) error {
	if err := e.addDataEntriesToUpdateBatch(batch); err != nil {
		return err
	}

	if err := e.addExpiryEntriesToUpdateBatch(batch); err != nil {
		return err
	}

	return e.addMissingDataEntriesToUpdateBatch(batch)
}

func (e *entriesForPvtDataOfOldBlocks) addDataEntriesToUpdateBatch(batch *leveldbhelper.UpdateBatch) error {
	var key, val []byte
	var err error

	for dataKey, pvtData := range e.dataEntries {
		key = encodeDataKey(&dataKey)
		if val, err = encodeDataValue(pvtData); err != nil {
			return err
		}
		batch.Put(key, val)
	}
	return nil
}

func (e *entriesForPvtDataOfOldBlocks) addExpiryEntriesToUpdateBatch(batch *leveldbhelper.UpdateBatch) error {
	var key, val []byte
	var err error

	for expiryKey, expiryData := range e.expiryEntries {
		key = encodeExpiryKey(&expiryKey)
		if val, err = encodeExpiryValue(expiryData); err != nil {
			return err
		}
		batch.Put(key, val)
	}
	return nil
}

func (e *entriesForPvtDataOfOldBlocks) addMissingDataEntriesToUpdateBatch(batch *leveldbhelper.UpdateBatch) error {
	var key, val []byte
	var err error

	for nsCollBlk, missingData := range e.missingDataEntries {
		key = encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk,
				isEligible: true,
			},
		)

		if missingData.None() {
			batch.Delete(key)
			continue
		}

		if val, err = encodeMissingDataValue(missingData); err != nil {
			return err
		}
		batch.Put(key, val)
	}
	return nil
}
