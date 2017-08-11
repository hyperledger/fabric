/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package transientstore

import (
	"bytes"
	"path/filepath"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/hyperledger/fabric/core/config"
)

var (
	prwsetPrefix     = []byte("P")[0] // key prefix for storing private read-write set in transient store.
	purgeIndexPrefix = []byte("I")[0] // key prefix for storing index on private read-write set using endorsement block height.
	compositeKeySep  = byte(0x00)
)

// createCompositeKeyForPvtRWSet creates a key for storing private read-write set
// in the transient store. The structure of the key is <prwsetPrefix>~txid~endorserid~endorsementBlkHt.
func createCompositeKeyForPvtRWSet(txid string, endorserid string, endorsementBlkHt uint64) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, prwsetPrefix)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(txid)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(endorserid)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(endorsementBlkHt)...)

	return compositeKey
}

// createCompositeKeyForPurgeIndex creates a key to index private read-write set based on
// endorsement block height such that purge based on block height can be achieved. The structure
// of the key is <purgeIndexPrefix>~endorsementBlkHt~txid~endorserid.
func createCompositeKeyForPurgeIndex(endorsementBlkHt uint64, txid string, endorserid string) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, purgeIndexPrefix)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(endorsementBlkHt)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(txid)...)
	compositeKey = append(compositeKey, compositeKeySep)
	compositeKey = append(compositeKey, []byte(endorserid)...)

	return compositeKey
}

// splitCompositeKeyOfPvtRWSet splits the compositeKey (<prwsetPrefix>~txid~endorserid~endorsementBlkHt) into endorserId and endorsementBlkHt.
func splitCompositeKeyOfPvtRWSet(compositeKey []byte) (endorserid string, endorsementBlkHt uint64) {
	compositeKey = compositeKey[2:]
	firstSepIndex := bytes.IndexByte(compositeKey, compositeKeySep)
	secondSepIndex := firstSepIndex + bytes.IndexByte(compositeKey[firstSepIndex+1:], compositeKeySep) + 1
	endorserid = string(compositeKey[firstSepIndex+1 : secondSepIndex])
	endorsementBlkHt, _ = util.DecodeOrderPreservingVarUint64(compositeKey[secondSepIndex+1:])
	return endorserid, endorsementBlkHt
}

// splitCompositeKeyOfPurgeIndex splits the compositeKey (<purgeIndexPrefix>~endorsementBlkHt~txid~endorserid) into txid, endorserid and endorsementBlkHt.
func splitCompositeKeyOfPurgeIndex(compositeKey []byte) (txid string, endorserid string, endorsementBlkHt uint64) {
	var n int
	endorsementBlkHt, n = util.DecodeOrderPreservingVarUint64(compositeKey[2:])
	splits := bytes.Split(compositeKey[n+3:], []byte{compositeKeySep})
	txid = string(splits[0])
	endorserid = string(splits[1])
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
	endKey = append(endKey, byte(0xff))
	return endKey
}

// createEndorsementBlkHtRangeStartKey returns a startKey to do a range query on index stored in transient store
// using endorsementBlkHt
func createEndorsementBlkHtRangeStartKey(endorsementBlkHt uint64) []byte {
	var startKey []byte
	startKey = append(startKey, purgeIndexPrefix)
	startKey = append(startKey, compositeKeySep)
	startKey = append(startKey, util.EncodeOrderPreservingVarUint64(endorsementBlkHt)...)
	startKey = append(startKey, compositeKeySep)
	return startKey
}

// createEndorsementBlkHtRangeStartKey returns a endKey to do a range query on index stored in transient store
// using endorsementBlkHt
func createEndorsementBlkHtRangeEndKey(endorsementBlkHt uint64) []byte {
	var endKey []byte
	endKey = append(endKey, purgeIndexPrefix)
	endKey = append(endKey, compositeKeySep)
	endKey = append(endKey, util.EncodeOrderPreservingVarUint64(endorsementBlkHt)...)
	endKey = append(endKey, byte(0xff))
	return endKey
}

// GetTransientStorePath returns the filesystem path for temporarily storing the private rwset
func GetTransientStorePath() string {
	sysPath := config.GetPath("peer.fileSystemPath")
	return filepath.Join(sysPath, "transientStore")
}
