/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

func TestDataKeyEncoding(t *testing.T) {
	dataKey1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns1", coll: "coll1", blkNum: 2}, txNum: 5}
	datakey2, err := decodeDatakey(encodeDataKey(dataKey1))
	require.NoError(t, err)
	require.Equal(t, dataKey1, datakey2)
}

func TestDataKeyRange(t *testing.T) {
	blockNum := uint64(20)
	startKey, endKey := getDataKeysForRangeScanByBlockNum(blockNum)
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
		require.Equal(t, bytes.Compare(keyOfPreviousBlock, startKey), -1)
		require.Equal(t, bytes.Compare(keyOfBlock, startKey), 1)
		require.Equal(t, bytes.Compare(keyOfBlock, endKey), -1)
		require.Equal(t, bytes.Compare(keyOfNextBlock, endKey), 1)
	}
}

func TestEligibleMissingDataRange(t *testing.T) {
	blockNum := uint64(20)
	startKey, endKey := eligibleMissingdatakeyRange(blockNum)
	var txNum uint64
	for txNum = 0; txNum < 100; txNum++ {
		keyOfBlock := encodeElgPrioMissingDataKey(
			&missingDataKey{
				nsCollBlk: nsCollBlk{
					ns:     "ns",
					coll:   "coll",
					blkNum: blockNum,
				},
			},
		)
		keyOfPreviousBlock := encodeElgPrioMissingDataKey(
			&missingDataKey{
				nsCollBlk: nsCollBlk{
					ns:     "ns",
					coll:   "coll",
					blkNum: blockNum - 1,
				},
			},
		)
		keyOfNextBlock := encodeElgPrioMissingDataKey(
			&missingDataKey{
				nsCollBlk: nsCollBlk{
					ns:     "ns",
					coll:   "coll",
					blkNum: blockNum + 1,
				},
			},
		)
		require.Equal(t, bytes.Compare(keyOfNextBlock, startKey), -1)
		require.Equal(t, bytes.Compare(keyOfBlock, startKey), 1)
		require.Equal(t, bytes.Compare(keyOfBlock, endKey), -1)
		require.Equal(t, bytes.Compare(keyOfPreviousBlock, endKey), 1)
	}
}

func TestEncodeDecodeMissingdataKey(t *testing.T) {
	for i := 0; i < 1000; i++ {
		testEncodeDecodeMissingdataKey(t, uint64(i))
	}
	testEncodeDecodeMissingdataKey(t, math.MaxUint64) // corner case
}

func testEncodeDecodeMissingdataKey(t *testing.T, blkNum uint64) {
	key := &missingDataKey{
		nsCollBlk: nsCollBlk{
			ns:     "ns",
			coll:   "coll",
			blkNum: blkNum,
		},
	}

	t.Run("ineligibileKey",
		func(t *testing.T) {
			decodedKey := decodeInelgMissingDataKey(
				encodeInelgMissingDataKey(key),
			)
			require.Equal(t, key, decodedKey)
		},
	)

	t.Run("eligiblePrioritizedKey",
		func(t *testing.T) {
			decodedKey := decodeElgMissingDataKey(
				encodeElgPrioMissingDataKey(key),
			)
			require.Equal(t, key, decodedKey)
		},
	)

	t.Run("eligibleDeprioritizedKey",
		func(t *testing.T) {
			decodedKey := decodeElgMissingDataKey(
				encodeElgDeprioMissingDataKey(key),
			)
			require.Equal(t, key, decodedKey)
		},
	)
}

func TestBasicEncodingDecoding(t *testing.T) {
	for i := 0; i < 10000; i++ {
		value := encodeReverseOrderVarUint64(uint64(i))
		nextValue := encodeReverseOrderVarUint64(uint64(i + 1))
		if !(bytes.Compare(value, nextValue) > 0) {
			t.Fatalf("A smaller integer should result into greater bytes. Encoded bytes for [%d] is [%x] and for [%d] is [%x]",
				i, i+1, value, nextValue)
		}
		decodedValue, _ := decodeReverseOrderVarUint64(value)
		if decodedValue != uint64(i) {
			t.Fatalf("Value not same after decoding. Original value = [%d], decode value = [%d]", i, decodedValue)
		}
	}
}

func TestDecodingAppendedValues(t *testing.T) {
	appendedValues := []byte{}
	for i := 0; i < 1000; i++ {
		appendedValues = append(appendedValues, encodeReverseOrderVarUint64(uint64(i))...)
	}

	len := 0
	value := uint64(0)
	for i := 0; i < 1000; i++ {
		appendedValues = appendedValues[len:]
		value, len = decodeReverseOrderVarUint64(appendedValues)
		if value != uint64(i) {
			t.Fatalf("expected value = [%d], decode value = [%d]", i, value)
		}
	}
}

func TestEncodingDecodingLastBlockInSnapshotVal(t *testing.T) {
	t.Run("basic-coding-encoding", func(t *testing.T) {
		for i := uint64(0); i < 100; i++ {
			encoded := encodeLastBlockInBootSnapshotVal(i)
			decoded, err := decodeLastBlockInBootSnapshotVal(encoded)
			require.NoError(t, err)
			require.Equal(t, i, decoded)
		}
	})

	t.Run("error-case", func(t *testing.T) {
		_, err := decodeLastBlockInBootSnapshotVal([]byte{0xff})
		require.EqualError(t, err, "unexpected bytes for interpreting as varint")
	})
}

func TestEncodingHashedIndexKey(t *testing.T) {
	encKey := encodeHashedIndexKey(&hashedIndexKey{
		ns:         "ns",
		coll:       "coll",
		pvtkeyHash: []byte("pvtkeyHash"),
		blkNum:     10,
		txNum:      100,
	})

	expectedEncKey := []byte("bns\x00coll\x00pvtkeyHash")
	expectedEncKey = append(expectedEncKey, version.NewHeight(10, 100).ToBytes()...)
	require.Equal(t, expectedEncKey, encKey)
}

func TestDeriveDataKeyFromHashedIndexKey(t *testing.T) {
	testcases := []struct {
		ns, coll      string
		blkNum, txNum uint64
	}{
		{
			"ns", "coll", 10, 1000,
		},
		{
			"ns", "coll", 0, 0,
		},
		{
			"", "coll", 0, 0,
		},
		{
			"ns", "", 0, 0,
		},
		{
			"", "", 0, 0,
		},
		{
			"", "", 10, 10,
		},
	}

	for i, testcase := range testcases {
		t.Run(fmt.Sprintf("testcase-%d", i), func(t *testing.T) {
			datakeyBytes, err := deriveDataKeyFromEncodedHashedIndexKey(
				encodeHashedIndexKey(&hashedIndexKey{
					ns:         testcase.ns,
					coll:       testcase.coll,
					blkNum:     testcase.blkNum,
					txNum:      testcase.txNum,
					pvtkeyHash: util.ComputeStringHash("dummyKey"),
				}),
			)
			require.NoError(t, err)

			dk, err := decodeDatakey(datakeyBytes)
			require.NoError(t, err)
			require.Equal(t,
				&dataKey{
					nsCollBlk: nsCollBlk{
						ns:     testcase.ns,
						coll:   testcase.coll,
						blkNum: testcase.blkNum,
					},
					txNum: testcase.txNum,
				},
				dk,
			)
		})
	}

	t.Run("random bytes should cause an error", func(t *testing.T) {
		_, err := deriveDataKeyFromEncodedHashedIndexKey([]byte("random-bytes"))
		require.EqualError(t, err, fmt.Sprintf("unexpected bytes [%x] for HashedIndexed key", "random-bytes"))
	})
}
