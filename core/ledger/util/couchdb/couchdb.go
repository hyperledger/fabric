/*
Copyright IBM Corp. 2016, 2017 All Rights Reserved.

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
	"encoding/base64"
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
	"time"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/flogging"
	logging "github.com/op/go-logging"
)

var logger = flogging.MustGetLogger("couchdb")

//time between retry attempts in milliseconds
const retryWaitTime = 125

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

//ConnectionInfo is a structure for capturing the database info and version
type ConnectionInfo struct {
	Couchdb string `json:"couchdb"`
	Version string `json:"version"`
	Vendor  struct {
		Name string `json:"name"`
	} `json:"vendor"`
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
	ID          string
	Value       []byte
	Attachments []*Attachment
}

//CouchConnectionDef contains parameters
type CouchConnectionDef struct {
	URL                 string
	Username            string
	Password            string
	MaxRetries          int
	MaxRetriesOnStartup int
	RequestTimeout      time.Duration
}

//CouchInstance represents a CouchDB instance
type CouchInstance struct {
	conf   CouchConnectionDef //connection configuration
	client *http.Client       // a client to connect to this instance
}

//CouchDatabase represents a database within a CouchDB instance
type CouchDatabase struct {
	CouchInstance CouchInstance //connection configuration
	DBName        string
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

//DocMetadata returns the ID, version and revision for a couchdb document
type DocMetadata struct {
	ID      string
	Rev     string
	Version string
}

//FileDetails defines the structure needed to send an attachment to couchdb
type FileDetails struct {
	Follows     bool   `json:"follows"`
	ContentType string `json:"content_type"`
	Length      int    `json:"length"`
}

//CouchDoc defines the structure for a JSON document value
type CouchDoc struct {
	JSONValue   []byte
	Attachments []*Attachment
}

//BatchRetrieveDocMedatadataResponse is used for processing REST batch responses from CouchDB
type BatchRetrieveDocMedatadataResponse struct {
	Rows []struct {
		ID  string `json:"id"`
		Doc struct {
			ID      string `json:"_id"`
			Rev     string `json:"_rev"`
			Version string `json:"version"`
		} `json:"doc"`
	} `json:"rows"`
}

//BatchUpdateResponse defines a structure for batch update response
type BatchUpdateResponse struct {
	ID     string `json:"id"`
	Error  string `json:"error"`
	Reason string `json:"reason"`
	Ok     bool   `json:"ok"`
	Rev    string `json:"rev"`
}

//Base64Attachment contains the definition for an attached file for couchdb
type Base64Attachment struct {
	ContentType    string `json:"content_type"`
	AttachmentData string `json:"data"`
}

// closeResponseBody discards the body and then closes it to enable returning it to
// connection pool
func closeResponseBody(resp *http.Response) {
	io.Copy(ioutil.Discard, resp.Body) // discard whatever is remaining of body
	resp.Body.Close()
}

//CreateConnectionDefinition for a new client connection
func CreateConnectionDefinition(couchDBAddress, username, password string, maxRetries,
	maxRetriesOnStartup int, requestTimeout time.Duration) (*CouchConnectionDef, error) {

	logger.Debugf("Entering CreateConnectionDefinition()")

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
	return &CouchConnectionDef{finalURL.String(), username, password, maxRetries,
		maxRetriesOnStartup, requestTimeout}, nil

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

		logger.Debugf("Database %s does not exist.", dbclient.DBName)

		connectURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
		if err != nil {
			logger.Errorf("URL parse error: %s", err.Error())
			return nil, err
		}
		connectURL.Path = dbclient.DBName

		//get the number of retries
		maxRetries := dbclient.CouchInstance.conf.MaxRetries

		//process the URL with a PUT, creates the database
		resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPut, connectURL.String(), nil, "", "", maxRetries, true)
		if err != nil {
			return nil, err
		}
		defer closeResponseBody(resp)

		//Get the response from the create REST call
		dbResponse := &DBOperationResponse{}
		json.NewDecoder(resp.Body).Decode(&dbResponse)

		if dbResponse.Ok == true {
			logger.Debugf("Created database %s ", dbclient.DBName)
		}

		logger.Debugf("Exiting CreateDatabaseIfNotExist()")

		return dbResponse, nil

	}

	logger.Debugf("Database %s already exists", dbclient.DBName)

	logger.Debugf("Exiting CreateDatabaseIfNotExist()")

	return nil, nil

}

//GetDatabaseInfo method provides function to retrieve database information
func (dbclient *CouchDatabase) GetDatabaseInfo() (*DBInfo, *DBReturn, error) {

	connectURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, nil, err
	}
	connectURL.Path = dbclient.DBName

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, couchDBReturn, err := dbclient.CouchInstance.handleRequest(http.MethodGet, connectURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		return nil, couchDBReturn, err
	}
	defer closeResponseBody(resp)

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

//VerifyCouchConfig method provides function to verify the connection information
func (couchInstance *CouchInstance) VerifyCouchConfig() (*ConnectionInfo, *DBReturn, error) {

	logger.Debugf("Entering VerifyCouchConfig()")
	defer logger.Debugf("Exiting VerifyCouchConfig()")

	connectURL, err := url.Parse(couchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, nil, err
	}
	connectURL.Path = "/"

	//get the number of retries for startup
	maxRetriesOnStartup := couchInstance.conf.MaxRetriesOnStartup

	resp, couchDBReturn, err := couchInstance.handleRequest(http.MethodGet, connectURL.String(), nil,
		couchInstance.conf.Username, couchInstance.conf.Password, maxRetriesOnStartup, true)

	if err != nil {
		return nil, couchDBReturn, fmt.Errorf("Unable to connect to CouchDB, check the hostname and port: %s", err.Error())
	}
	defer closeResponseBody(resp)

	dbResponse := &ConnectionInfo{}
	errJSON := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if errJSON != nil {
		return nil, nil, fmt.Errorf("Unable to connect to CouchDB, check the hostname and port: %s", errJSON.Error())
	}

	// trace the database info response
	if logger.IsEnabledFor(logging.DEBUG) {
		dbResponseJSON, err := json.Marshal(dbResponse)
		if err == nil {
			logger.Debugf("VerifyConnection() dbResponseJSON: %s", dbResponseJSON)
		}
	}

	//check to see if the system databases exist
	//Verifying the existence of the system database accomplishes two steps
	//1.  Ensures the system databases are created
	//2.  Verifies the username password provided in the CouchDB config are valid for system admin
	err = CreateSystemDatabasesIfNotExist(*couchInstance)
	if err != nil {
		logger.Errorf("Unable to connect to CouchDB,  error: %s   Check the admin username and password.\n", err.Error())
		return nil, nil, fmt.Errorf("Unable to connect to CouchDB,  error: %s   Check the admin username and password.\n", err.Error())
	}

	return dbResponse, couchDBReturn, nil
}

//DropDatabase provides method to drop an existing database
func (dbclient *CouchDatabase) DropDatabase() (*DBOperationResponse, error) {

	logger.Debugf("Entering DropDatabase()")

	connectURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	connectURL.Path = dbclient.DBName

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodDelete, connectURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	dbResponse := &DBOperationResponse{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	if dbResponse.Ok == true {
		logger.Debugf("Dropped database %s ", dbclient.DBName)
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

	connectURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	connectURL.Path = dbclient.DBName + "/_ensure_full_commit"

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, connectURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		logger.Errorf("Failed to invoke _ensure_full_commit Error: %s\n", err.Error())
		return nil, err
	}
	defer closeResponseBody(resp)

	dbResponse := &DBOperationResponse{}
	json.NewDecoder(resp.Body).Decode(&dbResponse)

	if dbResponse.Ok == true {
		logger.Debugf("_ensure_full_commit database %s ", dbclient.DBName)
	}

	logger.Debugf("Exiting EnsureFullCommit()")

	if dbResponse.Ok == true {

		return dbResponse, nil

	}

	return dbResponse, fmt.Errorf("Error syncing database")
}

//SaveDoc method provides a function to save a document, id and byte array
func (dbclient *CouchDatabase) SaveDoc(id string, rev string, couchDoc *CouchDoc) (string, error) {

	logger.Debugf("Entering SaveDoc()  id=[%s]", id)

	if !utf8.ValidString(id) {
		return "", fmt.Errorf("doc id [%x] not a valid utf8 string", id)
	}

	saveURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return "", err
	}

	saveURL.Path = dbclient.DBName
	// id can contain a '/', so encode separately
	saveURL = &url.URL{Opaque: saveURL.String() + "/" + encodePathElement(id)}

	logger.Debugf("  rev=%s", rev)

	//Set up a buffer for the data to be pushed to couchdb
	data := []byte{}

	//Set up a default boundary for use by multipart if sending attachments
	defaultBoundary := ""

	//Create a flag for shared connections.  This is set to false for zero length attachments
	keepConnectionOpen := true

	//check to see if attachments is nil, if so, then this is a JSON only
	if couchDoc.Attachments == nil {

		//Test to see if this is a valid JSON
		if IsJSON(string(couchDoc.JSONValue)) != true {
			return "", fmt.Errorf("JSON format is not valid")
		}

		// if there are no attachments, then use the bytes passed in as the JSON
		data = couchDoc.JSONValue

	} else { // there are attachments

		//attachments are included, create the multipart definition
		multipartData, multipartBoundary, err3 := createAttachmentPart(couchDoc, defaultBoundary)
		if err3 != nil {
			return "", err3
		}

		//If there is a zero length attachment, do not keep the connection open
		for _, attach := range couchDoc.Attachments {
			if attach.Length < 1 {
				keepConnectionOpen = false
			}
		}

		//Set the data buffer to the data from the create multi-part data
		data = multipartData.Bytes()

		//Set the default boundary to the value generated in the multipart creation
		defaultBoundary = multipartBoundary

	}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	//handle the request for saving document with a retry if there is a revision conflict
	resp, _, err := dbclient.handleRequestWithRevisionRetry(id, http.MethodPut,
		*saveURL, data, rev, defaultBoundary, maxRetries, keepConnectionOpen)

	if err != nil {
		return "", err
	}
	defer closeResponseBody(resp)

	//get the revision and return
	revision, err := getRevisionHeader(resp)
	if err != nil {
		return "", err
	}

	logger.Debugf("Exiting SaveDoc()")

	return revision, nil

}

//getDocumentRevision will return the revision if the document exists, otherwise it will return ""
func (dbclient *CouchDatabase) getDocumentRevision(id string) string {

	var rev = ""

	//See if the document already exists, we need the rev for saves and deletes
	_, revdoc, err := dbclient.ReadDoc(id)
	if err == nil {
		//set the revision to the rev returned from the document read
		rev = revdoc
	}
	return rev
}

func createAttachmentPart(couchDoc *CouchDoc, defaultBoundary string) (bytes.Buffer, string, error) {

	//Create a buffer for writing the result
	writeBuffer := new(bytes.Buffer)

	// read the attachment and save as an attachment
	writer := multipart.NewWriter(writeBuffer)

	//retrieve the boundary for the multipart
	defaultBoundary = writer.Boundary()

	fileAttachments := map[string]FileDetails{}

	for _, attachment := range couchDoc.Attachments {
		fileAttachments[attachment.Name] = FileDetails{true, attachment.ContentType, len(attachment.AttachmentBytes)}
	}

	attachmentJSONMap := map[string]interface{}{
		"_attachments": fileAttachments}

	//Add any data uploaded with the files
	if couchDoc.JSONValue != nil {

		//create a generic map
		genericMap := make(map[string]interface{})

		//unmarshal the data into the generic map
		decoder := json.NewDecoder(bytes.NewBuffer(couchDoc.JSONValue))
		decoder.UseNumber()
		decoder.Decode(&genericMap)

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

	for _, attachment := range couchDoc.Attachments {

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

//ReadDoc method provides function to retrieve a document and its revision
//from the database by id
func (dbclient *CouchDatabase) ReadDoc(id string) (*CouchDoc, string, error) {
	var couchDoc CouchDoc
	attachments := []*Attachment{}

	logger.Debugf("Entering ReadDoc()  id=[%s]", id)
	if !utf8.ValidString(id) {
		return nil, "", fmt.Errorf("doc id [%x] not a valid utf8 string", id)
	}

	readURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, "", err
	}
	readURL.Path = dbclient.DBName
	// id can contain a '/', so encode separately
	readURL = &url.URL{Opaque: readURL.String() + "/" + encodePathElement(id)}

	query := readURL.Query()
	query.Add("attachments", "true")

	readURL.RawQuery = query.Encode()

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, couchDBReturn, err := dbclient.CouchInstance.handleRequest(http.MethodGet, readURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		if couchDBReturn != nil && couchDBReturn.StatusCode == 404 {
			logger.Debug("Document not found (404), returning nil value instead of 404 error")
			// non-existent document should return nil value instead of a 404 error
			// for details see https://github.com/hyperledger-archives/fabric/issues/936
			return nil, "", nil
		}
		logger.Debugf("couchDBReturn=%v\n", couchDBReturn)
		return nil, "", err
	}
	defer closeResponseBody(resp)

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
				break // processed all parts
			}
			if err != nil {
				return nil, "", err
			}

			defer p.Close()

			logger.Debugf("part header=%s", p.Header)
			switch p.Header.Get("Content-Type") {
			case "application/json":
				partdata, err := ioutil.ReadAll(p)
				if err != nil {
					return nil, "", err
				}
				couchDoc.JSONValue = partdata
			default:

				//Create an attachment structure and load it
				attachment := &Attachment{}
				attachment.ContentType = p.Header.Get("Content-Type")
				contentDispositionParts := strings.Split(p.Header.Get("Content-Disposition"), ";")
				if strings.TrimSpace(contentDispositionParts[0]) == "attachment" {
					switch p.Header.Get("Content-Encoding") {
					case "gzip": //See if the part is gzip encoded

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
						attachment.AttachmentBytes = respBody
						attachment.Name = p.FileName()
						attachments = append(attachments, attachment)

					default:

						//retrieve the data,  this is not gzip
						partdata, err := ioutil.ReadAll(p)
						if err != nil {
							return nil, "", err
						}
						logger.Debugf("Retrieved attachment data")
						attachment.AttachmentBytes = partdata
						attachment.Name = p.FileName()
						attachments = append(attachments, attachment)

					} // end content-encoding switch
				} // end if attachment
			} // end content-type switch
		} // for all multiparts

		couchDoc.Attachments = attachments

		return &couchDoc, revision, nil
	}

	//handle as JSON document
	couchDoc.JSONValue, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	logger.Debugf("Exiting ReadDoc()")
	return &couchDoc, revision, nil
}

//ReadDocRange method provides function to a range of documents based on the start and end keys
//startKey and endKey can also be empty strings.  If startKey and endKey are empty, all documents are returned
//This function provides a limit option to specify the max number of entries and is supplied by config.
//Skip is reserved for possible future future use.
func (dbclient *CouchDatabase) ReadDocRange(startKey, endKey string, limit, skip int) (*[]QueryResult, error) {

	logger.Debugf("Entering ReadDocRange()  startKey=%s, endKey=%s", startKey, endKey)

	var results []QueryResult

	rangeURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	rangeURL.Path = dbclient.DBName + "/_all_docs"

	queryParms := rangeURL.Query()
	queryParms.Set("limit", strconv.Itoa(limit))
	queryParms.Add("skip", strconv.Itoa(skip))
	queryParms.Add("include_docs", "true")
	queryParms.Add("inclusive_end", "false") // endkey should be exclusive to be consistent with goleveldb

	//Append the startKey if provided

	if startKey != "" {
		var err error
		if startKey, err = encodeForJSON(startKey); err != nil {
			return nil, err
		}
		queryParms.Add("startkey", "\""+startKey+"\"")
	}

	//Append the endKey if provided
	if endKey != "" {
		var err error
		if endKey, err = encodeForJSON(endKey); err != nil {
			return nil, err
		}
		queryParms.Add("endkey", "\""+endKey+"\"")
	}

	rangeURL.RawQuery = queryParms.Encode()

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodGet, rangeURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

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

			logger.Debugf("Adding JSON document and attachments for id: %s", jsonDoc.ID)

			couchDoc, _, err := dbclient.ReadDoc(jsonDoc.ID)
			if err != nil {
				return nil, err
			}

			var addDocument = &QueryResult{jsonDoc.ID, couchDoc.JSONValue, couchDoc.Attachments}
			results = append(results, *addDocument)

		} else {

			logger.Debugf("Adding json docment for id: %s", jsonDoc.ID)

			var addDocument = &QueryResult{jsonDoc.ID, row.Doc, nil}
			results = append(results, *addDocument)

		}

	}

	logger.Debugf("Exiting ReadDocRange()")

	return &results, nil

}

//DeleteDoc method provides function to delete a document from the database by id
func (dbclient *CouchDatabase) DeleteDoc(id, rev string) error {

	logger.Debugf("Entering DeleteDoc()  id=%s", id)

	deleteURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return err
	}

	deleteURL.Path = dbclient.DBName
	// id can contain a '/', so encode separately
	deleteURL = &url.URL{Opaque: deleteURL.String() + "/" + encodePathElement(id)}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	//handle the request for saving document with a retry if there is a revision conflict
	resp, couchDBReturn, err := dbclient.handleRequestWithRevisionRetry(id, http.MethodDelete,
		*deleteURL, nil, "", "", maxRetries, true)

	if err != nil {
		if couchDBReturn != nil && couchDBReturn.StatusCode == 404 {
			logger.Debug("Document not found (404), returning nil value instead of 404 error")
			// non-existent document should return nil value instead of a 404 error
			// for details see https://github.com/hyperledger-archives/fabric/issues/936
			return nil
		}
		return err
	}
	defer closeResponseBody(resp)

	logger.Debugf("Exiting DeleteDoc()")

	return nil

}

//QueryDocuments method provides function for processing a query
func (dbclient *CouchDatabase) QueryDocuments(query string) (*[]QueryResult, error) {

	logger.Debugf("Entering QueryDocuments()  query=%s", query)

	var results []QueryResult

	queryURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}

	queryURL.Path = dbclient.DBName + "/_find"

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, queryURL.String(), []byte(query), "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

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

		var jsonDoc = &Doc{}
		err3 := json.Unmarshal(row, &jsonDoc)
		if err3 != nil {
			return nil, err3
		}

		if jsonDoc.Attachments != nil {

			logger.Debugf("Adding JSON docment and attachments for id: %s", jsonDoc.ID)

			couchDoc, _, err := dbclient.ReadDoc(jsonDoc.ID)
			if err != nil {
				return nil, err
			}
			var addDocument = &QueryResult{ID: jsonDoc.ID, Value: couchDoc.JSONValue, Attachments: couchDoc.Attachments}
			results = append(results, *addDocument)

		} else {
			logger.Debugf("Adding json docment for id: %s", jsonDoc.ID)
			var addDocument = &QueryResult{ID: jsonDoc.ID, Value: row, Attachments: nil}

			results = append(results, *addDocument)

		}
	}
	logger.Debugf("Exiting QueryDocuments()")

	return &results, nil

}

//BatchRetrieveIDRevision - batch method to retrieve IDs and revisions
func (dbclient *CouchDatabase) BatchRetrieveIDRevision(keys []string) ([]*DocMetadata, error) {

	batchURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	batchURL.Path = dbclient.DBName + "/_all_docs"

	queryParms := batchURL.Query()
	queryParms.Add("include_docs", "true")
	batchURL.RawQuery = queryParms.Encode()

	keymap := make(map[string]interface{})

	keymap["keys"] = keys

	jsonKeys, err := json.Marshal(keymap)
	if err != nil {
		return nil, err
	}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, batchURL.String(), jsonKeys, "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if logger.IsEnabledFor(logging.DEBUG) {
		dump, _ := httputil.DumpResponse(resp, false)
		// compact debug log by replacing carriage return / line feed with dashes to separate http headers
		logger.Debugf("HTTP Response: %s", bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
	}

	//handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonResponse = &BatchRetrieveDocMedatadataResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	revisionDocs := []*DocMetadata{}

	for _, row := range jsonResponse.Rows {
		revisionDoc := &DocMetadata{ID: row.ID, Rev: row.Doc.Rev, Version: row.Doc.Version}
		revisionDocs = append(revisionDocs, revisionDoc)
	}

	return revisionDocs, nil

}

//BatchUpdateDocuments - batch method to batch update documents
func (dbclient *CouchDatabase) BatchUpdateDocuments(documents []*CouchDoc) ([]*BatchUpdateResponse, error) {

	logger.Debugf("Entering BatchUpdateDocuments()  documents=%v", documents)

	batchURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	batchURL.Path = dbclient.DBName + "/_bulk_docs"

	documentMap := make(map[string]interface{})

	var jsonDocumentMap []interface{}

	for _, jsonDocument := range documents {

		//create a document map
		var document = make(map[string]interface{})

		//unmarshal the JSON component of the CouchDoc into the document
		json.Unmarshal(jsonDocument.JSONValue, &document)

		//iterate through any attachments
		if len(jsonDocument.Attachments) > 0 {

			//create a file attachment map
			fileAttachment := make(map[string]interface{})

			//for each attachment, create a Base64Attachment, name the attachment,
			//add the content type and base64 encode the attachment
			for _, attachment := range jsonDocument.Attachments {
				fileAttachment[attachment.Name] = Base64Attachment{attachment.ContentType,
					base64.StdEncoding.EncodeToString(attachment.AttachmentBytes)}
			}

			//add attachments to the document
			document["_attachments"] = fileAttachment

		}

		//Append the document to the map of documents
		jsonDocumentMap = append(jsonDocumentMap, document)

	}

	//Add the documents to the "docs" item
	documentMap["docs"] = jsonDocumentMap

	jsonKeys, err := json.Marshal(documentMap)

	if err != nil {
		return nil, err
	}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, batchURL.String(), jsonKeys, "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if logger.IsEnabledFor(logging.DEBUG) {
		dump, _ := httputil.DumpResponse(resp, false)
		// compact debug log by replacing carriage return / line feed with dashes to separate http headers
		logger.Debugf("HTTP Response: %s", bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
	}

	//handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonResponse = []*BatchUpdateResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	logger.Debugf("Exiting BatchUpdateDocuments()")

	return jsonResponse, nil

}

//handleRequestWithRevisionRetry method is a generic http request handler with
//a retry for document revision conflict errors,
//which may be detected during saves or deletes that timed out from client http perspective,
//but which eventually succeeded in couchdb
func (dbclient *CouchDatabase) handleRequestWithRevisionRetry(id, method string, connectURL url.URL, data []byte, rev string,
	multipartBoundary string, maxRetries int, keepConnectionOpen bool) (*http.Response, *DBReturn, error) {

	//Initialize a flag for the revsion conflict
	revisionConflictDetected := false
	var resp *http.Response
	var couchDBReturn *DBReturn
	var errResp error

	//attempt the http request for the max number of retries
	//In this case, the retry is to catch problems where a client timeout may miss a
	//successful CouchDB update and cause a document revision conflict on a retry in handleRequest
	for attempts := 0; attempts < maxRetries; attempts++ {

		//if the revision was not passed in, or if a revision conflict is detected on prior attempt,
		//query CouchDB for the document revision
		if rev == "" || revisionConflictDetected {
			rev = dbclient.getDocumentRevision(id)
		}

		//handle the request for saving/deleting the couchdb data
		resp, couchDBReturn, errResp = dbclient.CouchInstance.handleRequest(method, connectURL.String(),
			data, rev, multipartBoundary, maxRetries, keepConnectionOpen)

		//If there was a 409 conflict error during the save/delete, log it and retry it.
		//Otherwise, break out of the retry loop
		if couchDBReturn != nil && couchDBReturn.StatusCode == 409 {
			logger.Warningf("CouchDB document revision conflict detected, retrying. Attempt:%v", attempts+1)
			revisionConflictDetected = true
		} else {
			break
		}
	}

	// return the handleRequest results
	return resp, couchDBReturn, errResp
}

//handleRequest method is a generic http request handler.
// If it returns an error, it ensures that the response body is closed, else it is the
// callee's responsibility to close response correctly.
// Any http error or CouchDB error (4XX or 500) will result in a golang error getting returned
func (couchInstance *CouchInstance) handleRequest(method, connectURL string, data []byte, rev string,
	multipartBoundary string, maxRetries int, keepConnectionOpen bool) (*http.Response, *DBReturn, error) {

	logger.Debugf("Entering handleRequest()  method=%s  url=%v", method, connectURL)

	//create the return objects for couchDB
	var resp *http.Response
	var errResp error
	couchDBReturn := &DBReturn{}

	//set initial wait duration for retries
	waitDuration := retryWaitTime * time.Millisecond

	if maxRetries < 1 {
		return nil, nil, fmt.Errorf("Number of retries must be greater than zero.")
	}

	//attempt the http request for the max number of retries
	for attempts := 0; attempts < maxRetries; attempts++ {

		//Set up a buffer for the payload data
		payloadData := new(bytes.Buffer)

		payloadData.ReadFrom(bytes.NewReader(data))

		//Create request based on URL for couchdb operation
		req, err := http.NewRequest(method, connectURL, payloadData)
		if err != nil {
			return nil, nil, err
		}

		//set the request to close on completion if shared connections are not allowSharedConnection
		//Current CouchDB has a problem with zero length attachments, do not allow the connection to be reused.
		//Apache JIRA item for CouchDB   https://issues.apache.org/jira/browse/COUCHDB-3394
		if !keepConnectionOpen {
			req.Close = true
		}

		//add content header for PUT
		if method == http.MethodPut || method == http.MethodPost || method == http.MethodDelete {

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
		if couchInstance.conf.Username != "" && couchInstance.conf.Password != "" {
			req.SetBasicAuth(couchInstance.conf.Username, couchInstance.conf.Password)
		}

		if logger.IsEnabledFor(logging.DEBUG) {
			dump, _ := httputil.DumpRequestOut(req, false)
			// compact debug log by replacing carriage return / line feed with dashes to separate http headers
			logger.Debugf("HTTP Request: %s", bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
		}

		//Execute http request
		resp, errResp = couchInstance.client.Do(req)

		//check to see if the return from CouchDB is valid
		if invalidCouchDBReturn(resp, errResp) {
			continue
		}

		//if there is no golang http error and no CouchDB 500 error, then drop out of the retry
		if errResp == nil && resp != nil && resp.StatusCode < 500 {
			break
		}

		//if this is an unexpected golang http error, log the error and retry
		if errResp != nil {

			//Log the error with the retry count and continue
			logger.Warningf("Retrying couchdb request in %s. Attempt:%v  Error:%v",
				waitDuration.String(), attempts+1, errResp.Error())

			//otherwise this is an unexpected 500 error from CouchDB. Log the error and retry.
		} else {
			//Read the response body and close it for next attempt
			jsonError, err := ioutil.ReadAll(resp.Body)
			closeResponseBody(resp)
			if err != nil {
				return nil, nil, err
			}

			errorBytes := []byte(jsonError)

			//Unmarshal the response
			json.Unmarshal(errorBytes, &couchDBReturn)

			//Log the 500 error with the retry count and continue
			logger.Warningf("Retrying couchdb request in %s. Attempt:%v  Couch DB Error:%s,  Status Code:%v  Reason:%v",
				waitDuration.String(), attempts+1, couchDBReturn.Error, resp.Status, couchDBReturn.Reason)

		}
		//sleep for specified sleep time, then retry
		time.Sleep(waitDuration)

		//backoff, doubling the retry time for next attempt
		waitDuration *= 2

	} // end retry loop

	//if a golang http error is still present after retries are exhausted, return the error
	if errResp != nil {
		return nil, nil, errResp
	}

	//This situation should not occur according to the golang spec.
	//if this error returned (errResp) from an http call, then the resp should be not nil,
	//this is a structure and StatusCode is an int
	//This is meant to provide a more graceful error if this should occur
	if invalidCouchDBReturn(resp, errResp) {
		return nil, nil, fmt.Errorf("Unable to connect to CouchDB, check the hostname and port.")
	}

	//set the return code for the couchDB request
	couchDBReturn.StatusCode = resp.StatusCode

	//check to see if the status code from couchdb is 400 or higher
	//response codes 4XX and 500 will be treated as errors -
	//golang error will be created from the couchDBReturn contents and both will be returned
	if resp.StatusCode >= 400 {
		// close the response before returning error
		defer closeResponseBody(resp)

		//Read the response body
		jsonError, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}

		errorBytes := []byte(jsonError)

		//marshal the response
		json.Unmarshal(errorBytes, &couchDBReturn)

		logger.Debugf("Couch DB Error:%s,  Status Code:%v,  Reason:%s",
			couchDBReturn.Error, resp.StatusCode, couchDBReturn.Reason)

		return nil, couchDBReturn, fmt.Errorf("Couch DB Error:%s,  Status Code:%v,  Reason:%s",
			couchDBReturn.Error, resp.StatusCode, couchDBReturn.Reason)

	}

	logger.Debugf("Exiting handleRequest()")

	//If no errors, then return the http response and the couchdb return object
	return resp, couchDBReturn, nil
}

//invalidCouchDBResponse checks to make sure either a valid response or error is returned
func invalidCouchDBReturn(resp *http.Response, errResp error) bool {
	if resp == nil && errResp == nil {
		return true
	}
	return false
}

//IsJSON tests a string to determine if a valid JSON
func IsJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

// encodePathElement uses Golang for url path encoding, additionally:
// '/' is replaced by %2F, otherwise path encoding will treat as path separator and ignore it
// '+' is replaced by %2B, otherwise path encoding will ignore it, while CouchDB will unencode the plus as a space
// Note that all other URL special characters have been tested successfully without need for special handling
func encodePathElement(str string) string {

	logger.Debugf("Entering encodePathElement()  string=%s", str)

	u := &url.URL{}
	u.Path = str
	encodedStr := u.EscapedPath() // url encode using golang url path encoding rules
	encodedStr = strings.Replace(encodedStr, "/", "%2F", -1)
	encodedStr = strings.Replace(encodedStr, "+", "%2B", -1)

	logger.Debugf("Exiting encodePathElement()  encodedStr=%s", encodedStr)

	return encodedStr
}

func encodeForJSON(str string) (string, error) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(str); err != nil {
		return "", err
	}
	// Encode adds double quotes to string and terminates with \n - stripping them as bytes as they are all ascii(0-127)
	buffer := buf.Bytes()
	return string(buffer[1 : len(buffer)-2]), nil
}
