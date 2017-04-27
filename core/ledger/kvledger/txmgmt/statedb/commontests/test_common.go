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

// TestGetStateMultipleKeys tests read for given multiple keys
func TestGetStateMultipleKeys(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testgetmultiplekeys")
	testutil.AssertNoError(t, err, "")

	// Test that savepoint is nil for a new state db
	sp, err := db.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "Error upon GetLatestSavePoint()")
	testutil.AssertNil(t, sp)

	batch := statedb.NewUpdateBatch()
	expectedValues := make([]*statedb.VersionedValue, 2)
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	expectedValues[0] = &vv1
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	expectedValues[1] = &vv2
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte{}, Version: version.NewHeight(1, 4)}
	batch.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch.Put("ns1", "key2", vv2.Value, vv2.Version)
	batch.Put("ns2", "key3", vv3.Value, vv3.Version)
	batch.Put("ns2", "key4", vv4.Value, vv4.Version)
	savePoint := version.NewHeight(2, 5)
	db.ApplyUpdates(batch, savePoint)

	actualValues, _ := db.GetStateMultipleKeys("ns1", []string{"key1", "key2"})
	testutil.AssertEquals(t, actualValues, expectedValues)
}

// TestBasicRW tests basic read-write
func TestBasicRW(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testbasicrw")
	testutil.AssertNoError(t, err, "")

	// Test that savepoint is nil for a new state db
	sp, err := db.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "Error upon GetLatestSavePoint()")
	testutil.AssertNil(t, sp)

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

	sp, err = db.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, sp, savePoint)
}

// TestMultiDBBasicRW tests basic read-write on multiple dbs
func TestMultiDBBasicRW(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db1, err := dbProvider.GetDBHandle("testmultidbbasicrw")
	testutil.AssertNoError(t, err, "")

	db2, err := dbProvider.GetDBHandle("testmultidbbasicrw2")
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
	db, err := dbProvider.GetDBHandle("testdeletes")
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
	db, err := dbProvider.GetDBHandle("testiterator")
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
	db, err := dbProvider.GetDBHandle("testquery")
	testutil.AssertNoError(t, err, "")
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	jsonValue1 := "{\"asset_name\": \"marble1\",\"color\": \"blue\",\"size\": 1,\"owner\": \"tom\"}"
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := "{\"asset_name\": \"marble2\",\"color\": \"blue\",\"size\": 2,\"owner\": \"jerry\"}"
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 2))
	jsonValue3 := "{\"asset_name\": \"marble3\",\"color\": \"blue\",\"size\": 3,\"owner\": \"fred\"}"
	batch.Put("ns1", "key3", []byte(jsonValue3), version.NewHeight(1, 3))
	jsonValue4 := "{\"asset_name\": \"marble4\",\"color\": \"blue\",\"size\": 4,\"owner\": \"martha\"}"
	batch.Put("ns1", "key4", []byte(jsonValue4), version.NewHeight(1, 4))
	jsonValue5 := "{\"asset_name\": \"marble5\",\"color\": \"blue\",\"size\": 5,\"owner\": \"fred\"}"
	batch.Put("ns1", "key5", []byte(jsonValue5), version.NewHeight(1, 5))
	jsonValue6 := "{\"asset_name\": \"marble6\",\"color\": \"blue\",\"size\": 6,\"owner\": \"elaine\"}"
	batch.Put("ns1", "key6", []byte(jsonValue6), version.NewHeight(1, 6))
	jsonValue7 := "{\"asset_name\": \"marble7\",\"color\": \"blue\",\"size\": 7,\"owner\": \"fred\"}"
	batch.Put("ns1", "key7", []byte(jsonValue7), version.NewHeight(1, 7))
	jsonValue8 := "{\"asset_name\": \"marble8\",\"color\": \"blue\",\"size\": 8,\"owner\": \"elaine\"}"
	batch.Put("ns1", "key8", []byte(jsonValue8), version.NewHeight(1, 8))
	jsonValue9 := "{\"asset_name\": \"marble9\",\"color\": \"green\",\"size\": 9,\"owner\": \"fred\"}"
	batch.Put("ns1", "key9", []byte(jsonValue9), version.NewHeight(1, 9))
	jsonValue10 := "{\"asset_name\": \"marble10\",\"color\": \"green\",\"size\": 10,\"owner\": \"mary\"}"
	batch.Put("ns1", "key10", []byte(jsonValue10), version.NewHeight(1, 10))
	jsonValue11 := "{\"asset_name\": \"marble11\",\"color\": \"cyan\",\"size\": 1000007,\"owner\": \"joe\"}"
	batch.Put("ns1", "key11", []byte(jsonValue11), version.NewHeight(1, 11))

	//add keys for a separate namespace
	batch.Put("ns2", "key1", []byte(jsonValue1), version.NewHeight(1, 12))
	batch.Put("ns2", "key2", []byte(jsonValue2), version.NewHeight(1, 13))
	batch.Put("ns2", "key3", []byte(jsonValue3), version.NewHeight(1, 14))
	batch.Put("ns2", "key4", []byte(jsonValue4), version.NewHeight(1, 15))
	batch.Put("ns2", "key5", []byte(jsonValue5), version.NewHeight(1, 16))
	batch.Put("ns2", "key6", []byte(jsonValue6), version.NewHeight(1, 17))
	batch.Put("ns2", "key7", []byte(jsonValue7), version.NewHeight(1, 18))
	batch.Put("ns2", "key8", []byte(jsonValue8), version.NewHeight(1, 19))
	batch.Put("ns2", "key9", []byte(jsonValue9), version.NewHeight(1, 20))
	batch.Put("ns2", "key10", []byte(jsonValue10), version.NewHeight(1, 21))

	savePoint := version.NewHeight(2, 22)
	db.ApplyUpdates(batch, savePoint)

	// query for owner=jerry, use namespace "ns1"
	itr, err := db.ExecuteQuery("ns1", "{\"selector\":{\"owner\":\"jerry\"}}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord := queryResult1.(*statedb.VersionedKV)
	stringRecord := string(versionedQueryRecord.Value)
	bFoundRecord := strings.Contains(stringRecord, "jerry")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query for owner=jerry, use namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", "{\"selector\":{\"owner\":\"jerry\"}}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "jerry")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query for owner=jerry, use namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", "{\"selector\":{\"owner\":\"jerry\"}}")
	testutil.AssertNoError(t, err, "")

	// verify results - should be no records
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult1)

	// query using bad query string
	itr, err = db.ExecuteQuery("ns1", "this is an invalid query string")
	testutil.AssertError(t, err, "Should have received an error for invalid query string")

	// query returns 0 records
	itr, err = db.ExecuteQuery("ns1", "{\"selector\":{\"owner\":\"not_a_valid_name\"}}")
	testutil.AssertNoError(t, err, "")

	// verify no results
	queryResult3, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult3)

	// query with fields, namespace "ns1"
	itr, err = db.ExecuteQuery("ns1", "{\"selector\":{\"owner\":\"jerry\"},\"fields\": [\"owner\", \"asset_name\", \"color\", \"size\"]}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "jerry")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query with fields, namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", "{\"selector\":{\"owner\":\"jerry\"},\"fields\": [\"owner\", \"asset_name\", \"color\", \"size\"]}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "jerry")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query with fields, namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", "{\"selector\":{\"owner\":\"jerry\"},\"fields\": [\"owner\", \"asset_name\", \"color\", \"size\"]}")
	testutil.AssertNoError(t, err, "")

	// verify no results
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult1)

	// query with complex selector, namespace "ns1"
	itr, err = db.ExecuteQuery("ns1", "{\"selector\":{\"$and\":[{\"size\":{\"$gt\": 5}},{\"size\":{\"$lt\":8}},{\"$not\":{\"size\":6}}]}}")
	testutil.AssertNoError(t, err, "")

	// verify one fred result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "fred")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query with complex selector, namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", "{\"selector\":{\"$and\":[{\"size\":{\"$gt\": 5}},{\"size\":{\"$lt\":8}},{\"$not\":{\"size\":6}}]}}")
	testutil.AssertNoError(t, err, "")

	// verify one fred result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "fred")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

	// query with complex selector, namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", "{\"selector\":{\"$and\":[{\"size\":{\"$gt\": 5}},{\"size\":{\"$lt\":8}},{\"$not\":{\"size\":6}}]}}")
	testutil.AssertNoError(t, err, "")

	// verify no more results
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult1)

	// query with embedded implicit "AND" and explicit "OR", namespace "ns1"
	itr, err = db.ExecuteQuery("ns1", "{\"selector\":{\"color\":\"green\",\"$or\":[{\"owner\":\"fred\"},{\"owner\":\"mary\"}]}}")
	testutil.AssertNoError(t, err, "")

	// verify one green result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify another green result
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult2)
	versionedQueryRecord = queryResult2.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult3, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult3)

	// query with embedded implicit "AND" and explicit "OR", namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", "{\"selector\":{\"color\":\"green\",\"$or\":[{\"owner\":\"fred\"},{\"owner\":\"mary\"}]}}")
	testutil.AssertNoError(t, err, "")

	// verify one green result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify another green result
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult2)
	versionedQueryRecord = queryResult2.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult3, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult3)

	// query with embedded implicit "AND" and explicit "OR", namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", "{\"selector\":{\"color\":\"green\",\"$or\":[{\"owner\":\"fred\"},{\"owner\":\"mary\"}]}}")
	testutil.AssertNoError(t, err, "")

	// verify no results
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult1)

	// query with integer with digit-count equals 7 and response received is also received
	// with same digit-count and there is no float transformation
	itr, err = db.ExecuteQuery("ns1", "{\"selector\":{\"$and\":[{\"size\":{\"$eq\": 1000007}}]}}")
	testutil.AssertNoError(t, err, "")

	// verify one jerry result
	queryResult1, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNotNil(t, queryResult1)
	versionedQueryRecord = queryResult1.(*statedb.VersionedKV)
	stringRecord = string(versionedQueryRecord.Value)
	bFoundRecord = strings.Contains(stringRecord, "joe")
	testutil.AssertEquals(t, bFoundRecord, true)
	bFoundRecord = strings.Contains(stringRecord, "1000007")
	testutil.AssertEquals(t, bFoundRecord, true)

	// verify no more results
	queryResult2, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, queryResult2)

}
