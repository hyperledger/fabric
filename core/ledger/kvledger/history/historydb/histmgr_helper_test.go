/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package historydb

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/stretchr/testify/assert"
)

var strKeySep = string(CompositeKeySep)

func TestCompositeKeyConstruction(t *testing.T) {
	type keyComponents struct {
		ns, key         string
		blkNum, tranNum uint64
	}

	testData := []*keyComponents{
		{"ns1", "key1", 1, 0},
		{"ns1", "key1\x00", 1, 5},
		{"ns1", "key1\x00\x00", 100, 100},
		{"ns1", "key\x00\x001", 100, 100},
		{"ns1", "\x00key\x00\x001", 100, 100},
	}

	for _, testDatum := range testData {
		key := ConstructCompositeHistoryKey(testDatum.ns, testDatum.key, testDatum.blkNum, testDatum.tranNum)
		startKey := ConstructPartialCompositeHistoryKey(testDatum.ns, testDatum.key, false)
		endKey := ConstructPartialCompositeHistoryKey(testDatum.ns, testDatum.key, true)
		assert.Equal(t, bytes.Compare(startKey, key), -1) //startKey should be smaller than key
		assert.Equal(t, bytes.Compare(endKey, key), 1)    //endKey should be greater than key
	}

	for i, testDatum := range testData {
		for j, another := range testData {
			if i == j {
				continue
			}
			startKey := ConstructPartialCompositeHistoryKey(testDatum.ns, testDatum.key, false)
			endKey := ConstructPartialCompositeHistoryKey(testDatum.ns, testDatum.key, true)

			anotherKey := ConstructCompositeHistoryKey(another.ns, another.key, another.blkNum, another.tranNum)
			assert.False(t, bytes.Compare(anotherKey, startKey) == 1 && bytes.Compare(anotherKey, endKey) == -1) //any key should not fall in the range of start/end key range query for any other key
		}
	}
}

func TestSplitCompositeKey(t *testing.T) {
	compositeFullKey := ConstructCompositeHistoryKey("ns1", "key1", 20, 200)
	compositePartialKey := ConstructPartialCompositeHistoryKey("ns1", "key1", false)
	_, extraBytes := SplitCompositeHistoryKey(compositeFullKey, compositePartialKey)
	blkNum, bytesConsumed, err := util.DecodeOrderPreservingVarUint64(extraBytes)
	assert.NoError(t, err)
	txNum, _, err := util.DecodeOrderPreservingVarUint64(extraBytes[bytesConsumed:])
	assert.NoError(t, err)
	// second position should hold the extra bytes that were split off
	assert.Equal(t, blkNum, uint64(20))
	assert.Equal(t, txNum, uint64(200))
}
