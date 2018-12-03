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

package stateleveldb

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/txmgmt/statedb/stateleveldb")
	os.Exit(m.Run())
}

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
}

func TestCompositeKey(t *testing.T) {
	testCompositeKey(t, "ledger1", "ns", "key")
	testCompositeKey(t, "ledger2", "ns", "")
}

func testCompositeKey(t *testing.T, dbName string, ns string, key string) {
	compositeKey := constructCompositeKey(ns, key)
	t.Logf("compositeKey=%#v", compositeKey)
	ns1, key1 := splitCompositeKey(compositeKey)
	assert.Equal(t, ns, ns1)
	assert.Equal(t, key, key1)
}

// TestQueryOnLevelDB tests queries on levelDB.
func TestQueryOnLevelDB(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	db, err := env.DBProvider.GetDBHandle("testquery")
	assert.NoError(t, err)
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
	assert.Error(t, err, "ExecuteQuery not supported for leveldb")
	assert.Nil(t, itr)
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

	db, err := env.DBProvider.GetDBHandle("testutilityfunctions")
	assert.NoError(t, err)

	// BytesKeySupported should be true for goleveldb
	byteKeySupported := db.BytesKeySupported()
	assert.True(t, byteKeySupported)

	// ValidateKeyValue should return nil for a valid key and value
	assert.NoError(t, db.ValidateKeyValue("testKey", []byte("testValue")), "leveldb should accept all key-values")
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

func TestApplyUpdatesWithNilHeight(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestApplyUpdatesWithNilHeight(t, env.DBProvider)
}
