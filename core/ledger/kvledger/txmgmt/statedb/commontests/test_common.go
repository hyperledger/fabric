/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commontests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

// CheckDBsAfterDropFunc checks if the channel-specific dbs have been dropped
type CheckDBsAfterDropFunc func(channelName string)

// TestGetStateMultipleKeys tests read for given multiple keys
func TestGetStateMultipleKeys(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testgetmultiplekeys", nil)
	require.NoError(t, err)

	// Test that savepoint is nil for a new state db
	sp, err := db.GetLatestSavePoint()
	require.NoError(t, err, "Error upon GetLatestSavePoint()")
	require.Nil(t, sp)

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
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	actualValues, _ := db.GetStateMultipleKeys("ns1", []string{"key1", "key2"})
	require.Equal(t, expectedValues, actualValues)
}

// TestBasicRW tests basic read-write
func TestBasicRW(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testbasicrw", nil)
	require.NoError(t, err)

	// Test that savepoint is nil for a new state db
	sp, err := db.GetLatestSavePoint()
	require.NoError(t, err, "Error upon GetLatestSavePoint()")
	require.Nil(t, sp)

	// Test retrieval of non-existent key - returns nil rather than error
	// For more details see https://github.com/hyperledger-archives/fabric/issues/936.
	val, err := db.GetState("ns", "key1")
	require.NoError(t, err, "Should receive nil rather than error upon reading non existent key")
	require.Nil(t, val)

	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte{}, Version: version.NewHeight(1, 4)}
	vv5 := statedb.VersionedValue{Value: []byte("null"), Version: version.NewHeight(1, 5)}
	batch.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch.Put("ns1", "key2", vv2.Value, vv2.Version)
	batch.Put("ns2", "key3", vv3.Value, vv3.Version)
	batch.Put("ns2", "key4", vv4.Value, vv4.Version)
	batch.Put("ns2", "key5", vv5.Value, vv5.Version)
	savePoint := version.NewHeight(2, 5)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	vv, _ := db.GetState("ns1", "key1")
	require.Equal(t, &vv1, vv)

	vv, _ = db.GetState("ns2", "key4")
	require.Equal(t, &vv4, vv)

	vv, _ = db.GetState("ns2", "key5")
	require.Equal(t, &vv5, vv)

	sp, err = db.GetLatestSavePoint()
	require.NoError(t, err)
	require.Equal(t, savePoint, sp)
}

// TestDrop tests dropping channel-specific ledger data
func TestDrop(t *testing.T, dbProvider statedb.VersionedDBProvider, checkDBsAfterDropFunc CheckDBsAfterDropFunc) {
	channel1 := "testdrop-channel-1"
	channel2 := "testdrop-channel-2"
	namespaces := map[string]string{
		channel1: "ns1",
		channel2: "ns2",
	}

	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}

	for channelName, ns := range namespaces {
		db, err := dbProvider.GetDBHandle(channelName, nil)
		require.NoError(t, err)

		batch := statedb.NewUpdateBatch()
		batch.Put(ns, "key1", vv1.Value, vv1.Version)
		batch.Put(ns, "key2", vv2.Value, vv2.Version)
		savePoint := version.NewHeight(2, 2)
		require.NoError(t, db.ApplyUpdates(batch, savePoint))

		vv, err := db.GetState(ns, "key1")
		require.NoError(t, err)
		require.Equal(t, &vv1, vv)
		vv, err = db.GetState(ns, "key2")
		require.NoError(t, err)
		require.Equal(t, &vv2, vv)
	}

	require.NoError(t, dbProvider.Drop(channel1))

	// verify channel1 data are dropped
	checkDBsAfterDropFunc(channel1)

	// verify channel2 data remain as is
	db2, err := dbProvider.GetDBHandle(channel2, nil)
	require.NoError(t, err)

	vv, err := db2.GetState("ns2", "key1")
	require.NoError(t, err)
	require.Equal(t, &vv1, vv)

	vv, err = db2.GetState("ns2", "key2")
	require.NoError(t, err)
	require.Equal(t, &vv2, vv)

	// drop again should not fail
	require.NoError(t, dbProvider.Drop(channel1))
}

// TestMultiDBBasicRW tests basic read-write on multiple dbs
func TestMultiDBBasicRW(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db1, err := dbProvider.GetDBHandle("testmultidbbasicrw", nil)
	require.NoError(t, err)

	db2, err := dbProvider.GetDBHandle("testmultidbbasicrw2", nil)
	require.NoError(t, err)

	batch1 := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1_db1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2_db1"), Version: version.NewHeight(1, 2)}
	batch1.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch1.Put("ns1", "key2", vv2.Value, vv2.Version)
	savePoint1 := version.NewHeight(1, 2)
	require.NoError(t, db1.ApplyUpdates(batch1, savePoint1))

	batch2 := statedb.NewUpdateBatch()
	vv3 := statedb.VersionedValue{Value: []byte("value1_db2"), Version: version.NewHeight(1, 4)}
	vv4 := statedb.VersionedValue{Value: []byte("value2_db2"), Version: version.NewHeight(1, 5)}
	batch2.Put("ns1", "key1", vv3.Value, vv3.Version)
	batch2.Put("ns1", "key2", vv4.Value, vv4.Version)
	savePoint2 := version.NewHeight(1, 5)
	require.NoError(t, db2.ApplyUpdates(batch2, savePoint2))

	vv, _ := db1.GetState("ns1", "key1")
	require.Equal(t, &vv1, vv)

	sp, err := db1.GetLatestSavePoint()
	require.NoError(t, err)
	require.Equal(t, savePoint1, sp)

	vv, _ = db2.GetState("ns1", "key1")
	require.Equal(t, &vv3, vv)

	sp, err = db2.GetLatestSavePoint()
	require.NoError(t, err)
	require.Equal(t, savePoint2, sp)
}

// TestDeletes tests deletes
func TestDeletes(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testdeletes", nil)
	require.NoError(t, err)

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
	require.NoError(t, err)
	vv, _ := db.GetState("ns", "key2")
	require.Equal(t, &vv2, vv)

	vv, err = db.GetState("ns", "key3")
	require.NoError(t, err)
	require.Nil(t, vv)

	batch = statedb.NewUpdateBatch()
	batch.Delete("ns", "key2", version.NewHeight(1, 6))
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)
	vv, err = db.GetState("ns", "key2")
	require.NoError(t, err)
	require.Nil(t, vv)
}

// TestIterator tests the iterator
func TestIterator(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testiterator", nil)
	require.NoError(t, err)
	require.NoError(t, db.Open())
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
	require.NoError(t, db.ApplyUpdates(batch, savePoint))
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
		queryResult, err := itr.Next()
		require.NoError(t, err)
		require.Equal(t, expectedKey, queryResult.Key)
	}
	_, err := itr.Next()
	require.NoError(t, err)
}

// TestQuery tests queries
func TestQuery(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testquery", nil)
	require.NoError(t, err)
	require.NoError(t, db.Open())
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := `{"asset_name": "marble2","color": "blue","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 2))
	jsonValue3 := `{"asset_name": "marble3","color": "blue","size": 3,"owner": "fred"}`
	batch.Put("ns1", "key3", []byte(jsonValue3), version.NewHeight(1, 3))
	jsonValue4 := `{"asset_name": "marble4","color": "blue","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key4", []byte(jsonValue4), version.NewHeight(1, 4))
	jsonValue5 := `{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`
	batch.Put("ns1", "key5", []byte(jsonValue5), version.NewHeight(1, 5))
	jsonValue6 := `{"asset_name": "marble6","color": "blue","size": 6,"owner": "elaine"}`
	batch.Put("ns1", "key6", []byte(jsonValue6), version.NewHeight(1, 6))
	jsonValue7 := `{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`
	batch.Put("ns1", "key7", []byte(jsonValue7), version.NewHeight(1, 7))
	jsonValue8 := `{"asset_name": "marble8","color": "blue","size": 8,"owner": "elaine"}`
	batch.Put("ns1", "key8", []byte(jsonValue8), version.NewHeight(1, 8))
	jsonValue9 := `{"asset_name": "marble9","color": "green","size": 9,"owner": "fred"}`
	batch.Put("ns1", "key9", []byte(jsonValue9), version.NewHeight(1, 9))
	jsonValue10 := `{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`
	batch.Put("ns1", "key10", []byte(jsonValue10), version.NewHeight(1, 10))
	jsonValue11 := `{"asset_name": "marble11","color": "cyan","size": 1000007,"owner": "joe"}`
	batch.Put("ns1", "key11", []byte(jsonValue11), version.NewHeight(1, 11))

	// add keys for a separate namespace
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
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// query for owner=jerry, use namespace "ns1"
	itr, err := db.ExecuteQuery("ns1", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)

	// verify one jerry result
	queryResult1, err := itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)

	stringRecord := string(queryResult1.Value)
	bFoundRecord := strings.Contains(stringRecord, "jerry")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err := itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)

	// query for owner=jerry, use namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)

	// verify one jerry result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "jerry")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)

	// query for owner=jerry, use namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)

	// verify results - should be no records
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult1)

	// query using bad query string
	_, err = db.ExecuteQuery("ns1", "this is an invalid query string")
	require.Error(t, err, "Should have received an error for invalid query string")

	// query returns 0 records
	_, err = db.ExecuteQuery("ns1", `{"selector":{"owner":"not_a_valid_name"}}`)
	require.NoError(t, err)

	// verify no results
	queryResult3, err := itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult3)

	// query with fields, namespace "ns1"
	itr, err = db.ExecuteQuery("ns1", `{"selector":{"owner":"jerry"},"fields": ["owner", "asset_name", "color", "size"]}`)
	require.NoError(t, err)

	// verify one jerry result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "jerry")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)

	// query with fields, namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", `{"selector":{"owner":"jerry"},"fields": ["owner", "asset_name", "color", "size"]}`)
	require.NoError(t, err)

	// verify one jerry result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "jerry")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)

	// query with fields, namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", `{"selector":{"owner":"jerry"},"fields": ["owner", "asset_name", "color", "size"]}`)
	require.NoError(t, err)

	// verify no results
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult1)

	// query with complex selector, namespace "ns1"
	itr, err = db.ExecuteQuery("ns1", `{"selector":{"$and":[{"size":{"$gt": 5}},{"size":{"$lt":8}},{"$not":{"size":6}}]}}`)
	require.NoError(t, err)

	// verify one fred result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "fred")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)

	// query with complex selector, namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", `{"selector":{"$and":[{"size":{"$gt": 5}},{"size":{"$lt":8}},{"$not":{"size":6}}]}}`)
	require.NoError(t, err)

	// verify one fred result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "fred")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)

	// query with complex selector, namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", `{"selector":{"$and":[{"size":{"$gt": 5}},{"size":{"$lt":8}},{"$not":{"size":6}}]}}`)
	require.NoError(t, err)

	// verify no more results
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult1)

	// query with embedded implicit "AND" and explicit "OR", namespace "ns1"
	itr, err = db.ExecuteQuery("ns1", `{"selector":{"color":"green","$or":[{"owner":"fred"},{"owner":"mary"}]}}`)
	require.NoError(t, err)

	// verify one green result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	require.True(t, bFoundRecord)

	// verify another green result
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult2)
	stringRecord = string(queryResult2.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult3, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult3)

	// query with embedded implicit "AND" and explicit "OR", namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", `{"selector":{"color":"green","$or":[{"owner":"fred"},{"owner":"mary"}]}}`)
	require.NoError(t, err)

	// verify one green result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	require.True(t, bFoundRecord)

	// verify another green result
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult2)
	stringRecord = string(queryResult2.Value)
	bFoundRecord = strings.Contains(stringRecord, "green")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult3, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult3)

	// query with embedded implicit "AND" and explicit "OR", namespace "ns3"
	itr, err = db.ExecuteQuery("ns3", `{"selector":{"color":"green","$or":[{"owner":"fred"},{"owner":"mary"}]}}`)
	require.NoError(t, err)

	// verify no results
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult1)

	// query with integer with digit-count equals 7 and response received is also received
	// with same digit-count and there is no float transformation
	itr, err = db.ExecuteQuery("ns1", `{"selector":{"$and":[{"size":{"$eq": 1000007}}]}}`)
	require.NoError(t, err)

	// verify one jerry result
	queryResult1, err = itr.Next()
	require.NoError(t, err)
	require.NotNil(t, queryResult1)
	stringRecord = string(queryResult1.Value)
	bFoundRecord = strings.Contains(stringRecord, "joe")
	require.True(t, bFoundRecord)
	bFoundRecord = strings.Contains(stringRecord, "1000007")
	require.True(t, bFoundRecord)

	// verify no more results
	queryResult2, err = itr.Next()
	require.NoError(t, err)
	require.Nil(t, queryResult2)
}

// TestGetVersion tests retrieving the version by namespace and key
func TestGetVersion(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testgetversion", nil)
	require.NoError(t, err)

	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 4)}

	batch.Put("ns", "key1", vv1.Value, vv1.Version)
	batch.Put("ns", "key2", vv2.Value, vv2.Version)
	batch.Put("ns", "key3", vv2.Value, vv3.Version)
	batch.Put("ns", "key4", vv2.Value, vv4.Version)
	savePoint := version.NewHeight(1, 5)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// check to see if the bulk optimizable interface is supported (couchdb)
	if bulkdb, ok := db.(statedb.BulkOptimizable); ok {
		// clear the cached versions, this will force a read when getVerion is called
		bulkdb.ClearCachedVersions()
	}

	// retrieve a version by namespace and key
	resp, err := db.GetVersion("ns", "key2")
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(1, 2), resp)

	// attempt to retrieve an non-existent namespace and key
	resp, err = db.GetVersion("ns2", "key2")
	require.NoError(t, err)
	require.Nil(t, resp)

	// check to see if the bulk optimizable interface is supported (couchdb)
	if bulkdb, ok := db.(statedb.BulkOptimizable); ok {

		// clear the cached versions, this will force a read when getVerion is called
		bulkdb.ClearCachedVersions()

		// initialize a key list
		loadKeys := []*statedb.CompositeKey{}
		// create a composite key and add to the key list
		compositeKey := statedb.CompositeKey{Namespace: "ns", Key: "key3"}
		loadKeys = append(loadKeys, &compositeKey)
		// load the committed versions
		require.NoError(t, bulkdb.LoadCommittedVersions(loadKeys))

		// retrieve a version by namespace and key
		resp, err := db.GetVersion("ns", "key3")
		require.NoError(t, err)
		require.Equal(t, version.NewHeight(1, 3), resp)

	}
}

// TestSmallBatchSize tests multiple update batches
func TestSmallBatchSize(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testsmallbatchsize", nil)
	require.NoError(t, err)
	require.NoError(t, db.Open())
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	jsonValue1 := []byte(`{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`)
	batch.Put("ns1", "key1", jsonValue1, version.NewHeight(1, 1))
	jsonValue2 := []byte(`{"asset_name": "marble2","color": "blue","size": 2,"owner": "jerry"}`)
	batch.Put("ns1", "key2", jsonValue2, version.NewHeight(1, 2))
	jsonValue3 := []byte(`{"asset_name": "marble3","color": "blue","size": 3,"owner": "fred"}`)
	batch.Put("ns1", "key3", jsonValue3, version.NewHeight(1, 3))
	jsonValue4 := []byte(`{"asset_name": "marble4","color": "blue","size": 4,"owner": "martha"}`)
	batch.Put("ns1", "key4", jsonValue4, version.NewHeight(1, 4))
	jsonValue5 := []byte(`{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`)
	batch.Put("ns1", "key5", jsonValue5, version.NewHeight(1, 5))
	jsonValue6 := []byte(`{"asset_name": "marble6","color": "blue","size": 6,"owner": "elaine"}`)
	batch.Put("ns1", "key6", jsonValue6, version.NewHeight(1, 6))
	jsonValue7 := []byte(`{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`)
	batch.Put("ns1", "key7", jsonValue7, version.NewHeight(1, 7))
	jsonValue8 := []byte(`{"asset_name": "marble8","color": "blue","size": 8,"owner": "elaine"}`)
	batch.Put("ns1", "key8", jsonValue8, version.NewHeight(1, 8))
	jsonValue9 := []byte(`{"asset_name": "marble9","color": "green","size": 9,"owner": "fred"}`)
	batch.Put("ns1", "key9", jsonValue9, version.NewHeight(1, 9))
	jsonValue10 := []byte(`{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`)
	batch.Put("ns1", "key10", jsonValue10, version.NewHeight(1, 10))
	jsonValue11 := []byte(`{"asset_name": "marble11","color": "cyan","size": 1000007,"owner": "joe"}`)
	batch.Put("ns1", "key11", jsonValue11, version.NewHeight(1, 11))

	savePoint := version.NewHeight(1, 12)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// Verify all marbles were added

	vv, _ := db.GetState("ns1", "key1")
	require.JSONEq(t, string(jsonValue1), string(vv.Value))

	vv, _ = db.GetState("ns1", "key2")
	require.JSONEq(t, string(jsonValue2), string(vv.Value))

	vv, _ = db.GetState("ns1", "key3")
	require.JSONEq(t, string(jsonValue3), string(vv.Value))

	vv, _ = db.GetState("ns1", "key4")
	require.JSONEq(t, string(jsonValue4), string(vv.Value))

	vv, _ = db.GetState("ns1", "key5")
	require.JSONEq(t, string(jsonValue5), string(vv.Value))

	vv, _ = db.GetState("ns1", "key6")
	require.JSONEq(t, string(jsonValue6), string(vv.Value))

	vv, _ = db.GetState("ns1", "key7")
	require.JSONEq(t, string(jsonValue7), string(vv.Value))

	vv, _ = db.GetState("ns1", "key8")
	require.JSONEq(t, string(jsonValue8), string(vv.Value))

	vv, _ = db.GetState("ns1", "key9")
	require.JSONEq(t, string(jsonValue9), string(vv.Value))

	vv, _ = db.GetState("ns1", "key10")
	require.JSONEq(t, string(jsonValue10), string(vv.Value))

	vv, _ = db.GetState("ns1", "key11")
	require.JSONEq(t, string(jsonValue11), string(vv.Value))
}

// TestBatchWithIndividualRetry tests a single failure in a batch
func TestBatchWithIndividualRetry(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testbatchretry", nil)
	require.NoError(t, err)

	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte("value4"), Version: version.NewHeight(1, 4)}

	batch.Put("ns", "key1", vv1.Value, vv1.Version)
	batch.Put("ns", "key2", vv2.Value, vv2.Version)
	batch.Put("ns", "key3", vv3.Value, vv3.Version)
	batch.Put("ns", "key4", vv4.Value, vv4.Version)
	savePoint := version.NewHeight(1, 5)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// Clear the cache for the next batch, in place of simulation
	if bulkdb, ok := db.(statedb.BulkOptimizable); ok {
		// clear the cached versions, this will force a read when getVerion is called
		bulkdb.ClearCachedVersions()
	}

	batch = statedb.NewUpdateBatch()
	batch.Put("ns", "key1", vv1.Value, vv1.Version)
	batch.Put("ns", "key2", vv2.Value, vv2.Version)
	batch.Put("ns", "key3", vv3.Value, vv3.Version)
	batch.Put("ns", "key4", vv4.Value, vv4.Version)
	savePoint = version.NewHeight(1, 6)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// Update document key3
	batch = statedb.NewUpdateBatch()
	batch.Delete("ns", "key2", vv2.Version)
	batch.Put("ns", "key3", vv3.Value, vv3.Version)
	savePoint = version.NewHeight(1, 7)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// This should force a retry for couchdb revision conflict for both delete and update
	// Retry logic should correct the update and prevent delete from throwing an error
	batch = statedb.NewUpdateBatch()
	batch.Delete("ns", "key2", vv2.Version)
	batch.Put("ns", "key3", vv3.Value, vv3.Version)
	savePoint = version.NewHeight(1, 8)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// Create a new set of values that use JSONs instead of binary
	jsonValue5 := []byte(`{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`)
	jsonValue6 := []byte(`{"asset_name": "marble6","color": "blue","size": 6,"owner": "elaine"}`)
	jsonValue7 := []byte(`{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`)
	jsonValue8 := []byte(`{"asset_name": "marble8","color": "blue","size": 8,"owner": "elaine"}`)

	// Clear the cache for the next batch, in place of simulation
	if bulkdb, ok := db.(statedb.BulkOptimizable); ok {
		// clear the cached versions, this will force a read when getVersion is called
		bulkdb.ClearCachedVersions()
	}

	batch = statedb.NewUpdateBatch()
	batch.Put("ns1", "key5", jsonValue5, version.NewHeight(1, 9))
	batch.Put("ns1", "key6", jsonValue6, version.NewHeight(1, 10))
	batch.Put("ns1", "key7", jsonValue7, version.NewHeight(1, 11))
	batch.Put("ns1", "key8", jsonValue8, version.NewHeight(1, 12))
	savePoint = version.NewHeight(1, 6)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// Clear the cache for the next batch, in place of simulation
	if bulkdb, ok := db.(statedb.BulkOptimizable); ok {
		// clear the cached versions, this will force a read when getVersion is called
		bulkdb.ClearCachedVersions()
	}

	// Send the batch through again to test updates
	batch = statedb.NewUpdateBatch()
	batch.Put("ns1", "key5", jsonValue5, version.NewHeight(1, 9))
	batch.Put("ns1", "key6", jsonValue6, version.NewHeight(1, 10))
	batch.Put("ns1", "key7", jsonValue7, version.NewHeight(1, 11))
	batch.Put("ns1", "key8", jsonValue8, version.NewHeight(1, 12))
	savePoint = version.NewHeight(1, 6)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// Update document key3
	// this will cause an inconsistent cache entry for connection db2
	batch = statedb.NewUpdateBatch()
	batch.Delete("ns1", "key6", version.NewHeight(1, 13))
	batch.Put("ns1", "key7", jsonValue7, version.NewHeight(1, 14))
	savePoint = version.NewHeight(1, 15)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)

	// This should force a retry for couchdb revision conflict for both delete and update
	// Retry logic should correct the update and prevent delete from throwing an error
	batch = statedb.NewUpdateBatch()
	batch.Delete("ns1", "key6", version.NewHeight(1, 16))
	batch.Put("ns1", "key7", jsonValue7, version.NewHeight(1, 17))
	savePoint = version.NewHeight(1, 18)
	err = db.ApplyUpdates(batch, savePoint)
	require.NoError(t, err)
}

// TestValueAndMetadataWrites tests statedb for value and metadata read-writes
func TestValueAndMetadataWrites(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testvalueandmetadata", nil)
	require.NoError(t, err)
	batch := statedb.NewUpdateBatch()

	vv1 := statedb.VersionedValue{Value: []byte("value1"), Metadata: []byte("metadata1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Metadata: []byte("metadata2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte{}, Metadata: []byte("metadata4"), Version: version.NewHeight(1, 4)}

	batch.PutValAndMetadata("ns1", "key1", vv1.Value, vv1.Metadata, vv1.Version)
	batch.PutValAndMetadata("ns1", "key2", vv2.Value, vv2.Metadata, vv2.Version)
	batch.PutValAndMetadata("ns2", "key3", vv3.Value, vv3.Metadata, vv3.Version)
	batch.PutValAndMetadata("ns2", "key4", vv4.Value, vv4.Metadata, vv4.Version)
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(2, 5)))

	vv, _ := db.GetState("ns1", "key1")
	require.Equal(t, &vv1, vv)

	vv, _ = db.GetState("ns1", "key2")
	require.Equal(t, &vv2, vv)

	vv, _ = db.GetState("ns2", "key3")
	require.Equal(t, &vv3, vv)

	vv, _ = db.GetState("ns2", "key4")
	require.Equal(t, &vv4, vv)
}

// TestPaginatedRangeQuery tests range queries with pagination
func TestPaginatedRangeQuery(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testpaginatedrangequery", nil)
	require.NoError(t, err)
	require.NoError(t, db.Open())
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := `{"asset_name": "marble2","color": "red","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 2))
	jsonValue3 := `{"asset_name": "marble3","color": "red","size": 3,"owner": "fred"}`
	batch.Put("ns1", "key3", []byte(jsonValue3), version.NewHeight(1, 3))
	jsonValue4 := `{"asset_name": "marble4","color": "red","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key4", []byte(jsonValue4), version.NewHeight(1, 4))
	jsonValue5 := `{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`
	batch.Put("ns1", "key5", []byte(jsonValue5), version.NewHeight(1, 5))
	jsonValue6 := `{"asset_name": "marble6","color": "red","size": 6,"owner": "elaine"}`
	batch.Put("ns1", "key6", []byte(jsonValue6), version.NewHeight(1, 6))
	jsonValue7 := `{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`
	batch.Put("ns1", "key7", []byte(jsonValue7), version.NewHeight(1, 7))
	jsonValue8 := `{"asset_name": "marble8","color": "red","size": 8,"owner": "elaine"}`
	batch.Put("ns1", "key8", []byte(jsonValue8), version.NewHeight(1, 8))
	jsonValue9 := `{"asset_name": "marble9","color": "green","size": 9,"owner": "fred"}`
	batch.Put("ns1", "key9", []byte(jsonValue9), version.NewHeight(1, 9))
	jsonValue10 := `{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`
	batch.Put("ns1", "key10", []byte(jsonValue10), version.NewHeight(1, 10))

	jsonValue11 := `{"asset_name": "marble11","color": "cyan","size": 8,"owner": "joe"}`
	batch.Put("ns1", "key11", []byte(jsonValue11), version.NewHeight(1, 11))
	jsonValue12 := `{"asset_name": "marble12","color": "red","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key12", []byte(jsonValue12), version.NewHeight(1, 4))
	jsonValue13 := `{"asset_name": "marble13","color": "red","size": 6,"owner": "james"}`
	batch.Put("ns1", "key13", []byte(jsonValue13), version.NewHeight(1, 4))
	jsonValue14 := `{"asset_name": "marble14","color": "red","size": 10,"owner": "fred"}`
	batch.Put("ns1", "key14", []byte(jsonValue14), version.NewHeight(1, 4))
	jsonValue15 := `{"asset_name": "marble15","color": "red","size": 8,"owner": "mary"}`
	batch.Put("ns1", "key15", []byte(jsonValue15), version.NewHeight(1, 4))
	jsonValue16 := `{"asset_name": "marble16","color": "red","size": 4,"owner": "robert"}`
	batch.Put("ns1", "key16", []byte(jsonValue16), version.NewHeight(1, 4))
	jsonValue17 := `{"asset_name": "marble17","color": "red","size": 2,"owner": "alan"}`
	batch.Put("ns1", "key17", []byte(jsonValue17), version.NewHeight(1, 4))
	jsonValue18 := `{"asset_name": "marble18","color": "red","size": 10,"owner": "elaine"}`
	batch.Put("ns1", "key18", []byte(jsonValue18), version.NewHeight(1, 4))
	jsonValue19 := `{"asset_name": "marble19","color": "red","size": 2,"owner": "alan"}`
	batch.Put("ns1", "key19", []byte(jsonValue19), version.NewHeight(1, 4))
	jsonValue20 := `{"asset_name": "marble20","color": "red","size": 10,"owner": "elaine"}`
	batch.Put("ns1", "key20", []byte(jsonValue20), version.NewHeight(1, 4))

	jsonValue21 := `{"asset_name": "marble21","color": "cyan","size": 1000007,"owner": "joe"}`
	batch.Put("ns1", "key21", []byte(jsonValue21), version.NewHeight(1, 11))
	jsonValue22 := `{"asset_name": "marble22","color": "red","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key22", []byte(jsonValue22), version.NewHeight(1, 4))
	jsonValue23 := `{"asset_name": "marble23","color": "blue","size": 6,"owner": "james"}`
	batch.Put("ns1", "key23", []byte(jsonValue23), version.NewHeight(1, 4))
	jsonValue24 := `{"asset_name": "marble24","color": "red","size": 10,"owner": "fred"}`
	batch.Put("ns1", "key24", []byte(jsonValue24), version.NewHeight(1, 4))
	jsonValue25 := `{"asset_name": "marble25","color": "red","size": 8,"owner": "mary"}`
	batch.Put("ns1", "key25", []byte(jsonValue25), version.NewHeight(1, 4))
	jsonValue26 := `{"asset_name": "marble26","color": "red","size": 4,"owner": "robert"}`
	batch.Put("ns1", "key26", []byte(jsonValue26), version.NewHeight(1, 4))
	jsonValue27 := `{"asset_name": "marble27","color": "green","size": 2,"owner": "alan"}`
	batch.Put("ns1", "key27", []byte(jsonValue27), version.NewHeight(1, 4))
	jsonValue28 := `{"asset_name": "marble28","color": "red","size": 10,"owner": "elaine"}`
	batch.Put("ns1", "key28", []byte(jsonValue28), version.NewHeight(1, 4))
	jsonValue29 := `{"asset_name": "marble29","color": "red","size": 2,"owner": "alan"}`
	batch.Put("ns1", "key29", []byte(jsonValue29), version.NewHeight(1, 4))
	jsonValue30 := `{"asset_name": "marble30","color": "red","size": 10,"owner": "elaine"}`
	batch.Put("ns1", "key30", []byte(jsonValue30), version.NewHeight(1, 4))

	jsonValue31 := `{"asset_name": "marble31","color": "cyan","size": 1000007,"owner": "joe"}`
	batch.Put("ns1", "key31", []byte(jsonValue31), version.NewHeight(1, 11))
	jsonValue32 := `{"asset_name": "marble32","color": "red","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key32", []byte(jsonValue32), version.NewHeight(1, 4))
	jsonValue33 := `{"asset_name": "marble33","color": "red","size": 6,"owner": "james"}`
	batch.Put("ns1", "key33", []byte(jsonValue33), version.NewHeight(1, 4))
	jsonValue34 := `{"asset_name": "marble34","color": "red","size": 10,"owner": "fred"}`
	batch.Put("ns1", "key34", []byte(jsonValue34), version.NewHeight(1, 4))
	jsonValue35 := `{"asset_name": "marble35","color": "red","size": 8,"owner": "mary"}`
	batch.Put("ns1", "key35", []byte(jsonValue35), version.NewHeight(1, 4))
	jsonValue36 := `{"asset_name": "marble36","color": "orange","size": 4,"owner": "robert"}`
	batch.Put("ns1", "key36", []byte(jsonValue36), version.NewHeight(1, 4))
	jsonValue37 := `{"asset_name": "marble37","color": "red","size": 2,"owner": "alan"}`
	batch.Put("ns1", "key37", []byte(jsonValue37), version.NewHeight(1, 4))
	jsonValue38 := `{"asset_name": "marble38","color": "yellow","size": 10,"owner": "elaine"}`
	batch.Put("ns1", "key38", []byte(jsonValue38), version.NewHeight(1, 4))
	jsonValue39 := `{"asset_name": "marble39","color": "red","size": 2,"owner": "alan"}`
	batch.Put("ns1", "key39", []byte(jsonValue39), version.NewHeight(1, 4))
	jsonValue40 := `{"asset_name": "marble40","color": "red","size": 10,"owner": "elaine"}`
	batch.Put("ns1", "key40", []byte(jsonValue40), version.NewHeight(1, 4))

	savePoint := version.NewHeight(2, 22)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// Test range query with no pagination
	returnKeys := []string{}
	_, err = executeRangeQuery(t, db, "ns1", "key1", "key15", int32(0), returnKeys)
	require.NoError(t, err)

	// Test range query with large page size (single page return)
	returnKeys = []string{"key1", "key10", "key11", "key12", "key13", "key14"}
	_, err = executeRangeQuery(t, db, "ns1", "key1", "key15", int32(10), returnKeys)
	require.NoError(t, err)

	// Test explicit pagination
	// Test range query with multiple pages
	returnKeys = []string{"key1", "key10"}
	nextStartKey, err := executeRangeQuery(t, db, "ns1", "key1", "key22", int32(2), returnKeys)
	require.NoError(t, err)

	// NextStartKey is now passed in as startKey,  verify the pagesize is working
	returnKeys = []string{"key11", "key12"}
	_, err = executeRangeQuery(t, db, "ns1", nextStartKey, "key22", int32(2), returnKeys)
	require.NoError(t, err)

	// Test implicit pagination
	// Test range query with no pagesize and a small queryLimit
	returnKeys = []string{}
	_, err = executeRangeQuery(t, db, "ns1", "key1", "key15", int32(0), returnKeys)
	require.NoError(t, err)

	// Test range query with pagesize greater than the queryLimit
	returnKeys = []string{"key1", "key10", "key11", "key12"}
	_, err = executeRangeQuery(t, db, "ns1", "key1", "key15", int32(4), returnKeys)
	require.NoError(t, err)
}

// TestRangeQuerySpecialCharacters tests range queries for keys with special characters and/or non-English characters
func TestRangeQuerySpecialCharacters(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("testrangequeryspecialcharacters", nil)
	require.NoError(t, err)
	require.NoError(t, db.Open())
	defer db.Close()

	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := `{"asset_name": "marble2","color": "red","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 1))
	jsonValue3 := `{"asset_name": "marble3","color": "red","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key1&%-", []byte(jsonValue3), version.NewHeight(1, 1))
	jsonValue4 := `{"asset_name": "marble4","color": "red","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key1-a", []byte(jsonValue4), version.NewHeight(1, 4))
	jsonValue5 := `{"asset_name": "marble5","color": "red","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key1%=", []byte(jsonValue5), version.NewHeight(1, 2))
	jsonValue6 := `{"asset_name": "marble6","color": "red","size": 3,"owner": "fred"}`
	batch.Put("ns1", "key1español", []byte(jsonValue6), version.NewHeight(1, 3))
	jsonValue7 := `{"asset_name": "marble7","color": "blue","size": 5,"owner": "fred"}`
	batch.Put("ns1", "key1中文", []byte(jsonValue7), version.NewHeight(1, 5))
	jsonValue8 := `{"asset_name": "marble8","color": "blue","size": 7,"owner": "fred"}`
	batch.Put("ns1", "key1한국어", []byte(jsonValue8), version.NewHeight(1, 7))
	jsonValue9 := `{"asset_name": "marble9","color": "blue","size": 5,"owner": "fred"}`
	batch.Put("ns1", "中文key1", []byte(jsonValue9), version.NewHeight(1, 5))
	jsonValue10 := `{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`
	batch.Put("ns1", "key10", []byte(jsonValue10), version.NewHeight(1, 10))
	jsonValue11 := `{"asset_name": "marble11","color": "cyan","size": 8,"owner": "joe"}`
	batch.Put("ns1", "key1z", []byte(jsonValue11), version.NewHeight(1, 11))

	savePoint := version.NewHeight(2, 22)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// Test range query for the keys with special or non-English characters
	returnKeys := []string{"key1", "key1%=", "key1&%-", "key1-a", "key10", "key1español", "key1z", "key1中文", "key1한국어"}
	// returnKeys := []string{"key1", "key1%=", "key1&%-", "key1-a", "key10", "key1español", "key1z"}
	_, err = executeRangeQuery(t, db, "ns1", "key1", "key2", int32(10), returnKeys)
	require.NoError(t, err)
}

func executeRangeQuery(t *testing.T, db statedb.VersionedDB, namespace, startKey, endKey string, pageSize int32, returnKeys []string) (string, error) {
	var itr statedb.ResultsIterator
	var err error

	if pageSize == 0 {
		itr, err = db.GetStateRangeScanIterator(namespace, startKey, endKey)
		if err != nil {
			return "", err
		}

	} else {
		itr, err = db.GetStateRangeScanIteratorWithPagination(namespace, startKey, endKey, pageSize)
		if err != nil {
			return "", err
		}

		if pageSize > 0 {
			TestItrWithoutClose(t, itr, returnKeys)
		}
	}

	returnBookmark := ""
	if pageSize > 0 {
		if queryResultItr, ok := itr.(statedb.QueryResultsIterator); ok {
			returnBookmark = queryResultItr.GetBookmarkAndClose()
		}
	}
	return returnBookmark, nil
}

// TestItrWithoutClose verifies an iterator contains expected keys
func TestItrWithoutClose(t *testing.T, itr statedb.ResultsIterator, expectedKeys []string) {
	for _, expectedKey := range expectedKeys {
		queryResult, err := itr.Next()
		require.NoError(t, err, "An unexpected error was thrown during iterator Next()")
		require.Equal(t, expectedKey, queryResult.Key)
	}
	queryResult, err := itr.Next()
	require.NoError(t, err, "An unexpected error was thrown during iterator Next()")
	require.Nil(t, queryResult)
}

// TestApplyUpdatesWithNilHeight is a common test that is invoked by leveldb and couchdb for verifying that
// the statedb commits the batch with a nil height (used for committing the missing private data for the old blocks)
func TestApplyUpdatesWithNilHeight(t *testing.T, dbProvider statedb.VersionedDBProvider) {
	db, err := dbProvider.GetDBHandle("test-apply-updates-with-nil-height", nil)
	require.NoError(t, err)

	batch1 := statedb.NewUpdateBatch()
	batch1.Put("ns", "key1", []byte("value1"), version.NewHeight(1, 4))
	savePoint := version.NewHeight(1, 5)
	require.NoError(t, db.ApplyUpdates(batch1, savePoint))

	batch2 := statedb.NewUpdateBatch()
	batch2.Put("ns", "key1", []byte("value2"), version.NewHeight(1, 1))
	require.NoError(t, db.ApplyUpdates(batch2, nil))

	ht, err := db.GetLatestSavePoint()
	require.NoError(t, err)
	require.Equal(t, savePoint, ht) // savepoint should still be what was set with batch1
	// (because batch2 calls ApplyUpdates with savepoint as nil)
}

// TestDataExportImport is a common test that is invoked by leveldb and couchdb for testing the export and import of
// statedb for snapshotting functionality
func TestDataExportImport(
	t *testing.T,
	dbProvider statedb.VersionedDBProvider,
) {
	sourceDB, err := dbProvider.GetDBHandle("source_ledger", nil)
	require.NoError(t, err)

	// generateSampleData returns a slice of KVs. The returned value contains five KVs for each of the namespaces
	generateSampleData := func(namespaces ...string) []*statedb.VersionedKV {
		sampleData := []*statedb.VersionedKV{}
		for _, ns := range namespaces {
			for i := 0; i < 6; i++ {
				sampleKV := &statedb.VersionedKV{
					CompositeKey: &statedb.CompositeKey{
						Namespace: ns,
						Key:       fmt.Sprintf("key-%d", i),
					},
					VersionedValue: &statedb.VersionedValue{
						Version:  version.NewHeight(1, 1),
						Metadata: []byte(fmt.Sprintf("metadata-for-key-%d-for-%s", i, ns)),
					},
				}
				switch {
				case i < 3:
					binaryVal := fmt.Sprintf("value-for-key-%d-for-%s", i, ns)
					sampleKV.VersionedValue.Value = []byte(binaryVal)
				default:
					jsonVal := fmt.Sprintf(`{"color":"blue,"marble":"m%d", "namespace":"%s"}`, i, ns)
					sampleKV.VersionedValue.Value = []byte(jsonVal)
				}

				sampleData = append(sampleData, sampleKV)
			}
		}
		return sampleData
	}

	// add the sample data for five namespaces to the db
	allnamespaces := stringset{"", "ns1", "ns2", "ns3", "ns4"}
	batch := statedb.NewUpdateBatch()
	for _, kv := range generateSampleData(allnamespaces...) {
		batch.PutValAndMetadata(kv.Namespace, kv.Key, kv.Value, kv.Metadata, kv.Version)
	}
	require.NoError(t, sourceDB.ApplyUpdates(batch, version.NewHeight(5, 5)))

	// verifyExportImport uses FullScanIterator (with skipping zero or more namespaces)
	// for exporting the data to import into another ledger instance and verifies the
	// correctness of the imported data
	verifyExportImport := func(destDBName string, skipNamespaces stringset) {
		fullScanItr, err := sourceDB.GetFullScanIterator(
			func(ns string) bool {
				return skipNamespaces.contains(ns)
			},
		)
		require.NoError(t, err)

		err = dbProvider.ImportFromSnapshot(destDBName, version.NewHeight(10, 10), fullScanItr)
		require.NoError(t, err)

		destinationDB, err := dbProvider.GetDBHandle(destDBName, nil)
		require.NoError(t, err)

		fullScanItr, err = destinationDB.GetFullScanIterator(
			func(ns string) bool {
				return false
			},
		)
		require.NoError(t, err)

		expectedNamespacesInDestinationDB := allnamespaces.minus(skipNamespaces)
		actualResults := []*statedb.VersionedKV{}
		for {
			kv, err := fullScanItr.Next()
			require.NoError(t, err)
			if kv == nil {
				break
			}
			vv, err := destinationDB.GetState(kv.Namespace, kv.Key)
			require.NoError(t, err)
			actualResults = append(
				actualResults,
				&statedb.VersionedKV{
					CompositeKey:   kv.CompositeKey,
					VersionedValue: vv,
				},
			)
		}

		require.Equal(t, generateSampleData(expectedNamespacesInDestinationDB...), actualResults)
		retrievedSavepoint, err := destinationDB.GetLatestSavePoint()
		require.NoError(t, err)
		require.Equal(t, version.NewHeight(10, 10), retrievedSavepoint)
	}

	testCases := []stringset{
		{},                               // skip no namespaces
		{"", "ns1", "ns2", "ns3", "ns4"}, // skip all the namespaces
		{""},                             // skip the first namespace
		{"ns2"},                          // skip the middle namespace
		{"ns4"},                          // skip the last namespace
		{"", "ns1"},                      // skip the first two namespaces
		{"ns3", "ns4"},                   // skip the last two namespaces
		{"", "ns3"},                      // skip two non-consequitive namespaces
		{"ns1", "ns4"},                   // skip two non-consequitive namespaces
		{"", "ns4"},                      // skip the first and last namespace
	}

	for i, testCase := range testCases {
		name := fmt.Sprintf("test_case_%d", i)
		t.Run(
			name,
			func(t *testing.T) {
				verifyExportImport(name, testCase)
			},
		)
	}
}

// CreateTestData creates test data for the given namespace and number of keys.
func CreateTestData(t *testing.T, db statedb.VersionedDB, ns string, numKeys int) []string {
	batch := statedb.NewUpdateBatch()
	expectedKeys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		expectedKeys[i] = fmt.Sprintf("key%d", i)
		vv := statedb.VersionedValue{Value: []byte(fmt.Sprintf("value%d", i)), Version: version.NewHeight(1, uint64(i+1))}
		batch.Put(ns, expectedKeys[i], vv.Value, vv.Version)
	}
	savePoint := version.NewHeight(1, uint64(numKeys))
	require.NoError(t, db.ApplyUpdates(batch, savePoint))
	itr, _ := db.GetStateRangeScanIterator(ns, "", "")
	defer itr.Close()
	for _, expectedKey := range expectedKeys {
		queryResult, err := itr.Next()
		require.NoError(t, err)
		require.Equal(t, expectedKey, queryResult.Key)
	}
	return expectedKeys
}

type stringset []string

func (s stringset) contains(str string) bool {
	for _, element := range s {
		if element == str {
			return true
		}
	}
	return false
}

func (s stringset) minus(toMinus stringset) stringset {
	var final stringset
	for _, element := range s {
		if toMinus.contains(element) {
			continue
		}
		final = append(final, element)
	}
	return final
}
