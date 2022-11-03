/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"math"

	"github.com/bits-and-blooms/bitset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/core/ledger/util"
)

func prepareStoreEntries(blockNum uint64,
	pvtData []*ledger.TxPvtData,
	btlPolicy pvtdatapolicy.BTLPolicy,
	missingPvtData ledger.TxMissingPvtData,
	purgeMarkers []*PurgeMarker,
) (*storeEntries, error) {
	dataEntries := prepareDataEntries(blockNum, pvtData)

	hashedIndexEntries, err := prepareHashedIndexEntries(dataEntries)
	if err != nil {
		return nil, err
	}
	elgMissingDataEntries, inelgMissingDataEntries := prepareMissingDataEntries(blockNum, missingPvtData)

	expiryEntries, err := prepareExpiryEntries(blockNum, dataEntries, elgMissingDataEntries, inelgMissingDataEntries, btlPolicy)
	if err != nil {
		return nil, err
	}

	purgeMarkerEntries, purgeMarkerCollEntries := preparePurgerMarkerEntries(blockNum, purgeMarkers)

	return &storeEntries{
		dataEntries:             dataEntries,
		hashedIndexEntries:      hashedIndexEntries,
		purgeMarkerEntries:      purgeMarkerEntries,
		purgeMarkerCollEntries:  purgeMarkerCollEntries,
		expiryEntries:           expiryEntries,
		elgMissingDataEntries:   elgMissingDataEntries,
		inelgMissingDataEntries: inelgMissingDataEntries,
	}, nil
}

func prepareDataEntries(blockNum uint64, pvtData []*ledger.TxPvtData) []*dataEntry {
	var dataEntries []*dataEntry
	for _, txPvtdata := range pvtData {
		for _, nsPvtdata := range txPvtdata.WriteSet.NsPvtRwset {
			for _, collPvtdata := range nsPvtdata.CollectionPvtRwset {
				txnum := txPvtdata.SeqInBlock
				ns := nsPvtdata.Namespace
				coll := collPvtdata.CollectionName
				dataKey := &dataKey{nsCollBlk{ns, coll, blockNum}, txnum}
				dataEntries = append(dataEntries, &dataEntry{key: dataKey, value: collPvtdata})
			}
		}
	}
	return dataEntries
}

func prepareMissingDataEntries(
	committingBlk uint64,
	missingPvtData ledger.TxMissingPvtData,
) (map[missingDataKey]*bitset.BitSet, map[missingDataKey]*bitset.BitSet) {
	elgMissingDataEntries := make(map[missingDataKey]*bitset.BitSet)
	inelgMissingDataEntries := make(map[missingDataKey]*bitset.BitSet)

	for txNum, missingData := range missingPvtData {
		for _, nsColl := range missingData {
			key := missingDataKey{
				nsCollBlk{
					ns:     nsColl.Namespace,
					coll:   nsColl.Collection,
					blkNum: committingBlk,
				},
			}

			switch nsColl.IsEligible {
			case true:
				if _, ok := elgMissingDataEntries[key]; !ok {
					elgMissingDataEntries[key] = &bitset.BitSet{}
				}
				elgMissingDataEntries[key].Set(uint(txNum))
			default:
				if _, ok := inelgMissingDataEntries[key]; !ok {
					inelgMissingDataEntries[key] = &bitset.BitSet{}
				}
				inelgMissingDataEntries[key].Set(uint(txNum))
			}
		}
	}

	return elgMissingDataEntries, inelgMissingDataEntries
}

// prepareExpiryEntries returns expiry entries for both private data which is present in the committingBlk
// and missing private.
func prepareExpiryEntries(committingBlk uint64, dataEntries []*dataEntry, elgMissingDataEntries, inelgMissingDataEntries map[missingDataKey]*bitset.BitSet,
	btlPolicy pvtdatapolicy.BTLPolicy) ([]*expiryEntry, error) {
	var expiryEntries []*expiryEntry
	mapByExpiringBlk := make(map[uint64]*ExpiryData)

	for _, dataEntry := range dataEntries {
		if err := prepareExpiryEntriesForPresentData(mapByExpiringBlk, dataEntry.key, btlPolicy); err != nil {
			return nil, err
		}
	}

	for missingDataKey := range elgMissingDataEntries {
		if err := prepareExpiryEntriesForMissingData(mapByExpiringBlk, &missingDataKey, btlPolicy); err != nil {
			return nil, err
		}
	}

	for missingDataKey := range inelgMissingDataEntries {
		if err := prepareExpiryEntriesForMissingData(mapByExpiringBlk, &missingDataKey, btlPolicy); err != nil {
			return nil, err
		}
	}

	for expiryBlk, expiryData := range mapByExpiringBlk {
		expiryKey := &expiryKey{expiringBlk: expiryBlk, committingBlk: committingBlk}
		expiryEntries = append(expiryEntries, &expiryEntry{key: expiryKey, value: expiryData})
	}

	return expiryEntries, nil
}

// prepareExpiryDataForPresentData creates expiryData for non-missing pvt data
func prepareExpiryEntriesForPresentData(mapByExpiringBlk map[uint64]*ExpiryData, dataKey *dataKey, btlPolicy pvtdatapolicy.BTLPolicy) error {
	expiringBlk, err := btlPolicy.GetExpiringBlock(dataKey.ns, dataKey.coll, dataKey.blkNum)
	if err != nil {
		return err
	}
	if neverExpires(expiringBlk) {
		return nil
	}

	expiryData := getOrCreateExpiryData(mapByExpiringBlk, expiringBlk)

	expiryData.addPresentData(dataKey.ns, dataKey.coll, dataKey.txNum)
	return nil
}

// prepareExpiryDataForMissingData creates expiryData for missing pvt data
func prepareExpiryEntriesForMissingData(mapByExpiringBlk map[uint64]*ExpiryData, missingKey *missingDataKey, btlPolicy pvtdatapolicy.BTLPolicy) error {
	expiringBlk, err := btlPolicy.GetExpiringBlock(missingKey.ns, missingKey.coll, missingKey.blkNum)
	if err != nil {
		return err
	}
	if neverExpires(expiringBlk) {
		return nil
	}

	expiryData := getOrCreateExpiryData(mapByExpiringBlk, expiringBlk)

	expiryData.addMissingData(missingKey.ns, missingKey.coll)
	return nil
}

func prepareHashedIndexEntries(dataEntires []*dataEntry) ([]*hashedIndexEntry, error) {
	hashedIndexEntries := []*hashedIndexEntry{}
	for _, d := range dataEntires {
		collPvtWS, err := rwsetutil.CollPvtRwSetFromProtoMsg(d.value)
		if err != nil {
			return nil, err
		}
		for _, w := range collPvtWS.KvRwSet.Writes {
			hashedIndexEntries = append(hashedIndexEntries,
				&hashedIndexEntry{
					key: &hashedIndexKey{
						ns:         d.key.ns,
						coll:       d.key.coll,
						blkNum:     d.key.blkNum,
						txNum:      d.key.txNum,
						pvtkeyHash: util.ComputeStringHash(w.Key),
					},
					value: w.Key,
				})
		}
	}
	return hashedIndexEntries, nil
}

func preparePurgerMarkerEntries(blkNum uint64, purgeMarkers []*PurgeMarker) ([]*purgeMarkerEntry, []*purgeMarkerCollEntry) {
	purgeMarkersEntries := []*purgeMarkerEntry{}
	purgeMarkersCollEntries := []*purgeMarkerCollEntry{}
	nsCollVisitedMap := map[nsColl]*version.Height{}

	for _, m := range purgeMarkers {
		purgeMarkersEntries = append(purgeMarkersEntries,
			&purgeMarkerEntry{
				key: &purgeMarkerKey{
					ns:         m.Ns,
					coll:       m.Coll,
					pvtkeyHash: m.PvtkeyHash,
				},
				value: &purgeMarkerVal{
					blkNum: blkNum,
					txNum:  m.TxNum,
				},
			},
		)

		nsColl := nsColl{ns: m.Ns, coll: m.Coll}
		version := version.NewHeight(blkNum, m.TxNum)
		visitedVersion, ok := nsCollVisitedMap[nsColl]
		if ok && visitedVersion.Compare(version) > 0 {
			// a key in the same collection with higher version already caused adding of collection level purge mearker entry
			continue
		}
		purgeMarkersCollEntries = append(purgeMarkersCollEntries,
			&purgeMarkerCollEntry{
				key: &purgeMarkerCollKey{
					ns:   m.Ns,
					coll: m.Coll,
				},
				value: &purgeMarkerVal{
					blkNum: blkNum,
					txNum:  m.TxNum,
				},
			},
		)
		nsCollVisitedMap[nsColl] = version
	}
	return purgeMarkersEntries, purgeMarkersCollEntries
}

func getOrCreateExpiryData(mapByExpiringBlk map[uint64]*ExpiryData, expiringBlk uint64) *ExpiryData {
	expiryData, ok := mapByExpiringBlk[expiringBlk]
	if !ok {
		expiryData = newExpiryData()
		mapByExpiringBlk[expiringBlk] = expiryData
	}
	return expiryData
}

func deriveKeys(expiryEntry *expiryEntry) ([]*dataKey, []*missingDataKey, []*bootKVHashesKey) {
	var dataKeys []*dataKey
	var missingDataKeys []*missingDataKey
	var bootKVHashesKeys []*bootKVHashesKey

	for ns, colls := range expiryEntry.value.Map {
		for coll, txNums := range colls.PresentData {
			for _, txNum := range txNums.List {
				dataKeys = append(dataKeys,
					&dataKey{
						nsCollBlk: nsCollBlk{
							ns:     ns,
							coll:   coll,
							blkNum: expiryEntry.key.committingBlk,
						},
						txNum: txNum,
					})
			}
		}

		for coll := range colls.MissingData {
			missingDataKeys = append(missingDataKeys,
				&missingDataKey{
					nsCollBlk: nsCollBlk{
						ns:     ns,
						coll:   coll,
						blkNum: expiryEntry.key.committingBlk,
					},
				})
		}

		for coll, txNums := range colls.BootKVHashes {
			for _, txNum := range txNums.List {
				bootKVHashesKeys = append(bootKVHashesKeys,
					&bootKVHashesKey{
						blkNum: expiryEntry.key.committingBlk,
						txNum:  txNum,
						ns:     ns,
						coll:   coll,
					},
				)
			}
		}
	}

	return dataKeys, missingDataKeys, bootKVHashesKeys
}

func passesFilter(dataKey *dataKey, filter ledger.PvtNsCollFilter) bool {
	return filter == nil || filter.Has(dataKey.ns, dataKey.coll)
}

func isExpired(key nsCollBlk, btl pvtdatapolicy.BTLPolicy, latestBlkNum uint64) (bool, error) {
	expiringBlk, err := btl.GetExpiringBlock(key.ns, key.coll, key.blkNum)
	if err != nil {
		return false, err
	}

	return latestBlkNum >= expiringBlk, nil
}

func neverExpires(expiringBlkNum uint64) bool {
	return expiringBlkNum == math.MaxUint64
}

type txPvtdataAssembler struct {
	blockNum, txNum uint64
	txWset          *rwset.TxPvtReadWriteSet
	currentNsWSet   *rwset.NsPvtReadWriteSet
	firstCall       bool
}

func newTxPvtdataAssembler(blockNum, txNum uint64) *txPvtdataAssembler {
	return &txPvtdataAssembler{blockNum, txNum, &rwset.TxPvtReadWriteSet{}, nil, true}
}

func (a *txPvtdataAssembler) add(ns string, collPvtWset *rwset.CollectionPvtReadWriteSet) {
	// start a NsWset
	if a.firstCall {
		a.currentNsWSet = &rwset.NsPvtReadWriteSet{Namespace: ns}
		a.firstCall = false
	}

	// if a new ns started, add the existing NsWset to TxWset and start a new one
	if a.currentNsWSet.Namespace != ns {
		a.txWset.NsPvtRwset = append(a.txWset.NsPvtRwset, a.currentNsWSet)
		a.currentNsWSet = &rwset.NsPvtReadWriteSet{Namespace: ns}
	}
	// add the collWset to the current NsWset
	a.currentNsWSet.CollectionPvtRwset = append(a.currentNsWSet.CollectionPvtRwset, collPvtWset)
}

func (a *txPvtdataAssembler) done() {
	if a.currentNsWSet != nil {
		a.txWset.NsPvtRwset = append(a.txWset.NsPvtRwset, a.currentNsWSet)
	}
	a.currentNsWSet = nil
}

func (a *txPvtdataAssembler) getTxPvtdata() *ledger.TxPvtData {
	a.done()
	return &ledger.TxPvtData{SeqInBlock: a.txNum, WriteSet: a.txWset}
}
