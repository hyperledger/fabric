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

	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("pvtdatastorage", "debug")
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
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns-1", "coll-1", 0)
	cs.SetBTL("ns-1", "coll-2", 0)
	cs.SetBTL("ns-2", "coll-1", 0)
	cs.SetBTL("ns-2", "coll-2", 0)
	cs.SetBTL("ns-3", "coll-1", 0)
	cs.SetBTL("ns-4", "coll-1", 0)
	cs.SetBTL("ns-4", "coll-2", 0)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)
	env := NewTestStoreEnv(t, "TestStoreBasicCommitAndRetrieval", btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// construct missing data for block 1
	blk1MissingData := &ledger.MissingPrivateDataList{}

	// eligible missing data in tx1
	blk1MissingData.Add("tx1", 1, "ns-1", "coll-1", true)
	blk1MissingData.Add("tx1", 1, "ns-1", "coll-2", true)
	blk1MissingData.Add("tx1", 1, "ns-2", "coll-1", true)
	blk1MissingData.Add("tx1", 1, "ns-2", "coll-2", true)
	// eligible missing data in tx2
	blk1MissingData.Add("tx2", 2, "ns-3", "coll-1", true)
	// ineligible missing data in tx4
	blk1MissingData.Add("tx4", 4, "ns-4", "coll-1", false)
	blk1MissingData.Add("tx4", 4, "ns-4", "coll-2", false)

	// construct missing data for block 2
	blk2MissingData := &ledger.MissingPrivateDataList{}
	// eligible missing data in tx1
	blk2MissingData.Add("tx1", 1, "ns-1", "coll-1", true)
	blk2MissingData.Add("tx1", 3, "ns-1", "coll-1", true)
	blk2MissingData.Add("tx1", 1, "ns-1", "coll-2", true)

	// no pvt data with block 0
	assert.NoError(store.Prepare(0, nil, nil))
	assert.NoError(store.Commit())

	// pvt data with block 1 - commit
	assert.NoError(store.Prepare(1, testData, blk1MissingData))
	assert.NoError(store.Commit())

	// pvt data with block 2 - rollback
	assert.NoError(store.Prepare(2, testData, nil))
	assert.NoError(store.Rollback())

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

func TestExpiryDataNotIncluded(t *testing.T) {
	ledgerid := "TestExpiryDataNotIncluded"
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns-1", "coll-1", 1)
	cs.SetBTL("ns-1", "coll-2", 0)
	cs.SetBTL("ns-2", "coll-1", 0)
	cs.SetBTL("ns-2", "coll-2", 2)
	cs.SetBTL("ns-3", "coll-1", 1)
	cs.SetBTL("ns-3", "coll-2", 0)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore

	// construct missing data for block 1
	blk1MissingData := &ledger.MissingPrivateDataList{}
	// eligible missing data in tx1
	blk1MissingData.Add("tx1", 1, "ns-1", "coll-1", true)
	blk1MissingData.Add("tx1", 1, "ns-1", "coll-2", true)
	// ineligible missing data in tx4
	blk1MissingData.Add("tx4", 4, "ns-3", "coll-1", false)
	blk1MissingData.Add("tx4", 4, "ns-3", "coll-2", false)

	// construct missing data for block 2
	blk2MissingData := &ledger.MissingPrivateDataList{}
	// eligible missing data in tx1
	blk2MissingData.Add("tx1", 1, "ns-1", "coll-1", true)
	blk2MissingData.Add("tx1", 1, "ns-1", "coll-2", true)

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
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns-1", "coll-1", 1)
	cs.SetBTL("ns-1", "coll-2", 0)
	cs.SetBTL("ns-2", "coll-1", 0)
	cs.SetBTL("ns-2", "coll-2", 4)
	cs.SetBTL("ns-3", "coll-1", 1)
	cs.SetBTL("ns-3", "coll-2", 0)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	s := env.TestStore

	// no pvt data with block 0
	assert.NoError(s.Prepare(0, nil, nil))
	assert.NoError(s.Commit())

	// construct missing data for block 1
	blk1MissingData := &ledger.MissingPrivateDataList{}
	// eligible missing data in tx1
	blk1MissingData.Add("tx1", 1, "ns-1", "coll-1", true)
	blk1MissingData.Add("tx1", 1, "ns-1", "coll-2", true)
	// ineligible missing data in tx4
	blk1MissingData.Add("tx4", 4, "ns-3", "coll-1", false)
	blk1MissingData.Add("tx4", 4, "ns-3", "coll-2", false)

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
	ns1_coll1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns2_coll2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 2}

	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	ns1_coll1_elgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, isEligible: true}
	ns1_coll2_elgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, isEligible: true}

	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	ns3_coll1_inelgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-1", blkNum: 1}, isEligible: false}
	ns3_coll2_inelgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}, isEligible: false}

	testWaitForPurgerRoutineToFinish(s)
	assert.True(testDataKeyExists(t, s, ns1_coll1))
	assert.True(testDataKeyExists(t, s, ns2_coll2))

	assert.True(testMissingDataKeyExists(t, s, ns1_coll1_elgMD))
	assert.True(testMissingDataKeyExists(t, s, ns1_coll2_elgMD))

	assert.True(testMissingDataKeyExists(t, s, ns3_coll1_inelgMD))
	assert.True(testMissingDataKeyExists(t, s, ns3_coll2_inelgMD))

	// write pvt data for block 3
	assert.NoError(s.Prepare(3, nil, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store (because purger should not be launched at block 3)
	testWaitForPurgerRoutineToFinish(s)
	assert.True(testDataKeyExists(t, s, ns1_coll1))
	assert.True(testDataKeyExists(t, s, ns2_coll2))
	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	assert.True(testMissingDataKeyExists(t, s, ns1_coll1_elgMD))
	assert.True(testMissingDataKeyExists(t, s, ns1_coll2_elgMD))
	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	assert.True(testMissingDataKeyExists(t, s, ns3_coll1_inelgMD))
	assert.True(testMissingDataKeyExists(t, s, ns3_coll2_inelgMD))

	// write pvt data for block 4
	assert.NoError(s.Prepare(4, nil, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 should not exist in store (because purger should be launched at block 4)
	// but ns-2:coll-2 should exist because it expires at block 5
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, ns1_coll1))
	assert.True(testDataKeyExists(t, s, ns2_coll2))
	// eligible missingData entries for ns-1:coll-1 should have expired and ns-1:coll-2 (neverExpires) should exist in store
	assert.False(testMissingDataKeyExists(t, s, ns1_coll1_elgMD))
	assert.True(testMissingDataKeyExists(t, s, ns1_coll2_elgMD))
	// ineligible missingData entries for ns-3:col-1 should have expired and ns-3:coll-2 (neverExpires) should exist in store
	assert.False(testMissingDataKeyExists(t, s, ns3_coll1_inelgMD))
	assert.True(testMissingDataKeyExists(t, s, ns3_coll2_inelgMD))

	// write pvt data for block 5
	assert.NoError(s.Prepare(5, nil, nil))
	assert.NoError(s.Commit())
	// ns-2:coll-2 should exist because though the data expires at block 5 but purger is launched every second block
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, ns1_coll1))
	assert.True(testDataKeyExists(t, s, ns2_coll2))

	// write pvt data for block 6
	assert.NoError(s.Prepare(6, nil, nil))
	assert.NoError(s.Commit())
	// ns-2:coll-2 should not exists now (because purger should be launched at block 6)
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, ns1_coll1))
	assert.False(testDataKeyExists(t, s, ns2_coll2))

	// "ns-2:coll-1" should never have been purged (because, it was no btl was declared for this)
	assert.True(testDataKeyExists(t, s, &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}))
}

func TestStoreState(t *testing.T) {
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns-1", "coll-1", 0)
	cs.SetBTL("ns-1", "coll-2", 0)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

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

// TODO Add tests for simulating a crash between calls `Prepare` and `Commit`/`Rollback`

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
