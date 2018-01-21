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
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/spf13/viper"
)

const badConnectURL = "couchdb:5990"
const badParseConnectURL = "http://host.com|5432"
const updateDocumentConflictError = "conflict"
const updateDocumentConflictReason = "Document update conflict."

var couchDBDef *CouchDBDef

func cleanup(database string) error {
	//create a new connection
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)

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
	// Read the core.yaml file for default config.
	ledgertestutil.SetupCoreYAMLConfig()

	// Switch to CouchDB
	viper.Set("ledger.state.stateDatabase", "CouchDB")

	// both vagrant and CI have couchdb configured at host "couchdb"
	viper.Set("ledger.state.couchDBConfig.couchDBAddress", "couchdb:5984")
	// Replace with correct username/password such as
	// admin/admin if user security is enabled on couchdb.
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 10)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)

	// Create CouchDB definition from config parameters
	couchDBDef = GetCouchDBDefinition()

	//run the tests
	result := m.Run()

	//revert to default goleveldb
	viper.Set("ledger.state.stateDatabase", "goleveldb")
	os.Exit(result)
}

func TestDBConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
	testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create database connection definition"))

}

func TestDBBadConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition(badParseConnectURL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
	testutil.AssertError(t, err, fmt.Sprintf("Did not receive error when trying to create database connection definition with a bad hostname"))

}

func TestEncodePathElement(t *testing.T) {

	encodedString := encodePathElement("testelement")
	testutil.AssertEquals(t, encodedString, "testelement")

	encodedString = encodePathElement("test element")
	testutil.AssertEquals(t, encodedString, "test%20element")

	encodedString = encodePathElement("/test element")
	testutil.AssertEquals(t, encodedString, "%2Ftest%20element")

	encodedString = encodePathElement("/test element:")
	testutil.AssertEquals(t, encodedString, "%2Ftest%20element:")

	encodedString = encodePathElement("/test+ element:")
	testutil.AssertEquals(t, encodedString, "%2Ftest%2B%20element:")

}

func TestBadCouchDBInstance(t *testing.T) {

	//TODO continue changes to return and removal of sprintf in followon changes
	if !ledgerconfig.IsCouchDBEnabled() {
		t.Skip("CouchDB is not enabled")
		return
	}
	//Create a bad connection definition
	badConnectDef := CouchConnectionDef{URL: badParseConnectURL, Username: "", Password: "",
		MaxRetries: 3, MaxRetriesOnStartup: 10, RequestTimeout: time.Second * 30}

	client := &http.Client{}

	//Create a bad couchdb instance
	badCouchDBInstance := CouchInstance{badConnectDef, client}

	//Create a bad CouchDatabase
	badDB := CouchDatabase{badCouchDBInstance, "baddb"}

	//Test CreateCouchDatabase with bad connection
	_, err := CreateCouchDatabase(badCouchDBInstance, "baddbtest")
	testutil.AssertError(t, err, "Error should have been thrown with CreateCouchDatabase and invalid connection")

	//Test CreateSystemDatabasesIfNotExist with bad connection
	err = CreateSystemDatabasesIfNotExist(badCouchDBInstance)
	testutil.AssertError(t, err, "Error should have been thrown with CreateSystemDatabasesIfNotExist and invalid connection")

	//Test CreateDatabaseIfNotExist with bad connection
	_, err = badDB.CreateDatabaseIfNotExist()
	testutil.AssertError(t, err, "Error should have been thrown with CreateDatabaseIfNotExist and invalid connection")

	//Test GetDatabaseInfo with bad connection
	_, _, err = badDB.GetDatabaseInfo()
	testutil.AssertError(t, err, "Error should have been thrown with GetDatabaseInfo and invalid connection")

	//Test VerifyCouchConfig with bad connection
	_, _, err = badCouchDBInstance.VerifyCouchConfig()
	testutil.AssertError(t, err, "Error should have been thrown with VerifyCouchConfig and invalid connection")

	//Test EnsureFullCommit with bad connection
	_, err = badDB.EnsureFullCommit()
	testutil.AssertError(t, err, "Error should have been thrown with EnsureFullCommit and invalid connection")

	//Test DropDatabase with bad connection
	_, err = badDB.DropDatabase()
	testutil.AssertError(t, err, "Error should have been thrown with DropDatabase and invalid connection")

	//Test ReadDoc with bad connection
	_, _, err = badDB.ReadDoc("1")
	testutil.AssertError(t, err, "Error should have been thrown with ReadDoc and invalid connection")

	//Test SaveDoc with bad connection
	_, err = badDB.SaveDoc("1", "1", nil)
	testutil.AssertError(t, err, "Error should have been thrown with SaveDoc and invalid connection")

	//Test DeleteDoc with bad connection
	err = badDB.DeleteDoc("1", "1")
	testutil.AssertError(t, err, "Error should have been thrown with DeleteDoc and invalid connection")

	//Test ReadDocRange with bad connection
	_, err = badDB.ReadDocRange("1", "2", 1000, 0)
	testutil.AssertError(t, err, "Error should have been thrown with ReadDocRange and invalid connection")

	//Test QueryDocuments with bad connection
	_, err = badDB.QueryDocuments("1")
	testutil.AssertError(t, err, "Error should have been thrown with QueryDocuments and invalid connection")

	//Test BatchRetrieveIDRevision with bad connection
	_, err = badDB.BatchRetrieveIDRevision(nil)
	testutil.AssertError(t, err, "Error should have been thrown with BatchRetrieveIDRevision and invalid connection")

	//Test BatchUpdateDocuments with bad connection
	_, err = badDB.BatchUpdateDocuments(nil)
	testutil.AssertError(t, err, "Error should have been thrown with BatchUpdateDocuments and invalid connection")

}

func TestDBCreateSaveWithoutRevision(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbcreatesavewithoutrevision"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbensurefullcommit"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

	if ledgerconfig.IsCouchDBEnabled() {

		//create a new instance and database object using a valid database name mixed case
		couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr := CreateCouchDatabase(*couchInstance, "testDB")
		testutil.AssertError(t, dberr, "Error should have been thrown for an invalid db name")

		//create a new instance and database object using a valid database name letters and numbers
		couchInstance, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr = CreateCouchDatabase(*couchInstance, "test132")
		testutil.AssertNoError(t, dberr, fmt.Sprintf("Error when testing a valid database name"))

		//create a new instance and database object using a valid database name - special characters
		couchInstance, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr = CreateCouchDatabase(*couchInstance, "test1234~!@#$%^&*()[]{}.")
		testutil.AssertError(t, dberr, "Error should have been thrown for an invalid db name")

		//create a new instance and database object using a invalid database name - too long	/*
		couchInstance, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		_, dberr = CreateCouchDatabase(*couchInstance, "a12345678901234567890123456789012345678901234"+
			"56789012345678901234567890123456789012345678901234567890123456789012345678901234567890"+
			"12345678901234567890123456789012345678901234567890123456789012345678901234567890123456"+
			"78901234567890123456789012345678901234567890")
		testutil.AssertError(t, dberr, fmt.Sprintf("Error should have been thrown for invalid database name"))

	}
}

func TestDBBadConnection(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		//create a new instance and database object
		//Limit the maxRetriesOnStartup to 3 in order to reduce time for the failure
		_, err := CreateCouchInstance(badConnectURL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, 3, couchDBDef.RequestTimeout)
		testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for a bad connection"))
	}
}

func TestDBCreateDatabaseAndPersist(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbcreatedatabaseandpersist"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

			testBytes2 := []byte(`test attachment 2`)

			attachment2 := &Attachment{}
			attachment2.AttachmentBytes = testBytes2
			attachment2.ContentType = "application/octet-stream"
			attachment2.Name = "data"
			attachments2 := []*Attachment{}
			attachments2 = append(attachments2, attachment2)

			//Save the test document with an attachment
			_, saveerr = db.SaveDoc("2", "", &CouchDoc{JSONValue: nil, Attachments: attachments2})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Retrieve the test document with attachments
			dbGetResp, _, geterr = db.ReadDoc("2")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//verify the text from the attachment is correct
			testattach := dbGetResp.Attachments[0].AttachmentBytes
			testutil.AssertEquals(t, testattach, testBytes2)

			testBytes3 := []byte{}

			attachment3 := &Attachment{}
			attachment3.AttachmentBytes = testBytes3
			attachment3.ContentType = "application/octet-stream"
			attachment3.Name = "data"
			attachments3 := []*Attachment{}
			attachments3 = append(attachments3, attachment3)

			//Save the test document with a zero length attachment
			_, saveerr = db.SaveDoc("3", "", &CouchDoc{JSONValue: nil, Attachments: attachments3})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

			//Retrieve the test document with attachments
			dbGetResp, _, geterr = db.ReadDoc("3")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			//verify the text from the attachment is correct,  zero bytes
			testattach = dbGetResp.Attachments[0].AttachmentBytes
			testutil.AssertEquals(t, testattach, testBytes3)

			testBytes4a := []byte(`test attachment 4a`)
			attachment4a := &Attachment{}
			attachment4a.AttachmentBytes = testBytes4a
			attachment4a.ContentType = "application/octet-stream"
			attachment4a.Name = "data1"

			testBytes4b := []byte(`test attachment 4b`)
			attachment4b := &Attachment{}
			attachment4b.AttachmentBytes = testBytes4b
			attachment4b.ContentType = "application/octet-stream"
			attachment4b.Name = "data2"

			attachments4 := []*Attachment{}
			attachments4 = append(attachments4, attachment4a)
			attachments4 = append(attachments4, attachment4b)

			//Save the updated test document with multiple attachments
			_, saveerr = db.SaveDoc("4", "", &CouchDoc{JSONValue: assetJSON, Attachments: attachments4})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save the updated document"))

			//Retrieve the test document with attachments
			dbGetResp, _, geterr = db.ReadDoc("4")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			for _, attach4 := range dbGetResp.Attachments {

				currentName := attach4.Name
				if currentName == "data1" {
					testutil.AssertEquals(t, attach4.AttachmentBytes, testBytes4a)
				}
				if currentName == "data2" {
					testutil.AssertEquals(t, attach4.AttachmentBytes, testBytes4b)
				}

			}

			testBytes5a := []byte(`test attachment 5a`)
			attachment5a := &Attachment{}
			attachment5a.AttachmentBytes = testBytes5a
			attachment5a.ContentType = "application/octet-stream"
			attachment5a.Name = "data1"

			testBytes5b := []byte{}
			attachment5b := &Attachment{}
			attachment5b.AttachmentBytes = testBytes5b
			attachment5b.ContentType = "application/octet-stream"
			attachment5b.Name = "data2"

			attachments5 := []*Attachment{}
			attachments5 = append(attachments5, attachment5a)
			attachments5 = append(attachments5, attachment5b)

			//Save the updated test document with multiple attachments and zero length attachments
			_, saveerr = db.SaveDoc("5", "", &CouchDoc{JSONValue: assetJSON, Attachments: attachments5})
			testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save the updated document"))

			//Retrieve the test document with attachments
			dbGetResp, _, geterr = db.ReadDoc("5")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

			for _, attach5 := range dbGetResp.Attachments {

				currentName := attach5.Name
				if currentName == "data1" {
					testutil.AssertEquals(t, attach5.AttachmentBytes, testBytes5a)
				}
				if currentName == "data2" {
					testutil.AssertEquals(t, attach5.AttachmentBytes, testBytes5b)
				}

			}

			//Attempt to save the document with an invalid id
			_, saveerr = db.SaveDoc(string([]byte{0xff, 0xfe, 0xfd}), "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have been thrown when saving a document with an invalid ID"))

			//Attempt to read a document with an invalid id
			_, _, readerr := db.ReadDoc(string([]byte{0xff, 0xfe, 0xfd}))
			testutil.AssertError(t, readerr, fmt.Sprintf("Error should have been thrown when reading a document with an invalid ID"))

			//Drop the database
			_, errdbdrop := db.DropDatabase()
			testutil.AssertNoError(t, errdbdrop, fmt.Sprintf("Error dropping database"))

			//Make sure an error is thrown for getting info for a missing database
			_, _, errdbinfo := db.GetDatabaseInfo()
			testutil.AssertError(t, errdbinfo, fmt.Sprintf("Error should have been thrown for missing database"))

			//Attempt to save a document to a deleted database
			_, saveerr = db.SaveDoc("6", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
			testutil.AssertError(t, saveerr, fmt.Sprintf("Error should have been thrown while attempting to save to a deleted database"))

			//Attempt to read from a deleted database
			_, _, geterr = db.ReadDoc("6")
			testutil.AssertNoError(t, geterr, fmt.Sprintf("Error should not have been thrown for a missing database, nil value is returned"))

		}
	}

}

func TestDBRequestTimeout(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbrequesttimeout"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {

			//create an impossibly short timeout
			impossibleTimeout := time.Microsecond * 1

			//create a new instance and database object with a timeout that will fail
			//Also use a maxRetriesOnStartup=3 to reduce the number of retries
			_, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, 3, impossibleTimeout)
			testutil.AssertError(t, err, fmt.Sprintf("Error should have been thown while trying to create a couchdb instance with a connection timeout"))

			//see if the error message contains the timeout error
			testutil.AssertEquals(t, strings.Count(err.Error(), "Client.Timeout exceeded while awaiting headers"), 1)

		}
	}
}

func TestDBTimeoutConflictRetry(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbtimeoutretry"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		// if there was an error upon cleanup, return here
		if err != nil {
			return
		}

		//create a new instance and database object
		couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, 3, couchDBDef.RequestTimeout)
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
		_, saveerr := db.SaveDoc("1", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document"))

		//Retrieve the test document
		_, _, geterr := db.ReadDoc("1")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

		//Save the test document with an invalid rev.  This should cause a retry
		_, saveerr = db.SaveDoc("1", "1-11111111111111111111111111111111", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
		testutil.AssertNoError(t, saveerr, fmt.Sprintf("Error when trying to save a document with a revision conflict"))

		//Delete the test document with an invalid rev.  This should cause a retry
		deleteerr := db.DeleteDoc("1", "1-11111111111111111111111111111111")
		testutil.AssertNoError(t, deleteerr, fmt.Sprintf("Error when trying to delete a document with a revision conflict"))

	}
}

func TestDBBadNumberOfRetries(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbbadretries"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		// if there was an error upon cleanup, return here
		if err != nil {
			return
		}

		//create a new instance and database object
		_, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			0, 3, couchDBDef.RequestTimeout)
		testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown while attempting to create a database"))

	}
}

func TestDBBadJSON(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbbadjson"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {

			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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
		couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbsaveattachment"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {

			byteText := []byte(`This is a test document.  This is only a test`)

			attachment := &Attachment{}
			attachment.AttachmentBytes = byteText
			attachment.ContentType = "text/plain"
			attachment.Name = "valueBytes"

			attachments := []*Attachment{}
			attachments = append(attachments, attachment)

			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbdeletedocument"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

	if ledgerconfig.IsCouchDBEnabled() {

		database := "testdbdeletenonexistingdocument"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

	if ledgerconfig.IsCouchDBEnabled() {

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
		attachments1 := []*Attachment{}
		attachments1 = append(attachments1, attachment1)

		attachment2 := &Attachment{}
		attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
		attachment2.ContentType = "application/octet-stream"
		attachment2.Name = "data"
		attachments2 := []*Attachment{}
		attachments2 = append(attachments2, attachment2)

		attachment3 := &Attachment{}
		attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
		attachment3.ContentType = "application/octet-stream"
		attachment3.Name = "data"
		attachments3 := []*Attachment{}
		attachments3 = append(attachments3, attachment3)

		attachment4 := &Attachment{}
		attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
		attachment4.ContentType = "application/octet-stream"
		attachment4.Name = "data"
		attachments4 := []*Attachment{}
		attachments4 = append(attachments4, attachment4)

		attachment5 := &Attachment{}
		attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
		attachment5.ContentType = "application/octet-stream"
		attachment5.Name = "data"
		attachments5 := []*Attachment{}
		attachments5 = append(attachments5, attachment5)

		attachment6 := &Attachment{}
		attachment6.AttachmentBytes = []byte(`marble06 - test attachment`)
		attachment6.ContentType = "application/octet-stream"
		attachment6.Name = "data"
		attachments6 := []*Attachment{}
		attachments6 = append(attachments6, attachment6)

		attachment7 := &Attachment{}
		attachment7.AttachmentBytes = []byte(`marble07 - test attachment`)
		attachment7.ContentType = "application/octet-stream"
		attachment7.Name = "data"
		attachments7 := []*Attachment{}
		attachments7 = append(attachments7, attachment7)

		attachment8 := &Attachment{}
		attachment8.AttachmentBytes = []byte(`marble08 - test attachment`)
		attachment8.ContentType = "application/octet-stream"
		attachment7.Name = "data"
		attachments8 := []*Attachment{}
		attachments8 = append(attachments8, attachment8)

		attachment9 := &Attachment{}
		attachment9.AttachmentBytes = []byte(`marble09 - test attachment`)
		attachment9.ContentType = "application/octet-stream"
		attachment9.Name = "data"
		attachments9 := []*Attachment{}
		attachments9 = append(attachments9, attachment9)

		attachment10 := &Attachment{}
		attachment10.AttachmentBytes = []byte(`marble10 - test attachment`)
		attachment10.ContentType = "application/octet-stream"
		attachment10.Name = "data"
		attachments10 := []*Attachment{}
		attachments10 = append(attachments10, attachment10)

		attachment11 := &Attachment{}
		attachment11.AttachmentBytes = []byte(`marble11 - test attachment`)
		attachment11.ContentType = "application/octet-stream"
		attachment11.Name = "data"
		attachments11 := []*Attachment{}
		attachments11 = append(attachments11, attachment11)

		attachment12 := &Attachment{}
		attachment12.AttachmentBytes = []byte(`marble12 - test attachment`)
		attachment12.ContentType = "application/octet-stream"
		attachment12.Name = "data"
		attachments12 := []*Attachment{}
		attachments12 = append(attachments12, attachment12)

		database := "testrichquery"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		if err == nil {
			//create a new instance and database object   --------------------------------------------------------
			couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
				couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
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

			//Test query with invalid index  -------------------------------------------------------------------
			queryString = "{\"selector\":{\"owner\":\"tom\"}, \"use_index\":[\"_design/indexOwnerDoc\",\"indexOwner\"]}"

			_, err = db.QueryDocuments(queryString)
			testutil.AssertError(t, err, fmt.Sprintf("Error should have been thrown for an invalid index"))

		}
	}
}

func TestBatchBatchOperations(t *testing.T) {

	if ledgerconfig.IsCouchDBEnabled() {

		byteJSON01 := []byte(`{"_id":"marble01","asset_name":"marble01","color":"blue","size":"1","owner":"jerry"}`)
		byteJSON02 := []byte(`{"_id":"marble02","asset_name":"marble02","color":"red","size":"2","owner":"tom"}`)
		byteJSON03 := []byte(`{"_id":"marble03","asset_name":"marble03","color":"green","size":"3","owner":"jerry"}`)
		byteJSON04 := []byte(`{"_id":"marble04","asset_name":"marble04","color":"purple","size":"4","owner":"tom"}`)
		byteJSON05 := []byte(`{"_id":"marble05","asset_name":"marble05","color":"blue","size":"5","owner":"jerry"}`)

		attachment1 := &Attachment{}
		attachment1.AttachmentBytes = []byte(`marble01 - test attachment`)
		attachment1.ContentType = "application/octet-stream"
		attachment1.Name = "data"
		attachments1 := []*Attachment{}
		attachments1 = append(attachments1, attachment1)

		attachment2 := &Attachment{}
		attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
		attachment2.ContentType = "application/octet-stream"
		attachment2.Name = "data"
		attachments2 := []*Attachment{}
		attachments2 = append(attachments2, attachment2)

		attachment3 := &Attachment{}
		attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
		attachment3.ContentType = "application/octet-stream"
		attachment3.Name = "data"
		attachments3 := []*Attachment{}
		attachments3 = append(attachments3, attachment3)

		attachment4 := &Attachment{}
		attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
		attachment4.ContentType = "application/octet-stream"
		attachment4.Name = "data"
		attachments4 := []*Attachment{}
		attachments4 = append(attachments4, attachment4)

		attachment5 := &Attachment{}
		attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
		attachment5.ContentType = "application/octet-stream"
		attachment5.Name = "data"
		attachments5 := []*Attachment{}
		attachments5 = append(attachments5, attachment5)

		database := "testbatch"
		err := cleanup(database)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to cleanup  Error: %s", err))
		defer cleanup(database)

		//create a new instance and database object   --------------------------------------------------------
		couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
			couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when trying to create couch instance"))
		db := CouchDatabase{CouchInstance: *couchInstance, DBName: database}

		//create a new database
		_, errdb := db.CreateDatabaseIfNotExist()
		testutil.AssertNoError(t, errdb, fmt.Sprintf("Error when trying to create database"))

		batchUpdateDocs := []*CouchDoc{}

		value1 := &CouchDoc{JSONValue: byteJSON01, Attachments: attachments1}
		value2 := &CouchDoc{JSONValue: byteJSON02, Attachments: attachments2}
		value3 := &CouchDoc{JSONValue: byteJSON03, Attachments: attachments3}
		value4 := &CouchDoc{JSONValue: byteJSON04, Attachments: attachments4}
		value5 := &CouchDoc{JSONValue: byteJSON05, Attachments: attachments5}

		batchUpdateDocs = append(batchUpdateDocs, value1)
		batchUpdateDocs = append(batchUpdateDocs, value2)
		batchUpdateDocs = append(batchUpdateDocs, value3)
		batchUpdateDocs = append(batchUpdateDocs, value4)
		batchUpdateDocs = append(batchUpdateDocs, value5)

		batchUpdateResp, err := db.BatchUpdateDocuments(batchUpdateDocs)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to update a batch of documents"))

		//check to make sure each batch update response was successful
		for _, updateDoc := range batchUpdateResp {
			testutil.AssertEquals(t, updateDoc.Ok, true)
		}

		//----------------------------------------------
		//Test Retrieve JSON
		dbGetResp, _, geterr := db.ReadDoc("marble01")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when attempting read a document"))

		assetResp := &Asset{}
		geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))
		//Verify the owner retrieved matches
		testutil.AssertEquals(t, assetResp.Owner, "jerry")

		//----------------------------------------------
		//Test retrieve binary
		dbGetResp, _, geterr = db.ReadDoc("marble03")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when attempting read a document"))
		//Retrieve the attachments
		attachments := dbGetResp.Attachments
		//Only one was saved, so take the first
		retrievedAttachment := attachments[0]
		//Verify the text matches
		testutil.AssertEquals(t, attachment3.AttachmentBytes, retrievedAttachment.AttachmentBytes)
		//----------------------------------------------
		//Test Bad Updates
		batchUpdateDocs = []*CouchDoc{}
		batchUpdateDocs = append(batchUpdateDocs, value1)
		batchUpdateDocs = append(batchUpdateDocs, value2)
		batchUpdateResp, err = db.BatchUpdateDocuments(batchUpdateDocs)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to update a batch of documents"))
		//No revision was provided, so these two updates should fail
		//Verify that the "Ok" field is returned as false
		for _, updateDoc := range batchUpdateResp {
			testutil.AssertEquals(t, updateDoc.Ok, false)
			testutil.AssertEquals(t, updateDoc.Error, updateDocumentConflictError)
			testutil.AssertEquals(t, updateDoc.Reason, updateDocumentConflictReason)
		}

		//----------------------------------------------
		//Test Batch Retrieve Keys and Update

		var keys []string

		keys = append(keys, "marble01")
		keys = append(keys, "marble03")

		batchRevs, err := db.BatchRetrieveIDRevision(keys)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting retrieve revisions"))

		batchUpdateDocs = []*CouchDoc{}

		//iterate through the revision docs
		for _, revdoc := range batchRevs {
			if revdoc.ID == "marble01" {
				//update the json with the rev and add to the batch
				marble01Doc := addRevisionAndDeleteStatus(revdoc.Rev, byteJSON01, false)
				batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: marble01Doc, Attachments: attachments1})
			}

			if revdoc.ID == "marble03" {
				//update the json with the rev and add to the batch
				marble03Doc := addRevisionAndDeleteStatus(revdoc.Rev, byteJSON03, false)
				batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: marble03Doc, Attachments: attachments3})
			}
		}

		//Update couchdb with the batch
		batchUpdateResp, err = db.BatchUpdateDocuments(batchUpdateDocs)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to update a batch of documents"))
		//check to make sure each batch update response was successful
		for _, updateDoc := range batchUpdateResp {
			testutil.AssertEquals(t, updateDoc.Ok, true)
		}

		//----------------------------------------------
		//Test Batch Delete

		keys = []string{}

		keys = append(keys, "marble02")
		keys = append(keys, "marble04")

		batchRevs, err = db.BatchRetrieveIDRevision(keys)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting retrieve revisions"))

		batchUpdateDocs = []*CouchDoc{}

		//iterate through the revision docs
		for _, revdoc := range batchRevs {
			if revdoc.ID == "marble02" {
				//update the json with the rev and add to the batch
				marble02Doc := addRevisionAndDeleteStatus(revdoc.Rev, byteJSON02, true)
				batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: marble02Doc, Attachments: attachments1})
			}
			if revdoc.ID == "marble04" {
				//update the json with the rev and add to the batch
				marble04Doc := addRevisionAndDeleteStatus(revdoc.Rev, byteJSON04, true)
				batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: marble04Doc, Attachments: attachments3})
			}
		}

		//Update couchdb with the batch
		batchUpdateResp, err = db.BatchUpdateDocuments(batchUpdateDocs)
		testutil.AssertNoError(t, err, fmt.Sprintf("Error when attempting to update a batch of documents"))

		//check to make sure each batch update response was successful
		for _, updateDoc := range batchUpdateResp {
			testutil.AssertEquals(t, updateDoc.Ok, true)
		}

		//Retrieve the test document
		dbGetResp, _, geterr = db.ReadDoc("marble02")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

		//assert the value was deleted
		testutil.AssertNil(t, dbGetResp)

		//Retrieve the test document
		dbGetResp, _, geterr = db.ReadDoc("marble04")
		testutil.AssertNoError(t, geterr, fmt.Sprintf("Error when trying to retrieve a document"))

		//assert the value was deleted
		testutil.AssertNil(t, dbGetResp)
	}
}

//addRevisionAndDeleteStatus adds keys for version and chaincodeID to the JSON value
func addRevisionAndDeleteStatus(revision string, value []byte, deleted bool) []byte {

	//create a version mapping
	jsonMap := make(map[string]interface{})

	json.Unmarshal(value, &jsonMap)

	//add the revision
	if revision != "" {
		jsonMap["_rev"] = revision
	}

	//If this record is to be deleted, set the "_deleted" property to true
	if deleted {
		jsonMap["_deleted"] = true
	}
	//marshal the data to a byte array
	returnJSON, _ := json.Marshal(jsonMap)

	return returnJSON

}
