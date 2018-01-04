/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"strings"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
)

// TestVDBEnv provides a couch db backed versioned db for testing
type TestVDBEnv struct {
	t          testing.TB
	DBProvider statedb.VersionedDBProvider
}

// NewTestVDBEnv instantiates and new couch db backed TestVDB
func NewTestVDBEnv(t testing.TB) *TestVDBEnv {
	t.Logf("Creating new TestVDBEnv")

	dbProvider, _ := NewVersionedDBProvider()
	testVDBEnv := &TestVDBEnv{t, dbProvider}
	// No cleanup for new test environment.  Need to cleanup per test for each DB used in the test.
	return testVDBEnv
}

// Cleanup drops the test couch databases and closes the db provider
func (env *TestVDBEnv) Cleanup(dbName string) {
	env.t.Logf("Cleaningup TestVDBEnv")
	env.DBProvider.Close()
	CleanupDB(dbName)
}

// CleanupDB drops the test couch databases
func CleanupDB(dbName string) {
	//create a new connection
	couchDBDef := couchdb.GetCouchDBDefinition()
	couchInstance, _ := couchdb.CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
	db := couchdb.CouchDatabase{CouchInstance: couchInstance, DBName: strings.ToLower(dbName)}
	//drop the test database
	db.DropDatabase()
}
