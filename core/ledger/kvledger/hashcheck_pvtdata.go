/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"bytes"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerstorage"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/utils"
)

// constructValidAndInvalidPvtData computes the valid pvt data and hash mismatch list
// from a received pvt data list of old blocks.
func constructValidAndInvalidPvtData(blocksPvtData []*ledger.BlockPvtData, blockStore *ledgerstorage.Store) (
	map[uint64][]*ledger.TxPvtData, []*ledger.PvtdataHashMismatch, error,
) {
	// for each block, for each transaction, retrieve the txEnvelope to
	// compare the hash of pvtRwSet in the block and the hash of the received
	// txPvtData. On a mismatch, add an entry to hashMismatch list.
	// On a match, add the pvtData to the validPvtData list
	validPvtData := make(map[uint64][]*ledger.TxPvtData)
	var invalidPvtData []*ledger.PvtdataHashMismatch

	for _, blockPvtData := range blocksPvtData {
		validData, invalidData, err := findValidAndInvalidBlockPvtData(blockPvtData, blockStore)
		if err != nil {
			return nil, nil, err
		}
		if len(validData) > 0 {
			validPvtData[blockPvtData.BlockNum] = validData
		}
		invalidPvtData = append(invalidPvtData, invalidData...)
	} // for each block's pvtData
	return validPvtData, invalidPvtData, nil
}

func findValidAndInvalidBlockPvtData(blockPvtData *ledger.BlockPvtData, blockStore *ledgerstorage.Store) (
	[]*ledger.TxPvtData, []*ledger.PvtdataHashMismatch, error,
) {
	var validPvtData []*ledger.TxPvtData
	var invalidPvtData []*ledger.PvtdataHashMismatch
	for _, txPvtData := range blockPvtData.WriteSets {
		// (1) retrieve the txrwset from the blockstore
		logger.Debugf("Retrieving rwset of blockNum:[%d], txNum:[%d]", blockPvtData.BlockNum, txPvtData.SeqInBlock)
		txRWSet, err := retrieveRwsetForTx(blockPvtData.BlockNum, txPvtData.SeqInBlock, blockStore)
		if err != nil {
			return nil, nil, err
		}

		// (2) validate passed pvtData against the pvtData hash in the tx rwset.
		logger.Debugf("Constructing valid and invalid pvtData using rwset of blockNum:[%d], txNum:[%d]",
			blockPvtData.BlockNum, txPvtData.SeqInBlock)
		validData, invalidData := findValidAndInvalidTxPvtData(txPvtData, txRWSet, blockPvtData.BlockNum)

		// (3) append validData to validPvtDataPvt list of this block and
		// invalidData to invalidPvtData list
		if validData != nil {
			validPvtData = append(validPvtData, validData)
		}
		invalidPvtData = append(invalidPvtData, invalidData...)
	} // for each tx's pvtData
	return validPvtData, invalidPvtData, nil
}

func retrieveRwsetForTx(blkNum uint64, txNum uint64, blockStore *ledgerstorage.Store) (*rwsetutil.TxRwSet, error) {
	// retrieve the txEnvelope from the block store so that the hash of
	// the pvtData can be retrieved for comparison
	txEnvelope, err := blockStore.RetrieveTxByBlockNumTranNum(blkNum, txNum)
	if err != nil {
		return nil, err
	}
	// retrieve pvtRWset hash from the txEnvelope
	responsePayload, err := utils.GetActionFromEnvelopeMsg(txEnvelope)
	if err != nil {
		return nil, err
	}
	txRWSet := &rwsetutil.TxRwSet{}
	if err := txRWSet.FromProtoBytes(responsePayload.Results); err != nil {
		return nil, err
	}
	return txRWSet, nil
}

func findValidAndInvalidTxPvtData(txPvtData *ledger.TxPvtData, txRWSet *rwsetutil.TxRwSet, blkNum uint64) (
	*ledger.TxPvtData, []*ledger.PvtdataHashMismatch,
) {
	var invalidPvtData []*ledger.PvtdataHashMismatch
	var toDeleteNsColl []*nsColl
	// Compare the hash of pvtData with the hash present in the rwset to
	// find valid and invalid pvt data
	for _, nsRwset := range txPvtData.WriteSet.NsPvtRwset {
		txNum := txPvtData.SeqInBlock
		invalidData, invalidNsColl := findInvalidNsPvtData(nsRwset, txRWSet, blkNum, txNum)
		invalidPvtData = append(invalidPvtData, invalidData...)
		toDeleteNsColl = append(toDeleteNsColl, invalidNsColl...)
	}
	for _, nsColl := range toDeleteNsColl {
		txPvtData.WriteSet.Remove(nsColl.ns, nsColl.coll)
	}
	if len(txPvtData.WriteSet.NsPvtRwset) == 0 {
		// denotes that all namespaces had
		// invalid pvt data
		return nil, invalidPvtData
	}
	return txPvtData, invalidPvtData
}

type nsColl struct {
	ns, coll string
}

func findInvalidNsPvtData(nsRwset *rwset.NsPvtReadWriteSet, txRWSet *rwsetutil.TxRwSet, blkNum, txNum uint64) (
	[]*ledger.PvtdataHashMismatch, []*nsColl,
) {
	var invalidPvtData []*ledger.PvtdataHashMismatch
	var invalidNsColl []*nsColl

	ns := nsRwset.Namespace
	for _, collPvtRwset := range nsRwset.CollectionPvtRwset {
		coll := collPvtRwset.CollectionName
		rwsetHash := txRWSet.GetPvtDataHash(ns, coll)
		if rwsetHash == nil {
			logger.Warningf("namespace: %s collection: %s was not accessed by txNum %d in BlkNum %d. "+
				"Unnecessary pvtdata has been passed", ns, coll, txNum, blkNum)
			invalidNsColl = append(invalidNsColl, &nsColl{ns, coll})
			continue
		}

		if !bytes.Equal(util.ComputeSHA256(collPvtRwset.Rwset), rwsetHash) {
			invalidPvtData = append(invalidPvtData, &ledger.PvtdataHashMismatch{
				BlockNum:     blkNum,
				TxNum:        txNum,
				Namespace:    ns,
				Collection:   coll,
				ExpectedHash: rwsetHash})
			invalidNsColl = append(invalidNsColl, &nsColl{ns, coll})
		}
	}
	return invalidPvtData, invalidNsColl
}
