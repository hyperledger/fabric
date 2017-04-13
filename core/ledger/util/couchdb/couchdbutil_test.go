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
	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testcreatecouchdbconnectionanddb"
		cleanup(database)
		defer cleanup(database)
		//create a new connection
		couchInstance, err := CreateCouchInstance(connectURL, "", "")
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchInstance"))

		_, err = CreateCouchDatabase(*couchInstance, database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to CreateCouchDatabase"))
	}

}

func TestDatabaseMapping(t *testing.T) {

	//create a new instance and database object using a database name mixed case
	databaseName, err := mapAndValidateDatabaseName("testDB")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to map database name"))
	testutil.AssertEquals(t, databaseName, "testdb")

	//create a new instance and database object using a database name with numerics
	databaseName, err = mapAndValidateDatabaseName("test1234DB")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to map database name"))
	testutil.AssertEquals(t, databaseName, "test1234db")

	//create a new instance and database object using a database name with special characters
	databaseName, err = mapAndValidateDatabaseName("test1234_$(),+-/~!@#%^&*[]{}.")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to map database name"))
	testutil.AssertEquals(t, databaseName, "test1234_$(),+-/_____________")

	//create a new instance and database object using a database name with special characters
	databaseName, err = mapAndValidateDatabaseName("5test1234")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to map database name"))
	testutil.AssertEquals(t, databaseName, "db_5test1234")

	//create a new instance and database object using an empty string
	_, err = mapAndValidateDatabaseName("")
	testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for an invalid name"))

	_, err = mapAndValidateDatabaseName("A12345678901234567890123456789012345678901234" +
		"56789012345678901234567890123456789012345678901234567890123456789012345678901234567890" +
		"12345678901234567890123456789012345678901234567890123456789012345678901234567890123456" +
		"78901234567890123456789012345678901234567890")
	testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for an invalid name"))

}
