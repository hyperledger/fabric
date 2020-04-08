/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stateleveldb

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/state"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/stretchr/testify/assert"
)

func TestBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testbasicrw")
	assert.NoError(t, err)
	commontests.TestBasicRW(t, db.(*VersionedDB))
}

func TestMultiDBBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db1, err := env.DBProvider.GetDBHandle("testmultidbbasicrw")
	assert.NoError(t, err)
	db2, err := env.DBProvider.GetDBHandle("testmultidbbasicrw2")
	assert.NoError(t, err)
	commontests.TestMultiDBBasicRW(t, db1.(*VersionedDB), db2.(*VersionedDB))
}

func TestDeletes(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testdeletes")
	assert.NoError(t, err)
	commontests.TestDeletes(t, db.(*VersionedDB))
}

func TestIterator(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testiterator")
	assert.NoError(t, err)
	commontests.TestIterator(t, db.(*VersionedDB))
}

func TestDataKeyEncoding(t *testing.T) {
	testDataKeyEncoding(t, "ledger1", "ns", "key")
	testDataKeyEncoding(t, "ledger2", "ns", "")
}

func testDataKeyEncoding(t *testing.T, dbName string, ns string, key string) {
	dataKey := encodeDataKey(ns, key)
	t.Logf("dataKey=%#v", dataKey)
	ns1, key1 := decodeDataKey(dataKey)
	assert.Equal(t, ns, ns1)
	assert.Equal(t, key, key1)
}

// TestQueryOnLevelDB tests queries on levelDB.
func TestQueryOnLevelDB(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	vdb, err := env.DBProvider.GetDBHandle("testquery")
	assert.NoError(t, err)
	db := vdb.(*VersionedDB)
	db.Open()
	defer db.Close()
	batch := state.NewUpdateBatch()
	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint := version.NewHeight(2, 22)
	db.ApplyUpdates(batch, savePoint)

	// query for owner=jerry, use namespace "ns1"
	// As queries are not supported in levelDB, call to ExecuteQuery()
	// should return a error message
	itr, err := db.ExecuteQuery("ns1", `{"selector":{"owner":"jerry"}}`)
	assert.Error(t, err, "ExecuteQuery not supported for leveldb")
	assert.Nil(t, itr)
}

func TestGetStateMultipleKeys(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testgetmultiplekeys")
	assert.NoError(t, err)
	commontests.TestGetStateMultipleKeys(t, db.(*VersionedDB))
}

func TestGetVersion(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testgetversion")
	assert.NoError(t, err)
	commontests.TestGetVersion(t, db.(*VersionedDB))
}

func TestUtilityFunctions(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	vdb, err := env.DBProvider.GetDBHandle("testutilityfunctions")
	assert.NoError(t, err)
	db := vdb.(*VersionedDB)

	// BytesKeySupported should be true for goleveldb
	byteKeySupported := db.BytesKeySupported()
	assert.True(t, byteKeySupported)

	// ValidateKeyValue should return nil for a valid key and value
	assert.NoError(t, db.ValidateKeyValue("testKey", []byte("testValue")), "leveldb should accept all key-values")
}

func TestValueAndMetadataWrites(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testvalueandmetadata")
	assert.NoError(t, err)
	commontests.TestValueAndMetadataWrites(t, db.(*VersionedDB))
}

func TestPaginatedRangeQuery(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testpaginatedrangequery")
	assert.NoError(t, err)
	commontests.TestPaginatedRangeQuery(t, db.(*VersionedDB))
}

func TestRangeQuerySpecialCharacters(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testrangequeryspecialcharacters")
	assert.NoError(t, err)
	commontests.TestRangeQuerySpecialCharacters(t, db.(*VersionedDB))
}

func TestApplyUpdatesWithNilHeight(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("test-apply-updates-with-nil-height")
	assert.NoError(t, err)
	commontests.TestApplyUpdatesWithNilHeight(t, db.(*VersionedDB))
}
