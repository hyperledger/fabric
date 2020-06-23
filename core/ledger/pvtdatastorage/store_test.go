/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("pvtdatastorage=debug")
	os.Exit(m.Run())
}

func TestEmptyStore(t *testing.T) {
	env := NewTestStoreEnv(t, "TestEmptyStore", nil, pvtDataConf())
	defer env.Cleanup()
	require := require.New(t)
	store := env.TestStore
	require.True(store.isEmpty)
}

func TestStoreBasicCommitAndRetrieval(t *testing.T) {
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 0,
			{"ns-3", "coll-1"}: 0,
			{"ns-4", "coll-1"}: 0,
			{"ns-4", "coll-2"}: 0,
		},
	)

	env := NewTestStoreEnv(t, "TestStoreBasicCommitAndRetrieval", btlPolicy, pvtDataConf())
	defer env.Cleanup()
	require := require.New(t)
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtDataMap)

	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	blk1MissingData.Add(1, "ns-2", "coll-1", true)
	blk1MissingData.Add(1, "ns-2", "coll-2", true)
	// eligible missing data in tx2
	blk1MissingData.Add(2, "ns-3", "coll-1", true)
	// ineligible missing data in tx4
	blk1MissingData.Add(4, "ns-4", "coll-1", false)
	blk1MissingData.Add(4, "ns-4", "coll-2", false)

	// construct missing data for block 2
	blk2MissingData := make(ledger.TxMissingPvtDataMap)
	// eligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-1", true)
	blk2MissingData.Add(1, "ns-1", "coll-2", true)
	// eligible missing data in tx3
	blk2MissingData.Add(3, "ns-1", "coll-1", true)

	// no pvt data with block 0
	require.NoError(store.Commit(0, nil, nil))

	// pvt data with block 1 - commit
	require.NoError(store.Commit(1, testData, blk1MissingData))

	// pvt data retrieval for block 0 should return nil
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(0, nilFilter)
	require.NoError(err)
	require.Nil(retrievedData)

	// pvt data retrieval for block 1 should return full pvtdata
	retrievedData, err = store.GetPvtDataByBlockNum(1, nilFilter)
	require.NoError(err)
	for i, data := range retrievedData {
		require.Equal(data.SeqInBlock, testData[i].SeqInBlock)
		require.True(proto.Equal(data.WriteSet, testData[i].WriteSet))
	}

	// pvt data retrieval for block 1 with filter should return filtered pvtdata
	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	filter.Add("ns-2", "coll-2")
	retrievedData, err = store.GetPvtDataByBlockNum(1, filter)
	require.NoError(err)
	expectedRetrievedData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-2:coll-2"}),
	}
	for i, data := range retrievedData {
		require.Equal(data.SeqInBlock, expectedRetrievedData[i].SeqInBlock)
		require.True(proto.Equal(data.WriteSet, expectedRetrievedData[i].WriteSet))
	}

	// pvt data retrieval for block 2 should return ErrOutOfRange
	retrievedData, err = store.GetPvtDataByBlockNum(2, nilFilter)
	_, ok := err.(*ErrOutOfRange)
	require.True(ok)
	require.Nil(retrievedData)

	// pvt data with block 2 - commit
	require.NoError(store.Commit(2, testData, blk2MissingData))

	// retrieve the stored missing entries using GetMissingPvtDataInfoForMostRecentBlocks
	// Only the code path of eligible entries would be covered in this unit-test. For
	// ineligible entries, the code path will be covered in FAB-11437

	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 3, "ns-1", "coll-1")

	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(1)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-2")

	// missing data in block1, tx2
	expectedMissingPvtDataInfo.Add(1, 2, "ns-3", "coll-1")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(2)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)
}

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
	require := require.New(t)
	store := env.TestStore

	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// CONSTRUCT MISSING DATA FOR BLOCK 1
	blk1MissingData := make(ledger.TxMissingPvtDataMap)

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
	blk2MissingData := make(ledger.TxMissingPvtDataMap)
	// eligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-1", true)
	blk2MissingData.Add(1, "ns-1", "coll-2", true)
	// eligible missing data in tx3
	blk2MissingData.Add(3, "ns-1", "coll-1", true)

	// COMMIT BLOCK 0 WITH NO DATA
	require.NoError(store.Commit(0, nil, nil))

	// COMMIT BLOCK 1 WITH PVTDATA AND MISSINGDATA
	require.NoError(store.Commit(1, testData, blk1MissingData))

	// COMMIT BLOCK 2 WITH PVTDATA AND MISSINGDATA
	require.NoError(store.Commit(2, nil, blk2MissingData))

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
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

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
	require.NoError(err)

	// ENSURE THAT THE PREVIOUSLY MISSING PVTDATA OF BLOCK 1 & 2 EXIST IN THE STORE
	ns1Coll1Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 1}
	ns2Coll1Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-1", blkNum: 1}, txNum: 1}
	ns1Coll1Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns3Coll1Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns1Coll1Blk2Tx3 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 2}, txNum: 3}

	require.True(testDataKeyExists(t, store, ns1Coll1Blk1Tx1))
	require.True(testDataKeyExists(t, store, ns2Coll1Blk1Tx1))
	require.True(testDataKeyExists(t, store, ns1Coll1Blk1Tx2))
	require.True(testDataKeyExists(t, store, ns3Coll1Blk1Tx2))
	require.True(testDataKeyExists(t, store, ns1Coll1Blk2Tx3))

	// pvt data retrieval for block 2 should return the just committed pvtdata
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(2, nilFilter)
	require.NoError(err)
	for i, data := range retrievedData {
		require.Equal(data.SeqInBlock, oldBlocksPvtData[2][i].SeqInBlock)
		require.True(proto.Equal(data.WriteSet, oldBlocksPvtData[2][i].WriteSet))
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
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// COMMIT BLOCK 3 WITH NO PVTDATA
	require.NoError(store.Commit(3, nil, nil))

	// IN BLOCK 1, NS-1:COLL-2 AND NS-2:COLL-2 SHOULD HAVE EXPIRED BUT NOT PURGED
	// HENCE, THE FOLLOWING COMMIT SHOULD CREATE ENTRIES IN THE STORE
	oldBlocksPvtData = make(map[uint64][]*ledger.TxPvtData)
	oldBlocksPvtData[1] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 1, []string{"ns-1:coll-2"}), // though expired, it
		// would get committed to the store as it is not purged yet
		produceSamplePvtdata(t, 2, []string{"ns-3:coll-2"}), // never expires
	}

	err = store.CommitPvtDataOfOldBlocks(oldBlocksPvtData)
	require.NoError(err)

	ns1Coll2Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns2Coll2Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns1Coll2Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}
	ns3Coll2Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, txNum: 2}

	// though the pvtdata are expired but not purged yet, we do
	// commit the data and hence the entries would exist in the
	// store
	require.True(testDataKeyExists(t, store, ns1Coll2Blk1Tx1))  // expired but committed
	require.False(testDataKeyExists(t, store, ns2Coll2Blk1Tx1)) // expired but still missing
	require.False(testDataKeyExists(t, store, ns1Coll2Blk1Tx2)) // expired still missing
	require.True(testDataKeyExists(t, store, ns3Coll2Blk1Tx2))  // never expires

	// COMMIT BLOCK 4 WITH NO PVTDATA
	require.NoError(store.Commit(4, nil, nil))

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
	require.NoError(err)

	ns1Coll2Blk1Tx1 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns2Coll2Blk1Tx1 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns1Coll2Blk1Tx2 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}
	ns3Coll2Blk1Tx2 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, txNum: 2}

	require.False(testDataKeyExists(t, store, ns1Coll2Blk1Tx1)) // purged
	require.False(testDataKeyExists(t, store, ns2Coll2Blk1Tx1)) // purged
	require.False(testDataKeyExists(t, store, ns1Coll2Blk1Tx2)) // purged
	require.True(testDataKeyExists(t, store, ns3Coll2Blk1Tx2))  // never expires
}

func TestExpiryDataNotIncluded(t *testing.T) {
	ledgerid := "TestExpiryDataNotIncluded"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 1,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 2,
			{"ns-3", "coll-1"}: 1,
			{"ns-3", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, ledgerid, btlPolicy, pvtDataConf())
	defer env.Cleanup()
	require := require.New(t)
	store := env.TestStore

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtDataMap)
	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	// ineligible missing data in tx4
	blk1MissingData.Add(4, "ns-3", "coll-1", false)
	blk1MissingData.Add(4, "ns-3", "coll-2", false)

	// construct missing data for block 2
	blk2MissingData := make(ledger.TxMissingPvtDataMap)
	// eligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-1", true)
	blk2MissingData.Add(1, "ns-1", "coll-2", true)

	// no pvt data with block 0
	require.NoError(store.Commit(0, nil, nil))

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	require.NoError(store.Commit(1, testDataForBlk1, blk1MissingData))

	// write pvt data for block 2
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	require.NoError(store.Commit(2, testDataForBlk2, blk2MissingData))

	retrievedData, _ := store.GetPvtDataByBlockNum(1, nil)
	// block 1 data should still be not expired
	for i, data := range retrievedData {
		require.Equal(data.SeqInBlock, testDataForBlk1[i].SeqInBlock)
		require.True(proto.Equal(data.WriteSet, testDataForBlk1[i].WriteSet))
	}

	// none of the missing data entries would have expired
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Commit block 3 with no pvtdata
	require.NoError(store.Commit(3, nil, nil))

	// After committing block 3, the data for "ns-1:coll1" of block 1 should have expired and should not be returned by the store
	expectedPvtdataFromBlock1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	require.Equal(expectedPvtdataFromBlock1, retrievedData)

	// After committing block 3, the missing data of "ns1-coll1" in block1-tx1 should have expired
	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Commit block 4 with no pvtdata
	require.NoError(store.Commit(4, nil, nil))

	// After committing block 4, the data for "ns-2:coll2" of block 1 should also have expired and should not be returned by the store
	expectedPvtdataFromBlock1 = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	require.Equal(expectedPvtdataFromBlock1, retrievedData)

	// Now, for block 2, "ns-1:coll1" should also have expired
	expectedPvtdataFromBlock2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(2, nil)
	require.Equal(expectedPvtdataFromBlock2, retrievedData)

	// After committing block 4, the missing data of "ns1-coll1" in block2-tx1 should have expired
	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

}

func TestStorePurge(t *testing.T) {
	ledgerid := "TestStorePurge"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 1,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 4,
			{"ns-3", "coll-1"}: 1,
			{"ns-3", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, ledgerid, btlPolicy, pvtDataConf())
	defer env.Cleanup()
	require := require.New(t)
	s := env.TestStore

	// no pvt data with block 0
	require.NoError(s.Commit(0, nil, nil))

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtDataMap)
	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	// ineligible missing data in tx4
	blk1MissingData.Add(4, "ns-3", "coll-1", false)
	blk1MissingData.Add(4, "ns-3", "coll-2", false)

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	require.NoError(s.Commit(1, testDataForBlk1, blk1MissingData))

	// write pvt data for block 2
	require.NoError(s.Commit(2, nil, nil))
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store
	ns1Coll1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns2Coll2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 2}

	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	ns1Coll1elgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, isEligible: true}
	ns1Coll2elgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, isEligible: true}

	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	ns3Coll1inelgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-1", blkNum: 1}, isEligible: false}
	ns3Coll2inelgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, isEligible: false}

	testWaitForPurgerRoutineToFinish(s)
	require.True(testDataKeyExists(t, s, ns1Coll1))
	require.True(testDataKeyExists(t, s, ns2Coll2))

	require.True(testMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(testMissingDataKeyExists(t, s, ns1Coll2elgMD))

	require.True(testMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	require.True(testMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 3
	require.NoError(s.Commit(3, nil, nil))
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store (because purger should not be launched at block 3)
	testWaitForPurgerRoutineToFinish(s)
	require.True(testDataKeyExists(t, s, ns1Coll1))
	require.True(testDataKeyExists(t, s, ns2Coll2))
	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	require.True(testMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(testMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	require.True(testMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	require.True(testMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 4
	require.NoError(s.Commit(4, nil, nil))
	// data for ns-1:coll-1 should not exist in store (because purger should be launched at block 4)
	// but ns-2:coll-2 should exist because it expires at block 5
	testWaitForPurgerRoutineToFinish(s)
	require.False(testDataKeyExists(t, s, ns1Coll1))
	require.True(testDataKeyExists(t, s, ns2Coll2))
	// eligible missingData entries for ns-1:coll-1 should have expired and ns-1:coll-2 (neverExpires) should exist in store
	require.False(testMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(testMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// ineligible missingData entries for ns-3:col-1 should have expired and ns-3:coll-2 (neverExpires) should exist in store
	require.False(testMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	require.True(testMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 5
	require.NoError(s.Commit(5, nil, nil))
	// ns-2:coll-2 should exist because though the data expires at block 5 but purger is launched every second block
	testWaitForPurgerRoutineToFinish(s)
	require.False(testDataKeyExists(t, s, ns1Coll1))
	require.True(testDataKeyExists(t, s, ns2Coll2))

	// write pvt data for block 6
	require.NoError(s.Commit(6, nil, nil))
	// ns-2:coll-2 should not exists now (because purger should be launched at block 6)
	testWaitForPurgerRoutineToFinish(s)
	require.False(testDataKeyExists(t, s, ns1Coll1))
	require.False(testDataKeyExists(t, s, ns2Coll2))

	// "ns-2:coll-1" should never have been purged (because, it was no btl was declared for this)
	require.True(testDataKeyExists(t, s, &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}))
}

func TestStoreState(t *testing.T) {
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, "TestStoreState", btlPolicy, pvtDataConf())
	defer env.Cleanup()
	require := require.New(t)
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 0, []string{"ns-1:coll-1", "ns-1:coll-2"}),
	}
	_, ok := store.Commit(1, testData, nil).(*ErrIllegalArgs)
	require.True(ok)
}

func TestPendingBatch(t *testing.T) {
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, "TestPendingBatch", btlPolicy, pvtDataConf())
	defer env.Cleanup()
	require := require.New(t)
	s := env.TestStore
	existingLastBlockNum := uint64(25)
	batch := leveldbhelper.NewUpdateBatch()
	batch.Put(lastCommittedBlkkey, encodeLastCommittedBlockVal(existingLastBlockNum))
	require.NoError(s.db.WriteBatch(batch, true))
	s.lastCommittedBlock = existingLastBlockNum
	s.isEmpty = false
	testLastCommittedBlockHeight(existingLastBlockNum+1, require, s)

	// assume that a block has been prepared in v142 and the peer was
	// killed for upgrade. When the pvtdataStore is opened again with
	// v2.0 peer, the pendingBatch should be marked as committed.
	batch = leveldbhelper.NewUpdateBatch()

	// store pvtData entries
	dataKey := &dataKey{nsCollBlk{"ns-1", "coll-1", 26}, 1}
	dataValue := &rwset.CollectionPvtReadWriteSet{CollectionName: "coll-1", Rwset: []byte("pvtdata")}
	keyBytes := encodeDataKey(dataKey)
	valueBytes, err := encodeDataValue(dataValue)
	require.NoError(err)
	batch.Put(keyBytes, valueBytes)

	// store pendingBatch marker
	batch.Put(pendingCommitKey, emptyValue)

	// write to the store
	require.NoError(s.db.WriteBatch(batch, true))
	testLastCommittedBlockHeight(existingLastBlockNum+1, require, s)

	// as the block commit is pending, we cannot read the pvtData
	hasPendingBatch, err := s.hasPendingCommit()
	require.NoError(err)
	require.Equal(true, hasPendingBatch)
	pvtData, err := s.GetPvtDataByBlockNum(26, nil)
	_, ok := err.(*ErrOutOfRange)
	require.True(ok)
	require.Nil(pvtData)

	// emulate a version upgrade
	env.CloseAndReopen()

	s = env.TestStore
	testLastCommittedBlockHeight(existingLastBlockNum+2, require, s)
	hasPendingBatch, err = s.hasPendingCommit()
	require.NoError(err)
	require.Equal(false, hasPendingBatch)
	testDataKeyExists(t, s, dataKey)

	expectedPvtData := &rwset.TxPvtReadWriteSet{
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns-1",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					dataValue,
				},
			},
		},
	}
	pvtData, err = s.GetPvtDataByBlockNum(26, nil)
	require.NoError(err)
	require.Equal(1, len(pvtData))
	require.Equal(uint64(1), pvtData[0].SeqInBlock)
	require.True(proto.Equal(expectedPvtData, pvtData[0].WriteSet))
}

func TestCollElgEnabled(t *testing.T) {
	conf := pvtDataConf()
	testCollElgEnabled(t, conf)
	conf.BatchesInterval = 1
	conf.MaxBatchSize = 1
	testCollElgEnabled(t, conf)
}

func testCollElgEnabled(t *testing.T, conf *PrivateDataConfig) {
	ledgerid := "TestCollElgEnabled"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, ledgerid, btlPolicy, conf)
	defer env.Cleanup()
	require := require.New(t)
	testStore := env.TestStore

	// Initial state: eligible for {ns-1:coll-1 and ns-2:coll-1 }

	// no pvt data with block 0
	require.NoError(testStore.Commit(0, nil, nil))

	// construct and commit block 1
	blk1MissingData := make(ledger.TxMissingPvtDataMap)
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-2", "coll-1", true)
	blk1MissingData.Add(4, "ns-1", "coll-2", false)
	blk1MissingData.Add(4, "ns-2", "coll-2", false)
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1"}),
	}
	require.NoError(testStore.Commit(1, testDataForBlk1, blk1MissingData))

	// construct and commit block 2
	blk2MissingData := make(ledger.TxMissingPvtDataMap)
	// ineligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-2", false)
	blk2MissingData.Add(1, "ns-2", "coll-2", false)
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1"}),
	}
	require.NoError(testStore.Commit(2, testDataForBlk2, blk2MissingData))

	// Retrieve and verify missing data reported
	// Expected missing data should be only blk1-tx1 (because, the other missing data is marked as ineliigible)
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	missingPvtDataInfo, err := testStore.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Enable eligibility for {ns-1:coll2}
	testStore.ProcessCollsEligibilityEnabled(
		5,
		map[string][]string{
			"ns-1": {"coll-2"},
		},
	)
	testutilWaitForCollElgProcToFinish(testStore)

	// Retrieve and verify missing data reported
	// Expected missing data should include newly eiligible collections
	expectedMissingPvtDataInfo.Add(1, 4, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	missingPvtDataInfo, err = testStore.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Enable eligibility for {ns-2:coll2}
	testStore.ProcessCollsEligibilityEnabled(6,
		map[string][]string{
			"ns-2": {"coll-2"},
		},
	)
	testutilWaitForCollElgProcToFinish(testStore)

	// Retrieve and verify missing data reported
	// Expected missing data should include newly eiligible collections
	expectedMissingPvtDataInfo.Add(1, 4, "ns-2", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-2", "coll-2")
	missingPvtDataInfo, err = testStore.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(err)
	require.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)
}

func testLastCommittedBlockHeight(expectedBlockHt uint64, require *require.Assertions, store *Store) {
	blkHt, err := store.LastCommittedBlockHeight()
	require.NoError(err)
	require.Equal(expectedBlockHt, blkHt)
}

func testDataKeyExists(t *testing.T, s *Store, dataKey *dataKey) bool {
	dataKeyBytes := encodeDataKey(dataKey)
	val, err := s.db.Get(dataKeyBytes)
	require.NoError(t, err)
	return len(val) != 0
}

func testMissingDataKeyExists(t *testing.T, s *Store, missingDataKey *missingDataKey) bool {
	dataKeyBytes := encodeMissingDataKey(missingDataKey)
	val, err := s.db.Get(dataKeyBytes)
	require.NoError(t, err)
	return len(val) != 0
}

func testWaitForPurgerRoutineToFinish(s *Store) {
	time.Sleep(1 * time.Second)
	s.purgerLock.Lock()
	s.purgerLock.Unlock()
}

func testutilWaitForCollElgProcToFinish(s *Store) {
	s.collElgProcSync.waitForDone()
}

func produceSamplePvtdata(t *testing.T, txNum uint64, nsColls []string) *ledger.TxPvtData {
	builder := rwsetutil.NewRWSetBuilder()
	for _, nsColl := range nsColls {
		nsCollSplit := strings.Split(nsColl, ":")
		ns := nsCollSplit[0]
		coll := nsCollSplit[1]
		builder.AddToPvtAndHashedWriteSet(ns, coll, fmt.Sprintf("key-%s-%s", ns, coll), []byte(fmt.Sprintf("value-%s-%s", ns, coll)))
	}
	simRes, err := builder.GetTxSimulationResults()
	require.NoError(t, err)
	return &ledger.TxPvtData{SeqInBlock: txNum, WriteSet: simRes.PvtSimulationResults}
}
