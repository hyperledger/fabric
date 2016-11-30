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

//CreateCouchDBConnectionAndDB creates a CouchDB Connection and the database if it does not exist
func CreateCouchDBConnectionAndDB(couchDBConnectURL string, dbName string, id string, pw string) (*CouchDBConnectionDef, error) {
	couchDB, err := CreateConnectionDefinition(couchDBConnectURL,
		dbName,
		id,
		pw)
	if err != nil {
		logger.Errorf("Error during CouchDB CreateConnectionDefinition() to dbName: %s  error: %s\n", dbName, err.Error())
		return nil, err
	}

	// Create CouchDB database upon ledger startup, if it doesn't already exist
	_, err = couchDB.CreateDatabaseIfNotExist()
	if err != nil {
		logger.Errorf("Error during CouchDB CreateDatabaseIfNotExist() to dbName: %s  error: %s\n", dbName, err.Error())
		return nil, err
	}

	//return the couch db connection
	return couchDB, nil
}
