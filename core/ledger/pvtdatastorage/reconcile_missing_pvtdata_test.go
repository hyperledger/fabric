/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

func TestCommitPvtDataOfOldBlocks(t *testing.T) {
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 3,
			{"ns-1", "coll-2"}: 1,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 1,
			{"ns-3", "coll-1"}: 0,
			{"ns-3", "coll-2"}: 3,
			{"ns-4", "coll-1"}: 0,
			{"ns-4", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, "TestCommitPvtDataOfOldBlocks", btlPolicy, pvtDataConf())
	defer env.Cleanup()
	store := env.TestStore

	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// CONSTRUCT MISSING DATA FOR BLOCK 1
	blk1MissingData := make(ledger.TxMissingPvtData)

	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	blk1MissingData.Add(1, "ns-2", "coll-1", true)
	blk1MissingData.Add(1, "ns-2", "coll-2", true)
	// eligible missing data in tx2
	blk1MissingData.Add(2, "ns-1", "coll-1", true)
	blk1MissingData.Add(2, "ns-1", "coll-2", true)
	blk1MissingData.Add(2, "ns-3", "coll-1", true)
	blk1MissingData.Add(2, "ns-3", "coll-2", true)

	// CONSTRUCT MISSING DATA FOR BLOCK 2
	blk2MissingData := make(ledger.TxMissingPvtData)
	// eligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-1", true)
	blk2MissingData.Add(1, "ns-1", "coll-2", true)
	// eligible missing data in tx3
	blk2MissingData.Add(3, "ns-1", "coll-1", true)

	// COMMIT BLOCK 0 WITH NO DATA
	require.NoError(t, store.Commit(0, nil, nil))

	// COMMIT BLOCK 1 WITH PVTDATA AND MISSINGDATA
	require.NoError(t, store.Commit(1, testData, blk1MissingData))

	// COMMIT BLOCK 2 WITH PVTDATA AND MISSINGDATA
	require.NoError(t, store.Commit(2, nil, blk2MissingData))

	// CHECK MISSINGDATA ENTRIES ARE CORRECTLY STORED
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-2")

	// missing data in block1, tx2
	expectedMissingPvtDataInfo.Add(1, 2, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 2, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 2, "ns-3", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 2, "ns-3", "coll-2")

	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	// missing data in block2, tx3
	expectedMissingPvtDataInfo.Add(2, 3, "ns-1", "coll-1")

	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(2)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// COMMIT THE MISSINGDATA IN BLOCK 1 AND BLOCK 2
	oldBlocksPvtData := make(map[uint64][]*ledger.TxPvtData)
	oldBlocksPvtData[1] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 1, []string{"ns-1:coll-1", "ns-2:coll-1"}),
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-3:coll-1"}),
	}
	oldBlocksPvtData[2] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1"}),
	}

	err = store.CommitPvtDataOfOldBlocks(oldBlocksPvtData)
	require.NoError(t, err)

	// ENSURE THAT THE PREVIOUSLY MISSING PVTDATA OF BLOCK 1 & 2 EXIST IN THE STORE
	ns1Coll1Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 1}
	ns2Coll1Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-1", blkNum: 1}, txNum: 1}
	ns1Coll1Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns3Coll1Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns1Coll1Blk2Tx3 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 2}, txNum: 3}

	require.True(t, testDataKeyExists(t, store, ns1Coll1Blk1Tx1))
	require.True(t, testDataKeyExists(t, store, ns2Coll1Blk1Tx1))
	require.True(t, testDataKeyExists(t, store, ns1Coll1Blk1Tx2))
	require.True(t, testDataKeyExists(t, store, ns3Coll1Blk1Tx2))
	require.True(t, testDataKeyExists(t, store, ns1Coll1Blk2Tx3))

	// pvt data retrieval for block 2 should return the just committed pvtdata
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(2, nilFilter)
	require.NoError(t, err)
	for i, data := range retrievedData {
		require.Equal(t, data.SeqInBlock, oldBlocksPvtData[2][i].SeqInBlock)
		require.True(t, proto.Equal(data.WriteSet, oldBlocksPvtData[2][i].WriteSet))
	}

	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-2")

	// missing data in block1, tx2
	expectedMissingPvtDataInfo.Add(1, 2, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 2, "ns-3", "coll-2")

	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(2)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// COMMIT BLOCK 3 WITH NO PVTDATA
	require.NoError(t, store.Commit(3, nil, nil))

	// IN BLOCK 1, NS-1:COLL-2 AND NS-2:COLL-2 SHOULD HAVE EXPIRED BUT NOT PURGED
	// HENCE, THE FOLLOWING COMMIT SHOULD CREATE ENTRIES IN THE STORE
	oldBlocksPvtData = make(map[uint64][]*ledger.TxPvtData)
	oldBlocksPvtData[1] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 1, []string{"ns-1:coll-2"}), // though expired, it
		// would get committed to the store as it is not purged yet
		produceSamplePvtdata(t, 2, []string{"ns-3:coll-2"}), // never expires
	}

	err = store.CommitPvtDataOfOldBlocks(oldBlocksPvtData)
	require.NoError(t, err)

	ns1Coll2Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns2Coll2Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns1Coll2Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}
	ns3Coll2Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, txNum: 2}

	// though the pvtdata are expired but not purged yet, we do
	// commit the data and hence the entries would exist in the
	// store
	require.True(t, testDataKeyExists(t, store, ns1Coll2Blk1Tx1))  // expired but committed
	require.False(t, testDataKeyExists(t, store, ns2Coll2Blk1Tx1)) // expired but still missing
	require.False(t, testDataKeyExists(t, store, ns1Coll2Blk1Tx2)) // expired still missing
	require.True(t, testDataKeyExists(t, store, ns3Coll2Blk1Tx2))  // never expires

	// COMMIT BLOCK 4 WITH NO PVTDATA
	require.NoError(t, store.Commit(4, nil, nil))

	testWaitForPurgerRoutineToFinish(store)

	// IN BLOCK 1, NS-1:COLL-2 AND NS-2:COLL-2 SHOULD HAVE EXPIRED BUT NOT PURGED
	// HENCE, THE FOLLOWING COMMIT SHOULD NOT CREATE ENTRIES IN THE STORE
	oldBlocksPvtData = make(map[uint64][]*ledger.TxPvtData)
	oldBlocksPvtData[1] = []*ledger.TxPvtData{
		// both data are expired and purged. hence, it won't be
		// committed to the store
		produceSamplePvtdata(t, 1, []string{"ns-2:coll-2"}),
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2"}),
	}

	err = store.CommitPvtDataOfOldBlocks(oldBlocksPvtData)
	require.NoError(t, err)

	ns1Coll2Blk1Tx1 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns2Coll2Blk1Tx1 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns1Coll2Blk1Tx2 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}
	ns3Coll2Blk1Tx2 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, txNum: 2}

	require.False(t, testDataKeyExists(t, store, ns1Coll2Blk1Tx1)) // purged
	require.False(t, testDataKeyExists(t, store, ns2Coll2Blk1Tx1)) // purged
	require.False(t, testDataKeyExists(t, store, ns1Coll2Blk1Tx2)) // purged
	require.True(t, testDataKeyExists(t, store, ns3Coll2Blk1Tx2))  // never expires
}
