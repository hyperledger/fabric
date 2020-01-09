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

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/hyperledger/fabric/core/ledger/util/couchdbtest"
	"github.com/stretchr/testify/require"
)

// testVDBEnv provides a couch db backed versioned db for testing
type testVDBEnv struct {
	t              testing.TB
	couchAddress   string
	DBProvider     statedb.VersionedDBProvider
	config         *couchdb.Config
	cleanupCouchDB func()
}

func (env *testVDBEnv) init(t testing.TB, cache *statedb.Cache) {
	t.Logf("Initializing TestVDBEnv")
	redoPath, err := ioutil.TempDir("", "cvdbenv")
	if err != nil {
		t.Fatalf("Failed to create redo log directory: %s", err)
	}

	env.startExternalResource()

	config := &couchdb.Config{
		Address:             env.couchAddress,
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

	env.t = t
	env.DBProvider = dbProvider
	env.config = config
}

// startExternalResource sstarts external couchDB resources for testVDBEnv.
func (env *testVDBEnv) startExternalResource() {
	if env.couchAddress == "" {
		env.couchAddress, env.cleanupCouchDB = couchdbtest.CouchDBSetup(nil)
	}
}

// stopExternalResource stops external couchDB resources.
func (env *testVDBEnv) stopExternalResource() {
	if env.couchAddress != "" {
		env.cleanupCouchDB()
	}
}

func (env *testVDBEnv) closeAndReopen() {
	env.DBProvider.Close()
	dbProvider, _ := NewVersionedDBProvider(env.config, &disabled.Provider{}, &statedb.Cache{})
	env.DBProvider = dbProvider
}

// Cleanup drops the test couch databases and closes the db provider
func (env *testVDBEnv) cleanup() {
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
