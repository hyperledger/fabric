/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package history

import (
	"bytes"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/pkg/errors"
)

type (
	dataKey   []byte
	rangeScan struct {
		startKey, endKey []byte
	}
)

var (
	compositeKeySep = []byte{0x00} // used as a separator between different components of dataKey
	savePointKey    = []byte{'s'}  // a single key in db for persisting savepoint
	emptyValue      = []byte{}     // used to store as value for keys where only key needs to be stored (e.g., dataKeys)
)

// constructDataKey builds the key of the format namespace~len(key)~key~blocknum~trannum
// using an order preserving encoding so that history query results are ordered by height
// Note: this key format is different than the format in pre-v2.0 releases and requires
//
//	a historydb rebuild when upgrading an older version to v2.0.
func constructDataKey(ns string, key string, blocknum uint64, trannum uint64) dataKey {
	k := append([]byte(ns), compositeKeySep...)
	k = append(k, util.EncodeOrderPreservingVarUint64(uint64(len(key)))...)
	k = append(k, []byte(key)...)
	k = append(k, compositeKeySep...)
	k = append(k, util.EncodeOrderPreservingVarUint64(blocknum)...)
	k = append(k, util.EncodeOrderPreservingVarUint64(trannum)...)
	return dataKey(k)
}

// constructRangescanKeys returns start and endKey for performing a range scan
// that covers all the keys for <ns, key>.
// startKey = namespace~len(key)~key~
// endKey = namespace~len(key)~key~0xff
func constructRangeScan(ns string, key string) *rangeScan {
	k := append([]byte(ns), compositeKeySep...)
	k = append(k, util.EncodeOrderPreservingVarUint64(uint64(len(key)))...)
	k = append(k, []byte(key)...)
	k = append(k, compositeKeySep...)

	return &rangeScan{
		startKey: k,
		endKey:   append(k, 0xff),
	}
}

func (r *rangeScan) decodeBlockNumTranNum(dataKey dataKey) (uint64, uint64, error) {
	blockNumTranNumBytes := bytes.TrimPrefix(dataKey, r.startKey)
	blockNum, blockBytesConsumed, err := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes)
	if err != nil {
		return 0, 0, err
	}

	tranNum, tranBytesConsumed, err := util.DecodeOrderPreservingVarUint64(blockNumTranNumBytes[blockBytesConsumed:])
	if err != nil {
		return 0, 0, err
	}

	// The following error should never happen. Keep the check just in case there is some unknown bug.
	if blockBytesConsumed+tranBytesConsumed != len(blockNumTranNumBytes) {
		return 0, 0, errors.Errorf("number of decoded bytes (%d) is not equal to the length of blockNumTranNumBytes (%d)",
			blockBytesConsumed+tranBytesConsumed, len(blockNumTranNumBytes))
	}
	return blockNum, tranNum, nil
}
