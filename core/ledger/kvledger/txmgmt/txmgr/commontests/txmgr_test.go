/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package commontests

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

func TestTxSimulatorWithNoExistingData(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testTxSimulatorWithNoExistingData(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxSimulatorWithNoExistingData(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	s, _ := txMgr.NewTxSimulator()
	value, err := s.GetState("ns1", "key1")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error in GetState(): %s", err))
	testutil.AssertNil(t, value)

	s.SetState("ns1", "key1", []byte("value1"))
	s.SetState("ns1", "key2", []byte("value2"))
	s.SetState("ns2", "key3", []byte("value3"))
	s.SetState("ns2", "key4", []byte("value4"))

	value, _ = s.GetState("ns2", "key3")
	testutil.AssertNil(t, value)
}

func TestTxSimulatorWithExistingData(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testTxSimulatorWithExistingData(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxSimulatorWithExistingData(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator()
	s1.SetState("ns1", "key1", []byte("value1"))
	s1.SetState("ns1", "key2", []byte("value2"))
	s1.SetState("ns2", "key3", []byte("value3"))
	s1.SetState("ns2", "key4", []byte("value4"))
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1)

	// simulate tx2 that make changes to existing data
	s2, _ := txMgr.NewTxSimulator()
	value, _ := s2.GetState("ns1", "key1")
	testutil.AssertEquals(t, value, []byte("value1"))
	s2.SetState("ns1", "key1", []byte("value1_1"))
	s2.DeleteState("ns2", "key3")
	value, _ = s2.GetState("ns1", "key1")
	testutil.AssertEquals(t, value, []byte("value1"))
	s2.Done()
	// validate and commit RWset for tx2
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2)

	// simulate tx3
	s3, _ := txMgr.NewTxSimulator()
	value, _ = s3.GetState("ns1", "key1")
	testutil.AssertEquals(t, value, []byte("value1_1"))
	value, _ = s3.GetState("ns2", "key3")
	testutil.AssertEquals(t, value, nil)
	s3.Done()

	// verify the versions of keys in persistence
	vv, _ := env.getVDB().GetState("ns1", "key1")
	//TODO re-enable after adding couch version wrapper
	//testutil.AssertEquals(t, vv.Version, version.NewHeight(2, 1))
	vv, _ = env.getVDB().GetState("ns1", "key2")
	testutil.AssertEquals(t, vv.Version, version.NewHeight(1, 1))
}

func TestTxValidation(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testTxValidation(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxValidation(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator()
	s1.SetState("ns1", "key1", []byte("value1"))
	s1.SetState("ns1", "key2", []byte("value2"))
	s1.SetState("ns2", "key3", []byte("value3"))
	s1.SetState("ns2", "key4", []byte("value4"))
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1)

	// simulate tx2 that make changes to existing data
	s2, _ := txMgr.NewTxSimulator()
	value, _ := s2.GetState("ns1", "key1")
	testutil.AssertEquals(t, value, []byte("value1"))

	s2.SetState("ns1", "key1", []byte("value1_2"))
	s2.DeleteState("ns2", "key3")
	s2.Done()

	// simulate tx3 before committing tx2 changes. Reads and modifies the key changed by tx2
	s3, _ := txMgr.NewTxSimulator()
	s3.GetState("ns1", "key1")
	s3.SetState("ns1", "key1", []byte("value1_3"))
	s3.Done()

	// simulate tx4 before committing tx2 changes. Reads and Deletes the key changed by tx2
	s4, _ := txMgr.NewTxSimulator()
	s4.GetState("ns2", "key3")
	s4.DeleteState("ns2", "key3")
	s4.Done()

	// simulate tx5 before committing tx2 changes. Modifies and then Reads the key changed by tx2 and writes a new key
	s5, _ := txMgr.NewTxSimulator()
	s5.SetState("ns1", "key1", []byte("new_value"))
	s5.GetState("ns1", "key1")
	s5.Done()

	// simulate tx6 before committing tx2 changes. Only writes a new key, does not reads/writes a key changed by tx2
	s6, _ := txMgr.NewTxSimulator()
	s6.SetState("ns1", "new_key", []byte("new_value"))
	s6.Done()

	// validate and commit RWset for tx2
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2)
	//TODO re-enable after adding couch version wrapper
	/*
		//RWSet for tx3 and tx4 should not be invalid now
		txRWSet3, _ := s3.GetTxSimulationResults()
		txMgrHelper.checkRWsetInvalid(txRWSet3)

		txRWSet4, _ := s4.GetTxSimulationResults()
		txMgrHelper.checkRWsetInvalid(txRWSet4)

		//tx5 shold still be valid as it over-writes the key first and then reads
		txRWSet5, _ := s5.GetTxSimulationResults()
		txMgrHelper.validateAndCommitRWSet(txRWSet5)

		// tx6 should still be valid as it only writes a new key
		txRWSet6, _ := s6.GetTxSimulationResults()
		txMgrHelper.validateAndCommitRWSet(txRWSet6)
	*/
}

func TestTxPhantomValidation(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testTxPhantomValidation(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxPhantomValidation(t *testing.T, env testEnv) {
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator()
	s1.SetState("ns", "key1", []byte("value1"))
	s1.SetState("ns", "key2", []byte("value2"))
	s1.SetState("ns", "key3", []byte("value3"))
	s1.SetState("ns", "key4", []byte("value4"))
	s1.SetState("ns", "key5", []byte("value5"))
	s1.SetState("ns", "key6", []byte("value6"))
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1)

	// simulate tx2
	s2, _ := txMgr.NewTxSimulator()
	itr2, _ := s2.GetStateRangeScanIterator("ns", "key2", "key5")
	for {
		if result, _ := itr2.Next(); result == nil {
			break
		}
	}
	s2.DeleteState("ns", "key3")
	s2.Done()
	txRWSet2, _ := s2.GetTxSimulationResults()

	// simulate tx3
	s3, _ := txMgr.NewTxSimulator()
	itr3, _ := s3.GetStateRangeScanIterator("ns", "key2", "key5")
	for {
		if result, _ := itr3.Next(); result == nil {
			break
		}
	}
	s3.SetState("ns", "key3", []byte("value3_new"))
	s3.Done()
	txRWSet3, _ := s3.GetTxSimulationResults()

	// simulate tx4
	s4, _ := txMgr.NewTxSimulator()
	itr4, _ := s4.GetStateRangeScanIterator("ns", "key4", "key6")
	for {
		if result, _ := itr4.Next(); result == nil {
			break
		}
	}
	s4.SetState("ns", "key3", []byte("value3_new"))
	s4.Done()
	txRWSet4, _ := s4.GetTxSimulationResults()

	// txRWSet2 should be valid
	txMgrHelper.validateAndCommitRWSet(txRWSet2)
	// txRWSet2 makes txRWSet3 invalid as it deletes a key in the range
	txMgrHelper.checkRWsetInvalid(txRWSet3)
	// txRWSet4 should be valid as it iterates over a different range
	txMgrHelper.validateAndCommitRWSet(txRWSet4)
}

func TestIterator(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testIterator(t, testEnv, 10, 2, 7)
		testEnv.cleanup()

		testEnv.init(t)
		testIterator(t, testEnv, 10, 1, 11)
		testEnv.cleanup()

		testEnv.init(t)
		testIterator(t, testEnv, 10, 0, 0)
		testEnv.cleanup()

		testEnv.init(t)
		testIterator(t, testEnv, 10, 5, 0)
		testEnv.cleanup()

		testEnv.init(t)
		testIterator(t, testEnv, 10, 0, 5)
		testEnv.cleanup()
	}
}

func testIterator(t *testing.T, env testEnv, numKeys int, startKeyNum int, endKeyNum int) {
	cID := "cID"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	s, _ := txMgr.NewTxSimulator()
	for i := 1; i <= numKeys; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		s.SetState(cID, k, v)
	}
	s.Done()
	// validate and commit RWset
	txRWSet, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet)

	var startKey string
	var endKey string
	var begin int
	var end int

	if startKeyNum != 0 {
		begin = startKeyNum
		startKey = createTestKey(startKeyNum)
	} else {
		begin = 1 //first key in the db
		startKey = ""
	}

	if endKeyNum != 0 {
		endKey = createTestKey(endKeyNum)
		end = endKeyNum
	} else {
		endKey = ""
		end = numKeys + 1 //last key in the db
	}

	expectedCount := end - begin

	queryExecuter, _ := txMgr.NewQueryExecutor()
	itr, _ := queryExecuter.GetStateRangeScanIterator(cID, startKey, endKey)
	count := 0
	for {
		kv, _ := itr.Next()
		if kv == nil {
			break
		}
		keyNum := begin + count
		k := kv.(*ledger.KV).Key
		v := kv.(*ledger.KV).Value
		t.Logf("Retrieved k=%s, v=%s at count=%d start=%s end=%s", k, v, count, startKey, endKey)
		testutil.AssertEquals(t, k, createTestKey(keyNum))
		testutil.AssertEquals(t, v, createTestValue(keyNum))
		count++
	}
	testutil.AssertEquals(t, count, expectedCount)
}

func TestIteratorWithDeletes(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testIteratorWithDeletes(t, testEnv)
		testEnv.cleanup()
	}
}

func testIteratorWithDeletes(t *testing.T, env testEnv) {
	cID := "cID"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	s, _ := txMgr.NewTxSimulator()
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		s.SetState(cID, k, v)
	}
	s.Done()
	// validate and commit RWset
	txRWSet1, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1)

	s, _ = txMgr.NewTxSimulator()
	s.DeleteState(cID, createTestKey(4))
	s.Done()
	// validate and commit RWset
	txRWSet2, _ := s.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2)

	queryExecuter, _ := txMgr.NewQueryExecutor()
	itr, _ := queryExecuter.GetStateRangeScanIterator(cID, createTestKey(3), createTestKey(6))
	defer itr.Close()
	kv, _ := itr.Next()
	testutil.AssertEquals(t, kv.(*ledger.KV).Key, createTestKey(3))
	//TODO re-enable after adding couch delete function
	//kv, _ = itr.Next()
	//testutil.AssertEquals(t, kv.(*ledger.KV).Key, createTestKey(5))
}

func TestTxValidationWithItr(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testTxValidationWithItr(t, testEnv)
		testEnv.cleanup()
	}
}

func testTxValidationWithItr(t *testing.T, env testEnv) {
	cID := "cID"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)

	// simulate tx1
	s1, _ := txMgr.NewTxSimulator()
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		t.Logf("Adding k=[%s], v=[%s]", k, v)
		s1.SetState(cID, k, v)
	}
	s1.Done()
	// validate and commit RWset
	txRWSet1, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet1)

	// simulate tx2 that reads key_001 and key_002
	s2, _ := txMgr.NewTxSimulator()
	itr, _ := s2.GetStateRangeScanIterator(cID, createTestKey(1), createTestKey(5))
	// read key_001 and key_002
	itr.Next()
	itr.Next()
	itr.Close()
	s2.Done()

	// simulate tx3 that reads key_004 and key_005
	s3, _ := txMgr.NewTxSimulator()
	itr, _ = s3.GetStateRangeScanIterator(cID, createTestKey(4), createTestKey(6))
	// read key_001 and key_002
	itr.Next()
	itr.Next()
	itr.Close()
	s3.Done()

	// simulate tx4 before committing tx2 and tx3. Modifies a key read by tx3
	s4, _ := txMgr.NewTxSimulator()
	s4.DeleteState(cID, createTestKey(5))
	s4.Done()

	// validate and commit RWset for tx4
	txRWSet4, _ := s4.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet4)

	//TODO re-enable after adding couch version wrapper
	/*
		//RWSet tx3 should not be invalid now
		txRWSet3, _ := s3.GetTxSimulationResults()
		txMgrHelper.checkRWsetInvalid(txRWSet3)
	*/

	// tx2 should still be valid
	txRWSet2, _ := s2.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet2)

}

func TestGetSetMultipeKeys(t *testing.T) {
	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testGetSetMultipeKeys(t, testEnv)
		testEnv.cleanup()
	}
}

func testGetSetMultipeKeys(t *testing.T, env testEnv) {
	cID := "cID"
	txMgr := env.getTxMgr()
	txMgrHelper := newTxMgrTestHelper(t, txMgr)
	// simulate tx1
	s1, _ := txMgr.NewTxSimulator()
	multipleKeyMap := make(map[string][]byte)
	for i := 1; i <= 10; i++ {
		k := createTestKey(i)
		v := createTestValue(i)
		multipleKeyMap[k] = v
	}
	s1.SetStateMultipleKeys(cID, multipleKeyMap)
	s1.Done()
	// validate and commit RWset
	txRWSet, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet)
	qe, _ := txMgr.NewQueryExecutor()
	defer qe.Done()
	multipleKeys := []string{}
	for k := range multipleKeyMap {
		multipleKeys = append(multipleKeys, k)
	}
	values, _ := qe.GetStateMultipleKeys(cID, multipleKeys)
	testutil.AssertEquals(t, len(values), 10)
	for i, v := range values {
		testutil.AssertEquals(t, v, multipleKeyMap[multipleKeys[i]])
	}

	s2, _ := txMgr.NewTxSimulator()
	defer s2.Done()
	values, _ = s2.GetStateMultipleKeys(cID, multipleKeys[5:7])
	testutil.AssertEquals(t, len(values), 2)
	for i, v := range values {
		testutil.AssertEquals(t, v, multipleKeyMap[multipleKeys[i+5]])
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

//TestExecuteQueryQuery is only tested on the CouchDB testEnv
func TestExecuteQuery(t *testing.T) {

	// Query is only tested on the CouchDB testEnv
	if ledgerconfig.IsCouchDBEnabled() == true {
		testEnvs = []testEnv{&couchDBLockBasedEnv{}}
	} else {
		testEnvs = []testEnv{}
	}

	for _, testEnv := range testEnvs {
		t.Logf("Running test for TestEnv = %s", testEnv.getName())
		testEnv.init(t)
		testExecuteQuery(t, testEnv)
		testEnv.cleanup()
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

	s1, _ := txMgr.NewTxSimulator()

	s1.SetState("ns1", "key1", []byte("value1"))
	s1.SetState("ns1", "key2", []byte("value2"))
	s1.SetState("ns1", "key3", []byte("value3"))
	s1.SetState("ns1", "key4", []byte("value4"))
	s1.SetState("ns1", "key5", []byte("value5"))
	s1.SetState("ns1", "key6", []byte("value6"))
	s1.SetState("ns1", "key7", []byte("value7"))
	s1.SetState("ns1", "key8", []byte("value8"))

	s1.SetState("ns1", "key9", []byte(`{"asset_name":"marble1","color":"red","size":"25","owner":"jerry"}`))
	s1.SetState("ns1", "key10", []byte(`{"asset_name":"marble2","color":"blue","size":"10","owner":"bob"}`))
	s1.SetState("ns1", "key11", []byte(`{"asset_name":"marble3","color":"blue","size":"35","owner":"jerry"}`))
	s1.SetState("ns1", "key12", []byte(`{"asset_name":"marble4","color":"green","size":"15","owner":"bob"}`))
	s1.SetState("ns1", "key13", []byte(`{"asset_name":"marble5","color":"red","size":"35","owner":"jerry"}`))
	s1.SetState("ns1", "key14", []byte(`{"asset_name":"marble6","color":"blue","size":"25","owner":"bob"}`))

	s1.Done()

	// validate and commit RWset
	txRWSet, _ := s1.GetTxSimulationResults()
	txMgrHelper.validateAndCommitRWSet(txRWSet)

	queryExecuter, _ := txMgr.NewQueryExecutor()
	queryString := "{\"selector\":{\"owner\": {\"$eq\": \"bob\"}},\"limit\": 10,\"skip\": 0}"

	itr, _ := queryExecuter.ExecuteQuery(queryString)

	counter := 0
	for {
		queryRecord, _ := itr.Next()
		if queryRecord == nil {
			break
		}

		//Unmarshal the document to Asset structure
		assetResp := &Asset{}
		json.Unmarshal(queryRecord.(*ledger.QueryRecord).Record, &assetResp)

		//Verify the owner retrieved matches
		testutil.AssertEquals(t, assetResp.Owner, "bob")

		counter++

	}

	//Ensure the query returns 3 documents
	testutil.AssertEquals(t, counter, 3)

}
