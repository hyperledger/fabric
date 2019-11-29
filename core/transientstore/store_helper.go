/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transientstore

import (
	"bytes"
	"errors"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/ledger"
)

var (
	prwsetPrefix             = []byte("P")[0] // key prefix for storing private write set in transient store.
	purgeIndexByHeightPrefix = []byte("H")[0] // key prefix for storing index on private write set using received at block height.
	purgeIndexByTxidPrefix   = []byte("T")[0] // key prefix for storing index on private write set using txid
	compositeKeySep          = byte(0x00)
)

// createCompositeKeyForPvtRWSet creates a key for storing private write set
// in the transient store. The structure of the key is <prwsetPrefix>~txid~uuid~blockHeight.
func createCompositeKeyForPvtRWSet(txid string, uuid string, blockHeight uint64) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, prwsetPrefix)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, createCompositeKeyWithoutPrefixForTxid(txid, uuid, blockHeight)...)

	return compositeKey
}

// createCompositeKeyForPurgeIndexByTxid creates a key to index private write set based on
// txid such that purge based on txid can be achieved. The structure
// of the key is <purgeIndexByTxidPrefix>~txid~uuid~blockHeight.
func createCompositeKeyForPurgeIndexByTxid(txid string, uuid string, blockHeight uint64) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, purgeIndexByTxidPrefix)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, createCompositeKeyWithoutPrefixForTxid(txid, uuid, blockHeight)...)

	return compositeKey
}

// createCompositeKeyWithoutPrefixForTxid creates a composite key of structure txid~uuid~blockHeight.
func createCompositeKeyWithoutPrefixForTxid(txid string, uuid string, blockHeight uint64) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, []byte(txid)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(uuid)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(blockHeight)...)

	return compositeKey
}

// createCompositeKeyForPurgeIndexByHeight creates a key to index private write set based on
// received at block height such that purge based on block height can be achieved. The structure
// of the key is <purgeIndexByHeightPrefix>~blockHeight~txid~uuid.
func createCompositeKeyForPurgeIndexByHeight(blockHeight uint64, txid string, uuid string) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, purgeIndexByHeightPrefix)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(blockHeight)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(txid)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(uuid)...)

	return compositeKey
}

// splitCompositeKeyOfPvtRWSet splits the compositeKey (<prwsetPrefix>~txid~uuid~blockHeight)
// into uuid and blockHeight.
func splitCompositeKeyOfPvtRWSet(compositeKey []byte) (uuid string, blockHeight uint64, err error) {
	return splitCompositeKeyWithoutPrefixForTxid(compositeKey[2:])
}

// splitCompositeKeyOfPurgeIndexByTxid splits the compositeKey (<purgeIndexByTxidPrefix>~txid~uuid~blockHeight)
// into uuid and blockHeight.
func splitCompositeKeyOfPurgeIndexByTxid(compositeKey []byte) (uuid string, blockHeight uint64, err error) {
	return splitCompositeKeyWithoutPrefixForTxid(compositeKey[2:])
}

// splitCompositeKeyOfPurgeIndexByHeight splits the compositeKey (<purgeIndexByHeightPrefix>~blockHeight~txid~uuid)
// into txid, uuid and blockHeight.
func splitCompositeKeyOfPurgeIndexByHeight(compositeKey []byte) (txid string, uuid string, blockHeight uint64, err error) {
	var n int
	blockHeight, n, err = util.DecodeOrderPreservingVarUint64(compositeKey[2:])
	if err != nil {
		return
	}
	splits := bytes.Split(compositeKey[n+3:], []byte{compositeKeySep})
	txid = string(splits[0])
	uuid = string(splits[1])
	return
}

// splitCompositeKeyWithoutPrefixForTxid splits the composite key txid~uuid~blockHeight into
// uuid and blockHeight
func splitCompositeKeyWithoutPrefixForTxid(compositeKey []byte) (uuid string, blockHeight uint64, err error) {
	// skip txid as all functions which requires split of composite key already has it
	firstSepIndex := bytes.IndexByte(compositeKey, compositeKeySep)
	secondSepIndex := firstSepIndex + bytes.IndexByte(compositeKey[firstSepIndex+1:], compositeKeySep) + 1
	uuid = string(compositeKey[firstSepIndex+1 : secondSepIndex])
	blockHeight, _, err = util.DecodeOrderPreservingVarUint64(compositeKey[secondSepIndex+1:])
	return
}

// createTxidRangeStartKey returns a startKey to do a range query on transient store using txid
func createTxidRangeStartKey(txid string) []byte {
	var startKey []byte
	startKey = append(startKey, prwsetPrefix)
	startKey = append(startKey, compositeKeySep)
	startKey = append(startKey, []byte(txid)...)
	startKey = append(startKey, compositeKeySep)
	return startKey
}

// createTxidRangeEndKey returns a endKey to do a range query on transient store using txid
func createTxidRangeEndKey(txid string) []byte {
	var endKey []byte
	endKey = append(endKey, prwsetPrefix)
	endKey = append(endKey, compositeKeySep)
	endKey = append(endKey, []byte(txid)...)
	// As txid is a fixed length string (i.e., 128 bits long UUID), 0xff can be used as a stopper.
	// Otherwise a super-string of a given txid would also fall under the end key of range query.
	endKey = append(endKey, byte(0xff))
	return endKey
}

// createPurgeIndexByHeightRangeStartKey returns a startKey to do a range query on index stored in transient store
// using blockHeight
func createPurgeIndexByHeightRangeStartKey(blockHeight uint64) []byte {
	var startKey []byte
	startKey = append(startKey, purgeIndexByHeightPrefix)
	startKey = append(startKey, compositeKeySep)
	startKey = append(startKey, util.EncodeOrderPreservingVarUint64(blockHeight)...)
	startKey = append(startKey, compositeKeySep)
	return startKey
}

// createPurgeIndexByHeightRangeEndKey returns a endKey to do a range query on index stored in transient store
// using blockHeight
func createPurgeIndexByHeightRangeEndKey(blockHeight uint64) []byte {
	var endKey []byte
	endKey = append(endKey, purgeIndexByHeightPrefix)
	endKey = append(endKey, compositeKeySep)
	endKey = append(endKey, util.EncodeOrderPreservingVarUint64(blockHeight)...)
	endKey = append(endKey, byte(0xff))
	return endKey
}

// createPurgeIndexByTxidRangeStartKey returns a startKey to do a range query on index stored in transient store
// using txid
func createPurgeIndexByTxidRangeStartKey(txid string) []byte {
	var startKey []byte
	startKey = append(startKey, purgeIndexByTxidPrefix)
	startKey = append(startKey, compositeKeySep)
	startKey = append(startKey, []byte(txid)...)
	startKey = append(startKey, compositeKeySep)
	return startKey
}

// createPurgeIndexByTxidRangeEndKey returns a endKey to do a range query on index stored in transient store
// using txid
func createPurgeIndexByTxidRangeEndKey(txid string) []byte {
	var endKey []byte
	endKey = append(endKey, purgeIndexByTxidPrefix)
	endKey = append(endKey, compositeKeySep)
	endKey = append(endKey, []byte(txid)...)
	// As txid is a fixed length string (i.e., 128 bits long UUID), 0xff can be used as a stopper.
	// Otherwise a super-string of a given txid would also fall under the end key of range query.
	endKey = append(endKey, byte(0xff))
	return endKey
}

// trimPvtWSet returns a `TxPvtReadWriteSet` that retains only list of 'ns/collections' supplied in the filter
// A nil filter does not filter any results and returns the original `pvtWSet` as is
func trimPvtWSet(pvtWSet *rwset.TxPvtReadWriteSet, filter ledger.PvtNsCollFilter) *rwset.TxPvtReadWriteSet {
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

func trimPvtCollectionConfigs(configs map[string]*peer.CollectionConfigPackage,
	filter ledger.PvtNsCollFilter) (map[string]*peer.CollectionConfigPackage, error) {
	if filter == nil {
		return configs, nil
	}
	result := make(map[string]*peer.CollectionConfigPackage)

	for ns, pkg := range configs {
		result[ns] = &peer.CollectionConfigPackage{}
		for _, colConf := range pkg.GetConfig() {
			switch cconf := colConf.Payload.(type) {
			case *peer.CollectionConfig_StaticCollectionConfig:
				if filter.Has(ns, cconf.StaticCollectionConfig.Name) {
					result[ns].Config = append(result[ns].Config, colConf)
				}
			default:
				return nil, errors.New("unexpected collection type")
			}
		}
	}
	return result, nil
}
