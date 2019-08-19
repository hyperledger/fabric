/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historyleveldb

import (
	"bytes"

	"github.com/hyperledger/fabric/common/ledger/util"
)

var (
	// compositeKeySep is a nil byte used as a separator between different components of a composite key
	compositeKeySep = []byte{0x00}
	savePointKey    = []byte{0x00}
	emptyValue      = []byte{}
)

// constructCompositeHistoryKey builds the History Key of namespace~len(key)~key~blocknum~trannum
// using an order preserving encoding so that history query results are ordered by height
// Note: this key format is different than the format in pre-v2.0 releases and requires
//       a historydb rebuild when upgrading an older version to v2.0.
func constructCompositeHistoryKey(ns string, key string, blocknum uint64, trannum uint64) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, []byte(ns)...)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(uint64(len(key)))...)
	compositeKey = append(compositeKey, []byte(key)...)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(blocknum)...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(trannum)...)

	return compositeKey
}

// constructPartialCompositeHistoryKey builds a partial History Key namespace~len(key)~key~
// for use in history key range queries
func constructPartialCompositeHistoryKey(ns string, key string, endkey bool) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, []byte(ns)...)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(uint64(len(key)))...)
	compositeKey = append(compositeKey, []byte(key)...)
	compositeKey = append(compositeKey, compositeKeySep...)
	if endkey {
		compositeKey = append(compositeKey, []byte{0xff}...)
	}
	return compositeKey
}

//SplitCompositeHistoryKey splits the key bytes using a separator
func splitCompositeHistoryKey(bytesToSplit []byte, separator []byte) ([]byte, []byte) {
	split := bytes.SplitN(bytesToSplit, separator, 2)
	return split[0], split[1]
}
