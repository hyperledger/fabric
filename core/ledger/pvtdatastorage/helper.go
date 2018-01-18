/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"math"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

func prepareStoreEntries(blockNum uint64, pvtdata []*ledger.TxPvtData, btlPolicy pvtdatapolicy.BTLPolicy) ([]*dataEntry, []*expiryEntry, error) {
	dataEntries := prepareDataEntries(blockNum, pvtdata)
	expiryEntries, err := prepareExpiryEntries(blockNum, dataEntries, btlPolicy)
	if err != nil {
		return nil, nil, err
	}
	return dataEntries, expiryEntries, nil
}

func prepareDataEntries(blockNum uint64, pvtData []*ledger.TxPvtData) []*dataEntry {
	var dataEntries []*dataEntry
	for _, txPvtdata := range pvtData {
		for _, nsPvtdata := range txPvtdata.WriteSet.NsPvtRwset {
			for _, collPvtdata := range nsPvtdata.CollectionPvtRwset {
				txnum := txPvtdata.SeqInBlock
				ns := nsPvtdata.Namespace
				coll := collPvtdata.CollectionName
				dataKey := &dataKey{blockNum, txnum, ns, coll}
				dataEntries = append(dataEntries, &dataEntry{key: dataKey, value: collPvtdata})
			}
		}
	}
	return dataEntries
}

func prepareExpiryEntries(committingBlk uint64, dataEntries []*dataEntry, btlPolicy pvtdatapolicy.BTLPolicy) ([]*expiryEntry, error) {
	mapByExpiringBlk := make(map[uint64]*ExpiryData)
	for _, dataEntry := range dataEntries {
		expiringBlk, err := btlPolicy.GetExpiringBlock(dataEntry.key.ns, dataEntry.key.coll, dataEntry.key.blkNum)
		if err != nil {
			return nil, err
		}
		if neverExpires(expiringBlk) {
			continue
		}
		expiryData, ok := mapByExpiringBlk[expiringBlk]
		if !ok {
			expiryData = newExpiryData()
			mapByExpiringBlk[expiringBlk] = expiryData
		}
		expiryData.add(dataEntry.key.ns, dataEntry.key.coll, dataEntry.key.txNum)
	}
	var expiryEntries []*expiryEntry
	for expiryBlk, expiryData := range mapByExpiringBlk {
		expiryKey := &expiryKey{expiringBlk: expiryBlk, committingBlk: committingBlk}
		expiryEntries = append(expiryEntries, &expiryEntry{key: expiryKey, value: expiryData})
	}
	return expiryEntries, nil
}

func deriveDataKeys(expiryEntry *expiryEntry) []*dataKey {
	var dataKeys []*dataKey
	for ns, colls := range expiryEntry.value.Map {
		for coll, txNums := range colls.Map {
			for _, txNum := range txNums.List {
				dataKeys = append(dataKeys, &dataKey{expiryEntry.key.committingBlk, txNum, ns, coll})
			}
		}
	}
	return dataKeys
}

func passesFilter(dataKey *dataKey, filter ledger.PvtNsCollFilter) bool {
	return filter == nil || filter.Has(dataKey.ns, dataKey.coll)
}

func isExpired(dataKey *dataKey, btl pvtdatapolicy.BTLPolicy, latestBlkNum uint64) (bool, error) {
	expiringBlk, err := btl.GetExpiringBlock(dataKey.ns, dataKey.coll, dataKey.blkNum)
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
