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
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

// TestBasicRW tests basic read-write
func TestBasicRW(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	// Test retrieval of non-existent key - returns nil rather than error
	// For more details see https://github.com/hyperledger-archives/fabric/issues/936.
	val, err := db.GetState("ns", "key1")
	testutil.AssertNoError(t, err, "Should receive nil rather than error upon reading non existent key")
	testutil.AssertNil(t, val)

	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte{}, Version: version.NewHeight(1, 4)}
	batch.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch.Put("ns1", "key2", vv2.Value, vv2.Version)
	batch.Put("ns2", "key3", vv3.Value, vv3.Version)
	batch.Put("ns2", "key4", vv4.Value, vv4.Version)
	savePoint := version.NewHeight(2, 5)
	db.ApplyUpdates(batch, savePoint)

	vv, _ := db.GetState("ns1", "key1")
	testutil.AssertEquals(t, vv, &vv1)

	vv, _ = db.GetState("ns2", "key4")
	testutil.AssertEquals(t, vv, &vv4)

	sp, err := db.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, sp, savePoint)
}

// TestMultiDBBasicRW tests basic read-write on multiple dbs
func TestMultiDBBasicRW(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db1, err := dbProvider.GetDBHandle("TestDB1")
	testutil.AssertNoError(t, err, "")

	db2, err := dbProvider.GetDBHandle("TestDB2")
	testutil.AssertNoError(t, err, "")

	batch1 := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1_db1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2_db1"), Version: version.NewHeight(1, 2)}
	batch1.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch1.Put("ns1", "key2", vv2.Value, vv2.Version)
	savePoint1 := version.NewHeight(1, 2)
	db1.ApplyUpdates(batch1, savePoint1)

	batch2 := statedb.NewUpdateBatch()
	vv3 := statedb.VersionedValue{Value: []byte("value1_db2"), Version: version.NewHeight(1, 4)}
	vv4 := statedb.VersionedValue{Value: []byte("value2_db2"), Version: version.NewHeight(1, 5)}
	batch2.Put("ns1", "key1", vv3.Value, vv3.Version)
	batch2.Put("ns1", "key2", vv4.Value, vv4.Version)
	savePoint2 := version.NewHeight(1, 5)
	db2.ApplyUpdates(batch2, savePoint2)

	vv, _ := db1.GetState("ns1", "key1")
	testutil.AssertEquals(t, vv, &vv1)

	sp, err := db1.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, sp, savePoint1)

	vv, _ = db2.GetState("ns1", "key1")
	testutil.AssertEquals(t, vv, &vv3)

	sp, err = db2.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, sp, savePoint2)
}

// TestDeletes tests deteles
func TestDeletes(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 4)}

	batch.Put("ns", "key1", vv1.Value, vv1.Version)
	batch.Put("ns", "key2", vv2.Value, vv2.Version)
	batch.Put("ns", "key3", vv2.Value, vv3.Version)
	batch.Put("ns", "key4", vv2.Value, vv4.Version)
	batch.Delete("ns", "key3", version.NewHeight(1, 5))
	savePoint := version.NewHeight(1, 5)
	err = db.ApplyUpdates(batch, savePoint)
	testutil.AssertNoError(t, err, "")
	vv, _ := db.GetState("ns", "key2")
	testutil.AssertEquals(t, vv, &vv2)

	vv, err = db.GetState("ns", "key3")
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, vv)

	batch = statedb.NewUpdateBatch()
	batch.Delete("ns", "key2", version.NewHeight(1, 6))
	err = db.ApplyUpdates(batch, savePoint)
	testutil.AssertNoError(t, err, "")
	vv, err = db.GetState("ns", "key2")
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, vv)
}

// TestIterator tests the iterator
func TestIterator(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 4))
	batch.Put("ns2", "key5", []byte("value5"), version.NewHeight(1, 5))
	batch.Put("ns2", "key6", []byte("value6"), version.NewHeight(1, 6))
	batch.Put("ns3", "key7", []byte("value7"), version.NewHeight(1, 7))
	savePoint := version.NewHeight(2, 5)
	db.ApplyUpdates(batch, savePoint)

	itr1, _ := db.GetStateRangeScanIterator("ns1", "key1", "")
	testItr(t, itr1, []string{"key1", "key2", "key3", "key4"})

	itr2, _ := db.GetStateRangeScanIterator("ns1", "key2", "key3")
	testItr(t, itr2, []string{"key2"})

	itr3, _ := db.GetStateRangeScanIterator("ns1", "", "")
	testItr(t, itr3, []string{"key1", "key2", "key3", "key4"})

	itr4, _ := db.GetStateRangeScanIterator("ns2", "", "")
	testItr(t, itr4, []string{"key5", "key6"})
}

func testItr(t *testing.T, itr statedb.ResultsIterator, expectedKeys []string) {
	defer itr.Close()
	for _, expectedKey := range expectedKeys {
		queryResult, _ := itr.Next()
		vkv := queryResult.(*statedb.VersionedKV)
		key := vkv.Key
		testutil.AssertEquals(t, key, expectedKey)
	}
	last, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, last)
}

// TestQuery tests queries
func TestQuery(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	jsonValue1 := "{\"asset_name\": \"marble1\",\"color\": \"blue\",\"size\": 35,\"owner\": \"tom\"}"
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := "{\"asset_name\": \"marble1\",\"color\": \"blue\",\"size\": 35,\"owner\": \"jerry\"}"
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 2))
	savePoint := version.NewHeight(2, 5)
	db.ApplyUpdates(batch, savePoint)

	// query for owner=jerry
	itr, err := db.ExecuteQuery("{\"selector\":{\"owner\":\"jerry\"}}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord := queryResult1.(*statedb.VersionedQueryRecord)
	stringRecord := string(versionedQueryRecord.Record)
	bFoundJerry := strings.Contains(stringRecord, "jerry")
	testutil.AssertEquals(t, bFoundJerry, true)

	// verify no more results
	queryResult2, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query using bad query string
	itr, err = db.ExecuteQuery("this is an invalid query string")
	testutil.AssertError(t, err, "Should have received an error for invalid query string")

	// query returns 0 records
	itr, err = db.ExecuteQuery("{\"selector\":{\"owner\":\"not_a_valid_name\"}}")
	testutil.AssertNoError(t, err, "")

	// verify no results
	queryResult3, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult3)

	// query with fields
	itr, err = db.ExecuteQuery("{\"selector\":{\"owner\":\"jerry\"},\"fields\": [\"owner\", \"asset_name\", \"color\", \"size\"]}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedQueryRecord)
	stringRecord = string(versionedQueryRecord.Record)
	bFoundJerry = strings.Contains(stringRecord, "jerry")
	testutil.AssertEquals(t, bFoundJerry, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

}
