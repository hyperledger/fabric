/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
)

var couchAddress string

// TestVDBEnv provides a couch db backed versioned db for testing
type TestVDBEnv struct {
	t          testing.TB
	DBProvider statedb.VersionedDBProvider
	config     *couchdb.Config
}

// NewTestVDBEnv instantiates and new couch db backed TestVDB
func NewTestVDBEnv(t testing.TB) *TestVDBEnv {
	return newTestVDBEnvWithCache(t, &statedb.Cache{})
}

func newTestVDBEnvWithCache(t testing.TB, cache *statedb.Cache) *TestVDBEnv {
	t.Logf("Creating new TestVDBEnv")
	redoPath, err := ioutil.TempDir("", "cvdbenv")
	if err != nil {
		t.Fatalf("Failed to create redo log directory: %s", err)
	}
	config := &couchdb.Config{
		Address:             couchAddress,
		Username:            "",
		Password:            "",
		InternalQueryLimit:  1000,
		MaxBatchUpdateSize:  1000,
		MaxRetries:          3,
		MaxRetriesOnStartup: 20,
		RequestTimeout:      35 * time.Second,
		RedoLogPath:         redoPath,
	}
	dbProvider, err := NewVersionedDBProvider(config, &disabled.Provider{}, cache)
	if err != nil {
		t.Fatalf("Error creating CouchDB Provider: %s", err)
	}
	testVDBEnv := &TestVDBEnv{
		t:          t,
		DBProvider: dbProvider,
		config:     config,
	}
	// No cleanup for new test environment.  Need to cleanup per test for each DB used in the test.
	return testVDBEnv
}

func (env *TestVDBEnv) CloseAndReopen() {
	env.DBProvider.Close()
	dbProvider, _ := NewVersionedDBProvider(env.config, &disabled.Provider{}, &statedb.Cache{})
	env.DBProvider = dbProvider
}

// Cleanup drops the test couch databases and closes the db provider
func (env *TestVDBEnv) Cleanup() {
	env.t.Logf("Cleaningup TestVDBEnv")
	cleanupDB(env.t, env.DBProvider.(*VersionedDBProvider).couchInstance)
	env.DBProvider.Close()
	os.RemoveAll(env.config.RedoLogPath)
}

// CleanupDB deletes all the databases other than fabric internal database
func CleanupDB(t testing.TB, dbProvider statedb.VersionedDBProvider) {
	cleanupDB(t, dbProvider.(*VersionedDBProvider).couchInstance)
}

func cleanupDB(t testing.TB, couchInstance *couchdb.CouchInstance) {
	dbNames, err := couchInstance.RetrieveApplicationDBNames()
	require.NoError(t, err)
	for _, dbName := range dbNames {
		if dbName != fabricInternalDBName {
			testutilDropDB(t, couchInstance, dbName)
		}
	}
}

func testutilDropDB(t testing.TB, couchInstance *couchdb.CouchInstance, dbName string) {
	db := &couchdb.CouchDatabase{
		CouchInstance: couchInstance,
		DBName:        dbName,
	}
	response, err := db.DropDatabase()
	require.NoError(t, err)
	require.True(t, response.Ok)
}
