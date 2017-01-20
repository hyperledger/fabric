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

package statecouchdb

import (
	"strings"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
)

//Basic setup to test couch
var connectURL = "localhost:5984"
var badConnectURL = "localhost:5990"
var username = ""
var password = ""

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
	cleanupDB(strings.ToLower(dbName))
	env.DBProvider.Close()

}
func cleanupDB(dbName string) {
	//create a new connection
	couchInstance, _ := couchdb.CreateCouchInstance(connectURL, username, password)
	db, _ := couchdb.CreateCouchDatabase(*couchInstance, dbName)
	//drop the test database
	db.DropDatabase()
}
