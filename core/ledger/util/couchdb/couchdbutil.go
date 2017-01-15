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

//CreateCouchInstance creates a CouchDB instance
func CreateCouchInstance(couchDBConnectURL string, id string, pw string) (*CouchInstance, error) {
	couchConf, err := CreateConnectionDefinition(couchDBConnectURL,
		id,
		pw)
	if err != nil {
		logger.Errorf("Error during CouchDB CreateConnectionDefinition(): %s\n", err.Error())
		return nil, err
	}

	return &CouchInstance{conf: *couchConf}, nil
}

//CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
func CreateCouchDatabase(couchInstance CouchInstance, dbName string) (*CouchDatabase, error) {

	couchDBDatabase := CouchDatabase{couchInstance: couchInstance, dbName: dbName}

	// Create CouchDB database upon ledger startup, if it doesn't already exist
	_, err := couchDBDatabase.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() for dbName: %s  error: %s\n", dbName, err.Error())
		return nil, err
	}

	return &couchDBDatabase, nil
}
