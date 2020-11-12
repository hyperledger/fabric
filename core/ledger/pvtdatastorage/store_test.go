/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestEmptyStore(t *testing.T) {
	env := NewTestStoreEnv(t, "TestEmptyStore", nil, pvtDataConf())
	defer env.Cleanup()
	store := env.TestStore
	require.True(t, store.isEmpty)
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
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtData)

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
	blk2MissingData := make(ledger.TxMissingPvtData)
	// eligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-1", true)
	blk2MissingData.Add(1, "ns-1", "coll-2", true)
	// eligible missing data in tx3
	blk2MissingData.Add(3, "ns-1", "coll-1", true)

	// no pvt data with block 0
	require.NoError(t, store.Commit(0, nil, nil))

	// pvt data with block 1 - commit
	require.NoError(t, store.Commit(1, testData, blk1MissingData))

	// pvt data retrieval for block 0 should return nil
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(0, nilFilter)
	require.NoError(t, err)
	require.Nil(t, retrievedData)

	// pvt data retrieval for block 1 should return full pvtdata
	retrievedData, err = store.GetPvtDataByBlockNum(1, nilFilter)
	require.NoError(t, err)
	for i, data := range retrievedData {
		require.Equal(t, data.SeqInBlock, testData[i].SeqInBlock)
		require.True(t, proto.Equal(data.WriteSet, testData[i].WriteSet))
	}

	// pvt data retrieval for block 1 with filter should return filtered pvtdata
	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	filter.Add("ns-2", "coll-2")
	retrievedData, err = store.GetPvtDataByBlockNum(1, filter)
	require.NoError(t, err)
	expectedRetrievedData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-2:coll-2"}),
	}
	for i, data := range retrievedData {
		require.Equal(t, data.SeqInBlock, expectedRetrievedData[i].SeqInBlock)
		require.True(t, proto.Equal(data.WriteSet, expectedRetrievedData[i].WriteSet))
	}

	// pvt data retrieval for block 2 should return ErrOutOfRange
	retrievedData, err = store.GetPvtDataByBlockNum(2, nilFilter)
	require.EqualError(t, err, "last committed block number [1] smaller than the requested block number [2]")
	require.Nil(t, retrievedData)

	// pvt data with block 2 - commit
	require.NoError(t, store.Commit(2, testData, blk2MissingData))

	// retrieve the stored missing entries using GetMissingPvtDataInfoForMostRecentBlocks
	// Only the code path of eligible entries would be covered in this unit-test. For
	// ineligible entries, the code path will be covered in FAB-11437

	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 3, "ns-1", "coll-1")

	missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(1)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-2")

	// missing data in block1, tx2
	expectedMissingPvtDataInfo.Add(1, 2, "ns-3", "coll-1")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(2)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)
}

func TestStoreIteratorError(t *testing.T) {
	env := NewTestStoreEnv(t, "TestStoreIteratorError", nil, pvtDataConf())
	defer env.Cleanup()
	store := env.TestStore
	require.NoError(t, store.Commit(0, nil, nil))
	env.TestStoreProvider.Close()
	errStr := "internal leveldb error while obtaining db iterator: leveldb: closed"

	t.Run("GetPvtDataByBlockNum", func(t *testing.T) {
		block, err := store.GetPvtDataByBlockNum(0, nil)
		require.EqualError(t, err, errStr)
		require.Nil(t, block)
	})

	t.Run("GetMissingPvtDataInfoForMostRecentBlocks", func(t *testing.T) {
		missingPvtDataInfo, err := store.GetMissingPvtDataInfoForMostRecentBlocks(10)
		require.EqualError(t, err, errStr)
		require.Nil(t, missingPvtDataInfo)
	})

	t.Run("retrieveExpiryEntries", func(t *testing.T) {
		expiryEntries, err := store.retrieveExpiryEntries(0, 1)
		require.EqualError(t, err, errStr)
		require.Nil(t, expiryEntries)
	})

	t.Run("processCollElgEvents", func(t *testing.T) {
		storeDir, err := ioutil.TempDir("", "pdstore")
		require.NoError(t, err)
		s := &Store{}
		dbProvider, err := leveldbhelper.NewProvider(&leveldbhelper.Conf{DBPath: storeDir})
		require.NoError(t, err)
		s.db = dbProvider.GetDBHandle("test-ledger")
		dbProvider.Close()
		require.EqualError(t, s.processCollElgEvents(), errStr)
	})
}

func TestGetMissingDataInfo(t *testing.T) {
	setup := func(ledgerid string, c *PrivateDataConfig) *Store {
		btlPolicy := btltestutil.SampleBTLPolicy(
			map[[2]string]uint64{
				{"ns-1", "coll-1"}: 0,
				{"ns-1", "coll-2"}: 0,
			},
		)

		env := NewTestStoreEnv(t, ledgerid, btlPolicy, c)
		t.Cleanup(
			func() {
				defer env.Cleanup()
			},
		)
		store := env.TestStore

		// construct missing data for block 1
		blk1MissingData := make(ledger.TxMissingPvtData)
		blk1MissingData.Add(1, "ns-1", "coll-1", true)
		blk1MissingData.Add(1, "ns-1", "coll-2", true)

		require.NoError(t, store.Commit(0, nil, nil))
		require.NoError(t, store.Commit(1, nil, blk1MissingData))

		deprioritizedList := ledger.MissingPvtDataInfo{
			1: ledger.MissingBlockPvtdataInfo{
				1: {
					{
						Namespace:  "ns-1",
						Collection: "coll-2",
					},
				},
			},
		}
		require.NoError(t, store.CommitPvtDataOfOldBlocks(nil, deprioritizedList))

		return env.TestStore
	}

	t.Run("always access deprioritized missing data", func(t *testing.T) {
		conf := pvtDataConf()
		conf.DeprioritizedDataReconcilerInterval = 0
		store := setup("testGetMissingDataInfoFromDeprioList", conf)

		expectedDeprioMissingDataInfo := ledger.MissingPvtDataInfo{
			1: ledger.MissingBlockPvtdataInfo{
				1: {
					{
						Namespace:  "ns-1",
						Collection: "coll-2",
					},
				},
			},
		}

		for i := 0; i < 2; i++ {
			assertMissingDataInfo(t, store, expectedDeprioMissingDataInfo, 2)
		}
	})

	t.Run("change the deprioritized missing data access time", func(t *testing.T) {
		conf := pvtDataConf()
		conf.DeprioritizedDataReconcilerInterval = 300 * time.Minute
		store := setup("testGetMissingDataInfoFromPrioAndDeprioList", conf)

		expectedPrioMissingDataInfo := ledger.MissingPvtDataInfo{
			1: ledger.MissingBlockPvtdataInfo{
				1: {
					{
						Namespace:  "ns-1",
						Collection: "coll-1",
					},
				},
			},
		}

		expectedDeprioMissingDataInfo := ledger.MissingPvtDataInfo{
			1: ledger.MissingBlockPvtdataInfo{
				1: {
					{
						Namespace:  "ns-1",
						Collection: "coll-2",
					},
				},
			},
		}

		for i := 0; i < 3; i++ {
			assertMissingDataInfo(t, store, expectedPrioMissingDataInfo, 2)
		}

		store.accessDeprioMissingDataAfter = time.Now().Add(-time.Second)
		lesserThanNextAccessTime := time.Now().Add(store.deprioritizedDataReconcilerInterval).Add(-2 * time.Second)
		greaterThanNextAccessTime := time.Now().Add(store.deprioritizedDataReconcilerInterval).Add(2 * time.Second)
		assertMissingDataInfo(t, store, expectedDeprioMissingDataInfo, 2)

		require.True(t, store.accessDeprioMissingDataAfter.After(lesserThanNextAccessTime))
		require.False(t, store.accessDeprioMissingDataAfter.After(greaterThanNextAccessTime))
		for i := 0; i < 3; i++ {
			assertMissingDataInfo(t, store, expectedPrioMissingDataInfo, 2)
		}
	})

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
	store := env.TestStore

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtData)
	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	// ineligible missing data in tx4
	blk1MissingData.Add(4, "ns-3", "coll-1", false)
	blk1MissingData.Add(4, "ns-3", "coll-2", false)

	// construct missing data for block 2
	blk2MissingData := make(ledger.TxMissingPvtData)
	// eligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-1", true)
	blk2MissingData.Add(1, "ns-1", "coll-2", true)

	// no pvt data with block 0
	require.NoError(t, store.Commit(0, nil, nil))

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	require.NoError(t, store.Commit(1, testDataForBlk1, blk1MissingData))

	// write pvt data for block 2
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	require.NoError(t, store.Commit(2, testDataForBlk2, blk2MissingData))

	retrievedData, _ := store.GetPvtDataByBlockNum(1, nil)
	// block 1 data should still be not expired
	for i, data := range retrievedData {
		require.Equal(t, data.SeqInBlock, testDataForBlk1[i].SeqInBlock)
		require.True(t, proto.Equal(data.WriteSet, testDataForBlk1[i].WriteSet))
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
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Commit block 3 with no pvtdata
	require.NoError(t, store.Commit(3, nil, nil))

	// After committing block 3, the data for "ns-1:coll1" of block 1 should have expired and should not be returned by the store
	expectedPvtdataFromBlock1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	require.Equal(t, expectedPvtdataFromBlock1, retrievedData)

	// After committing block 3, the missing data of "ns1-coll1" in block1-tx1 should have expired
	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Commit block 4 with no pvtdata
	require.NoError(t, store.Commit(4, nil, nil))

	// After committing block 4, the data for "ns-2:coll2" of block 1 should also have expired and should not be returned by the store
	expectedPvtdataFromBlock1 = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	require.Equal(t, expectedPvtdataFromBlock1, retrievedData)

	// Now, for block 2, "ns-1:coll1" should also have expired
	expectedPvtdataFromBlock2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(2, nil)
	require.Equal(t, expectedPvtdataFromBlock2, retrievedData)

	// After committing block 4, the missing data of "ns1-coll1" in block2-tx1 should have expired
	expectedMissingPvtDataInfo = make(ledger.MissingPvtDataInfo)
	// missing data in block2, tx1
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")

	// missing data in block1, tx1
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-2")

	missingPvtDataInfo, err = store.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)
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
	s := env.TestStore

	// no pvt data with block 0
	require.NoError(t, s.Commit(0, nil, nil))

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtData)
	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	// eligible missing data in tx3
	blk1MissingData.Add(3, "ns-1", "coll-1", true)
	blk1MissingData.Add(3, "ns-1", "coll-2", true)
	// ineligible missing data in tx4
	blk1MissingData.Add(4, "ns-3", "coll-1", false)
	blk1MissingData.Add(4, "ns-3", "coll-2", false)

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	require.NoError(t, s.Commit(1, testDataForBlk1, blk1MissingData))

	// write pvt data for block 2
	require.NoError(t, s.Commit(2, nil, nil))
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store
	ns1Coll1 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}, txNum: 2}
	ns2Coll2 := &dataKey{nsCollBlk: nsCollBlk{ns: "ns-2", coll: "coll-2", blkNum: 1}, txNum: 2}

	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	ns1Coll1elgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-1", blkNum: 1}}
	ns1Coll2elgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}}

	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	ns3Coll1inelgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-1", blkNum: 1}}
	ns3Coll2inelgMD := &missingDataKey{nsCollBlk: nsCollBlk{ns: "ns-3", coll: "coll-2", blkNum: 1}}

	testWaitForPurgerRoutineToFinish(s)
	require.True(t, testDataKeyExists(t, s, ns1Coll1))
	require.True(t, testDataKeyExists(t, s, ns2Coll2))

	require.True(t, testElgPrioMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(t, testElgPrioMissingDataKeyExists(t, s, ns1Coll2elgMD))

	require.True(t, testInelgMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	require.True(t, testInelgMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	deprioritizedList := ledger.MissingPvtDataInfo{
		1: ledger.MissingBlockPvtdataInfo{
			3: {
				{
					Namespace:  "ns-1",
					Collection: "coll-1",
				},
				{
					Namespace:  "ns-1",
					Collection: "coll-2",
				},
			},
		},
	}
	require.NoError(t, s.CommitPvtDataOfOldBlocks(nil, deprioritizedList))

	// write pvt data for block 3
	require.NoError(t, s.Commit(3, nil, nil))
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store (because purger should not be launched at block 3)
	testWaitForPurgerRoutineToFinish(s)
	require.True(t, testDataKeyExists(t, s, ns1Coll1))
	require.True(t, testDataKeyExists(t, s, ns2Coll2))
	// eligible missingData entries for ns-1:coll-1, ns-1:coll-2 (neverExpires) should exist in store
	require.True(t, testElgPrioMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(t, testElgPrioMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// some transactions which miss ns-1:coll-1 and ns-1:coll-2 has be moved to deprioritizedList list
	require.True(t, testElgDeprioMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(t, testElgDeprioMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// ineligible missingData entries for ns-3:col-1, ns-3:coll-2 (neverExpires) should exist in store
	require.True(t, testInelgMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	require.True(t, testInelgMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 4
	require.NoError(t, s.Commit(4, nil, nil))
	// data for ns-1:coll-1 should not exist in store (because purger should be launched at block 4)
	// but ns-2:coll-2 should exist because it expires at block 5
	testWaitForPurgerRoutineToFinish(s)
	require.False(t, testDataKeyExists(t, s, ns1Coll1))
	require.True(t, testDataKeyExists(t, s, ns2Coll2))
	// eligible missingData entries for ns-1:coll-1 should have expired and ns-1:coll-2 (neverExpires) should exist in store
	require.False(t, testElgPrioMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(t, testElgPrioMissingDataKeyExists(t, s, ns1Coll2elgMD))
	require.False(t, testElgDeprioMissingDataKeyExists(t, s, ns1Coll1elgMD))
	require.True(t, testElgDeprioMissingDataKeyExists(t, s, ns1Coll2elgMD))
	// ineligible missingData entries for ns-3:col-1 should have expired and ns-3:coll-2 (neverExpires) should exist in store
	require.False(t, testInelgMissingDataKeyExists(t, s, ns3Coll1inelgMD))
	require.True(t, testInelgMissingDataKeyExists(t, s, ns3Coll2inelgMD))

	// write pvt data for block 5
	require.NoError(t, s.Commit(5, nil, nil))
	// ns-2:coll-2 should exist because though the data expires at block 5 but purger is launched every second block
	testWaitForPurgerRoutineToFinish(s)
	require.False(t, testDataKeyExists(t, s, ns1Coll1))
	require.True(t, testDataKeyExists(t, s, ns2Coll2))

	// write pvt data for block 6
	require.NoError(t, s.Commit(6, nil, nil))
	// ns-2:coll-2 should not exists now (because purger should be launched at block 6)
	testWaitForPurgerRoutineToFinish(s)
	require.False(t, testDataKeyExists(t, s, ns1Coll1))
	require.False(t, testDataKeyExists(t, s, ns2Coll2))

	// "ns-2:coll-1" should never have been purged (because, it was no btl was declared for this)
	require.True(t, testDataKeyExists(t, s, &dataKey{nsCollBlk: nsCollBlk{ns: "ns-1", coll: "coll-2", blkNum: 1}, txNum: 2}))
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
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 0, []string{"ns-1:coll-1", "ns-1:coll-2"}),
	}

	require.EqualError(t,
		store.Commit(1, testData, nil),
		"expected block number=0, received block number=1",
	)
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
	s := env.TestStore
	existingLastBlockNum := uint64(25)
	batch := s.db.NewUpdateBatch()
	batch.Put(lastCommittedBlkkey, encodeLastCommittedBlockVal(existingLastBlockNum))
	require.NoError(t, s.db.WriteBatch(batch, true))
	s.lastCommittedBlock = existingLastBlockNum
	s.isEmpty = false
	testLastCommittedBlockHeight(t, existingLastBlockNum+1, s)

	// assume that a block has been prepared in v142 and the peer was
	// killed for upgrade. When the pvtdataStore is opened again with
	// v2.0 peer, the pendingBatch should be marked as committed.
	batch = s.db.NewUpdateBatch()

	// store pvtData entries
	dataKey := &dataKey{nsCollBlk{"ns-1", "coll-1", 26}, 1}
	dataValue := &rwset.CollectionPvtReadWriteSet{CollectionName: "coll-1", Rwset: []byte("pvtdata")}
	keyBytes := encodeDataKey(dataKey)
	valueBytes, err := encodeDataValue(dataValue)
	require.NoError(t, err)
	batch.Put(keyBytes, valueBytes)

	// store pendingBatch marker
	batch.Put(pendingCommitKey, emptyValue)

	// write to the store
	require.NoError(t, s.db.WriteBatch(batch, true))
	testLastCommittedBlockHeight(t, existingLastBlockNum+1, s)

	// as the block commit is pending, we cannot read the pvtData
	hasPendingBatch, err := s.hasPendingCommit()
	require.NoError(t, err)
	require.Equal(t, true, hasPendingBatch)
	pvtData, err := s.GetPvtDataByBlockNum(26, nil)
	require.EqualError(t, err, "last committed block number [25] smaller than the requested block number [26]")
	require.Nil(t, pvtData)

	// emulate a version upgrade
	env.CloseAndReopen()

	s = env.TestStore
	testLastCommittedBlockHeight(t, existingLastBlockNum+2, s)
	hasPendingBatch, err = s.hasPendingCommit()
	require.NoError(t, err)
	require.Equal(t, false, hasPendingBatch)
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
	require.NoError(t, err)
	require.Equal(t, 1, len(pvtData))
	require.Equal(t, uint64(1), pvtData[0].SeqInBlock)
	require.True(t, proto.Equal(expectedPvtData, pvtData[0].WriteSet))
}

func TestCollElgEnabled(t *testing.T) {
	conf := pvtDataConf()
	testCollElgEnabled(t, conf)
	conf.BatchesInterval = 1
	conf.MaxBatchSize = 1
	testCollElgEnabled(t, conf)
}

func TestDrop(t *testing.T) {
	ledgerid := "testremove"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns-1", "coll-1"}: 0,
			{"ns-1", "coll-2"}: 0,
			{"ns-2", "coll-1"}: 0,
			{"ns-2", "coll-2"}: 0,
		},
	)

	env := NewTestStoreEnv(t, ledgerid, btlPolicy, pvtDataConf())
	defer env.Cleanup()
	store := env.TestStore

	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// construct missing data for block 1
	blk1MissingData := make(ledger.TxMissingPvtData)

	// eligible missing data in tx1
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-1", "coll-2", true)
	blk1MissingData.Add(1, "ns-2", "coll-1", true)
	blk1MissingData.Add(1, "ns-2", "coll-2", true)

	// no pvt data with block 0
	require.NoError(t, store.Commit(0, nil, nil))

	// pvt data with block 1 - commit
	require.NoError(t, store.Commit(1, testData, blk1MissingData))

	// pvt data retrieval for block 0 should return nil
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(0, nilFilter)
	require.NoError(t, err)
	require.Nil(t, retrievedData)

	// pvt data retrieval for block 1 should return full pvtdata
	retrievedData, err = store.GetPvtDataByBlockNum(1, nilFilter)
	require.NoError(t, err)
	require.Equal(t, len(testData), len(retrievedData))
	for i, data := range retrievedData {
		require.Equal(t, data.SeqInBlock, testData[i].SeqInBlock)
		require.True(t, proto.Equal(data.WriteSet, testData[i].WriteSet))
	}

	require.NoError(t, env.TestStoreProvider.Drop(ledgerid))

	// pvt data should be removed
	retrievedData, err = store.GetPvtDataByBlockNum(0, nilFilter)
	require.NoError(t, err)
	require.Nil(t, retrievedData)

	retrievedData, err = store.GetPvtDataByBlockNum(1, nilFilter)
	require.NoError(t, err)
	require.Nil(t, retrievedData)

	itr, err := env.TestStoreProvider.dbProvider.GetDBHandle(ledgerid).GetIterator(nil, nil)
	require.NoError(t, err)
	require.False(t, itr.Next())

	// drop again is not an error
	require.NoError(t, env.TestStoreProvider.Drop(ledgerid))

	// negative test
	env.TestStoreProvider.Close()
	require.EqualError(t, env.TestStoreProvider.Drop(ledgerid), "internal leveldb error while obtaining db iterator: leveldb: closed")
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
	testStore := env.TestStore

	// Initial state: eligible for {ns-1:coll-1 and ns-2:coll-1 }

	// no pvt data with block 0
	require.NoError(t, testStore.Commit(0, nil, nil))

	// construct and commit block 1
	blk1MissingData := make(ledger.TxMissingPvtData)
	blk1MissingData.Add(1, "ns-1", "coll-1", true)
	blk1MissingData.Add(1, "ns-2", "coll-1", true)
	blk1MissingData.Add(4, "ns-1", "coll-2", false)
	blk1MissingData.Add(4, "ns-2", "coll-2", false)
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1"}),
	}
	require.NoError(t, testStore.Commit(1, testDataForBlk1, blk1MissingData))

	// construct and commit block 2
	blk2MissingData := make(ledger.TxMissingPvtData)
	// ineligible missing data in tx1
	blk2MissingData.Add(1, "ns-1", "coll-2", false)
	blk2MissingData.Add(1, "ns-2", "coll-2", false)
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1"}),
	}
	require.NoError(t, testStore.Commit(2, testDataForBlk2, blk2MissingData))

	// Retrieve and verify missing data reported
	// Expected missing data should be only blk1-tx1 (because, the other missing data is marked as ineliigible)
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtDataInfo.Add(1, 1, "ns-1", "coll-1")
	expectedMissingPvtDataInfo.Add(1, 1, "ns-2", "coll-1")
	missingPvtDataInfo, err := testStore.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Enable eligibility for {ns-1:coll2}
	require.NoError(t,
		testStore.ProcessCollsEligibilityEnabled(
			5,
			map[string][]string{
				"ns-1": {"coll-2"},
			},
		))
	testutilWaitForCollElgProcToFinish(testStore)

	// Retrieve and verify missing data reported
	// Expected missing data should include newly eiligible collections
	expectedMissingPvtDataInfo.Add(1, 4, "ns-1", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-1", "coll-2")
	missingPvtDataInfo, err = testStore.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)

	// Enable eligibility for {ns-2:coll2}
	require.NoError(t,
		testStore.ProcessCollsEligibilityEnabled(6,
			map[string][]string{
				"ns-2": {"coll-2"},
			},
		))
	testutilWaitForCollElgProcToFinish(testStore)

	// Retrieve and verify missing data reported
	// Expected missing data should include newly eiligible collections
	expectedMissingPvtDataInfo.Add(1, 4, "ns-2", "coll-2")
	expectedMissingPvtDataInfo.Add(2, 1, "ns-2", "coll-2")
	missingPvtDataInfo, err = testStore.GetMissingPvtDataInfoForMostRecentBlocks(10)
	require.NoError(t, err)
	require.Equal(t, expectedMissingPvtDataInfo, missingPvtDataInfo)
}

func testLastCommittedBlockHeight(t *testing.T, expectedBlockHt uint64, store *Store) {
	blkHt, err := store.LastCommittedBlockHeight()
	require.NoError(t, err)
	require.Equal(t, expectedBlockHt, blkHt)
}

func testDataKeyExists(t *testing.T, s *Store, dataKey *dataKey) bool {
	dataKeyBytes := encodeDataKey(dataKey)
	val, err := s.db.Get(dataKeyBytes)
	require.NoError(t, err)
	return len(val) != 0
}

func testElgPrioMissingDataKeyExists(t *testing.T, s *Store, missingDataKey *missingDataKey) bool {
	key := encodeElgPrioMissingDataKey(missingDataKey)

	val, err := s.db.Get(key)
	require.NoError(t, err)
	return len(val) != 0
}

func testElgDeprioMissingDataKeyExists(t *testing.T, s *Store, missingDataKey *missingDataKey) bool {
	key := encodeElgDeprioMissingDataKey(missingDataKey)

	val, err := s.db.Get(key)
	require.NoError(t, err)
	return len(val) != 0
}
func testInelgMissingDataKeyExists(t *testing.T, s *Store, missingDataKey *missingDataKey) bool {
	key := encodeInelgMissingDataKey(missingDataKey)

	val, err := s.db.Get(key)
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
