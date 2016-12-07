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

package history

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
)

//Complex setup to test the use of couch in ledger
type testEnvHistoryCouchDB struct {
	couchDBAddress    string
	couchDatabaseName string
	couchUsername     string
	couchPassword     string
}

func newTestEnvHistoryCouchDB(t testing.TB, dbName string) *testEnvHistoryCouchDB {

	couchDBDef := ledgerconfig.GetCouchDBDefinition()

	return &testEnvHistoryCouchDB{
		couchDBAddress:    couchDBDef.URL,
		couchDatabaseName: dbName,
		couchUsername:     couchDBDef.Username,
		couchPassword:     couchDBDef.Password,
	}
}

func (env *testEnvHistoryCouchDB) cleanup() {

	//create a new connection
	couchDB, err := couchdb.CreateConnectionDefinition(env.couchDBAddress, env.couchDatabaseName, env.couchUsername, env.couchPassword)
	if err == nil {
		//drop the test database if it already existed
		couchDB.DropDatabase()
	}
}
