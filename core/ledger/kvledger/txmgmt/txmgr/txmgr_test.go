/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

func TestTxSimulatorWithNoExistingData(t *testing.T) {
	// run the tests for each environment configured in pkg_test.go
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testtxsimulatorwithnoexistingdata"
		testEnv.init(t, testLedgerID, nil)
		testTxSimulatorWithNoExistingData(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxSimulatorWithNoExistingData(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	s, _ := txMgr.NewTxSimulator("test_txid")
	value, err := s.GetState("ns1", "key1")
	require.NoErrorf(t, err, "Error in GetState(): %s", err)
	require.Nil(t, value)

	require.NoError(t, s.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, s.SetState("ns1", "key2", []byte("value2")))
	require.NoError(t, s.SetState("ns2", "key3", []byte("value3")))
	require.NoError(t, s.SetState("ns2", "key4", []byte("value4")))

	value, _ = s.GetState("ns2", "key3")
	require.Nil(t, value)

	simulationResults, err := s.GetTxSimulationResults()
	require.NoError(t, err)
	require.Nil(t, simulationResults.PvtSimulationResults)
}

func TestTxSimulatorGetResults(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testLedger", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns1", "coll1"},
			{"ns1", "coll3"},
			{"ns2", "coll2"},
			{"ns3", "coll3"},
		},
		version.NewHeight(1, 1),
	)

	var err error

	// Create a simulator and get/set keys in one namespace "ns1"
	simulator, _ := testEnv.getTxMgr().NewTxSimulator("test_txid1")
	_, err = simulator.GetState("ns1", "key1")
	require.NoError(t, err)
	_, err = simulator.GetPrivateData("ns1", "coll1", "key1")
	require.NoError(t, err)
	err = simulator.SetState("ns1", "key1", []byte("value1"))
	require.NoError(t, err)
	// get simulation results and verify that this contains rwset only for one namespace
	simulationResults1, err := simulator.GetTxSimulationResults()
	require.NoError(t, err)
	require.Len(t, simulationResults1.PubSimulationResults.NsRwset, 1)
	// verify the Private Read has been captured
	expectedPrivateReads := ledger.PrivateReads{}
	expectedPrivateReads.Add("ns1", "coll1")
	require.Equal(t, expectedPrivateReads, simulationResults1.PrivateReads)
	// clone freeze simulationResults1
	buff1 := new(bytes.Buffer)
	require.NoError(t, gob.NewEncoder(buff1).Encode(simulationResults1))
	frozenSimulationResults1 := &ledger.TxSimulationResults{}
	require.NoError(t, gob.NewDecoder(buff1).Decode(&frozenSimulationResults1))

	// use the same simulator after obtaining the simulation results by get/set keys in one more namespace "ns2"
	_, err = simulator.GetState("ns2", "key2")
	require.NoError(t, err)
	_, err = simulator.GetPrivateData("ns2", "coll2", "key2")
	require.NoError(t, err)
	err = simulator.SetState("ns2", "key2", []byte("value2"))
	require.NoError(t, err)
	// get simulation results and verify that an error is raised when obtaining the simulation results more than once
	_, err = simulator.GetTxSimulationResults()
	require.Error(t, err) // calling 'GetTxSimulationResults()' more than once should raise error
	// Now, verify that the simulator operations did not have an effect on previously obtained results
	require.Equal(t, frozenSimulationResults1, simulationResults1)

	// Call 'Done' and all the data get/set operations after calling 'Done' should fail.
	simulator.Done()
	_, err = simulator.GetState("ns3", "key3")
	require.Errorf(t, err, "An error is expected when using simulator to get/set data after calling `Done` function()")
	err = simulator.SetState("ns3", "key3", []byte("value3"))
	require.Errorf(t, err, "An error is expected when using simulator to get/set data after calling `Done` function()")
	_, err = simulator.GetPrivateData("ns3", "coll3", "key3")
	require.Errorf(t, err, "An error is expected when using simulator to get/set data after calling `Done` function()")
	err = simulator.SetPrivateData("ns3", "coll3", "key3", []byte("value3"))
	require.Errorf(t, err, "An error is expected when using simulator to get/set data after calling `Done` function()")
}

func TestTxSimulatorWithExistingData(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Run(testEnv.getName(), func(t *testing.T) {
			testLedgerID := "testtxsimulatorwithexistingdata"
			testEnv.init(t, testLedgerID, nil)
			testTxSimulatorWithExistingData(t, testEnv)
			testEnv.cleanup()
		})
	}
}

func testTxSimulatorWithExistingData(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	require.NoError(t, s1.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, s1.SetState("ns1", "key2", []byte("value2")))
	require.NoError(t, s1.SetState("ns2", "key3", []byte("value3")))
	require.NoError(t, s1.SetState("ns2", "key4", []byte("value4")))
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// simulate tx2 that make changes to existing data
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	value, _ := s2.GetState("ns1", "key1")
	require.Equal(t, []byte("value1"), value)
	require.NoError(t, s2.SetState("ns1", "key1", []byte("value1_1")))
	require.NoError(t, s2.DeleteState("ns2", "key3"))
	value, err := s2.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1"), value)
	s2.Done()
	// validate and commit RWset for tx2
	txRWSet2, err := s2.GetTxSimulationResults()
	require.NoError(t, err)
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)

	// simulate tx3
	s3, err := txMgr.NewTxSimulator("test_tx3")
	require.NoError(t, err)
	value, err = s3.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, []byte("value1_1"), value)
	value, err = s3.GetState("ns2", "key3")
	require.NoError(t, err)
	require.Nil(t, value)
	s3.Done()

	// verify the versions of keys in persistence
	vv, _ := env.getVDB().GetState("ns1", "key1")
	require.Equal(t, version.NewHeight(2, 0), vv.Version)
	vv, _ = env.getVDB().GetState("ns1", "key2")
	require.Equal(t, version.NewHeight(1, 0), vv.Version)
}

func TestTxValidation(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testtxvalidation"
		testEnv.init(t, testLedgerID, nil)
		testTxValidation(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxValidation(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	require.NoError(t, s1.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, s1.SetState("ns1", "key2", []byte("value2")))
	require.NoError(t, s1.SetState("ns2", "key3", []byte("value3")))
	require.NoError(t, s1.SetState("ns2", "key4", []byte("value4")))
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// simulate tx2 that make changes to existing data.
	// tx2: Read/Update ns1:key1, Delete ns2:key3.
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	value, _ := s2.GetState("ns1", "key1")
	require.Equal(t, []byte("value1"), value)

	require.NoError(t, s2.SetState("ns1", "key1", []byte("value1_2")))
	require.NoError(t, s2.DeleteState("ns2", "key3"))
	s2.Done()

	// simulate tx3 before committing tx2 changes. Reads and modifies the key changed by tx2.
	// tx3: Read/Update ns1:key1
	s3, _ := txMgr.NewTxSimulator("test_tx3")
	_, err := s3.GetState("ns1", "key1")
	require.NoError(t, err)
	require.NoError(t, s3.SetState("ns1", "key1", []byte("value1_3")))
	s3.Done()

	// simulate tx4 before committing tx2 changes. Reads and Deletes the key changed by tx2
	// tx4: Read/Delete ns2:key3
	s4, _ := txMgr.NewTxSimulator("test_tx4")
	_, err = s4.GetState("ns2", "key3")
	require.NoError(t, err)
	require.NoError(t, s4.DeleteState("ns2", "key3"))
	s4.Done()

	// simulate tx5 before committing tx2 changes. Modifies and then Reads the key changed by tx2 and writes a new key
	// tx5: Update/Read ns1:key1
	s5, err := txMgr.NewTxSimulator("test_tx5")
	require.NoError(t, err)
	require.NoError(t, s5.SetState("ns1", "key1", []byte("new_value")))
	_, err = s5.GetState("ns1", "key1")
	require.NoError(t, err)
	s5.Done()

	// simulate tx6 before committing tx2 changes. Only writes a new key, does not reads/writes a key changed by tx2
	// tx6: Update ns1:new_key
	s6, _ := txMgr.NewTxSimulator("test_tx6")
	require.NoError(t, s6.SetState("ns1", "new_key", []byte("new_value")))
	s6.Done()

	// Summary of simulated transactions
	// tx2: Read/Update ns1:key1, Delete ns2:key3.
	// tx3: Read/Update ns1:key1
	// tx4: Read/Delete ns2:key3
	// tx5: Update/Read ns1:key1
	// tx6: Update ns1:new_key

	// validate and commit RWset for tx2
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)

	// RWSet for tx3 and tx4 and tx5 should be invalid now due to read conflicts
	txRWSet3, _ := s3.GetTxSimulationResults()
	txMgrHelper.checkRWsetInvalid(txRWSet3.PubSimulationResults)

	txRWSet4, _ := s4.GetTxSimulationResults()
	txMgrHelper.checkRWsetInvalid(txRWSet4.PubSimulationResults)

	txRWSet5, _ := s5.GetTxSimulationResults()
	txMgrHelper.checkRWsetInvalid(txRWSet5.PubSimulationResults)

	// tx6 should still be valid as it only writes a new key
	txRWSet6, _ := s6.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet6.PubSimulationResults)
}

func TestTxPhantomValidation(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testtxphantomvalidation"
		testEnv.init(t, testLedgerID, nil)
		testTxPhantomValidation(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxPhantomValidation(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	require.NoError(t, s1.SetState("ns", "key1", []byte("value1")))
	require.NoError(t, s1.SetState("ns", "key2", []byte("value2")))
	require.NoError(t, s1.SetState("ns", "key3", []byte("value3")))
	require.NoError(t, s1.SetState("ns", "key4", []byte("value4")))
	require.NoError(t, s1.SetState("ns", "key5", []byte("value5")))
	require.NoError(t, s1.SetState("ns", "key6", []byte("value6")))
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	s1.Done() // explicitly calling done after obtaining the results to verify FAB-10788
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// simulate tx2
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	itr2, _ := s2.GetStateRangeScanIterator("ns", "key2", "key5")
	for {
		if result, _ := itr2.Next(); result == nil {
			break
		}
	}
	require.NoError(t, s2.DeleteState("ns", "key3"))
	txRWSet2, err := s2.GetTxSimulationResults()
	require.NoError(t, err)
	s2.Done()

	// simulate tx3
	s3, err := txMgr.NewTxSimulator("test_tx3")
	require.NoError(t, err)
	itr3, err := s3.GetStateRangeScanIterator("ns", "key2", "key5")
	require.NoError(t, err)
	for {
		if result, _ := itr3.Next(); result == nil {
			break
		}
	}
	require.NoError(t, s3.SetState("ns", "key3", []byte("value3_new")))
	txRWSet3, _ := s3.GetTxSimulationResults()
	s3.Done()
	// simulate tx4
	s4, _ := txMgr.NewTxSimulator("test_tx4")
	itr4, _ := s4.GetStateRangeScanIterator("ns", "key4", "key6")
	for {
		if result, _ := itr4.Next(); result == nil {
			break
		}
	}
	require.NoError(t, s4.SetState("ns", "key3", []byte("value3_new")))
	txRWSet4, _ := s4.GetTxSimulationResults()
	s4.Done()

	// txRWSet2 should be valid
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)
	// txRWSet2 makes txRWSet3 invalid as it deletes a key in the range
	txMgrHelper.checkRWsetInvalid(txRWSet3.PubSimulationResults)
	// txRWSet4 should be valid as it iterates over a different range
	txMgrHelper.validateAndCommitRWSet(txRWSet4.PubSimulationResults)
}

func TestIterator(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())

		testLedgerID := "testiterator.1"
		testEnv.init(t, testLedgerID, nil)
		testIterator(t, testEnv, 10, 2, 7)
		testEnv.cleanup()

		testLedgerID = "testiterator.2"
		testEnv.init(t, testLedgerID, nil)
		testIterator(t, testEnv, 10, 1, 11)
		testEnv.cleanup()

		testLedgerID = "testiterator.3"
		testEnv.init(t, testLedgerID, nil)
		testIterator(t, testEnv, 10, 0, 0)
		testEnv.cleanup()

		testLedgerID = "testiterator.4"
		testEnv.init(t, testLedgerID, nil)
		testIterator(t, testEnv, 10, 5, 0)
		testEnv.cleanup()

		testLedgerID = "testiterator.5"
		testEnv.init(t, testLedgerID, nil)
		testIterator(t, testEnv, 10, 0, 5)
		testEnv.cleanup()
	}
}

func testIterator(t *testing.T, env testEnv, numKeys int, startKeyNum int, endKeyNum int) {
	cID := "cid"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	s, _ := txMgr.NewTxSimulator("test_tx1")
	for i := 1; i <= numKeys; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		require.NoError(t, s.SetState(cID, k, v))
	}
	s.Done()
	// validate and commit RWset
	txRWSet, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet.PubSimulationResults)

	var startKey string
	var endKey string
	var begin int
	var end int

	if startKeyNum != 0 {
		begin = startKeyNum
		startKey = createTestKey(startKeyNum)
	} else {
		begin = 1 // first key in the db
		startKey = ""
	}

	if endKeyNum != 0 {
		endKey = createTestKey(endKeyNum)
		end = endKeyNum
	} else {
		endKey = ""
		end = numKeys + 1 // last key in the db
	}

	expectedCount := end - begin

	queryExecuter, _ := txMgr.NewQueryExecutor("test_tx2")
	itr, _ := queryExecuter.GetStateRangeScanIterator(cID, startKey, endKey)
	count := 0
	for {
		kv, _ := itr.Next()
		if kv == nil {
			break
		}
		keyNum := begin + count
		k := kv.(*queryresult.KV).Key
		v := kv.(*queryresult.KV).Value
		t.Logf("Retrieved k=%s, v=%s at count=%d start=%s end=%s", k, v, count, startKey, endKey)
		require.Equal(t, createTestKey(keyNum), k)
		require.Equal(t, createTestValue(keyNum), v)
		count++
	}
	require.Equal(t, expectedCount, count)
}

func TestIteratorPaging(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())

		// test explicit paging
		testLedgerID := "testiterator.1"
		testEnv.init(t, testLedgerID, nil)
		testIteratorPagingInit(t, testEnv, 10)
		returnKeys := []string{"key_002", "key_003"}
		nextStartKey := testIteratorPaging(t, testEnv, 10, "key_002", "key_007", int32(2), returnKeys)
		returnKeys = []string{"key_004", "key_005"}
		nextStartKey = testIteratorPaging(t, testEnv, 10, nextStartKey, "key_007", int32(2), returnKeys)
		returnKeys = []string{"key_006"}
		testIteratorPaging(t, testEnv, 10, nextStartKey, "key_007", int32(2), returnKeys)
		testEnv.cleanup()
	}
}

func testIteratorPagingInit(t *testing.T, env testEnv, numKeys int) {
	cID := "cid"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	s, _ := txMgr.NewTxSimulator("test_tx1")
	for i := 1; i <= numKeys; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		require.NoError(t, s.SetState(cID, k, v))
	}
	s.Done()
	// validate and commit RWset
	txRWSet, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet.PubSimulationResults)
}

func testIteratorPaging(t *testing.T, env testEnv, numKeys int, startKey, endKey string,
	pageSize int32, expectedKeys []string) string {
	cID := "cid"
	txMgr := env.getTxMgr()

	queryExecuter, _ := txMgr.NewQueryExecutor("test_tx2")
	itr, _ := queryExecuter.GetStateRangeScanIteratorWithPagination(cID, startKey, endKey, pageSize)

	// Verify the keys returned
	testItrWithoutClose(t, itr, expectedKeys)

	returnBookmark := ""
	if pageSize > 0 {
		returnBookmark = itr.GetBookmarkAndClose()
	}

	return returnBookmark
}

// testItrWithoutClose verifies an iterator contains expected keys
func testItrWithoutClose(t *testing.T, itr ledger.QueryResultsIterator, expectedKeys []string) {
	for _, expectedKey := range expectedKeys {
		queryResult, err := itr.Next()
		require.NoError(t, err, "An unexpected error was thrown during iterator Next()")
		vkv := queryResult.(*queryresult.KV)
		key := vkv.Key
		require.Equal(t, expectedKey, key)
	}
	queryResult, err := itr.Next()
	require.NoError(t, err, "An unexpected error was thrown during iterator Next()")
	require.Nil(t, queryResult)
}

func TestIteratorWithDeletes(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testiteratorwithdeletes"
		testEnv.init(t, testLedgerID, nil)
		testIteratorWithDeletes(t, testEnv)
		testEnv.cleanup()
	}
}

func testIteratorWithDeletes(t *testing.T, env testEnv) {
	cID := "cid"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	s, _ := txMgr.NewTxSimulator("test_tx1")
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		require.NoError(t, s.SetState(cID, k, v))
	}
	s.Done()
	// validate and commit RWset
	txRWSet1, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	s, _ = txMgr.NewTxSimulator("test_tx2")
	require.NoError(t, s.DeleteState(cID, createTestKey(4)))
	s.Done()
	// validate and commit RWset
	txRWSet2, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)

	queryExecuter, _ := txMgr.NewQueryExecutor("test_tx3")
	itr, _ := queryExecuter.GetStateRangeScanIterator(cID, createTestKey(3), createTestKey(6))
	defer itr.Close()
	kv, _ := itr.Next()
	require.Equal(t, createTestKey(3), kv.(*queryresult.KV).Key)
	kv, _ = itr.Next()
	require.Equal(t, createTestKey(5), kv.(*queryresult.KV).Key)
}

func TestTxValidationWithItr(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testtxvalidationwithitr"
		testEnv.init(t, testLedgerID, nil)
		testTxValidationWithItr(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxValidationWithItr(t *testing.T, env testEnv) {
	cID := "cid"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)

	// simulate tx1
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		require.NoError(t, s1.SetState(cID, k, v))
	}
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// simulate tx2 that reads key_001 and key_002
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	itr, _ := s2.GetStateRangeScanIterator(cID, createTestKey(1), createTestKey(5))
	// read key_001 and key_002
	_, err := itr.Next()
	require.NoError(t, err)
	_, err = itr.Next()
	require.NoError(t, err)
	itr.Close()
	s2.Done()

	// simulate tx3 that reads key_004 and key_005
	s3, _ := txMgr.NewTxSimulator("test_tx3")
	itr, _ = s3.GetStateRangeScanIterator(cID, createTestKey(4), createTestKey(6))
	// read key_001 and key_002
	_, err = itr.Next()
	require.NoError(t, err)
	_, err = itr.Next()
	require.NoError(t, err)
	itr.Close()
	s3.Done()

	// simulate tx4 before committing tx2 and tx3. Modifies a key read by tx3
	s4, _ := txMgr.NewTxSimulator("test_tx4")
	require.NoError(t, s4.DeleteState(cID, createTestKey(5)))
	s4.Done()

	// validate and commit RWset for tx4
	txRWSet4, _ := s4.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet4.PubSimulationResults)

	// RWSet tx3 should be invalid now
	txRWSet3, _ := s3.GetTxSimulationResults()
	txMgrHelper.checkRWsetInvalid(txRWSet3.PubSimulationResults)

	// tx2 should still be valid
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)
}

func TestGetSetMultipeKeys(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testgetsetmultipekeys"
		testEnv.init(t, testLedgerID, nil)
		testGetSetMultipeKeys(t, testEnv)
		testEnv.cleanup()
	}
}

func testGetSetMultipeKeys(t *testing.T, env testEnv) {
	cID := "cid"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	multipleKeyMap := make(map[string][]byte)
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		multipleKeyMap[k] = v
	}
	require.NoError(t, s1.SetStateMultipleKeys(cID, multipleKeyMap))
	s1.Done()
	// validate and commit RWset
	txRWSet, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet.PubSimulationResults)
	qe, _ := txMgr.NewQueryExecutor("test_tx2")
	defer qe.Done()
	multipleKeys := []string{}
	for k := range multipleKeyMap {
		multipleKeys = append(multipleKeys, k)
	}
	values, _ := qe.GetStateMultipleKeys(cID, multipleKeys)
	require.Len(t, values, 10)
	for i, v := range values {
		require.Equal(t, multipleKeyMap[multipleKeys[i]], v)
	}

	s2, _ := txMgr.NewTxSimulator("test_tx3")
	defer s2.Done()
	values, _ = s2.GetStateMultipleKeys(cID, multipleKeys[5:7])
	require.Len(t, values, 2)
	for i, v := range values {
		require.Equal(t, multipleKeyMap[multipleKeys[i+5]], v)
	}
}

func createTestKey(i int) string {
	if i == 0 {
		return ""
	}
	return fmt.Sprintf("key_%03d", i)
}

func createTestValue(i int) []byte {
	return []byte(fmt.Sprintf("value_%03d", i))
}

// TestExecuteQuery is only tested on the CouchDB testEnv
func TestExecuteQuery(t *testing.T) {
	for _, testEnv := range testEnvs {
		// Query is only supported and tested on the CouchDB testEnv
		if testEnv.getName() == couchDBtestEnvName {
			t.Logf("Running test for TestEnv = %s", testEnv.getName())
			testLedgerID := "testexecutequery"
			testEnv.init(t, testLedgerID, nil)
			testExecuteQuery(t, testEnv)
			testEnv.cleanup()
		}
	}
}

func testExecuteQuery(t *testing.T, env testEnv) {
	type Asset struct {
		ID        string `json:"_id"`
		Rev       string `json:"_rev"`
		AssetName string `json:"asset_name"`
		Color     string `json:"color"`
		Size      string `json:"size"`
		Owner     string `json:"owner"`
	}

	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)

	s1, _ := txMgr.NewTxSimulator("test_tx1")

	require.NoError(t, s1.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, s1.SetState("ns1", "key2", []byte("value2")))
	require.NoError(t, s1.SetState("ns1", "key3", []byte("value3")))
	require.NoError(t, s1.SetState("ns1", "key4", []byte("value4")))
	require.NoError(t, s1.SetState("ns1", "key5", []byte("value5")))
	require.NoError(t, s1.SetState("ns1", "key6", []byte("value6")))
	require.NoError(t, s1.SetState("ns1", "key7", []byte("value7")))
	require.NoError(t, s1.SetState("ns1", "key8", []byte("value8")))

	require.NoError(t, s1.SetState("ns1", "key9", []byte(`{"asset_name":"marble1","color":"red","size":"25","owner":"jerry"}`)))
	require.NoError(t, s1.SetState("ns1", "key10", []byte(`{"asset_name":"marble2","color":"blue","size":"10","owner":"bob"}`)))
	require.NoError(t, s1.SetState("ns1", "key11", []byte(`{"asset_name":"marble3","color":"blue","size":"35","owner":"jerry"}`)))
	require.NoError(t, s1.SetState("ns1", "key12", []byte(`{"asset_name":"marble4","color":"green","size":"15","owner":"bob"}`)))
	require.NoError(t, s1.SetState("ns1", "key13", []byte(`{"asset_name":"marble5","color":"red","size":"35","owner":"jerry"}`)))
	require.NoError(t, s1.SetState("ns1", "key14", []byte(`{"asset_name":"marble6","color":"blue","size":"25","owner":"bob"}`)))

	s1.Done()

	// validate and commit RWset
	txRWSet, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet.PubSimulationResults)

	queryExecuter, _ := txMgr.NewQueryExecutor("test_tx2")
	queryString := "{\"selector\":{\"owner\": {\"$eq\": \"bob\"}},\"limit\": 10,\"skip\": 0}"

	itr, err := queryExecuter.ExecuteQuery("ns1", queryString)
	require.NoError(t, err, "Error upon ExecuteQuery()")
	counter := 0
	for {
		queryRecord, _ := itr.Next()
		if queryRecord == nil {
			break
		}
		// Unmarshal the document to Asset structure
		assetResp := &Asset{}
		require.NoError(t, json.Unmarshal(queryRecord.(*queryresult.KV).Value, &assetResp))
		// Verify the owner retrieved matches
		require.Equal(t, "bob", assetResp.Owner)
		counter++
	}
	// Ensure the query returns 3 documents
	require.Equal(t, 3, counter)
}

// TestExecutePaginatedQuery is only tested on the CouchDB testEnv
func TestExecutePaginatedQuery(t *testing.T) {
	for _, testEnv := range testEnvs {
		// Query is only supported and tested on the CouchDB testEnv
		if testEnv.getName() == couchDBtestEnvName {
			t.Logf("Running test for TestEnv = %s", testEnv.getName())
			testLedgerID := "testexecutepaginatedquery"
			testEnv.init(t, testLedgerID, nil)
			testExecutePaginatedQuery(t, testEnv)
			testEnv.cleanup()
		}
	}
}

func testExecutePaginatedQuery(t *testing.T, env testEnv) {
	type Asset struct {
		ID        string `json:"_id"`
		Rev       string `json:"_rev"`
		AssetName string `json:"asset_name"`
		Color     string `json:"color"`
		Size      string `json:"size"`
		Owner     string `json:"owner"`
	}

	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)

	s1, _ := txMgr.NewTxSimulator("test_tx1")

	require.NoError(t, s1.SetState("ns1", "key1", []byte(`{"asset_name":"marble1","color":"red","size":"25","owner":"jerry"}`)))
	require.NoError(t, s1.SetState("ns1", "key2", []byte(`{"asset_name":"marble2","color":"blue","size":"10","owner":"bob"}`)))
	require.NoError(t, s1.SetState("ns1", "key3", []byte(`{"asset_name":"marble3","color":"blue","size":"35","owner":"jerry"}`)))
	require.NoError(t, s1.SetState("ns1", "key4", []byte(`{"asset_name":"marble4","color":"green","size":"15","owner":"bob"}`)))
	require.NoError(t, s1.SetState("ns1", "key5", []byte(`{"asset_name":"marble5","color":"red","size":"35","owner":"jerry"}`)))
	require.NoError(t, s1.SetState("ns1", "key6", []byte(`{"asset_name":"marble6","color":"blue","size":"25","owner":"bob"}`)))

	s1.Done()

	// validate and commit RWset
	txRWSet, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet.PubSimulationResults)

	queryExecuter, _ := txMgr.NewQueryExecutor("test_tx2")
	queryString := `{"selector":{"owner":{"$eq":"bob"}}}`

	itr, err := queryExecuter.ExecuteQueryWithPagination("ns1", queryString, "", 2)
	require.NoError(t, err, "Error upon ExecuteQueryWithMetadata()")
	counter := 0
	for {
		queryRecord, _ := itr.Next()
		if queryRecord == nil {
			break
		}
		// Unmarshal the document to Asset structure
		assetResp := &Asset{}
		require.NoError(t, json.Unmarshal(queryRecord.(*queryresult.KV).Value, &assetResp))
		// Verify the owner retrieved matches
		require.Equal(t, "bob", assetResp.Owner)
		counter++
	}
	// Ensure the query returns 2 documents
	require.Equal(t, 2, counter)

	bookmark := itr.GetBookmarkAndClose()

	itr, err = queryExecuter.ExecuteQueryWithPagination("ns1", queryString, bookmark, 2)
	require.NoError(t, err, "Error upon ExecuteQuery()")
	counter = 0
	for {
		queryRecord, _ := itr.Next()
		if queryRecord == nil {
			break
		}
		// Unmarshal the document to Asset structure
		assetResp := &Asset{}
		require.NoError(t, json.Unmarshal(queryRecord.(*queryresult.KV).Value, &assetResp))
		// Verify the owner retrieved matches
		require.Equal(t, "bob", assetResp.Owner)
		counter++
	}
	// Ensure the query returns 1 documents
	require.Equal(t, 1, counter)
}

func TestValidateKey(t *testing.T) {
	nonUTF8Key := string([]byte{0xff, 0xff})
	dummyValue := []byte("dummyValue")
	for _, testEnv := range testEnvs {
		testLedgerID := "test.validate.key"
		testEnv.init(t, testLedgerID, nil)
		txSimulator, _ := testEnv.getTxMgr().NewTxSimulator("test_tx1")
		err := txSimulator.SetState("ns1", nonUTF8Key, dummyValue)
		if testEnv.getName() == levelDBtestEnvName {
			require.NoError(t, err)
		}
		if testEnv.getName() == couchDBtestEnvName {
			require.Error(t, err)
		}
		testEnv.cleanup()
	}
}

// TestTxSimulatorUnsupportedTx verifies that a simulation must throw an error when an unsupported transaction
// is perfromed - queries on private data are supported in a read-only tran
func TestTxSimulatorUnsupportedTx(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testtxsimulatorunsupportedtx", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns1", "coll1"},
			{"ns1", "coll2"},
			{"ns1", "coll3"},
			{"ns1", "coll4"},
		},
		version.NewHeight(1, 1))

	simulator, _ := txMgr.NewTxSimulator("txid1")
	err := simulator.SetState("ns", "key", []byte("value"))
	require.NoError(t, err)
	_, err = simulator.GetPrivateDataRangeScanIterator("ns1", "coll1", "startKey", "endKey")
	require.EqualError(t, err, "txid [txid1]: unsuppored transaction. Queries on pvt data is supported only in a read-only transaction")

	simulator, _ = txMgr.NewTxSimulator("txid2")
	_, err = simulator.GetPrivateDataRangeScanIterator("ns1", "coll1", "startKey", "endKey")
	require.NoError(t, err)
	err = simulator.SetState("ns", "key", []byte("value"))
	require.EqualError(t, err, "txid [txid2]: unsuppored transaction. Transaction has already performed queries on pvt data. Writes are not allowed")

	simulator, _ = txMgr.NewTxSimulator("txid3")
	err = simulator.SetState("ns", "key", []byte("value"))
	require.NoError(t, err)
	_, err = simulator.GetStateRangeScanIteratorWithPagination("ns1", "startKey", "endKey", 2)
	require.EqualError(t, err, "txid [txid3]: unsuppored transaction. Paginated queries are supported only in a read-only transaction")

	simulator, _ = txMgr.NewTxSimulator("txid4")
	_, err = simulator.GetStateRangeScanIteratorWithPagination("ns1", "startKey", "endKey", 2)
	require.NoError(t, err)
	err = simulator.SetState("ns", "key", []byte("value"))
	require.EqualError(t, err, "txid [txid4]: unsuppored transaction. Transaction has already performed a paginated query. Writes are not allowed")
}

func TestTxSimulatorUnsupportedTxCouchDBQuery(t *testing.T) {
	testEnv := testEnvsMap[couchDBtestEnvName]
	testEnv.init(t, "testtxsimulatorunsupportedtxqueries", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	queryString := `{"selector":{"owner":{"$eq":"bob"}}}`

	simulator, _ := txMgr.NewTxSimulator("txid1")
	err := simulator.SetState("ns1", "key1", []byte(`{"asset_name":"marble1","color":"red","size":"25","owner":"jerry"}`))
	require.NoError(t, err)
	_, err = simulator.ExecuteQueryWithPagination("ns1", queryString, "", 2)
	require.EqualError(t, err, "txid [txid1]: unsuppored transaction. Paginated queries are supported only in a read-only transaction")

	simulator, _ = txMgr.NewTxSimulator("txid2")
	_, err = simulator.ExecuteQueryWithPagination("ns1", queryString, "", 2)
	require.NoError(t, err)
	err = simulator.SetState("ns1", "key1", []byte(`{"asset_name":"marble1","color":"red","size":"25","owner":"jerry"}`))
	require.EqualError(t, err, "txid [txid2]: unsuppored transaction. Transaction has already performed a paginated query. Writes are not allowed")
}

func TestConstructUniquePvtData(t *testing.T) {
	v1 := []byte{1}
	// ns1-coll1-key1 should be rejected as it is updated in the future by Blk2Tx1
	pvtDataBlk1Tx1 := producePvtdata(t, 1, []string{"ns1:coll1"}, []string{"key1"}, [][]byte{v1})
	// ns1-coll2-key3 should be accepted but ns1-coll1-key2 as it is updated in the future by Blk2Tx2
	pvtDataBlk1Tx2 := producePvtdata(t, 2, []string{"ns1:coll1", "ns1:coll2"}, []string{"key2", "key3"}, [][]byte{v1, v1})
	// ns1-coll2-key4 should be accepted
	pvtDataBlk1Tx3 := producePvtdata(t, 3, []string{"ns1:coll2"}, []string{"key4"}, [][]byte{v1})

	v2 := []byte{2}
	// ns1-coll1-key1 should be rejected as it is updated in the future by Blk3Tx1
	pvtDataBlk2Tx1 := producePvtdata(t, 1, []string{"ns1:coll1"}, []string{"key1"}, [][]byte{v2})
	// ns1-coll1-key2 should be accepted
	pvtDataBlk2Tx2 := producePvtdata(t, 2, []string{"ns1:coll1"}, []string{"key2"}, [][]byte{nil})

	v3 := []byte{3}
	// ns1-coll1-key1 should be accepted
	pvtDataBlk3Tx1 := producePvtdata(t, 1, []string{"ns1:coll1"}, []string{"key1"}, [][]byte{v3})

	pvtDataBlk3Tx2WriteSetBytes, err := proto.Marshal(
		&kvrwset.KVRWSet{
			Writes: []*kvrwset.KVWrite{
				{Key: "key5", IsDelete: false, Value: nil},
			},
		},
	)
	require.NoError(t, err)
	pvtDataBlk3Tx2 := &ledger.TxPvtData{
		SeqInBlock: 2,
		WriteSet: &rwset.TxPvtReadWriteSet{
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				{
					Namespace: "ns1",
					CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
						{
							CollectionName: "coll1",
							Rwset:          pvtDataBlk3Tx2WriteSetBytes,
						},
					},
				},
			},
		},
	}

	blocksPvtData := map[uint64][]*ledger.TxPvtData{
		1: {
			pvtDataBlk1Tx1,
			pvtDataBlk1Tx2,
			pvtDataBlk1Tx3,
		},
		2: {
			pvtDataBlk2Tx1,
			pvtDataBlk2Tx2,
		},
		3: {
			pvtDataBlk3Tx1,
			pvtDataBlk3Tx2,
		},
	}

	hashedCompositeKeyNs1Coll2Key3 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll2", KeyHash: string(util.ComputeStringHash("key3"))}
	pvtKVWriteNs1Coll2Key3 := &privacyenabledstate.PvtKVWrite{Key: "key3", IsDelete: false, Value: v1, Version: version.NewHeight(1, 2)}

	hashedCompositeKeyNs1Coll2Key4 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll2", KeyHash: string(util.ComputeStringHash("key4"))}
	pvtKVWriteNs1Coll2Key4 := &privacyenabledstate.PvtKVWrite{Key: "key4", IsDelete: false, Value: v1, Version: version.NewHeight(1, 3)}

	hashedCompositeKeyNs1Coll1Key2 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll1", KeyHash: string(util.ComputeStringHash("key2"))}
	pvtKVWriteNs1Coll1Key2 := &privacyenabledstate.PvtKVWrite{Key: "key2", IsDelete: true, Value: nil, Version: version.NewHeight(2, 2)}

	hashedCompositeKeyNs1Coll1Key1 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll1", KeyHash: string(util.ComputeStringHash("key1"))}
	pvtKVWriteNs1Coll1Key1 := &privacyenabledstate.PvtKVWrite{Key: "key1", IsDelete: false, Value: v3, Version: version.NewHeight(3, 1)}

	hashedCompositeKeyNs1Coll1Key5 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll1", KeyHash: string(util.ComputeStringHash("key5"))}
	pvtKVWriteNs1Coll1Key5 := &privacyenabledstate.PvtKVWrite{Key: "key5", IsDelete: true, Value: nil, Version: version.NewHeight(3, 2)}

	expectedUniquePvtData := uniquePvtDataMap{
		hashedCompositeKeyNs1Coll2Key3: pvtKVWriteNs1Coll2Key3,
		hashedCompositeKeyNs1Coll2Key4: pvtKVWriteNs1Coll2Key4,
		hashedCompositeKeyNs1Coll1Key2: pvtKVWriteNs1Coll1Key2,
		hashedCompositeKeyNs1Coll1Key1: pvtKVWriteNs1Coll1Key1,
		hashedCompositeKeyNs1Coll1Key5: pvtKVWriteNs1Coll1Key5,
	}

	uniquePvtData, err := constructUniquePvtData(blocksPvtData)
	require.NoError(t, err)
	require.Equal(t, expectedUniquePvtData, uniquePvtData)
}

func TestFindAndRemoveStalePvtData(t *testing.T) {
	ledgerid := "TestFindAndRemoveStalePvtData"
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, ledgerid, nil)
	defer testEnv.cleanup()
	db := testEnv.getVDB()

	batch := privacyenabledstate.NewUpdateBatch()
	batch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key1"), util.ComputeStringHash("value_1_1_1"), version.NewHeight(1, 1))
	batch.HashUpdates.Put("ns1", "coll2", util.ComputeStringHash("key2"), util.ComputeStringHash("value_1_2_2"), version.NewHeight(1, 2))
	batch.HashUpdates.Put("ns2", "coll1", util.ComputeStringHash("key2"), util.ComputeStringHash("value_2_1_2"), version.NewHeight(2, 1))
	batch.HashUpdates.Put("ns2", "coll2", util.ComputeStringHash("key3"), util.ComputeStringHash("value_2_2_3"), version.NewHeight(10, 10))

	// all pvt data associated with the hash updates are missing
	require.NoError(t, db.ApplyPrivacyAwareUpdates(batch, version.NewHeight(11, 1)))

	// construct pvt data for some of the above missing data. note that no
	// duplicate entries are expected

	// existent keyhash - a kvwrite with lower version (than the version of existent keyhash) should be considered stale
	hashedCompositeKeyNs1Coll1Key1 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll1", KeyHash: string(util.ComputeStringHash("key1"))}
	pvtKVWriteNs1Coll1Key1 := &privacyenabledstate.PvtKVWrite{Key: "key1", IsDelete: false, Value: []byte("old_value_1_1_1"), Version: version.NewHeight(1, 0)}

	// existent keyhash - a kvwrite with higher version (than the version of existent keyhash) should not be considered stale
	hashedCompositeKeyNs2Coll1Key2 := privacyenabledstate.HashedCompositeKey{Namespace: "ns2", CollectionName: "coll1", KeyHash: string(util.ComputeStringHash("key2"))}
	pvtKVWriteNs2Coll1Key2 := &privacyenabledstate.PvtKVWrite{Key: "key2", IsDelete: false, Value: []byte("value_2_1_2"), Version: version.NewHeight(2, 1)}

	// non existent keyhash (because deleted earlier or expired) - a kvwrite for delete should not be considered stale
	hashedCompositeKeyNs1Coll3Key3 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll3", KeyHash: string(util.ComputeStringHash("key3"))}
	pvtKVWriteNs1Coll3Key3 := &privacyenabledstate.PvtKVWrite{Key: "key3", IsDelete: true, Value: nil, Version: version.NewHeight(2, 3)}

	// non existent keyhash (because deleted earlier or expired) - a kvwrite for value set should be considered stale
	hashedCompositeKeyNs1Coll4Key4 := privacyenabledstate.HashedCompositeKey{Namespace: "ns1", CollectionName: "coll4", KeyHash: string(util.ComputeStringHash("key4"))}
	pvtKVWriteNs1Coll4Key4 := &privacyenabledstate.PvtKVWrite{Key: "key4", Value: []byte("value_1_4_4"), Version: version.NewHeight(2, 3)}

	// there would be a version mismatch but the hash value must be the same. hence,
	// this should be accepted too
	hashedCompositeKeyNs2Coll2Key3 := privacyenabledstate.HashedCompositeKey{Namespace: "ns2", CollectionName: "coll2", KeyHash: string(util.ComputeStringHash("key3"))}
	pvtKVWriteNs2Coll2Key3 := &privacyenabledstate.PvtKVWrite{Key: "key3", IsDelete: false, Value: []byte("value_2_2_3"), Version: version.NewHeight(9, 9)}

	uniquePvtData := uniquePvtDataMap{
		hashedCompositeKeyNs1Coll1Key1: pvtKVWriteNs1Coll1Key1,
		hashedCompositeKeyNs2Coll1Key2: pvtKVWriteNs2Coll1Key2,
		hashedCompositeKeyNs1Coll3Key3: pvtKVWriteNs1Coll3Key3,
		hashedCompositeKeyNs2Coll2Key3: pvtKVWriteNs2Coll2Key3,
		hashedCompositeKeyNs1Coll4Key4: pvtKVWriteNs1Coll4Key4,
	}

	// created the expected batch from ValidateAndPrepareBatchForPvtDataofOldBlocks
	expectedBatch := privacyenabledstate.NewUpdateBatch()
	expectedBatch.PvtUpdates.Put("ns2", "coll1", "key2", []byte("value_2_1_2"), version.NewHeight(2, 1))
	expectedBatch.PvtUpdates.Delete("ns1", "coll3", "key3", version.NewHeight(2, 3))
	expectedBatch.PvtUpdates.Put("ns2", "coll2", "key3", []byte("value_2_2_3"), version.NewHeight(10, 10))

	err := uniquePvtData.findAndRemoveStalePvtData(db)
	require.NoError(t, err, "uniquePvtData.findAndRemoveStatePvtData resulted in an error")
	batch = uniquePvtData.transformToUpdateBatch()
	require.Equal(t, expectedBatch.PvtUpdates, batch.PvtUpdates)
}

func producePvtdata(t *testing.T, txNum uint64, nsColls []string, keys []string, values [][]byte) *ledger.TxPvtData {
	builder := rwsetutil.NewRWSetBuilder()
	for index, nsColl := range nsColls {
		nsCollSplit := strings.Split(nsColl, ":")
		ns := nsCollSplit[0]
		coll := nsCollSplit[1]
		key := keys[index]
		value := values[index]
		builder.AddToPvtAndHashedWriteSet(ns, coll, key, value)
	}
	simRes, err := builder.GetTxSimulationResults()
	require.NoError(t, err)
	return &ledger.TxPvtData{
		SeqInBlock: txNum,
		WriteSet:   simRes.PvtSimulationResults,
	}
}

func TestRemoveStaleAndCommitPvtDataOfOldBlocks(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testValidationAndCommitOfOldPvtData(t, testEnv)
	}
}

func testValidationAndCommitOfOldPvtData(t *testing.T, env testEnv) {
	ledgerid := "testvalidationandcommitofoldpvtdata"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns1", "coll1"}: 0,
			{"ns1", "coll2"}: 0,
		},
	)
	env.init(t, ledgerid, btlPolicy)
	defer env.cleanup()
	txMgr := env.getTxMgr()
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns1", "coll1"},
			{"ns1", "coll2"},
		},
		version.NewHeight(1, 1),
	)

	db := env.getVDB()
	updateBatch := privacyenabledstate.NewUpdateBatch()
	// all pvt data are missing
	updateBatch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key1"), util.ComputeStringHash("value1"), version.NewHeight(1, 1)) // E1
	updateBatch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key2"), util.ComputeStringHash("value2"), version.NewHeight(1, 2)) // E2
	updateBatch.HashUpdates.Put("ns1", "coll2", util.ComputeStringHash("key3"), util.ComputeStringHash("value3"), version.NewHeight(1, 2)) // E3
	updateBatch.HashUpdates.Put("ns1", "coll2", util.ComputeStringHash("key4"), util.ComputeStringHash("value4"), version.NewHeight(1, 3)) // E4
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 2)))

	updateBatch = privacyenabledstate.NewUpdateBatch()
	updateBatch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key1"), util.ComputeStringHash("new-value1"), version.NewHeight(2, 1)) // E1 is updated
	updateBatch.HashUpdates.Delete("ns1", "coll1", util.ComputeStringHash("key2"), version.NewHeight(2, 2))                                    // E2 is being deleted
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(2, 2)))

	updateBatch = privacyenabledstate.NewUpdateBatch()
	updateBatch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key1"), util.ComputeStringHash("another-new-value1"), version.NewHeight(3, 1)) // E1 is again updated
	updateBatch.HashUpdates.Put("ns1", "coll2", util.ComputeStringHash("key3"), util.ComputeStringHash("value3"), version.NewHeight(3, 2))             // E3 gets only metadata update
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(3, 2)))

	v1 := []byte("value1")
	// ns1-coll1-key1 should be rejected as it is updated in the future by Blk2Tx1
	pvtDataBlk1Tx1 := producePvtdata(t, 1, []string{"ns1:coll1"}, []string{"key1"}, [][]byte{v1})
	// ns1-coll2-key3 should be accepted but ns1-coll1-key2
	// should be rejected as it is updated in the future by Blk2Tx2
	v2 := []byte("value2")
	v3 := []byte("value3")
	pvtDataBlk1Tx2 := producePvtdata(t, 2, []string{"ns1:coll1", "ns1:coll2"}, []string{"key2", "key3"}, [][]byte{v2, v3})
	// ns1-coll2-key4 should be accepted
	v4 := []byte("value4")
	pvtDataBlk1Tx3 := producePvtdata(t, 3, []string{"ns1:coll2"}, []string{"key4"}, [][]byte{v4})

	nv1 := []byte("new-value1")
	// ns1-coll1-key1 should be rejected as it is updated in the future by Blk3Tx1
	pvtDataBlk2Tx1 := producePvtdata(t, 1, []string{"ns1:coll1"}, []string{"key1"}, [][]byte{nv1})
	// ns1-coll1-key2 should be accepted -- a delete operation
	pvtDataBlk2Tx2 := producePvtdata(t, 2, []string{"ns1:coll1"}, []string{"key2"}, [][]byte{nil})

	anv1 := []byte("another-new-value1")
	// ns1-coll1-key1 should be accepted
	pvtDataBlk3Tx1 := producePvtdata(t, 1, []string{"ns1:coll1"}, []string{"key1"}, [][]byte{anv1})
	// ns1-coll2-key3 should be accepted -- assume that only metadata is being updated
	pvtDataBlk3Tx2 := producePvtdata(t, 2, []string{"ns1:coll2"}, []string{"key3"}, [][]byte{v3})

	blocksPvtData := map[uint64][]*ledger.TxPvtData{
		1: {
			pvtDataBlk1Tx1,
			pvtDataBlk1Tx2,
			pvtDataBlk1Tx3,
		},
		2: {
			pvtDataBlk2Tx1,
			pvtDataBlk2Tx2,
		},
		3: {
			pvtDataBlk3Tx1,
			pvtDataBlk3Tx2,
		},
	}

	err := txMgr.RemoveStaleAndCommitPvtDataOfOldBlocks(blocksPvtData)
	require.NoError(t, err)

	vv, err := db.GetPrivateData("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Equal(t, anv1, vv.Value) // last updated value

	vv, err = db.GetPrivateData("ns1", "coll1", "key2")
	require.NoError(t, err)
	require.Nil(t, vv) // deleted

	vv, err = db.GetPrivateData("ns1", "coll2", "key3")
	require.NoError(t, err)
	require.Equal(t, v3, vv.Value)
	require.Equal(t, version.NewHeight(3, 2), vv.Version) // though we passed with version {1,2}, we should get {3,2} due to metadata update

	vv, err = db.GetPrivateData("ns1", "coll2", "key4")
	require.NoError(t, err)
	require.Equal(t, v4, vv.Value)
}

func TestTxSimulatorMissingPvtdata(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "TestTxSimulatorUnsupportedTxQueries", nil)
	defer testEnv.cleanup()

	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns1", "coll1"},
			{"ns1", "coll2"},
			{"ns1", "coll3"},
			{"ns1", "coll4"},
		},
		version.NewHeight(1, 1),
	)

	db := testEnv.getVDB()
	updateBatch := privacyenabledstate.NewUpdateBatch()
	updateBatch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key1"), util.ComputeStringHash("value1"), version.NewHeight(1, 1))
	updateBatch.PvtUpdates.Put("ns1", "coll1", "key1", []byte("value1"), version.NewHeight(1, 1))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 1)))

	verifyPvtKeyValue(t, txMgr, "ns1", "coll1", "key1", []byte("value1"))

	updateBatch = privacyenabledstate.NewUpdateBatch()
	updateBatch.HashUpdates.Put("ns1", "coll1", util.ComputeStringHash("key1"), util.ComputeStringHash("value1"), version.NewHeight(2, 1))
	updateBatch.HashUpdates.Put("ns1", "coll2", util.ComputeStringHash("key2"), util.ComputeStringHash("value2"), version.NewHeight(2, 1))
	updateBatch.HashUpdates.Put("ns1", "coll3", util.ComputeStringHash("key3"), util.ComputeStringHash("value3"), version.NewHeight(2, 1))
	updateBatch.PvtUpdates.Put("ns1", "coll3", "key3", []byte("value3"), version.NewHeight(2, 1))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(2, 1)))

	verifyPvtKeyVersionStale(t, txMgr, "ns1", "coll1", "key1")
	verifyPvtKeyVersionStale(t, txMgr, "ns1", "coll2", "key2")
	verifyPvtKeyValue(t, txMgr, "ns1", "coll3", "key3", []byte("value3"))
	verifyPvtKeyValue(t, txMgr, "ns1", "coll4", "key4", nil)
}

func TestRemoveStaleAndCommitPvtDataOfOldBlocksWithExpiry(t *testing.T) {
	ledgerid := "TestTxSimulatorMissingPvtdataExpiry"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1,
		},
	)
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, ledgerid, btlPolicy)
	defer testEnv.cleanup()

	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr,
		[]collConfigkey{
			{"ns", "coll"},
		},
		version.NewHeight(1, 1),
	)

	bg, _ := testutil.NewBlockGenerator(t, ledgerid, false)

	// storing hashed data but the pvt key is missing
	// stored pvt key would get expired and purged while committing block 3
	blkAndPvtdata := prepareNextBlockForTest(t, txMgr, bg, "txid-1",
		map[string]string{"pubkey1": "pub-value1"}, map[string]string{"pvtkey1": "pvt-value1"}, true)
	_, _, _, err := txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	// committing block 1
	require.NoError(t, txMgr.Commit())

	// pvt data should not exist
	verifyPvtKeyVersionStale(t, txMgr, "ns", "coll", "pvtkey1")

	// committing pvt data of block 1
	v1 := []byte("pvt-value1")
	pvtDataBlk1Tx1 := producePvtdata(t, 1, []string{"ns:coll"}, []string{"pvtkey1"}, [][]byte{v1})
	blocksPvtData := map[uint64][]*ledger.TxPvtData{
		1: {
			pvtDataBlk1Tx1,
		},
	}
	err = txMgr.RemoveStaleAndCommitPvtDataOfOldBlocks(blocksPvtData)
	require.NoError(t, err)

	// pvt data should exist
	verifyPvtKeyValue(t, txMgr, "ns", "coll", "pvtkey1", v1)

	// storing hashed data but the pvt key is missing
	// stored pvt key would get expired and purged while committing block 4
	blkAndPvtdata = prepareNextBlockForTest(t, txMgr, bg, "txid-2",
		map[string]string{"pubkey2": "pub-value2"}, map[string]string{"pvtkey2": "pvt-value2"}, true)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	// committing block 2
	require.NoError(t, txMgr.Commit())

	// pvt data should not exist
	verifyPvtKeyVersionStale(t, txMgr, "ns", "coll", "pvtkey2")

	blkAndPvtdata = prepareNextBlockForTest(t, txMgr, bg, "txid-3",
		map[string]string{"pubkey3": "pub-value3"}, nil, false)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	// committing block 3
	require.NoError(t, txMgr.Commit())

	// prepareForExpiringKey must have selected the pvtkey2 as it would
	// get expired during next block commit

	// committing pvt data of block 2
	v2 := []byte("pvt-value2")
	pvtDataBlk2Tx1 := producePvtdata(t, 1, []string{"ns:coll"}, []string{"pvtkey2"}, [][]byte{v2})
	blocksPvtData = map[uint64][]*ledger.TxPvtData{
		2: {
			pvtDataBlk2Tx1,
		},
	}

	err = txMgr.RemoveStaleAndCommitPvtDataOfOldBlocks(blocksPvtData)
	require.NoError(t, err)

	// pvt data should exist
	verifyPvtKeyValue(t, txMgr, "ns", "coll", "pvtkey2", v2)

	blkAndPvtdata = prepareNextBlockForTest(t, txMgr, bg, "txid-4",
		map[string]string{"pubkey4": "pub-value4"}, nil, false)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	// committing block 4 and should purge pvtkey2
	require.NoError(t, txMgr.Commit())
	verifyPvtKeyValue(t, txMgr, "ns", "coll", "pvtkey2", nil)
}

func verifyPvtKeyVersionStale(t *testing.T, txMgr *LockBasedTxMgr, ns, coll, key string) {
	simulator, _ := txMgr.NewTxSimulator("tx-tmp")
	defer simulator.Done()
	_, err := simulator.GetPrivateData(ns, coll, key)
	require.Contains(t, err.Error(), "private data matching public hash version is not available")
}

func verifyPvtKeyValue(t *testing.T, txMgr *LockBasedTxMgr, ns, coll, key string, expectedValue []byte) {
	simulator, _ := txMgr.NewTxSimulator("tx-tmp")
	defer simulator.Done()
	pvtValue, err := simulator.GetPrivateData(ns, coll, key)
	require.NoError(t, err)
	require.Equal(t, expectedValue, pvtValue)
}

func TestDeleteOnCursor(t *testing.T) {
	cID := "cid"
	env := testEnvsMap[levelDBtestEnvName]
	env.init(t, "TestDeleteOnCursor", nil)
	defer env.cleanup()

	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)

	// Simulate and commit tx1 to populate sample data (key_001 through key_010)
	s, _ := txMgr.NewTxSimulator("test_tx1")
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		require.NoError(t, s.SetState(cID, k, v))
	}
	s.Done()
	txRWSet1, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// simulate and commit tx2 that reads keys key_001 through key_004 and deletes them one by one (in a loop - itr.Next() followed by Delete())
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	itr2, _ := s2.GetStateRangeScanIterator(cID, createTestKey(1), createTestKey(5))
	for i := 1; i <= 4; i++ {
		kv, err := itr2.Next()
		require.NoError(t, err)
		require.NotNil(t, kv)
		key := kv.(*queryresult.KV).Key
		require.NoError(t, s2.DeleteState(cID, key))
	}
	itr2.Close()
	s2.Done()
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)

	// simulate tx3 to verify that the keys key_001 through key_004 got deleted
	s3, _ := txMgr.NewTxSimulator("test_tx3")
	itr3, _ := s3.GetStateRangeScanIterator(cID, createTestKey(1), createTestKey(10))
	kv, err := itr3.Next()
	require.NoError(t, err)
	require.NotNil(t, kv)
	key := kv.(*queryresult.KV).Key
	require.Equal(t, "key_005", key)
	itr3.Close()
	s3.Done()
}

func TestTxSimulatorMissingPvtdataExpiry(t *testing.T) {
	ledgerid := "TestTxSimulatorMissingPvtdataExpiry"
	testEnv := testEnvsMap[levelDBtestEnvName]
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1,
		},
	)
	testEnv.init(t, ledgerid, btlPolicy)
	defer testEnv.cleanup()

	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr, []collConfigkey{{"ns", "coll"}}, version.NewHeight(1, 1))

	bg, _ := testutil.NewBlockGenerator(t, ledgerid, false)

	blkAndPvtdata := prepareNextBlockForTest(t, txMgr, bg, "txid-1",
		map[string]string{"pubkey1": "pub-value1"}, map[string]string{"pvtkey1": "pvt-value1"}, false)
	_, _, _, err := txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())

	verifyPvtKeyValue(t, txMgr, "ns", "coll", "pvtkey1", []byte("pvt-value1"))

	blkAndPvtdata = prepareNextBlockForTest(t, txMgr, bg, "txid-2",
		map[string]string{"pubkey1": "pub-value2"}, map[string]string{"pvtkey2": "pvt-value2"}, false)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())
	verifyPvtKeyValue(t, txMgr, "ns", "coll", "pvtkey1", []byte("pvt-value1"))

	blkAndPvtdata = prepareNextBlockForTest(t, txMgr, bg, "txid-2",
		map[string]string{"pubkey1": "pub-value3"}, map[string]string{"pvtkey3": "pvt-value3"}, false)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())
	verifyPvtKeyValue(t, txMgr, "ns", "coll", "pvtkey1", nil)
}

func TestTxWithPubMetadata(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testLedgerID := "testtxwithpubmetadata"
		testEnv.init(t, testLedgerID, nil)
		testTxWithPubMetadata(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxWithPubMetadata(t *testing.T, env testEnv) {
	namespace := "testns"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)

	// Simulate and commit tx1 - set val and metadata for key1 and key2. Set only metadata for key3
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	key1, value1, metadata1 := "key1", []byte("value1"), map[string][]byte{"entry1": []byte("meatadata1-entry1")}
	key2, value2, metadata2 := "key2", []byte("value2"), map[string][]byte{"entry1": []byte("meatadata2-entry1")}
	key3, metadata3 := "key3", map[string][]byte{"entry1": []byte("meatadata3-entry")}

	require.NoError(t, s1.SetState(namespace, key1, value1))
	require.NoError(t, s1.SetStateMetadata(namespace, key1, metadata1))
	require.NoError(t, s1.SetState(namespace, key2, value2))
	require.NoError(t, s1.SetStateMetadata(namespace, key2, metadata2))
	require.NoError(t, s1.SetStateMetadata(namespace, key3, metadata3))
	s1.Done()
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// Run query - key1 and key2 should return both value and metadata. Key3 should still be non-exsting in db
	qe, _ := txMgr.NewQueryExecutor("test_tx2")
	checkTestQueryResults(t, qe, namespace, key1, value1, metadata1)
	checkTestQueryResults(t, qe, namespace, key2, value2, metadata2)
	checkTestQueryResults(t, qe, namespace, key3, nil, nil)
	qe.Done()

	// Simulate and commit tx3 - update metadata for key1 and delete metadata for key2
	updatedMetadata1 := map[string][]byte{"entry1": []byte("meatadata1-entry1"), "entry2": []byte("meatadata1-entry2")}
	s2, _ := txMgr.NewTxSimulator("test_tx3")
	require.NoError(t, s2.SetStateMetadata(namespace, key1, updatedMetadata1))
	require.NoError(t, s2.DeleteStateMetadata(namespace, key2))
	s2.Done()
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)

	// Run query - key1 should return updated metadata. Key2 should return 'nil' metadata
	qe, _ = txMgr.NewQueryExecutor("test_tx4")
	checkTestQueryResults(t, qe, namespace, key1, value1, updatedMetadata1)
	checkTestQueryResults(t, qe, namespace, key2, value2, nil)
	qe.Done()
}

func TestTxWithPvtdataMetadata(t *testing.T) {
	ledgerid, ns, coll := "testtxwithpvtdatametadata", "ns", "coll"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1000,
		},
	)
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t, ledgerid, btlPolicy)
		testTxWithPvtdataMetadata(t, testEnv, ns, coll)
		testEnv.cleanup()
	}
}

func testTxWithPvtdataMetadata(t *testing.T, env testEnv, ns, coll string) {
	ledgerid := "testtxwithpvtdatametadata"
	txMgr := env.getTxMgr()
	bg, _ := testutil.NewBlockGenerator(t, ledgerid, false)

	populateCollConfigForTest(t, txMgr, []collConfigkey{{"ns", "coll"}}, version.NewHeight(1, 1))

	// Simulate and commit tx1 - set val and metadata for key1 and key2. Set only metadata for key3
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	key1, value1, metadata1 := "key1", []byte("value1"), map[string][]byte{"entry1": []byte("meatadata1-entry1")}
	key2, value2, metadata2 := "key2", []byte("value2"), map[string][]byte{"entry1": []byte("meatadata2-entry1")}
	key3, metadata3 := "key3", map[string][]byte{"entry1": []byte("meatadata3-entry")}
	require.NoError(t, s1.SetPrivateData(ns, coll, key1, value1))
	require.NoError(t, s1.SetPrivateDataMetadata(ns, coll, key1, metadata1))
	require.NoError(t, s1.SetPrivateData(ns, coll, key2, value2))
	require.NoError(t, s1.SetPrivateDataMetadata(ns, coll, key2, metadata2))
	require.NoError(t, s1.SetPrivateDataMetadata(ns, coll, key3, metadata3))
	s1.Done()

	blkAndPvtdata1, _ := prepareNextBlockForTestFromSimulator(t, bg, s1)
	_, _, _, err := txMgr.ValidateAndPrepare(blkAndPvtdata1, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())

	// Run query - key1 and key2 should return both value and metadata. Key3 should still be non-exsting in db
	qe, _ := txMgr.NewQueryExecutor("test_tx2")
	checkPvtdataTestQueryResults(t, qe, ns, coll, key1, value1, metadata1)
	checkPvtdataTestQueryResults(t, qe, ns, coll, key2, value2, metadata2)
	checkPvtdataTestQueryResults(t, qe, ns, coll, key3, nil, nil)
	qe.Done()

	// Simulate and commit tx3 - update metadata for key1 and delete metadata for key2
	updatedMetadata1 := map[string][]byte{"entry1": []byte("meatadata1-entry1"), "entry2": []byte("meatadata1-entry2")}
	s2, _ := txMgr.NewTxSimulator("test_tx3")
	require.NoError(t, s2.SetPrivateDataMetadata(ns, coll, key1, updatedMetadata1))
	require.NoError(t, s2.DeletePrivateDataMetadata(ns, coll, key2))
	s2.Done()

	blkAndPvtdata2, _ := prepareNextBlockForTestFromSimulator(t, bg, s2)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata2, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())

	// Run query - key1 should return updated metadata. Key2 should return 'nil' metadata
	qe, _ = txMgr.NewQueryExecutor("test_tx4")
	checkPvtdataTestQueryResults(t, qe, ns, coll, key1, value1, updatedMetadata1)
	checkPvtdataTestQueryResults(t, qe, ns, coll, key2, value2, nil)
	qe.Done()
}

func prepareNextBlockForTest(t *testing.T, txMgr *LockBasedTxMgr, bg *testutil.BlockGenerator,
	txid string, pubKVs map[string]string, pvtKVs map[string]string, isMissing bool) *ledger.BlockAndPvtData {
	simulator, _ := txMgr.NewTxSimulator(txid)
	// simulating transaction
	for k, v := range pubKVs {
		require.NoError(t, simulator.SetState("ns", k, []byte(v)))
	}
	for k, v := range pvtKVs {
		require.NoError(t, simulator.SetPrivateData("ns", "coll", k, []byte(v)))
	}
	simulator.Done()
	if isMissing {
		return prepareNextBlockForTestFromSimulatorWithMissingData(t, bg, simulator, txid, 1, "ns", "coll", true)
	}
	nb, _ := prepareNextBlockForTestFromSimulator(t, bg, simulator)
	return nb
}

func prepareNextBlockForTestFromSimulator(t *testing.T, bg *testutil.BlockGenerator, simulator ledger.TxSimulator) (*ledger.BlockAndPvtData, *ledger.TxSimulationResults) {
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block := bg.NextBlock([][]byte{pubSimBytes})
	return &ledger.BlockAndPvtData{
		Block:   block,
		PvtData: ledger.TxPvtDataMap{0: {SeqInBlock: 0, WriteSet: simRes.PvtSimulationResults}},
	}, simRes
}

func prepareNextBlockForTestFromSimulatorWithMissingData(t *testing.T, bg *testutil.BlockGenerator, simulator ledger.TxSimulator,
	txid string, txNum uint64, ns, coll string, isEligible bool) *ledger.BlockAndPvtData {
	simRes, _ := simulator.GetTxSimulationResults()
	pubSimBytes, _ := simRes.GetPubSimulationBytes()
	block := bg.NextBlock([][]byte{pubSimBytes})
	missingData := make(ledger.TxMissingPvtData)
	missingData.Add(txNum, ns, coll, isEligible)
	return &ledger.BlockAndPvtData{Block: block, MissingPvtData: missingData}
}

func checkTestQueryResults(t *testing.T, qe ledger.QueryExecutor, ns, key string,
	expectedVal []byte, expectedMetadata map[string][]byte) {
	committedVal, err := qe.GetState(ns, key)
	require.NoError(t, err)
	require.Equal(t, expectedVal, committedVal)

	committedMetadata, err := qe.GetStateMetadata(ns, key)
	require.NoError(t, err)
	require.Equal(t, expectedMetadata, committedMetadata)
	t.Logf("key=%s, value=%s, metadata=%s", key, committedVal, committedMetadata)
}

func checkPvtdataTestQueryResults(t *testing.T, qe ledger.QueryExecutor, ns, coll, key string,
	expectedVal []byte, expectedMetadata map[string][]byte) {
	committedVal, err := qe.GetPrivateData(ns, coll, key)
	require.NoError(t, err)
	require.Equal(t, expectedVal, committedVal)

	committedMetadata, err := qe.GetPrivateDataMetadata(ns, coll, key)
	require.NoError(t, err)
	require.Equal(t, expectedMetadata, committedMetadata)
	t.Logf("key=%s, value=%s, metadata=%s", key, committedVal, committedMetadata)
}

func TestName(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	testEnv.init(t, "testLedger", nil)
	defer testEnv.cleanup()
	txMgr := testEnv.getTxMgr()
	require.Equal(t, "state", txMgr.Name())
}

func TestTxSimulatorWithStateBasedEndorsement(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Run(testEnv.getName(), func(t *testing.T) {
			testEnv.init(t, "testtxsimulatorwithdtstebasedendorsement", nil)
			testTxSimulatorWithStateBasedEndorsement(t, testEnv)
			testEnv.cleanup()
		})
	}
}

func testTxSimulatorWithStateBasedEndorsement(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	sbe1 := map[string][]byte{peer.MetaDataKeys_VALIDATION_PARAMETER.String(): []byte("SBE1")}
	sbe2 := map[string][]byte{peer.MetaDataKeys_VALIDATION_PARAMETER.String(): []byte("SBE2")}
	sbe3 := map[string][]byte{peer.MetaDataKeys_VALIDATION_PARAMETER.String(): []byte("SBE3")}

	// simulate tx1
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	require.NoError(t, s1.SetState("ns1", "key1", []byte("value1")))
	require.NoError(t, s1.SetState("ns1", "key2", []byte("value2")))
	require.NoError(t, s1.SetStateMetadata("ns1", "key2", sbe1))
	require.NoError(t, s1.SetState("ns2", "key3", []byte("value3")))
	require.NoError(t, s1.SetStateMetadata("ns2", "key3", sbe2))
	require.NoError(t, s1.SetState("ns2", "key4", []byte("value4")))
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1.PubSimulationResults)

	// simulate tx2 that make changes to existing data and updates a key policy
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	require.NoError(t, s2.SetState("ns1", "key1", []byte("value1b")))
	require.NoError(t, s2.SetState("ns1", "key2", []byte("value2b")))
	require.NoError(t, s2.SetStateMetadata("ns2", "key3", sbe3))
	require.NoError(t, s2.SetState("ns2", "key4", []byte("value4b")))
	s2.Done()
	// validate and commit RWset for tx2
	txRWSet2, err := s2.GetTxSimulationResults()
	require.NoError(t, err)
	txMgrHelper.validateAndCommitRWSet(txRWSet2.PubSimulationResults)
	// check the metadata are captured
	metadata := ledger.WritesetMetadata{}
	metadata.Add("ns1", "", "key1", nil)
	metadata.Add("ns1", "", "key2", sbe1)
	metadata.Add("ns2", "", "key3", sbe2)
	metadata.Add("ns2", "", "key4", nil)
	require.Equal(t, metadata, txRWSet2.WritesetMetadata)

	// simulate tx3
	s3, _ := txMgr.NewTxSimulator("test_tx3")
	require.NoError(t, s3.SetState("ns1", "key1", []byte("value1c")))
	require.NoError(t, s3.SetState("ns1", "key2", []byte("value2c")))
	require.NoError(t, s3.SetState("ns2", "key3", []byte("value3c")))
	require.NoError(t, s3.SetState("ns2", "key4", []byte("value4c")))
	s3.Done()
	txRWSet3, err := s3.GetTxSimulationResults()
	require.NoError(t, err)

	// check the metadata are captured
	metadata = ledger.WritesetMetadata{}
	metadata.Add("ns1", "", "key1", nil)
	metadata.Add("ns1", "", "key2", sbe1)
	metadata.Add("ns2", "", "key3", sbe3)
	metadata.Add("ns2", "", "key4", nil)
	require.Equal(t, metadata, txRWSet3.WritesetMetadata)
}

func TestTxSimulatorWithPrivateDataStateBasedEndorsement(t *testing.T) {
	ledgerid, ns, coll := "testtxwithprivatedatastatebasedendorsement", "ns1", "coll1"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{ns, coll}: 1000,
		},
	)
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t, ledgerid, btlPolicy)
		testTxSimulatorWithPrivateDataStateBasedEndorsement(t, testEnv, ns, coll)
		testEnv.cleanup()
	}
}

func testTxSimulatorWithPrivateDataStateBasedEndorsement(t *testing.T, env testEnv, ns, coll string) {
	ledgerid := "testtxwithprivatedatastatebasedendorsement"
	txMgr := env.getTxMgr()
	bg, _ := testutil.NewBlockGenerator(t, ledgerid, false)

	populateCollConfigForTest(t, txMgr, []collConfigkey{{ns, coll}}, version.NewHeight(1, 1))

	sbe1 := map[string][]byte{peer.MetaDataKeys_VALIDATION_PARAMETER.String(): []byte("SBE1")}
	sbe2 := map[string][]byte{peer.MetaDataKeys_VALIDATION_PARAMETER.String(): []byte("SBE2")}

	// Simulate and commit tx1 - set private data and key policy
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	require.NoError(t, s1.SetPrivateData(ns, coll, "key1", []byte("private_value1")))
	require.NoError(t, s1.SetPrivateDataMetadata(ns, coll, "key1", sbe1))
	s1.Done()

	blkAndPvtdata1, _ := prepareNextBlockForTestFromSimulator(t, bg, s1)
	_, _, _, err := txMgr.ValidateAndPrepare(blkAndPvtdata1, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())

	// simulate tx2 that make changes to existing data and updates a key policy
	s2, _ := txMgr.NewTxSimulator("test_tx2")
	require.NoError(t, s2.SetPrivateData(ns, coll, "key1", []byte("private_value2")))
	require.NoError(t, s2.SetPrivateDataMetadata(ns, coll, "key1", sbe2))
	s2.Done()

	blkAndPvtdata2, simRes2 := prepareNextBlockForTestFromSimulator(t, bg, s2)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata2, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())
	// check the metadata are captured
	metadata := ledger.WritesetMetadata{}
	metadata.Add(ns, coll, "key1", sbe1)
	require.Equal(t, metadata, simRes2.WritesetMetadata)

	// simulate tx3 that make changes to existing data
	s3, _ := txMgr.NewTxSimulator("test_tx3")
	require.NoError(t, s3.SetPrivateData(ns, coll, "key1", []byte("private_value2")))
	s3.Done()

	blkAndPvtdata3, simRes3 := prepareNextBlockForTestFromSimulator(t, bg, s3)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata3, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())
	// check the metadata are captured
	metadata = ledger.WritesetMetadata{}
	metadata.Add(ns, coll, "key1", sbe2)
	require.Equal(t, metadata, simRes3.WritesetMetadata)

	// simulate tx4 that purges existing private data
	s4, _ := txMgr.NewTxSimulator("test_tx4")
	require.NoError(t, s4.PurgePrivateData(ns, coll, "key1"))
	s4.Done()

	blkAndPvtdata4, simRes4 := prepareNextBlockForTestFromSimulator(t, bg, s4)
	_, _, _, err = txMgr.ValidateAndPrepare(blkAndPvtdata4, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())
	// check the metadata are captured
	metadata = ledger.WritesetMetadata{}
	metadata.Add(ns, coll, "key1", sbe2)
	require.Equal(t, metadata, simRes4.WritesetMetadata)
}
