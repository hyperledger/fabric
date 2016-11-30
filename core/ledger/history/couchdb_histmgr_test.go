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
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
)

//Complex setup to test the use of couch in ledger
type testEnvCouch struct {
	couchDBPath       string
	couchDBAddress    string
	couchDatabaseName string
	couchUsername     string
	couchPassword     string
}

func newTestEnvCouch(t testing.TB, dbPath string, dbName string) *testEnvCouch {

	couchDBDef := ledgerconfig.GetCouchDBDefinition()
	os.RemoveAll(dbPath)

	return &testEnvCouch{
		couchDBPath:       dbPath,
		couchDBAddress:    couchDBDef.URL,
		couchDatabaseName: dbName,
		couchUsername:     couchDBDef.Username,
		couchPassword:     couchDBDef.Password,
	}
}

func (env *testEnvCouch) cleanup() {
	os.RemoveAll(env.couchDBPath)
	//create a new connection
	couchDB, _ := couchdb.CreateConnectionDefinition(env.couchDBAddress, env.couchDatabaseName, env.couchUsername, env.couchPassword)
	//drop the test database if it already existed
	couchDB.DropDatabase()
}

// couchdb_test.go tests couchdb functions already.  This test just tests that a CouchDB history database is auto-created
// upon creating a new history transaction manager
func TestHistoryDatabaseAutoCreate(t *testing.T) {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../peer")

	//Only run the tests if CouchDB is explitily enabled in the code,
	//otherwise CouchDB may not be installed and all the tests would fail
	//TODO replace this with external config property rather than config within the code
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := newTestEnvCouch(t, "/tmp/tests/ledger/history", "history-test")
		env.cleanup()       //cleanup at the beginning to ensure the database doesn't exist already
		defer env.cleanup() //and cleanup at the end

		histMgr := NewCouchDBHistMgr(
			env.couchDBAddress,    //couchDB Address
			env.couchDatabaseName, //couchDB db name
			env.couchUsername,     //enter couchDB id
			env.couchPassword)     //enter couchDB pw

		//NewCouchDBhistMgr should have automatically created the database, let's make sure it has been created
		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := histMgr.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, env.couchDatabaseName)

		//Call NewCouchDBhistMgr again, this time the database will already exist from last time
		histMgr2 := NewCouchDBHistMgr(
			env.couchDBAddress,    //couchDB Address
			env.couchDatabaseName, //couchDB db name
			env.couchUsername,     //enter couchDB id
			env.couchPassword)     //enter couchDB pw

		//Retrieve the info for the database again, and make sure the name still matches
		dbResp2, _, errdb2 := histMgr2.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb2, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp2.DbName, env.couchDatabaseName)

	}

}
