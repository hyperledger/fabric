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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("couchdb")

// DBOperationResponse is body for successful database calls.
type DBOperationResponse struct {
	Ok  bool
	id  string
	rev string
}

// DBInfo is body for database information.
type DBInfo struct {
	DbName    string `json:"db_name"`
	UpdateSeq string `json:"update_seq"`
	Sizes     struct {
		File     int `json:"file"`
		External int `json:"external"`
		Active   int `json:"active"`
	} `json:"sizes"`
	PurgeSeq int `json:"purge_seq"`
	Other    struct {
		DataSize int `json:"data_size"`
	} `json:"other"`
	DocDelCount       int    `json:"doc_del_count"`
	DocCount          int    `json:"doc_count"`
	DiskSize          int    `json:"disk_size"`
	DiskFormatVersion int    `json:"disk_format_version"`
	DataSize          int    `json:"data_size"`
	CompactRunning    bool   `json:"compact_running"`
	InstanceStartTime string `json:"instance_start_time"`
}

// DBConnectionDef contains parameters
type DBConnectionDef struct {
	URL      string
	Username string
	Password string
	Database string
}

//CouchDBReturn contains an error reported by CouchDB
type CouchDBReturn struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

//CreateConnectionDefinition for a new client connection
func CreateConnectionDefinition(host string, port int, databaseName, username, password string) (*DBConnectionDef, error) {

	logger.Debugf("===COUCHDB=== Entering CreateConnectionDefinition()")

	connectURI := []string{}
	connectURI = append(connectURI, "http://")
	connectURI = append(connectURI, host)
	connectURI = append(connectURI, ":")

	connectURI = append(connectURI, strconv.Itoa(port))

	urlConcat := strings.Join(connectURI, "")

	//parse the constructed URL to verify no errors
	finalURL, err := url.Parse(urlConcat)
	if err != nil {
		return nil, err
	}

	logger.Debugf("===COUCHDB=== Created database configuration  URL=[%s]  database=%s", finalURL.String(), databaseName)

	logger.Debugf("===COUCHDB=== Exiting CreateConnectionDefinition()")

	//return an object containing the connection information
	return &DBConnectionDef{finalURL.String(), username, password, databaseName}, nil
}

//CreateDatabaseIfNotExist method provides function to create database
func (dbclient *DBConnectionDef) CreateDatabaseIfNotExist() (*DBOperationResponse, error) {

	logger.Debugf("===COUCHDB=== Entering CreateDatabaseIfNotExist()")

	dbInfo, couchDBReturn, err := dbclient.GetDatabaseInfo()
	if err != nil {
		if couchDBReturn == nil || couchDBReturn.StatusCode != 404 {
			return nil, err
		}
	}

	if dbInfo == nil && couchDBReturn.StatusCode == 404 {

		logger.Debugf("===COUCHDB=== Database %s does not exist.", dbclient.Database)

		//create a URL for creating a database
		url := fmt.Sprintf("%s/%s", dbclient.URL, dbclient.Database)

		//process the URL with a PUT, creates the database
		resp, _, err := dbclient.handleRequest(http.MethodPut, url, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		//Get the response from the create REST call
		dbResponse := &DBOperationResponse{}
		json.NewDecoder(resp.Body).Decode(&dbResponse)

		if dbResponse.Ok == true {
			logger.Debugf("===COUCHDB=== Created database %s ", dbclient.Database)
		}

		logger.Debugf("===COUCHDB=== Exiting CreateDatabaseIfNotExist()")

		return dbResponse, nil

	}

	logger.Debugf("===COUCHDB=== Database %s already exists", dbclient.Database)

	logger.Debugf("===COUCHDB=== Exiting CreateDatabaseIfNotExist()")

	return nil, nil

}

//GetDatabaseInfo method provides function to retrieve database information
func (dbclient *DBConnectionDef) GetDatabaseInfo() (*DBInfo, *CouchDBReturn, error) {

	url := fmt.Sprintf("%s/%s", dbclient.URL, dbclient.Database)

	resp, couchDBReturn, err := dbclient.handleRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, couchDBReturn, err
	}
	defer resp.Body.Close()

	dbResponse := &DBInfo{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	return dbResponse, couchDBReturn, nil

}

//DropDatabase provides method to drop an existing database
func (dbclient *DBConnectionDef) DropDatabase() (*DBOperationResponse, error) {

	logger.Debugf("===COUCHDB=== Entering DropDatabase()")

	url := fmt.Sprintf("%s/%s", dbclient.URL, dbclient.Database)

	resp, _, err := dbclient.handleRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dbResponse := &DBOperationResponse{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	if dbResponse.Ok == true {
		logger.Debugf("===COUCHDB=== Dropped database %s ", dbclient.Database)
	}

	logger.Debugf("===COUCHDB=== Exiting DropDatabase()")

	if dbResponse.Ok == true {

		return dbResponse, nil

	} else {

		return dbResponse, fmt.Errorf("Error dropping database")

	}

}

//SaveDoc method provides a function to save a document, id and byte array
func (dbclient *DBConnectionDef) SaveDoc(id string, bytesDoc []byte) (string, error) {

	logger.Debugf("===COUCHDB=== Entering SaveDoc()")

	url := fmt.Sprintf("%s/%s/%s", dbclient.URL, dbclient.Database, id)

	logger.Debugf("===COUCHDB===   id=%s,  value=%s", id, string(bytesDoc))

	data := bytes.NewReader(bytesDoc)

	resp, _, err := dbclient.handleRequest(http.MethodPut, url, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	revision, err := getRevisionHeader(resp)
	if err != nil {
		return "", err
	}

	logger.Debugf("===COUCHDB=== Exiting SaveDoc()")

	return revision, nil

}

func getRevisionHeader(resp *http.Response) (string, error) {

	if revision := resp.Header.Get("Etag"); revision == "" {
		return "", fmt.Errorf("No revision tag detected")
	} else {
		reg := regexp.MustCompile(`"([^"]*)"`)
		revisionNoQuotes := reg.ReplaceAllString(revision, "${1}")
		return revisionNoQuotes, nil
	}

}

//ReadDoc method provides function to retrieve a document from the database by id
func (dbclient *DBConnectionDef) ReadDoc(id string) ([]byte, string, error) {

	logger.Debugf("===COUCHDB=== Entering ReadDoc()  id=%s", id)

	url := fmt.Sprintf("%s/%s/%s", dbclient.URL, dbclient.Database, id)

	resp, _, err := dbclient.handleRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	jsonDoc, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	logger.Debugf("===COUCHDB=== Read document, id=%s, value=%s", id, string(jsonDoc))

	revision, err := getRevisionHeader(resp)
	if err != nil {
		return nil, "", err
	}

	logger.Debugf("===COUCHDB=== Exiting ReadDoc()")

	return jsonDoc, revision, nil

}

//handleRequest method is a generic http request handler
func (dbclient *DBConnectionDef) handleRequest(method, url string, data io.Reader) (*http.Response, *CouchDBReturn, error) {

	logger.Debugf("===COUCHDB=== Entering handleRequest()  method=%s  url=%s", method, url)

	//Create request based on URL for couchdb operation
	req, err := http.NewRequest(method, url, data)
	if err != nil {
		return nil, nil, err
	}

	//add content header for PUT
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}

	//add content header for PUT or GET
	if method == http.MethodGet || method == http.MethodPut {
		req.Header.Set("Accept", "application/json")
	}

	//If username and password are set the use basic auth
	if dbclient.Username != "" && dbclient.Password != "" {
		req.SetBasicAuth(dbclient.Username, dbclient.Password)
	}

	//Create the http client
	client := &http.Client{}

	//Execute http request
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	//create the return object for couchDB
	couchDBReturn := &CouchDBReturn{}

	//set the return code for the couchDB request
	couchDBReturn.StatusCode = resp.StatusCode

	//check to see if the status code is 400 or higher
	//in this case, the http request succeeded but CouchDB is reporing an error
	if resp.StatusCode >= 400 {

		jsonError, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}

		logger.Debugf("===COUCHDB=== Couch DB error  status code=%v  error=%s", resp.StatusCode, jsonError)

		errorBytes := []byte(jsonError)

		json.Unmarshal(errorBytes, &couchDBReturn)

		return nil, couchDBReturn, fmt.Errorf("Couch DB Error: %s", couchDBReturn.Reason)

	}

	logger.Debugf("===COUCHDB=== Exiting handleRequest()")

	//If no errors, then return the results
	return resp, couchDBReturn, nil
}
