/*
Copyright IBM Corp. 2016, 2017 All Rights Reserved.

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
	"os"
	"testing"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
)

//Basic setup to test couch
var connectURL = "couchdb:5984"
var badConnectURL = "couchdb:5990"
var username = ""
var password = ""

func cleanup(database string) error {
	//create a new connection
	couchInstance, err := CreateCouchInstance(connectURL, username, password)
	if err != nil {
		fmt.Println("Unexpected error", err)
		return err
	}
	db := CouchDatabase{couchInstance: *couchInstance, dbName: database}
	//drop the test database
	db.DropDatabase()
	return nil
}

type Asset struct {
	ID        string `json:"_id"`
	Rev       string `json:"_rev"`
	AssetName string `json:"asset_name"`
	Color     string `json:"color"`
	Size      string `json:"size"`
	Owner     string `json:"owner"`
}

var assetJSON = []byte(`{"asset_name":"marble1","color":"blue","size":"35","owner":"jerry"}`)

func TestMain(m *testing.M) {
	ledgertestutil.SetupCoreYAMLConfig("./../../../../peer")
	viper.Set("ledger.state.stateDatabase", "CouchDB")
	result := m.Run()
	viper.Set("ledger.state.stateDatabase", "goleveldb")
	os.Exit(result)
}

func TestDBConnectionDef(t *testing.T) {

	//call a helper method to load the core.yaml
	ledgertestutil.SetupCoreYAMLConfig("./../../../../peer")

	//create a new connection
	_, err := CreateConnectionDefinition(connectURL, "", "")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

}

func TestDBBadConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition("^^^localhost:5984", "", "")
	testutil.AssertError(t, err, fmt.Sprintf("Did not receive error when trying to create database connection definition with a bad hostname"))

}

func TestDBCreateSaveWithoutRevision(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbcreatesavewithoutrevision"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))
		}
	}
}

func TestDBBadDatabaseName(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		//create a new instance and database object using a valid database name mixed case
		couchInstance, err := CreateCouchInstance(connectURL, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr := CreateCouchDatabase(*couchInstance, "testDB")
		testutil.AssertNoError(t, dberr, fmt.Sprintf("Error when testing a valid database name"))

		//create a new instance and database object using a valid database name letters and numbers
		couchInstance, err = CreateCouchInstance(connectURL, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr = CreateCouchDatabase(*couchInstance, "test132")
		testutil.AssertNoError(t, dberr, fmt.Sprintf("Error when testing a valid database name"))

		//create a new instance and database object using a valid database name - special characters
		couchInstance, err = CreateCouchInstance(connectURL, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr = CreateCouchDatabase(*couchInstance, "test1234~!@#$%^&*()[]{}.")
		testutil.AssertNoError(t, dberr, fmt.Sprintf("Error when testing a valid database name"))

		//create a new instance and database object using a invalid database name - too long	/*
		couchInstance, err = CreateCouchInstance(connectURL, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr = CreateCouchDatabase(*couchInstance, "A12345678901234567890123456789012345678901234"+
			"56789012345678901234567890123456789012345678901234567890123456789012345678901234567890"+
			"12345678901234567890123456789012345678901234567890123456789012345678901234567890123456"+
			"78901234567890123456789012345678901234567890")
		testutil.AssertError(t, dberr, fmt.Sprintf("Error should have been thrown for invalid database name"))

	}

}

func TestDBBadConnection(t *testing.T) {

	// TestDBBadConnectionDef skipped since retry logic stalls the unit tests for two minutes.
	// TODO Re-enable once configurable retry logic is introduced
	t.Skip()

	if ledgerconfig.IsCouchDBEnabled() == true {

		//create a new instance and database object
		_, err := CreateCouchInstance(badConnectURL, username, password)
		testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for a bad connection"))
	}
}

func TestDBCreateDatabaseAndPersist(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbcreatedatabaseandpersist"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Retrieve the info for the new database and make sure the name matches
			dbResp, _, errdb := db.GetDatabaseInfo()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
			testutil.AssertEquals(t, dbResp.DbName, database)

			//Save the test document
			_, saveerr := db.SaveDoc("idWith/slash", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Retrieve the test document
			dbGetResp, _, geterr := db.ReadDoc("idWith/slash")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//Unmarshal the document to Asset structure
			assetResp := &Asset{}
			geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//Verify the owner retrieved matches
			testutil.AssertEquals(t, assetResp.Owner, "jerry")

			//Save the test document
			_, saveerr = db.SaveDoc("1", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Retrieve the test document
			dbGetResp, _, geterr = db.ReadDoc("1")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//Unmarshal the document to Asset structure
			assetResp = &Asset{}
			geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//Verify the owner retrieved matches
			testutil.AssertEquals(t, assetResp.Owner, "jerry")

			//Change owner to bob
			assetResp.Owner = "bob"

			//create a byte array of the JSON
			assetDocUpdated, _ := json.Marshal(assetResp)

			//Save the updated test document
			_, saveerr = db.SaveDoc("1", "", &CouchDoc{JSONValue: assetDocUpdated, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save the updated document"))

			//Retrieve the updated test document
			dbGetResp, _, geterr = db.ReadDoc("1")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//Unmarshal the document to Asset structure
			assetResp = &Asset{}
			json.Unmarshal(dbGetResp.JSONValue, &assetResp)

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

}

func TestDBBadJSON(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbbadjson"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {

			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Retrieve the info for the new database and make sure the name matches
			dbResp, _, errdb := db.GetDatabaseInfo()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
			testutil.AssertEquals(t, dbResp.DbName, database)

			badJSON := []byte(`{"asset_name"}`)

			//Save the test document
			_, saveerr := db.SaveDoc("1", "", &CouchDoc{JSONValue: badJSON, Attachments: nil})
			testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have been thrown for a bad JSON"))

		}

	}

}

func TestPrefixScan(t *testing.T) {
	if !ledgerconfig.IsCouchDBEnabled() {
		return
	}
	database := "testprefixscan"
	err := cleanup(database)
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
	defer cleanup(database)

	if err == nil {
		//create a new instance and database object
		couchInstance, err := CreateCouchInstance(connectURL, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, database)

		//Save documents
		for i := 0; i < 20; i++ {
			id1 := string(0) + string(i) + string(0)
			id2 := string(0) + string(i) + string(1)
			id3 := string(0) + string(i) + string(utf8.MaxRune-1)
			_, saveerr := db.SaveDoc(id1, "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))
			_, saveerr = db.SaveDoc(id2, "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))
			_, saveerr = db.SaveDoc(id3, "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

		}
		startKey := string(0) + string(10)
		endKey := startKey + string(utf8.MaxRune)
		_, _, geterr := db.ReadDoc(endKey)
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to get lastkey"))

		resultsPtr, geterr := db.ReadDocRange(startKey, endKey, 1000, 0)
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to perform a range scan"))
		testutil.AssertNotNil(t, resultsPtr)
		results := *resultsPtr
		testutil.AssertEquals(t, len(results), 3)
		testutil.AssertEquals(t, results[0].ID, string(0)+string(10)+string(0))
		testutil.AssertEquals(t, results[1].ID, string(0)+string(10)+string(1))
		testutil.AssertEquals(t, results[2].ID, string(0)+string(10)+string(utf8.MaxRune-1))

		//Drop the database
		_, errdbdrop := db.DropDatabase()
		testutil.AssertNoError(t, errdbdrop, fmt.Sprintf("Error dropping database"))

		//Retrieve the info for the new database and make sure the name matches
		_, _, errdbinfo := db.GetDatabaseInfo()
		testutil.AssertError(t, errdbinfo, fmt.Sprintf("Error should have been thrown for missing database"))
	}
}

func TestDBSaveAttachment(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbsaveattachment"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {

			byteText := []byte(`This is a test document.  This is only a test`)

			attachment := Attachment{}
			attachment.AttachmentBytes = byteText
			attachment.ContentType = "text/plain"
			attachment.Name = "valueBytes"

			attachments := []Attachment{}
			attachments = append(attachments, attachment)

			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			_, saveerr := db.SaveDoc("10", "", &CouchDoc{JSONValue: nil, Attachments: attachments})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Attempt to retrieve the updated test document with attachments
			couchDoc, _, geterr2 := db.ReadDoc("10")
			testutil.AssertNoError(t, geterr2, fmt.Sprintf("Error when trying to retrieve a document with attachment"))
			testutil.AssertNotNil(t, couchDoc.Attachments)
			testutil.AssertEquals(t, couchDoc.Attachments[0].AttachmentBytes, byteText)
		}

	}
}

func TestDBDeleteDocument(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbdeletedocument"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Attempt to retrieve the test document
			_, _, readErr := db.ReadDoc("2")
			testutil.AssertNoError(t, readErr, fmt.Sprintf("Error when trying to retrieve a document with attachment"))

			//Delete the test document
			deleteErr := db.DeleteDoc("2", "")
			testutil.AssertNoError(t, deleteErr, fmt.Sprintf("Error when trying to delete a document"))

			//Attempt to retrieve the test document
			readValue, _, _ := db.ReadDoc("2")
			testutil.AssertNil(t, readValue)
		}
	}
}

func TestDBDeleteNonExistingDocument(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbdeletenonexistingdocument"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{couchInstance: *couchInstance, dbName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			deleteErr := db.DeleteDoc("2", "")
			testutil.AssertNoError(t, deleteErr, fmt.Sprintf("Error when trying to delete a non existing document"))
		}
	}
}

func TestCouchDBVersion(t *testing.T) {

	err := checkCouchDBVersion("2.0.0")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error should not have been thrown for valid version"))

	err = checkCouchDBVersion("4.5.0")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error should not have been thrown for valid version"))

	err = checkCouchDBVersion("1.6.5.4")
	testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for invalid version"))

	err = checkCouchDBVersion("0.0.0.0")
	testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for invalid version"))

}
