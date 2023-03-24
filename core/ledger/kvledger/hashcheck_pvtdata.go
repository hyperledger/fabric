/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"bytes"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/pvtdatastorage"
	"github.com/hyperledger/fabric/protoutil"
)

// extractValidPvtData computes the valid and invalid pvt data
// from a received pvt data list of old blocks. The reconciled data elements that belong
// to a block which is not available in the blockstore (i.e., the block is prior to or equal
// to the lastBlockInBootSnapshot), the boot KV hashes in the private data store are used for
// verifying the hashes -- in this case, the write-set of a collection is trimmed in order
// to remove the key-values that were not present in the snapshot (most likely because, they
// were over-written in a later version). Also, the keys that have been purged explicitly by a
// user transaction are removed from the reconciled data
func extractValidPvtData(
	reconciledPvtdata []*ledger.ReconciledPvtdata,
	blockStore *blkstorage.BlockStore,
	pvtdataStore *pvtdatastorage.Store,
	lastBlockInBootSnapshot uint64) (map[uint64][]*ledger.TxPvtData, error) {
	validPvtData := map[uint64][]*ledger.TxPvtData{}
	for _, blkPvtdata := range reconciledPvtdata {
		validData, err := extractValidPvtDataForBlock(
			blkPvtdata, blockStore, pvtdataStore, lastBlockInBootSnapshot,
		)
		if err != nil {
			return nil, err
		}
		if len(validData) > 0 {
			validPvtData[blkPvtdata.BlockNum] = validData
		}
	}
	return validPvtData, nil
}

// extractValidPvtDataForBlock constructs valid and invalid pvt data for a block
func extractValidPvtDataForBlock(
	blkPvtdata *ledger.ReconciledPvtdata,
	blockStore *blkstorage.BlockStore,
	pvtdataStore *pvtdatastorage.Store,
	lastBlockInBootSnapshot uint64) ([]*ledger.TxPvtData, error) {
	validPvtData := []*ledger.TxPvtData{}
	blkNum := blkPvtdata.BlockNum

	for txNum, txPvtdata := range blkPvtdata.WriteSets { // Tx loop
		finalNsWrites := []*rwset.NsPvtReadWriteSet{}

		for _, nsPvtdata := range txPvtdata.WriteSet.GetNsPvtRwset() { // Ns Loop
			ns := nsPvtdata.Namespace
			finalCollsWrites := []*rwset.CollectionPvtReadWriteSet{}

			for _, collPvtdata := range nsPvtdata.GetCollectionPvtRwset() { // coll loop
				coll := collPvtdata.CollectionName
				var kvHashes map[string][]byte
				var pvtWSHashFromBlock []byte
				var err error

				if blkNum > lastBlockInBootSnapshot {
					txRWSet, err := retrieveRwsetForTx(blkNum, txNum, blockStore)
					if err != nil {
						return nil, err
					}
					kvHashes = retrieveCollKVHashes(txRWSet, ns, coll)
					pvtWSHashFromBlock = txRWSet.GetPvtDataHash(ns, coll)
				} else {
					kvHashes, err = pvtdataStore.FetchBootKVHashes(blkNum, txNum, ns, coll)
					if err != nil {
						return nil, err
					}
				}

				if len(kvHashes) == 0 {
					logSkippedCollection(ns, coll, blkNum, txNum,
						"Pvtdata not expected. There are no corresponding hashes present on this peer")
					continue
				}

				finalCollWs, err := validateAndRemovePurgedKeys(
					pvtdataStore, collPvtdata, kvHashes, pvtWSHashFromBlock, ns, blkNum, txNum)
				if err != nil {
					return nil, err
				}
				if finalCollWs != nil {
					finalCollsWrites = append(finalCollsWrites, finalCollWs)
				}
			} // end coll loop
			if len(finalCollsWrites) == 0 {
				continue
			}
			finalNsWrites = append(finalNsWrites,
				&rwset.NsPvtReadWriteSet{
					Namespace:          ns,
					CollectionPvtRwset: finalCollsWrites,
				},
			)
		} // end Ns loop
		if len(finalNsWrites) == 0 {
			continue
		}
		validPvtData = append(validPvtData,
			&ledger.TxPvtData{
				SeqInBlock: txNum,
				WriteSet: &rwset.TxPvtReadWriteSet{
					DataModel:  txPvtdata.WriteSet.DataModel,
					NsPvtRwset: finalNsWrites,
				},
			},
		)
	} // end Tx loop
	return validPvtData, nil
}

// validateAndRemovePurgedKeys operates on the pvt data for a collection. This function
// validates the pvt data by matching againsts the expected hashes. In addition, it removes
// the keys that have already been purged on this peer, if any. This function returns nil if
// the collection data should not be committed and returns a rejection reason instead.
func validateAndRemovePurgedKeys(
	pvtdataStore *pvtdatastorage.Store,
	collPvtProto *rwset.CollectionPvtReadWriteSet,
	kvHashes map[string][]byte,
	collWSHashFromBlock []byte,
	ns string, blkNum, txNum uint64) (*rwset.CollectionPvtReadWriteSet, error) {
	coll := collPvtProto.CollectionName
	trimmedKVHashes, err := pvtdataStore.RemoveAppInitiatedPurgesUsingReconMarker(
		kvHashes, ns, coll, blkNum, txNum)
	if err != nil {
		return nil, err
	}

	if len(trimmedKVHashes) == 0 {
		// All keys in the collection are purged, drop this collection and add an empty (with no keys) collection data
		// Note that we add empty collection data for two reasons
		// 1) This would cause removal of the missing data entry, when we commit this reconciled data
		// 2) This would return empty data when some other peer queries for this collection data (this is consistent with how we
		// maintain an empty collection when all the keys in a collection are purged)
		return &rwset.CollectionPvtReadWriteSet{CollectionName: coll}, nil
	}

	collPvtWS, err := rwsetutil.CollPvtRwSetFromProtoMsg(collPvtProto)
	if err != nil {
		logSkippedCollection(ns, coll, blkNum, txNum, "Error during unmarshalling of collection proto")
		return nil, nil
	}
	pvtKVs := collPvtWS.KvRwSet.Writes

	// No key purge is observed by this peer as well as by sending peer (as yet!) so checking hash of collection writeset is enough,
	// provided block is available
	if collWSHashFromBlock != nil &&
		len(trimmedKVHashes) == len(kvHashes) &&
		len(pvtKVs) == len(kvHashes) {

		if !bytes.Equal(util.ComputeSHA256(collPvtProto.Rwset), collWSHashFromBlock) {
			logSkippedCollection(ns, coll, blkNum, txNum, "Hash mismatched")
			return nil, nil
		}
		return collPvtProto, nil
	}

	if len(pvtKVs) < len(trimmedKVHashes) {
		// this could happen when the sending peer has observed more purge transactions than this peer (as yet!)
		// we skip this collection and it will be reconcilied on a subsequent iteration, after this peer also observe
		// the remaining purges
		logSkippedCollection(ns, coll, blkNum, txNum, "Only subset of expected keys present")
		return nil, nil
	}

	// If we reach here, this means that either the same numbers of keys (including zero) have been purged at both the peers,
	// or this peer has seen more purges. This means that we can retrieve desired unpurged data from the
	// reconciled collection data

	finalPvtKVs := []*kvrwset.KVWrite{}
	for _, kv := range pvtKVs {
		keyHash := string(util.ComputeSHA256([]byte(kv.Key)))
		expectedValueHash, ok := trimmedKVHashes[keyHash]
		if !ok {
			continue
		}
		if !bytes.Equal(expectedValueHash, util.ComputeSHA256(kv.Value)) {
			logSkippedCollection(ns, coll, blkNum, txNum, "Hash mismatched")
			return nil, nil
		}
		finalPvtKVs = append(finalPvtKVs, kv)
		delete(trimmedKVHashes, keyHash)
	}

	if len(trimmedKVHashes) > 0 {
		// some expected keys were missing in the pvtKVs
		logSkippedCollection(ns, coll, blkNum, txNum, "Only subset of expected keys present")
		return nil, nil
	}

	r := &rwsetutil.CollPvtRwSet{
		CollectionName: coll,
		KvRwSet: &kvrwset.KVRWSet{
			Writes: finalPvtKVs,
		},
	}
	p, err := r.ToProtoMsg()
	if err != nil {
		return nil, err
	}
	return p, nil
}

func retrieveCollKVHashes(txRWSet *rwsetutil.TxRwSet, ns, coll string) map[string][]byte {
	for _, nsRWSet := range txRWSet.NsRwSets {
		if nsRWSet.NameSpace != ns {
			continue
		}
		for _, collHahsedRWSet := range nsRWSet.CollHashedRwSets {
			if collHahsedRWSet.CollectionName != coll {
				continue
			}

			kvHashses := map[string][]byte{}
			for _, h := range collHahsedRWSet.HashedRwSet.HashedWrites {
				kvHashses[string(h.KeyHash)] = h.ValueHash
			}
			return kvHashses
		}
	}
	return nil
}

func retrieveRwsetForTx(blkNum uint64, txNum uint64, blockStore *blkstorage.BlockStore) (*rwsetutil.TxRwSet, error) {
	// retrieve the txEnvelope from the block store so that the hash of
	// the pvtData can be retrieved for comparison
	txEnvelope, err := blockStore.RetrieveTxByBlockNumTranNum(blkNum, txNum)
	if err != nil {
		return nil, err
	}
	// retrieve pvtRWset hash from the txEnvelope
	responsePayload, err := protoutil.GetActionFromEnvelopeMsg(txEnvelope)
	if err != nil {
		return nil, err
	}
	txRWSet := &rwsetutil.TxRwSet{}
	if err := txRWSet.FromProtoBytes(responsePayload.Results); err != nil {
		return nil, err
	}
	return txRWSet, nil
}

func logSkippedCollection(ns, coll string, blkNum, txNum uint64, reason string) {
	logger.Warnw("Failed to reconcile pvtdata for",
		"chaincode", ns, "collection", coll, "block num", blkNum, "tx num", txNum, "reason", reason)
}
