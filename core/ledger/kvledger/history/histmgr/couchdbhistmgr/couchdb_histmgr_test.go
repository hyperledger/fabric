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
	"fmt"
	"testing"

	helper "github.com/hyperledger/fabric/core/ledger/kvledger/history/histmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

/*
Note that these test are only run if HistoryDB is explitily enabled
otherwise HistoryDB may not be installed and all the tests would fail
*/

// Note that the couchdb_test.go tests couchdb functions already.  This test just tests that a
// CouchDB history database is auto-created upon creating a new history manager
func TestHistoryDatabaseAutoCreate(t *testing.T) {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../../../peer")
	logger.Debugf("===HISTORYDB=== TestHistoryDatabaseAutoCreate IsCouchDBEnabled()value: %v , IsHistoryDBEnabled()value: %v\n",
		ledgerconfig.IsCouchDBEnabled(), ledgerconfig.IsHistoryDBEnabled())

	if ledgerconfig.IsHistoryDBEnabled() == true {

		env := newTestEnvCouchDB(t, "history-test")
		env.cleanupCouchDB()       //cleanup at the beginning to ensure the database doesn't exist already
		defer env.cleanupCouchDB() //and cleanup at the end

		logger.Debugf("===HISTORYDB=== env.CouchDBAddress: %v , env.CouchDatabaseName: %v env.CouchUsername: %v env.CouchPassword: %v\n",
			env.CouchDBAddress, env.CouchDatabaseName, env.CouchUsername, env.CouchPassword)

		histMgr := NewCouchDBHistMgr(
			env.CouchDBAddress,    //couchDB Address
			env.CouchDatabaseName, //couchDB db name
			env.CouchUsername,     //enter couchDB id
			env.CouchPassword)     //enter couchDB pw

		//NewCouchDBhistMgr should have automatically created the database, let's make sure it has been created
		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := histMgr.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, env.CouchDatabaseName)

		//Call NewCouchDBhistMgr again, this time the database will already exist from last time
		histMgr2 := NewCouchDBHistMgr(
			env.CouchDBAddress,    //couchDB Address
			env.CouchDatabaseName, //couchDB db name
			env.CouchUsername,     //enter couchDB id
			env.CouchPassword)     //enter couchDB pw

		//Retrieve the info for the database again, and make sure the name still matches
		dbResp2, _, errdb2 := histMgr2.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb2, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp2.DbName, env.CouchDatabaseName)

	}
}

func TestConstructCompositeKey(t *testing.T) {
	compositeKey := helper.ConstructCompositeKey("ns1", "key1", 1, 1)

	var compositeKeySep = []byte{0x00}
	var strKeySep = string(compositeKeySep)

	testutil.AssertEquals(t, compositeKey, "ns1"+strKeySep+"key1"+strKeySep+"1"+strKeySep+"1")
}

//TestSavepoint tests the recordSavepoint and GetBlockNumfromSavepoint methods for recording and reading a savepoint document
func TestSavepoint(t *testing.T) {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../../../peer")
	logger.Debugf("===HISTORYDB=== TestHistoryDatabaseAutoCreate IsCouchDBEnabled()value: %v , IsHistoryDBEnabled()value: %v\n",
		ledgerconfig.IsCouchDBEnabled(), ledgerconfig.IsHistoryDBEnabled())

	if ledgerconfig.IsHistoryDBEnabled() == true {

		env := newTestEnvCouchDB(t, "history-test")
		env.cleanupCouchDB()       //cleanup at the beginning to ensure the database doesn't exist already
		defer env.cleanupCouchDB() //and cleanup at the end

		logger.Debugf("===HISTORYDB=== env.couchDBAddress: %v , env.couchDatabaseName: %v env.couchUsername: %v env.couchPassword: %v\n",
			env.CouchDBAddress, env.CouchDatabaseName, env.CouchUsername, env.CouchPassword)

		histMgr := NewCouchDBHistMgr(
			env.CouchDBAddress,    //couchDB Address
			env.CouchDatabaseName, //couchDB db name
			env.CouchUsername,     //enter couchDB id
			env.CouchPassword)     //enter couchDB pw

		// read the savepoint
		blockNum, err := histMgr.GetBlockNumFromSavepoint()
		testutil.AssertEquals(t, blockNum, uint64(0))

		// record savepoint
		blockNo := uint64(5)
		err = histMgr.recordSavepoint(blockNo)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when saving recordpoint data"))

		// read the savepoint
		blockNum, err = histMgr.GetBlockNumFromSavepoint()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when saving recordpoint data"))
		testutil.AssertEquals(t, blockNo, blockNum)
	}
}

/*
//History Database commit and read is being tested with kv_ledger_test.go.
//This test will push some of the testing down into history itself
func TestHistoryDatabaseCommit(t *testing.T) {

	testutil.SetupCoreYAMLConfig("./../../../../../../peer")
	logger.Debugf("===HISTORYDB=== TestHistoryDatabaseAutoCreate IsCouchDBEnabled()value: %v , IsHistoryDBEnabled()value: %v\n",
		ledgerconfig.IsCouchDBEnabled(), ledgerconfig.IsHistoryDBEnabled())

	if ledgerconfig.IsHistoryDBEnabled() == true {
		//TODO Build the necessary infrastructure so that history can be tested without ledger

	}
}
*/
