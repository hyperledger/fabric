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
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var expectedChannelNamePattern = `[a-z][a-z0-9.-]*`
var maxLength = 249

//CreateCouchInstance creates a CouchDB instance
func CreateCouchInstance(couchDBConnectURL, id, pw string, maxRetries,
	maxRetriesOnStartup int, connectionTimeout time.Duration) (*CouchInstance, error) {

	couchConf, err := CreateConnectionDefinition(couchDBConnectURL,
		id, pw, maxRetries, maxRetriesOnStartup, connectionTimeout)
	if err != nil {
		logger.Errorf("Error during CouchDB CreateConnectionDefinition(): %s\n", err.Error())
		return nil, err
	}

	// Create the http client once
	// Clients and Transports are safe for concurrent use by multiple goroutines
	// and for efficiency should only be created once and re-used.
	client := &http.Client{Timeout: couchConf.RequestTimeout}

	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	transport.DisableCompression = false
	client.Transport = transport

	//Create the CouchDB instance
	couchInstance := &CouchInstance{conf: *couchConf, client: client}

	connectInfo, retVal, verifyErr := couchInstance.VerifyCouchConfig()
	if verifyErr != nil {
		return nil, verifyErr
	}

	//return an error if the http return value is not 200
	if retVal.StatusCode != 200 {
		return nil, fmt.Errorf("CouchDB connection error, expecting return code of 200, received %v", retVal.StatusCode)
	}

	//check the CouchDB version number, return an error if the version is not at least 2.0.0
	errVersion := checkCouchDBVersion(connectInfo.Version)
	if errVersion != nil {
		return nil, errVersion
	}

	return couchInstance, nil
}

//checkCouchDBVersion verifies CouchDB is at least 2.0.0
func checkCouchDBVersion(version string) error {

	//split the version into parts
	majorVersion := strings.Split(version, ".")

	//check to see that the major version number is at least 2
	majorVersionInt, _ := strconv.Atoi(majorVersion[0])
	if majorVersionInt < 2 {
		return fmt.Errorf("CouchDB must be at least version 2.0.0.  Detected version %s", version)
	}

	return nil
}

//CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
func CreateCouchDatabase(couchInstance CouchInstance, dbName string) (*CouchDatabase, error) {

	databaseName, err := mapAndValidateDatabaseName(dbName)
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for dbName: %s  error: %s\n", dbName, err.Error())
		return nil, err
	}

	couchDBDatabase := CouchDatabase{CouchInstance: couchInstance, DBName: databaseName}

	// Create CouchDB database upon ledger startup, if it doesn't already exist
	_, err = couchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for dbName: %s  error: %s\n", dbName, err.Error())
		return nil, err
	}

	return &couchDBDatabase, nil
}

//CreateSystemDatabasesIfNotExist - creates the system databases if they do not exist
func CreateSystemDatabasesIfNotExist(couchInstance CouchInstance) error {

	dbName := "_users"
	systemCouchDBDatabase := CouchDatabase{CouchInstance: couchInstance, DBName: dbName}
	_, err := systemCouchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for system dbName: %s  error: %s\n", dbName, err.Error())
		return err
	}

	dbName = "_replicator"
	systemCouchDBDatabase = CouchDatabase{CouchInstance: couchInstance, DBName: dbName}
	_, err = systemCouchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for system dbName: %s  error: %s\n", dbName, err.Error())
		return err
	}

	dbName = "_global_changes"
	systemCouchDBDatabase = CouchDatabase{CouchInstance: couchInstance, DBName: dbName}
	_, err = systemCouchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for system dbName: %s  error: %s\n", dbName, err.Error())
		return err
	}

	return nil

}

//mapAndValidateDatabaseName checks to see if the database name contains illegal characters
//CouchDB Rules: Only lowercase characters (a-z), digits (0-9), and any of the characters
//_, $, (, ), +, -, and / are allowed. Must begin with a letter.
//
//Restictions have already been applied to the database name from Orderer based on
//restrictions required by Kafka and couchDB (except a '.' char). The databaseName
// passed in here is expected to follow `[a-z][a-z0-9.-]*` pattern.
//
//This validation will simply check whether the database name matches the above pattern and will replace
// all occurence of '.' by '-'. This will not cause collisions in the trnasformed named
func mapAndValidateDatabaseName(databaseName string) (string, error) {
	// test Length
	if len(databaseName) <= 0 {
		return "", fmt.Errorf("Database name is illegal, cannot be empty")
	}
	if len(databaseName) > maxLength {
		return "", fmt.Errorf("Database name is illegal, cannot be longer than %d", maxLength)
	}
	re, err := regexp.Compile(expectedChannelNamePattern)
	if err != nil {
		return "", err
	}
	matched := re.FindString(databaseName)
	if len(matched) != len(databaseName) {
		return "", fmt.Errorf("databaseName '%s' does not matches pattern '%s'", databaseName, expectedChannelNamePattern)
	}
	// replace all '.' to '_'. The databaseName passed in will never contain an '_'. So, this translation will not cause collisions
	databaseName = strings.Replace(databaseName, ".", "_", -1)
	return databaseName, nil
}
