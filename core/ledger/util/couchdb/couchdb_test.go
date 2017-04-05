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
	db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}
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
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))
		}
	}
}

func TestDBCreateEnsureFullCommit(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		database := "testdbensurefullcommit"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Ensure a full commit
			_, commiterr := db.EnsureFullCommit()
			testutil.AssertNoError(t, commiterr, fmt.Sprintf("Error when trying to ensure a full commit"))

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
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

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
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

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
		db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

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
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

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
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

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
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

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

func TestRichQuery(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		byteJSON01 := []byte(`{"asset_name":"marble01","color":"blue","size":1,"owner":"jerry"}`)
		byteJSON02 := []byte(`{"asset_name":"marble02","color":"red","size":2,"owner":"tom"}`)
		byteJSON03 := []byte(`{"asset_name":"marble03","color":"green","size":3,"owner":"jerry"}`)
		byteJSON04 := []byte(`{"asset_name":"marble04","color":"purple","size":4,"owner":"tom"}`)
		byteJSON05 := []byte(`{"asset_name":"marble05","color":"blue","size":5,"owner":"jerry"}`)
		byteJSON06 := []byte(`{"asset_name":"marble06","color":"white","size":6,"owner":"tom"}`)
		byteJSON07 := []byte(`{"asset_name":"marble07","color":"white","size":7,"owner":"tom"}`)
		byteJSON08 := []byte(`{"asset_name":"marble08","color":"white","size":8,"owner":"tom"}`)
		byteJSON09 := []byte(`{"asset_name":"marble09","color":"white","size":9,"owner":"tom"}`)
		byteJSON10 := []byte(`{"asset_name":"marble10","color":"white","size":10,"owner":"tom"}`)
		byteJSON11 := []byte(`{"asset_name":"marble11","color":"green","size":11,"owner":"tom"}`)
		byteJSON12 := []byte(`{"asset_name":"marble12","color":"green","size":12,"owner":"frank"}`)

		attachment1 := &Attachment{}
		attachment1.AttachmentBytes = []byte(`marble01 - test attachment`)
		attachment1.ContentType = "application/octet-stream"
		attachment1.Name = "data"
		attachments1 := []Attachment{}
		attachments1 = append(attachments1, *attachment1)

		attachment2 := &Attachment{}
		attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
		attachment2.ContentType = "application/octet-stream"
		attachment2.Name = "data"
		attachments2 := []Attachment{}
		attachments2 = append(attachments2, *attachment2)

		attachment3 := &Attachment{}
		attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
		attachment3.ContentType = "application/octet-stream"
		attachment3.Name = "data"
		attachments3 := []Attachment{}
		attachments3 = append(attachments3, *attachment3)

		attachment4 := &Attachment{}
		attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
		attachment4.ContentType = "application/octet-stream"
		attachment4.Name = "data"
		attachments4 := []Attachment{}
		attachments4 = append(attachments4, *attachment4)

		attachment5 := &Attachment{}
		attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
		attachment5.ContentType = "application/octet-stream"
		attachment5.Name = "data"
		attachments5 := []Attachment{}
		attachments5 = append(attachments5, *attachment5)

		attachment6 := &Attachment{}
		attachment6.AttachmentBytes = []byte(`marble06 - test attachment`)
		attachment6.ContentType = "application/octet-stream"
		attachment6.Name = "data"
		attachments6 := []Attachment{}
		attachments6 = append(attachments6, *attachment6)

		attachment7 := &Attachment{}
		attachment7.AttachmentBytes = []byte(`marble07 - test attachment`)
		attachment7.ContentType = "application/octet-stream"
		attachment7.Name = "data"
		attachments7 := []Attachment{}
		attachments7 = append(attachments7, *attachment7)

		attachment8 := &Attachment{}
		attachment8.AttachmentBytes = []byte(`marble08 - test attachment`)
		attachment8.ContentType = "application/octet-stream"
		attachment7.Name = "data"
		attachments8 := []Attachment{}
		attachments8 = append(attachments8, *attachment8)

		attachment9 := &Attachment{}
		attachment9.AttachmentBytes = []byte(`marble09 - test attachment`)
		attachment9.ContentType = "application/octet-stream"
		attachment9.Name = "data"
		attachments9 := []Attachment{}
		attachments9 = append(attachments9, *attachment9)

		attachment10 := &Attachment{}
		attachment10.AttachmentBytes = []byte(`marble10 - test attachment`)
		attachment10.ContentType = "application/octet-stream"
		attachment10.Name = "data"
		attachments10 := []Attachment{}
		attachments10 = append(attachments10, *attachment10)

		attachment11 := &Attachment{}
		attachment11.AttachmentBytes = []byte(`marble11 - test attachment`)
		attachment11.ContentType = "application/octet-stream"
		attachment11.Name = "data"
		attachments11 := []Attachment{}
		attachments11 = append(attachments11, *attachment11)

		attachment12 := &Attachment{}
		attachment12.AttachmentBytes = []byte(`marble12 - test attachment`)
		attachment12.ContentType = "application/octet-stream"
		attachment12.Name = "data"
		attachments12 := []Attachment{}
		attachments12 = append(attachments12, *attachment12)

		database := "testrichquery"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object   --------------------------------------------------------
			couchInstance, err := CreateCouchInstance(connectURL, username, password)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
			db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

			//create a new database
			_, errdb := db.CreateDatabaseIfNotExist()
			testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

			//Save the test document
			_, saveerr := db.SaveDoc("marble01", "", &CouchDoc{JSONValue: byteJSON01, Attachments: attachments1})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble02", "", &CouchDoc{JSONValue: byteJSON02, Attachments: attachments2})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble03", "", &CouchDoc{JSONValue: byteJSON03, Attachments: attachments3})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble04", "", &CouchDoc{JSONValue: byteJSON04, Attachments: attachments4})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble05", "", &CouchDoc{JSONValue: byteJSON05, Attachments: attachments5})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble06", "", &CouchDoc{JSONValue: byteJSON06, Attachments: attachments6})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble07", "", &CouchDoc{JSONValue: byteJSON07, Attachments: attachments7})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble08", "", &CouchDoc{JSONValue: byteJSON08, Attachments: attachments8})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble09", "", &CouchDoc{JSONValue: byteJSON09, Attachments: attachments9})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble10", "", &CouchDoc{JSONValue: byteJSON10, Attachments: attachments10})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble11", "", &CouchDoc{JSONValue: byteJSON11, Attachments: attachments11})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Save the test document
			_, saveerr = db.SaveDoc("marble12", "", &CouchDoc{JSONValue: byteJSON12, Attachments: attachments12})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Test query with invalid JSON -------------------------------------------------------------------
			queryString := "{\"selector\":{\"owner\":}}"

			_, err = db.QueryDocuments(queryString)
			testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for bad json"))

			//Test query with object  -------------------------------------------------------------------
			queryString = "{\"selector\":{\"owner\":{\"$eq\":\"jerry\"}}}"

			queryResult, err := db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 3 results for owner="jerry"
			testutil.AssertEquals(t, len(*queryResult), 3)

			//Test query with implicit operator   --------------------------------------------------------------
			queryString = "{\"selector\":{\"owner\":\"jerry\"}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 3 results for owner="jerry"
			testutil.AssertEquals(t, len(*queryResult), 3)

			//Test query with specified fields   -------------------------------------------------------------------
			queryString = "{\"selector\":{\"owner\":{\"$eq\":\"jerry\"}},\"fields\": [\"owner\",\"asset_name\",\"color\",\"size\"]}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 3 results for owner="jerry"
			testutil.AssertEquals(t, len(*queryResult), 3)

			//Test query with a leading operator   -------------------------------------------------------------------
			queryString = "{\"selector\":{\"$or\":[{\"owner\":{\"$eq\":\"jerry\"}},{\"owner\": {\"$eq\": \"frank\"}}]}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 4 results for owner="jerry" or owner="frank"
			testutil.AssertEquals(t, len(*queryResult), 4)

			//Test query implicit and explicit operator   ------------------------------------------------------------------
			queryString = "{\"selector\":{\"color\":\"green\",\"$or\":[{\"owner\":\"tom\"},{\"owner\":\"frank\"}]}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 2 results for color="green" and (owner="jerry" or owner="frank")
			testutil.AssertEquals(t, len(*queryResult), 2)

			//Test query with a leading operator  -------------------------------------------------------------------------
			queryString = "{\"selector\":{\"$and\":[{\"size\":{\"$gte\":2}},{\"size\":{\"$lte\":5}}]}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 4 results for size >= 2 and size <= 5
			testutil.AssertEquals(t, len(*queryResult), 4)

			//Test query with leading and embedded operator  -------------------------------------------------------------
			queryString = "{\"selector\":{\"$and\":[{\"size\":{\"$gte\":3}},{\"size\":{\"$lte\":10}},{\"$not\":{\"size\":7}}]}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 7 results for size >= 3 and size <= 10 and not 7
			testutil.AssertEquals(t, len(*queryResult), 7)

			//Test query with leading operator and array of objects ----------------------------------------------------------
			queryString = "{\"selector\":{\"$and\":[{\"size\":{\"$gte\":2}},{\"size\":{\"$lte\":10}},{\"$nor\":[{\"size\":3},{\"size\":5},{\"size\":7}]}]}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 6 results for size >= 2 and size <= 10 and not 3,5 or 7
			testutil.AssertEquals(t, len(*queryResult), 6)

			//Test a range query ---------------------------------------------------------------------------------------------
			queryResult, err = db.ReadDocRange("marble02", "marble06", 10000, 0)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a range query"))

			//There should be 4 results
			testutil.AssertEquals(t, len(*queryResult), 4)

			//Test query with for tom  -------------------------------------------------------------------
			queryString = "{\"selector\":{\"owner\":{\"$eq\":\"tom\"}}}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 8 results for owner="tom"
			testutil.AssertEquals(t, len(*queryResult), 8)

			//Test query with for tom with limit  -------------------------------------------------------------------
			queryString = "{\"selector\":{\"owner\":{\"$eq\":\"tom\"}},\"limit\":2}"

			queryResult, err = db.QueryDocuments(queryString)
			testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to execute a query"))

			//There should be 2 results for owner="tom" with a limit of 2
			testutil.AssertEquals(t, len(*queryResult), 2)

		}
	}
}
