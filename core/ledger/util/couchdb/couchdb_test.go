/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

const badConnectURL = "couchdb:5990"
const badParseConnectURL = "http://host.com|5432"
const updateDocumentConflictError = "conflict"
const updateDocumentConflictReason = "Document update conflict."

var couchDBDef *CouchDBDef

func cleanup(database string) error {
	//create a new connection
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	if err != nil {
		fmt.Println("Unexpected error", err)
		return err
	}
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}
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
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	// Read the core.yaml file for default config.
	ledgertestutil.SetupCoreYAMLConfig()

	// Switch to CouchDB
	couchAddress, cleanup := couchDBSetup()
	defer cleanup()
	viper.Set("ledger.state.stateDatabase", "CouchDB")
	defer viper.Set("ledger.state.stateDatabase", "goleveldb")

	viper.Set("ledger.state.couchDBConfig.couchDBAddress", couchAddress)
	// Replace with correct username/password such as
	// admin/admin if user security is enabled on couchdb.
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 20)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)
	viper.Set("ledger.state.couchDBConfig.createGlobalChangesDB", true)

	//set the logging level to DEBUG to test debug only code
	flogging.ActivateSpec("couchdb=debug")

	// Create CouchDB definition from config parameters
	couchDBDef = GetCouchDBDefinition()

	//run the tests
	return m.Run()
}

func couchDBSetup() (addr string, cleanup func()) {
	externalCouch, set := os.LookupEnv("COUCHDB_ADDR")
	if set {
		return externalCouch, func() {}
	}

	couchDB := &runner.CouchDB{}
	if err := couchDB.Start(); err != nil {
		err := fmt.Errorf("failed to start couchDB: %s", err)
		panic(err)
	}
	return couchDB.Address(), func() { couchDB.Stop() }
}

func TestDBConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB)
	assert.NoError(t, err, "Error when trying to create database connection definition")

}

func TestDBBadConnectionDef(t *testing.T) {

	//create a new connection
	_, err := CreateConnectionDefinition(badParseConnectURL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB)
	assert.Error(t, err, "Did not receive error when trying to create database connection definition with a bad hostname")

}

func TestEncodePathElement(t *testing.T) {

	encodedString := encodePathElement("testelement")
	assert.Equal(t, "testelement", encodedString)

	encodedString = encodePathElement("test element")
	assert.Equal(t, "test%20element", encodedString)

	encodedString = encodePathElement("/test element")
	assert.Equal(t, "%2Ftest%20element", encodedString)

	encodedString = encodePathElement("/test element:")
	assert.Equal(t, "%2Ftest%20element:", encodedString)

	encodedString = encodePathElement("/test+ element:")
	assert.Equal(t, "%2Ftest%2B%20element:", encodedString)

}

func TestHealthCheck(t *testing.T) {
	client := &http.Client{}

	//Create a bad couchdb instance
	badURL := fmt.Sprintf("http://%s", couchDBDef.URL+"1") // no couch db service available at this port
	badConnectDef := CouchConnectionDef{URL: badURL, Username: "", Password: "",
		MaxRetries: 1, MaxRetriesOnStartup: 1, RequestTimeout: time.Second * 30}

	badCouchDBInstance := CouchInstance{badConnectDef, client, newStats(&disabled.Provider{})}
	err := badCouchDBInstance.HealthCheck(context.Background())
	assert.Error(t, err, "Health check should result in an error if unable to connect to couch db")
	assert.Contains(t, err.Error(), "failed to connect to couch db")

	//Create a good couchdb instance
	goodURL := fmt.Sprintf("http://%s", couchDBDef.URL) // no couch db service available at this port
	goodConnectDef := CouchConnectionDef{URL: goodURL, Username: "", Password: "",
		MaxRetries: 1, MaxRetriesOnStartup: 1, RequestTimeout: time.Second * 30}

	goodCouchDBInstance := CouchInstance{goodConnectDef, client, newStats(&disabled.Provider{})}
	err = goodCouchDBInstance.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestBadCouchDBInstance(t *testing.T) {

	//Create a bad connection definition
	badConnectDef := CouchConnectionDef{URL: badParseConnectURL, Username: "", Password: "",
		MaxRetries: 3, MaxRetriesOnStartup: 10, RequestTimeout: time.Second * 30}

	client := &http.Client{}

	//Create a bad couchdb instance
	badCouchDBInstance := CouchInstance{badConnectDef, client, newStats(&disabled.Provider{})}

	//Create a bad CouchDatabase
	badDB := CouchDatabase{&badCouchDBInstance, "baddb", 1}

	//Test CreateCouchDatabase with bad connection
	_, err := CreateCouchDatabase(&badCouchDBInstance, "baddbtest")
	assert.Error(t, err, "Error should have been thrown with CreateCouchDatabase and invalid connection")

	//Test CreateSystemDatabasesIfNotExist with bad connection
	err = CreateSystemDatabasesIfNotExist(&badCouchDBInstance)
	assert.Error(t, err, "Error should have been thrown with CreateSystemDatabasesIfNotExist and invalid connection")

	//Test CreateDatabaseIfNotExist with bad connection
	err = badDB.CreateDatabaseIfNotExist()
	assert.Error(t, err, "Error should have been thrown with CreateDatabaseIfNotExist and invalid connection")

	//Test GetDatabaseInfo with bad connection
	_, _, err = badDB.GetDatabaseInfo()
	assert.Error(t, err, "Error should have been thrown with GetDatabaseInfo and invalid connection")

	//Test VerifyCouchConfig with bad connection
	_, _, err = badCouchDBInstance.VerifyCouchConfig()
	assert.Error(t, err, "Error should have been thrown with VerifyCouchConfig and invalid connection")

	//Test EnsureFullCommit with bad connection
	_, err = badDB.EnsureFullCommit()
	assert.Error(t, err, "Error should have been thrown with EnsureFullCommit and invalid connection")

	//Test DropDatabase with bad connection
	_, err = badDB.DropDatabase()
	assert.Error(t, err, "Error should have been thrown with DropDatabase and invalid connection")

	//Test ReadDoc with bad connection
	_, _, err = badDB.ReadDoc("1")
	assert.Error(t, err, "Error should have been thrown with ReadDoc and invalid connection")

	//Test SaveDoc with bad connection
	_, err = badDB.SaveDoc("1", "1", nil)
	assert.Error(t, err, "Error should have been thrown with SaveDoc and invalid connection")

	//Test DeleteDoc with bad connection
	err = badDB.DeleteDoc("1", "1")
	assert.Error(t, err, "Error should have been thrown with DeleteDoc and invalid connection")

	//Test ReadDocRange with bad connection
	_, _, err = badDB.ReadDocRange("1", "2", 1000)
	assert.Error(t, err, "Error should have been thrown with ReadDocRange and invalid connection")

	//Test QueryDocuments with bad connection
	_, _, err = badDB.QueryDocuments("1")
	assert.Error(t, err, "Error should have been thrown with QueryDocuments and invalid connection")

	//Test BatchRetrieveDocumentMetadata with bad connection
	_, err = badDB.BatchRetrieveDocumentMetadata(nil)
	assert.Error(t, err, "Error should have been thrown with BatchRetrieveDocumentMetadata and invalid connection")

	//Test BatchUpdateDocuments with bad connection
	_, err = badDB.BatchUpdateDocuments(nil)
	assert.Error(t, err, "Error should have been thrown with BatchUpdateDocuments and invalid connection")

	//Test ListIndex with bad connection
	_, err = badDB.ListIndex()
	assert.Error(t, err, "Error should have been thrown with ListIndex and invalid connection")

	//Test CreateIndex with bad connection
	_, err = badDB.CreateIndex("")
	assert.Error(t, err, "Error should have been thrown with CreateIndex and invalid connection")

	//Test DeleteIndex with bad connection
	err = badDB.DeleteIndex("", "")
	assert.Error(t, err, "Error should have been thrown with DeleteIndex and invalid connection")

}

func TestDBCreateSaveWithoutRevision(t *testing.T) {

	database := "testdbcreatesavewithoutrevision"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Save the test document
	_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document")

}

func TestDBCreateEnsureFullCommit(t *testing.T) {

	database := "testdbensurefullcommit"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Save the test document
	_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Ensure a full commit
	_, commiterr := db.EnsureFullCommit()
	assert.NoError(t, commiterr, "Error when trying to ensure a full commit")
}

func TestDBBadDatabaseName(t *testing.T) {

	//create a new instance and database object using a valid database name mixed case
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	_, dberr := CreateCouchDatabase(couchInstance, "testDB")
	assert.Error(t, dberr, "Error should have been thrown for an invalid db name")

	//create a new instance and database object using a valid database name letters and numbers
	couchInstance, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	_, dberr = CreateCouchDatabase(couchInstance, "test132")
	assert.NoError(t, dberr, "Error when testing a valid database name")

	//create a new instance and database object using a valid database name - special characters
	couchInstance, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	_, dberr = CreateCouchDatabase(couchInstance, "test1234~!@#$%^&*()[]{}.")
	assert.Error(t, dberr, "Error should have been thrown for an invalid db name")

	//create a new instance and database object using a invalid database name - too long	/*
	couchInstance, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	_, dberr = CreateCouchDatabase(couchInstance, "a12345678901234567890123456789012345678901234"+
		"56789012345678901234567890123456789012345678901234567890123456789012345678901234567890"+
		"12345678901234567890123456789012345678901234567890123456789012345678901234567890123456"+
		"78901234567890123456789012345678901234567890")
	assert.Error(t, dberr, "Error should have been thrown for invalid database name")

}

func TestDBBadConnection(t *testing.T) {

	//create a new instance and database object
	//Limit the maxRetriesOnStartup to 3 in order to reduce time for the failure
	_, err := CreateCouchInstance(badConnectURL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, 3, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.Error(t, err, "Error should have been thrown for a bad connection")
}

func TestBadDBCredentials(t *testing.T) {

	database := "testdbbadcredentials"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	_, err = CreateCouchInstance(couchDBDef.URL, "fred", "fred",
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.Error(t, err, "Error should have been thrown for bad credentials")

}

func TestDBCreateDatabaseAndPersist(t *testing.T) {

	//Test create and persist with default configured maxRetries
	testDBCreateDatabaseAndPersist(t, couchDBDef.MaxRetries)

	//Test create and persist with 0 retries
	testDBCreateDatabaseAndPersist(t, 0)

	//Test batch operations with default configured maxRetries
	testBatchBatchOperations(t, couchDBDef.MaxRetries)

	//Test batch operations with 0 retries
	testBatchBatchOperations(t, 0)

}

func testDBCreateDatabaseAndPersist(t *testing.T, maxRetries int) {

	database := "testdbcreatedatabaseandpersist"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		maxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.GetDatabaseInfo()
	assert.NoError(t, errdb, "Error when trying to retrieve database information")
	assert.Equal(t, database, dbResp.DbName)

	//Save the test document
	_, saveerr := db.SaveDoc("idWith/slash", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Retrieve the test document
	dbGetResp, _, geterr := db.ReadDoc("idWith/slash")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//Unmarshal the document to Asset structure
	assetResp := &Asset{}
	geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//Verify the owner retrieved matches
	assert.Equal(t, "jerry", assetResp.Owner)

	//Save the test document
	_, saveerr = db.SaveDoc("1", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Retrieve the test document
	dbGetResp, _, geterr = db.ReadDoc("1")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//Unmarshal the document to Asset structure
	assetResp = &Asset{}
	geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//Verify the owner retrieved matches
	assert.Equal(t, "jerry", assetResp.Owner)

	//Change owner to bob
	assetResp.Owner = "bob"

	//create a byte array of the JSON
	assetDocUpdated, _ := json.Marshal(assetResp)

	//Save the updated test document
	_, saveerr = db.SaveDoc("1", "", &CouchDoc{JSONValue: assetDocUpdated, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save the updated document")

	//Retrieve the updated test document
	dbGetResp, _, geterr = db.ReadDoc("1")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//Unmarshal the document to Asset structure
	assetResp = &Asset{}
	json.Unmarshal(dbGetResp.JSONValue, &assetResp)

	//Assert that the update was saved and retrieved
	assert.Equal(t, "bob", assetResp.Owner)

	testBytes2 := []byte(`test attachment 2`)

	attachment2 := &AttachmentInfo{}
	attachment2.AttachmentBytes = testBytes2
	attachment2.ContentType = "application/octet-stream"
	attachment2.Name = "data"
	attachments2 := []*AttachmentInfo{}
	attachments2 = append(attachments2, attachment2)

	//Save the test document with an attachment
	_, saveerr = db.SaveDoc("2", "", &CouchDoc{JSONValue: nil, Attachments: attachments2})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Retrieve the test document with attachments
	dbGetResp, _, geterr = db.ReadDoc("2")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//verify the text from the attachment is correct
	testattach := dbGetResp.Attachments[0].AttachmentBytes
	assert.Equal(t, testBytes2, testattach)

	testBytes3 := []byte{}

	attachment3 := &AttachmentInfo{}
	attachment3.AttachmentBytes = testBytes3
	attachment3.ContentType = "application/octet-stream"
	attachment3.Name = "data"
	attachments3 := []*AttachmentInfo{}
	attachments3 = append(attachments3, attachment3)

	//Save the test document with a zero length attachment
	_, saveerr = db.SaveDoc("3", "", &CouchDoc{JSONValue: nil, Attachments: attachments3})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Retrieve the test document with attachments
	dbGetResp, _, geterr = db.ReadDoc("3")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//verify the text from the attachment is correct,  zero bytes
	testattach = dbGetResp.Attachments[0].AttachmentBytes
	assert.Equal(t, testBytes3, testattach)

	testBytes4a := []byte(`test attachment 4a`)
	attachment4a := &AttachmentInfo{}
	attachment4a.AttachmentBytes = testBytes4a
	attachment4a.ContentType = "application/octet-stream"
	attachment4a.Name = "data1"

	testBytes4b := []byte(`test attachment 4b`)
	attachment4b := &AttachmentInfo{}
	attachment4b.AttachmentBytes = testBytes4b
	attachment4b.ContentType = "application/octet-stream"
	attachment4b.Name = "data2"

	attachments4 := []*AttachmentInfo{}
	attachments4 = append(attachments4, attachment4a)
	attachments4 = append(attachments4, attachment4b)

	//Save the updated test document with multiple attachments
	_, saveerr = db.SaveDoc("4", "", &CouchDoc{JSONValue: assetJSON, Attachments: attachments4})
	assert.NoError(t, saveerr, "Error when trying to save the updated document")

	//Retrieve the test document with attachments
	dbGetResp, _, geterr = db.ReadDoc("4")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	for _, attach4 := range dbGetResp.Attachments {

		currentName := attach4.Name
		if currentName == "data1" {
			assert.Equal(t, testBytes4a, attach4.AttachmentBytes)
		}
		if currentName == "data2" {
			assert.Equal(t, testBytes4b, attach4.AttachmentBytes)
		}

	}

	testBytes5a := []byte(`test attachment 5a`)
	attachment5a := &AttachmentInfo{}
	attachment5a.AttachmentBytes = testBytes5a
	attachment5a.ContentType = "application/octet-stream"
	attachment5a.Name = "data1"

	testBytes5b := []byte{}
	attachment5b := &AttachmentInfo{}
	attachment5b.AttachmentBytes = testBytes5b
	attachment5b.ContentType = "application/octet-stream"
	attachment5b.Name = "data2"

	attachments5 := []*AttachmentInfo{}
	attachments5 = append(attachments5, attachment5a)
	attachments5 = append(attachments5, attachment5b)

	//Save the updated test document with multiple attachments and zero length attachments
	_, saveerr = db.SaveDoc("5", "", &CouchDoc{JSONValue: assetJSON, Attachments: attachments5})
	assert.NoError(t, saveerr, "Error when trying to save the updated document")

	//Retrieve the test document with attachments
	dbGetResp, _, geterr = db.ReadDoc("5")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	for _, attach5 := range dbGetResp.Attachments {

		currentName := attach5.Name
		if currentName == "data1" {
			assert.Equal(t, testBytes5a, attach5.AttachmentBytes)
		}
		if currentName == "data2" {
			assert.Equal(t, testBytes5b, attach5.AttachmentBytes)
		}

	}

	//Attempt to save the document with an invalid id
	_, saveerr = db.SaveDoc(string([]byte{0xff, 0xfe, 0xfd}), "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.Error(t, saveerr, "Error should have been thrown when saving a document with an invalid ID")

	//Attempt to read a document with an invalid id
	_, _, readerr := db.ReadDoc(string([]byte{0xff, 0xfe, 0xfd}))
	assert.Error(t, readerr, "Error should have been thrown when reading a document with an invalid ID")

	//Drop the database
	_, errdbdrop := db.DropDatabase()
	assert.NoError(t, errdbdrop, "Error dropping database")

	//Make sure an error is thrown for getting info for a missing database
	_, _, errdbinfo := db.GetDatabaseInfo()
	assert.Error(t, errdbinfo, "Error should have been thrown for missing database")

	//Attempt to save a document to a deleted database
	_, saveerr = db.SaveDoc("6", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.Error(t, saveerr, "Error should have been thrown while attempting to save to a deleted database")

	//Attempt to read from a deleted database
	_, _, geterr = db.ReadDoc("6")
	assert.NoError(t, geterr, "Error should not have been thrown for a missing database, nil value is returned")

}

func TestDBRequestTimeout(t *testing.T) {

	database := "testdbrequesttimeout"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create an impossibly short timeout
	impossibleTimeout := time.Microsecond * 1

	//create a new instance and database object with a timeout that will fail
	//Also use a maxRetriesOnStartup=3 to reduce the number of retries
	_, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, 3, impossibleTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.Error(t, err, "Error should have been thown while trying to create a couchdb instance with a connection timeout")

	//create a new instance and database object
	_, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		-1, 3, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.Error(t, err, "Error should have been thrown while attempting to create a database")

}

func TestDBTimeoutConflictRetry(t *testing.T) {

	database := "testdbtimeoutretry"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, 3, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.GetDatabaseInfo()
	assert.NoError(t, errdb, "Error when trying to retrieve database information")
	assert.Equal(t, database, dbResp.DbName)

	//Save the test document
	_, saveerr := db.SaveDoc("1", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Retrieve the test document
	_, _, geterr := db.ReadDoc("1")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//Save the test document with an invalid rev.  This should cause a retry
	_, saveerr = db.SaveDoc("1", "1-11111111111111111111111111111111", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document with a revision conflict")

	//Delete the test document with an invalid rev.  This should cause a retry
	deleteerr := db.DeleteDoc("1", "1-11111111111111111111111111111111")
	assert.NoError(t, deleteerr, "Error when trying to delete a document with a revision conflict")

}

func TestDBBadNumberOfRetries(t *testing.T) {

	database := "testdbbadretries"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	_, err = CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		-1, 3, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.Error(t, err, "Error should have been thrown while attempting to create a database")

}

func TestDBBadJSON(t *testing.T) {

	database := "testdbbadjson"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.GetDatabaseInfo()
	assert.NoError(t, errdb, "Error when trying to retrieve database information")
	assert.Equal(t, database, dbResp.DbName)

	badJSON := []byte(`{"asset_name"}`)

	//Save the test document
	_, saveerr := db.SaveDoc("1", "", &CouchDoc{JSONValue: badJSON, Attachments: nil})
	assert.Error(t, saveerr, "Error should have been thrown for a bad JSON")

}

func TestPrefixScan(t *testing.T) {

	database := "testprefixscan"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.GetDatabaseInfo()
	assert.NoError(t, errdb, "Error when trying to retrieve database information")
	assert.Equal(t, database, dbResp.DbName)

	//Save documents
	for i := 0; i < 20; i++ {
		id1 := string(0) + string(i) + string(0)
		id2 := string(0) + string(i) + string(1)
		id3 := string(0) + string(i) + string(utf8.MaxRune-1)
		_, saveerr := db.SaveDoc(id1, "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
		assert.NoError(t, saveerr, "Error when trying to save a document")
		_, saveerr = db.SaveDoc(id2, "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
		assert.NoError(t, saveerr, "Error when trying to save a document")
		_, saveerr = db.SaveDoc(id3, "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
		assert.NoError(t, saveerr, "Error when trying to save a document")

	}
	startKey := string(0) + string(10)
	endKey := startKey + string(utf8.MaxRune)
	_, _, geterr := db.ReadDoc(endKey)
	assert.NoError(t, geterr, "Error when trying to get lastkey")

	resultsPtr, _, geterr := db.ReadDocRange(startKey, endKey, 1000)
	assert.NoError(t, geterr, "Error when trying to perform a range scan")
	assert.NotNil(t, resultsPtr)
	results := resultsPtr
	assert.Equal(t, 3, len(results))
	assert.Equal(t, string(0)+string(10)+string(0), results[0].ID)
	assert.Equal(t, string(0)+string(10)+string(1), results[1].ID)
	assert.Equal(t, string(0)+string(10)+string(utf8.MaxRune-1), results[2].ID)

	//Drop the database
	_, errdbdrop := db.DropDatabase()
	assert.NoError(t, errdbdrop, "Error dropping database")

	//Retrieve the info for the new database and make sure the name matches
	_, _, errdbinfo := db.GetDatabaseInfo()
	assert.Error(t, errdbinfo, "Error should have been thrown for missing database")

}

func TestDBSaveAttachment(t *testing.T) {

	database := "testdbsaveattachment"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	byteText := []byte(`This is a test document.  This is only a test`)

	attachment := &AttachmentInfo{}
	attachment.AttachmentBytes = byteText
	attachment.ContentType = "text/plain"
	attachment.Length = uint64(len(byteText))
	attachment.Name = "valueBytes"

	attachments := []*AttachmentInfo{}
	attachments = append(attachments, attachment)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Save the test document
	_, saveerr := db.SaveDoc("10", "", &CouchDoc{JSONValue: nil, Attachments: attachments})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Attempt to retrieve the updated test document with attachments
	couchDoc, _, geterr2 := db.ReadDoc("10")
	assert.NoError(t, geterr2, "Error when trying to retrieve a document with attachment")
	assert.NotNil(t, couchDoc.Attachments)
	assert.Equal(t, byteText, couchDoc.Attachments[0].AttachmentBytes)
	assert.Equal(t, attachment.Length, couchDoc.Attachments[0].Length)

}

func TestDBDeleteDocument(t *testing.T) {

	database := "testdbdeletedocument"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Save the test document
	_, saveerr := db.SaveDoc("2", "", &CouchDoc{JSONValue: assetJSON, Attachments: nil})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Attempt to retrieve the test document
	_, _, readErr := db.ReadDoc("2")
	assert.NoError(t, readErr, "Error when trying to retrieve a document with attachment")

	//Delete the test document
	deleteErr := db.DeleteDoc("2", "")
	assert.NoError(t, deleteErr, "Error when trying to delete a document")

	//Attempt to retrieve the test document
	readValue, _, _ := db.ReadDoc("2")
	assert.Nil(t, readValue)

}

func TestDBDeleteNonExistingDocument(t *testing.T) {

	database := "testdbdeletenonexistingdocument"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Save the test document
	deleteErr := db.DeleteDoc("2", "")
	assert.NoError(t, deleteErr, "Error when trying to delete a non existing document")
}

func TestCouchDBVersion(t *testing.T) {

	err := checkCouchDBVersion("2.0.0")
	assert.NoError(t, err, "Error should not have been thrown for valid version")

	err = checkCouchDBVersion("4.5.0")
	assert.NoError(t, err, "Error should not have been thrown for valid version")

	err = checkCouchDBVersion("1.6.5.4")
	assert.Error(t, err, "Error should have been thrown for invalid version")

	err = checkCouchDBVersion("0.0.0.0")
	assert.Error(t, err, "Error should have been thrown for invalid version")

}

func TestIndexOperations(t *testing.T) {

	database := "testindexoperations"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	byteJSON1 := []byte(`{"_id":"1", "asset_name":"marble1","color":"blue","size":1,"owner":"jerry"}`)
	byteJSON2 := []byte(`{"_id":"2", "asset_name":"marble2","color":"red","size":2,"owner":"tom"}`)
	byteJSON3 := []byte(`{"_id":"3", "asset_name":"marble3","color":"green","size":3,"owner":"jerry"}`)
	byteJSON4 := []byte(`{"_id":"4", "asset_name":"marble4","color":"purple","size":4,"owner":"tom"}`)
	byteJSON5 := []byte(`{"_id":"5", "asset_name":"marble5","color":"blue","size":5,"owner":"jerry"}`)
	byteJSON6 := []byte(`{"_id":"6", "asset_name":"marble6","color":"white","size":6,"owner":"tom"}`)
	byteJSON7 := []byte(`{"_id":"7", "asset_name":"marble7","color":"white","size":7,"owner":"tom"}`)
	byteJSON8 := []byte(`{"_id":"8", "asset_name":"marble8","color":"white","size":8,"owner":"tom"}`)
	byteJSON9 := []byte(`{"_id":"9", "asset_name":"marble9","color":"white","size":9,"owner":"tom"}`)
	byteJSON10 := []byte(`{"_id":"10", "asset_name":"marble10","color":"white","size":10,"owner":"tom"}`)

	//create a new instance and database object   --------------------------------------------------------
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	batchUpdateDocs := []*CouchDoc{}

	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON1, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON2, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON3, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON4, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON5, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON6, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON7, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON8, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON9, Attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &CouchDoc{JSONValue: byteJSON10, Attachments: nil})

	_, err = db.BatchUpdateDocuments(batchUpdateDocs)
	assert.NoError(t, err, "Error adding batch of documents")

	//Create an index definition
	indexDefSize := `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortDoc", "name":"indexSizeSortName","type":"json"}`

	//Create the index
	_, err = db.CreateIndex(indexDefSize)
	assert.NoError(t, err, "Error thrown while creating an index")

	//Retrieve the list of indexes
	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err := db.ListIndex()
	assert.NoError(t, err, "Error thrown while retrieving indexes")

	//There should only be one item returned
	assert.Equal(t, 1, len(listResult))

	//Verify the returned definition
	for _, elem := range listResult {
		assert.Equal(t, "indexSizeSortDoc", elem.DesignDocument)
		assert.Equal(t, "indexSizeSortName", elem.Name)
		//ensure the index definition is correct,  CouchDB 2.1.1 will also return "partial_filter_selector":{}
		assert.Equal(t, true, strings.Contains(elem.Definition, `"fields":[{"size":"desc"}]`))
	}

	//Create an index definition with no DesignDocument or name
	indexDefColor := `{"index":{"fields":[{"color":"desc"}]}}`

	//Create the index
	_, err = db.CreateIndex(indexDefColor)
	assert.NoError(t, err, "Error thrown while creating an index")

	//Retrieve the list of indexes
	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.ListIndex()
	assert.NoError(t, err, "Error thrown while retrieving indexes")

	//There should be two indexes returned
	assert.Equal(t, 2, len(listResult))

	//Delete the named index
	err = db.DeleteIndex("indexSizeSortDoc", "indexSizeSortName")
	assert.NoError(t, err, "Error thrown while deleting an index")

	//Retrieve the list of indexes
	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.ListIndex()
	assert.NoError(t, err, "Error thrown while retrieving indexes")

	//There should be one index returned
	assert.Equal(t, 1, len(listResult))

	//Delete the unnamed index
	for _, elem := range listResult {
		err = db.DeleteIndex(elem.DesignDocument, elem.Name)
		assert.NoError(t, err, "Error thrown while deleting an index")
	}

	//Retrieve the list of indexes
	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.ListIndex()
	assert.NoError(t, err, "Error thrown while retrieving indexes")
	assert.Equal(t, 0, len(listResult))

	//Create a query string with a descending sort, this will require an index
	queryString := `{"selector":{"size": {"$gt": 0}},"fields": ["_id", "_rev", "owner", "asset_name", "color", "size"], "sort":[{"size":"desc"}], "limit": 10,"skip": 0}`

	//Execute a query with a sort, this should throw the exception
	_, _, err = db.QueryDocuments(queryString)
	assert.Error(t, err, "Error should have thrown while querying without a valid index")

	//Create the index
	_, err = db.CreateIndex(indexDefSize)
	assert.NoError(t, err, "Error thrown while creating an index")

	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)

	//Execute a query with an index,  this should succeed
	_, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error thrown while querying with an index")

	//Create another index definition
	indexDefSize = `{"index":{"fields":[{"data.size":"desc"},{"data.owner":"desc"}]},"ddoc":"indexSizeOwnerSortDoc", "name":"indexSizeOwnerSortName","type":"json"}`

	//Create the index
	dbResp, err := db.CreateIndex(indexDefSize)
	assert.NoError(t, err, "Error thrown while creating an index")

	//verify the response is "created" for an index creation
	assert.Equal(t, "created", dbResp.Result)

	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)

	//Update the index
	dbResp, err = db.CreateIndex(indexDefSize)
	assert.NoError(t, err, "Error thrown while creating an index")

	//verify the response is "exists" for an update
	assert.Equal(t, "exists", dbResp.Result)

	//Retrieve the list of indexes
	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.ListIndex()
	assert.NoError(t, err, "Error thrown while retrieving indexes")

	//There should only be two definitions
	assert.Equal(t, 2, len(listResult))

	//Create an invalid index definition with an invalid JSON
	indexDefSize = `{"index"{"fields":[{"data.size":"desc"},{"data.owner":"desc"}]},"ddoc":"indexSizeOwnerSortDoc", "name":"indexSizeOwnerSortName","type":"json"}`

	//Create the index
	_, err = db.CreateIndex(indexDefSize)
	assert.Error(t, err, "Error should have been thrown for an invalid index JSON")

	//Create an invalid index definition with a valid JSON and an invalid index definition
	indexDefSize = `{"index":{"fields2":[{"data.size":"desc"},{"data.owner":"desc"}]},"ddoc":"indexSizeOwnerSortDoc", "name":"indexSizeOwnerSortName","type":"json"}`

	//Create the index
	_, err = db.CreateIndex(indexDefSize)
	assert.Error(t, err, "Error should have been thrown for an invalid index definition")

}

func TestRichQuery(t *testing.T) {

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

	attachment1 := &AttachmentInfo{}
	attachment1.AttachmentBytes = []byte(`marble01 - test attachment`)
	attachment1.ContentType = "application/octet-stream"
	attachment1.Name = "data"
	attachments1 := []*AttachmentInfo{}
	attachments1 = append(attachments1, attachment1)

	attachment2 := &AttachmentInfo{}
	attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
	attachment2.ContentType = "application/octet-stream"
	attachment2.Name = "data"
	attachments2 := []*AttachmentInfo{}
	attachments2 = append(attachments2, attachment2)

	attachment3 := &AttachmentInfo{}
	attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
	attachment3.ContentType = "application/octet-stream"
	attachment3.Name = "data"
	attachments3 := []*AttachmentInfo{}
	attachments3 = append(attachments3, attachment3)

	attachment4 := &AttachmentInfo{}
	attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
	attachment4.ContentType = "application/octet-stream"
	attachment4.Name = "data"
	attachments4 := []*AttachmentInfo{}
	attachments4 = append(attachments4, attachment4)

	attachment5 := &AttachmentInfo{}
	attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
	attachment5.ContentType = "application/octet-stream"
	attachment5.Name = "data"
	attachments5 := []*AttachmentInfo{}
	attachments5 = append(attachments5, attachment5)

	attachment6 := &AttachmentInfo{}
	attachment6.AttachmentBytes = []byte(`marble06 - test attachment`)
	attachment6.ContentType = "application/octet-stream"
	attachment6.Name = "data"
	attachments6 := []*AttachmentInfo{}
	attachments6 = append(attachments6, attachment6)

	attachment7 := &AttachmentInfo{}
	attachment7.AttachmentBytes = []byte(`marble07 - test attachment`)
	attachment7.ContentType = "application/octet-stream"
	attachment7.Name = "data"
	attachments7 := []*AttachmentInfo{}
	attachments7 = append(attachments7, attachment7)

	attachment8 := &AttachmentInfo{}
	attachment8.AttachmentBytes = []byte(`marble08 - test attachment`)
	attachment8.ContentType = "application/octet-stream"
	attachment7.Name = "data"
	attachments8 := []*AttachmentInfo{}
	attachments8 = append(attachments8, attachment8)

	attachment9 := &AttachmentInfo{}
	attachment9.AttachmentBytes = []byte(`marble09 - test attachment`)
	attachment9.ContentType = "application/octet-stream"
	attachment9.Name = "data"
	attachments9 := []*AttachmentInfo{}
	attachments9 = append(attachments9, attachment9)

	attachment10 := &AttachmentInfo{}
	attachment10.AttachmentBytes = []byte(`marble10 - test attachment`)
	attachment10.ContentType = "application/octet-stream"
	attachment10.Name = "data"
	attachments10 := []*AttachmentInfo{}
	attachments10 = append(attachments10, attachment10)

	attachment11 := &AttachmentInfo{}
	attachment11.AttachmentBytes = []byte(`marble11 - test attachment`)
	attachment11.ContentType = "application/octet-stream"
	attachment11.Name = "data"
	attachments11 := []*AttachmentInfo{}
	attachments11 = append(attachments11, attachment11)

	attachment12 := &AttachmentInfo{}
	attachment12.AttachmentBytes = []byte(`marble12 - test attachment`)
	attachment12.ContentType = "application/octet-stream"
	attachment12.Name = "data"
	attachments12 := []*AttachmentInfo{}
	attachments12 = append(attachments12, attachment12)

	database := "testrichquery"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object   --------------------------------------------------------
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Save the test document
	_, saveerr := db.SaveDoc("marble01", "", &CouchDoc{JSONValue: byteJSON01, Attachments: attachments1})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble02", "", &CouchDoc{JSONValue: byteJSON02, Attachments: attachments2})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble03", "", &CouchDoc{JSONValue: byteJSON03, Attachments: attachments3})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble04", "", &CouchDoc{JSONValue: byteJSON04, Attachments: attachments4})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble05", "", &CouchDoc{JSONValue: byteJSON05, Attachments: attachments5})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble06", "", &CouchDoc{JSONValue: byteJSON06, Attachments: attachments6})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble07", "", &CouchDoc{JSONValue: byteJSON07, Attachments: attachments7})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble08", "", &CouchDoc{JSONValue: byteJSON08, Attachments: attachments8})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble09", "", &CouchDoc{JSONValue: byteJSON09, Attachments: attachments9})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble10", "", &CouchDoc{JSONValue: byteJSON10, Attachments: attachments10})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble11", "", &CouchDoc{JSONValue: byteJSON11, Attachments: attachments11})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Save the test document
	_, saveerr = db.SaveDoc("marble12", "", &CouchDoc{JSONValue: byteJSON12, Attachments: attachments12})
	assert.NoError(t, saveerr, "Error when trying to save a document")

	//Test query with invalid JSON -------------------------------------------------------------------
	queryString := `{"selector":{"owner":}}`

	_, _, err = db.QueryDocuments(queryString)
	assert.Error(t, err, "Error should have been thrown for bad json")

	//Test query with object  -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"jerry"}}}`

	queryResult, _, err := db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 3 results for owner="jerry"
	assert.Equal(t, 3, len(queryResult))

	//Test query with implicit operator   --------------------------------------------------------------
	queryString = `{"selector":{"owner":"jerry"}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 3 results for owner="jerry"
	assert.Equal(t, 3, len(queryResult))

	//Test query with specified fields   -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"jerry"}},"fields": ["owner","asset_name","color","size"]}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 3 results for owner="jerry"
	assert.Equal(t, 3, len(queryResult))

	//Test query with a leading operator   -------------------------------------------------------------------
	queryString = `{"selector":{"$or":[{"owner":{"$eq":"jerry"}},{"owner": {"$eq": "frank"}}]}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 4 results for owner="jerry" or owner="frank"
	assert.Equal(t, 4, len(queryResult))

	//Test query implicit and explicit operator   ------------------------------------------------------------------
	queryString = `{"selector":{"color":"green","$or":[{"owner":"tom"},{"owner":"frank"}]}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 2 results for color="green" and (owner="jerry" or owner="frank")
	assert.Equal(t, 2, len(queryResult))

	//Test query with a leading operator  -------------------------------------------------------------------------
	queryString = `{"selector":{"$and":[{"size":{"$gte":2}},{"size":{"$lte":5}}]}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 4 results for size >= 2 and size <= 5
	assert.Equal(t, 4, len(queryResult))

	//Test query with leading and embedded operator  -------------------------------------------------------------
	queryString = `{"selector":{"$and":[{"size":{"$gte":3}},{"size":{"$lte":10}},{"$not":{"size":7}}]}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 7 results for size >= 3 and size <= 10 and not 7
	assert.Equal(t, 7, len(queryResult))

	//Test query with leading operator and array of objects ----------------------------------------------------------
	queryString = `{"selector":{"$and":[{"size":{"$gte":2}},{"size":{"$lte":10}},{"$nor":[{"size":3},{"size":5},{"size":7}]}]}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 6 results for size >= 2 and size <= 10 and not 3,5 or 7
	assert.Equal(t, 6, len(queryResult))

	//Test a range query ---------------------------------------------------------------------------------------------
	queryResult, _, err = db.ReadDocRange("marble02", "marble06", 10000)
	assert.NoError(t, err, "Error when attempting to execute a range query")

	//There should be 4 results
	assert.Equal(t, 4, len(queryResult))

	//Attachments retrieved should be correct
	assert.Equal(t, attachment2.AttachmentBytes, queryResult[0].Attachments[0].AttachmentBytes)
	assert.Equal(t, attachment3.AttachmentBytes, queryResult[1].Attachments[0].AttachmentBytes)
	assert.Equal(t, attachment4.AttachmentBytes, queryResult[2].Attachments[0].AttachmentBytes)
	assert.Equal(t, attachment5.AttachmentBytes, queryResult[3].Attachments[0].AttachmentBytes)

	//Test query with for tom  -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"tom"}}}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 8 results for owner="tom"
	assert.Equal(t, 8, len(queryResult))

	//Test query with for tom with limit  -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"tom"}},"limit":2}`

	queryResult, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query")

	//There should be 2 results for owner="tom" with a limit of 2
	assert.Equal(t, 2, len(queryResult))

	//Create an index definition
	indexDefSize := `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortDoc", "name":"indexSizeSortName","type":"json"}`

	//Create the index
	_, err = db.CreateIndex(indexDefSize)
	assert.NoError(t, err, "Error thrown while creating an index")

	//Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)

	//Test query with valid index  -------------------------------------------------------------------
	queryString = `{"selector":{"size":{"$gt":0}}, "use_index":["indexSizeSortDoc","indexSizeSortName"]}`

	_, _, err = db.QueryDocuments(queryString)
	assert.NoError(t, err, "Error when attempting to execute a query with a valid index")

}

func testBatchBatchOperations(t *testing.T, maxRetries int) {

	byteJSON01 := []byte(`{"_id":"marble01","asset_name":"marble01","color":"blue","size":"1","owner":"jerry"}`)
	byteJSON02 := []byte(`{"_id":"marble02","asset_name":"marble02","color":"red","size":"2","owner":"tom"}`)
	byteJSON03 := []byte(`{"_id":"marble03","asset_name":"marble03","color":"green","size":"3","owner":"jerry"}`)
	byteJSON04 := []byte(`{"_id":"marble04","asset_name":"marble04","color":"purple","size":"4","owner":"tom"}`)
	byteJSON05 := []byte(`{"_id":"marble05","asset_name":"marble05","color":"blue","size":"5","owner":"jerry"}`)
	byteJSON06 := []byte(`{"_id":"marble06#$&'()*+,/:;=?@[]","asset_name":"marble06#$&'()*+,/:;=?@[]","color":"blue","size":"6","owner":"jerry"}`)

	attachment1 := &AttachmentInfo{}
	attachment1.AttachmentBytes = []byte(`marble01 - test attachment`)
	attachment1.ContentType = "application/octet-stream"
	attachment1.Name = "data"
	attachments1 := []*AttachmentInfo{}
	attachments1 = append(attachments1, attachment1)

	attachment2 := &AttachmentInfo{}
	attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
	attachment2.ContentType = "application/octet-stream"
	attachment2.Name = "data"
	attachments2 := []*AttachmentInfo{}
	attachments2 = append(attachments2, attachment2)

	attachment3 := &AttachmentInfo{}
	attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
	attachment3.ContentType = "application/octet-stream"
	attachment3.Name = "data"
	attachments3 := []*AttachmentInfo{}
	attachments3 = append(attachments3, attachment3)

	attachment4 := &AttachmentInfo{}
	attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
	attachment4.ContentType = "application/octet-stream"
	attachment4.Name = "data"
	attachments4 := []*AttachmentInfo{}
	attachments4 = append(attachments4, attachment4)

	attachment5 := &AttachmentInfo{}
	attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
	attachment5.ContentType = "application/octet-stream"
	attachment5.Name = "data"
	attachments5 := []*AttachmentInfo{}
	attachments5 = append(attachments5, attachment5)

	attachment6 := &AttachmentInfo{}
	attachment6.AttachmentBytes = []byte(`marble06#$&'()*+,/:;=?@[] - test attachment`)
	attachment6.ContentType = "application/octet-stream"
	attachment6.Name = "data"
	attachments6 := []*AttachmentInfo{}
	attachments6 = append(attachments6, attachment6)

	database := "testbatch"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object   --------------------------------------------------------
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	batchUpdateDocs := []*CouchDoc{}

	value1 := &CouchDoc{JSONValue: byteJSON01, Attachments: attachments1}
	value2 := &CouchDoc{JSONValue: byteJSON02, Attachments: attachments2}
	value3 := &CouchDoc{JSONValue: byteJSON03, Attachments: attachments3}
	value4 := &CouchDoc{JSONValue: byteJSON04, Attachments: attachments4}
	value5 := &CouchDoc{JSONValue: byteJSON05, Attachments: attachments5}
	value6 := &CouchDoc{JSONValue: byteJSON06, Attachments: attachments6}

	batchUpdateDocs = append(batchUpdateDocs, value1)
	batchUpdateDocs = append(batchUpdateDocs, value2)
	batchUpdateDocs = append(batchUpdateDocs, value3)
	batchUpdateDocs = append(batchUpdateDocs, value4)
	batchUpdateDocs = append(batchUpdateDocs, value5)
	batchUpdateDocs = append(batchUpdateDocs, value6)

	batchUpdateResp, err := db.BatchUpdateDocuments(batchUpdateDocs)
	assert.NoError(t, err, "Error when attempting to update a batch of documents")

	//check to make sure each batch update response was successful
	for _, updateDoc := range batchUpdateResp {
		assert.Equal(t, true, updateDoc.Ok)
	}

	//----------------------------------------------
	//Test Retrieve JSON
	dbGetResp, _, geterr := db.ReadDoc("marble01")
	assert.NoError(t, geterr, "Error when attempting read a document")

	assetResp := &Asset{}
	geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
	assert.NoError(t, geterr, "Error when trying to retrieve a document")
	//Verify the owner retrieved matches
	assert.Equal(t, "jerry", assetResp.Owner)

	//----------------------------------------------
	// Test Retrieve JSON using ID with URL special characters,
	// this will confirm that batch document IDs and URL IDs are consistent, even if they include special characters
	dbGetResp, _, geterr = db.ReadDoc("marble06#$&'()*+,/:;=?@[]")
	assert.NoError(t, geterr, "Error when attempting read a document")

	assetResp = &Asset{}
	geterr = json.Unmarshal(dbGetResp.JSONValue, &assetResp)
	assert.NoError(t, geterr, "Error when trying to retrieve a document")
	//Verify the owner retrieved matches
	assert.Equal(t, "jerry", assetResp.Owner)

	//----------------------------------------------
	//Test retrieve binary
	dbGetResp, _, geterr = db.ReadDoc("marble03")
	assert.NoError(t, geterr, "Error when attempting read a document")
	//Retrieve the attachments
	attachments := dbGetResp.Attachments
	//Only one was saved, so take the first
	retrievedAttachment := attachments[0]
	//Verify the text matches
	assert.Equal(t, retrievedAttachment.AttachmentBytes, attachment3.AttachmentBytes)
	//----------------------------------------------
	//Test Bad Updates
	batchUpdateDocs = []*CouchDoc{}
	batchUpdateDocs = append(batchUpdateDocs, value1)
	batchUpdateDocs = append(batchUpdateDocs, value2)
	batchUpdateResp, err = db.BatchUpdateDocuments(batchUpdateDocs)
	assert.NoError(t, err, "Error when attempting to update a batch of documents")
	//No revision was provided, so these two updates should fail
	//Verify that the "Ok" field is returned as false
	for _, updateDoc := range batchUpdateResp {
		assert.Equal(t, false, updateDoc.Ok)
		assert.Equal(t, updateDocumentConflictError, updateDoc.Error)
		assert.Equal(t, updateDocumentConflictReason, updateDoc.Reason)
	}

	//----------------------------------------------
	//Test Batch Retrieve Keys and Update

	var keys []string

	keys = append(keys, "marble01")
	keys = append(keys, "marble03")

	batchRevs, err := db.BatchRetrieveDocumentMetadata(keys)
	assert.NoError(t, err, "Error when attempting retrieve revisions")

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
	assert.NoError(t, err, "Error when attempting to update a batch of documents")
	//check to make sure each batch update response was successful
	for _, updateDoc := range batchUpdateResp {
		assert.Equal(t, true, updateDoc.Ok)
	}

	//----------------------------------------------
	//Test Batch Delete

	keys = []string{}

	keys = append(keys, "marble02")
	keys = append(keys, "marble04")

	batchRevs, err = db.BatchRetrieveDocumentMetadata(keys)
	assert.NoError(t, err, "Error when attempting retrieve revisions")

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
	assert.NoError(t, err, "Error when attempting to update a batch of documents")

	//check to make sure each batch update response was successful
	for _, updateDoc := range batchUpdateResp {
		assert.Equal(t, true, updateDoc.Ok)
	}

	//Retrieve the test document
	dbGetResp, _, geterr = db.ReadDoc("marble02")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//assert the value was deleted
	assert.Nil(t, dbGetResp)

	//Retrieve the test document
	dbGetResp, _, geterr = db.ReadDoc("marble04")
	assert.NoError(t, geterr, "Error when trying to retrieve a document")

	//assert the value was deleted
	assert.Nil(t, dbGetResp)

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

func TestDatabaseSecuritySettings(t *testing.T) {

	database := "testdbsecuritysettings"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	//create a new instance and database object   --------------------------------------------------------
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	//Create a database security object
	securityPermissions := &DatabaseSecurity{}
	securityPermissions.Admins.Names = append(securityPermissions.Admins.Names, "admin")
	securityPermissions.Members.Names = append(securityPermissions.Members.Names, "admin")

	//Apply security
	err = db.ApplyDatabaseSecurity(securityPermissions)
	assert.NoError(t, err, "Error when trying to apply database security")

	//Retrieve database security
	databaseSecurity, err := db.GetDatabaseSecurity()
	assert.NoError(t, err, "Error when retrieving database security")

	//Verify retrieval of admins
	assert.Equal(t, "admin", databaseSecurity.Admins.Names[0])

	//Verify retrieval of members
	assert.Equal(t, "admin", databaseSecurity.Members.Names[0])

	//Create an empty database security object
	securityPermissions = &DatabaseSecurity{}

	//Apply the security
	err = db.ApplyDatabaseSecurity(securityPermissions)
	assert.NoError(t, err, "Error when trying to apply database security")

	//Retrieve database security
	databaseSecurity, err = db.GetDatabaseSecurity()
	assert.NoError(t, err, "Error when retrieving database security")

	//Verify retrieval of admins, should be an empty array
	assert.Equal(t, 0, len(databaseSecurity.Admins.Names))

	//Verify retrieval of members, should be an empty array
	assert.Equal(t, 0, len(databaseSecurity.Members.Names))

}

func TestURLWithSpecialCharacters(t *testing.T) {

	database := "testdb+with+plus_sign"
	err := cleanup(database)
	assert.NoError(t, err, "Error when trying to cleanup  Error: %s", err)
	defer cleanup(database)

	// parse a contructed URL
	finalURL, err := url.Parse("http://127.0.0.1:5984")
	assert.NoError(t, err, "error thrown while parsing couchdb url")

	// test the constructCouchDBUrl function with multiple path elements
	couchdbURL := constructCouchDBUrl(finalURL, database, "_index", "designdoc", "json", "indexname")
	assert.Equal(t, "http://127.0.0.1:5984/testdb%2Bwith%2Bplus_sign/_index/designdoc/json/indexname", couchdbURL.String())

	//create a new instance and database object   --------------------------------------------------------
	couchInstance, err := CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB, &disabled.Provider{})
	assert.NoError(t, err, "Error when trying to create couch instance")
	db := CouchDatabase{CouchInstance: couchInstance, DBName: database}

	//create a new database
	errdb := db.CreateDatabaseIfNotExist()
	assert.NoError(t, errdb, "Error when trying to create database")

	dbInfo, _, errInfo := db.GetDatabaseInfo()
	assert.NoError(t, errInfo, "Error when trying to get database info")

	assert.Equal(t, database, dbInfo.DbName)

}
