/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

func v11Format(datakeyBytes []byte) bool {
	_, n := version.NewHeightFromBytes(datakeyBytes[1:])
	remainingBytes := datakeyBytes[n+1:]
	return len(remainingBytes) == 0
}

func v11DecodePK(key blkTranNumKey) (blockNum uint64, tranNum uint64) {
	height, _ := version.NewHeightFromBytes(key[1:])
	return height.BlockNum, height.TxNum
}

func v11DecodePvtRwSet(encodedBytes []byte) (*rwset.TxPvtReadWriteSet, error) {
	writeset := &rwset.TxPvtReadWriteSet{}
	return writeset, proto.Unmarshal(encodedBytes, writeset)
}

func v11RetrievePvtdata(itr *leveldbhelper.Iterator, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error) {
	var blkPvtData []*ledger.TxPvtData
	txPvtData, err := v11DecodeKV(itr.Key(), itr.Value(), filter)
	if err != nil {
		return nil, err
	}
	blkPvtData = append(blkPvtData, txPvtData)
	for itr.Next() {
		pvtDatum, err := v11DecodeKV(itr.Key(), itr.Value(), filter)
		if err != nil {
			return nil, err
		}
		blkPvtData = append(blkPvtData, pvtDatum)
	}
	return blkPvtData, nil
}

func v11DecodeKV(k, v []byte, filter ledger.PvtNsCollFilter) (*ledger.TxPvtData, error) {
	bNum, tNum := v11DecodePK(k)
	var pvtWSet *rwset.TxPvtReadWriteSet
	var err error
	if pvtWSet, err = v11DecodePvtRwSet(v); err != nil {
		return nil, err
	}
	logger.Debugf("Retrieved V11 private data write set for block [%d] tran [%d]", bNum, tNum)
	filteredWSet := v11TrimPvtWSet(pvtWSet, filter)
	return &ledger.TxPvtData{SeqInBlock: tNum, WriteSet: filteredWSet}, nil
}

func v11TrimPvtWSet(pvtWSet *rwset.TxPvtReadWriteSet, filter ledger.PvtNsCollFilter) *rwset.TxPvtReadWriteSet {
	if filter == nil {
		return pvtWSet
	}

	var filteredNsRwSet []*rwset.NsPvtReadWriteSet
	for _, ns := range pvtWSet.NsPvtRwset {
		var filteredCollRwSet []*rwset.CollectionPvtReadWriteSet
		for _, coll := range ns.CollectionPvtRwset {
			if filter.Has(ns.Namespace, coll.CollectionName) {
				filteredCollRwSet = append(filteredCollRwSet, coll)
			}
		}
		if filteredCollRwSet != nil {
			filteredNsRwSet = append(filteredNsRwSet,
				&rwset.NsPvtReadWriteSet{
					Namespace:          ns.Namespace,
					CollectionPvtRwset: filteredCollRwSet,
				},
			)
		}
	}
	var filteredTxPvtRwSet *rwset.TxPvtReadWriteSet
	if filteredNsRwSet != nil {
		filteredTxPvtRwSet = &rwset.TxPvtReadWriteSet{
			DataModel:  pvtWSet.GetDataModel(),
			NsPvtRwset: filteredNsRwSet,
		}
	}
	return filteredTxPvtRwSet
}
