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
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
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

	couchDBDef := ledgerconfig.GetCouchDBDefinition()

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

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../../peer")

	//Only run the tests if CouchDB is explitily enabled in the code,
	//otherwise CouchDB may not be installed and all the tests would fail
	//TODO replace this with external config property rather than config within the code
	if ledgerconfig.IsCouchDBEnabled() == true {

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
		dbResp2, _, errdb2 := txMgr2.couchDB.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb2, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp2.DbName, env.couchDatabaseName)

		txMgr2.Shutdown()

	}

}

//TestSavepoint tests the recordSavepoint and GetBlockNumfromSavepoint methods for recording and reading a savepoint document
func TestSavepoint(t *testing.T) {

	//Only run the tests if CouchDB is explitily enabled in the code,
	//otherwise CouchDB may not be installed and all the tests would fail
	//TODO replace this with external config property rather than config within the code
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := newTestEnv(t)

		txMgr := NewCouchDBTxMgr(env.conf,
			env.couchDBAddress,    //couchDB Address
			env.couchDatabaseName, //couchDB db name
			env.couchUsername,     //enter couchDB id
			env.couchPassword)     //enter couchDB pw

		// record savepoint
		txMgr.blockNum = 5
		err := txMgr.recordSavepoint()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when saving recordpoint data"))

		// read the savepoint
		blockNum, err := txMgr.GetBlockNumFromSavepoint()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when saving recordpoint data"))
		testutil.AssertEquals(t, txMgr.blockNum, blockNum)

		txMgr.Shutdown()
	}
}

func TestDatabaseQuery(t *testing.T) {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../../peer")

	//Only run the tests if CouchDB is explitily enabled in the code,
	//otherwise CouchDB may not be installed and all the tests would fail
	//TODO replace this with external config property rather than config within the code
	if ledgerconfig.IsCouchDBEnabled() == true {

		env := newTestEnv(t)
		//env.Cleanup() //cleanup at the beginning to ensure the database doesn't exist already
		//defer env.Cleanup() //and cleanup at the end

		txMgr := NewCouchDBTxMgr(env.conf,
			env.couchDBAddress,    //couchDB Address
			env.couchDatabaseName, //couchDB db name
			env.couchUsername,     //enter couchDB id
			env.couchPassword)     //enter couchDB pw

		type Asset struct {
			ID        string `json:"_id"`
			Rev       string `json:"_rev"`
			AssetName string `json:"asset_name"`
			Color     string `json:"color"`
			Size      string `json:"size"`
			Owner     string `json:"owner"`
		}

		s1, _ := txMgr.NewTxSimulator()

		s1.SetState("ns1", "key1", []byte("value1"))
		s1.SetState("ns1", "key2", []byte("value2"))
		s1.SetState("ns1", "key3", []byte("value3"))
		s1.SetState("ns1", "key4", []byte("value4"))
		s1.SetState("ns1", "key5", []byte("value5"))
		s1.SetState("ns1", "key6", []byte("value6"))
		s1.SetState("ns1", "key7", []byte("value7"))
		s1.SetState("ns1", "key8", []byte("value8"))

		s1.SetState("ns1", "key9", []byte(`{"asset_name":"marble1","color":"red","size":"25","owner":"jerry"}`))
		s1.SetState("ns1", "key10", []byte(`{"asset_name":"marble2","color":"blue","size":"10","owner":"bob"}`))
		s1.SetState("ns1", "key11", []byte(`{"asset_name":"marble3","color":"blue","size":"35","owner":"jerry"}`))
		s1.SetState("ns1", "key12", []byte(`{"asset_name":"marble4","color":"green","size":"15","owner":"bob"}`))
		s1.SetState("ns1", "key13", []byte(`{"asset_name":"marble5","color":"red","size":"35","owner":"jerry"}`))
		s1.SetState("ns1", "key14", []byte(`{"asset_name":"marble6","color":"blue","size":"25","owner":"bob"}`))

		s1.Done()

		// validate and commit RWset
		txRWSet := s1.(*CouchDBTxSimulator).getTxReadWriteSet()
		isValid, err := txMgr.validateTx(txRWSet)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error in validateTx(): %s", err))
		testutil.AssertSame(t, isValid, true)
		txMgr.addWriteSetToBatch(txRWSet, version.NewHeight(1, 1))
		err = txMgr.Commit()
		testutil.AssertNoError(t, err, fmt.Sprintf("Error while calling commit(): %s", err))

		queryExecuter, _ := txMgr.NewQueryExecutor()
		queryString := "{\"selector\":{\"owner\": {\"$eq\": \"bob\"}},\"limit\": 10,\"skip\": 0}"

		itr, _ := queryExecuter.ExecuteQuery(queryString)

		counter := 0
		for {
			queryRecord, _ := itr.Next()
			if queryRecord == nil {
				break
			}

			//Unmarshal the document to Asset structure
			assetResp := &Asset{}
			json.Unmarshal(queryRecord.(*ledger.QueryRecord).Record, &assetResp)

			//Verify the owner retrieved matches
			testutil.AssertEquals(t, assetResp.Owner, "bob")

			counter++

		}

		//Ensure the query returns 3 documents
		testutil.AssertEquals(t, counter, 3)

		txMgr.Shutdown()

	}

}
