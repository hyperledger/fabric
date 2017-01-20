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

package couchdbhistmgr

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
)

//TestEnvCouchDB Complex setup to test the use of couchDB in ledger
type testEnvCouchDB struct {
	CouchDBAddress    string
	CouchDatabaseName string
	CouchUsername     string
	CouchPassword     string
}

//CleanupCouchDB to clean up the test of couchDB in ledger
func (env *testEnvCouchDB) cleanupCouchDB() {
	//create a new connection
	couchInstance, err := couchdb.CreateCouchInstance(env.CouchDBAddress, env.CouchUsername, env.CouchPassword)
	couchDB, err := couchdb.CreateCouchDatabase(*couchInstance, env.CouchDatabaseName)
	if err == nil {
		//drop the test database if it already existed
		couchDB.DropDatabase()
	}
}

//NewTestEnvCouchDB to clean up the test of couchDB in ledger
func newTestEnvCouchDB(t testing.TB, dbName string) *testEnvCouchDB {

	couchDBDef := ledgerconfig.GetCouchDBDefinition()

	return &testEnvCouchDB{
		CouchDBAddress:    couchDBDef.URL,
		CouchDatabaseName: dbName,
		CouchUsername:     couchDBDef.Username,
		CouchPassword:     couchDBDef.Password,
	}
}
