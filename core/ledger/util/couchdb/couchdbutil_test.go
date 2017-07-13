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

package couchdb

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

//Unit test of couch db util functionality
func TestCreateCouchDBConnectionAndDB(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() {

		database := "testcreatecouchdbconnectionanddb"
		cleanup(database)
		defer cleanup(database)
		//create a new connection
		couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchInstance"))

		_, err = CreateCouchDatabase(*couchInstance, database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchDatabase"))
	}

}

//Unit test of couch db util functionality
func TestCreateCouchDBSystemDBs(t *testing.T) {
	if ledgerconfig.IsCouchDBEnabled() {

		database := "testcreatecouchdbsystemdb"
		cleanup(database)
		defer cleanup(database)

		//create a new connection
		couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)

		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchInstance"))

		err = CreateSystemDatabasesIfNotExist(*couchInstance)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create system databases"))

		db := CouchDatabase{CouchInstance: *couchInstance, DBName: "_users"}

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve _users database information"))
		testutil.AssertEquals(t, dbResp.DbName, "_users")

		db = CouchDatabase{CouchInstance: *couchInstance, DBName: "_replicator"}

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb = db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve _replicator database information"))
		testutil.AssertEquals(t, dbResp.DbName, "_replicator")

		db = CouchDatabase{CouchInstance: *couchInstance, DBName: "_global_changes"}

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb = db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve _global_changes database information"))
		testutil.AssertEquals(t, dbResp.DbName, "_global_changes")

	}

}
func TestDatabaseMapping(t *testing.T) {
	//create a new instance and database object using a database name mixed case
	_, err := mapAndValidateDatabaseName("testDB")
	testutil.AssertError(t, err, "Error expected because the name contains capital letters")

	//create a new instance and database object using a database name with special characters
	_, err = mapAndValidateDatabaseName("test1234_1")
	testutil.AssertError(t, err, "Error expected because the name contains illegal chars")

	//create a new instance and database object using a database name with special characters
	_, err = mapAndValidateDatabaseName("5test1234")
	testutil.AssertError(t, err, "Error expected because the name starts with a number")

	//create a new instance and database object using an empty string
	_, err = mapAndValidateDatabaseName("")
	testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for an invalid name"))

	_, err = mapAndValidateDatabaseName("a12345678901234567890123456789012345678901234" +
		"56789012345678901234567890123456789012345678901234567890123456789012345678901234567890" +
		"12345678901234567890123456789012345678901234567890123456789012345678901234567890123456" +
		"78901234567890123456789012345678901234567890")
	testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for an invalid name"))

	transformedName, err := mapAndValidateDatabaseName("test.my.db-1")
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, transformedName, "test_my_db-1")
}
