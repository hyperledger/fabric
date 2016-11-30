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
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
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

// CouchDBConnectionDef contains parameters
type CouchDBConnectionDef struct {
	URL      string
	Username string
	Password string
	Database string
}

//DBReturn contains an error reported by CouchDB
type DBReturn struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

//Attachment contains the definition for an attached file for couchdb
type Attachment struct {
	Name            string
	ContentType     string
	Length          uint64
	AttachmentBytes []byte
}

//DocRev returns the Id and revision for a couchdb document
type DocRev struct {
	Id  string `json:"_id"`
	Rev string `json:"_rev"`
}

//FileDetails defines the structure needed to send an attachment to couchdb
type FileDetails struct {
	Follows     bool   `json:"follows"`
	ContentType string `json:"content_type"`
	Length      int    `json:"length"`
}

//CreateConnectionDefinition for a new client connection
func CreateConnectionDefinition(couchDBAddress string, databaseName, username, password string) (*CouchDBConnectionDef, error) {

	logger.Debugf("===COUCHDB=== Entering CreateConnectionDefinition()")

	connectURL := fmt.Sprintf("%s//%s", "http:", couchDBAddress)

	//parse the constructed URL to verify no errors
	finalURL, err := url.Parse(connectURL)
	if err != nil {
		logger.Errorf("===COUCHDB=== URL parse error: %s", err.Error())
		return nil, err
	}

	logger.Debugf("===COUCHDB=== Created database configuration  URL=[%s]  database=%s", finalURL.String(), databaseName)

	logger.Debugf("===COUCHDB=== Exiting CreateConnectionDefinition()")

	//return an object containing the connection information
	return &CouchDBConnectionDef{finalURL.String(), username, password, databaseName}, nil
}

//CreateDatabaseIfNotExist method provides function to create database
func (dbclient *CouchDBConnectionDef) CreateDatabaseIfNotExist() (*DBOperationResponse, error) {

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
		resp, _, err := dbclient.handleRequest(http.MethodPut, url, nil, "", "")
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
func (dbclient *CouchDBConnectionDef) GetDatabaseInfo() (*DBInfo, *DBReturn, error) {

	url := fmt.Sprintf("%s/%s", dbclient.URL, dbclient.Database)

	resp, couchDBReturn, err := dbclient.handleRequest(http.MethodGet, url, nil, "", "")
	if err != nil {
		return nil, couchDBReturn, err
	}
	defer resp.Body.Close()

	dbResponse := &DBInfo{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	// trace the database info response
	if logger.IsEnabledFor(logging.DEBUG) {
		dbResponseJSON, err := json.Marshal(dbResponse)
		if err == nil {
			logger.Debugf("===COUCHDB=== GetDatabaseInfo() dbResponseJSON: %s", dbResponseJSON)
		}
	}

	return dbResponse, couchDBReturn, nil

}

//DropDatabase provides method to drop an existing database
func (dbclient *CouchDBConnectionDef) DropDatabase() (*DBOperationResponse, error) {

	logger.Debugf("===COUCHDB=== Entering DropDatabase()")

	url := fmt.Sprintf("%s/%s", dbclient.URL, dbclient.Database)

	resp, _, err := dbclient.handleRequest(http.MethodDelete, url, nil, "", "")
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

	}

	return dbResponse, fmt.Errorf("Error dropping database")

}

//SaveDoc method provides a function to save a document, id and byte array
func (dbclient *CouchDBConnectionDef) SaveDoc(id string, rev string, bytesDoc []byte, attachments []Attachment) (string, error) {

	logger.Debugf("===COUCHDB=== Entering SaveDoc()")

	url := fmt.Sprintf("%s/%s/%s", dbclient.URL, dbclient.Database, id)

	logger.Debugf("===COUCHDB===   id=%s,  value=%s", id, string(bytesDoc))

	if rev == "" {

		//See if the document already exists, we need the rev for save
		_, revdoc, err := dbclient.ReadDoc(id)
		if err != nil {
			//set the revision to indicate that the document was not found
			rev = ""
		} else {
			//set the revision to the rev returned from the document read
			rev = revdoc
		}
	}

	logger.Debugf("===COUCHDB===   rev=%s", rev)

	//Set up a buffer for the data to be pushed to couchdb
	data := new(bytes.Buffer)

	//Set up a default boundary for use by multipart if sending attachments
	defaultBoundary := ""

	//check to see if attachments is nil, if so, then this is a JSON only
	if attachments == nil {

		//Test to see if this is a valid JSON
		if IsJSON(string(bytesDoc)) != true {
			return "", fmt.Errorf("JSON format is not valid")
		}

		// if there are no attachments, then use the bytes passed in as the JSON
		data.ReadFrom(bytes.NewReader(bytesDoc))

	} else {

		//attachments are included, create the multipart definition
		multipartData, multipartBoundary, err := createAttachmentPart(*data, attachments, defaultBoundary)
		if err != nil {
			return "", err
		}

		//Set the data buffer to the data from the create multi-part data
		data.ReadFrom(&multipartData)

		//Set the default boundary to the value generated in the multipart creation
		defaultBoundary = multipartBoundary

	}

	//handle the request for saving the JSON or attachments
	resp, _, err := dbclient.handleRequest(http.MethodPut, url, data, rev, defaultBoundary)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	//get the revision and return
	revision, err := getRevisionHeader(resp)
	if err != nil {
		return "", err
	}

	logger.Debugf("===COUCHDB=== Exiting SaveDoc()")

	return revision, nil

}

func createAttachmentPart(data bytes.Buffer, attachments []Attachment, defaultBoundary string) (bytes.Buffer, string, error) {

	// read the attachment and save as an attachment
	writer := multipart.NewWriter(&data)

	//retrieve the boundary for the multipart
	defaultBoundary = writer.Boundary()

	fileAttachments := map[string]FileDetails{}

	for _, attachment := range attachments {

		fileAttachments[attachment.Name] = FileDetails{true, attachment.ContentType, len(attachment.AttachmentBytes)}

	}

	attachmentJSONMap := map[string]interface{}{
		"_attachments": fileAttachments}

	filesForUpload, _ := json.Marshal(attachmentJSONMap)
	logger.Debugf(string(filesForUpload))

	//create the header for the JSON
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return data, defaultBoundary, err
	}

	part.Write(filesForUpload)

	for _, attachment := range attachments {

		header := make(textproto.MIMEHeader)
		part, err2 := writer.CreatePart(header)
		if err2 != nil {
			return data, defaultBoundary, err2
		}
		part.Write(attachment.AttachmentBytes)

	}

	err = writer.Close()
	if err != nil {
		return data, defaultBoundary, err
	}

	return data, defaultBoundary, nil

}

func getRevisionHeader(resp *http.Response) (string, error) {

	revision := resp.Header.Get("Etag")

	if revision == "" {
		return "", fmt.Errorf("No revision tag detected")
	}

	reg := regexp.MustCompile(`"([^"]*)"`)
	revisionNoQuotes := reg.ReplaceAllString(revision, "${1}")
	return revisionNoQuotes, nil

}

//ReadDoc method provides function to retrieve a document from the database by id
func (dbclient *CouchDBConnectionDef) ReadDoc(id string) ([]byte, string, error) {

	logger.Debugf("===COUCHDB=== Entering ReadDoc()  id=%s", id)

	url := fmt.Sprintf("%s/%s/%s?attachments=true", dbclient.URL, dbclient.Database, id)

	resp, _, err := dbclient.handleRequest(http.MethodGet, url, nil, "", "")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	/*
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s", dump)
	*/

	//Get the media type from the Content-Type header
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		log.Fatal(err)
	}

	//Get the revision from header
	revision, err := getRevisionHeader(resp)
	if err != nil {
		return nil, "", err
	}

	//check to see if the is multipart,  handle as attachment if multipart is detected
	if strings.HasPrefix(mediaType, "multipart/") {

		//Set up the multipart reader based on the boundary
		multipartReader := multipart.NewReader(resp.Body, params["boundary"])
		for {

			p, err := multipartReader.NextPart()

			if err == io.EOF {
				return nil, "", err
			}

			if err != nil {
				return nil, "", err
			}

			logger.Debugf("===COUCHDB=== part header=%s", p.Header)

			//See if the part is gzip encoded
			switch p.Header.Get("Content-Encoding") {
			case "gzip":

				var respBody []byte

				gr, err := gzip.NewReader(p)
				if err != nil {
					return nil, "", err
				}
				respBody, err = ioutil.ReadAll(gr)
				if err != nil {
					return nil, "", err
				}

				logger.Debugf("===COUCHDB=== Retrieved attachment data")

				if p.Header.Get("Content-Disposition") == "attachment; filename=\"valueBytes\"" {

					return respBody, revision, nil

				}

			default:

				//retrieve the data,  this is not gzip
				partdata, err := ioutil.ReadAll(p)
				if err != nil {
					return nil, "", err
				}
				logger.Debugf("===COUCHDB=== Retrieved attachment data")

				if p.Header.Get("Content-Disposition") == "attachment; filename=\"valueBytes\"" {

					return partdata, revision, nil

				}

			}

		}

	} else {

		//handle as JSON document
		jsonDoc, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}

		logger.Debugf("===COUCHDB=== Read document, id=%s, value=%s", id, string(jsonDoc))

		logger.Debugf("===COUCHDB=== Exiting ReadDoc()")

		return jsonDoc, revision, nil

	}

}

//handleRequest method is a generic http request handler
func (dbclient *CouchDBConnectionDef) handleRequest(method, url string, data io.Reader, rev string, multipartBoundary string) (*http.Response, *DBReturn, error) {

	logger.Debugf("===COUCHDB=== Entering handleRequest()  method=%s  url=%s", method, url)

	//Create request based on URL for couchdb operation
	req, err := http.NewRequest(method, url, data)
	if err != nil {
		return nil, nil, err
	}

	//add content header for PUT
	if method == http.MethodPut {

		//If the multipartBoundary is not set, then this is a JSON and content-type should be set
		//to application/json.   Else, this is contains an attachment and needs to be multipart
		if multipartBoundary == "" {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "multipart/related;boundary=\""+multipartBoundary+"\"")
		}

		//check to see if the revision is set,  if so, pass as a header
		if rev != "" {
			req.Header.Set("If-Match", rev)
		}
	}

	//add content header for PUT
	if method == http.MethodPut {
		req.Header.Set("Accept", "application/json")
	}

	//add content header for GET
	if method == http.MethodGet {
		req.Header.Set("Accept", "multipart/related")
	}

	//If username and password are set the use basic auth
	if dbclient.Username != "" && dbclient.Password != "" {
		req.SetBasicAuth(dbclient.Username, dbclient.Password)
	}

	/*
		dump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s", dump)
	*/

	//Create the http client
	client := &http.Client{}

	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	transport.DisableCompression = false
	client.Transport = transport

	//Execute http request
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	//create the return object for couchDB
	couchDBReturn := &DBReturn{}

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

//IsJSON tests a string to determine if a valid JSON
func IsJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
