/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"
	"math"
	"testing"

	"github.com/bits-and-blooms/bitset"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

type blockTxPvtDataInfoForTest struct {
	blkNum         uint64
	txNum          uint64
	pvtDataPresent map[string][]string
	pvtDataMissing map[string][]string
}

type pvtDataForTest struct {
	pvtData            []*ledger.TxPvtData
	dataKeys           []*dataKey
	hashedIndexEntries []*hashedIndexEntry
	missingDataInfo    ledger.TxMissingPvtData
}

func TestCommitPvtDataOfOldBlocks(t *testing.T) {
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, "TestCommitPvtDataOfOldBlocks", btlPolicy, pvtDataConf())
	defer env.Cleanup()
	store := env.TestStore

	blockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
		{
			blkNum: 1,
			txNum:  1,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1", "coll-2"},
				"ns-2": {"coll-1", "coll-2"},
			},
		},
		{
			blkNum: 1,
			txNum:  2,
			pvtDataPresent: map[string][]string{
				"ns-2": {"coll-1", "coll-2"},
			},
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1", "coll-2"},
			},
		},
		{
			blkNum: 1,
			txNum:  4,
			pvtDataPresent: map[string][]string{
				"ns-1": {"coll-1", "coll-2"},
				"ns-2": {"coll-1", "coll-2"},
			},
		},
		{
			blkNum: 2,
			txNum:  1,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1", "coll-2"},
			},
		},
		{
			blkNum: 2,
			txNum:  3,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1"},
			},
		},
	}

	blocksPvtData, missingDataSummary := constructPvtDataForTest(t, blockTxPvtDataInfo)

	require.NoError(t, store.Commit(0, nil, nil, nil))
	require.NoError(t, store.Commit(1, blocksPvtData[1].pvtData, blocksPvtData[1].missingDataInfo, nil))
	require.NoError(t, store.Commit(2, blocksPvtData[2].pvtData, blocksPvtData[2].missingDataInfo, nil))

	assertMissingDataInfo(t, store, missingDataSummary, 2)

	// COMMIT some of the missing data in the block 1 and block 2
	oldBlockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
		{
			blkNum: 1,
			txNum:  1,
			pvtDataPresent: map[string][]string{
				"ns-1": {"coll-1"},
				"ns-2": {"coll-1"},
			},
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-2"},
				"ns-2": {"coll-2"},
			},
		},
		{
			blkNum: 1,
			txNum:  2,
			pvtDataPresent: map[string][]string{
				"ns-1": {"coll-1"},
			},
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-2"},
			},
		},
		{
			blkNum: 2,
			txNum:  1,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1", "coll-2"},
			},
		},
		{
			blkNum: 2,
			txNum:  3,
			pvtDataPresent: map[string][]string{
				"ns-1": {"coll-1"},
			},
		},
	}

	blocksPvtData, missingDataSummary = constructPvtDataForTest(t, oldBlockTxPvtDataInfo)
	oldBlocksPvtData := map[uint64][]*ledger.TxPvtData{
		1: blocksPvtData[1].pvtData,
		2: blocksPvtData[2].pvtData,
	}
	require.NoError(t, store.CommitPvtDataOfOldBlocks(oldBlocksPvtData, nil))

	for _, b := range blocksPvtData {
		for _, dkey := range b.dataKeys {
			require.True(t, testDataKeyExists(t, store, dkey))
		}
		for _, b := range blocksPvtData {
			for _, h := range b.hashedIndexEntries {
				require.True(t, testHashedIndexExists(t, store, h.key))
			}
		}
	}
	assertMissingDataInfo(t, store, missingDataSummary, 2)
}

func TestCommitPvtDataOfOldBlocksWithBTL(t *testing.T) {
	setup := func(store *Store) {
		blockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
			{
				blkNum: 1,
				txNum:  1,
				pvtDataMissing: map[string][]string{
					"ns-1": {"coll-1"},
					"ns-2": {"coll-1"},
				},
			},
			{
				blkNum: 1,
				txNum:  2,
				pvtDataMissing: map[string][]string{
					"ns-1": {"coll-1"},
					"ns-2": {"coll-1"},
				},
			},
			{
				blkNum: 1,
				txNum:  3,
				pvtDataMissing: map[string][]string{
					"ns-1": {"coll-1"},
					"ns-2": {"coll-1"},
				},
			},
		}

		blocksPvtData, missingDataSummary := constructPvtDataForTest(t, blockTxPvtDataInfo)

		require.NoError(t, store.Commit(0, nil, nil, nil))
		require.NoError(t, store.Commit(1, blocksPvtData[1].pvtData, blocksPvtData[1].missingDataInfo, nil))

		assertMissingDataInfo(t, store, missingDataSummary, 1)

		// COMMIT BLOCK 2 & 3 WITH NO PVTDATA
		require.NoError(t, store.Commit(2, nil, nil, nil))
		require.NoError(t, store.Commit(3, nil, nil, nil))
	}

	t.Run("expired but not purged", func(t *testing.T) {
		btlPolicy := btltestutil.SampleBTLPolicy(
			map[[2]string]uint64{
				{"ns-1", "coll-1"}: 1,
				{"ns-2", "coll-1"}: 1,
			},
		)
		env := NewTestStoreEnv(t, "TestCommitPvtDataOfOldBlocksWithBTL", btlPolicy, pvtDataConf())
		defer env.Cleanup()
		store := env.TestStore

		setup(store)
		// in block 1, ns-1:coll-1 and ns-2:coll-2 should have expired but not purged.
		// hence, the commit of pvtdata of block 1 transaction 1 should create entries
		// in the store
		oldBlockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
			{
				blkNum: 1,
				txNum:  1,
				pvtDataPresent: map[string][]string{
					"ns-1": {"coll-1"},
					"ns-2": {"coll-1"},
				},
			},
		}

		blocksPvtData, _ := constructPvtDataForTest(t, oldBlockTxPvtDataInfo)
		oldBlocksPvtData := map[uint64][]*ledger.TxPvtData{
			1: blocksPvtData[1].pvtData,
		}
		deprioritizedList := ledger.MissingPvtDataInfo{
			1: ledger.MissingBlockPvtdataInfo{
				2: {
					{
						Namespace:  "ns-1",
						Collection: "coll-1",
					},
					{
						Namespace:  "ns-2",
						Collection: "coll-1",
					},
				},
			},
		}
		require.NoError(t, store.CommitPvtDataOfOldBlocks(oldBlocksPvtData, deprioritizedList))

		for _, b := range blocksPvtData {
			for _, dkey := range b.dataKeys {
				require.True(t, testDataKeyExists(t, store, dkey))
			}
		}
		// as all missing data are expired, get missing info would return nil though
		// it is not purged yet
		assertMissingDataInfo(t, store, make(ledger.MissingPvtDataInfo), 1)

		// deprioritized list should be present
		tests := []struct {
			key            nsCollBlk
			expectedBitmap *bitset.BitSet
		}{
			{
				key: nsCollBlk{
					ns:     "ns-1",
					coll:   "coll-1",
					blkNum: 1,
				},
				expectedBitmap: constructBitSetForTest(2),
			},
			{
				key: nsCollBlk{
					ns:     "ns-2",
					coll:   "coll-1",
					blkNum: 1,
				},
				expectedBitmap: constructBitSetForTest(2),
			},
		}

		for _, tt := range tests {
			encKey := encodeElgDeprioMissingDataKey(&missingDataKey{tt.key})
			missingData, err := store.db.Get(encKey)
			require.NoError(t, err)

			expectedMissingData, err := encodeMissingDataValue(tt.expectedBitmap)
			require.NoError(t, err)
			require.Equal(t, expectedMissingData, missingData)
		}
	})

	t.Run("expired and purged", func(t *testing.T) {
		btlPolicy := btltestutil.SampleBTLPolicy(
			map[[2]string]uint64{
				{"ns-1", "coll-1"}: 1,
				{"ns-2", "coll-1"}: 1,
			},
		)
		env := NewTestStoreEnv(t, "TestCommitPvtDataOfOldBlocksWithBTL", btlPolicy, pvtDataConf())
		defer env.Cleanup()
		store := env.TestStore

		setup(store)
		require.NoError(t, store.Commit(4, nil, nil, nil))

		testWaitForPurgerRoutineToFinish(store)

		// in block 1, ns-1:coll-1 and ns-2:coll-2 should have expired and purged.
		// hence, the commit of pvtdata of block 1 transaction 2 should not create
		// entries in the store
		oldBlockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
			{
				blkNum: 1,
				txNum:  2,
				pvtDataPresent: map[string][]string{
					"ns-1": {"coll-1"},
					"ns-2": {"coll-1"},
				},
			},
		}
		blocksPvtData, _ := constructPvtDataForTest(t, oldBlockTxPvtDataInfo)
		oldBlocksPvtData := map[uint64][]*ledger.TxPvtData{
			1: blocksPvtData[1].pvtData,
		}
		deprioritizedList := ledger.MissingPvtDataInfo{
			1: ledger.MissingBlockPvtdataInfo{
				3: {
					{
						Namespace:  "ns-1",
						Collection: "coll-1",
					},
					{
						Namespace:  "ns-2",
						Collection: "coll-1",
					},
				},
			},
		}
		require.NoError(t, store.CommitPvtDataOfOldBlocks(oldBlocksPvtData, deprioritizedList))

		for _, b := range blocksPvtData {
			for _, dkey := range b.dataKeys {
				require.False(t, testDataKeyExists(t, store, dkey))
			}
		}

		// deprioritized list should not be present
		keys := []nsCollBlk{
			{
				ns:     "ns-1",
				coll:   "coll-1",
				blkNum: 1,
			},
			{
				ns:     "ns-2",
				coll:   "coll-1",
				blkNum: 1,
			},
		}

		for _, k := range keys {
			encKey := encodeElgDeprioMissingDataKey(&missingDataKey{k})
			missingData, err := store.db.Get(encKey)
			require.NoError(t, err)
			require.Nil(t, missingData)
		}
	})
}

func TestCommitPvtDataOfOldBlocksWithDeprioritization(t *testing.T) {
	blockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
		{
			blkNum: 1,
			txNum:  1,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1"},
				"ns-2": {"coll-1"},
			},
		},
		{
			blkNum: 1,
			txNum:  2,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1"},
				"ns-2": {"coll-1"},
			},
		},
		{
			blkNum: 2,
			txNum:  1,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1"},
				"ns-2": {"coll-1"},
			},
		},
		{
			blkNum: 2,
			txNum:  2,
			pvtDataMissing: map[string][]string{
				"ns-1": {"coll-1"},
				"ns-2": {"coll-1"},
			},
		},
	}

	blocksPvtData, missingDataSummary := constructPvtDataForTest(t, blockTxPvtDataInfo)

	tests := []struct {
		name                        string
		deprioritizedList           ledger.MissingPvtDataInfo
		expectedPrioMissingDataKeys ledger.MissingPvtDataInfo
	}{
		{
			name:                        "all keys deprioritized",
			deprioritizedList:           missingDataSummary,
			expectedPrioMissingDataKeys: make(ledger.MissingPvtDataInfo),
		},
		{
			name: "some keys deprioritized",
			deprioritizedList: ledger.MissingPvtDataInfo{
				1: ledger.MissingBlockPvtdataInfo{
					2: {
						{
							Namespace:  "ns-1",
							Collection: "coll-1",
						},
					},
				},
				2: ledger.MissingBlockPvtdataInfo{
					2: {
						{
							Namespace:  "ns-1",
							Collection: "coll-1",
						},
					},
				},
			},
			expectedPrioMissingDataKeys: ledger.MissingPvtDataInfo{
				1: ledger.MissingBlockPvtdataInfo{
					1: {
						{
							Namespace:  "ns-1",
							Collection: "coll-1",
						},
						{
							Namespace:  "ns-2",
							Collection: "coll-1",
						},
					},
					2: {
						{
							Namespace:  "ns-2",
							Collection: "coll-1",
						},
					},
				},
				2: ledger.MissingBlockPvtdataInfo{
					1: {
						{
							Namespace:  "ns-1",
							Collection: "coll-1",
						},
						{
							Namespace:  "ns-2",
							Collection: "coll-1",
						},
					},
					2: {
						{
							Namespace:  "ns-2",
							Collection: "coll-1",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			btlPolicy := btltestutil.SampleBTLPolicy(
				map[[2]string]uint64{
					{"ns-1", "coll-1"}: 0,
					{"ns-2", "coll-1"}: 0,
				},
			)
			env := NewTestStoreEnv(t, "TestCommitPvtDataOfOldBlocksWithDeprio", btlPolicy, pvtDataConf())
			defer env.Cleanup()
			store := env.TestStore

			// COMMIT BLOCK 0 WITH NO DATA
			require.NoError(t, store.Commit(0, nil, nil, nil))
			require.NoError(t, store.Commit(1, blocksPvtData[1].pvtData, blocksPvtData[1].missingDataInfo, nil))
			require.NoError(t, store.Commit(2, blocksPvtData[2].pvtData, blocksPvtData[2].missingDataInfo, nil))

			assertMissingDataInfo(t, store, missingDataSummary, 2)

			require.NoError(t, store.CommitPvtDataOfOldBlocks(nil, tt.deprioritizedList))

			prioMissingData, err := store.getMissingData(elgPrioritizedMissingDataGroup, math.MaxUint64, 3)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedPrioMissingDataKeys), len(prioMissingData))
			for blkNum, txsMissingData := range tt.expectedPrioMissingDataKeys {
				for txNum, expectedMissingData := range txsMissingData {
					require.ElementsMatch(t, expectedMissingData, prioMissingData[blkNum][txNum])
				}
			}

			deprioMissingData, err := store.getMissingData(elgDeprioritizedMissingDataGroup, math.MaxUint64, 3)
			require.NoError(t, err)
			require.Equal(t, len(tt.deprioritizedList), len(deprioMissingData))
			for blkNum, txsMissingData := range tt.deprioritizedList {
				for txNum, expectedMissingData := range txsMissingData {
					require.ElementsMatch(t, expectedMissingData, deprioMissingData[blkNum][txNum])
				}
			}

			oldBlockTxPvtDataInfo := []*blockTxPvtDataInfoForTest{
				{
					blkNum: 1,
					txNum:  1,
					pvtDataPresent: map[string][]string{
						"ns-1": {"coll-1"},
						"ns-2": {"coll-1"},
					},
				},
				{
					blkNum: 1,
					txNum:  2,
					pvtDataPresent: map[string][]string{
						"ns-1": {"coll-1"},
						"ns-2": {"coll-1"},
					},
				},
				{
					blkNum: 2,
					txNum:  1,
					pvtDataPresent: map[string][]string{
						"ns-1": {"coll-1"},
						"ns-2": {"coll-1"},
					},
				},
				{
					blkNum: 2,
					txNum:  2,
					pvtDataPresent: map[string][]string{
						"ns-1": {"coll-1"},
						"ns-2": {"coll-1"},
					},
				},
			}

			pvtDataOfOldBlocks, _ := constructPvtDataForTest(t, oldBlockTxPvtDataInfo)
			oldBlocksPvtData := map[uint64][]*ledger.TxPvtData{
				1: pvtDataOfOldBlocks[1].pvtData,
				2: pvtDataOfOldBlocks[2].pvtData,
			}
			require.NoError(t, store.CommitPvtDataOfOldBlocks(oldBlocksPvtData, nil))

			prioMissingData, err = store.getMissingData(elgPrioritizedMissingDataGroup, math.MaxUint64, 3)
			require.NoError(t, err)
			require.Equal(t, make(ledger.MissingPvtDataInfo), prioMissingData)

			deprioMissingData, err = store.getMissingData(elgDeprioritizedMissingDataGroup, math.MaxUint64, 3)
			require.NoError(t, err)
			require.Equal(t, make(ledger.MissingPvtDataInfo), deprioMissingData)
		})
	}
}

func constructPvtDataForTest(t *testing.T, blockInfo []*blockTxPvtDataInfoForTest) (map[uint64]*pvtDataForTest, ledger.MissingPvtDataInfo) {
	blocksPvtData := make(map[uint64]*pvtDataForTest)
	missingPvtDataInfoSummary := make(ledger.MissingPvtDataInfo)

	for _, b := range blockInfo {
		p, ok := blocksPvtData[b.blkNum]
		if !ok {
			p = &pvtDataForTest{
				missingDataInfo: make(ledger.TxMissingPvtData),
			}
			blocksPvtData[b.blkNum] = p
		}

		for ns, colls := range b.pvtDataMissing {
			for _, coll := range colls {
				p.missingDataInfo.Add(b.txNum, ns, coll, true)
				missingPvtDataInfoSummary.Add(b.blkNum, b.txNum, ns, coll)
			}
		}

		var nsColls []string
		for ns, colls := range b.pvtDataPresent {
			for _, coll := range colls {
				nsColls = append(nsColls, fmt.Sprintf("%s:%s", ns, coll))
				p.dataKeys = append(p.dataKeys, &dataKey{
					nsCollBlk: nsCollBlk{
						ns:     ns,
						coll:   coll,
						blkNum: b.blkNum,
					},
					txNum: b.txNum,
				})
			}
		}

		if len(nsColls) == 0 {
			continue
		}
		p.pvtData = append(
			p.pvtData,
			produceSamplePvtdata(t, b.txNum, nsColls),
		)
		for _, txPvtdata := range p.pvtData {
			txPvtWS, err := rwsetutil.TxPvtRwSetFromProtoMsg(txPvtdata.WriteSet)
			require.NoError(t, err)
			for _, ns := range txPvtWS.NsPvtRwSet {
				for _, coll := range ns.CollPvtRwSets {
					for _, kv := range coll.KvRwSet.Writes {
						p.hashedIndexEntries = append(p.hashedIndexEntries, &hashedIndexEntry{
							key: &hashedIndexKey{
								ns:         ns.NameSpace,
								coll:       coll.CollectionName,
								pvtkeyHash: util.ComputeStringHash(kv.Key),
								blkNum:     b.blkNum,
								txNum:      b.txNum,
							},
							value: kv.Key,
						})
					}
				}
			}

		}
	}

	return blocksPvtData, missingPvtDataInfoSummary
}

func assertMissingDataInfo(t *testing.T, store *Store, expected ledger.MissingPvtDataInfo, numRecentBlocks int) {
	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(math.MaxUint64, numRecentBlocks)
	require.NoError(t, err)
	require.Equal(t, len(expected), len(missingPvtDataInfo))
	for blkNum, txsMissingData := range expected {
		for txNum, expectedMissingData := range txsMissingData {
			require.ElementsMatch(t, expectedMissingData, missingPvtDataInfo[blkNum][txNum])
		}
	}
}

func constructBitSetForTest(txNums ...uint) *bitset.BitSet {
	bitmap := &bitset.BitSet{}
	for _, txNum := range txNums {
		bitmap.Set(txNum)
	}
	return bitmap
}
