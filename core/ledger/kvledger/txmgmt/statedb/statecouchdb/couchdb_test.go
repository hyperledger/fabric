/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/stretchr/testify/require"
)

const (
	badConnectURL                = "couchdb:5990"
	badParseConnectURL           = "http://host.com|5432"
	updateDocumentConflictError  = "conflict"
	updateDocumentConflictReason = "Document update conflict."
)

type Asset struct {
	ID        string `json:"_id"`
	Rev       string `json:"_rev"`
	AssetName string `json:"asset_name"`
	Color     string `json:"color"`
	Size      string `json:"size"`
	Owner     string `json:"owner"`
}

var assetJSON = []byte(`{"asset_name":"marble1","color":"blue","size":"35","owner":"jerry"}`)

func testConfig() *ledger.CouchDBConfig {
	return &ledger.CouchDBConfig{
		Address:               "",
		Username:              "admin",
		Password:              "adminpw",
		MaxRetries:            3,
		MaxRetriesOnStartup:   20,
		RequestTimeout:        35 * time.Second,
		CreateGlobalChangesDB: false,
	}
}

func TestDBBadConnectionDef(t *testing.T) {
	config := &ledger.CouchDBConfig{
		Address:             badParseConnectURL,
		Username:            "admin",
		Password:            "adminpw",
		MaxRetries:          3,
		MaxRetriesOnStartup: 3,
		RequestTimeout:      35 * time.Second,
	}
	_, err := createCouchInstance(config, &disabled.Provider{})
	require.Error(t, err, "Did not receive error when trying to create database connection definition with a bad hostname")
}

func TestEncodePathElement(t *testing.T) {
	encodedString := encodePathElement("testelement")
	require.Equal(t, "testelement", encodedString)

	encodedString = encodePathElement("test element")
	require.Equal(t, "test%20element", encodedString)

	encodedString = encodePathElement("/test element")
	require.Equal(t, "%2Ftest%20element", encodedString)

	encodedString = encodePathElement("/test element:")
	require.Equal(t, "%2Ftest%20element:", encodedString)

	encodedString = encodePathElement("/test+ element:")
	require.Equal(t, "%2Ftest%2B%20element:", encodedString)
}

func TestHealthCheck(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	configWithIncorrectAddress := testConfig()
	client := &http.Client{}
	badCouchDBInstance := couchInstance{
		conf:   configWithIncorrectAddress,
		client: client,
		stats:  newStats(&disabled.Provider{}),
	}
	err := badCouchDBInstance.healthCheck(context.Background())
	require.Error(t, err, "Health check should result in an error if unable to connect to couch db")
	require.Contains(t, err.Error(), "failed to connect to couch db")

	// Create a good couchdb instance
	goodCouchDBInstance := couchInstance{
		conf:   config,
		client: client,
		stats:  newStats(&disabled.Provider{}),
	}
	err = goodCouchDBInstance.healthCheck(context.Background())
	require.NoError(t, err)
}

func TestBadCouchDBInstance(t *testing.T) {
	client := &http.Client{}

	// Create a bad couchdb instance
	badCouchDBInstance := couchInstance{
		conf: &ledger.CouchDBConfig{
			Address:             badParseConnectURL,
			Username:            "admin",
			Password:            "adminpw",
			MaxRetries:          3,
			MaxRetriesOnStartup: 10,
			RequestTimeout:      30 * time.Second,
		},
		client: client,
		stats:  newStats(&disabled.Provider{}),
	}

	// Create a bad CouchDatabase
	badDB := couchDatabase{&badCouchDBInstance, "baddb"}

	// Test createCouchDatabase with bad connection
	_, err := createCouchDatabase(&badCouchDBInstance, "baddbtest")
	require.Error(t, err, "Error should have been thrown with createCouchDatabase and invalid connection")

	// Test createSystemDatabasesIfNotExist with bad connection
	err = createSystemDatabasesIfNotExist(&badCouchDBInstance)
	require.Error(t, err, "Error should have been thrown with createSystemDatabasesIfNotExist and invalid connection")

	// Test createDatabaseIfNotExist with bad connection
	err = badDB.createDatabaseIfNotExist()
	require.Error(t, err, "Error should have been thrown with createDatabaseIfNotExist and invalid connection")

	// Test getDatabaseInfo with bad connection
	_, _, err = badDB.getDatabaseInfo()
	require.Error(t, err, "Error should have been thrown with getDatabaseInfo and invalid connection")

	// Test verifyCouchConfig with bad connection
	_, _, err = badCouchDBInstance.verifyCouchConfig()
	require.Error(t, err, "Error should have been thrown with verifyCouchConfig and invalid connection")

	// Test dropDatabase with bad connection
	err = badDB.dropDatabase()
	require.Error(t, err, "Error should have been thrown with dropDatabase and invalid connection")

	// Test readDoc with bad connection
	_, _, err = badDB.readDoc("1")
	require.Error(t, err, "Error should have been thrown with readDoc and invalid connection")

	// Test saveDoc with bad connection
	_, err = badDB.saveDoc("1", "1", nil)
	require.Error(t, err, "Error should have been thrown with saveDoc and invalid connection")

	// Test deleteDoc with bad connection
	err = badDB.deleteDoc("1", "1")
	require.Error(t, err, "Error should have been thrown with deleteDoc and invalid connection")

	// Test readDocRange with bad connection
	_, _, err = badDB.readDocRange("1", "2", 1000)
	require.Error(t, err, "Error should have been thrown with readDocRange and invalid connection")

	// Test queryDocuments with bad connection
	_, _, err = badDB.queryDocuments("1")
	require.Error(t, err, "Error should have been thrown with queryDocuments and invalid connection")

	// Test batchRetrieveDocumentMetadata with bad connection
	_, err = badDB.batchRetrieveDocumentMetadata(nil)
	require.Error(t, err, "Error should have been thrown with batchRetrieveDocumentMetadata and invalid connection")

	// Test batchUpdateDocuments with bad connection
	_, err = badDB.batchUpdateDocuments(nil)
	require.Error(t, err, "Error should have been thrown with batchUpdateDocuments and invalid connection")

	// Test listIndex with bad connection
	_, err = badDB.listIndex()
	require.Error(t, err, "Error should have been thrown with listIndex and invalid connection")

	// Test createIndex with bad connection
	_, err = badDB.createIndex("")
	require.Error(t, err, "Error should have been thrown with createIndex and invalid connection")

	// Test deleteIndex with bad connection
	err = badDB.deleteIndex("", "")
	require.Error(t, err, "Error should have been thrown with deleteIndex and invalid connection")
}

func TestDBCreateSaveWithoutRevision(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbcreatesavewithoutrevision"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Save the test document
	_, saveerr := db.saveDoc("2", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document")
}

func TestDBCreateEnsureFullCommit(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbensurefullcommit"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Save the test document
	_, saveerr := db.saveDoc("2", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document")
}

func TestIsEmpty(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err)

	ignore := []string{"_global_changes", "_replicator", "_users", "fabric__internal"}
	isEmpty, err := couchInstance.isEmpty(ignore)
	require.NoError(t, err)
	require.True(t, isEmpty)

	testdbs := []string{"testdb1", "testdb2"}
	couchDBEnv.cleanup(config)

	for _, d := range testdbs {
		db := couchDatabase{couchInstance: couchInstance, dbName: d}
		require.NoError(t, db.createDatabaseIfNotExist())
	}
	isEmpty, err = couchInstance.isEmpty(ignore)
	require.NoError(t, err)
	require.False(t, isEmpty)

	ignore = append(ignore, "testdb1")
	isEmpty, err = couchInstance.isEmpty(ignore)
	require.NoError(t, err)
	require.False(t, isEmpty)

	ignore = append(ignore, "testdb2")
	isEmpty, err = couchInstance.isEmpty(ignore)
	require.NoError(t, err)
	require.True(t, isEmpty)

	configCopy := *config
	configCopy.Address = "address-and-port.invalid:0"
	configCopy.MaxRetries = 0
	couchInstance.conf = &configCopy
	_, err = couchInstance.isEmpty(ignore)
	require.Error(t, err)
	require.Regexp(t, `unable to connect to CouchDB, check the hostname and port: http error calling couchdb: Get "?http://address-and-port.invalid:0/_all_dbs"?`, err.Error())
}

func TestDBBadDatabaseName(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	// create a new instance and database object using a valid database name mixed case
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	_, dberr := createCouchDatabase(couchInstance, "testDB")
	require.Error(t, dberr, "Error should have been thrown for an invalid db name")

	// create a new instance and database object using a valid database name letters and numbers
	couchInstance, err = createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	_, dberr = createCouchDatabase(couchInstance, "test132")
	require.NoError(t, dberr, "Error when testing a valid database name")

	// create a new instance and database object using a valid database name - special characters
	couchInstance, err = createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	_, dberr = createCouchDatabase(couchInstance, "test1234~!@#$%^&*()[]{}.")
	require.Error(t, dberr, "Error should have been thrown for an invalid db name")

	// create a new instance and database object using a invalid database name - too long	/*
	couchInstance, err = createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	_, dberr = createCouchDatabase(couchInstance, "a12345678901234567890123456789012345678901234"+
		"56789012345678901234567890123456789012345678901234567890123456789012345678901234567890"+
		"12345678901234567890123456789012345678901234567890123456789012345678901234567890123456"+
		"78901234567890123456789012345678901234567890")
	require.Error(t, dberr, "Error should have been thrown for invalid database name")
}

func TestDBBadConnection(t *testing.T) {
	// create a new instance and database object
	// Limit the maxRetriesOnStartup to 3 in order to reduce time for the failure
	config := &ledger.CouchDBConfig{
		Address:             badConnectURL,
		Username:            "admin",
		Password:            "adminpw",
		MaxRetries:          3,
		MaxRetriesOnStartup: 3,
		RequestTimeout:      35 * time.Second,
	}
	_, err := createCouchInstance(config, &disabled.Provider{})
	require.Error(t, err, "Error should have been thrown for a bad connection")
}

func TestBadDBCredentials(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	badConfig := testConfig()
	badConfig.Address = config.Address
	badConfig.Username = "fred"
	badConfig.Password = "fred"
	// create a new instance and database object
	_, err := createCouchInstance(badConfig, &disabled.Provider{})
	require.Error(t, err, "Error should have been thrown for bad credentials")
}

func TestDBCreateDatabaseAndPersist(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	// Test create and persist with default configured maxRetries
	testDBCreateDatabaseAndPersist(t, config)
	couchDBEnv.cleanup(config)

	// Test create and persist with 0 retries
	configCopy := *config
	configCopy.MaxRetries = 0
	testDBCreateDatabaseAndPersist(t, &configCopy)
	couchDBEnv.cleanup(config)

	// Test batch operations with default configured maxRetries
	testBatchBatchOperations(t, config)
	couchDBEnv.cleanup(config)

	// Test batch operations with 0 retries
	testBatchBatchOperations(t, config)
}

func testDBCreateDatabaseAndPersist(t *testing.T, config *ledger.CouchDBConfig) {
	database := "testdbcreatedatabaseandpersist"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.getDatabaseInfo()
	require.NoError(t, errdb, "Error when trying to retrieve database information")
	require.Equal(t, database, dbResp.DbName)

	// Save the test document
	_, saveerr := db.saveDoc("idWith/slash", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Retrieve the test document
	dbGetResp, _, geterr := db.readDoc("idWith/slash")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// Unmarshal the document to Asset structure
	assetResp := &Asset{}
	geterr = json.Unmarshal(dbGetResp.jsonValue, &assetResp)
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// Verify the owner retrieved matches
	require.Equal(t, "jerry", assetResp.Owner)

	// Save the test document
	_, saveerr = db.saveDoc("1", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Retrieve the test document
	dbGetResp, _, geterr = db.readDoc("1")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// Unmarshal the document to Asset structure
	assetResp = &Asset{}
	geterr = json.Unmarshal(dbGetResp.jsonValue, &assetResp)
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// Verify the owner retrieved matches
	require.Equal(t, "jerry", assetResp.Owner)

	// Change owner to bob
	assetResp.Owner = "bob"

	// create a byte array of the JSON
	assetDocUpdated, _ := json.Marshal(assetResp)

	// Save the updated test document
	_, saveerr = db.saveDoc("1", "", &couchDoc{jsonValue: assetDocUpdated, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save the updated document")

	// Retrieve the updated test document
	dbGetResp, _, geterr = db.readDoc("1")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// Unmarshal the document to Asset structure
	assetResp = &Asset{}
	require.NoError(t, json.Unmarshal(dbGetResp.jsonValue, &assetResp))

	// Assert that the update was saved and retrieved
	require.Equal(t, "bob", assetResp.Owner)

	testBytes2 := []byte(`test attachment 2`)

	attachment2 := &attachmentInfo{}
	attachment2.AttachmentBytes = testBytes2
	attachment2.ContentType = "application/octet-stream"
	attachment2.Name = "data"
	attachments2 := []*attachmentInfo{}
	attachments2 = append(attachments2, attachment2)

	// Save the test document with an attachment
	_, saveerr = db.saveDoc("2", "", &couchDoc{jsonValue: nil, attachments: attachments2})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Retrieve the test document with attachments
	dbGetResp, _, geterr = db.readDoc("2")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// verify the text from the attachment is correct
	testattach := dbGetResp.attachments[0].AttachmentBytes
	require.Equal(t, testBytes2, testattach)

	testBytes3 := []byte{}

	attachment3 := &attachmentInfo{}
	attachment3.AttachmentBytes = testBytes3
	attachment3.ContentType = "application/octet-stream"
	attachment3.Name = "data"
	attachments3 := []*attachmentInfo{}
	attachments3 = append(attachments3, attachment3)

	// Save the test document with a zero length attachment
	_, saveerr = db.saveDoc("3", "", &couchDoc{jsonValue: nil, attachments: attachments3})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Retrieve the test document with attachments
	dbGetResp, _, geterr = db.readDoc("3")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// verify the text from the attachment is correct,  zero bytes
	testattach = dbGetResp.attachments[0].AttachmentBytes
	require.Equal(t, testBytes3, testattach)

	testBytes4a := []byte(`test attachment 4a`)
	attachment4a := &attachmentInfo{}
	attachment4a.AttachmentBytes = testBytes4a
	attachment4a.ContentType = "application/octet-stream"
	attachment4a.Name = "data1"

	testBytes4b := []byte(`test attachment 4b`)
	attachment4b := &attachmentInfo{}
	attachment4b.AttachmentBytes = testBytes4b
	attachment4b.ContentType = "application/octet-stream"
	attachment4b.Name = "data2"

	attachments4 := []*attachmentInfo{}
	attachments4 = append(attachments4, attachment4a)
	attachments4 = append(attachments4, attachment4b)

	// Save the updated test document with multiple attachments
	_, saveerr = db.saveDoc("4", "", &couchDoc{jsonValue: assetJSON, attachments: attachments4})
	require.NoError(t, saveerr, "Error when trying to save the updated document")

	// Retrieve the test document with attachments
	dbGetResp, _, geterr = db.readDoc("4")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	for _, attach4 := range dbGetResp.attachments {

		currentName := attach4.Name
		if currentName == "data1" {
			require.Equal(t, testBytes4a, attach4.AttachmentBytes)
		}
		if currentName == "data2" {
			require.Equal(t, testBytes4b, attach4.AttachmentBytes)
		}

	}

	testBytes5a := []byte(`test attachment 5a`)
	attachment5a := &attachmentInfo{}
	attachment5a.AttachmentBytes = testBytes5a
	attachment5a.ContentType = "application/octet-stream"
	attachment5a.Name = "data1"

	testBytes5b := []byte{}
	attachment5b := &attachmentInfo{}
	attachment5b.AttachmentBytes = testBytes5b
	attachment5b.ContentType = "application/octet-stream"
	attachment5b.Name = "data2"

	attachments5 := []*attachmentInfo{}
	attachments5 = append(attachments5, attachment5a)
	attachments5 = append(attachments5, attachment5b)

	// Save the updated test document with multiple attachments and zero length attachments
	_, saveerr = db.saveDoc("5", "", &couchDoc{jsonValue: assetJSON, attachments: attachments5})
	require.NoError(t, saveerr, "Error when trying to save the updated document")

	// Retrieve the test document with attachments
	dbGetResp, _, geterr = db.readDoc("5")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	for _, attach5 := range dbGetResp.attachments {

		currentName := attach5.Name
		if currentName == "data1" {
			require.Equal(t, testBytes5a, attach5.AttachmentBytes)
		}
		if currentName == "data2" {
			require.Equal(t, testBytes5b, attach5.AttachmentBytes)
		}

	}

	// Attempt to save the document with an invalid id
	_, saveerr = db.saveDoc(string([]byte{0xff, 0xfe, 0xfd}), "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.Error(t, saveerr, "Error should have been thrown when saving a document with an invalid ID")

	// Attempt to read a document with an invalid id
	_, _, readerr := db.readDoc(string([]byte{0xff, 0xfe, 0xfd}))
	require.Error(t, readerr, "Error should have been thrown when reading a document with an invalid ID")

	// Drop the database
	errdbdrop := db.dropDatabase()
	require.NoError(t, errdbdrop, "Error dropping database")

	// Make sure an error is thrown for getting info for a missing database
	_, _, errdbinfo := db.getDatabaseInfo()
	require.Error(t, errdbinfo, "Error should have been thrown for missing database")

	// Attempt to save a document to a deleted database
	_, saveerr = db.saveDoc("6", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.Error(t, saveerr, "Error should have been thrown while attempting to save to a deleted database")

	// Attempt to read from a deleted database
	_, _, geterr = db.readDoc("6")
	require.NoError(t, geterr, "Error should not have been thrown for a missing database, nil value is returned")
}

func TestDBRequestTimeout(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	// create a new instance and database object with a timeout that will fail
	// Also use a maxRetriesOnStartup=3 to reduce the number of retries
	configCopy := *config
	configCopy.MaxRetriesOnStartup = 3
	// create an impossibly short timeout
	impossibleTimeout := time.Nanosecond
	configCopy.RequestTimeout = impossibleTimeout
	_, err := createCouchInstance(&configCopy, &disabled.Provider{})
	require.Error(t, err, "Error should have been thown while trying to create a couchdb instance with a connection timeout")

	// create a new instance and database object
	configCopy.MaxRetries = -1
	configCopy.MaxRetriesOnStartup = 3
	_, err = createCouchInstance(&configCopy, &disabled.Provider{})
	require.Error(t, err, "Error should have been thrown while attempting to create a database")
}

func TestDBTimeoutConflictRetry(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbtimeoutretry"

	// create a new instance and database object
	configCopy := *config
	configCopy.MaxRetriesOnStartup = 3
	couchInstance, err := createCouchInstance(&configCopy, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.getDatabaseInfo()
	require.NoError(t, errdb, "Error when trying to retrieve database information")
	require.Equal(t, database, dbResp.DbName)

	// Save the test document
	_, saveerr := db.saveDoc("1", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Retrieve the test document
	_, _, geterr := db.readDoc("1")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// Save the test document with an invalid rev.  This should cause a retry
	_, saveerr = db.saveDoc("1", "1-11111111111111111111111111111111", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document with a revision conflict")

	// Delete the test document with an invalid rev.  This should cause a retry
	deleteerr := db.deleteDoc("1", "1-11111111111111111111111111111111")
	require.NoError(t, deleteerr, "Error when trying to delete a document with a revision conflict")
}

func TestDBBadNumberOfRetries(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	// create a new instance and database object
	configCopy := *config
	configCopy.MaxRetries = -1
	configCopy.MaxRetriesOnStartup = 3
	_, err := createCouchInstance(&configCopy, &disabled.Provider{})
	require.Error(t, err, "Error should have been thrown while attempting to create a database")
}

func TestDBBadJSON(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbbadjson"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.getDatabaseInfo()
	require.NoError(t, errdb, "Error when trying to retrieve database information")
	require.Equal(t, database, dbResp.DbName)

	badJSON := []byte(`{"asset_name"}`)

	// Save the test document
	_, saveerr := db.saveDoc("1", "", &couchDoc{jsonValue: badJSON, attachments: nil})
	require.Error(t, saveerr, "Error should have been thrown for a bad JSON")
}

func TestPrefixScan(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testprefixscan"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Retrieve the info for the new database and make sure the name matches
	dbResp, _, errdb := db.getDatabaseInfo()
	require.NoError(t, errdb, "Error when trying to retrieve database information")
	require.Equal(t, database, dbResp.DbName)

	// Save documents
	for i := rune(0); i < 20; i++ {
		id1 := string([]rune{0, i, 0})
		id2 := string([]rune{0, i, 1})
		id3 := string([]rune{0, i, utf8.MaxRune - 1})
		_, saveerr := db.saveDoc(id1, "", &couchDoc{jsonValue: assetJSON, attachments: nil})
		require.NoError(t, saveerr, "Error when trying to save a document")
		_, saveerr = db.saveDoc(id2, "", &couchDoc{jsonValue: assetJSON, attachments: nil})
		require.NoError(t, saveerr, "Error when trying to save a document")
		_, saveerr = db.saveDoc(id3, "", &couchDoc{jsonValue: assetJSON, attachments: nil})
		require.NoError(t, saveerr, "Error when trying to save a document")

	}
	startKey := string([]rune{0, 10})
	endKey := startKey + string(utf8.MaxRune)
	_, _, geterr := db.readDoc(endKey)
	require.NoError(t, geterr, "Error when trying to get lastkey")

	resultsPtr, _, geterr := db.readDocRange(startKey, endKey, 1000)
	require.NoError(t, geterr, "Error when trying to perform a range scan")
	require.NotNil(t, resultsPtr)
	results := resultsPtr
	require.Equal(t, 3, len(results))
	require.Equal(t, string([]rune{0, 10, 0}), results[0].id)
	require.Equal(t, string([]rune{0, 10, 1}), results[1].id)
	require.Equal(t, string([]rune{0, 10, utf8.MaxRune - 1}), results[2].id)

	// Drop the database
	errdbdrop := db.dropDatabase()
	require.NoError(t, errdbdrop, "Error dropping database")

	// Drop again is not an error
	require.NoError(t, db.dropDatabase())

	// Retrieve the info for the new database and make sure the name matches
	_, _, errdbinfo := db.getDatabaseInfo()
	require.Error(t, errdbinfo, "Error should have been thrown for missing database")
}

func TestDBSaveAttachment(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbsaveattachment"

	byteText := []byte(`This is a test document.  This is only a test`)

	attachment := &attachmentInfo{}
	attachment.AttachmentBytes = byteText
	attachment.ContentType = "text/plain"
	attachment.Length = uint64(len(byteText))
	attachment.Name = "valueBytes"

	attachments := []*attachmentInfo{}
	attachments = append(attachments, attachment)

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Save the test document
	_, saveerr := db.saveDoc("10", "", &couchDoc{jsonValue: nil, attachments: attachments})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Attempt to retrieve the updated test document with attachments
	couchDoc, _, geterr2 := db.readDoc("10")
	require.NoError(t, geterr2, "Error when trying to retrieve a document with attachment")
	require.NotNil(t, couchDoc.attachments)
	require.Equal(t, byteText, couchDoc.attachments[0].AttachmentBytes)
	require.Equal(t, attachment.Length, couchDoc.attachments[0].Length)
}

func TestDBDeleteDocument(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbdeletedocument"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Save the test document
	_, saveerr := db.saveDoc("2", "", &couchDoc{jsonValue: assetJSON, attachments: nil})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Attempt to retrieve the test document
	_, _, readErr := db.readDoc("2")
	require.NoError(t, readErr, "Error when trying to retrieve a document with attachment")

	// Delete the test document
	deleteErr := db.deleteDoc("2", "")
	require.NoError(t, deleteErr, "Error when trying to delete a document")

	// Attempt to retrieve the test document
	readValue, _, _ := db.readDoc("2")
	require.Nil(t, readValue)
}

func TestDBDeleteNonExistingDocument(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbdeletenonexistingdocument"

	// create a new instance and database object
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Save the test document
	deleteErr := db.deleteDoc("2", "")
	require.NoError(t, deleteErr, "Error when trying to delete a non existing document")
}

func TestCouchDBVersion(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)

	err := checkCouchDBVersion("2.0.0")
	require.NoError(t, err, "Error should not have been thrown for valid version")

	err = checkCouchDBVersion("4.5.0")
	require.NoError(t, err, "Error should not have been thrown for valid version")

	err = checkCouchDBVersion("1.6.5.4")
	require.Error(t, err, "Error should have been thrown for invalid version")

	err = checkCouchDBVersion("0.0.0.0")
	require.Error(t, err, "Error should have been thrown for invalid version")
}

func TestIndexOperations(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testindexoperations"

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

	// create a new instance and database object   --------------------------------------------------------
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	batchUpdateDocs := []*couchDoc{}

	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON1, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON2, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON3, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON4, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON5, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON6, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON7, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON8, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON9, attachments: nil})
	batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: byteJSON10, attachments: nil})

	_, err = db.batchUpdateDocuments(batchUpdateDocs)
	require.NoError(t, err, "Error adding batch of documents")

	// Create an index definition
	indexDefSize := `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortDoc", "name":"indexSizeSortName","type":"json"}`

	// Create the index
	_, err = db.createIndex(indexDefSize)
	require.NoError(t, err, "Error thrown while creating an index")

	// Retrieve the list of indexes
	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err := db.listIndex()
	require.NoError(t, err, "Error thrown while retrieving indexes")

	// There should only be one item returned
	require.Equal(t, 1, len(listResult))

	// Verify the returned definition
	for _, elem := range listResult {
		require.Equal(t, "indexSizeSortDoc", elem.DesignDocument)
		require.Equal(t, "indexSizeSortName", elem.Name)
		// ensure the index definition is correct,  CouchDB 2.1.1 will also return "partial_filter_selector":{}
		require.Equal(t, true, strings.Contains(elem.Definition, `"fields":[{"size":"desc"}]`))
	}

	// Create an index definition with no DesignDocument or name
	indexDefColor := `{"index":{"fields":[{"color":"desc"}]}}`

	// Create the index
	_, err = db.createIndex(indexDefColor)
	require.NoError(t, err, "Error thrown while creating an index")

	// Retrieve the list of indexes
	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.listIndex()
	require.NoError(t, err, "Error thrown while retrieving indexes")

	// There should be two indexes returned
	require.Equal(t, 2, len(listResult))

	// Delete the named index
	err = db.deleteIndex("indexSizeSortDoc", "indexSizeSortName")
	require.NoError(t, err, "Error thrown while deleting an index")

	// Retrieve the list of indexes
	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.listIndex()
	require.NoError(t, err, "Error thrown while retrieving indexes")

	// There should be one index returned
	require.Equal(t, 1, len(listResult))

	// Delete the unnamed index
	for _, elem := range listResult {
		err = db.deleteIndex(elem.DesignDocument, elem.Name)
		require.NoError(t, err, "Error thrown while deleting an index")
	}

	// Retrieve the list of indexes
	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.listIndex()
	require.NoError(t, err, "Error thrown while retrieving indexes")
	require.Equal(t, 0, len(listResult))

	// Create a query string with a descending sort, this will require an index
	queryString := `{"selector":{"size": {"$gt": 0}},"fields": ["_id", "_rev", "owner", "asset_name", "color", "size"], "sort":[{"size":"desc"}], "limit": 10,"skip": 0}`

	// Execute a query with a sort, this should throw the exception
	_, _, err = db.queryDocuments(queryString)
	require.Error(t, err, "Error should have thrown while querying without a valid index")

	// Create the index
	_, err = db.createIndex(indexDefSize)
	require.NoError(t, err, "Error thrown while creating an index")

	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)

	// Execute a query with an index,  this should succeed
	_, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error thrown while querying with an index")

	// Create another index definition
	indexDefSize = `{"index":{"fields":[{"data.size":"desc"},{"data.owner":"desc"}]},"ddoc":"indexSizeOwnerSortDoc", "name":"indexSizeOwnerSortName","type":"json"}`

	// Create the index
	dbResp, err := db.createIndex(indexDefSize)
	require.NoError(t, err, "Error thrown while creating an index")

	// verify the response is "created" for an index creation
	require.Equal(t, "created", dbResp.Result)

	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)

	// Update the index
	dbResp, err = db.createIndex(indexDefSize)
	require.NoError(t, err, "Error thrown while creating an index")

	// verify the response is "exists" for an update
	require.Equal(t, "exists", dbResp.Result)

	// Retrieve the list of indexes
	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)
	listResult, err = db.listIndex()
	require.NoError(t, err, "Error thrown while retrieving indexes")

	// There should only be two definitions
	require.Equal(t, 2, len(listResult))

	// Create an invalid index definition with an invalid JSON
	indexDefSize = `{"index"{"fields":[{"data.size":"desc"},{"data.owner":"desc"}]},"ddoc":"indexSizeOwnerSortDoc", "name":"indexSizeOwnerSortName","type":"json"}`

	// Create the index
	_, err = db.createIndex(indexDefSize)
	require.Error(t, err, "Error should have been thrown for an invalid index JSON")

	// Create an invalid index definition with a valid JSON and an invalid index definition
	indexDefSize = `{"index":{"fields2":[{"data.size":"desc"},{"data.owner":"desc"}]},"ddoc":"indexSizeOwnerSortDoc", "name":"indexSizeOwnerSortName","type":"json"}`

	// Create the index
	_, err = db.createIndex(indexDefSize)
	require.Error(t, err, "Error should have been thrown for an invalid index definition")
}

func TestRichQuery(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
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

	attachment1 := &attachmentInfo{}
	attachment1.AttachmentBytes = []byte(`marble01 - test attachment`)
	attachment1.ContentType = "application/octet-stream"
	attachment1.Name = "data"
	attachments1 := []*attachmentInfo{}
	attachments1 = append(attachments1, attachment1)

	attachment2 := &attachmentInfo{}
	attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
	attachment2.ContentType = "application/octet-stream"
	attachment2.Name = "data"
	attachments2 := []*attachmentInfo{}
	attachments2 = append(attachments2, attachment2)

	attachment3 := &attachmentInfo{}
	attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
	attachment3.ContentType = "application/octet-stream"
	attachment3.Name = "data"
	attachments3 := []*attachmentInfo{}
	attachments3 = append(attachments3, attachment3)

	attachment4 := &attachmentInfo{}
	attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
	attachment4.ContentType = "application/octet-stream"
	attachment4.Name = "data"
	attachments4 := []*attachmentInfo{}
	attachments4 = append(attachments4, attachment4)

	attachment5 := &attachmentInfo{}
	attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
	attachment5.ContentType = "application/octet-stream"
	attachment5.Name = "data"
	attachments5 := []*attachmentInfo{}
	attachments5 = append(attachments5, attachment5)

	attachment6 := &attachmentInfo{}
	attachment6.AttachmentBytes = []byte(`marble06 - test attachment`)
	attachment6.ContentType = "application/octet-stream"
	attachment6.Name = "data"
	attachments6 := []*attachmentInfo{}
	attachments6 = append(attachments6, attachment6)

	attachment7 := &attachmentInfo{}
	attachment7.AttachmentBytes = []byte(`marble07 - test attachment`)
	attachment7.ContentType = "application/octet-stream"
	attachment7.Name = "data"
	attachments7 := []*attachmentInfo{}
	attachments7 = append(attachments7, attachment7)

	attachment8 := &attachmentInfo{}
	attachment8.AttachmentBytes = []byte(`marble08 - test attachment`)
	attachment8.ContentType = "application/octet-stream"
	attachment7.Name = "data"
	attachments8 := []*attachmentInfo{}
	attachments8 = append(attachments8, attachment8)

	attachment9 := &attachmentInfo{}
	attachment9.AttachmentBytes = []byte(`marble09 - test attachment`)
	attachment9.ContentType = "application/octet-stream"
	attachment9.Name = "data"
	attachments9 := []*attachmentInfo{}
	attachments9 = append(attachments9, attachment9)

	attachment10 := &attachmentInfo{}
	attachment10.AttachmentBytes = []byte(`marble10 - test attachment`)
	attachment10.ContentType = "application/octet-stream"
	attachment10.Name = "data"
	attachments10 := []*attachmentInfo{}
	attachments10 = append(attachments10, attachment10)

	attachment11 := &attachmentInfo{}
	attachment11.AttachmentBytes = []byte(`marble11 - test attachment`)
	attachment11.ContentType = "application/octet-stream"
	attachment11.Name = "data"
	attachments11 := []*attachmentInfo{}
	attachments11 = append(attachments11, attachment11)

	attachment12 := &attachmentInfo{}
	attachment12.AttachmentBytes = []byte(`marble12 - test attachment`)
	attachment12.ContentType = "application/octet-stream"
	attachment12.Name = "data"
	attachments12 := []*attachmentInfo{}
	attachments12 = append(attachments12, attachment12)

	database := "testrichquery"

	// create a new instance and database object   --------------------------------------------------------
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Save the test document
	_, saveerr := db.saveDoc("marble01", "", &couchDoc{jsonValue: byteJSON01, attachments: attachments1})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble02", "", &couchDoc{jsonValue: byteJSON02, attachments: attachments2})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble03", "", &couchDoc{jsonValue: byteJSON03, attachments: attachments3})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble04", "", &couchDoc{jsonValue: byteJSON04, attachments: attachments4})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble05", "", &couchDoc{jsonValue: byteJSON05, attachments: attachments5})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble06", "", &couchDoc{jsonValue: byteJSON06, attachments: attachments6})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble07", "", &couchDoc{jsonValue: byteJSON07, attachments: attachments7})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble08", "", &couchDoc{jsonValue: byteJSON08, attachments: attachments8})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble09", "", &couchDoc{jsonValue: byteJSON09, attachments: attachments9})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble10", "", &couchDoc{jsonValue: byteJSON10, attachments: attachments10})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble11", "", &couchDoc{jsonValue: byteJSON11, attachments: attachments11})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Save the test document
	_, saveerr = db.saveDoc("marble12", "", &couchDoc{jsonValue: byteJSON12, attachments: attachments12})
	require.NoError(t, saveerr, "Error when trying to save a document")

	// Test query with invalid JSON -------------------------------------------------------------------
	queryString := `{"selector":{"owner":}}`

	_, _, err = db.queryDocuments(queryString)
	require.Error(t, err, "Error should have been thrown for bad json")

	// Test query with object  -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"jerry"}}}`

	queryResult, _, err := db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 3 results for owner="jerry"
	require.Equal(t, 3, len(queryResult))

	// Test query with implicit operator   --------------------------------------------------------------
	queryString = `{"selector":{"owner":"jerry"}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 3 results for owner="jerry"
	require.Equal(t, 3, len(queryResult))

	// Test query with specified fields   -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"jerry"}},"fields": ["owner","asset_name","color","size"]}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 3 results for owner="jerry"
	require.Equal(t, 3, len(queryResult))

	// Test query with a leading operator   -------------------------------------------------------------------
	queryString = `{"selector":{"$or":[{"owner":{"$eq":"jerry"}},{"owner": {"$eq": "frank"}}]}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 4 results for owner="jerry" or owner="frank"
	require.Equal(t, 4, len(queryResult))

	// Test query implicit and explicit operator   ------------------------------------------------------------------
	queryString = `{"selector":{"color":"green","$or":[{"owner":"tom"},{"owner":"frank"}]}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 2 results for color="green" and (owner="jerry" or owner="frank")
	require.Equal(t, 2, len(queryResult))

	// Test query with a leading operator  -------------------------------------------------------------------------
	queryString = `{"selector":{"$and":[{"size":{"$gte":2}},{"size":{"$lte":5}}]}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 4 results for size >= 2 and size <= 5
	require.Equal(t, 4, len(queryResult))

	// Test query with leading and embedded operator  -------------------------------------------------------------
	queryString = `{"selector":{"$and":[{"size":{"$gte":3}},{"size":{"$lte":10}},{"$not":{"size":7}}]}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 7 results for size >= 3 and size <= 10 and not 7
	require.Equal(t, 7, len(queryResult))

	// Test query with leading operator and array of objects ----------------------------------------------------------
	queryString = `{"selector":{"$and":[{"size":{"$gte":2}},{"size":{"$lte":10}},{"$nor":[{"size":3},{"size":5},{"size":7}]}]}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 6 results for size >= 2 and size <= 10 and not 3,5 or 7
	require.Equal(t, 6, len(queryResult))

	// Test a range query ---------------------------------------------------------------------------------------------
	queryResult, _, err = db.readDocRange("marble02", "marble06", 10000)
	require.NoError(t, err, "Error when attempting to execute a range query")

	// There should be 4 results
	require.Equal(t, 4, len(queryResult))

	// Attachments retrieved should be correct
	require.Equal(t, attachment2.AttachmentBytes, queryResult[0].attachments[0].AttachmentBytes)
	require.Equal(t, attachment3.AttachmentBytes, queryResult[1].attachments[0].AttachmentBytes)
	require.Equal(t, attachment4.AttachmentBytes, queryResult[2].attachments[0].AttachmentBytes)
	require.Equal(t, attachment5.AttachmentBytes, queryResult[3].attachments[0].AttachmentBytes)

	// Test query with for tom  -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"tom"}}}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 8 results for owner="tom"
	require.Equal(t, 8, len(queryResult))

	// Test query with for tom with limit  -------------------------------------------------------------------
	queryString = `{"selector":{"owner":{"$eq":"tom"}},"limit":2}`

	queryResult, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query")

	// There should be 2 results for owner="tom" with a limit of 2
	require.Equal(t, 2, len(queryResult))

	// Create an index definition
	indexDefSize := `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortDoc", "name":"indexSizeSortName","type":"json"}`

	// Create the index
	_, err = db.createIndex(indexDefSize)
	require.NoError(t, err, "Error thrown while creating an index")

	// Delay for 100ms since CouchDB index list is updated async after index create/drop
	time.Sleep(100 * time.Millisecond)

	// Test query with valid index  -------------------------------------------------------------------
	queryString = `{"selector":{"size":{"$gt":0}}, "use_index":["indexSizeSortDoc","indexSizeSortName"]}`

	_, _, err = db.queryDocuments(queryString)
	require.NoError(t, err, "Error when attempting to execute a query with a valid index")
}

func testBatchBatchOperations(t *testing.T, config *ledger.CouchDBConfig) {
	byteJSON01 := []byte(`{"_id":"marble01","asset_name":"marble01","color":"blue","size":"1","owner":"jerry"}`)
	byteJSON02 := []byte(`{"_id":"marble02","asset_name":"marble02","color":"red","size":"2","owner":"tom"}`)
	byteJSON03 := []byte(`{"_id":"marble03","asset_name":"marble03","color":"green","size":"3","owner":"jerry"}`)
	byteJSON04 := []byte(`{"_id":"marble04","asset_name":"marble04","color":"purple","size":"4","owner":"tom"}`)
	byteJSON05 := []byte(`{"_id":"marble05","asset_name":"marble05","color":"blue","size":"5","owner":"jerry"}`)
	byteJSON06 := []byte(`{"_id":"marble06#$&'()*+,/:;=?@[]","asset_name":"marble06#$&'()*+,/:;=?@[]","color":"blue","size":"6","owner":"jerry"}`)

	attachment1 := &attachmentInfo{}
	attachment1.AttachmentBytes = []byte(`marble01 - test attachment`)
	attachment1.ContentType = "application/octet-stream"
	attachment1.Name = "data"
	attachments1 := []*attachmentInfo{}
	attachments1 = append(attachments1, attachment1)

	attachment2 := &attachmentInfo{}
	attachment2.AttachmentBytes = []byte(`marble02 - test attachment`)
	attachment2.ContentType = "application/octet-stream"
	attachment2.Name = "data"
	attachments2 := []*attachmentInfo{}
	attachments2 = append(attachments2, attachment2)

	attachment3 := &attachmentInfo{}
	attachment3.AttachmentBytes = []byte(`marble03 - test attachment`)
	attachment3.ContentType = "application/octet-stream"
	attachment3.Name = "data"
	attachments3 := []*attachmentInfo{}
	attachments3 = append(attachments3, attachment3)

	attachment4 := &attachmentInfo{}
	attachment4.AttachmentBytes = []byte(`marble04 - test attachment`)
	attachment4.ContentType = "application/octet-stream"
	attachment4.Name = "data"
	attachments4 := []*attachmentInfo{}
	attachments4 = append(attachments4, attachment4)

	attachment5 := &attachmentInfo{}
	attachment5.AttachmentBytes = []byte(`marble05 - test attachment`)
	attachment5.ContentType = "application/octet-stream"
	attachment5.Name = "data"
	attachments5 := []*attachmentInfo{}
	attachments5 = append(attachments5, attachment5)

	attachment6 := &attachmentInfo{}
	attachment6.AttachmentBytes = []byte(`marble06#$&'()*+,/:;=?@[] - test attachment`)
	attachment6.ContentType = "application/octet-stream"
	attachment6.Name = "data"
	attachments6 := []*attachmentInfo{}
	attachments6 = append(attachments6, attachment6)

	database := "testbatch"

	// create a new instance and database object   --------------------------------------------------------
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	batchUpdateDocs := []*couchDoc{}

	value1 := &couchDoc{jsonValue: byteJSON01, attachments: attachments1}
	value2 := &couchDoc{jsonValue: byteJSON02, attachments: attachments2}
	value3 := &couchDoc{jsonValue: byteJSON03, attachments: attachments3}
	value4 := &couchDoc{jsonValue: byteJSON04, attachments: attachments4}
	value5 := &couchDoc{jsonValue: byteJSON05, attachments: attachments5}
	value6 := &couchDoc{jsonValue: byteJSON06, attachments: attachments6}

	batchUpdateDocs = append(batchUpdateDocs, value1)
	batchUpdateDocs = append(batchUpdateDocs, value2)
	batchUpdateDocs = append(batchUpdateDocs, value3)
	batchUpdateDocs = append(batchUpdateDocs, value4)
	batchUpdateDocs = append(batchUpdateDocs, value5)
	batchUpdateDocs = append(batchUpdateDocs, value6)

	batchUpdateResp, err := db.batchUpdateDocuments(batchUpdateDocs)
	require.NoError(t, err, "Error when attempting to update a batch of documents")

	// check to make sure each batch update response was successful
	for _, updateDoc := range batchUpdateResp {
		require.Equal(t, true, updateDoc.Ok)
	}

	//----------------------------------------------
	//Test Retrieve JSON
	dbGetResp, _, geterr := db.readDoc("marble01")
	require.NoError(t, geterr, "Error when attempting read a document")

	assetResp := &Asset{}
	geterr = json.Unmarshal(dbGetResp.jsonValue, &assetResp)
	require.NoError(t, geterr, "Error when trying to retrieve a document")
	// Verify the owner retrieved matches
	require.Equal(t, "jerry", assetResp.Owner)

	//----------------------------------------------
	// Test Retrieve JSON using ID with URL special characters,
	// this will confirm that batch document IDs and URL IDs are consistent, even if they include special characters
	dbGetResp, _, geterr = db.readDoc("marble06#$&'()*+,/:;=?@[]")
	require.NoError(t, geterr, "Error when attempting read a document")

	assetResp = &Asset{}
	geterr = json.Unmarshal(dbGetResp.jsonValue, &assetResp)
	require.NoError(t, geterr, "Error when trying to retrieve a document")
	// Verify the owner retrieved matches
	require.Equal(t, "jerry", assetResp.Owner)

	//----------------------------------------------
	//Test retrieve binary
	dbGetResp, _, geterr = db.readDoc("marble03")
	require.NoError(t, geterr, "Error when attempting read a document")
	// Retrieve the attachments
	attachments := dbGetResp.attachments
	// Only one was saved, so take the first
	retrievedAttachment := attachments[0]
	// Verify the text matches
	require.Equal(t, retrievedAttachment.AttachmentBytes, attachment3.AttachmentBytes)
	//----------------------------------------------
	//Test Bad Updates
	batchUpdateDocs = []*couchDoc{}
	batchUpdateDocs = append(batchUpdateDocs, value1)
	batchUpdateDocs = append(batchUpdateDocs, value2)
	batchUpdateResp, err = db.batchUpdateDocuments(batchUpdateDocs)
	require.NoError(t, err, "Error when attempting to update a batch of documents")
	// No revision was provided, so these two updates should fail
	// Verify that the "Ok" field is returned as false
	for _, updateDoc := range batchUpdateResp {
		require.Equal(t, false, updateDoc.Ok)
		require.Equal(t, updateDocumentConflictError, updateDoc.Error)
		require.Equal(t, updateDocumentConflictReason, updateDoc.Reason)
	}

	//----------------------------------------------
	//Test Batch Retrieve Keys and Update

	var keys []string

	keys = append(keys, "marble01")
	keys = append(keys, "marble03")

	batchRevs, err := db.batchRetrieveDocumentMetadata(keys)
	require.NoError(t, err, "Error when attempting retrieve revisions")

	batchUpdateDocs = []*couchDoc{}

	// iterate through the revision docs
	for _, revdoc := range batchRevs {
		if revdoc.ID == "marble01" {
			// update the json with the rev and add to the batch
			marble01Doc := addRevisionAndDeleteStatus(t, revdoc.Rev, byteJSON01, false)
			batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: marble01Doc, attachments: attachments1})
		}

		if revdoc.ID == "marble03" {
			// update the json with the rev and add to the batch
			marble03Doc := addRevisionAndDeleteStatus(t, revdoc.Rev, byteJSON03, false)
			batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: marble03Doc, attachments: attachments3})
		}
	}

	// Update couchdb with the batch
	batchUpdateResp, err = db.batchUpdateDocuments(batchUpdateDocs)
	require.NoError(t, err, "Error when attempting to update a batch of documents")
	// check to make sure each batch update response was successful
	for _, updateDoc := range batchUpdateResp {
		require.Equal(t, true, updateDoc.Ok)
	}

	//----------------------------------------------
	//Test Batch Delete

	keys = []string{}

	keys = append(keys, "marble02")
	keys = append(keys, "marble04")

	batchRevs, err = db.batchRetrieveDocumentMetadata(keys)
	require.NoError(t, err, "Error when attempting retrieve revisions")

	batchUpdateDocs = []*couchDoc{}

	// iterate through the revision docs
	for _, revdoc := range batchRevs {
		if revdoc.ID == "marble02" {
			// update the json with the rev and add to the batch
			marble02Doc := addRevisionAndDeleteStatus(t, revdoc.Rev, byteJSON02, true)
			batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: marble02Doc, attachments: attachments1})
		}
		if revdoc.ID == "marble04" {
			// update the json with the rev and add to the batch
			marble04Doc := addRevisionAndDeleteStatus(t, revdoc.Rev, byteJSON04, true)
			batchUpdateDocs = append(batchUpdateDocs, &couchDoc{jsonValue: marble04Doc, attachments: attachments3})
		}
	}

	// Update couchdb with the batch
	batchUpdateResp, err = db.batchUpdateDocuments(batchUpdateDocs)
	require.NoError(t, err, "Error when attempting to update a batch of documents")

	// check to make sure each batch update response was successful
	for _, updateDoc := range batchUpdateResp {
		require.Equal(t, true, updateDoc.Ok)
	}

	// Retrieve the test document
	dbGetResp, _, geterr = db.readDoc("marble02")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// assert the value was deleted
	require.Nil(t, dbGetResp)

	// Retrieve the test document
	dbGetResp, _, geterr = db.readDoc("marble04")
	require.NoError(t, geterr, "Error when trying to retrieve a document")

	// assert the value was deleted
	require.Nil(t, dbGetResp)
}

// addRevisionAndDeleteStatus adds keys for version and chaincodeID to the JSON value
func addRevisionAndDeleteStatus(t *testing.T, revision string, value []byte, deleted bool) []byte {
	// create a version mapping
	jsonMap := make(map[string]interface{})

	require.NoError(t, json.Unmarshal(value, &jsonMap))

	// add the revision
	if revision != "" {
		jsonMap["_rev"] = revision
	}

	// If this record is to be deleted, set the "_deleted" property to true
	if deleted {
		jsonMap["_deleted"] = true
	}
	// marshal the data to a byte array
	returnJSON, _ := json.Marshal(jsonMap)

	return returnJSON
}

func TestDatabaseSecuritySettings(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdbsecuritysettings"

	// create a new instance and database object   --------------------------------------------------------
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	// Create a database security object
	securityPermissions := &databaseSecurity{}
	securityPermissions.Admins.Names = append(securityPermissions.Admins.Names, "admin")
	securityPermissions.Members.Names = append(securityPermissions.Members.Names, "admin")

	// Apply security
	err = db.applyDatabaseSecurity(securityPermissions)
	require.NoError(t, err, "Error when trying to apply database security")

	// Retrieve database security
	dbSecurity, err := db.getDatabaseSecurity()
	require.NoError(t, err, "Error when retrieving database security")

	// Verify retrieval of admins
	require.Equal(t, "admin", dbSecurity.Admins.Names[0])

	// Verify retrieval of members
	require.Equal(t, "admin", dbSecurity.Members.Names[0])

	// Create an empty database security object
	securityPermissions = &databaseSecurity{}

	// Apply the security
	err = db.applyDatabaseSecurity(securityPermissions)
	require.NoError(t, err, "Error when trying to apply database security")

	// Retrieve database security
	dbSecurity, err = db.getDatabaseSecurity()
	require.NoError(t, err, "Error when retrieving database security")

	// Verify retrieval of admins, should be an empty array
	require.Equal(t, 0, len(dbSecurity.Admins.Names))

	// Verify retrieval of members, should be an empty array
	require.Equal(t, 0, len(dbSecurity.Members.Names))
}

func TestURLWithSpecialCharacters(t *testing.T) {
	config := testConfig()
	couchDBEnv.startCouchDB(t)
	config.Address = couchDBEnv.couchAddress
	defer couchDBEnv.cleanup(config)
	database := "testdb+with+plus_sign"

	// parse a contructed URL
	finalURL, err := url.Parse("http://127.0.0.1:5984")
	require.NoError(t, err, "error thrown while parsing couchdb url")

	// test the constructCouchDBUrl function with multiple path elements
	couchdbURL := constructCouchDBUrl(finalURL, database, "_index", "designdoc", "json", "indexname")
	require.Equal(t, "http://127.0.0.1:5984/testdb%2Bwith%2Bplus_sign/_index/designdoc/json/indexname", couchdbURL.String())

	// create a new instance and database object   --------------------------------------------------------
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err, "Error when trying to create couch instance")
	db := couchDatabase{couchInstance: couchInstance, dbName: database}

	// create a new database
	errdb := db.createDatabaseIfNotExist()
	require.NoError(t, errdb, "Error when trying to create database")

	dbInfo, _, errInfo := db.getDatabaseInfo()
	require.NoError(t, errInfo, "Error when trying to get database info")

	require.Equal(t, database, dbInfo.DbName)
}

func TestCouchDocKey(t *testing.T) {
	m := make(jsonValue)
	m[idField] = "key-1"
	m[revField] = "rev-1"
	m["a"] = "b"
	json, err := json.Marshal(m)
	require.NoError(t, err)
	doc := &couchDoc{jsonValue: json}
	actualKey, err := doc.key()
	require.NoError(t, err)
	require.Equal(t, "key-1", actualKey)

	doc = &couchDoc{jsonValue: []byte("random")}
	_, err = doc.key()
	require.Error(t, err)
}

func TestCouchDocLength(t *testing.T) {
	testData := []struct {
		description    string
		doc            *couchDoc
		expectedLength int
	}{
		{
			description: "doc has a json value and attachments",
			doc: &couchDoc{
				jsonValue: []byte("7length"),
				attachments: []*attachmentInfo{
					{
						Name:            "5leng",
						ContentType:     "3le",
						AttachmentBytes: []byte("8length."),
					},
					{
						AttachmentBytes: []byte("8length."),
					},
					{},
				},
			},
			expectedLength: 31, // 7 + 5 + 3 + 8 + 8
		},
		{
			description: "doc has only json value",
			doc: &couchDoc{
				jsonValue: []byte("7length"),
			},
			expectedLength: 7,
		},
		{
			description:    "doc is empty",
			doc:            &couchDoc{},
			expectedLength: 0,
		},
		{
			description:    "doc is nil",
			doc:            nil,
			expectedLength: 0,
		},
	}

	for _, tData := range testData {
		t.Run(tData.description, func(t *testing.T) {
			require.Equal(t, tData.expectedLength, tData.doc.len())
		})
	}
}
