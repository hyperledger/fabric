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

package couchdbtxmgmt

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/kvledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/couchdbtxmgmt/couchdb"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

type testEnv struct {
	conf              *Conf
	couchDBAddress    string
	couchDatabaseName string
	couchUsername     string
	couchPassword     string
}

func newTestEnv(t testing.TB) *testEnv {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../../peer")

	couchDBDef := kvledgerconfig.GetCouchDBDefinition()

	conf := &Conf{"/tmp/tests/ledger/kvledger/txmgmt/couchdbtxmgmt"}
	os.RemoveAll(conf.DBPath)
	return &testEnv{
		conf:              conf,
		couchDBAddress:    couchDBDef.URL,
		couchDatabaseName: "system_test",
		couchUsername:     couchDBDef.Username,
		couchPassword:     couchDBDef.Password,
	}
}

func (env *testEnv) Cleanup() {
	os.RemoveAll(env.conf.DBPath)

	//create a new connection
	couchDB, _ := couchdb.CreateConnectionDefinition(env.couchDBAddress, env.couchDatabaseName, env.couchUsername, env.couchPassword)

	//drop the test database if it already existed
	couchDB.DropDatabase()
}

// couchdb_test.go tests couchdb functions already.  This test just tests that a CouchDB state database is auto-created
// upon creating a new ledger transaction manager
func TestDatabaseAutoCreate(t *testing.T) {

	//Only run the tests if CouchDB is explitily enabled in the code,
	//otherwise CouchDB may not be installed and all the tests would fail
	//TODO replace this with external config property rather than config within the code
	if kvledgerconfig.IsCouchDBEnabled() == true {

		env := newTestEnv(t)
		env.Cleanup()       //cleanup at the beginning to ensure the database doesn't exist already
		defer env.Cleanup() //and cleanup at the end

		txMgr := NewCouchDBTxMgr(env.conf,
			env.couchDBAddress,    //couchDB Address
			env.couchDatabaseName, //couchDB db name
			env.couchUsername,     //enter couchDB id
			env.couchPassword)     //enter couchDB pw

		//NewCouchDBTxMgr should have automatically created the database, let's make sure it has been created
		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := txMgr.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, env.couchDatabaseName)

		txMgr.Shutdown()

		//Call NewCouchDBTxMgr again, this time the database will already exist from last time
		txMgr2 := NewCouchDBTxMgr(env.conf,
			env.couchDBAddress,    //couchDB Address
			env.couchDatabaseName, //couchDB db name
			env.couchUsername,     //enter couchDB id
			env.couchPassword)     //enter couchDB pw

		//Retrieve the info for the database again, and make sure the name still matches
		dbResp2, _, errdb2 := txMgr.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb2, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp2.DbName, env.couchDatabaseName)

		txMgr2.Shutdown()

	}

}
