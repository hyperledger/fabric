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
	"encoding/json"
	"fmt"
	"testing"

	kvledgerconfig "github.com/hyperledger/fabric/core/ledger/kvledger/kvledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

type Asset struct {
	ID        string `json:"_id"`
	Rev       string `json:"_rev"`
	AssetName string `json:"asset_name"`
	Color     string `json:"color"`
	Size      string `json:"size"`
	Owner     string `json:"owner"`
}

var hostname = "localhost"
var port = 5984
var database = "testdb1"
var username = ""
var password = ""

var assetJSON = []byte(`{"asset_name":"marble1","color":"blue","size":"35","owner":"jerry"}`)

func TestDBConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition("localhost", 5984, "database", "", "")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

}

func TestDBBadConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition("^^^localhost", 5984, "database", "", "")
	testutil.AssertError(t, err, fmt.Sprintf("Did not receive error when trying to create database connection definition with a bad hostname"))

}

func TestDBCreateSaveWithoutRevision(t *testing.T) {

	if kvledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(hostname, port, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Save the test document
		_, saveerr := db.SaveDoc("2", assetJSON)
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

		//Attempt to save the document again without updating the revision.  this should fail
		_, saveerr = db.SaveDoc("2", assetJSON)
		testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have thown for a missing revision number"))

	}
}

func TestDBBadConnection(t *testing.T) {

	if kvledgerconfig.IsCouchDBEnabled() == true {

		//create a new connection
		db, err := CreateConnectionDefinition(hostname, port+5, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertError(t, errdb, fmt.Sprintf("Error should have been thrown while creating a database with an invalid connecion"))

		//Save the test document
		_, saveerr := db.SaveDoc("3", assetJSON)
		testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have been thrown while saving a document with an invalid connecion"))

		//Retrieve the updated test document
		_, _, geterr := db.ReadDoc("3")
		testutil.AssertError(t, geterr, fmt.Sprintf("Error should have been thrown while retrieving a document with an invalid connecion"))

	}
}

func TestDBCreateDatabaseAndPersist(t *testing.T) {

	if kvledgerconfig.IsCouchDBEnabled() == true {

		cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(hostname, port, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, database)

		//Save the test document
		_, saveerr := db.SaveDoc("1", assetJSON)
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

		//Retrieve the test document
		dbGetResp, _, geterr := db.ReadDoc("1")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

		//Unmarshal the document to Asset structure
		assetResp := &Asset{}
		json.Unmarshal(dbGetResp, &assetResp)

		//Verify the owner retrieved matches
		testutil.AssertEquals(t, assetResp.Owner, "jerry")

		//Change owner to bob
		assetResp.Owner = "bob"

		//create a byte array of the JSON
		assetDocUpdated, _ := json.Marshal(assetResp)

		//Save the updated test document
		_, saveerr = db.SaveDoc("1", assetDocUpdated)
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save the updated document"))

		//Retrieve the updated test document
		dbGetResp, _, geterr = db.ReadDoc("1")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

		//Unmarshal the document to Asset structure
		assetResp = &Asset{}
		json.Unmarshal(dbGetResp, &assetResp)

		//Assert that the update was saved and retrieved
		testutil.AssertEquals(t, assetResp.Owner, "bob")

		//Drop the database
		_, errdbdrop := db.DropDatabase()
		testutil.AssertNoError(t, errdbdrop, fmt.Sprintf("Error dropping database"))

		//Retrieve the info for the new database and make sure the name matches
		_, _, errdbinfo := db.GetDatabaseInfo()
		testutil.AssertError(t, errdbinfo, fmt.Sprintf("Error should have been thrown for missing database"))

	}

}

func TestDBRetrieveNonExistingDocument(t *testing.T) {

	if kvledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(hostname, port, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when attempting to create a database"))

		//Attempt to retrieve the updated test document
		_, _, geterr2 := db.ReadDoc("2")
		testutil.AssertError(t, geterr2, fmt.Sprintf("Error should have been thrown while attempting to retrieve a nonexisting document"))

	}
}

func cleanup() {

	//create a new connection
	db, _ := CreateConnectionDefinition(hostname, port, database, username, password)

	//drop the test database
	db.DropDatabase()

}
