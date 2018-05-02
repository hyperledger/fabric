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

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
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
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)
	env := NewTestStoreEnv(t, "TestStoreBasicCommitAndRetrieval", btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore
	testData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}

	// no pvt data with block 0
	assert.NoError(store.Prepare(0, nil))
	assert.NoError(store.Commit())

	// pvt data with block 1 - commit
	assert.NoError(store.Prepare(1, testData))
	assert.NoError(store.Commit())

	// pvt data with block 2 - rollback
	assert.NoError(store.Prepare(2, testData))
	assert.NoError(store.Rollback())

	// pvt data retrieval for block 0 should return nil
	var nilFilter ledger.PvtNsCollFilter
	retrievedData, err := store.GetPvtDataByBlockNum(0, nilFilter)
	assert.NoError(err)
	assert.Nil(retrievedData)

	// pvt data retrieval for block 1 should return full pvtdata
	retrievedData, err = store.GetPvtDataByBlockNum(1, nilFilter)
	assert.NoError(err)
	assert.Equal(testData, retrievedData)

	// pvt data retrieval for block 1 with filter should return filtered pvtdata
	filter := ledger.NewPvtNsCollFilter()
	filter.Add("ns-1", "coll-1")
	filter.Add("ns-2", "coll-2")
	retrievedData, err = store.GetPvtDataByBlockNum(1, filter)
	expectedRetrievedData := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-2:coll-2"}),
	}
	testutil.AssertEquals(t, retrievedData, expectedRetrievedData)

	// pvt data retrieval for block 2 should return ErrOutOfRange
	retrievedData, err = store.GetPvtDataByBlockNum(2, nilFilter)
	_, ok := err.(*ErrOutOfRange)
	assert.True(ok)
	assert.Nil(retrievedData)
}

func TestExpiryDataNotIncluded(t *testing.T) {
	ledgerid := "TestExpiryDataNotIncluded"
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns-1", "coll-1", 1)
	cs.SetBTL("ns-1", "coll-2", 0)
	cs.SetBTL("ns-2", "coll-1", 0)
	cs.SetBTL("ns-2", "coll-2", 2)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	store := env.TestStore

	// no pvt data with block 0
	assert.NoError(store.Prepare(0, nil))
	assert.NoError(store.Commit())

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	assert.NoError(store.Prepare(1, testDataForBlk1))
	assert.NoError(store.Commit())

	// write pvt data for block 2
	testDataForBlk2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	assert.NoError(store.Prepare(2, testDataForBlk2))
	assert.NoError(store.Commit())

	retrievedData, _ := store.GetPvtDataByBlockNum(1, nil)
	// block 1 data should still be not expired
	testutil.AssertEquals(t, retrievedData, testDataForBlk1)

	// Commit block 3 with no pvtdata
	assert.NoError(store.Prepare(3, nil))
	assert.NoError(store.Commit())

	// After committing block 3, the data for "ns-1:coll1" of block 1 should have expired and should not be returned by the store
	expectedPvtdataFromBlock1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	testutil.AssertEquals(t, retrievedData, expectedPvtdataFromBlock1)

	// Commit block 4 with no pvtdata
	assert.NoError(store.Prepare(4, nil))
	assert.NoError(store.Commit())

	// After committing block 4, the data for "ns-2:coll2" of block 1 should also have expired and should not be returned by the store
	expectedPvtdataFromBlock1 = []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-2", "ns-2:coll-1"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-2", "ns-2:coll-1"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(1, nil)
	testutil.AssertEquals(t, retrievedData, expectedPvtdataFromBlock1)

	// Now, for block 2, "ns-1:coll1" should also have expired
	expectedPvtdataFromBlock2 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 3, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 5, []string{"ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	retrievedData, _ = store.GetPvtDataByBlockNum(2, nil)
	testutil.AssertEquals(t, retrievedData, expectedPvtdataFromBlock2)
}

func TestStorePurge(t *testing.T) {
	ledgerid := "TestStorePurge"
	viper.Set("ledger.pvtdataStore.purgeInterval", 2)
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns-1", "coll-1", 1)
	cs.SetBTL("ns-1", "coll-2", 0)
	cs.SetBTL("ns-2", "coll-1", 0)
	cs.SetBTL("ns-2", "coll-2", 4)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

	env := NewTestStoreEnv(t, ledgerid, btlPolicy)
	defer env.Cleanup()
	assert := assert.New(t)
	s := env.TestStore

	// no pvt data with block 0
	assert.NoError(s.Prepare(0, nil))
	assert.NoError(s.Commit())

	// write pvt data for block 1
	testDataForBlk1 := []*ledger.TxPvtData{
		produceSamplePvtdata(t, 2, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
		produceSamplePvtdata(t, 4, []string{"ns-1:coll-1", "ns-1:coll-2", "ns-2:coll-1", "ns-2:coll-2"}),
	}
	assert.NoError(s.Prepare(1, testDataForBlk1))
	assert.NoError(s.Commit())

	// write pvt data for block 2
	assert.NoError(s.Prepare(2, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store
	testWaitForPurgerRoutineToFinish(s)
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-1", coll: "coll-1"}))
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-2", coll: "coll-2"}))

	// write pvt data for block 3
	assert.NoError(s.Prepare(3, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 and ns-2:coll-2 should exist in store (because purger should not be launched at block 3)
	testWaitForPurgerRoutineToFinish(s)
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-1", coll: "coll-1"}))
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-2", coll: "coll-2"}))

	// write pvt data for block 4
	assert.NoError(s.Prepare(4, nil))
	assert.NoError(s.Commit())
	// data for ns-1:coll-1 should not exist in store (because purger should be launched at block 4) but ns-2:coll-2 should exist because it
	// expires at block 5
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-1", coll: "coll-1"}))
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-2", coll: "coll-2"}))

	// write pvt data for block 5
	assert.NoError(s.Prepare(5, nil))
	assert.NoError(s.Commit())
	// ns-2:coll-2 should exist because though the data expires at block 5 but purger is launched every second block
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-1", coll: "coll-1"}))
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-2", coll: "coll-2"}))

	// write pvt data for block 6
	assert.NoError(s.Prepare(6, nil))
	assert.NoError(s.Commit())
	// ns-2:coll-2 should not exists now (because purger should be launched at block 6)
	testWaitForPurgerRoutineToFinish(s)
	assert.False(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-1", coll: "coll-1"}))
	assert.False(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-2", coll: "coll-2"}))

	// "ns-2:coll-1" should never have been purged (because, it was no btl was declared for this)
	assert.True(testDataKeyExists(t, s, &dataKey{blkNum: 1, txNum: 2, ns: "ns-1", coll: "coll-2"}))
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
	_, ok := store.Prepare(1, testData).(*ErrIllegalArgs)
	assert.True(ok)

	assert.Nil(store.Prepare(0, testData))
	assert.NoError(store.Commit())

	assert.Nil(store.Prepare(1, testData))
	_, ok = store.Prepare(2, testData).(*ErrIllegalCall)
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
