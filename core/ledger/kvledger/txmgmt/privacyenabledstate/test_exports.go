/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"os"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestEnv - an interface that a test environment implements
type TestEnv interface {
	Init(t testing.TB)
	GetDBHandle(id string) DB
	GetName() string
	Cleanup()
}

// Tests will be run against each environment in this array
// For example, to skip CouchDB tests, remove &couchDBLockBasedEnv{}
//var testEnvs = []testEnv{&levelDBCommonStorageTestEnv{}, &couchDBCommonStorageTestEnv{}}
var testEnvs = []TestEnv{&LevelDBCommonStorageTestEnv{}, &CouchDBCommonStorageTestEnv{}}

///////////// LevelDB Environment //////////////

// LevelDBCommonStorageTestEnv implements TestEnv interface for leveldb based storage
type LevelDBCommonStorageTestEnv struct {
	t        testing.TB
	provider DBProvider
}

// Init implements corresponding function from interface TestEnv
func (env *LevelDBCommonStorageTestEnv) Init(t testing.TB) {
	viper.Set("ledger.state.stateDatabase", "")
	removeDBPath(t)
	dbProvider, err := NewCommonStorageDBProvider()
	assert.NoError(t, err)
	env.t = t
	env.provider = dbProvider
}

// GetDBHandle implements corresponding function from interface TestEnv
func (env *LevelDBCommonStorageTestEnv) GetDBHandle(id string) DB {
	db, err := env.provider.GetDBHandle(id)
	assert.NoError(env.t, err)
	return db
}

// GetName implements corresponding function from interface TestEnv
func (env *LevelDBCommonStorageTestEnv) GetName() string {
	return "levelDBCommonStorageTestEnv"
}

// Cleanup implements corresponding function from interface TestEnv
func (env *LevelDBCommonStorageTestEnv) Cleanup() {
	env.provider.Close()
	removeDBPath(env.t)
}

///////////// CouchDB Environment //////////////

// CouchDBCommonStorageTestEnv implements TestEnv interface for couchdb based storage
type CouchDBCommonStorageTestEnv struct {
	t         testing.TB
	provider  DBProvider
	openDbIds map[string]bool
}

// Init implements corresponding function from interface TestEnv
func (env *CouchDBCommonStorageTestEnv) Init(t testing.TB) {
	viper.Set("ledger.state.stateDatabase", "CouchDB")
	// both vagrant and CI have couchdb configured at host "couchdb"
	viper.Set("ledger.state.couchDBConfig.couchDBAddress", "couchdb:5984")
	// Replace with correct username/password such as
	// admin/admin if user security is enabled on couchdb.
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 10)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)
	dbProvider, err := NewCommonStorageDBProvider()
	assert.NoError(t, err)
	env.t = t
	env.provider = dbProvider
	env.openDbIds = make(map[string]bool)
}

// GetDBHandle implements corresponding function from interface TestEnv
func (env *CouchDBCommonStorageTestEnv) GetDBHandle(id string) DB {
	db, err := env.provider.GetDBHandle(id)
	assert.NoError(env.t, err)
	env.openDbIds[id] = true
	return db
}

// GetName implements corresponding function from interface TestEnv
func (env *CouchDBCommonStorageTestEnv) GetName() string {
	return "couchDBCommonStorageTestEnv"
}

// Cleanup implements corresponding function from interface TestEnv
func (env *CouchDBCommonStorageTestEnv) Cleanup() {
	for id := range env.openDbIds {
		statecouchdb.CleanupDB(id)
	}
	env.provider.Close()
}

func removeDBPath(t testing.TB) {
	dbPath := ledgerconfig.GetStateLevelDBPath()
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatalf("Err: %s", err)
		t.FailNow()
	}
}
