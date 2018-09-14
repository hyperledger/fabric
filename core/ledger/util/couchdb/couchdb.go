/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
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
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
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

// DocMetadata is used for capturing CouchDB document header info,
// used to capture id, version, rev and determine if attachments are returned in the query from CouchDB
type DocMetadata struct {
	ID              string          `json:"_id"`
	Rev             string          `json:"_rev"`
	Version         string          `json:"~version"`
	AttachmentsInfo json.RawMessage `json:"_attachments"`
}

//DocID is a minimal structure for capturing the ID from a query result
type DocID struct {
	ID string `json:"_id"`
}

//QueryResult is used for returning query results from CouchDB
type QueryResult struct {
	ID          string
	Value       []byte
	Attachments []*AttachmentInfo
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
	CouchInstance    *CouchInstance //connection configuration
	DBName           string
	IndexWarmCounter int
}

//DBReturn contains an error reported by CouchDB
type DBReturn struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

//CreateIndexResponse contains an the index creation response from CouchDB
type CreateIndexResponse struct {
	Result string `json:"result"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

//AttachmentInfo contains the definition for an attached file for couchdb
type AttachmentInfo struct {
	Name            string
	ContentType     string
	Length          uint64
	AttachmentBytes []byte
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
	Attachments []*AttachmentInfo
}

//BatchRetrieveDocMetadataResponse is used for processing REST batch responses from CouchDB
type BatchRetrieveDocMetadataResponse struct {
	Rows []struct {
		ID          string `json:"id"`
		DocMetadata struct {
			ID      string `json:"_id"`
			Rev     string `json:"_rev"`
			Version string `json:"~version"`
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

//IndexResult contains the definition for a couchdb index
type IndexResult struct {
	DesignDocument string `json:"designdoc"`
	Name           string `json:"name"`
	Definition     string `json:"definition"`
}

//DatabaseSecurity contains the definition for CouchDB database security
type DatabaseSecurity struct {
	Admins struct {
		Names []string `json:"names"`
		Roles []string `json:"roles"`
	} `json:"admins"`
	Members struct {
		Names []string `json:"names"`
		Roles []string `json:"roles"`
	} `json:"members"`
}

// closeResponseBody discards the body and then closes it to enable returning it to
// connection pool
func closeResponseBody(resp *http.Response) {
	if resp != nil {
		io.Copy(ioutil.Discard, resp.Body) // discard whatever is remaining of body
		resp.Body.Close()
	}
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
func (dbclient *CouchDatabase) CreateDatabaseIfNotExist() error {

	logger.Debugf("Entering CreateDatabaseIfNotExist()")

	dbInfo, couchDBReturn, err := dbclient.GetDatabaseInfo()
	if err != nil {
		if couchDBReturn == nil || couchDBReturn.StatusCode != 404 {
			return err
		}
	}

	//If the dbInfo returns populated and status code is 200, then the database exists
	if dbInfo != nil && couchDBReturn.StatusCode == 200 {

		//Apply database security if needed
		errSecurity := dbclient.applyDatabasePermissions()
		if errSecurity != nil {
			return errSecurity
		}

		logger.Debugf("Database %s already exists", dbclient.DBName)

		logger.Debugf("Exiting CreateDatabaseIfNotExist()")

		return nil
	}

	logger.Debugf("Database %s does not exist.", dbclient.DBName)

	connectURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return err
	}
	connectURL.Path = dbclient.DBName

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	//process the URL with a PUT, creates the database
	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPut, connectURL.String(), nil, "", "", maxRetries, true)

	if err != nil {

		// Check to see if the database exists
		// Even though handleRequest() returned an error, the
		// database may have been created and a false error
		// returned due to a timeout or race condition.
		// Do a final check to see if the database really got created.
		dbInfo, couchDBReturn, errDbInfo := dbclient.GetDatabaseInfo()
		//If there is no error, then the database exists,  return without an error
		if errDbInfo == nil && dbInfo != nil && couchDBReturn.StatusCode == 200 {

			errSecurity := dbclient.applyDatabasePermissions()
			if errSecurity != nil {
				return errSecurity
			}

			logger.Infof("Created state database %s", dbclient.DBName)
			logger.Debugf("Exiting CreateDatabaseIfNotExist()")
			return nil
		}

		return err

	}
	defer closeResponseBody(resp)

	errSecurity := dbclient.applyDatabasePermissions()
	if errSecurity != nil {
		return errSecurity
	}

	logger.Infof("Created state database %s", dbclient.DBName)

	logger.Debugf("Exiting CreateDatabaseIfNotExist()")

	return nil

}

//applyDatabaseSecurity
func (dbclient *CouchDatabase) applyDatabasePermissions() error {

	//If the username and password are not set, then skip applying permissions
	if dbclient.CouchInstance.conf.Username == "" && dbclient.CouchInstance.conf.Password == "" {
		return nil
	}

	securityPermissions := &DatabaseSecurity{}

	securityPermissions.Admins.Names = append(securityPermissions.Admins.Names, dbclient.CouchInstance.conf.Username)
	securityPermissions.Members.Names = append(securityPermissions.Members.Names, dbclient.CouchInstance.conf.Username)

	err := dbclient.ApplyDatabaseSecurity(securityPermissions)
	if err != nil {
		return err
	}

	return nil
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
	decodeErr := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if decodeErr != nil {
		return nil, nil, decodeErr
	}

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
	decodeErr := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if decodeErr != nil {
		return nil, nil, decodeErr
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
	err = CreateSystemDatabasesIfNotExist(couchInstance)
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
	decodeErr := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if decodeErr != nil {
		return nil, decodeErr
	}

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
	decodeErr := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if decodeErr != nil {
		return nil, decodeErr
	}

	if dbResponse.Ok == true {
		logger.Debugf("_ensure_full_commit database %s ", dbclient.DBName)
	}

	//Check to see if autoWarmIndexes is enabled
	//If autoWarmIndexes is enabled, indexes will be refreshed after the number of blocks
	//in GetWarmIndexesAfterNBlocks() have been committed to the state database
	//Check to see if the number of blocks committed exceeds the threshold for index warming
	//Use a go routine to launch WarmIndexAllIndexes(), this will execute as a background process
	if ledgerconfig.IsAutoWarmIndexesEnabled() {

		if dbclient.IndexWarmCounter >= ledgerconfig.GetWarmIndexesAfterNBlocks() {
			go dbclient.runWarmIndexAllIndexes()
			dbclient.IndexWarmCounter = 0
		}
		dbclient.IndexWarmCounter++

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
		decodeErr := decoder.Decode(&genericMap)
		if decodeErr != nil {
			return *writeBuffer, "", decodeErr
		}

		//add all key/values to the attachmentJSONMap
		for jsonKey, jsonValue := range genericMap {
			attachmentJSONMap[jsonKey] = jsonValue
		}

	}

	filesForUpload, err := json.Marshal(attachmentJSONMap)
	if err != nil {
		return *writeBuffer, "", err
	}

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

	if resp == nil {
		return "", fmt.Errorf("No response was received from CouchDB")
	}

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
	attachments := []*AttachmentInfo{}

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
				attachment := &AttachmentInfo{}
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

		var docMetadata = &DocMetadata{}
		err3 := json.Unmarshal(row.Doc, &docMetadata)
		if err3 != nil {
			return nil, err3
		}

		if docMetadata.AttachmentsInfo != nil {

			logger.Debugf("Adding JSON document and attachments for id: %s", docMetadata.ID)

			couchDoc, _, err := dbclient.ReadDoc(docMetadata.ID)
			if err != nil {
				return nil, err
			}

			var addDocument = &QueryResult{docMetadata.ID, couchDoc.JSONValue, couchDoc.Attachments}
			results = append(results, *addDocument)

		} else {

			logger.Debugf("Adding json docment for id: %s", docMetadata.ID)

			var addDocument = &QueryResult{docMetadata.ID, row.Doc, nil}
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

	if jsonResponse.Warning != "" {
		logger.Warningf("The query [%s] caused the following warning: [%s]", query, jsonResponse.Warning)
	}

	for _, row := range jsonResponse.Docs {

		var docMetadata = &DocMetadata{}
		err3 := json.Unmarshal(row, &docMetadata)
		if err3 != nil {
			return nil, err3
		}

		if docMetadata.AttachmentsInfo != nil {

			logger.Debugf("Adding JSON docment and attachments for id: %s", docMetadata.ID)

			couchDoc, _, err := dbclient.ReadDoc(docMetadata.ID)
			if err != nil {
				return nil, err
			}
			var addDocument = &QueryResult{ID: docMetadata.ID, Value: couchDoc.JSONValue, Attachments: couchDoc.Attachments}
			results = append(results, *addDocument)

		} else {
			logger.Debugf("Adding json docment for id: %s", docMetadata.ID)
			var addDocument = &QueryResult{ID: docMetadata.ID, Value: row, Attachments: nil}

			results = append(results, *addDocument)

		}
	}
	logger.Debugf("Exiting QueryDocuments()")

	return &results, nil

}

// ListIndex method lists the defined indexes for a database
func (dbclient *CouchDatabase) ListIndex() ([]*IndexResult, error) {

	//IndexDefinition contains the definition for a couchdb index
	type indexDefinition struct {
		DesignDocument string          `json:"ddoc"`
		Name           string          `json:"name"`
		Type           string          `json:"type"`
		Definition     json.RawMessage `json:"def"`
	}

	//ListIndexResponse contains the definition for listing couchdb indexes
	type listIndexResponse struct {
		TotalRows int               `json:"total_rows"`
		Indexes   []indexDefinition `json:"indexes"`
	}

	logger.Debug("Entering ListIndex()")

	indexURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}

	indexURL.Path = dbclient.DBName + "/_index/"

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodGet, indexURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	//handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonResponse = &listIndexResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	var results []*IndexResult

	for _, row := range jsonResponse.Indexes {

		//if the DesignDocument does not begin with "_design/", then this is a system
		//level index and is not meaningful and cannot be edited or deleted
		designDoc := row.DesignDocument
		s := strings.SplitAfterN(designDoc, "_design/", 2)
		if len(s) > 1 {
			designDoc = s[1]

			//Add the index definition to the results
			var addIndexResult = &IndexResult{DesignDocument: designDoc, Name: row.Name, Definition: fmt.Sprintf("%s", row.Definition)}
			results = append(results, addIndexResult)
		}

	}

	logger.Debugf("Exiting ListIndex()")

	return results, nil

}

// CreateIndex method provides a function creating an index
func (dbclient *CouchDatabase) CreateIndex(indexdefinition string) (*CreateIndexResponse, error) {

	logger.Debugf("Entering CreateIndex()  indexdefinition=%s", indexdefinition)

	//Test to see if this is a valid JSON
	if IsJSON(indexdefinition) != true {
		return nil, fmt.Errorf("JSON format is not valid")
	}

	indexURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}

	indexURL.Path = dbclient.DBName + "/_index"

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, indexURL.String(), []byte(indexdefinition), "", "", maxRetries, true)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if resp == nil {
		return nil, fmt.Errorf("An invalid response was received from CouchDB")
	}

	//Read the response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	couchDBReturn := &CreateIndexResponse{}

	jsonBytes := []byte(respBody)

	//unmarshal the response
	err = json.Unmarshal(jsonBytes, &couchDBReturn)
	if err != nil {
		return nil, err
	}

	if couchDBReturn.Result == "created" {

		logger.Infof("Created CouchDB index [%s] in state database [%s] using design document [%s]", couchDBReturn.Name, dbclient.DBName, couchDBReturn.ID)

		return couchDBReturn, nil

	}

	logger.Infof("Updated CouchDB index [%s] in state database [%s] using design document [%s]", couchDBReturn.Name, dbclient.DBName, couchDBReturn.ID)

	return couchDBReturn, nil
}

// DeleteIndex method provides a function deleting an index
func (dbclient *CouchDatabase) DeleteIndex(designdoc, indexname string) error {

	logger.Debugf("Entering DeleteIndex()  designdoc=%s  indexname=%s", designdoc, indexname)

	indexURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return err
	}

	indexURL.Path = dbclient.DBName + "/_index/" + designdoc + "/json/" + indexname

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodDelete, indexURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	return nil

}

//WarmIndex method provides a function for warming a single index
func (dbclient *CouchDatabase) WarmIndex(designdoc, indexname string) error {

	logger.Debugf("Entering WarmIndex()  designdoc=%s  indexname=%s", designdoc, indexname)

	indexURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return err
	}

	//URL to execute the view function associated with the index
	indexURL.Path = dbclient.DBName + "/_design/" + designdoc + "/_view/" + indexname

	queryParms := indexURL.Query()
	//Query parameter that allows the execution of the URL to return immediately
	//The update_after will cause the index update to run after the URL returns
	queryParms.Add("stale", "update_after")
	indexURL.RawQuery = queryParms.Encode()

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodGet, indexURL.String(), nil, "", "", maxRetries, true)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	return nil

}

//runWarmIndexAllIndexes is a wrapper for WarmIndexAllIndexes to catch and report any errors
func (dbclient *CouchDatabase) runWarmIndexAllIndexes() {

	err := dbclient.WarmIndexAllIndexes()
	if err != nil {
		logger.Errorf("Error detected during WarmIndexAllIndexes(): %s", err.Error())
	}

}

//WarmIndexAllIndexes method provides a function for warming all indexes for a database
func (dbclient *CouchDatabase) WarmIndexAllIndexes() error {

	logger.Debugf("Entering WarmIndexAllIndexes()")

	//Retrieve all indexes
	listResult, err := dbclient.ListIndex()
	if err != nil {
		return err
	}

	//For each index definition, execute an index refresh
	for _, elem := range listResult {

		err := dbclient.WarmIndex(elem.DesignDocument, elem.Name)
		if err != nil {
			return err
		}

	}

	logger.Debugf("Exiting WarmIndexAllIndexes()")

	return nil

}

//GetDatabaseSecurity method provides function to retrieve the security config for a database
func (dbclient *CouchDatabase) GetDatabaseSecurity() (*DatabaseSecurity, error) {

	logger.Debugf("Entering GetDatabaseSecurity()")

	securityURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}

	securityURL.Path = dbclient.DBName + "/_security"

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodGet, securityURL.String(),
		nil, "", "", maxRetries, true)

	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	//handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonResponse = &DatabaseSecurity{}

	err2 := json.Unmarshal(jsonResponseRaw, jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	logger.Debugf("Exiting GetDatabaseSecurity()")

	return jsonResponse, nil

}

//ApplyDatabaseSecurity method provides function to update the security config for a database
func (dbclient *CouchDatabase) ApplyDatabaseSecurity(databaseSecurity *DatabaseSecurity) error {

	logger.Debugf("Entering ApplyDatabaseSecurity()")

	securityURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return err
	}

	securityURL.Path = dbclient.DBName + "/_security"

	//Ensure all of the arrays are initialized to empty arrays instead of nil
	if databaseSecurity.Admins.Names == nil {
		databaseSecurity.Admins.Names = make([]string, 0)
	}
	if databaseSecurity.Admins.Roles == nil {
		databaseSecurity.Admins.Roles = make([]string, 0)
	}
	if databaseSecurity.Members.Names == nil {
		databaseSecurity.Members.Names = make([]string, 0)
	}
	if databaseSecurity.Members.Roles == nil {
		databaseSecurity.Members.Roles = make([]string, 0)
	}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	databaseSecurityJSON, err := json.Marshal(databaseSecurity)
	if err != nil {
		return err
	}

	logger.Debugf("Applying security to database [%s]: %s", dbclient.DBName, string(databaseSecurityJSON))

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPut, securityURL.String(), databaseSecurityJSON, "", "", maxRetries, true)

	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	logger.Debugf("Exiting ApplyDatabaseSecurity()")

	return nil

}

//BatchRetrieveDocumentMetadata - batch method to retrieve document metadata for  a set of keys,
// including ID, couchdb revision number, and ledger version
func (dbclient *CouchDatabase) BatchRetrieveDocumentMetadata(keys []string) ([]*DocMetadata, error) {

	logger.Debugf("Entering BatchRetrieveDocumentMetadata()  keys=%s", keys)

	batchRetrieveURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	batchRetrieveURL.Path = dbclient.DBName + "/_all_docs"

	queryParms := batchRetrieveURL.Query()

	// While BatchRetrieveDocumentMetadata() does not return the entire document,
	// for reads/writes, we do need to get document so that we can get the ledger version of the key.
	// TODO For blind writes we do not need to get the version, therefore when we bulk get
	// the revision numbers for the write keys that were not represented in read set
	// (the second time BatchRetrieveDocumentMetadata is called during block processing),
	// we could set include_docs to false to optimize the response.
	queryParms.Add("include_docs", "true")
	batchRetrieveURL.RawQuery = queryParms.Encode()

	keymap := make(map[string]interface{})

	keymap["keys"] = keys

	jsonKeys, err := json.Marshal(keymap)
	if err != nil {
		return nil, err
	}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, batchRetrieveURL.String(), jsonKeys, "", "", maxRetries, true)
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

	var jsonResponse = &BatchRetrieveDocMetadataResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, err2
	}

	docMetadataArray := []*DocMetadata{}

	for _, row := range jsonResponse.Rows {
		docMetadata := &DocMetadata{ID: row.ID, Rev: row.DocMetadata.Rev, Version: row.DocMetadata.Version}
		docMetadataArray = append(docMetadataArray, docMetadata)
	}

	logger.Debugf("Exiting BatchRetrieveDocumentMetadata()")

	return docMetadataArray, nil

}

//BatchUpdateDocuments - batch method to batch update documents
func (dbclient *CouchDatabase) BatchUpdateDocuments(documents []*CouchDoc) ([]*BatchUpdateResponse, error) {

	if logger.IsEnabledFor(logging.DEBUG) {
		documentIdsString, err := printDocumentIds(documents)
		if err == nil {
			logger.Debugf("Entering BatchUpdateDocuments()  document ids=[%s]", documentIdsString)
		} else {
			logger.Debugf("Entering BatchUpdateDocuments()  Could not print document ids due to error:" + err.Error())
		}
	}

	batchUpdateURL, err := url.Parse(dbclient.CouchInstance.conf.URL)
	if err != nil {
		logger.Errorf("URL parse error: %s", err.Error())
		return nil, err
	}
	batchUpdateURL.Path = dbclient.DBName + "/_bulk_docs"

	documentMap := make(map[string]interface{})

	var jsonDocumentMap []interface{}

	for _, jsonDocument := range documents {

		//create a document map
		var document = make(map[string]interface{})

		//unmarshal the JSON component of the CouchDoc into the document
		err = json.Unmarshal(jsonDocument.JSONValue, &document)
		if err != nil {
			return nil, err
		}

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

	bulkDocsJSON, err := json.Marshal(documentMap)
	if err != nil {
		return nil, err
	}

	//get the number of retries
	maxRetries := dbclient.CouchInstance.conf.MaxRetries

	resp, _, err := dbclient.CouchInstance.handleRequest(http.MethodPost, batchUpdateURL.String(), bulkDocsJSON, "", "", maxRetries, true)
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

	logger.Debugf("Exiting BatchUpdateDocuments() _bulk_docs response=[%s]", string(jsonResponseRaw))

	return jsonResponse, nil

}

//handleRequestWithRevisionRetry method is a generic http request handler with
//a retry for document revision conflict errors,
//which may be detected during saves or deletes that timed out from client http perspective,
//but which eventually succeeded in couchdb
func (dbclient *CouchDatabase) handleRequestWithRevisionRetry(id, method string, connectURL url.URL, data []byte, rev string,
	multipartBoundary string, maxRetries int, keepConnectionOpen bool) (*http.Response, *DBReturn, error) {

	//Initialize a flag for the revision conflict
	revisionConflictDetected := false
	var resp *http.Response
	var couchDBReturn *DBReturn
	var errResp error

	//attempt the http request for the max number of retries
	//In this case, the retry is to catch problems where a client timeout may miss a
	//successful CouchDB update and cause a document revision conflict on a retry in handleRequest
	for attempts := 0; attempts <= maxRetries; attempts++ {

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

	if maxRetries < 0 {
		return nil, nil, fmt.Errorf("Number of retries must be zero or greater.")
	}

	//attempt the http request for the max number of retries
	// if maxRetries is 0, the database creation will be attempted once and will
	//    return an error if unsuccessful
	// if maxRetries is 3 (default), a maximum of 4 attempts (one attempt with 3 retries)
	//    will be made with warning entries for unsuccessful attempts
	for attempts := 0; attempts <= maxRetries; attempts++ {

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
			//req.Header.Set("Authorization", "Basic YWRtaW46YWRtaW5w")
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
			// if this is an error, then populate the couchDBReturn
			if resp.StatusCode >= 400 {
				//Read the response body and close it for next attempt
				jsonError, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, err
				}
				defer closeResponseBody(resp)

				errorBytes := []byte(jsonError)
				//Unmarshal the response
				err = json.Unmarshal(errorBytes, &couchDBReturn)
				if err != nil {
					return nil, nil, err
				}
			}

			break
		}

		// If the maxRetries is greater than 0, then log the retry info
		if maxRetries > 0 {

			//if this is an unexpected golang http error, log the error and retry
			if errResp != nil {

				//Log the error with the retry count and continue
				logger.Warningf("Retrying couchdb request in %s. Attempt:%v  Error:%v",
					waitDuration.String(), attempts+1, errResp.Error())

				//otherwise this is an unexpected 500 error from CouchDB. Log the error and retry.
			} else {
				//Read the response body and close it for next attempt
				jsonError, err := ioutil.ReadAll(resp.Body)
				defer closeResponseBody(resp)
				if err != nil {
					return nil, nil, err
				}

				errorBytes := []byte(jsonError)
				//Unmarshal the response
				err = json.Unmarshal(errorBytes, &couchDBReturn)
				if err != nil {
					return nil, nil, err
				}

				//Log the 500 error with the retry count and continue
				logger.Warningf("Retrying couchdb request in %s. Attempt:%v  Couch DB Error:%s,  Status Code:%v  Reason:%v",
					waitDuration.String(), attempts+1, couchDBReturn.Error, resp.Status, couchDBReturn.Reason)

			}
			//sleep for specified sleep time, then retry
			time.Sleep(waitDuration)

			//backoff, doubling the retry time for next attempt
			waitDuration *= 2

		}

	} // end retry loop

	//if a golang http error is still present after retries are exhausted, return the error
	if errResp != nil {
		return nil, couchDBReturn, errResp
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

	// check to see if the status code from couchdb is 400 or higher
	// response codes 4XX and 500 will be treated as errors -
	// golang error will be created from the couchDBReturn contents and both will be returned
	if resp.StatusCode >= 400 {

		// if the status code is 400 or greater, log and return an error
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

// printDocumentIds is a convenience method to print readable log entries for arrays of pointers
// to couch document IDs
func printDocumentIds(documentPointers []*CouchDoc) (string, error) {

	documentIds := []string{}

	for _, documentPointer := range documentPointers {
		docMetadata := &DocMetadata{}
		err := json.Unmarshal(documentPointer.JSONValue, &docMetadata)
		if err != nil {
			return "", err
		}
		documentIds = append(documentIds, docMetadata.ID)
	}
	return strings.Join(documentIds, ","), nil
}
