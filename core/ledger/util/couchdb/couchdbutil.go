/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"bytes"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/util"
	"github.com/pkg/errors"
)

var expectedDatabaseNamePattern = `[a-z][a-z0-9.$_()+-]*`
var maxLength = 238

// To restrict the length of couchDB database name to the
// allowed length of 249 chars, the string length limit
// for chain/channel name, namespace/chaincode name, and
// collection name, which constitutes the database name,
// is defined.
var chainNameAllowedLength = 50
var namespaceNameAllowedLength = 50
var collectionNameAllowedLength = 50

//CreateCouchInstance creates a CouchDB instance
func CreateCouchInstance(couchDBConnectURL, id, pw string, maxRetries,
	maxRetriesOnStartup int, connectionTimeout time.Duration, createGlobalChangesDB bool, metricsProvider metrics.Provider) (*CouchInstance, error) {

	couchConf, err := CreateConnectionDefinition(couchDBConnectURL,
		id, pw, maxRetries, maxRetriesOnStartup, connectionTimeout, createGlobalChangesDB)
	if err != nil {
		logger.Errorf("Error calling CouchDB CreateConnectionDefinition(): %s", err)
		return nil, err
	}

	// Create the http client once
	// Clients and Transports are safe for concurrent use by multiple goroutines
	// and for efficiency should only be created once and re-used.
	client := &http.Client{Timeout: couchConf.RequestTimeout}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          2000,
		MaxIdleConnsPerHost:   2000,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client.Transport = transport

	//Create the CouchDB instance
	couchInstance := &CouchInstance{conf: *couchConf, client: client}
	couchInstance.stats = newStats(metricsProvider)
	connectInfo, retVal, verifyErr := couchInstance.VerifyCouchConfig()
	if verifyErr != nil {
		return nil, verifyErr
	}

	//return an error if the http return value is not 200
	if retVal.StatusCode != 200 {
		return nil, errors.Errorf("CouchDB connection error, expecting return code of 200, received %v", retVal.StatusCode)
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
		return errors.Errorf("CouchDB must be at least version 2.0.0. Detected version %s", version)
	}

	return nil
}

//CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
func CreateCouchDatabase(couchInstance *CouchInstance, dbName string) (*CouchDatabase, error) {

	databaseName, err := mapAndValidateDatabaseName(dbName)
	if err != nil {
		logger.Errorf("Error calling CouchDB CreateDatabaseIfNotExist() for dbName: %s, error: %s", dbName, err)
		return nil, err
	}

	couchDBDatabase := CouchDatabase{CouchInstance: couchInstance, DBName: databaseName, IndexWarmCounter: 1}

	// Create CouchDB database upon ledger startup, if it doesn't already exist
	err = couchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error calling CouchDB CreateDatabaseIfNotExist() for dbName: %s, error: %s", dbName, err)
		return nil, err
	}

	return &couchDBDatabase, nil
}

//CreateSystemDatabasesIfNotExist - creates the system databases if they do not exist
func CreateSystemDatabasesIfNotExist(couchInstance *CouchInstance) error {

	dbName := "_users"
	systemCouchDBDatabase := CouchDatabase{CouchInstance: couchInstance, DBName: dbName, IndexWarmCounter: 1}
	err := systemCouchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error calling CouchDB CreateDatabaseIfNotExist() for system dbName: %s, error: %s", dbName, err)
		return err
	}

	dbName = "_replicator"
	systemCouchDBDatabase = CouchDatabase{CouchInstance: couchInstance, DBName: dbName, IndexWarmCounter: 1}
	err = systemCouchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error calling CouchDB CreateDatabaseIfNotExist() for system dbName: %s, error: %s", dbName, err)
		return err
	}
	if couchInstance.conf.CreateGlobalChangesDB {
		dbName = "_global_changes"
		systemCouchDBDatabase = CouchDatabase{CouchInstance: couchInstance, DBName: dbName, IndexWarmCounter: 1}
		err = systemCouchDBDatabase.CreateDatabaseIfNotExist()
		if err != nil {
			logger.Errorf("Error calling CouchDB CreateDatabaseIfNotExist() for system dbName: %s, error: %s", dbName, err)
			return err
		}
	}
	return nil

}

// constructCouchDBUrl constructs a couchDB url with encoding for the database name
// and all path elements
func constructCouchDBUrl(connectURL *url.URL, dbName string, pathElements ...string) *url.URL {
	var buffer bytes.Buffer
	buffer.WriteString(connectURL.String())
	if dbName != "" {
		buffer.WriteString("/")
		buffer.WriteString(encodePathElement(dbName))
	}
	for _, pathElement := range pathElements {
		buffer.WriteString("/")
		buffer.WriteString(encodePathElement(pathElement))
	}
	return &url.URL{Opaque: buffer.String()}
}

// ConstructMetadataDBName truncates the db name to couchdb allowed length to
// construct the metadataDBName
func ConstructMetadataDBName(dbName string) string {
	if len(dbName) > maxLength {
		untruncatedDBName := dbName
		// Truncate the name if the length violates the allowed limit
		// As the passed dbName is same as chain/channel name, truncate using chainNameAllowedLength
		dbName = dbName[:chainNameAllowedLength]
		// For metadataDB (i.e., chain/channel DB), the dbName contains <first 50 chars
		// (i.e., chainNameAllowedLength) of chainName> + (SHA256 hash of actual chainName)
		dbName = dbName + "(" + hex.EncodeToString(util.ComputeSHA256([]byte(untruncatedDBName))) + ")"
		// 50 chars for dbName + 1 char for ( + 64 chars for sha256 + 1 char for ) = 116 chars
	}
	return dbName + "_"
}

// ConstructNamespaceDBName truncates db name to couchdb allowed length to
// construct the namespaceDBName
func ConstructNamespaceDBName(chainName, namespace string) string {
	// replace upper-case in namespace with a escape sequence '$' and the respective lower-case letter
	escapedNamespace := escapeUpperCase(namespace)
	namespaceDBName := chainName + "_" + escapedNamespace

	// For namespaceDBName of form 'chainName_namespace', on length limit violation, the truncated
	// namespaceDBName would contain <first 50 chars (i.e., chainNameAllowedLength) of chainName> + "_" +
	// <first 50 chars (i.e., namespaceNameAllowedLength) chars of namespace> +
	// (<SHA256 hash of [chainName_namespace]>)
	//
	// For namespaceDBName of form 'chainName_namespace$$collection', on length limit violation, the truncated
	// namespaceDBName would contain <first 50 chars (i.e., chainNameAllowedLength) of chainName> + "_" +
	// <first 50 chars (i.e., namespaceNameAllowedLength) of namespace> + "$$" + <first 50 chars
	// (i.e., collectionNameAllowedLength) of collection> + (<SHA256 hash of [chainName_namespace$$pcollection]>)

	if len(namespaceDBName) > maxLength {
		// Compute the hash of untruncated namespaceDBName that needs to be appended to
		// truncated namespaceDBName for maintaining uniqueness
		hashOfNamespaceDBName := hex.EncodeToString(util.ComputeSHA256([]byte(chainName + "_" + namespace)))

		// As truncated namespaceDBName is of form 'chainName_escapedNamespace', both chainName
		// and escapedNamespace need to be truncated to defined allowed length.
		if len(chainName) > chainNameAllowedLength {
			// Truncate chainName to chainNameAllowedLength
			chainName = chainName[0:chainNameAllowedLength]
		}
		// As escapedNamespace can be of either 'namespace' or 'namespace$$collectionName',
		// both 'namespace' and 'collectionName' need to be truncated to defined allowed length.
		// '$$' is used as joiner between namespace and collection name.
		// Split the escapedNamespace into escaped namespace and escaped collection name if exist.
		names := strings.Split(escapedNamespace, "$$")
		namespace := names[0]
		if len(namespace) > namespaceNameAllowedLength {
			// Truncate the namespace
			namespace = namespace[0:namespaceNameAllowedLength]
		}

		escapedNamespace = namespace

		// Check and truncate the length of collection name if exist
		if len(names) == 2 {
			collection := names[1]
			if len(collection) > collectionNameAllowedLength {
				// Truncate the escaped collection name
				collection = collection[0:collectionNameAllowedLength]
			}
			// Append truncated collection name to escapedNamespace
			escapedNamespace = escapedNamespace + "$$" + collection
		}
		// Construct and return the namespaceDBName
		// 50 chars for chainName + 1 char for '_' + 102 chars for escaped namespace + 1 char for '(' + 64 chars
		// for sha256 hash + 1 char for ')' = 219 chars
		return chainName + "_" + escapedNamespace + "(" + hashOfNamespaceDBName + ")"
	}
	return namespaceDBName
}

//mapAndValidateDatabaseName checks to see if the database name contains illegal characters
//CouchDB Rules: Only lowercase characters (a-z), digits (0-9), and any of the characters
//_, $, (, ), +, -, and / are allowed. Must begin with a letter.
//
//Restictions have already been applied to the database name from Orderer based on
//restrictions required by Kafka and couchDB (except a '.' char). The databaseName
// passed in here is expected to follow `[a-z][a-z0-9.$_-]*` pattern.
//
//This validation will simply check whether the database name matches the above pattern and will replace
// all occurence of '.' by '$'. This will not cause collisions in the transformed named
func mapAndValidateDatabaseName(databaseName string) (string, error) {
	// test Length
	if len(databaseName) <= 0 {
		return "", errors.Errorf("database name is illegal, cannot be empty")
	}
	if len(databaseName) > maxLength {
		return "", errors.Errorf("database name is illegal, cannot be longer than %d", maxLength)
	}
	re, err := regexp.Compile(expectedDatabaseNamePattern)
	if err != nil {
		return "", errors.Wrapf(err, "error compiling regexp: %s", expectedDatabaseNamePattern)
	}
	matched := re.FindString(databaseName)
	if len(matched) != len(databaseName) {
		return "", errors.Errorf("databaseName '%s' does not match pattern '%s'", databaseName, expectedDatabaseNamePattern)
	}
	// replace all '.' to '$'. The databaseName passed in will never contain an '$'.
	// So, this translation will not cause collisions
	databaseName = strings.Replace(databaseName, ".", "$", -1)
	return databaseName, nil
}

// escapeUpperCase replaces every upper case letter with a '$' and the respective
// lower-case letter
func escapeUpperCase(dbName string) string {
	re := regexp.MustCompile(`([A-Z])`)
	dbName = re.ReplaceAllString(dbName, "$$"+"$1")
	return strings.ToLower(dbName)
}
