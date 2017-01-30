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
	"net/http/httputil"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
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

//RangeQueryResponse is used for processing REST range query responses from CouchDB
type RangeQueryResponse struct {
	TotalRows int `json:"total_rows"`
	Offset    int `json:"offset"`
	Rows      []struct {
		ID    string `json:"id"`
		Key   string `json:"key"`
		Value struct {
			Rev string `json:"rev"`
		} `json:"value"`
		Doc json.RawMessage `json:"doc"`
	} `json:"rows"`
}

//QueryResponse is used for processing REST query responses from CouchDB
type QueryResponse struct {
	Warning string            `json:"warning"`
	Docs    []json.RawMessage `json:"docs"`
}

//Doc is used for capturing if attachments are return in the query from CouchDB
type Doc struct {
	ID          string          `json:"_id"`
	Rev         string          `json:"_rev"`
	Attachments json.RawMessage `json:"_attachments"`
}

//DocID is a minimal structure for capturing the ID from a query result
type DocID struct {
	ID string `json:"_id"`
}

//QueryResult is used for returning query results from CouchDB
type QueryResult struct {
	ID      string
	Version *version.Height
	Value   []byte
}

//CouchConnectionDef contains parameters
type CouchConnectionDef struct {
	URL      string
	Username string
	Password string
}

//CouchInstance represents a CouchDB instance
type CouchInstance struct {
	conf CouchConnectionDef //connection configuration
}

//CouchDatabase represents a database within a CouchDB instance
type CouchDatabase struct {
	couchInstance CouchInstance //connection configuration
	dbName        string
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
func CreateConnectionDefinition(couchDBAddress, username, password string) (*CouchConnectionDef, error) {

	logger.Debugf("Entering CreateConnectionDefinition()")

	//connectURL := fmt.Sprintf("%s//%s", "http:", couchDBAddress)
	//connectURL := couchDBAddress

	connectURL := &url.URL{
		Host:   couchDBAddress,
		Scheme: "http",
	}

	//parse the constructed URL to verify no errors
	finalURL, err := url.Parse(connectURL.String())
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}

	logger.Debugf("Created database configuration  URL=[%s]", finalURL.String())
	logger.Debugf("Exiting CreateConnectionDefinition()")

	//return an object containing the connection information
	return &CouchConnectionDef{finalURL.String(), username, password}, nil
}

//CreateDatabaseIfNotExist method provides function to create database
func (dbclient *CouchDatabase) CreateDatabaseIfNotExist() (*DBOperationResponse, error) {

	logger.Debugf("Entering CreateDatabaseIfNotExist()")

	dbInfo, couchDBReturn, err := dbclient.GetDatabaseInfo()
	if err != nil {
		if couchDBReturn == nil || couchDBReturn.StatusCode != 404 {
			return nil, err
		}
	}

	if dbInfo == nil && couchDBReturn.StatusCode == 404 {

		logger.Debugf("Database %s does not exist.", dbclient.dbName)

		connectURL, err := url.Parse(dbclient.couchInstance.conf.URL)
		if err != nil {
			logger.Errorf("URL parse error: %s", err.Error())
			return nil, err
		}
		connectURL.Path = dbclient.dbName

		//process the URL with a PUT, creates the database
		resp, _, err := dbclient.handleRequest(http.MethodPut, connectURL.String(), nil, "", "")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		//Get the response from the create REST call
		dbResponse := &DBOperationResponse{}
		json.NewDecoder(resp.Body).Decode(&dbResponse)

		if dbResponse.Ok == true {
			logger.Debugf("Created database %s ", dbclient.dbName)
		}

		logger.Debugf("Exiting CreateDatabaseIfNotExist()")

		return dbResponse, nil

	}

	logger.Debugf("Database %s already exists", dbclient.dbName)

	logger.Debugf("Exiting CreateDatabaseIfNotExist()")

	return nil, nil

}

//GetDatabaseInfo method provides function to retrieve database information
func (dbclient *CouchDatabase) GetDatabaseInfo() (*DBInfo, *DBReturn, error) {

	connectURL, err := url.Parse(dbclient.couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, nil, err
	}
	connectURL.Path = dbclient.dbName

	resp, couchDBReturn, err := dbclient.handleRequest(http.MethodGet, connectURL.String(), nil, "", "")
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
			logger.Debugf("GetDatabaseInfo() dbResponseJSON: %s", dbResponseJSON)
		}
	}

	return dbResponse, couchDBReturn, nil

}

//DropDatabase provides method to drop an existing database
func (dbclient *CouchDatabase) DropDatabase() (*DBOperationResponse, error) {

	logger.Debugf("Entering DropDatabase()")

	connectURL, err := url.Parse(dbclient.couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	connectURL.Path = dbclient.dbName

	resp, _, err := dbclient.handleRequest(http.MethodDelete, connectURL.String(), nil, "", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dbResponse := &DBOperationResponse{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	if dbResponse.Ok == true {
		logger.Debugf("Dropped database %s ", dbclient.dbName)
	}

	logger.Debugf("Exiting DropDatabase()")

	if dbResponse.Ok == true {

		return dbResponse, nil

	}

	return dbResponse, fmt.Errorf("Error dropping database")

}

// EnsureFullCommit calls _ensure_full_commit for explicit fsync
func (dbclient *CouchDatabase) EnsureFullCommit() (*DBOperationResponse, error) {

	logger.Debugf("Entering EnsureFullCommit()")

	url := fmt.Sprintf("%s/%s/_ensure_full_commit", dbclient.couchInstance.conf.URL, dbclient.dbName)

	resp, _, err := dbclient.handleRequest(http.MethodPost, url, nil, "", "")
	if err != nil {
		logger.Errorf("Failed to invoke _ensure_full_commit Error: %s\n", err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	dbResponse := &DBOperationResponse{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	if dbResponse.Ok == true {
		logger.Debugf("_ensure_full_commit database %s ", dbclient.dbName)
	}

	logger.Debugf("Exiting EnsureFullCommit()")

	if dbResponse.Ok == true {

		return dbResponse, nil

	}

	return dbResponse, fmt.Errorf("Error syncing database")
}

//SaveDoc method provides a function to save a document, id and byte array
func (dbclient *CouchDatabase) SaveDoc(id string, rev string, bytesDoc []byte, attachments []Attachment) (string, error) {

	logger.Debugf("Entering SaveDoc()")

	saveURL, err := url.Parse(dbclient.couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return "", err
	}
	saveURL.Path = dbclient.dbName + "/" + id

	logger.Debugf("  id=%s,  value=%s", id, string(bytesDoc))

	if rev == "" {

		//See if the document already exists, we need the rev for save
		_, revdoc, err2 := dbclient.ReadDoc(id)
		if err2 != nil {
			//set the revision to indicate that the document was not found
			rev = ""
		} else {
			//set the revision to the rev returned from the document read
			rev = revdoc
		}
	}

	logger.Debugf("  rev=%s", rev)

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

	} else { // there are attachments

		//attachments are included, create the multipart definition
		multipartData, multipartBoundary, err3 := createAttachmentPart(bytesDoc, attachments, defaultBoundary)
		if err3 != nil {
			return "", err3
		}

		//Set the data buffer to the data from the create multi-part data
		data.ReadFrom(&multipartData)

		//Set the default boundary to the value generated in the multipart creation
		defaultBoundary = multipartBoundary

	}

	//handle the request for saving the JSON or attachments
	resp, _, err := dbclient.handleRequest(http.MethodPut, saveURL.String(), data, rev, defaultBoundary)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	//get the revision and return
	revision, err := getRevisionHeader(resp)
	if err != nil {
		return "", err
	}

	logger.Debugf("Exiting SaveDoc()")

	return revision, nil

}

func createAttachmentPart(data []byte, attachments []Attachment, defaultBoundary string) (bytes.Buffer, string, error) {

	//Create a buffer for writing the result
	writeBuffer := new(bytes.Buffer)

	// read the attachment and save as an attachment
	writer := multipart.NewWriter(writeBuffer)

	//retrieve the boundary for the multipart
	defaultBoundary = writer.Boundary()

	fileAttachments := map[string]FileDetails{}

	for _, attachment := range attachments {
		fileAttachments[attachment.Name] = FileDetails{true, attachment.ContentType, len(attachment.AttachmentBytes)}
	}

	attachmentJSONMap := map[string]interface{}{
		"_attachments": fileAttachments}

	//Add any data uploaded with the files
	if data != nil {

		//create a generic map
		genericMap := make(map[string]interface{})
		//unmarshal the data into the generic map
		json.Unmarshal(data, &genericMap)

		//add all key/values to the attachmentJSONMap
		for jsonKey, jsonValue := range genericMap {
			attachmentJSONMap[jsonKey] = jsonValue
		}

	}

	filesForUpload, _ := json.Marshal(attachmentJSONMap)
	logger.Debugf(string(filesForUpload))

	//create the header for the JSON
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return *writeBuffer, defaultBoundary, err
	}

	part.Write(filesForUpload)

	for _, attachment := range attachments {

		header := make(textproto.MIMEHeader)
		part, err2 := writer.CreatePart(header)
		if err2 != nil {
			return *writeBuffer, defaultBoundary, err2
		}
		part.Write(attachment.AttachmentBytes)

	}

	err = writer.Close()
	if err != nil {
		return *writeBuffer, defaultBoundary, err
	}

	return *writeBuffer, defaultBoundary, nil

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
func (dbclient *CouchDatabase) ReadDoc(id string) ([]byte, string, error) {

	logger.Debugf("Entering ReadDoc()  id=%s", id)

	readURL, err := url.Parse(dbclient.couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, "", err
	}
	readURL.Path = dbclient.dbName + "/" + id

	query := readURL.Query()
	query.Add("attachments", "true")

	readURL.RawQuery = query.Encode()

	resp, couchDBReturn, err := dbclient.handleRequest(http.MethodGet, readURL.String(), nil, "", "")
	if err != nil {
		fmt.Printf("couchDBReturn=%v", couchDBReturn)
		if couchDBReturn != nil && couchDBReturn.StatusCode == 404 {
			logger.Debug("Document not found (404), returning nil value instead of 404 error")
			// non-existent document should return nil value instead of a 404 error
			// for details see https://github.com/hyperledger-archives/fabric/issues/936
			return nil, "", nil
		}
		return nil, "", err
	}
	defer resp.Body.Close()

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

			logger.Debugf("part header=%s", p.Header)

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

				logger.Debugf("Retrieved attachment data")

				if p.Header.Get("Content-Disposition") == "attachment; filename=\"valueBytes\"" {

					return respBody, revision, nil

				}

			default:

				//retrieve the data,  this is not gzip
				partdata, err := ioutil.ReadAll(p)
				if err != nil {
					return nil, "", err
				}
				logger.Debugf("Retrieved attachment data")

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

		logger.Debugf("Read document, id=%s, value=%s", id, string(jsonDoc))

		logger.Debugf("Exiting ReadDoc()")

		return jsonDoc, revision, nil

	}

}

//ReadDocRange method provides function to a range of documents based on the start and end keys
//startKey and endKey can also be empty strings.  If startKey and endKey are empty, all documents are returned
//TODO This function provides a limit option to specify the max number of entries.   This will
//need to be added to configuration options.  Skip will not be used by Fabric since a consistent
//result set is required
func (dbclient *CouchDatabase) ReadDocRange(startKey, endKey string, limit, skip int) (*[]QueryResult, error) {

	logger.Debugf("Entering ReadDocRange()  startKey=%s, endKey=%s", startKey, endKey)

	var results []QueryResult

	rangeURL, err := url.Parse(dbclient.couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	rangeURL.Path = dbclient.dbName + "/_all_docs"

	queryParms := rangeURL.Query()
	queryParms.Set("limit", strconv.Itoa(limit))
	queryParms.Add("skip", strconv.Itoa(skip))
	queryParms.Add("include_docs", "true")
	queryParms.Add("inclusive_end", "false") // endkey should be exclusive to be consistent with goleveldb

	//Append the startKey if provided
	if startKey != "" {
		startKey = strconv.QuoteToGraphic(startKey)
		startKey = strings.Replace(startKey, "\\x00", "\\u0000", -1)
		startKey = strings.Replace(startKey, "\\x1e", "\\u001e", -1)
		startKey = strings.Replace(startKey, "\\x1f", "\\u001f", -1)
		startKey = strings.Replace(startKey, "\\xff", "\\u00ff", -1)
		//TODO add general unicode support instead of special cases

		queryParms.Add("startkey", startKey)
	}

	//Append the endKey if provided
	if endKey != "" {
		endKey = strconv.QuoteToGraphic(endKey)
		endKey = strings.Replace(endKey, "\\x00", "\\u0000", -1)
		endKey = strings.Replace(endKey, "\\x01", "\\u0001", -1)
		endKey = strings.Replace(endKey, "\\x1e", "\\u001e", -1)
		endKey = strings.Replace(endKey, "\\x1f", "\\u001f", -1)
		endKey = strings.Replace(endKey, "\\xff", "\\u00ff", -1)
		//TODO add general unicode support instead of special cases

		queryParms.Add("endkey", endKey)
	}

	rangeURL.RawQuery = queryParms.Encode()

	resp, _, err := dbclient.handleRequest(http.MethodGet, rangeURL.String(), nil, "", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if logger.IsEnabledFor(logging.DEBUG) {
		dump, err2 := httputil.DumpResponse(resp, true)
		if err2 != nil {
			log.Fatal(err2)
		}
		logger.Debugf("%s", dump)
	}

	//handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonResponse = &RangeQueryResponse{}
	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	logger.Debugf("Total Rows: %d", jsonResponse.TotalRows)

	for _, row := range jsonResponse.Rows {

		var jsonDoc = &Doc{}
		err3 := json.Unmarshal(row.Doc, &jsonDoc)
		if err3 != nil {
			return nil, err3
		}

		if jsonDoc.Attachments != nil {

			logger.Debugf("Adding binary docment for id: %s", jsonDoc.ID)

			binaryDocument, _, err := dbclient.ReadDoc(jsonDoc.ID)
			if err != nil {
				return nil, err
			}

			var addDocument = &QueryResult{jsonDoc.ID, version.NewHeight(1, 1), binaryDocument}

			results = append(results, *addDocument)

		} else {

			logger.Debugf("Adding json docment for id: %s", jsonDoc.ID)

			var addDocument = &QueryResult{jsonDoc.ID, version.NewHeight(1, 1), row.Doc}

			results = append(results, *addDocument)

		}

	}

	logger.Debugf("Exiting ReadDocRange()")

	return &results, nil

}

//QueryDocuments method provides function for processing a query
func (dbclient *CouchDatabase) QueryDocuments(query string, limit, skip int) (*[]QueryResult, error) {

	logger.Debugf("Entering QueryDocuments()  query=%s", query)

	var results []QueryResult

	queryURL, err := url.Parse(dbclient.couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}

	queryURL.Path = dbclient.dbName + "/_find"

	queryParms := queryURL.Query()
	queryParms.Set("limit", strconv.Itoa(limit))
	queryParms.Add("skip", strconv.Itoa(skip))

	queryURL.RawQuery = queryParms.Encode()

	//Set up a buffer for the data to be pushed to couchdb
	data := new(bytes.Buffer)

	data.ReadFrom(bytes.NewReader([]byte(query)))

	resp, _, err := dbclient.handleRequest(http.MethodPost, queryURL.String(), data, "", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if logger.IsEnabledFor(logging.DEBUG) {
		dump, err2 := httputil.DumpResponse(resp, true)
		if err2 != nil {
			log.Fatal(err2)
		}
		logger.Debugf("%s", dump)
	}

	//handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonResponse = &QueryResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	for _, row := range jsonResponse.Docs {

		var jsonDoc = &DocID{}
		err3 := json.Unmarshal(row, &jsonDoc)
		if err3 != nil {
			return nil, err3
		}

		logger.Debugf("Adding row to resultset: %s", row)

		//TODO Replace the temporary NewHeight version when available
		var addDocument = &QueryResult{jsonDoc.ID, version.NewHeight(1, 1), row}

		results = append(results, *addDocument)

	}

	logger.Debugf("Exiting QueryDocuments()")

	return &results, nil

}

//handleRequest method is a generic http request handler
func (dbclient *CouchDatabase) handleRequest(method, connectURL string, data io.Reader, rev string, multipartBoundary string) (*http.Response, *DBReturn, error) {

	logger.Debugf("Entering handleRequest()  method=%s  url=%v", method, connectURL)

	//Create request based on URL for couchdb operation
	req, err := http.NewRequest(method, connectURL, data)
	if err != nil {
		return nil, nil, err
	}

	//add content header for PUT
	if method == http.MethodPut || method == http.MethodPost {

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
	if method == http.MethodPut || method == http.MethodPost {
		req.Header.Set("Accept", "application/json")
	}

	//add content header for GET
	if method == http.MethodGet {
		req.Header.Set("Accept", "multipart/related")
	}

	//If username and password are set the use basic auth
	if dbclient.couchInstance.conf.Username != "" && dbclient.couchInstance.conf.Password != "" {
		req.SetBasicAuth(dbclient.couchInstance.conf.Username, dbclient.couchInstance.conf.Password)
	}

	if logger.IsEnabledFor(logging.DEBUG) {
		dump, err2 := httputil.DumpRequestOut(req, true)
		if err2 != nil {
			log.Fatal(err2)
		}
		// trace the first 200 bytes of http request only, in case it is huge
		if dump != nil {
			if len(dump) <= 200 {
				logger.Debugf("HTTP Request: %s", dump)
			} else {
				logger.Debugf("HTTP Request: %s...", dump[0:200])
			}
		}
	}

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

		logger.Debugf("Couch DB error  status code=%v  error=%s", resp.StatusCode, jsonError)

		errorBytes := []byte(jsonError)

		json.Unmarshal(errorBytes, &couchDBReturn)

		return nil, couchDBReturn, fmt.Errorf("Couch DB Error: %s", couchDBReturn.Reason)

	}

	logger.Debugf("Exiting handleRequest()")

	//If no errors, then return the results
	return resp, couchDBReturn, nil
}

//IsJSON tests a string to determine if a valid JSON
func IsJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
