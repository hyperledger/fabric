/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package history

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

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
		key := constructDataKey(testDatum.ns, testDatum.key, testDatum.blkNum, testDatum.tranNum)
		rangeScan := constructRangeScan(testDatum.ns, testDatum.key)
		require.Equal(t, bytes.Compare(rangeScan.startKey, key), -1) // startKey should be smaller than key
		require.Equal(t, bytes.Compare(rangeScan.endKey, key), 1)    // endKey should be greater than key
	}

	for i, testDatum := range testData {
		for j, another := range testData {
			if i == j {
				continue
			}
			rangeScan := constructRangeScan(testDatum.ns, testDatum.key)
			anotherKey := constructDataKey(another.ns, another.key, another.blkNum, another.tranNum)
			require.False(t, bytes.Compare(anotherKey, rangeScan.startKey) == 1 && bytes.Compare(anotherKey, rangeScan.endKey) == -1) // any key should not fall in the range of start/end key range query for any other key
		}
	}
}

func TestSplitCompositeKey(t *testing.T) {
	dataKey := constructDataKey("ns1", "key1", 20, 200)
	rangeScan := constructRangeScan("ns1", "key1")
	blkNum, txNum, err := rangeScan.decodeBlockNumTranNum(dataKey)
	require.NoError(t, err)
	require.Equal(t, blkNum, uint64(20))
	require.Equal(t, txNum, uint64(200))
}
