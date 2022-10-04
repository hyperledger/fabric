/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/mock"
	"github.com/stretchr/testify/require"
)

// testVDBEnv provides a couch db backed versioned db for testing
type testVDBEnv struct {
	t             *testing.T
	DBProvider    statedb.VersionedDBProvider
	config        *ledger.CouchDBConfig
	cache         *cache
	sysNamespaces []string
	couchDBEnv    *testCouchDBEnv
}

func (env *testVDBEnv) init(t *testing.T, sysNamespaces []string) {
	t.Logf("Initializing TestVDBEnv")

	if env.couchDBEnv == nil {
		couchDBEnv := &testCouchDBEnv{}
		couchDBEnv.startCouchDB(t)
		env.couchDBEnv = couchDBEnv
	}

	redoPath := t.TempDir()
	config := &ledger.CouchDBConfig{
		Address:             env.couchDBEnv.couchAddress,
		Username:            "admin",
		Password:            "adminpw",
		InternalQueryLimit:  1000,
		MaxBatchUpdateSize:  1000,
		MaxRetries:          3,
		MaxRetriesOnStartup: 20,
		RequestTimeout:      35 * time.Second,
		RedoLogPath:         redoPath,
		UserCacheSizeMBs:    8,
	}

	disableKeepAlive = true
	dbProvider, err := NewVersionedDBProvider(config, &disabled.Provider{}, sysNamespaces)
	if err != nil {
		t.Fatalf("Error creating CouchDB Provider: %s", err)
	}

	env.t = t
	env.config = config
	env.DBProvider = dbProvider
	env.config = config
	env.cache = dbProvider.cache
	env.sysNamespaces = sysNamespaces
}

func (env *testVDBEnv) closeAndReopen() {
	env.DBProvider.Close()
	dbProvider, _ := NewVersionedDBProvider(env.config, &disabled.Provider{}, env.sysNamespaces)
	env.DBProvider = dbProvider
	env.cache = dbProvider.cache
}

// Cleanup drops the test couch databases and closes the db provider
func (env *testVDBEnv) cleanup() {
	env.t.Logf("Cleaningup TestVDBEnv")
	if env.DBProvider != nil {
		env.DBProvider.Close()
	}
	env.couchDBEnv.cleanup(env.config)
}

// testVDBEnv provides a couch db for testing
type testCouchDBEnv struct {
	t              *testing.T
	couchAddress   string
	cleanupCouchDB func()
}

// startCouchDB starts external couchDB resources for testCouchDBEnv.
func (env *testCouchDBEnv) startCouchDB(t *testing.T) {
	if env.couchAddress != "" {
		return
	}
	env.t = t
	env.couchAddress, env.cleanupCouchDB = StartCouchDB(t, nil)
}

// stopCouchDB stops external couchDB resources.
func (env *testCouchDBEnv) stopCouchDB() {
	if env.couchAddress != "" {
		env.cleanupCouchDB()
	}
}

func (env *testCouchDBEnv) cleanup(config *ledger.CouchDBConfig) {
	err := DropApplicationDBs(config)
	require.NoError(env.t, err)
}

// we create two CouchDB instances/containers---one is used to test the
// functionality of the versionedDB and another for testing the CouchDB
// util functions.
var (
	vdbEnv     = &testVDBEnv{}
	couchDBEnv = &testCouchDBEnv{}
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("statecouchdb=debug")

	rc := m.Run()
	if vdbEnv.couchDBEnv != nil {
		vdbEnv.couchDBEnv.stopCouchDB()
	}
	couchDBEnv.stopCouchDB()
	os.Exit(rc)
}

func TestBasicRW(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestBasicRW(t, vdbEnv.DBProvider)
}

// TestGetStateFromCache checks cache hits, cache misses, and cache
// updates during GetState call.
func TestGetStateFromCache(t *testing.T) {
	vdbEnv.init(t, []string{"lscc", "_lifecycle"})
	defer vdbEnv.cleanup()

	chainID := "testgetstatefromcache"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// scenario 1: get state would receives a
	// cache miss as the given key does not exist.
	// As the key does not exist in the
	// db also, get state call would not update
	// the cache.
	vv, err := db.GetState("ns", "key1")
	require.NoError(t, err)
	require.Nil(t, vv)
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns", "key1")

	// scenario 2: get state would receive a cache hit.
	// directly store an entry in the cache
	cacheValue := &CacheValue{
		Value:          []byte("value1"),
		Metadata:       []byte("meta1"),
		Version:        version.NewHeight(1, 1).ToBytes(),
		AdditionalInfo: []byte("rev1"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns", "key1", cacheValue))

	vv, err = db.GetState("ns", "key1")
	require.NoError(t, err)
	expectedVV, err := constructVersionedValue(cacheValue)
	require.NoError(t, err)
	require.Equal(t, expectedVV, vv)

	// scenario 3: get state would receives a
	// cache miss as the given key does not present.
	// The value associated with the key would be
	// fetched from the database and the cache would
	// be updated accordingly.

	// store an entry in the db
	batch := statedb.NewUpdateBatch()
	vv2 := &statedb.VersionedValue{Value: []byte("value2"), Metadata: []byte("meta2"), Version: version.NewHeight(1, 2)}
	batch.PutValAndMetadata("lscc", "key1", vv2.Value, vv2.Metadata, vv2.Version)
	savePoint := version.NewHeight(1, 2)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))
	// Note that the ApplyUpdates() updates only the existing entry in the cache. Currently, the
	// cache has only ns, key1 but we are storing lscc, key1. Hence, no changes would happen in the cache.
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "lscc", "key1")

	// calling GetState() would update the cache
	vv, err = db.GetState("lscc", "key1")
	require.NoError(t, err)
	require.Equal(t, vv2, vv)

	// cache should have been updated with lscc, key1
	nsdb, err := db.(*VersionedDB).getNamespaceDBHandle("lscc")
	require.NoError(t, err)
	testExistInCache(t, nsdb, vdbEnv.cache, chainID, "lscc", "key1", vv2)
}

// TestGetVersionFromCache checks cache hits, cache misses, and
// updates during GetVersion call.
func TestGetVersionFromCache(t *testing.T) {
	vdbEnv.init(t, []string{"lscc", "_lifecycle"})
	defer vdbEnv.cleanup()

	chainID := "testgetstatefromcache"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// scenario 1: get version would receives a
	// cache miss as the given key does not exist.
	// As the key does not exist in the
	// db also, get version call would not update
	// the cache.
	ver, err := db.GetVersion("ns", "key1")
	require.Nil(t, err)
	require.Nil(t, ver)
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns", "key1")

	// scenario 2: get version would receive a cache hit.
	// directly store an entry in the cache
	cacheValue := &CacheValue{
		Value:          []byte("value1"),
		Metadata:       []byte("meta1"),
		Version:        version.NewHeight(1, 1).ToBytes(),
		AdditionalInfo: []byte("rev1"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns", "key1", cacheValue))

	ver, err = db.GetVersion("ns", "key1")
	require.NoError(t, err)
	expectedVer, _, err := version.NewHeightFromBytes(cacheValue.Version)
	require.NoError(t, err)
	require.Equal(t, expectedVer, ver)

	// scenario 3: get version would receives a
	// cache miss as the given key does not present.
	// The value associated with the key would be
	// fetched from the database and the cache would
	// be updated accordingly.

	// store an entry in the db
	batch := statedb.NewUpdateBatch()
	vv2 := &statedb.VersionedValue{Value: []byte("value2"), Metadata: []byte("meta2"), Version: version.NewHeight(1, 2)}
	batch.PutValAndMetadata("lscc", "key1", vv2.Value, vv2.Metadata, vv2.Version)
	savePoint := version.NewHeight(1, 2)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))
	// Note that the ApplyUpdates() updates only the existing entry in the cache. Currently, the
	// cache has only ns, key1 but we are storing lscc, key1. Hence, no changes would happen in the cache.
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "lscc", "key1")

	// calling GetVersion() would update the cache
	ver, err = db.GetVersion("lscc", "key1")
	require.NoError(t, err)
	require.Equal(t, vv2.Version, ver)

	// cache should have been updated with lscc, key1
	nsdb, err := db.(*VersionedDB).getNamespaceDBHandle("lscc")
	require.NoError(t, err)
	testExistInCache(t, nsdb, vdbEnv.cache, chainID, "lscc", "key1", vv2)
}

// TestGetMultipleStatesFromCache checks cache hits, cache misses,
// and updates during GetStateMultipleKeys call.
func TestGetMultipleStatesFromCache(t *testing.T) {
	vdbEnv.init(t, []string{"lscc", "_lifecycle"})
	defer vdbEnv.cleanup()

	chainID := "testgetmultiplestatesfromcache"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// scenario: given 5 keys, get multiple states find
	// 2 keys in the cache. The remaining 2 keys would be fetched
	// from the database and the cache would be updated. The last
	// key is not present in the db and hence it won't be sent to
	// the cache.

	// key1 and key2 exist only in the cache
	cacheValue1 := &CacheValue{
		Value:          []byte("value1"),
		Metadata:       []byte("meta1"),
		Version:        version.NewHeight(1, 1).ToBytes(),
		AdditionalInfo: []byte("rev1"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns", "key1", cacheValue1))
	cacheValue2 := &CacheValue{
		Value:          []byte("value2"),
		Metadata:       []byte("meta2"),
		Version:        version.NewHeight(1, 1).ToBytes(),
		AdditionalInfo: []byte("rev2"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns", "key2", cacheValue2))

	// key3 and key4 exist only in the db
	batch := statedb.NewUpdateBatch()
	vv3 := &statedb.VersionedValue{Value: []byte("value3"), Metadata: []byte("meta3"), Version: version.NewHeight(1, 1)}
	batch.PutValAndMetadata("ns", "key3", vv3.Value, vv3.Metadata, vv3.Version)
	vv4 := &statedb.VersionedValue{Value: []byte("value4"), Metadata: []byte("meta4"), Version: version.NewHeight(1, 1)}
	batch.PutValAndMetadata("ns", "key4", vv4.Value, vv4.Metadata, vv4.Version)
	savePoint := version.NewHeight(1, 2)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns", "key3")
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns", "key4")

	// key5 does not exist at all while key3 and key4 does not exist in the cache
	vvalues, err := db.GetStateMultipleKeys("ns", []string{"key1", "key2", "key3", "key4", "key5"})
	require.Nil(t, err)
	vv1, err := constructVersionedValue(cacheValue1)
	require.NoError(t, err)
	vv2, err := constructVersionedValue(cacheValue2)
	require.NoError(t, err)
	require.Equal(t, []*statedb.VersionedValue{vv1, vv2, vv3, vv4, nil}, vvalues)

	// cache should have been updated with key3 and key4
	nsdb, err := db.(*VersionedDB).getNamespaceDBHandle("ns")
	require.NoError(t, err)
	testExistInCache(t, nsdb, vdbEnv.cache, chainID, "ns", "key3", vv3)
	testExistInCache(t, nsdb, vdbEnv.cache, chainID, "ns", "key4", vv4)
}

// TestCacheUpdatesAfterCommit checks whether the cache is updated
// after a commit of a update batch.
func TestCacheUpdatesAfterCommit(t *testing.T) {
	vdbEnv.init(t, []string{"lscc", "_lifecycle"})
	defer vdbEnv.cleanup()

	chainID := "testcacheupdatesaftercommit"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// scenario: cache has 4 keys while the commit operation
	// updates 2 of those keys, delete the remaining 2 keys, and
	// adds a new key. At the end of the commit operation, only
	// those 2 keys should be present with the recent value
	// in the cache and the new key should not be present in the cache.

	// store 4 keys in the db
	batch := statedb.NewUpdateBatch()
	vv1 := &statedb.VersionedValue{Value: []byte("value1"), Metadata: []byte("meta1"), Version: version.NewHeight(1, 2)}
	vv2 := &statedb.VersionedValue{Value: []byte("value2"), Metadata: []byte("meta2"), Version: version.NewHeight(1, 2)}
	vv3 := &statedb.VersionedValue{Value: []byte("value3"), Metadata: []byte("meta3"), Version: version.NewHeight(1, 2)}
	vv4 := &statedb.VersionedValue{Value: []byte("value4"), Metadata: []byte("meta4"), Version: version.NewHeight(1, 2)}

	batch.PutValAndMetadata("ns1", "key1", vv1.Value, vv1.Metadata, vv1.Version)
	batch.PutValAndMetadata("ns1", "key2", vv2.Value, vv2.Metadata, vv2.Version)
	batch.PutValAndMetadata("ns2", "key1", vv3.Value, vv3.Metadata, vv3.Version)
	batch.PutValAndMetadata("ns2", "key2", vv4.Value, vv4.Metadata, vv4.Version)
	savePoint := version.NewHeight(1, 5)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// key1, key2 in ns1 and ns2 would not be in cache
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns1", "key1")
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns1", "key2")
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns2", "key1")
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns2", "key2")

	// add key1 and key2 from ns1 to the cache
	_, err = db.GetState("ns1", "key1")
	require.NoError(t, err)
	_, err = db.GetState("ns1", "key2")
	require.NoError(t, err)
	// add key1 and key2 from ns2 to the cache
	_, err = db.GetState("ns2", "key1")
	require.NoError(t, err)
	_, err = db.GetState("ns2", "key2")
	require.NoError(t, err)

	v, err := vdbEnv.cache.getState(chainID, "ns1", "key1")
	require.NoError(t, err)
	ns1key1rev := string(v.AdditionalInfo)

	v, err = vdbEnv.cache.getState(chainID, "ns1", "key2")
	require.NoError(t, err)
	ns1key2rev := string(v.AdditionalInfo)

	// update key1 and key2 in ns1. delete key1 and key2 in ns2. add a new key3 in ns2.
	batch = statedb.NewUpdateBatch()
	vv1Update := &statedb.VersionedValue{Value: []byte("new-value1"), Metadata: []byte("meta1"), Version: version.NewHeight(2, 2)}
	vv2Update := &statedb.VersionedValue{Value: []byte("new-value2"), Metadata: []byte("meta2"), Version: version.NewHeight(2, 2)}
	vv3Update := &statedb.VersionedValue{Version: version.NewHeight(2, 4)}
	vv4Update := &statedb.VersionedValue{Version: version.NewHeight(2, 5)}
	vv5 := &statedb.VersionedValue{Value: []byte("value5"), Metadata: []byte("meta5"), Version: version.NewHeight(1, 2)}

	batch.PutValAndMetadata("ns1", "key1", vv1Update.Value, vv1Update.Metadata, vv1Update.Version)
	batch.PutValAndMetadata("ns1", "key2", vv2Update.Value, vv2Update.Metadata, vv2Update.Version)
	batch.Delete("ns2", "key1", vv3Update.Version)
	batch.Delete("ns2", "key2", vv4Update.Version)
	batch.PutValAndMetadata("ns2", "key3", vv5.Value, vv5.Metadata, vv5.Version)
	savePoint = version.NewHeight(2, 5)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// cache should have only the update key1 and key2 in ns1
	cacheValue, err := vdbEnv.cache.getState(chainID, "ns1", "key1")
	require.NoError(t, err)
	vv, err := constructVersionedValue(cacheValue)
	require.NoError(t, err)
	require.Equal(t, vv1Update, vv)
	require.NotEqual(t, ns1key1rev, string(cacheValue.AdditionalInfo))

	cacheValue, err = vdbEnv.cache.getState(chainID, "ns1", "key2")
	require.NoError(t, err)
	vv, err = constructVersionedValue(cacheValue)
	require.NoError(t, err)
	require.Equal(t, vv2Update, vv)
	require.NotEqual(t, ns1key2rev, string(cacheValue.AdditionalInfo))

	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns2", "key1")
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns2", "key2")
	testDoesNotExistInCache(t, vdbEnv.cache, chainID, "ns2", "key3")
}

func TestMultiDBBasicRW(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestMultiDBBasicRW(t, vdbEnv.DBProvider)
}

func TestDeletes(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestDeletes(t, vdbEnv.DBProvider)
}

func TestIterator(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestIterator(t, vdbEnv.DBProvider)
}

// The following tests are unique to couchdb, they are not used in leveldb
//
//	query test
func TestQuery(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestQuery(t, vdbEnv.DBProvider)
}

func TestGetStateMultipleKeys(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestGetStateMultipleKeys(t, vdbEnv.DBProvider)
}

func TestGetVersion(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestGetVersion(t, vdbEnv.DBProvider)
}

func TestSmallBatchSize(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestSmallBatchSize(t, vdbEnv.DBProvider)
}

func TestBatchRetry(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestBatchWithIndividualRetry(t, vdbEnv.DBProvider)
}

func TestValueAndMetadataWrites(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestValueAndMetadataWrites(t, vdbEnv.DBProvider)
}

func TestPaginatedRangeQuery(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestPaginatedRangeQuery(t, vdbEnv.DBProvider)
}

func TestRangeQuerySpecialCharacters(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	commontests.TestRangeQuerySpecialCharacters(t, vdbEnv.DBProvider)
}

// TestUtilityFunctions tests utility functions
func TestUtilityFunctions(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	db, err := vdbEnv.DBProvider.GetDBHandle("testutilityfunctions", nil)
	require.NoError(t, err)

	require.False(t, vdbEnv.DBProvider.BytesKeySupported())
	require.False(t, db.BytesKeySupported())

	// ValidateKeyValue should return nil for a valid key and value
	err = db.ValidateKeyValue("testKey", []byte("Some random bytes"))
	require.Nil(t, err)

	// ValidateKeyValue should return an error for a key that is not a utf-8 valid string
	err = db.ValidateKeyValue(string([]byte{0xff, 0xfe, 0xfd}), []byte("Some random bytes"))
	require.Error(t, err, "ValidateKey should have thrown an error for an invalid utf-8 string")

	// ValidateKeyValue should return an error for a key that is an empty string
	require.EqualError(t, db.ValidateKeyValue("", []byte("validValue")),
		"invalid key. Empty string is not supported as a key by couchdb")

	reservedFields := []string{"~version", "_id", "_test"}

	// ValidateKeyValue should return an error for a json value that contains one of the reserved fields
	// at the top level
	for _, reservedField := range reservedFields {
		testVal := fmt.Sprintf(`{"%s":"dummyVal"}`, reservedField)
		err = db.ValidateKeyValue("testKey", []byte(testVal))
		require.Error(t, err, fmt.Sprintf(
			"ValidateKey should have thrown an error for a json value %s, as contains one of the reserved fields", testVal))
	}

	// ValidateKeyValue should not return an error for a json value that contains one of the reserved fields
	// if not at the top level
	for _, reservedField := range reservedFields {
		testVal := fmt.Sprintf(`{"data.%s":"dummyVal"}`, reservedField)
		err = db.ValidateKeyValue("testKey", []byte(testVal))
		require.NoError(t, err, fmt.Sprintf(
			"ValidateKey should not have thrown an error the json value %s since the reserved field was not at the top level", testVal))
	}

	// ValidateKeyValue should return an error for a key that begins with an underscore
	err = db.ValidateKeyValue("_testKey", []byte("testValue"))
	require.Error(t, err, "ValidateKey should have thrown an error for a key that begins with an underscore")
}

// TestInvalidJSONFields tests for invalid JSON fields
func TestInvalidJSONFields(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	db, err := vdbEnv.DBProvider.GetDBHandle("testinvalidfields", nil)
	require.NoError(t, err)

	require.NoError(t, db.Open())
	defer db.Close()

	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"_id":"key1","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint := version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	require.Error(t, err, "Invalid field _id should have thrown an error")

	batch = statedb.NewUpdateBatch()
	jsonValue1 = `{"_rev":"rev1","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint = version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	require.Error(t, err, "Invalid field _rev should have thrown an error")

	batch = statedb.NewUpdateBatch()
	jsonValue1 = `{"_deleted":"true","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint = version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	require.Error(t, err, "Invalid field _deleted should have thrown an error")

	batch = statedb.NewUpdateBatch()
	jsonValue1 = `{"~version":"v1","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint = version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	require.Error(t, err, "Invalid field ~version should have thrown an error")
}

func TestDebugFunctions(t *testing.T) {
	// Test printCompositeKeys
	// initialize a key list
	loadKeys := []*statedb.CompositeKey{}
	// create a composite key and add to the key list
	compositeKey3 := statedb.CompositeKey{Namespace: "ns", Key: "key3"}
	loadKeys = append(loadKeys, &compositeKey3)
	compositeKey4 := statedb.CompositeKey{Namespace: "ns", Key: "key4"}
	loadKeys = append(loadKeys, &compositeKey4)
	require.Equal(t, "[ns,key3],[ns,key4]", printCompositeKeys(loadKeys))
}

func TestHandleChaincodeDeploy(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	db, err := vdbEnv.DBProvider.GetDBHandle("testinit", nil)
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

	indexData := map[string][]byte{
		"META-INF/statedb/couchdb/indexes/indexColorSortName.json":                                               []byte(`{"index":{"fields":[{"color":"desc"}]},"ddoc":"indexColorSortName","name":"indexColorSortName","type":"json"}`),
		"META-INF/statedb/couchdb/indexes/indexSizeSortName.json":                                                []byte(`{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`),
		"META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexCollMarbles.json":                   []byte(`{"index":{"fields":["docType","owner"]},"ddoc":"indexCollectionMarbles", "name":"indexCollectionMarbles","type":"json"}`),
		"META-INF/statedb/couchdb/collections/collectionMarblesPrivateDetails/indexes/indexCollPrivDetails.json": []byte(`{"index":{"fields":["docType","price"]},"ddoc":"indexPrivateDetails", "name":"indexPrivateDetails","type":"json"}`),
	}

	// Create a query
	queryString := `{"selector":{"owner":"fred"}}`

	_, err = db.ExecuteQuery("ns1", queryString)
	require.NoError(t, err)

	// Create a query with a sort
	queryString = `{"selector":{"owner":"fred"}, "sort": [{"size": "desc"}]}`

	_, err = db.ExecuteQuery("ns1", queryString)
	require.Error(t, err, "Error should have been thrown for a missing index")

	indexCapable, ok := db.(statedb.IndexCapable)
	if !ok {
		t.Fatalf("Couchdb state impl is expected to implement interface `statedb.IndexCapable`")
	}
	require.NoError(t, indexCapable.ProcessIndexesForChaincodeDeploy("ns1", indexData))

	queryString = `{"selector":{"owner":"fred"}, "sort": [{"size": "desc"}]}`
	queryUsingIndex := func() bool {
		_, err = db.ExecuteQuery("ns1", queryString)
		return err == nil
	}
	require.Eventually(t, queryUsingIndex, 2*time.Second, 100*time.Millisecond, "error executing query with sort")

	// Query namespace "ns2", index is only created in "ns1".  This should return an error.
	_, err = db.ExecuteQuery("ns2", queryString)
	require.Error(t, err, "Error should have been thrown for a missing index")
}

func TestTryCastingToJSON(t *testing.T) {
	sampleJSON := []byte(`{"a":"A", "b":"B"}`)
	isJSON, jsonVal := tryCastingToJSON(sampleJSON)
	require.True(t, isJSON)
	require.Equal(t, "A", jsonVal["a"])
	require.Equal(t, "B", jsonVal["b"])

	sampleNonJSON := []byte(`This is not a json`)
	isJSON, _ = tryCastingToJSON(sampleNonJSON)
	require.False(t, isJSON)
}

func TestIndexDeploymentWithOrderAndBadSyntax(t *testing.T) {
	channelName := "ch1"
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	db, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	require.NoError(t, db.Open())
	defer db.Close()

	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte(`{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte(`{"asset_name": "marble2","color": "blue","size": 2,"owner": "jerry"}`), version.NewHeight(1, 2))

	indexCapable, ok := db.(statedb.IndexCapable)
	if !ok {
		t.Fatalf("Couchdb state impl is expected to implement interface `statedb.IndexCapable`")
	}

	badSyntaxFileContent := `{"index":{"fields": This is a bad json}`
	indexData := map[string][]byte{
		"META-INF/statedb/couchdb/indexes/indexColorSortName.json":                             []byte(`{"index":{"fields":[{"color":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`),
		"META-INF/statedb/couchdb/indexes/indexSizeSortName.json":                              []byte(`{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`),
		"META-INF/statedb/couchdb/indexes/badSyntax.json":                                      []byte(badSyntaxFileContent),
		"META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexCollMarbles.json": []byte(`{"index":{"fields":["docType","owner"]},"ddoc":"indexCollectionMarbles", "name":"indexCollectionMarbles","type":"json"}`),
	}

	// as the indexes are sorted by file names, the order of index processing would be
	// (1) indexCollMarbles.json, (2) badSyntax.json, (3) indexColorSortName, (4) indexSizeSortName.
	// As the indexColorSortName.json and indexSizeSortName has the same index name but different
	// index fields, the later would replace the former, i.e., index would be created on size field
	// rather than the color field. Further, the index with a bad syntax would not stop the processing
	// of other valid indexes.
	require.NoError(t, indexCapable.ProcessIndexesForChaincodeDeploy("ns1", indexData))

	queryString := `{"selector":{"owner":"fred"}, "sort": [{"docType": "desc"}]}`
	queryUsingIndex := func() bool {
		_, err = db.ExecuteQuery("ns1", queryString)
		return err == nil
	}
	require.Eventually(t, queryUsingIndex, 2*time.Second, 100*time.Millisecond, "error executing query with sort")

	queryString = `{"selector":{"owner":"fred"}, "sort": [{"size": "desc"}]}`
	queryUsingIndex = func() bool {
		_, err = db.ExecuteQuery("ns1", queryString)
		return err == nil
	}
	require.Eventually(t, queryUsingIndex, 2*time.Second, 100*time.Millisecond, "error executing query with sort")

	// though the indexColorSortName.json is processed before indexSizeSortName.json as per the order,
	// the later would replace the former as the index names are the same. Hence, a query using the color
	// field in sort should fail.
	queryString = `{"selector":{"owner":"fred"}, "sort": [{"color": "desc"}]}`
	queryUsingIndex = func() bool {
		_, err = db.ExecuteQuery("ns1", queryString)
		return err == nil
	}
	require.Never(t, queryUsingIndex, 2*time.Second, 100*time.Millisecond, "error should have occurred as there is no index on color field")
}

func TestIsBulkOptimizable(t *testing.T) {
	var db statedb.VersionedDB = &VersionedDB{}
	_, ok := db.(statedb.BulkOptimizable)
	if !ok {
		t.Fatal("state couch db is expected to implement interface statedb.BulkOptimizable")
	}
}

func printCompositeKeys(keys []*statedb.CompositeKey) string {
	compositeKeyString := []string{}
	for _, key := range keys {
		compositeKeyString = append(compositeKeyString, "["+key.Namespace+","+key.Key+"]")
	}
	return strings.Join(compositeKeyString, ",")
}

// TestPaginatedQuery tests queries with pagination
func TestPaginatedQuery(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	db, err := vdbEnv.DBProvider.GetDBHandle("testpaginatedquery", nil)
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

	jsonValue11 := `{"asset_name": "marble11","color": "cyan","size": 11,"owner": "joe"}`
	batch.Put("ns1", "key11", []byte(jsonValue11), version.NewHeight(1, 11))
	jsonValue12 := `{"asset_name": "marble12","color": "red","size": 12,"owner": "martha"}`
	batch.Put("ns1", "key12", []byte(jsonValue12), version.NewHeight(1, 4))
	jsonValue13 := `{"asset_name": "marble13","color": "red","size": 13,"owner": "james"}`
	batch.Put("ns1", "key13", []byte(jsonValue13), version.NewHeight(1, 4))
	jsonValue14 := `{"asset_name": "marble14","color": "red","size": 14,"owner": "fred"}`
	batch.Put("ns1", "key14", []byte(jsonValue14), version.NewHeight(1, 4))
	jsonValue15 := `{"asset_name": "marble15","color": "red","size": 15,"owner": "mary"}`
	batch.Put("ns1", "key15", []byte(jsonValue15), version.NewHeight(1, 4))
	jsonValue16 := `{"asset_name": "marble16","color": "red","size": 16,"owner": "robert"}`
	batch.Put("ns1", "key16", []byte(jsonValue16), version.NewHeight(1, 4))
	jsonValue17 := `{"asset_name": "marble17","color": "red","size": 17,"owner": "alan"}`
	batch.Put("ns1", "key17", []byte(jsonValue17), version.NewHeight(1, 4))
	jsonValue18 := `{"asset_name": "marble18","color": "red","size": 18,"owner": "elaine"}`
	batch.Put("ns1", "key18", []byte(jsonValue18), version.NewHeight(1, 4))
	jsonValue19 := `{"asset_name": "marble19","color": "red","size": 19,"owner": "alan"}`
	batch.Put("ns1", "key19", []byte(jsonValue19), version.NewHeight(1, 4))
	jsonValue20 := `{"asset_name": "marble20","color": "red","size": 20,"owner": "elaine"}`
	batch.Put("ns1", "key20", []byte(jsonValue20), version.NewHeight(1, 4))

	jsonValue21 := `{"asset_name": "marble21","color": "cyan","size": 21,"owner": "joe"}`
	batch.Put("ns1", "key21", []byte(jsonValue21), version.NewHeight(1, 11))
	jsonValue22 := `{"asset_name": "marble22","color": "red","size": 22,"owner": "martha"}`
	batch.Put("ns1", "key22", []byte(jsonValue22), version.NewHeight(1, 4))
	jsonValue23 := `{"asset_name": "marble23","color": "blue","size": 23,"owner": "james"}`
	batch.Put("ns1", "key23", []byte(jsonValue23), version.NewHeight(1, 4))
	jsonValue24 := `{"asset_name": "marble24","color": "red","size": 24,"owner": "fred"}`
	batch.Put("ns1", "key24", []byte(jsonValue24), version.NewHeight(1, 4))
	jsonValue25 := `{"asset_name": "marble25","color": "red","size": 25,"owner": "mary"}`
	batch.Put("ns1", "key25", []byte(jsonValue25), version.NewHeight(1, 4))
	jsonValue26 := `{"asset_name": "marble26","color": "red","size": 26,"owner": "robert"}`
	batch.Put("ns1", "key26", []byte(jsonValue26), version.NewHeight(1, 4))
	jsonValue27 := `{"asset_name": "marble27","color": "green","size": 27,"owner": "alan"}`
	batch.Put("ns1", "key27", []byte(jsonValue27), version.NewHeight(1, 4))
	jsonValue28 := `{"asset_name": "marble28","color": "red","size": 28,"owner": "elaine"}`
	batch.Put("ns1", "key28", []byte(jsonValue28), version.NewHeight(1, 4))
	jsonValue29 := `{"asset_name": "marble29","color": "red","size": 29,"owner": "alan"}`
	batch.Put("ns1", "key29", []byte(jsonValue29), version.NewHeight(1, 4))
	jsonValue30 := `{"asset_name": "marble30","color": "red","size": 30,"owner": "elaine"}`
	batch.Put("ns1", "key30", []byte(jsonValue30), version.NewHeight(1, 4))

	jsonValue31 := `{"asset_name": "marble31","color": "cyan","size": 31,"owner": "joe"}`
	batch.Put("ns1", "key31", []byte(jsonValue31), version.NewHeight(1, 11))
	jsonValue32 := `{"asset_name": "marble32","color": "red","size": 32,"owner": "martha"}`
	batch.Put("ns1", "key32", []byte(jsonValue32), version.NewHeight(1, 4))
	jsonValue33 := `{"asset_name": "marble33","color": "red","size": 33,"owner": "james"}`
	batch.Put("ns1", "key33", []byte(jsonValue33), version.NewHeight(1, 4))
	jsonValue34 := `{"asset_name": "marble34","color": "red","size": 34,"owner": "fred"}`
	batch.Put("ns1", "key34", []byte(jsonValue34), version.NewHeight(1, 4))
	jsonValue35 := `{"asset_name": "marble35","color": "red","size": 35,"owner": "mary"}`
	batch.Put("ns1", "key35", []byte(jsonValue35), version.NewHeight(1, 4))
	jsonValue36 := `{"asset_name": "marble36","color": "orange","size": 36,"owner": "robert"}`
	batch.Put("ns1", "key36", []byte(jsonValue36), version.NewHeight(1, 4))
	jsonValue37 := `{"asset_name": "marble37","color": "red","size": 37,"owner": "alan"}`
	batch.Put("ns1", "key37", []byte(jsonValue37), version.NewHeight(1, 4))
	jsonValue38 := `{"asset_name": "marble38","color": "yellow","size": 38,"owner": "elaine"}`
	batch.Put("ns1", "key38", []byte(jsonValue38), version.NewHeight(1, 4))
	jsonValue39 := `{"asset_name": "marble39","color": "red","size": 39,"owner": "alan"}`
	batch.Put("ns1", "key39", []byte(jsonValue39), version.NewHeight(1, 4))
	jsonValue40 := `{"asset_name": "marble40","color": "red","size": 40,"owner": "elaine"}`
	batch.Put("ns1", "key40", []byte(jsonValue40), version.NewHeight(1, 4))

	savePoint := version.NewHeight(2, 22)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	indexData := map[string][]byte{
		"META-INF/statedb/couchdb/indexes/indexSizeSortName.json": []byte(`{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`),
	}

	// Create a query
	queryString := `{"selector":{"color":"red"}}`

	_, err = db.ExecuteQuery("ns1", queryString)
	require.NoError(t, err)

	indexCapable, ok := db.(statedb.IndexCapable)
	if !ok {
		t.Fatalf("Couchdb state impl is expected to implement interface `statedb.IndexCapable`")
	}

	require.NoError(t, indexCapable.ProcessIndexesForChaincodeDeploy("ns1", indexData))
	// Sleep to allow time for index creation
	time.Sleep(100 * time.Millisecond)
	// Create a query with a sort
	queryString = `{"selector":{"color":"red"}, "sort": [{"size": "asc"}]}`

	// Query should complete without error
	_, err = db.ExecuteQuery("ns1", queryString)
	require.NoError(t, err)

	// Test explicit paging
	// Execute 3 page queries, there are 28 records with color red, use page size 10
	returnKeys := []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15", "key16"}
	bookmark, err := executeQuery(t, db, "ns1", queryString, "", int32(10), returnKeys)
	require.NoError(t, err)
	returnKeys = []string{"key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29"}
	bookmark, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	require.NoError(t, err)

	returnKeys = []string{"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40"}
	_, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	require.NoError(t, err)

	// Test explicit paging
	// Increase pagesize to 50,  should return all values
	returnKeys = []string{
		"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15",
		"key16", "key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29",
		"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40",
	}
	_, err = executeQuery(t, db, "ns1", queryString, "", int32(50), returnKeys)
	require.NoError(t, err)

	// Test explicit paging
	// Pagesize is 10, so all 28 records should be return in 3 "pages"
	returnKeys = []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15", "key16"}
	bookmark, err = executeQuery(t, db, "ns1", queryString, "", int32(10), returnKeys)
	require.NoError(t, err)
	returnKeys = []string{"key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29"}
	bookmark, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	require.NoError(t, err)
	returnKeys = []string{"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40"}
	_, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	require.NoError(t, err)

	// Test implicit paging
	returnKeys = []string{
		"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15",
		"key16", "key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29",
		"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40",
	}
	_, err = executeQuery(t, db, "ns1", queryString, "", int32(0), returnKeys)
	require.NoError(t, err)

	// pagesize greater than querysize will execute with implicit paging
	returnKeys = []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15", "key16"}
	_, err = executeQuery(t, db, "ns1", queryString, "", int32(10), returnKeys)
	require.NoError(t, err)
}

func executeQuery(t *testing.T, db statedb.VersionedDB, namespace, query, bookmark string, pageSize int32, returnKeys []string) (string, error) {
	var itr statedb.ResultsIterator
	var err error

	if pageSize == int32(0) && bookmark == "" {
		itr, err = db.ExecuteQuery(namespace, query)
		if err != nil {
			return "", err
		}
	} else {
		itr, err = db.ExecuteQueryWithPagination(namespace, query, bookmark, pageSize)
		if err != nil {
			return "", err
		}
	}

	// Verify the keys returned
	commontests.TestItrWithoutClose(t, itr, returnKeys)

	returnBookmark := ""
	if queryResultItr, ok := itr.(statedb.QueryResultsIterator); ok {
		returnBookmark = queryResultItr.GetBookmarkAndClose()
	}

	return returnBookmark, nil
}

func TestApplyUpdatesWithNilHeight(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	commontests.TestApplyUpdatesWithNilHeight(t, vdbEnv.DBProvider)
}

func TestRangeScanWithCouchInternalDocsPresent(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	db, err := vdbEnv.DBProvider.GetDBHandle("testrangescanfiltercouchinternaldocs", nil)
	require.NoError(t, err)
	couchDatabse, err := db.(*VersionedDB).getNamespaceDBHandle("ns")
	require.NoError(t, err)
	require.NoError(t, db.Open())
	defer db.Close()
	_, err = couchDatabse.createIndex(`{
		"index" : {"fields" : ["asset_name"]},
			"ddoc" : "indexAssetName",
			"name" : "indexAssetName",
			"type" : "json"
		}`)
	require.NoError(t, err)

	_, err = couchDatabse.createIndex(`{
		"index" : {"fields" : ["assetValue"]},
			"ddoc" : "indexAssetValue",
			"name" : "indexAssetValue",
			"type" : "json"
		}`)
	require.NoError(t, err)

	batch := statedb.NewUpdateBatch()
	for i := 1; i <= 3; i++ {
		keySmallerThanDesignDoc := fmt.Sprintf("Key-%d", i)
		keyGreaterThanDesignDoc := fmt.Sprintf("key-%d", i)
		jsonValue := fmt.Sprintf(`{"asset_name": "marble-%d"}`, i)
		batch.Put("ns", keySmallerThanDesignDoc, []byte(jsonValue), version.NewHeight(1, uint64(i)))
		batch.Put("ns", keyGreaterThanDesignDoc, []byte(jsonValue), version.NewHeight(1, uint64(i)))
	}
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(2, 2)))
	require.NoError(t, err)

	// The Keys in db are in this order
	// Key-1, Key-2, Key-3,_design/indexAssetNam, _design/indexAssetValue, key-1, key-2, key-3
	// query different ranges and verify results
	s, err := newQueryScanner("ns", couchDatabse, "", 3, 3, "", "", "")
	require.NoError(t, err)
	assertQueryResults(t, s.resultsInfo.results, []string{"Key-1", "Key-2", "Key-3"})
	require.Equal(t, "key-1", s.queryDefinition.startKey)

	s, err = newQueryScanner("ns", couchDatabse, "", 4, 4, "", "", "")
	require.NoError(t, err)
	assertQueryResults(t, s.resultsInfo.results, []string{"Key-1", "Key-2", "Key-3", "key-1"})
	require.Equal(t, "key-2", s.queryDefinition.startKey)

	s, err = newQueryScanner("ns", couchDatabse, "", 2, 2, "", "", "")
	require.NoError(t, err)
	assertQueryResults(t, s.resultsInfo.results, []string{"Key-1", "Key-2"})
	require.Equal(t, "Key-3", s.queryDefinition.startKey)
	require.NoError(t, s.getNextStateRangeScanResults())
	assertQueryResults(t, s.resultsInfo.results, []string{"Key-3", "key-1"})
	require.Equal(t, "key-2", s.queryDefinition.startKey)

	s, err = newQueryScanner("ns", couchDatabse, "", 2, 2, "", "_", "")
	require.NoError(t, err)
	assertQueryResults(t, s.resultsInfo.results, []string{"key-1", "key-2"})
	require.Equal(t, "key-3", s.queryDefinition.startKey)
}

func assertQueryResults(t *testing.T, results []*queryResult, expectedIds []string) {
	var actualIds []string
	for _, res := range results {
		actualIds = append(actualIds, res.id)
	}
	require.Equal(t, expectedIds, actualIds)
}

func TestFormatCheck(t *testing.T) {
	testCases := []struct {
		dataFormat     string                        // precondition
		dataExists     bool                          // precondition
		expectedFormat string                        // postcondition
		expectedErr    *dataformat.ErrFormatMismatch // postcondition
	}{
		{
			dataFormat: "",
			dataExists: true,
			expectedErr: &dataformat.ErrFormatMismatch{
				DBInfo:         "CouchDB for state database",
				Format:         "",
				ExpectedFormat: "2.0",
			},
			expectedFormat: "does not matter as the test should not reach to check this",
		},

		{
			dataFormat:     "",
			dataExists:     false,
			expectedErr:    nil,
			expectedFormat: dataformat.CurrentFormat,
		},

		{
			dataFormat:     dataformat.CurrentFormat,
			dataExists:     false,
			expectedFormat: dataformat.CurrentFormat,
			expectedErr:    nil,
		},

		{
			dataFormat:     dataformat.CurrentFormat,
			dataExists:     true,
			expectedFormat: dataformat.CurrentFormat,
			expectedErr:    nil,
		},

		{
			dataFormat: "3.0",
			dataExists: true,
			expectedErr: &dataformat.ErrFormatMismatch{
				DBInfo:         "CouchDB for state database",
				Format:         "3.0",
				ExpectedFormat: dataformat.CurrentFormat,
			},
			expectedFormat: "does not matter as the test should not reach to check this",
		},
	}

	vdbEnv.init(t, nil)
	for i, testCase := range testCases {
		t.Run(
			fmt.Sprintf("testCase %d", i),
			func(t *testing.T) {
				testFormatCheck(t, testCase.dataFormat, testCase.dataExists, testCase.expectedErr, testCase.expectedFormat, vdbEnv)
			})
	}
}

func testFormatCheck(t *testing.T, dataFormat string, dataExists bool, expectedErr *dataformat.ErrFormatMismatch, expectedFormat string, vdbEnv *testVDBEnv) {
	redoPath := t.TempDir()
	config := &ledger.CouchDBConfig{
		Address:             vdbEnv.couchDBEnv.couchAddress,
		Username:            "admin",
		Password:            "adminpw",
		MaxRetries:          3,
		MaxRetriesOnStartup: 20,
		RequestTimeout:      35 * time.Second,
		RedoLogPath:         redoPath,
	}
	dbProvider, err := NewVersionedDBProvider(config, &disabled.Provider{}, nil)
	require.NoError(t, err)

	// create preconditions for test
	if dataExists {
		db, err := dbProvider.GetDBHandle("testns", nil)
		require.NoError(t, err)
		batch := statedb.NewUpdateBatch()
		batch.Put("testns", "testkey", []byte("testVal"), version.NewHeight(1, 1))
		require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 1)))
	}
	if dataFormat == "" {
		err := dropDB(dbProvider.couchInstance, fabricInternalDBName)
		require.NoError(t, err)
	} else {
		require.NoError(t, writeDataFormatVersion(dbProvider.couchInstance, dataFormat))
	}
	dbProvider.Close()
	defer func() {
		require.NoError(t, DropApplicationDBs(vdbEnv.config))
	}()

	// close and reopen with preconditions set and check the expected behavior
	dbProvider, err = NewVersionedDBProvider(config, &disabled.Provider{}, nil)
	if expectedErr != nil {
		require.Equal(t, expectedErr, err)
		return
	}
	require.NoError(t, err)
	defer func() {
		if dbProvider != nil {
			dbProvider.Close()
		}
	}()
	format, err := readDataformatVersion(dbProvider.couchInstance)
	require.NoError(t, err)
	require.Equal(t, expectedFormat, format)
}

func testDoesNotExistInCache(t *testing.T, cache *cache, chainID, ns, key string) {
	cacheValue, err := cache.getState(chainID, ns, key)
	require.NoError(t, err)
	require.Nil(t, cacheValue)
}

func testExistInCache(t *testing.T, db *couchDatabase, cache *cache, chainID, ns, key string, expectedVV *statedb.VersionedValue) {
	cacheValue, err := cache.getState(chainID, ns, key)
	require.NoError(t, err)
	vv, err := constructVersionedValue(cacheValue)
	require.NoError(t, err)
	require.Equal(t, expectedVV, vv)
	metadata, err := retrieveNsMetadata(db, []string{key})
	require.NoError(t, err)
	require.Equal(t, metadata[0].Rev, string(cacheValue.AdditionalInfo))
}

func TestLoadCommittedVersion(t *testing.T) {
	vdbEnv.init(t, []string{"lscc", "_lifecycle"})
	defer vdbEnv.cleanup()

	chainID := "testloadcommittedversion"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// scenario: state cache has (ns1, key1), (ns1, key2),
	// and (ns2, key1) but misses (ns2, key2). The
	// LoadCommittedVersions will fetch the first
	// three keys from the state cache and the remaining one from
	// the db. To ensure that, the db contains only
	// the missing key (ns2, key2).

	// store (ns1, key1), (ns1, key2), (ns2, key1) in the state cache
	cacheValue := &CacheValue{
		Value:          []byte("value1"),
		Metadata:       []byte("meta1"),
		Version:        version.NewHeight(1, 1).ToBytes(),
		AdditionalInfo: []byte("rev1"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns1", "key1", cacheValue))

	cacheValue = &CacheValue{
		Value:          []byte("value2"),
		Metadata:       []byte("meta2"),
		Version:        version.NewHeight(1, 2).ToBytes(),
		AdditionalInfo: []byte("rev2"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns1", "key2", cacheValue))

	cacheValue = &CacheValue{
		Value:          []byte("value3"),
		Metadata:       []byte("meta3"),
		Version:        version.NewHeight(1, 3).ToBytes(),
		AdditionalInfo: []byte("rev3"),
	}
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns2", "key1", cacheValue))

	// store (ns2, key2) in the db
	batch := statedb.NewUpdateBatch()
	vv := &statedb.VersionedValue{Value: []byte("value4"), Metadata: []byte("meta4"), Version: version.NewHeight(1, 4)}
	batch.PutValAndMetadata("ns2", "key2", vv.Value, vv.Metadata, vv.Version)
	savePoint := version.NewHeight(2, 2)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// version cache should be empty
	ver, ok := db.(*VersionedDB).GetCachedVersion("ns1", "key1")
	require.Nil(t, ver)
	require.False(t, ok)
	ver, ok = db.(*VersionedDB).GetCachedVersion("ns1", "key2")
	require.Nil(t, ver)
	require.False(t, ok)
	ver, ok = db.(*VersionedDB).GetCachedVersion("ns2", "key1")
	require.Nil(t, ver)
	require.False(t, ok)
	ver, ok = db.(*VersionedDB).GetCachedVersion("ns2", "key2")
	require.Nil(t, ver)
	require.False(t, ok)

	keys := []*statedb.CompositeKey{
		{
			Namespace: "ns1",
			Key:       "key1",
		},
		{
			Namespace: "ns1",
			Key:       "key2",
		},
		{
			Namespace: "ns2",
			Key:       "key1",
		},
		{
			Namespace: "ns2",
			Key:       "key2",
		},
	}

	require.NoError(t, db.(*VersionedDB).LoadCommittedVersions(keys))

	ver, ok = db.(*VersionedDB).GetCachedVersion("ns1", "key1")
	require.Equal(t, version.NewHeight(1, 1), ver)
	require.True(t, ok)
	ver, ok = db.(*VersionedDB).GetCachedVersion("ns1", "key2")
	require.Equal(t, version.NewHeight(1, 2), ver)
	require.True(t, ok)
	ver, ok = db.(*VersionedDB).GetCachedVersion("ns2", "key1")
	require.Equal(t, version.NewHeight(1, 3), ver)
	require.True(t, ok)
	ver, ok = db.(*VersionedDB).GetCachedVersion("ns2", "key2")
	require.Equal(t, version.NewHeight(1, 4), ver)
	require.True(t, ok)
}

func TestMissingRevisionRetrievalFromDB(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	chainID := "testmissingrevisionfromdb"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// store key1, key2, key3 to the DB
	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	batch.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch.Put("ns1", "key2", vv2.Value, vv2.Version)
	batch.Put("ns1", "key3", vv3.Value, vv3.Version)
	savePoint := version.NewHeight(2, 5)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// retrieve the versions of key1, key2, and key3
	revisions := make(map[string]string)
	require.NoError(t, db.(*VersionedDB).addMissingRevisionsFromDB("ns1", []string{"key1", "key2", "key3"}, revisions))
	require.Equal(t, 3, len(revisions))

	// update key1 and key2 but not key3
	batch = statedb.NewUpdateBatch()
	vv4 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv5 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	batch.Put("ns1", "key1", vv4.Value, vv4.Version)
	batch.Put("ns1", "key2", vv5.Value, vv5.Version)
	savePoint = version.NewHeight(3, 5)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// for key3, the revision should be the same but not for key1 and key2
	newRevisions := make(map[string]string)
	require.NoError(t, db.(*VersionedDB).addMissingRevisionsFromDB("ns1", []string{"key1", "key2", "key3"}, newRevisions))
	require.Equal(t, 3, len(newRevisions))
	require.NotEqual(t, revisions["key1"], newRevisions["key1"])
	require.NotEqual(t, revisions["key2"], newRevisions["key2"])
	require.Equal(t, revisions["key3"], newRevisions["key3"])
}

func TestMissingRevisionRetrievalFromCache(t *testing.T) {
	vdbEnv.init(t, []string{"lscc", "_lifecycle"})
	defer vdbEnv.cleanup()

	chainID := "testmissingrevisionfromcache"
	db, err := vdbEnv.DBProvider.GetDBHandle(chainID, nil)
	require.NoError(t, err)

	// scenario 1: missing from cache.
	revisions := make(map[string]string)
	stillMissingKeys, err := db.(*VersionedDB).addMissingRevisionsFromCache("ns1", []string{"key1", "key2"}, revisions)
	require.NoError(t, err)
	require.Equal(t, []string{"key1", "key2"}, stillMissingKeys)
	require.Empty(t, revisions)

	// scenario 2: key1 is available in the cache
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns1", "key1", &CacheValue{AdditionalInfo: []byte("rev1")}))
	revisions = make(map[string]string)
	stillMissingKeys, err = db.(*VersionedDB).addMissingRevisionsFromCache("ns1", []string{"key1", "key2"}, revisions)
	require.NoError(t, err)
	require.Equal(t, []string{"key2"}, stillMissingKeys)
	require.Equal(t, "rev1", revisions["key1"])

	// scenario 3: both key1 and key2 are available in the cache
	require.NoError(t, vdbEnv.cache.putState(chainID, "ns1", "key2", &CacheValue{AdditionalInfo: []byte("rev2")}))
	revisions = make(map[string]string)
	stillMissingKeys, err = db.(*VersionedDB).addMissingRevisionsFromCache("ns1", []string{"key1", "key2"}, revisions)
	require.NoError(t, err)
	require.Empty(t, stillMissingKeys)
	require.Equal(t, "rev1", revisions["key1"])
	require.Equal(t, "rev2", revisions["key2"])
}

func TestChannelMetadata(t *testing.T) {
	vdbEnv.init(t, sysNamespaces)
	defer vdbEnv.cleanup()
	channelName := "testchannelmetadata"

	db, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	vdb := db.(*VersionedDB)
	expectedChannelMetadata := &channelMetadata{
		ChannelName:      channelName,
		NamespaceDBsInfo: make(map[string]*namespaceDBInfo),
	}
	savedChannelMetadata, err := vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)

	// call getNamespaceDBHandle for new dbs, verify that new db names are added to dbMetadataMapping
	namepsaces := make([]string, 10)
	for i := 0; i < 10; i++ {
		ns := fmt.Sprintf("nsname_%d", i)
		_, err := vdb.getNamespaceDBHandle(ns)
		require.NoError(t, err)
		namepsaces[i] = ns
		expectedChannelMetadata.NamespaceDBsInfo[ns] = &namespaceDBInfo{
			Namespace: ns,
			DBName:    constructNamespaceDBName(channelName, ns),
		}
	}

	savedChannelMetadata, err = vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)

	// call getNamespaceDBHandle for existing dbs, verify that no new db names are added to dbMetadataMapping
	for _, ns := range namepsaces {
		_, err := vdb.getNamespaceDBHandle(ns)
		require.NoError(t, err)
	}

	savedChannelMetadata, err = vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)
}

func TestChannelMetadata_NegativeTests(t *testing.T) {
	vdbEnv.init(t, sysNamespaces)
	defer vdbEnv.cleanup()

	channelName := "testchannelmetadata-errorpropagation"
	origCouchAddress := vdbEnv.config.Address
	vdbEnv.config.MaxRetries = 1
	vdbEnv.config.MaxRetriesOnStartup = 1
	vdbEnv.config.RequestTimeout = 1 * time.Second

	// simulate db connection error by setting an invalid address before GetDBHandle, verify error is propagated
	vdbEnv.config.Address = "127.0.0.1:1"
	expectedErrMsg := fmt.Sprintf("http error calling couchdb: Get \"http://%s/testchannelmetadata-errorpropagation_\": dial tcp %s: connect: connection refused",
		vdbEnv.config.Address, vdbEnv.config.Address)
	_, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.EqualError(t, err, expectedErrMsg)
	vdbEnv.config.Address = origCouchAddress

	// simulate db connection error by setting an invalid address before getNamespaceDBHandle, verify error is propagated
	db, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	vdb := db.(*VersionedDB)
	vdbEnv.config.Address = "127.0.0.1:1"
	expectedErrMsg = fmt.Sprintf("http error calling couchdb: Put \"http://%s/testchannelmetadata-errorpropagation_/channel_metadata\": dial tcp %s: connect: connection refused",
		vdbEnv.config.Address, vdbEnv.config.Address)
	_, err = vdb.getNamespaceDBHandle("testnamepsace1")
	require.EqualError(t, err, expectedErrMsg)
	vdb.couchInstance.conf.Address = origCouchAddress

	// call createCouchDatabase to simulate peer crashes after metadataDB is created but before channelMetadata is updated
	// then call DBProvider.GetDBHandle and verify channelMetadata is correctly generated
	channelName = "testchannelmetadata-simulatefailure-in-between"
	couchInstance, err := createCouchInstance(vdbEnv.config, &disabled.Provider{})
	require.NoError(t, err)
	metadatadbName := constructMetadataDBName(channelName)
	metadataDB, err := createCouchDatabase(couchInstance, metadatadbName)
	require.NoError(t, err)
	vdb = &VersionedDB{
		metadataDB: metadataDB,
	}
	savedChannelMetadata, err := vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Nil(t, savedChannelMetadata)

	db, err = vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	vdb = db.(*VersionedDB)
	expectedChannelMetadata := &channelMetadata{
		ChannelName:      channelName,
		NamespaceDBsInfo: make(map[string]*namespaceDBInfo),
	}
	savedChannelMetadata, err = vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)

	// call writeChannelMetadata to simulate peer crashes after channelMetada is saved but before namespace DB is created
	// then call vdb.getNamespaceDBHandle and verify namespaceDB is created and channelMetadata is correct
	namespace := "testnamepsace2"
	namespaceDBName := constructNamespaceDBName(channelName, namespace)
	vdb.channelMetadata.NamespaceDBsInfo[namespace] = &namespaceDBInfo{Namespace: namespace, DBName: namespaceDBName}
	err = vdb.writeChannelMetadata()
	require.NoError(t, err)
	expectedChannelMetadata.NamespaceDBsInfo = map[string]*namespaceDBInfo{
		namespace: {Namespace: namespace, DBName: namespaceDBName},
	}
	savedChannelMetadata, err = vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)

	_, err = vdb.getNamespaceDBHandle(namespace)
	require.NoError(t, err)
	savedChannelMetadata, err = vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)
}

func TestInitChannelMetadta(t *testing.T) {
	vdbEnv.init(t, sysNamespaces)
	defer vdbEnv.cleanup()
	channelName1 := "testinithannelmetadata"
	channelName2 := "testinithannelmetadata_anotherchannel"

	// create versioned DBs for channelName1 and channelName2
	db, err := vdbEnv.DBProvider.GetDBHandle(channelName1, nil)
	require.NoError(t, err)
	vdb := db.(*VersionedDB)
	db2, err := vdbEnv.DBProvider.GetDBHandle(channelName2, nil)
	require.NoError(t, err)
	vdb2 := db2.(*VersionedDB)

	// prepare test data:
	// create dbs for channelName1: "ns1" and "ns3", which should match channelName1 namespaces
	// create dbs for channelName2: "ns2" and "ns4", which should not match any channelName1 namespaces
	_, err = vdb.getNamespaceDBHandle("ns1")
	require.NoError(t, err)
	_, err = vdb.getNamespaceDBHandle("ns3")
	require.NoError(t, err)
	_, err = vdb2.getNamespaceDBHandle("ns2")
	require.NoError(t, err)
	_, err = vdb2.getNamespaceDBHandle("ns4")
	require.NoError(t, err)

	namespaces := []string{"ns1", "ns2", "ns3", "ns4"}
	fakeNsProvider := &mock.NamespaceProvider{}
	fakeNsProvider.PossibleNamespacesReturns(namespaces, nil)
	expectedDBsInfo := map[string]*namespaceDBInfo{
		"ns1": {Namespace: "ns1", DBName: constructNamespaceDBName(channelName1, "ns1")},
		"ns3": {Namespace: "ns3", DBName: constructNamespaceDBName(channelName1, "ns3")},
	}
	expectedChannelMetadata := &channelMetadata{
		ChannelName:      channelName1,
		NamespaceDBsInfo: expectedDBsInfo,
	}

	// test an existing DB with channelMetadata, namespace provider should not be called
	require.NoError(t, vdb.initChannelMetadata(false, fakeNsProvider))
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)
	require.Equal(t, 0, fakeNsProvider.PossibleNamespacesCallCount())

	// test an existing DB with no channelMetadata by deleting channelMetadata, namespace provider should be called
	require.NoError(t, vdb.metadataDB.deleteDoc(channelMetadataDocID, ""))
	require.NoError(t, vdb.initChannelMetadata(false, fakeNsProvider))
	require.Equal(t, expectedChannelMetadata, vdb.channelMetadata)
	require.Equal(t, 1, fakeNsProvider.PossibleNamespacesCallCount())
	savedChannelMetadata, err := vdb.readChannelMetadata()
	require.NoError(t, err)
	require.Equal(t, expectedChannelMetadata, savedChannelMetadata)

	// test namespaceProvider error
	fakeNsProvider.PossibleNamespacesReturns(nil, errors.New("fake-namespaceprivder-error"))
	require.NoError(t, vdb.metadataDB.deleteDoc(channelMetadataDocID, ""))
	err = vdb.initChannelMetadata(false, fakeNsProvider)
	require.EqualError(t, err, "fake-namespaceprivder-error")

	// test db error
	origCouchAddress := vdbEnv.config.Address
	vdbEnv.config.Address = "127.0.0.1:1"
	vdbEnv.config.MaxRetries = 1
	vdbEnv.config.MaxRetriesOnStartup = 1
	expectedErrMsg := fmt.Sprintf("http error calling couchdb: Get \"http://%s/testinithannelmetadata_/channel_metadata?attachments=true\": dial tcp %s: connect: connection refused",
		vdbEnv.config.Address, vdbEnv.config.Address)
	vdb.channelMetadata = nil
	err = vdb.initChannelMetadata(false, fakeNsProvider)
	require.EqualError(t, err, expectedErrMsg)
	vdbEnv.config.Address = origCouchAddress
}

func TestRangeQueryWithInternalLimitAndPageSize(t *testing.T) {
	// generateSampleData returns a slice of KVs. The returned value contains 12 KVs for a namespace ns1
	generateSampleData := func() []*statedb.VersionedKV {
		sampleData := []*statedb.VersionedKV{}
		ver := version.NewHeight(1, 1)
		sampleKV := &statedb.VersionedKV{
			CompositeKey:   &statedb.CompositeKey{Namespace: "ns1", Key: string('\u0000')},
			VersionedValue: &statedb.VersionedValue{Value: []byte("v0"), Version: ver, Metadata: []byte("m0")},
		}
		sampleData = append(sampleData, sampleKV)
		for i := 0; i < 10; i++ {
			sampleKV = &statedb.VersionedKV{
				CompositeKey: &statedb.CompositeKey{
					Namespace: "ns1",
					Key:       fmt.Sprintf("key-%d", i),
				},
				VersionedValue: &statedb.VersionedValue{
					Value:    []byte(fmt.Sprintf("value-for-key-%d-for-ns1", i)),
					Version:  ver,
					Metadata: []byte(fmt.Sprintf("metadata-for-key-%d-for-ns1", i)),
				},
			}
			sampleData = append(sampleData, sampleKV)
		}
		sampleKV = &statedb.VersionedKV{
			CompositeKey: &statedb.CompositeKey{
				Namespace: "ns1",
				Key:       string(utf8.MaxRune),
			},
			VersionedValue: &statedb.VersionedValue{
				Value:    []byte("v1"),
				Version:  ver,
				Metadata: []byte("m1"),
			},
		}
		sampleData = append(sampleData, sampleKV)
		return sampleData
	}

	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	channelName := "ch1"
	vdb, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	db := vdb.(*VersionedDB)

	sampleData := generateSampleData()
	batch := statedb.NewUpdateBatch()
	for _, d := range sampleData {
		batch.PutValAndMetadata(d.Namespace, d.Key, d.Value, d.Metadata, d.Version)
	}
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 1)))

	defaultLimit := vdbEnv.config.InternalQueryLimit

	// Scenario 1: We try to fetch either 11 records or all 12 records. We pass various internalQueryLimits.
	// key utf8.MaxRune would not be included as inclusive_end is always set to false
	testRangeQueryWithInternalLimit(t, "ns1", db, 2, string('\u0000'), string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithInternalLimit(t, "ns1", db, 5, string('\u0000'), string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithInternalLimit(t, "ns1", db, 2, string('\u0000'), "", sampleData)
	testRangeQueryWithInternalLimit(t, "ns1", db, 5, string('\u0000'), "", sampleData)
	testRangeQueryWithInternalLimit(t, "ns1", db, 2, "", string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithInternalLimit(t, "ns1", db, 5, "", string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithInternalLimit(t, "ns1", db, 2, "", "", sampleData)
	testRangeQueryWithInternalLimit(t, "ns1", db, 5, "", "", sampleData)

	// Scenario 2: We try to fetch either 11 records or all 12 records using pagination. We pass various page sizes while
	// keeping the internalQueryLimit as the default one, i.e., 1000.
	vdbEnv.config.InternalQueryLimit = defaultLimit
	testRangeQueryWithPageSize(t, "ns1", db, 2, string('\u0000'), string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 15, string('\u0000'), string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 2, string('\u0000'), "", sampleData)
	testRangeQueryWithPageSize(t, "ns1", db, 15, string('\u0000'), "", sampleData)
	testRangeQueryWithPageSize(t, "ns1", db, 2, "", string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 15, "", string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 2, "", "", sampleData)
	testRangeQueryWithPageSize(t, "ns1", db, 15, "", "", sampleData)

	// Scenario 3: We try to fetch either 11 records or all 12 records using pagination. We pass various page sizes while
	// keeping the internalQueryLimit to 1.
	vdbEnv.config.InternalQueryLimit = 1
	testRangeQueryWithPageSize(t, "ns1", db, 2, string('\u0000'), string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 15, string('\u0000'), string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 2, string('\u0000'), "", sampleData)
	testRangeQueryWithPageSize(t, "ns1", db, 15, string('\u0000'), "", sampleData)
	testRangeQueryWithPageSize(t, "ns1", db, 2, "", string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 15, "", string(utf8.MaxRune), sampleData[:len(sampleData)-1])
	testRangeQueryWithPageSize(t, "ns1", db, 2, "", "", sampleData)
	testRangeQueryWithPageSize(t, "ns1", db, 15, "", "", sampleData)
}

func testRangeQueryWithInternalLimit(
	t *testing.T,
	ns string,
	db *VersionedDB,
	limit int,
	startKey, endKey string,
	expectedResults []*statedb.VersionedKV,
) {
	vdbEnv.config.InternalQueryLimit = limit
	require.Equal(t, int32(limit), db.couchInstance.internalQueryLimit())
	itr, err := db.GetStateRangeScanIterator(ns, startKey, endKey)
	require.NoError(t, err)
	require.Equal(t, int32(limit), itr.(*queryScanner).queryDefinition.internalQueryLimit)
	results := []*statedb.VersionedKV{}
	for {
		result, err := itr.Next()
		require.NoError(t, err)
		if result == nil {
			itr.Close()
			break
		}
		results = append(results, result)
	}
	require.Equal(t, expectedResults, results)
}

func testRangeQueryWithPageSize(
	t *testing.T,
	ns string,
	db *VersionedDB,
	pageSize int,
	startKey, endKey string,
	expectedResults []*statedb.VersionedKV,
) {
	itr, err := db.GetStateRangeScanIteratorWithPagination(ns, startKey, endKey, int32(pageSize))
	require.NoError(t, err)
	results := []*statedb.VersionedKV{}
	for {
		result, err := itr.Next()
		require.NoError(t, err)
		if result != nil {
			results = append(results, result)
			continue
		}
		nextStartKey := itr.GetBookmarkAndClose()
		if nextStartKey == endKey {
			break
		}
		itr, err = db.GetStateRangeScanIteratorWithPagination(ns, nextStartKey, endKey, int32(pageSize))
		require.NoError(t, err)
		continue
	}
	require.Equal(t, expectedResults, results)
}

func TestDataExportImport(t *testing.T) {
	t.Run("export-import", func(t *testing.T) {
		vdbEnv.init(t, nil)
		defer vdbEnv.cleanup()

		for _, size := range []int{10, 2 * 1024 * 1024} {
			maxDataImportBatchMemorySize = size
			commontests.TestDataExportImport(t, vdbEnv.DBProvider)
		}
	})

	t.Run("nil-iterator", func(t *testing.T) {
		vdbEnv.init(t, nil)
		defer vdbEnv.cleanup()

		require.NoError(
			t,
			vdbEnv.DBProvider.ImportFromSnapshot("testdb", nil, nil),
		)
	})

	t.Run("error-couchdb-connection", func(t *testing.T) {
		vdbEnv.init(t, nil)
		configBackup := *vdbEnv.config
		defer func() {
			vdbEnv.config = &configBackup
			vdbEnv.cleanup()
		}()

		vdbEnv.config.MaxRetries = 1
		vdbEnv.config.MaxRetriesOnStartup = 1
		vdbEnv.config.RequestTimeout = 1 * time.Second
		vdbEnv.config.Address = "127.0.0.1:1"
		require.Contains(
			t,
			vdbEnv.DBProvider.ImportFromSnapshot("testdb", nil, nil).Error(),
			"error while creating the metadata database for channel testdb: http error calling couchdb",
		)
	})

	t.Run("error-reading-from-iter", func(t *testing.T) {
		vdbEnv.init(t, nil)
		defer vdbEnv.cleanup()

		itr := &dummyFullScanIter{
			err: errors.New("error while reading from source"),
		}
		require.EqualError(
			t,
			vdbEnv.DBProvider.ImportFromSnapshot("testdb", nil, itr),
			"error while reading from source",
		)
	})

	t.Run("error-creating-database", func(t *testing.T) {
		vdbEnv.init(t, nil)
		configBackup := *vdbEnv.config
		defer func() {
			vdbEnv.config = &configBackup
			vdbEnv.cleanup()
		}()

		vdb, err := vdbEnv.DBProvider.GetDBHandle("testdb", nil)
		require.NoError(t, err)
		nsDB, err := vdb.(*VersionedDB).getNamespaceDBHandle("ns")
		require.NoError(t, err)

		vdbEnv.config.MaxRetries = 1
		vdbEnv.config.MaxRetriesOnStartup = 1
		vdbEnv.config.RequestTimeout = 1 * time.Second
		vdbEnv.config.Address = "127.0.0.1:1"

		// creating database for the first encountered namespace
		s := &snapshotImporter{
			vdb: vdb.(*VersionedDB),
			itr: &dummyFullScanIter{
				kv: &statedb.VersionedKV{
					CompositeKey: &statedb.CompositeKey{
						Namespace: "ns1",
					},
				},
			},
		}
		require.Contains(
			t,
			s.importState().Error(),
			"error while creating database for the namespace ns1",
		)

		// creating database as next namespace
		// does not match the current namespace
		s = &snapshotImporter{
			vdb: vdb.(*VersionedDB),
			itr: &dummyFullScanIter{
				kv: &statedb.VersionedKV{
					CompositeKey: &statedb.CompositeKey{
						Namespace: "ns2",
					},
				},
			},
			currentNsDB: nsDB,
			currentNs:   "ns",
		}
		require.Contains(
			t,
			s.importState().Error(),
			"error while creating database for the namespace ns2",
		)
	})

	t.Run("error-while-storing", func(t *testing.T) {
		vdbEnv.init(t, nil)
		configBackup := *vdbEnv.config
		defer func() {
			vdbEnv.config = &configBackup
			vdbEnv.cleanup()
		}()

		vdb, err := vdbEnv.DBProvider.GetDBHandle("testdb", nil)
		require.NoError(t, err)
		ns1DB, err := vdb.(*VersionedDB).getNamespaceDBHandle("ns1")
		require.NoError(t, err)

		vdbEnv.config.MaxRetries = 1
		vdbEnv.config.MaxRetriesOnStartup = 1
		vdbEnv.config.RequestTimeout = 1 * time.Second
		vdbEnv.config.Address = "127.0.0.1:1"

		// same namespace but the pending doc limit reached
		s := &snapshotImporter{
			vdb: vdb.(*VersionedDB),
			itr: &dummyFullScanIter{
				kv: &statedb.VersionedKV{
					CompositeKey: &statedb.CompositeKey{
						Namespace: "ns1",
					},
					VersionedValue: &statedb.VersionedValue{
						Value:   []byte("random"),
						Version: version.NewHeight(1, 1),
					},
				},
			},
			currentNsDB:      ns1DB,
			currentNs:        "ns1",
			pendingDocsBatch: []*couchDoc{{}, {}},
			batchMemorySize:  4 * 1024 * 1024,
		}
		require.Contains(
			t,
			s.importState().Error(),
			"error while storing 3 states associated with namespace ns1",
		)

		// next namespace does not match the current namespace
		s = &snapshotImporter{
			vdb: vdb.(*VersionedDB),
			itr: &dummyFullScanIter{
				kv: &statedb.VersionedKV{
					CompositeKey: &statedb.CompositeKey{
						Namespace: "ns2",
					},
					VersionedValue: &statedb.VersionedValue{
						Value:   []byte("random"),
						Version: version.NewHeight(1, 1),
					},
				},
			},
			currentNsDB:      ns1DB,
			currentNs:        "ns1",
			pendingDocsBatch: []*couchDoc{{}, {}},
		}
		require.Contains(
			t,
			s.importState().Error(),
			"error while storing 2 states associated with namespace ns1",
		)
	})
}

func TestFullScanIteratorDeterministicJSONOutput(t *testing.T) {
	generateSampleData := func(ns string, sortedJSON bool) []*statedb.VersionedKV {
		sampleData := []*statedb.VersionedKV{}
		ver := version.NewHeight(1, 1)
		for i := 0; i < 10; i++ {
			sampleKV := &statedb.VersionedKV{
				CompositeKey: &statedb.CompositeKey{
					Namespace: ns,
					Key:       fmt.Sprintf("key-%d", i),
				},
				VersionedValue: &statedb.VersionedValue{
					Version:  ver,
					Metadata: []byte(fmt.Sprintf("metadata-for-key-%d-for-ns1", i)),
				},
			}
			if sortedJSON {
				sampleKV.Value = []byte(fmt.Sprintf(`{"a":0,"b":0,"c":%d}`, i))
			} else {
				sampleKV.Value = []byte(fmt.Sprintf(`{"c":%d,"b":0,"a":0}`, i))
			}
			sampleData = append(sampleData, sampleKV)
		}
		return sampleData
	}

	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	channelName := "ch1"
	vdb, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	db := vdb.(*VersionedDB)

	// creating and storing JSON value with sorted keys
	sampleDataWithSortedJSON := generateSampleData("ns1", true)
	batch := statedb.NewUpdateBatch()
	for _, d := range sampleDataWithSortedJSON {
		batch.PutValAndMetadata(d.Namespace, d.Key, d.Value, d.Metadata, d.Version)
	}
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 1)))

	retrieveOnlyNs1 := func(ns string) bool {
		return ns != "ns1"
	}
	dbItr, err := db.GetFullScanIterator(retrieveOnlyNs1)
	require.NoError(t, err)
	require.NotNil(t, dbItr)
	verifyFullScanIterator(t, dbItr, sampleDataWithSortedJSON)

	// creating and storing JSON value with unsorted JSON-keys
	sampleDataWithUnsortedJSON := generateSampleData("ns2", false)
	batch = statedb.NewUpdateBatch()
	for _, d := range sampleDataWithUnsortedJSON {
		batch.PutValAndMetadata(d.Namespace, d.Key, d.Value, d.Metadata, d.Version)
	}
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 1)))

	retrieveOnlyNs2 := func(ns string) bool {
		return ns != "ns2"
	}
	sampleDataWithSortedJSON = generateSampleData("ns2", true)
	dbItr, err = db.GetFullScanIterator(retrieveOnlyNs2)
	require.NoError(t, err)
	require.NotNil(t, dbItr)
	verifyFullScanIterator(t, dbItr, sampleDataWithSortedJSON)
}

func TestFullScanIteratorSkipInternalKeys(t *testing.T) {
	generateSampleData := func(ns string, keys []string) []*statedb.VersionedKV {
		sampleData := []*statedb.VersionedKV{}
		ver := version.NewHeight(1, 1)
		for i := 0; i < len(keys); i++ {
			sampleKV := &statedb.VersionedKV{
				CompositeKey: &statedb.CompositeKey{
					Namespace: ns,
					Key:       keys[i],
				},
				VersionedValue: &statedb.VersionedValue{
					Value:    []byte(fmt.Sprintf("value-for-%s-for-ns1", keys[i])),
					Version:  ver,
					Metadata: []byte(fmt.Sprintf("metadata-for-%s-for-ns1", keys[i])),
				},
			}
			sampleData = append(sampleData, sampleKV)
		}
		return sampleData
	}

	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	channelName := "ch1"
	vdb, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	db := vdb.(*VersionedDB)

	keys := []string{channelMetadataDocID, "key-1", "key-2", "key-3", "key-4", "key-5", savepointDocID}
	sampleData := generateSampleData("ns1", keys)
	batch := statedb.NewUpdateBatch()
	for _, d := range sampleData {
		batch.PutValAndMetadata(d.Namespace, d.Key, d.Value, d.Metadata, d.Version)
	}
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 1)))

	retrieveOnlyNs1 := func(ns string) bool {
		return ns != "ns1"
	}
	dbItr, err := db.GetFullScanIterator(retrieveOnlyNs1)
	require.NoError(t, err)
	require.NotNil(t, dbItr)
	verifyFullScanIterator(t, dbItr, sampleData)

	sampleData = generateSampleData("", keys)
	batch = statedb.NewUpdateBatch()
	for _, d := range sampleData {
		batch.PutValAndMetadata(d.Namespace, d.Key, d.Value, d.Metadata, d.Version)
	}
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 1)))

	retrieveOnlyEmptyNs := func(ns string) bool {
		return ns != ""
	}
	// remove internal keys such as savepointDocID and channelMetadataDocID
	// as it is an empty namespace
	keys = []string{"key-1", "key-2", "key-3", "key-4", "key-5"}
	sampleData = generateSampleData("", keys)
	dbItr, err = db.GetFullScanIterator(retrieveOnlyEmptyNs)
	require.NoError(t, err)
	require.NotNil(t, dbItr)
	verifyFullScanIterator(t, dbItr, sampleData)
}

func verifyFullScanIterator(
	t *testing.T,
	dbIter statedb.FullScanIterator,
	expectedResult []*statedb.VersionedKV,
) {
	results := []*statedb.VersionedKV{}
	for {
		kv, err := dbIter.Next()
		require.NoError(t, err)
		if kv == nil {
			break
		}
		require.NoError(t, err)
		results = append(results, kv)
	}
	require.Equal(t, expectedResult, results)
}

func TestDrop(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	checkDBsAfterDropFunc := func(channelName string) {
		appDBNames := RetrieveApplicationDBNames(t, vdbEnv.config)
		for _, dbName := range appDBNames {
			require.NotContains(t, dbName, channelName+"_")
		}
	}

	commontests.TestDrop(t, vdbEnv.DBProvider, checkDBsAfterDropFunc)
}

func TestDropErrorPath(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()
	channelName := "testdroperror"

	_, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)

	vdbEnv.config.MaxRetries = 1
	vdbEnv.config.MaxRetriesOnStartup = 1
	vdbEnv.config.RequestTimeout = 1 * time.Second
	origAddress := vdbEnv.config.Address
	vdbEnv.config.Address = "127.0.0.1:1"
	err = vdbEnv.DBProvider.Drop(channelName)
	require.Contains(t, err.Error(), "connection refused")
	vdbEnv.config.Address = origAddress

	vdbEnv.DBProvider.Close()
	require.EqualError(t, vdbEnv.DBProvider.Drop(channelName), "internal leveldb error while obtaining db iterator: leveldb: closed")
}

func TestReadFromDBInvalidKey(t *testing.T) {
	vdbEnv.init(t, sysNamespaces)
	defer vdbEnv.cleanup()
	channelName := "test_getstate_invalidkey"
	db, err := vdbEnv.DBProvider.GetDBHandle(channelName, nil)
	require.NoError(t, err)
	vdb := db.(*VersionedDB)

	testcase := []struct {
		key              string
		expectedErrorMsg string
	}{
		{
			key:              string([]byte{0xff, 0xfe, 0xfd}),
			expectedErrorMsg: "invalid key [fffefd], must be a UTF-8 string",
		},
		{
			key:              "",
			expectedErrorMsg: "invalid key. Empty string is not supported as a key by couchdb",
		},
		{
			key:              "_key_starting_with_an_underscore",
			expectedErrorMsg: `invalid key [_key_starting_with_an_underscore], cannot begin with "_"`,
		},
	}

	for i, tc := range testcase {
		t.Run(fmt.Sprintf("testcase-%d", i), func(t *testing.T) {
			_, err = vdb.readFromDB("ns", tc.key)
			require.EqualError(t, err, tc.expectedErrorMsg)
		})
	}
}

type dummyFullScanIter struct {
	err error
	kv  *statedb.VersionedKV
}

func (d *dummyFullScanIter) Next() (*statedb.VersionedKV, error) {
	return d.kv, d.err
}

func (d *dummyFullScanIter) Close() {
}
