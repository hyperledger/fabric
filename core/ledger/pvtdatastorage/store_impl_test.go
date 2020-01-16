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
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("pvtdatastorage=debug")
	viper.Set("peer.fileSystemPath", "/tmp/fabric/core/ledger/pvtdatastorage")
	os.Exit(m.Run())
}

func TestEmptyStore(t *testing.T) {
	env := NewTestStoreEnv(t, "TestEmptyStore", nil)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore
	testEmpty(true, assert, store)
	testPendingBatch(false, assert, store)
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

	env := NewTestStoreEnv(t, "TestStoreBasicCommitAndRetrieval", btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
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
	assert.NoError(store.Prepare(0, nil, nil))
	assert.NoError(store.Commit())

	// pvt data with block 1 - commit
	assert.NoError(store.Prepare(1, testData, blk1MissingData))
	assert.NoError(store.Commit())

	// pvt data retrieval for block 0 should return nil
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(0, nilFilter)
	assert.NoError(err)
	assert.Nil(retrievedData)

	// pvt data retrieval for block 1 should return full pvtdata
	retrievedData, err = store.GetPvtDataByBlockNum(1, nilFilter)
	assert.NoError(err)
	for i, data := range retrievedData {
		assert.Equal(data.SeqInBlock, testData[i].SeqInBlock)
		assert.True(proto.Equal(data.WriteSet, testData[i].WriteSet))
	}

	// pvt data retrieval for block 1 with filter should return filtered pvtdata
	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	filter.Add("ns-2", "coll-2")
	retrievedData, err = store.GetPvtDataByBlockNum(1, filter)
	expectedRetrievedData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-2:coll-2"}),
	}
	for i, data := range retrievedData {
		assert.Equal(data.SeqInBlock, expectedRetrievedData[i].SeqInBlock)
		assert.True(proto.Equal(data.WriteSet, expectedRetrievedData[i].WriteSet))
	}

	// pvt data retrieval for block 2 should return ErrOutOfRange
	retrievedData, err = store.GetPvtDataByBlockNum(2, nilFilter)
	_, ok := err.(*ErrOutOfRange)
	assert.True(ok)
	assert.Nil(retrievedData)

	// pvt data with block 2 - commit
	assert.NoError(store.Prepare(2, testData, blk2MissingData))
	assert.NoError(store.Commit())

	// retrieve the stored missing entries using GetMissingPvtDataInfoForMostRecentBlocks
	// Only the code path of eligible entries would be covered in this unit-test. For
	// ineligible entries, the code path will be covered in FAB-11437

	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 3, "ns-1", "coll-1")

	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(1)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-2")

	// missing data in block1, tx2
	expectedMissingPvtDataInfo.Add(1, 2, "ns-3", "coll-1")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(2)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)
}

func TestCommitPvtDataOfOldBlocks(t *testing.T) {
	viper.Set("ledger.pvtdataStore.purgeInterval", 2)
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
	env := NewTestStoreEnv(t, "TestCommitPvtDataOfOldBlocks", btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
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
	assert.NoError(store.Prepare(0, nil, nil))
	assert.NoError(store.Commit())

	// COMMIT BLOCK 1 WITH PVTDATA AND MISSINGDATA
	assert.NoError(store.Prepare(1, testData, blk1MissingData))
	assert.NoError(store.Commit())

	// COMMIT BLOCK 2 WITH PVTDATA AND MISSINGDATA
	assert.NoError(store.Prepare(2, nil, blk2MissingData))
	assert.NoError(store.Commit())

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
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

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
	assert.NoError(err)

	// ENSURE THAT THE PREVIOUSLY MISSING PVTDATA OF BLOCK 1 & 2 EXIST IN THE STORE
	ns1Coll1Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 1}
	ns2Coll1Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-1", blkNum: 1}, txNum: 1}
	ns1Coll1Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns3Coll1Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns1Coll1Blk2Tx3 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 2}, txNum: 3}

	assert.True(testDataKeyExists(t, store, ns1Coll1Blk1Tx1))
	assert.True(testDataKeyExists(t, store, ns2Coll1Blk1Tx1))
	assert.True(testDataKeyExists(t, store, ns1Coll1Blk1Tx2))
	assert.True(testDataKeyExists(t, store, ns3Coll1Blk1Tx2))
	assert.True(testDataKeyExists(t, store, ns1Coll1Blk2Tx3))

	// pvt data retrieval for block 2 should return the just committed pvtdata
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(2, nilFilter)
	assert.NoError(err)
	for i, data := range retrievedData {
		assert.Equal(data.SeqInBlock, oldBlocksPvtData[2][i].SeqInBlock)
		assert.True(proto.Equal(data.WriteSet, oldBlocksPvtData[2][i].WriteSet))
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
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// blksPvtData returns all the pvt data for a block for which the any pvtdata has been submitted
	// using CommitPvtDataOfOldBlocks
	blksPvtData, err := store.GetLastUpdatedOldBlocksPvtData()
	assert.NoError(err)

	expectedLastupdatedPvtdata := make(map[uint64][]*ledger.TxPvtData)
	expectedLastupdatedPvtdata[1] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 1, []string{"ns-1:coll-1", "ns-2:coll-1"}),
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-2:coll-1", "ns-2:coll-2", "ns-3:coll-1"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	expectedLastupdatedPvtdata[2] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1"}),
	}
	assert.Equal(expectedLastupdatedPvtdata, blksPvtData)

	err = store.ResetLastUpdatedOldBlocksList()
	assert.NoError(err)

	blksPvtData, err = store.GetLastUpdatedOldBlocksPvtData()
	assert.NoError(err)
	assert.Nil(blksPvtData)

	// COMMIT BLOCK 3 WITH NO PVTDATA
	assert.NoError(store.Prepare(3, nil, nil))
	assert.NoError(store.Commit())

	// IN BLOCK 1, NS-1:COLL-2 AND NS-2:COLL-2 SHOULD HAVE EXPIRED BUT NOT PURGED
	// HENCE, THE FOLLOWING COMMIT SHOULD CREATE ENTRIES IN THE STORE
	oldBlocksPvtData = make(map[uint64][]*ledger.TxPvtData)
	oldBlocksPvtData[1] = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 1, []string{"ns-1:coll-2"}), // though expired, it
		// would get committed to the store as it is not purged yet
		produceSamplePvtdata(t, 2, []string{"ns-3:coll-2"}), // never expires
	}

	err = store.CommitPvtDataOfOldBlocks(oldBlocksPvtData)
	assert.NoError(err)

	ns1Coll2Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns2Coll2Blk1Tx1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns1Coll2Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}
	ns3Coll2Blk1Tx2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, txNum: 2}

	// though the pvtdata are expired but not purged yet, we do
	// commit the data and hence the entries would exist in the
	// store
	assert.True(testDataKeyExists(t, store, ns1Coll2Blk1Tx1))  // expired but committed
	assert.False(testDataKeyExists(t, store, ns2Coll2Blk1Tx1)) // expired but still missing
	assert.False(testDataKeyExists(t, store, ns1Coll2Blk1Tx2)) // expired still missing
	assert.True(testDataKeyExists(t, store, ns3Coll2Blk1Tx2))  // never expires

	err = store.ResetLastUpdatedOldBlocksList()
	assert.NoError(err)

	// COMMIT BLOCK 4 WITH NO PVTDATA
	assert.NoError(store.Prepare(4, nil, nil))
	assert.NoError(store.Commit())

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
	assert.NoError(err)

	ns1Coll2Blk1Tx1 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns2Coll2Blk1Tx1 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 1}
	ns1Coll2Blk1Tx2 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}
	ns3Coll2Blk1Tx2 = &dataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, txNum: 2}

	assert.False(testDataKeyExists(t, store, ns1Coll2Blk1Tx1)) // purged
	assert.False(testDataKeyExists(t, store, ns2Coll2Blk1Tx1)) // purged
	assert.False(testDataKeyExists(t, store, ns1Coll2Blk1Tx2)) // purged
	assert.True(testDataKeyExists(t, store, ns3Coll2Blk1Tx2))  // never expires
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
	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
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
	assert.NoError(store.Prepare(0, nil, nil))
	assert.NoError(store.Commit())

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	assert.NoError(store.Prepare(1, testDataForBlk1, blk1MissingData))
	assert.NoError(store.Commit())

	// write pvt data for block 2
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	assert.NoError(store.Prepare(2, testDataForBlk2, blk2MissingData))
	assert.NoError(store.Commit())

	retrievedData, _ := store.GetPvtDataByBlockNum(1, nil)
	// block 1 data should still be not expired
	for i, data := range retrievedData {
		assert.Equal(data.SeqInBlock, testDataForBlk1[i].SeqInBlock)
		assert.True(proto.Equal(data.WriteSet, testDataForBlk1[i].WriteSet))
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
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Commit block 3 with no pvtdata
	assert.NoError(store.Prepare(3, nil, nil))
	assert.NoError(store.Commit())

	// After committing block 3, the data for "ns-1:coll1" of block 1 should have expired and should not be returned by the store
	expectedPvtdataFromBlock1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	assert.Equal(expectedPvtdataFromBlock1, retrievedData)

	// After committing block 3, the missing data of "ns1-coll1" in block1-tx1 should have expired
	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Commit block 4 with no pvtdata
	assert.NoError(store.Prepare(4, nil, nil))
	assert.NoError(store.Commit())

	// After committing block 4, the data for "ns-2:coll2" of block 1 should also have expired and should not be returned by the store
	expectedPvtdataFromBlock1 = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	assert.Equal(expectedPvtdataFromBlock1, retrievedData)

	// Now, for block 2, "ns-1:coll1" should also have expired
	expectedPvtdataFromBlock2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(2, nil)
	assert.Equal(expectedPvtdataFromBlock2, retrievedData)

	// After committing block 4, the missing data of "ns1-coll1" in block2-tx1 should have expired
	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

}

func TestStorePurge(t *testing.T) {
	ledgerid := "TestStorePurge"
	viper.Set("ledger.pvtdataStore.purgeInterval", 2)
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
	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	s := env.TestStore

	// no pvt data with block 0
	assert.NoError(s.Prepare(0, nil, nil))
	assert.NoError(s.Commit())

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
	assert.NoError(s.Prepare(1, testDataForBlk1, blk1MissingData))
	assert.NoError(s.Commit())

	// write pvt data for block 2
	assert.NoError(s.Prepare(2, nil, nil))
	assert.NoError(s.Commit())
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
	assert.True(testDataKeyExists(t, s, ns1Coll1))
	assert.True(testDataKeyExists(t, s, ns2Coll2))

	assert.True(testMissingDataKeyExists(t, s, ns1Coll1elgMD))
	assert.True(testMissingDataKeyExists(t, s, ns1Coll2elgMD))

	assert.True(testMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	assert.True(testMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 3
	assert.NoError(s.Prepare(3, nil, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store (because purger should not be launched at block 3)
	testWaitForPurgerRoutineToFinish(s)
	assert.True(testDataKeyExists(t, s, ns1Coll1))
	assert.True(testDataKeyExists(t, s, ns2Coll2))
	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	assert.True(testMissingDataKeyExists(t, s, ns1Coll1elgMD))
	assert.True(testMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	assert.True(testMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	assert.True(testMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 4
	assert.NoError(s.Prepare(4, nil, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 should not exist in store (because purger should be launched at block 4)
	// but ns-2:coll-2 should exist because it expires at block 5
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, ns1Coll1))
	assert.True(testDataKeyExists(t, s, ns2Coll2))
	// eligible missingData entries for ns-1:coll-1 should have expired and ns-1:coll-2 (neverExpires) should exist in store
	assert.False(testMissingDataKeyExists(t, s, ns1Coll1elgMD))
	assert.True(testMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// ineligible missingData entries for ns-3:col-1 should have expired and ns-3:coll-2 (neverExpires) should exist in store
	assert.False(testMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	assert.True(testMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 5
	assert.NoError(s.Prepare(5, nil, nil))
	assert.NoError(s.Commit())
	// ns-2:coll-2 should exist because though the data expires at block 5 but purger is launched every second block
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, ns1Coll1))
	assert.True(testDataKeyExists(t, s, ns2Coll2))

	// write pvt data for block 6
	assert.NoError(s.Prepare(6, nil, nil))
	assert.NoError(s.Commit())
	// ns-2:coll-2 should not exists now (because purger should be launched at block 6)
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, ns1Coll1))
	assert.False(testDataKeyExists(t, s, ns2Coll2))

	// "ns-2:coll-1" should never have been purged (because, it was no btl was declared for this)
	assert.True(testDataKeyExists(t, s, &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}))
}

func TestStoreState(t *testing.T) {
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, "TestStoreState", btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 0, []string{"ns-1:coll-1", "ns-1:coll-2"}),
	}
	_, ok := store.Prepare(1, testData, nil).(*ErrIllegalArgs)
	assert.True(ok)

	assert.Nil(store.Prepare(0, testData, nil))
	assert.NoError(store.Commit())

	assert.Nil(store.Prepare(1, testData, nil))
	_, ok = store.Prepare(2, testData, nil).(*ErrIllegalCall)
	assert.True(ok)
}

func TestInitLastCommittedBlock(t *testing.T) {
	env := NewTestStoreEnv(t, "TestStoreState", nil)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore
	existingLastBlockNum := uint64(25)
	assert.NoError(store.InitLastCommittedBlock(existingLastBlockNum))

	testEmpty(false, assert, store)
	testPendingBatch(false, assert, store)
	testLastCommittedBlockHeight(existingLastBlockNum+1, assert, store)

	env.CloseAndReopen()
	testEmpty(false, assert, store)
	testPendingBatch(false, assert, store)
	testLastCommittedBlockHeight(existingLastBlockNum+1, assert, store)

	err := store.InitLastCommittedBlock(30)
	_, ok := err.(*ErrIllegalCall)
	assert.True(ok)
}

func TestCollElgEnabled(t *testing.T) {
	testCollElgEnabled(t)
	defaultValBatchSize := ledgerconfig.GetPvtdataStoreCollElgProcMaxDbBatchSize()
	defaultValInterval := ledgerconfig.GetPvtdataStoreCollElgProcDbBatchesInterval()
	defer func() {
		viper.Set("ledger.pvtdataStore.collElgProcMaxDbBatchSize", defaultValBatchSize)
		viper.Set("ledger.pvtdataStore.collElgProcMaxDbBatchSize", defaultValInterval)
	}()
	viper.Set("ledger.pvtdataStore.collElgProcMaxDbBatchSize", 1)
	viper.Set("ledger.pvtdataStore.collElgProcDbBatchesInterval", 1)
	testCollElgEnabled(t)
}

func testCollElgEnabled(t *testing.T) {
	ledgerid := "TestCollElgEnabled"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 0,
		},
	)
	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore

	// Initial state: eligible for {ns-1:coll-1 and ns-2:coll-1 }

	// no pvt data with block 0
	assert.NoError(store.Prepare(0, nil, nil))
	assert.NoError(store.Commit())

	// construct and commit block 1
	blk1MissingData := make(ledger.TxMissingPvtDataMap)
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-2", "coll-1", true)
	blk1MissingData.Add(4, "ns-1", "coll-2", false)
	blk1MissingData.Add(4, "ns-2", "coll-2", false)
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1"}),
	}
	assert.NoError(store.Prepare(1, testDataForBlk1, blk1MissingData))
	assert.NoError(store.Commit())

	// construct and commit block 2
	blk2MissingData := make(ledger.TxMissingPvtDataMap)
	// ineligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-2", false)
	blk2MissingData.Add(1, "ns-2", "coll-2", false)
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1"}),
	}
	assert.NoError(store.Prepare(2, testDataForBlk2, blk2MissingData))
	assert.NoError(store.Commit())

	// Retrieve and verify missing data reported
	// Expected missing data should be only blk1-tx1 (because, the other missing data is marked as ineliigible)
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Enable eligibility for {ns-1:coll2}
	store.ProcessCollsEligibilityEnabled(
		5,
		map[string][]string{
			"ns-1": {"coll-2"},
		},
	)
	testutilWaitForCollElgProcToFinish(store)

	// Retrieve and verify missing data reported
	// Expected missing data should include newly eiligible collections
	expectedMissingPvtDataInfo.Add(1, 4, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	assert.NoError(err)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Enable eligibility for {ns-2:coll2}
	store.ProcessCollsEligibilityEnabled(6,
		map[string][]string{
			"ns-2": {"coll-2"},
		},
	)
	testutilWaitForCollElgProcToFinish(store)

	// Retrieve and verify missing data reported
	// Expected missing data should include newly eiligible collections
	expectedMissingPvtDataInfo.Add(1, 4, "ns-2", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-2", "coll-2")
	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	assert.Equal(expectedMissingPvtDataInfo, missingPvtDataInfo)
}

// TODO Add tests for simulating a crash between calls `Prepare` and `Commit` - [FAB-13099]

func testEmpty(expectedEmpty bool, assert *assert.Assertions, store Store) {
	isEmpty, err := store.IsEmpty()
	assert.NoError(err)
	assert.Equal(expectedEmpty, isEmpty)
}

func testPendingBatch(expectedPending bool, assert *assert.Assertions, store Store) {
	hasPendingBatch, err := store.HasPendingBatch()
	assert.NoError(err)
	assert.Equal(expectedPending, hasPendingBatch)
}

func testLastCommittedBlockHeight(expectedBlockHt uint64, assert *assert.Assertions, store Store) {
	blkHt, err := store.LastCommittedBlockHeight()
	assert.NoError(err)
	assert.Equal(expectedBlockHt, blkHt)
}

func testDataKeyExists(t *testing.T, s Store, dataKey *dataKey) bool {
	dataKeyBytes := encodeDataKey(dataKey)
	val, err := s.(*store).db.Get(dataKeyBytes)
	assert.NoError(t, err)
	return len(val) != 0
}

func testMissingDataKeyExists(t *testing.T, s Store, missingDataKey *missingDataKey) bool {
	dataKeyBytes := encodeMissingDataKey(missingDataKey)
	val, err := s.(*store).db.Get(dataKeyBytes)
	assert.NoError(t, err)
	return len(val) != 0
}

func testWaitForPurgerRoutineToFinish(s Store) {
	time.Sleep(1 * time.Second)
	s.(*store).purgerLock.Lock()
	s.(*store).purgerLock.Unlock()
}

func testutilWaitForCollElgProcToFinish(s Store) {
	s.(*store).collElgProcSync.waitForDone()
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
	assert.NoError(t, err)
	return &ledger.TxPvtData{SeqInBlock: txNum, WriteSet: simRes.PvtSimulationResults}
}
