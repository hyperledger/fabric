/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataKeyEncoding(t *testing.T) {
	dataKey1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns1", coll: "coll1", blkNum: 2}, txNum: 5}
	datakey2 := decodeDatakey(encodeDataKey(dataKey1))
	assert.Equal(t, dataKey1, datakey2)
}

func TestDatakeyRange(t *testing.T) {
	blockNum := uint64(20)
	startKey, endKey := datakeyRange(blockNum)
	var txNum uint64
	for txNum = 0; txNum < 100; txNum++ {
		keyOfBlock := encodeDataKey(
			&dataKey{
				nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum},
				txNum:     txNum,
			},
		)
		keyOfPreviousBlock := encodeDataKey(
			&dataKey{
				nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum - 1},
				txNum:     txNum,
			},
		)
		keyOfNextBlock := encodeDataKey(
			&dataKey{
				nsCollBlk: nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum + 1},
				txNum:     txNum,
			},
		)
		assert.Equal(t, bytes.Compare(keyOfPreviousBlock, startKey), -1)
		assert.Equal(t, bytes.Compare(keyOfBlock, startKey), 1)
		assert.Equal(t, bytes.Compare(keyOfBlock, endKey), -1)
		assert.Equal(t, bytes.Compare(keyOfNextBlock, endKey), 1)
	}
}

func TestEligibleMissingdataRange(t *testing.T) {
	blockNum := uint64(20)
	startKey, endKey := eligibleMissingdatakeyRange(blockNum)
	var txNum uint64
	for txNum = 0; txNum < 100; txNum++ {
		keyOfBlock := encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum},
				isEligible: true,
			},
		)
		keyOfPreviousBlock := encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum - 1},
				isEligible: true,
			},
		)
		keyOfNextBlock := encodeMissingDataKey(
			&missingDataKey{
				nsCollBlk:  nsCollBlk{ns: "ns", coll: "coll", blkNum: blockNum + 1},
				isEligible: true,
			},
		)
		assert.Equal(t, bytes.Compare(keyOfNextBlock, startKey), -1)
		assert.Equal(t, bytes.Compare(keyOfBlock, startKey), 1)
		assert.Equal(t, bytes.Compare(keyOfBlock, endKey), -1)
		assert.Equal(t, bytes.Compare(keyOfPreviousBlock, endKey), 1)
	}
}
