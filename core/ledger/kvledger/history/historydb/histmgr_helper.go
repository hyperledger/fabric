/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historydb

import (
	"bytes"

	"github.com/hyperledger/fabric/common/ledger/util"
)

// CompositeKeySep is a nil byte used as a separator between different components of a composite key
var CompositeKeySep = []byte{0x00}

// ConstructCompositeHistoryKey builds the History Key of namespace~len(key)~key~blocknum~trannum
// using an order preserving encoding so that history query results are ordered by height
// Note: this key format is different than the format in pre-v2.0 releases and requires
//       a historydb rebuild when upgrading an older version to v2.0.
func ConstructCompositeHistoryKey(ns string, key string, blocknum uint64, trannum uint64) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, []byte(ns)...)
	compositeKey = append(compositeKey, CompositeKeySep...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(uint64(len(key)))...)
	compositeKey = append(compositeKey, []byte(key)...)
	compositeKey = append(compositeKey, CompositeKeySep...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(blocknum)...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(trannum)...)

	return compositeKey
}

// ConstructPartialCompositeHistoryKey builds a partial History Key namespace~len(key)~key~
// for use in history key range queries
func ConstructPartialCompositeHistoryKey(ns string, key string, endkey bool) []byte {
	var compositeKey []byte
	compositeKey = append(compositeKey, []byte(ns)...)
	compositeKey = append(compositeKey, CompositeKeySep...)
	compositeKey = append(compositeKey, util.EncodeOrderPreservingVarUint64(uint64(len(key)))...)
	compositeKey = append(compositeKey, []byte(key)...)
	compositeKey = append(compositeKey, CompositeKeySep...)
	if endkey {
		compositeKey = append(compositeKey, []byte{0xff}...)
	}
	return compositeKey
}

//SplitCompositeHistoryKey splits the key bytes using a separator
func SplitCompositeHistoryKey(bytesToSplit []byte, separator []byte) ([]byte, []byte) {
	split := bytes.SplitN(bytesToSplit, separator, 2)
	return split[0], split[1]
}
