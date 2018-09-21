/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
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
func (env *TestVDBEnv) Cleanup() {
	env.t.Logf("Cleaningup TestVDBEnv")
	CleanupDB(env.t, env.DBProvider)

	env.DBProvider.Close()
}

func CleanupDB(t testing.TB, dbProvider statedb.VersionedDBProvider) {
	couchdbProvider, _ := dbProvider.(*VersionedDBProvider)
	for _, v := range couchdbProvider.databases {
		if _, err := v.metadataDB.DropDatabase(); err != nil {
			assert.Failf(t, "DropDatabase %s fails. err: %v", v.metadataDB.DBName, err)
		}

		for _, db := range v.namespaceDBs {
			if _, err := db.DropDatabase(); err != nil {
				assert.Failf(t, "DropDatabase %s fails. err: %v", db.DBName, err)
			}
		}
	}
}
