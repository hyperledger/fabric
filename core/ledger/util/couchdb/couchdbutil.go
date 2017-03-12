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
	"regexp"
	"strconv"
	"strings"
)

var validNamePattern = `^[a-z][a-z0-9_$(),+/-]+`
var maxLength = 249

//CreateCouchInstance creates a CouchDB instance
func CreateCouchInstance(couchDBConnectURL string, id string, pw string) (*CouchInstance, error) {
	couchConf, err := CreateConnectionDefinition(couchDBConnectURL,
		id,
		pw)
	if err != nil {
		logger.Errorf("Error during CouchDB CreateConnectionDefinition(): %s\n", err.Error())
		return nil, err
	}

	//Create the CouchDB instance
	couchInstance := &CouchInstance{conf: *couchConf}

	connectInfo, retVal, verifyErr := couchInstance.VerifyConnection()
	if verifyErr != nil {
		return nil, fmt.Errorf("Unable to connect to CouchDB, check the hostname and port: %s", verifyErr.Error())
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

	couchDBDatabase := CouchDatabase{couchInstance: couchInstance, dbName: databaseName}

	// Create CouchDB database upon ledger startup, if it doesn't already exist
	_, err = couchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for dbName: %s  error: %s\n", dbName, err.Error())
		return nil, err
	}

	return &couchDBDatabase, nil
}

//mapAndValidateDatabaseName checks to see if the database name contains illegal characters
//CouchDB Rules: Only lowercase characters (a-z), digits (0-9), and any of the characters
//_, $, (, ), +, -, and / are allowed. Must begin with a letter.
//
//Restictions have already been applied to the database name from Orderer based on
//restrictions required by Kafka
//
//The validation will validate upper case, the string will be lower cased
//Replace any characters not allowed in CouchDB with an "_"
//Check for a leading letter, if not present, the prepend "db_"
func mapAndValidateDatabaseName(databaseName string) (string, error) {

	// test Length
	if len(databaseName) <= 0 {
		return "", fmt.Errorf("Database name is illegal, cannot be empty")
	}
	if len(databaseName) > maxLength {
		return "", fmt.Errorf("Database name is illegal, cannot be longer than %d", maxLength)
	}

	//force the name to all lowercase
	databaseName = strings.ToLower(databaseName)

	//Replace any characters not allowed in CouchDB with an "_"
	replaceString := regexp.MustCompile(`[^a-z0-9_$(),+/-]`)

	//Set up the replace pattern for special characters
	validatedDatabaseName := replaceString.ReplaceAllString(databaseName, "_")

	//if the first character is not a letter, then prepend "db_"
	testLeadingLetter := regexp.MustCompile("^[a-z]")
	isLeadingLetter := testLeadingLetter.MatchString(validatedDatabaseName)
	if !isLeadingLetter {
		validatedDatabaseName = "db_" + validatedDatabaseName
	}

	//create the expression for valid characters
	validString := regexp.MustCompile(validNamePattern)

	// Illegal characters
	matched := validString.MatchString(validatedDatabaseName)
	if !matched {
		return "", fmt.Errorf("Database name '%s' contains illegal characters", validatedDatabaseName)
	}
	return validatedDatabaseName, nil
}
