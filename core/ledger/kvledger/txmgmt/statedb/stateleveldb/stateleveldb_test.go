/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/stretchr/testify/require"
)

func TestBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestBasicRW(t, env.DBProvider)
}

func TestMultiDBBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestMultiDBBasicRW(t, env.DBProvider)
}

func TestDeletes(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestDeletes(t, env.DBProvider)
}

func TestIterator(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestIterator(t, env.DBProvider)
	t.Run("test-iter-error-path", func(t *testing.T) {
		db, err := env.DBProvider.GetDBHandle("testiterator", nil)
		require.NoError(t, err)
		env.DBProvider.Close()
		itr, err := db.GetStateRangeScanIterator("ns1", "", "")
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
		require.Nil(t, itr)
	})
}

func TestDataKeyEncoding(t *testing.T) {
	testDataKeyEncoding(t, "ledger1", "ns", "key")
	testDataKeyEncoding(t, "ledger2", "ns", "")
}

func testDataKeyEncoding(t *testing.T, dbName string, ns string, key string) {
	dataKey := encodeDataKey(ns, key)
	t.Logf("dataKey=%#v", dataKey)
	ns1, key1 := decodeDataKey(dataKey)
	require.Equal(t, ns, ns1)
	require.Equal(t, key, key1)
}

// TestQueryOnLevelDB tests queries on levelDB.
func TestQueryOnLevelDB(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	db, err := env.DBProvider.GetDBHandle("testquery", nil)
	require.NoError(t, err)
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint := version.NewHeight(2, 22)
	db.ApplyUpdates(batch, savePoint)

	// query for owner=jerry, use namespace "ns1"
	// As queries are not supported in levelDB, call to ExecuteQuery()
	// should return a error message
	itr, err := db.ExecuteQuery("ns1", `{"selector":{"owner":"jerry"}}`)
	require.Error(t, err, "ExecuteQuery not supported for leveldb")
	require.Nil(t, itr)
}

func TestGetStateMultipleKeys(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestGetStateMultipleKeys(t, env.DBProvider)
}

func TestGetVersion(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestGetVersion(t, env.DBProvider)
}

func TestUtilityFunctions(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testutilityfunctions", nil)
	require.NoError(t, err)

	// BytesKeySupported should be true for goleveldb
	byteKeySupported := db.BytesKeySupported()
	require.True(t, byteKeySupported)

	// ValidateKeyValue should return nil for a valid key and value
	require.NoError(t, db.ValidateKeyValue("testKey", []byte("testValue")), "leveldb should accept all key-values")
}

func TestValueAndMetadataWrites(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestValueAndMetadataWrites(t, env.DBProvider)
}

func TestPaginatedRangeQuery(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestPaginatedRangeQuery(t, env.DBProvider)
}

func TestRangeQuerySpecialCharacters(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestRangeQuerySpecialCharacters(t, env.DBProvider)
}

func TestApplyUpdatesWithNilHeight(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestApplyUpdatesWithNilHeight(t, env.DBProvider)
}

func TestFullScanIterator(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestFullScanIterator(
		t,
		env.DBProvider,
		byte(1),
		func(dbVal []byte) (*statedb.VersionedValue, error) {
			return decodeValue(dbVal)
		},
	)
}

func TestFullScanIteratorErrorPropagation(t *testing.T) {
	var env *TestVDBEnv
	var cleanup func()
	var vdbProvider *VersionedDBProvider
	var vdb *versionedDB

	initEnv := func() {
		env = NewTestVDBEnv(t)
		vdbProvider = env.DBProvider
		db, err := vdbProvider.GetDBHandle("TestFullScanIteratorErrorPropagation", nil)
		require.NoError(t, err)
		vdb = db.(*versionedDB)
		cleanup = func() {
			env.Cleanup()
		}
	}

	reInitEnv := func() {
		env.Cleanup()
		initEnv()
	}

	initEnv()
	defer cleanup()

	// error from function GetFullScanIterator
	vdbProvider.Close()
	_, _, err := vdb.GetFullScanIterator(
		func(string) bool {
			return false
		},
	)
	require.Contains(t, err.Error(), "internal leveldb error while obtaining db iterator:")

	// error from function Next
	reInitEnv()
	itr, _, err := vdb.GetFullScanIterator(
		func(string) bool {
			return false
		},
	)
	require.NoError(t, err)
	itr.Close()
	_, _, err = itr.Next()
	require.Contains(t, err.Error(), "internal leveldb error while retrieving data from db iterator:")
}
