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

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

//Basic setup to test couch
var connectURL = "localhost:5984"
var badConnectURL = "localhost:5990"
var database = "testdb1"
var username = ""
var password = ""

func cleanup() {
	//create a new connection
	db, _ := CreateConnectionDefinition(connectURL, database, username, password)
	//drop the test database
	db.DropDatabase()
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

func TestDBConnectionDef(t *testing.T) {

	//call a helper method to load the core.yaml
	testutil.SetupCoreYAMLConfig("./../../../../peer")

	//create a new connection
	_, err := CreateConnectionDefinition(connectURL, "database", "", "")
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

}

func TestDBBadConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition("^^^localhost:5984", "database", "", "")
	testutil.AssertError(t, err, fmt.Sprintf("Did not receive error when trying to create database connection definition with a bad hostname"))

}

func TestDBCreateSaveWithoutRevision(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Save the test document
		_, saveerr := db.SaveDoc("2", "", assetJSON, nil)
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

	}
}

func TestDBBadConnection(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		//create a new connection
		db, err := CreateConnectionDefinition(badConnectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertError(t, errdb, fmt.Sprintf("Error should have been thrown while creating a database with an invalid connecion"))

		//Save the test document
		_, saveerr := db.SaveDoc("3", "", assetJSON, nil)
		testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have been thrown while saving a document with an invalid connecion"))

		//Retrieve the updated test document
		_, _, geterr := db.ReadDoc("3")
		testutil.AssertError(t, geterr, fmt.Sprintf("Error should have been thrown while retrieving a document with an invalid connecion"))

	}
}

func TestDBCreateDatabaseAndPersist(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, database)

		//Save the test document
		_, saveerr := db.SaveDoc("1", "", assetJSON, nil)
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
		_, saveerr = db.SaveDoc("1", "", assetDocUpdated, nil)
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

func TestDBBadJSON(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, database)

		badJSON := []byte(`{"asset_name"}`)

		//Save the test document
		_, saveerr := db.SaveDoc("1", "", badJSON, nil)
		testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have been thrown for a bad JSON"))

	}

}

func TestDBSaveAttachment(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()
		byteText := []byte(`This is a test document.  This is only a test`)

		attachment := Attachment{}
		attachment.AttachmentBytes = byteText
		attachment.ContentType = "text/plain"
		attachment.Name = "valueBytes"

		attachments := []Attachment{}
		attachments = append(attachments, attachment)

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Save the test document
		_, saveerr := db.SaveDoc("10", "", nil, attachments)
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

		//Attempt to retrieve the updated test document with attachments
		returnDoc, _, geterr2 := db.ReadDoc("10")
		testutil.AssertNoError(t, geterr2, fmt.Sprintf("Error when trying to retrieve a document with attachment"))

		//Test to see that the result from CouchDB matches the initial text
		testutil.AssertEquals(t, string(returnDoc), string(byteText))

	}
}

func TestDBRetrieveNonExistingDocument(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when attempting to create a database"))

		//Attempt to retrieve the updated test document
		_, _, geterr2 := db.ReadDoc("2")
		testutil.AssertError(t, geterr2, fmt.Sprintf("Error should have been thrown while attempting to retrieve a nonexisting document"))

	}
}

func TestDBTestExistingDB(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when attempting to create a database"))

		//run the create if not exist again.  should not return an error
		_, errdb = db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when attempting to create a database"))

	}
}

func TestDBTestDropNonExistDatabase(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//Attempt to drop the database without creating first
		_, errdbdrop := db.DropDatabase()
		testutil.AssertError(t, errdbdrop, fmt.Sprintf("Error should have been reported for attempting to drop a database before creation"))
	}

}

func TestDBTestDropDatabaseBadConnection(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		//create a new connection
		db, err := CreateConnectionDefinition(badConnectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//Attempt to drop the database without creating first
		_, errdbdrop := db.DropDatabase()
		testutil.AssertError(t, errdbdrop, fmt.Sprintf("Error should have been reported for attempting to drop a database before creation"))
	}

}

func TestDBReadDocumentRange(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() == true {

		cleanup()
		defer cleanup()

		var assetJSON1 = []byte(`{"asset_name":"marble1","color":"blue","size":"35","owner":"jerry"}`)
		var assetJSON2 = []byte(`{"asset_name":"marble2","color":"green","size":"25","owner":"jerry"}`)
		var assetJSON3 = []byte(`{"asset_name":"marble3","color":"green","size":"15","owner":"bob"}`)
		var assetJSON4 = []byte(`{"asset_name":"marble4","color":"red","size":"25","owner":"bob"}`)

		var textString1 = []byte("This is a test. iteration 1")
		var textString2 = []byte("This is a test. iteration 2")
		var textString3 = []byte("This is a test. iteration 3")
		var textString4 = []byte("This is a test. iteration 4")

		attachment1 := Attachment{}
		attachment1.AttachmentBytes = textString1
		attachment1.ContentType = "text/plain"
		attachment1.Name = "valueBytes"

		attachments1 := []Attachment{}
		attachments1 = append(attachments1, attachment1)

		attachment2 := Attachment{}
		attachment2.AttachmentBytes = textString2
		attachment2.ContentType = "text/plain"
		attachment2.Name = "valueBytes"

		attachments2 := []Attachment{}
		attachments2 = append(attachments2, attachment2)

		attachment3 := Attachment{}
		attachment3.AttachmentBytes = textString3
		attachment3.ContentType = "text/plain"
		attachment3.Name = "valueBytes"

		attachments3 := []Attachment{}
		attachments3 = append(attachments3, attachment3)

		attachment4 := Attachment{}
		attachment4.AttachmentBytes = textString4
		attachment4.ContentType = "text/plain"
		attachment4.Name = "valueBytes"

		attachments4 := []Attachment{}
		attachments4 = append(attachments4, attachment4)

		//create a new connection
		db, err := CreateConnectionDefinition(connectURL, database, username, password)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		//Retrieve the info for the new database and make sure the name matches
		dbResp, _, errdb := db.GetDatabaseInfo()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to retrieve database information"))
		testutil.AssertEquals(t, dbResp.DbName, database)

		//Save the test document
		_, saveerr1 := db.SaveDoc("1", "", assetJSON1, nil)
		testutil.AssertNoError(t, saveerr1, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr2 := db.SaveDoc("2", "", assetJSON2, nil)
		testutil.AssertNoError(t, saveerr2, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr3 := db.SaveDoc("3", "", assetJSON3, nil)
		testutil.AssertNoError(t, saveerr3, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr4 := db.SaveDoc("4", "", assetJSON4, nil)
		testutil.AssertNoError(t, saveerr4, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr5 := db.SaveDoc("11", "", nil, attachments1)
		testutil.AssertNoError(t, saveerr5, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr6 := db.SaveDoc("12", "", nil, attachments2)
		testutil.AssertNoError(t, saveerr6, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr7 := db.SaveDoc("13", "", nil, attachments3)
		testutil.AssertNoError(t, saveerr7, fmt.Sprintf("Error when trying to save a document"))

		//Save the test document
		_, saveerr8 := db.SaveDoc("5", "", nil, attachments4)
		testutil.AssertNoError(t, saveerr8, fmt.Sprintf("Error when trying to save a document"))

		queryResp, _ := db.ReadDocRange("1", "12", 1000, 0)

		//Ensure the query returns 3 documents
		testutil.AssertEquals(t, len(*queryResp), 3)

		for _, item := range *queryResp {

			if item.ID == "1" {

				//Unmarshal the document to Asset structure
				assetResp := &Asset{}
				json.Unmarshal(item.Value, &assetResp)

				//Verify the owner retrieved matches
				testutil.AssertEquals(t, assetResp.Owner, "jerry")

				//Verify the size retrieved matches
				testutil.AssertEquals(t, assetResp.Size, "35")
			}

			if item.ID == "11" {
				testutil.AssertEquals(t, string(item.Value), string(textString1))
			}

			if item.ID == "12" {
				testutil.AssertEquals(t, string(item.Value), string(textString2))
			}

		}

		queryString := "{\"selector\":{\"owner\": {\"$eq\": \"bob\"}},\"fields\": [\"_id\", \"_rev\", \"owner\", \"asset_name\", \"color\", \"size\"],\"limit\": 10,\"skip\": 0}"

		queryResult, _ := db.QueryDocuments(queryString, 1000, 0)

		//Ensure the query returns 2 documents
		testutil.AssertEquals(t, len(*queryResult), 2)

		for _, item := range *queryResult {

			//Unmarshal the document to Asset structure
			assetResp := &Asset{}
			json.Unmarshal(item.Value, &assetResp)

			//Verify the owner retrieved matches
			testutil.AssertEquals(t, assetResp.Owner, "bob")

		}

	}

}
