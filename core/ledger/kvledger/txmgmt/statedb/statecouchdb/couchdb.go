/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"compress/gzip"
	"context"
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
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var couchdbLogger = flogging.MustGetLogger("couchdb")

// time between retry attempts in milliseconds
const retryWaitTime = 125

// dbInfo is body for database information.
type dbInfo struct {
	DbName string `json:"db_name"`
	Sizes  struct {
		File     int `json:"file"`
		External int `json:"external"`
		Active   int `json:"active"`
	} `json:"sizes"`
	Other struct {
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

// connectionInfo is a structure for capturing the database info and version
type connectionInfo struct {
	Couchdb string `json:"couchdb"`
	Version string `json:"version"`
	Vendor  struct {
		Name string `json:"name"`
	} `json:"vendor"`
}

// rangeQueryResponse is used for processing REST range query responses from CouchDB
type rangeQueryResponse struct {
	TotalRows int32 `json:"total_rows"`
	Offset    int32 `json:"offset"`
	Rows      []struct {
		ID    string `json:"id"`
		Key   string `json:"key"`
		Value struct {
			Rev string `json:"rev"`
		} `json:"value"`
		Doc json.RawMessage `json:"doc"`
	} `json:"rows"`
}

// queryResponse is used for processing REST query responses from CouchDB
type queryResponse struct {
	Warning  string            `json:"warning"`
	Docs     []json.RawMessage `json:"docs"`
	Bookmark string            `json:"bookmark"`
}

// docMetadata is used for capturing CouchDB document header info,
// used to capture id, version, rev and attachments returned in the query from CouchDB
type docMetadata struct {
	ID              string                     `json:"_id"`
	Rev             string                     `json:"_rev"`
	Version         string                     `json:"~version"`
	AttachmentsInfo map[string]*attachmentInfo `json:"_attachments"`
}

// queryResult is used for returning query results from CouchDB
type queryResult struct {
	id          string
	value       []byte
	attachments []*attachmentInfo
}

// couchInstance represents a CouchDB instance
type couchInstance struct {
	conf   *ledger.CouchDBConfig
	client *http.Client // a client to connect to this instance
	stats  *stats
}

// couchDatabase represents a database within a CouchDB instance
type couchDatabase struct {
	couchInstance *couchInstance // connection configuration
	dbName        string
}

// dbReturn contains an error reported by CouchDB
type dbReturn struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error"`
	Reason     string `json:"reason"`
}

// createIndexResponse contains an the index creation response from CouchDB
type createIndexResponse struct {
	Result string `json:"result"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

// attachmentInfo contains the definition for an attached file for couchdb
type attachmentInfo struct {
	Name            string
	ContentType     string `json:"content_type"`
	Length          uint64
	AttachmentBytes []byte `json:"data"`
}

func (a *attachmentInfo) len() int {
	if a == nil {
		return 0
	}
	return len(a.Name) + len(a.ContentType) + len(a.AttachmentBytes)
}

// fileDetails defines the structure needed to send an attachment to couchdb
type fileDetails struct {
	Follows     bool   `json:"follows"`
	ContentType string `json:"content_type"`
	Length      int    `json:"length"`
}

// batchRetrieveDocMetadataResponse is used for processing REST batch responses from CouchDB
type batchRetrieveDocMetadataResponse struct {
	Rows []struct {
		ID          string `json:"id"`
		DocMetadata struct {
			ID      string `json:"_id"`
			Rev     string `json:"_rev"`
			Version string `json:"~version"`
		} `json:"doc"`
	} `json:"rows"`
}

// batchUpdateResponse defines a structure for batch update response
type batchUpdateResponse struct {
	ID     string `json:"id"`
	Error  string `json:"error"`
	Reason string `json:"reason"`
	Ok     bool   `json:"ok"`
	Rev    string `json:"rev"`
}

// base64Attachment contains the definition for an attached file for couchdb
type base64Attachment struct {
	ContentType    string `json:"content_type"`
	AttachmentData string `json:"data"`
}

// indexResult contains the definition for a couchdb index
type indexResult struct {
	DesignDocument string `json:"designdoc"`
	Name           string `json:"name"`
	Definition     string `json:"definition"`
}

// databaseSecurity contains the definition for CouchDB database security
type databaseSecurity struct {
	Admins struct {
		Names []string `json:"names"`
		Roles []string `json:"roles"`
	} `json:"admins"`
	Members struct {
		Names []string `json:"names"`
		Roles []string `json:"roles"`
	} `json:"members"`
}

// couchDoc defines the structure for a JSON document value
type couchDoc struct {
	jsonValue   []byte
	attachments []*attachmentInfo
}

func (d *couchDoc) key() (string, error) {
	m := make(jsonValue)
	if err := json.Unmarshal(d.jsonValue, &m); err != nil {
		return "", err
	}
	return m[idField].(string), nil
}

func (d *couchDoc) len() int {
	if d == nil {
		return 0
	}
	size := len(d.jsonValue)
	for _, a := range d.attachments {
		size += a.len()
	}
	return size
}

// closeResponseBody discards the body and then closes it to enable returning it to
// connection pool
func closeResponseBody(resp *http.Response) {
	if resp != nil {
		io.Copy(ioutil.Discard, resp.Body) // discard whatever is remaining of body
		resp.Body.Close()
	}
}

// createDatabaseIfNotExist method provides function to create database
func (dbclient *couchDatabase) createDatabaseIfNotExist() error {
	couchdbLogger.Debugf("[%s] Entering CreateDatabaseIfNotExist()", dbclient.dbName)

	dbInfo, couchDBReturn, err := dbclient.getDatabaseInfo()
	if err != nil {
		if couchDBReturn == nil || couchDBReturn.StatusCode != 404 {
			return err
		}
	}

	if dbInfo == nil || couchDBReturn.StatusCode == 404 {
		couchdbLogger.Debugf("[%s] Database does not exist.", dbclient.dbName)

		connectURL, err := url.Parse(dbclient.couchInstance.url())
		if err != nil {
			couchdbLogger.Errorf("URL parse error: %s", err)
			return errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
		}

		// get the number of retries
		maxRetries := dbclient.couchInstance.conf.MaxRetries

		// process the URL with a PUT, creates the database
		resp, _, err := dbclient.handleRequest(http.MethodPut, "CreateDatabaseIfNotExist", connectURL, nil, "", "", maxRetries, true, nil)
		if err != nil {
			// Check to see if the database exists
			// Even though handleRequest() returned an error, the
			// database may have been created and a false error
			// returned due to a timeout or race condition.
			// Do a final check to see if the database really got created.
			dbInfo, couchDBReturn, dbInfoErr := dbclient.getDatabaseInfo()
			if dbInfoErr != nil || dbInfo == nil || couchDBReturn.StatusCode == 404 {
				return err
			}
		}
		defer closeResponseBody(resp)
		couchdbLogger.Infof("Created state database %s", dbclient.dbName)
	} else {
		couchdbLogger.Debugf("[%s] Database already exists", dbclient.dbName)
	}

	if dbclient.dbName != "_users" {
		errSecurity := dbclient.applyDatabasePermissions()
		if errSecurity != nil {
			return errSecurity
		}
	}

	couchdbLogger.Debugf("[%s] Exiting CreateDatabaseIfNotExist()", dbclient.dbName)
	return nil
}

func (dbclient *couchDatabase) applyDatabasePermissions() error {
	// If the username and password are not set, then skip applying permissions
	if dbclient.couchInstance.conf.Username == "" && dbclient.couchInstance.conf.Password == "" {
		return nil
	}

	securityPermissions := &databaseSecurity{}

	securityPermissions.Admins.Names = append(securityPermissions.Admins.Names, dbclient.couchInstance.conf.Username)
	securityPermissions.Members.Names = append(securityPermissions.Members.Names, dbclient.couchInstance.conf.Username)

	err := dbclient.applyDatabaseSecurity(securityPermissions)
	if err != nil {
		return err
	}

	return nil
}

// getDatabaseInfo method provides function to retrieve database information
func (dbclient *couchDatabase) getDatabaseInfo() (*dbInfo, *dbReturn, error) {
	connectURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, nil, errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, couchDBReturn, err := dbclient.handleRequest(http.MethodGet, "GetDatabaseInfo", connectURL, nil, "", "", maxRetries, true, nil)
	if err != nil {
		return nil, couchDBReturn, err
	}
	defer closeResponseBody(resp)

	dbResponse := &dbInfo{}
	decodeErr := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if decodeErr != nil {
		return nil, nil, errors.Wrap(decodeErr, "error decoding response body")
	}

	// trace the database info response
	couchdbLogger.Debugw("GetDatabaseInfo()", "dbResponseJSON", dbResponse)

	return dbResponse, couchDBReturn, nil
}

// verifyCouchConfig method provides function to verify the connection information
func (couchInstance *couchInstance) verifyCouchConfig() (*connectionInfo, *dbReturn, error) {
	couchdbLogger.Debugf("Entering VerifyCouchConfig()")
	defer couchdbLogger.Debugf("Exiting VerifyCouchConfig()")

	connectURL, err := url.Parse(couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, nil, errors.Wrapf(err, "error parsing couch instance URL: %s", couchInstance.url())
	}
	connectURL.Path = "/"

	// get the number of retries for startup
	maxRetriesOnStartup := couchInstance.conf.MaxRetriesOnStartup

	resp, couchDBReturn, err := couchInstance.handleRequest(context.Background(), http.MethodGet, "", "VerifyCouchConfig", connectURL, nil,
		"", "", maxRetriesOnStartup, true, nil)
	if err != nil {
		return nil, couchDBReturn, errors.WithMessage(err, "unable to connect to CouchDB, check the hostname and port")
	}
	defer closeResponseBody(resp)

	dbResponse := &connectionInfo{}
	decodeErr := json.NewDecoder(resp.Body).Decode(&dbResponse)
	if decodeErr != nil {
		return nil, nil, errors.Wrap(decodeErr, "error decoding response body")
	}

	// trace the database info response
	couchdbLogger.Debugw("VerifyConnection()", "dbResponseJSON", dbResponse)

	// check to see if the system databases exist
	// Verifying the existence of the system database accomplishes two steps
	// 1.  Ensures the system databases are created
	// 2.  Verifies the username password provided in the CouchDB config are valid for system admin
	err = createSystemDatabasesIfNotExist(couchInstance)
	if err != nil {
		couchdbLogger.Errorf("Unable to connect to CouchDB, error: %s. Check the admin username and password.", err)
		return nil, nil, errors.WithMessage(err, "unable to connect to CouchDB. Check the admin username and password")
	}

	return dbResponse, couchDBReturn, nil
}

// isEmpty returns false if couchInstance contains any databases
// (except couchdb system databases and any database name supplied in the parameter 'databasesToIgnore')
func (couchInstance *couchInstance) isEmpty(databasesToIgnore []string) (bool, error) {
	toIgnore := map[string]bool{}
	for _, s := range databasesToIgnore {
		toIgnore[s] = true
	}
	applicationDBNames, err := couchInstance.retrieveApplicationDBNames()
	if err != nil {
		return false, err
	}
	for _, dbName := range applicationDBNames {
		if !toIgnore[dbName] {
			return false, nil
		}
	}
	return true, nil
}

// retrieveApplicationDBNames returns all the application database names in the couch instance
func (couchInstance *couchInstance) retrieveApplicationDBNames() ([]string, error) {
	connectURL, err := url.Parse(couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, errors.Wrapf(err, "error parsing couch instance URL: %s", couchInstance.url())
	}
	connectURL.Path = "/_all_dbs"
	maxRetries := couchInstance.conf.MaxRetries
	resp, _, err := couchInstance.handleRequest(
		context.Background(),
		http.MethodGet,
		"",
		"IsEmpty",
		connectURL,
		nil,
		"",
		"",
		maxRetries,
		true,
		nil,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "unable to connect to CouchDB, check the hostname and port")
	}

	var dbNames []string
	defer closeResponseBody(resp)
	if err := json.NewDecoder(resp.Body).Decode(&dbNames); err != nil {
		return nil, errors.Wrap(err, "error decoding response body")
	}
	couchdbLogger.Debugf("dbNames = %s", dbNames)
	applicationsDBNames := []string{}
	for _, d := range dbNames {
		if !isCouchSystemDBName(d) {
			applicationsDBNames = append(applicationsDBNames, d)
		}
	}
	return applicationsDBNames, nil
}

func isCouchSystemDBName(name string) bool {
	return strings.HasPrefix(name, "_")
}

// healthCheck checks if the peer is able to communicate with CouchDB
func (couchInstance *couchInstance) healthCheck(ctx context.Context) error {
	connectURL, err := url.Parse(couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return errors.Wrapf(err, "error parsing CouchDB URL: %s", couchInstance.url())
	}
	_, _, err = couchInstance.handleRequest(ctx, http.MethodHead, "", "HealthCheck", connectURL, nil, "", "", 0, true, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to couch db [%s]", err)
	}
	return nil
}

// internalQueryLimit returns the maximum number of records to return internally
// when querying CouchDB.
func (couchInstance *couchInstance) internalQueryLimit() int32 {
	return int32(couchInstance.conf.InternalQueryLimit)
}

// maxBatchUpdateSize returns the maximum number of records to include in a
// bulk update operation.
func (couchInstance *couchInstance) maxBatchUpdateSize() int {
	return couchInstance.conf.MaxBatchUpdateSize
}

// url returns the URL for the CouchDB instance.
func (couchInstance *couchInstance) url() string {
	URL := &url.URL{
		Host:   couchInstance.conf.Address,
		Scheme: "http",
	}
	return URL.String()
}

// dropDatabase provides method to drop an existing database
func (dbclient *couchDatabase) dropDatabase() error {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering DropDatabase()", dbName)

	connectURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, couchdbReturn, err := dbclient.handleRequest(http.MethodDelete, "DropDatabase", connectURL, nil, "", "", maxRetries, true, nil)
	defer closeResponseBody(resp)
	if couchdbReturn != nil && couchdbReturn.StatusCode == 404 {
		couchdbLogger.Debugf("[%s] Exiting DropDatabase(), database does not exist", dbclient.dbName)
		return nil
	}
	if err != nil {
		return err
	}

	couchdbLogger.Debugf("[%s] Exiting DropDatabase(), database dropped", dbclient.dbName)
	return nil
}

// saveDoc method provides a function to save a document, id and byte array
func (dbclient *couchDatabase) saveDoc(id string, rev string, couchDoc *couchDoc) (string, error) {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering SaveDoc() id=[%s]", dbName, id)

	if !utf8.ValidString(id) {
		return "", errors.Errorf("doc id [%x] not a valid utf8 string", id)
	}

	saveURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return "", errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// Set up a buffer for the data to be pushed to couchdb
	var data []byte

	// Set up a default boundary for use by multipart if sending attachments
	defaultBoundary := ""

	// Create a flag for shared connections.  This is set to false for zero length attachments
	keepConnectionOpen := true

	// check to see if attachments is nil, if so, then this is a JSON only
	if couchDoc.attachments == nil {

		// Test to see if this is a valid JSON
		if !isJSON(string(couchDoc.jsonValue)) {
			return "", errors.New("JSON format is not valid")
		}

		// if there are no attachments, then use the bytes passed in as the JSON
		data = couchDoc.jsonValue

	} else { // there are attachments

		// attachments are included, create the multipart definition
		multipartData, multipartBoundary, err3 := createAttachmentPart(couchDoc)
		if err3 != nil {
			return "", err3
		}

		// If there is a zero length attachment, do not keep the connection open
		for _, attach := range couchDoc.attachments {
			if attach.Length < 1 {
				keepConnectionOpen = false
			}
		}

		// Set the data buffer to the data from the create multi-part data
		data = multipartData.Bytes()

		// Set the default boundary to the value generated in the multipart creation
		defaultBoundary = multipartBoundary

	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	// handle the request for saving document with a retry if there is a revision conflict
	resp, _, err := dbclient.handleRequestWithRevisionRetry(id, http.MethodPut, dbName, "SaveDoc", saveURL, data, rev, defaultBoundary, maxRetries, keepConnectionOpen, nil)
	if err != nil {
		return "", err
	}
	defer closeResponseBody(resp)

	// get the revision and return
	revision, err := getRevisionHeader(resp)
	if err != nil {
		return "", err
	}

	couchdbLogger.Debugf("[%s] Exiting SaveDoc()", dbclient.dbName)

	return revision, nil
}

// getDocumentRevision will return the revision if the document exists, otherwise it will return ""
func (dbclient *couchDatabase) getDocumentRevision(id string) string {
	rev := ""

	// See if the document already exists, we need the rev for saves and deletes
	_, revdoc, err := dbclient.readDoc(id)
	if err == nil {
		// set the revision to the rev returned from the document read
		rev = revdoc
	}
	return rev
}

func createAttachmentPart(couchDoc *couchDoc) (bytes.Buffer, string, error) {
	// Create a buffer for writing the result
	writeBuffer := new(bytes.Buffer)

	// read the attachment and save as an attachment
	writer := multipart.NewWriter(writeBuffer)

	// retrieve the boundary for the multipart
	defaultBoundary := writer.Boundary()

	fileAttachments := map[string]fileDetails{}

	for _, attachment := range couchDoc.attachments {
		fileAttachments[attachment.Name] = fileDetails{true, attachment.ContentType, len(attachment.AttachmentBytes)}
	}

	attachmentJSONMap := map[string]interface{}{
		"_attachments": fileAttachments,
	}

	// Add any data uploaded with the files
	if couchDoc.jsonValue != nil {

		// create a generic map
		genericMap := make(map[string]interface{})

		// unmarshal the data into the generic map
		decoder := json.NewDecoder(bytes.NewBuffer(couchDoc.jsonValue))
		decoder.UseNumber()
		decodeErr := decoder.Decode(&genericMap)
		if decodeErr != nil {
			return *writeBuffer, "", errors.Wrap(decodeErr, "error decoding json data")
		}

		// add all key/values to the attachmentJSONMap
		for jsonKey, jsonValue := range genericMap {
			attachmentJSONMap[jsonKey] = jsonValue
		}

	}

	filesForUpload, err := json.Marshal(attachmentJSONMap)
	if err != nil {
		return *writeBuffer, "", errors.Wrap(err, "error marshalling json data")
	}

	couchdbLogger.Debugf(string(filesForUpload))

	// create the header for the JSON
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return *writeBuffer, defaultBoundary, errors.Wrap(err, "error creating multipart")
	}

	part.Write(filesForUpload)

	for _, attachment := range couchDoc.attachments {

		header := make(textproto.MIMEHeader)
		part, err2 := writer.CreatePart(header)
		if err2 != nil {
			return *writeBuffer, defaultBoundary, errors.Wrap(err2, "error creating multipart")
		}
		part.Write(attachment.AttachmentBytes)

	}

	err = writer.Close()
	if err != nil {
		return *writeBuffer, defaultBoundary, errors.Wrap(err, "error closing multipart writer")
	}

	return *writeBuffer, defaultBoundary, nil
}

func getRevisionHeader(resp *http.Response) (string, error) {
	if resp == nil {
		return "", errors.New("no response received from CouchDB")
	}

	revision := resp.Header.Get("Etag")

	if revision == "" {
		return "", errors.New("no revision tag detected")
	}

	reg := regexp.MustCompile(`"([^"]*)"`)
	revisionNoQuotes := reg.ReplaceAllString(revision, "${1}")
	return revisionNoQuotes, nil
}

// readDoc method provides function to retrieve a document and its revision
// from the database by id
func (dbclient *couchDatabase) readDoc(id string) (*couchDoc, string, error) {
	var couchDoc couchDoc
	attachments := []*attachmentInfo{}
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering ReadDoc()  id=[%s]", dbName, id)
	defer couchdbLogger.Debugf("[%s] Exiting ReadDoc()", dbclient.dbName)

	if !utf8.ValidString(id) {
		return nil, "", errors.Errorf("doc id [%x] not a valid utf8 string", id)
	}

	readURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, "", errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	query := readURL.Query()
	query.Add("attachments", "true")

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, couchDBReturn, err := dbclient.handleRequest(http.MethodGet, "ReadDoc", readURL, nil, "", "", maxRetries, true, &query, id)
	if err != nil {
		if couchDBReturn != nil && couchDBReturn.StatusCode == 404 {
			couchdbLogger.Debugf("[%s] Document not found (404), returning nil value instead of 404 error", dbclient.dbName)
			// non-existent document should return nil value instead of a 404 error
			// for details see https://github.com/hyperledger-archives/fabric/issues/936
			return nil, "", nil
		}
		couchdbLogger.Debugf("[%s] couchDBReturn=%v\n", dbclient.dbName, couchDBReturn)
		return nil, "", err
	}
	defer closeResponseBody(resp)

	// Get the media type from the Content-Type header
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		log.Fatal(err)
	}

	// Get the revision from header
	revision, err := getRevisionHeader(resp)
	if err != nil {
		return nil, "", err
	}

	// Handle as JSON if multipart is NOT detected
	if !strings.HasPrefix(mediaType, "multipart/") {
		couchDoc.jsonValue, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, "", errors.Wrap(err, "error reading response body")
		}

		return &couchDoc, revision, nil
	}

	// Handle as attachment.
	// Set up the multipart reader based on the boundary.
	multipartReader := multipart.NewReader(resp.Body, params["boundary"])
	for {
		p, err := multipartReader.NextPart()
		if err == io.EOF {
			break // processed all parts
		}
		if err != nil {
			return nil, "", errors.Wrap(err, "error reading next multipart")
		}

		defer p.Close()

		couchdbLogger.Debugf("[%s] part header=%s", dbclient.dbName, p.Header)

		if p.Header.Get("Content-Type") == "application/json" {
			partdata, err := ioutil.ReadAll(p)
			if err != nil {
				return nil, "", errors.Wrap(err, "error reading multipart data")
			}
			couchDoc.jsonValue = partdata
			continue
		}

		// Create an attachment structure and load it.
		attachment := &attachmentInfo{}
		attachment.ContentType = p.Header.Get("Content-Type")
		contentDispositionParts := strings.Split(p.Header.Get("Content-Disposition"), ";")

		if strings.TrimSpace(contentDispositionParts[0]) != "attachment" {
			continue
		}

		switch p.Header.Get("Content-Encoding") {
		case "gzip": // See if the part is gzip encoded

			var respBody []byte

			gr, err := gzip.NewReader(p)
			if err != nil {
				return nil, "", errors.Wrap(err, "error creating gzip reader")
			}
			respBody, err = ioutil.ReadAll(gr)
			if err != nil {
				return nil, "", errors.Wrap(err, "error reading gzip data")
			}

			couchdbLogger.Debugf("[%s] Retrieved attachment data", dbclient.dbName)
			attachment.AttachmentBytes = respBody
			attachment.Length = uint64(len(attachment.AttachmentBytes))
			attachment.Name = p.FileName()
			attachments = append(attachments, attachment)

		default:

			// retrieve the data,  this is not gzip
			partdata, err := ioutil.ReadAll(p)
			if err != nil {
				return nil, "", errors.Wrap(err, "error reading multipart data")
			}
			couchdbLogger.Debugf("[%s] Retrieved attachment data", dbclient.dbName)
			attachment.AttachmentBytes = partdata
			attachment.Length = uint64(len(attachment.AttachmentBytes))
			attachment.Name = p.FileName()
			attachments = append(attachments, attachment)
		}
	}

	couchDoc.attachments = attachments
	return &couchDoc, revision, nil
}

// readDocRange method provides function to a range of documents based on the start and end keys
// startKey and endKey can also be empty strings.  If startKey and endKey are empty, all documents are returned
// This function provides a limit option to specify the max number of entries and is supplied by config.
// Skip is reserved for possible future use.
func (dbclient *couchDatabase) readDocRange(startKey, endKey string, limit int32) ([]*queryResult, string, error) {
	dbName := dbclient.dbName
	couchdbLogger.Debugf("[%s] Entering ReadDocRange()  startKey=%s, endKey=%s", dbName, startKey, endKey)

	var results []*queryResult

	rangeURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, "", errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	queryParms := rangeURL.Query()
	// Increment the limit by 1 to see if there are more qualifying records
	queryParms.Set("limit", strconv.FormatInt(int64(limit+1), 10))
	queryParms.Add("include_docs", "true")
	queryParms.Add("inclusive_end", "false") // endkey should be exclusive to be consistent with goleveldb
	queryParms.Add("attachments", "true")    // get the attachments as well

	// Append the startKey if provided
	if startKey != "" {
		if startKey, err = encodeForJSON(startKey); err != nil {
			return nil, "", err
		}
		queryParms.Add("startkey", "\""+startKey+"\"")
	}

	// Append the endKey if provided
	if endKey != "" {
		var err error
		if endKey, err = encodeForJSON(endKey); err != nil {
			return nil, "", err
		}
		queryParms.Add("endkey", "\""+endKey+"\"")
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodGet, "RangeDocRange", rangeURL, nil, "", "", maxRetries, true, &queryParms, "_all_docs")
	if err != nil {
		return nil, "", err
	}
	defer closeResponseBody(resp)

	if couchdbLogger.IsEnabledFor(zapcore.DebugLevel) {
		dump, err2 := httputil.DumpResponse(resp, false)
		if err2 != nil {
			log.Fatal(err2)
		}
		// compact debug log by replacing carriage return / line feed with dashes to separate http headers
		couchdbLogger.Debugf("[%s] HTTP Response: %s", dbclient.dbName, bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
	}

	// handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", errors.Wrap(err, "error reading response body")
	}

	jsonResponse := &rangeQueryResponse{}
	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, "", errors.Wrap(err2, "error unmarshalling json data")
	}

	// if an additional record is found, then reduce the count by 1
	// and populate the nextStartKey
	if jsonResponse.TotalRows > limit {
		jsonResponse.TotalRows = limit
	}

	couchdbLogger.Debugf("[%s] Total Rows: %d", dbclient.dbName, jsonResponse.TotalRows)

	// Use the next endKey as the starting default for the nextStartKey
	nextStartKey := endKey

	for index, row := range jsonResponse.Rows {

		docMetadata := &docMetadata{}
		err3 := json.Unmarshal(row.Doc, &docMetadata)
		if err3 != nil {
			return nil, "", errors.Wrap(err3, "error unmarshalling json data")
		}

		// if there is an extra row for the nextStartKey, then do not add the row to the result set
		// and populate the nextStartKey variable
		if int32(index) >= jsonResponse.TotalRows {
			nextStartKey = docMetadata.ID
			continue
		}

		if docMetadata.AttachmentsInfo != nil {

			couchdbLogger.Debugf("[%s] Adding JSON document and attachments for id: %s", dbclient.dbName, docMetadata.ID)

			attachments := []*attachmentInfo{}
			for attachmentName, attachment := range docMetadata.AttachmentsInfo {
				attachment.Name = attachmentName

				attachments = append(attachments, attachment)
			}

			addDocument := &queryResult{docMetadata.ID, row.Doc, attachments}
			results = append(results, addDocument)

		} else {

			couchdbLogger.Debugf("[%s] Adding json docment for id: %s", dbclient.dbName, docMetadata.ID)

			addDocument := &queryResult{docMetadata.ID, row.Doc, nil}
			results = append(results, addDocument)

		}

	}

	couchdbLogger.Debugf("[%s] Exiting ReadDocRange()", dbclient.dbName)

	return results, nextStartKey, nil
}

// deleteDoc method provides function to delete a document from the database by id
func (dbclient *couchDatabase) deleteDoc(id, rev string) error {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering DeleteDoc()  id=%s", dbName, id)

	deleteURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	// handle the request for saving document with a retry if there is a revision conflict
	resp, couchDBReturn, err := dbclient.handleRequestWithRevisionRetry(id, http.MethodDelete, dbName, "DeleteDoc",
		deleteURL, nil, "", "", maxRetries, true, nil)
	if err != nil {
		if couchDBReturn != nil && couchDBReturn.StatusCode == 404 {
			couchdbLogger.Debugf("[%s] Document not found (404), returning nil value instead of 404 error", dbclient.dbName)
			// non-existent document should return nil value instead of a 404 error
			// for details see https://github.com/hyperledger-archives/fabric/issues/936
			return nil
		}
		return err
	}
	defer closeResponseBody(resp)

	couchdbLogger.Debugf("[%s] Exiting DeleteDoc()", dbclient.dbName)

	return nil
}

// queryDocuments method provides function for processing a query
func (dbclient *couchDatabase) queryDocuments(query string) ([]*queryResult, string, error) {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering QueryDocuments()  query=%s", dbName, query)

	var results []*queryResult

	queryURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, "", errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodPost, "QueryDocuments", queryURL, []byte(query), "", "", maxRetries, true, nil, "_find")
	if err != nil {
		return nil, "", err
	}
	defer closeResponseBody(resp)

	if couchdbLogger.IsEnabledFor(zapcore.DebugLevel) {
		dump, err2 := httputil.DumpResponse(resp, false)
		if err2 != nil {
			log.Fatal(err2)
		}
		// compact debug log by replacing carriage return / line feed with dashes to separate http headers
		couchdbLogger.Debugf("[%s] HTTP Response: %s", dbclient.dbName, bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
	}

	// handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, "", errors.Wrap(err, "error reading response body")
	}

	jsonResponse := &queryResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, "", errors.Wrap(err2, "error unmarshalling json data")
	}

	if jsonResponse.Warning != "" {
		couchdbLogger.Warnf("The query [%s] caused the following warning: [%s]", query, jsonResponse.Warning)
	}

	for _, row := range jsonResponse.Docs {

		docMetadata := &docMetadata{}
		err3 := json.Unmarshal(row, &docMetadata)
		if err3 != nil {
			return nil, "", errors.Wrap(err3, "error unmarshalling json data")
		}

		// JSON Query results never have attachments
		// The If block below will never be executed
		if docMetadata.AttachmentsInfo != nil {

			couchdbLogger.Debugf("[%s] Adding JSON docment and attachments for id: %s", dbclient.dbName, docMetadata.ID)

			couchDoc, _, err := dbclient.readDoc(docMetadata.ID)
			if err != nil {
				return nil, "", err
			}
			addDocument := &queryResult{id: docMetadata.ID, value: couchDoc.jsonValue, attachments: couchDoc.attachments}
			results = append(results, addDocument)

		} else {
			couchdbLogger.Debugf("[%s] Adding json docment for id: %s", dbclient.dbName, docMetadata.ID)
			addDocument := &queryResult{id: docMetadata.ID, value: row, attachments: nil}

			results = append(results, addDocument)

		}
	}

	couchdbLogger.Debugf("[%s] Exiting QueryDocuments()", dbclient.dbName)

	return results, jsonResponse.Bookmark, nil
}

// listIndex method lists the defined indexes for a database
func (dbclient *couchDatabase) listIndex() ([]*indexResult, error) {
	// IndexDefinition contains the definition for a couchdb index
	type indexDefinition struct {
		DesignDocument string          `json:"ddoc"`
		Name           string          `json:"name"`
		Type           string          `json:"type"`
		Definition     json.RawMessage `json:"def"`
	}

	// ListIndexResponse contains the definition for listing couchdb indexes
	type listIndexResponse struct {
		TotalRows int               `json:"total_rows"`
		Indexes   []indexDefinition `json:"indexes"`
	}

	dbName := dbclient.dbName
	couchdbLogger.Debugf("[%s] Entering ListIndex()", dbName)

	indexURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodGet, "ListIndex", indexURL, nil, "", "", maxRetries, true, nil, "_index")
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	jsonResponse := &listIndexResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, jsonResponse)
	if err2 != nil {
		return nil, errors.Wrap(err2, "error unmarshalling json data")
	}

	var results []*indexResult

	for _, row := range jsonResponse.Indexes {

		// if the DesignDocument does not begin with "_design/", then this is a system
		// level index and is not meaningful and cannot be edited or deleted
		designDoc := row.DesignDocument
		s := strings.SplitAfterN(designDoc, "_design/", 2)
		if len(s) > 1 {
			designDoc = s[1]

			// Add the index definition to the results
			addIndexResult := &indexResult{DesignDocument: designDoc, Name: row.Name, Definition: string(row.Definition)}
			results = append(results, addIndexResult)
		}

	}

	couchdbLogger.Debugf("[%s] Exiting ListIndex()", dbclient.dbName)

	return results, nil
}

// createIndex method provides a function creating an index
func (dbclient *couchDatabase) createIndex(indexdefinition string) (*createIndexResponse, error) {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering CreateIndex()  indexdefinition=%s", dbName, indexdefinition)

	// Test to see if this is a valid JSON
	if !isJSON(indexdefinition) {
		return nil, errors.New("JSON format is not valid")
	}

	indexURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodPost, "CreateIndex", indexURL, []byte(indexdefinition), "", "", maxRetries, true, nil, "_index")
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if resp == nil {
		return nil, errors.New("invalid response received from CouchDB")
	}

	// Read the response body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	couchDBReturn := &createIndexResponse{}

	jsonBytes := []byte(respBody)

	// unmarshal the response
	err = json.Unmarshal(jsonBytes, &couchDBReturn)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshalling json data")
	}

	if couchDBReturn.Result == "created" {

		couchdbLogger.Infof("Created CouchDB index [%s] in state database [%s] using design document [%s]", couchDBReturn.Name, dbclient.dbName, couchDBReturn.ID)

		return couchDBReturn, nil

	}

	couchdbLogger.Infof("Updated CouchDB index [%s] in state database [%s] using design document [%s]", couchDBReturn.Name, dbclient.dbName, couchDBReturn.ID)

	return couchDBReturn, nil
}

// deleteIndex method provides a function deleting an index
func (dbclient *couchDatabase) deleteIndex(designdoc, indexname string) error {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering DeleteIndex()  designdoc=%s  indexname=%s", dbName, designdoc, indexname)

	indexURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodDelete, "DeleteIndex", indexURL, nil, "", "", maxRetries, true, nil, "_index", designdoc, "json", indexname)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	return nil
}

// getDatabaseSecurity method provides function to retrieve the security config for a database
func (dbclient *couchDatabase) getDatabaseSecurity() (*databaseSecurity, error) {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering GetDatabaseSecurity()", dbName)

	securityURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodGet, "GetDatabaseSecurity", securityURL, nil, "", "", maxRetries, true, nil, "_security")
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	// handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	jsonResponse := &databaseSecurity{}

	err2 := json.Unmarshal(jsonResponseRaw, jsonResponse)
	if err2 != nil {
		return nil, errors.Wrap(err2, "error unmarshalling json data")
	}

	couchdbLogger.Debugf("[%s] Exiting GetDatabaseSecurity()", dbclient.dbName)

	return jsonResponse, nil
}

// applyDatabaseSecurity method provides function to update the security config for a database
func (dbclient *couchDatabase) applyDatabaseSecurity(databaseSecurity *databaseSecurity) error {
	dbName := dbclient.dbName

	couchdbLogger.Debugf("[%s] Entering ApplyDatabaseSecurity()", dbName)

	securityURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	// Ensure all of the arrays are initialized to empty arrays instead of nil
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

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	databaseSecurityJSON, err := json.Marshal(databaseSecurity)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling json data")
	}

	couchdbLogger.Debugf("[%s] Applying security to database: %s", dbclient.dbName, string(databaseSecurityJSON))

	resp, _, err := dbclient.handleRequest(http.MethodPut, "ApplyDatabaseSecurity", securityURL, databaseSecurityJSON, "", "", maxRetries, true, nil, "_security")
	if err != nil {
		return err
	}
	defer closeResponseBody(resp)

	couchdbLogger.Debugf("[%s] Exiting ApplyDatabaseSecurity()", dbclient.dbName)

	return nil
}

// batchRetrieveDocumentMetadata - batch method to retrieve document metadata for  a set of keys,
// including ID, couchdb revision number, and ledger version
func (dbclient *couchDatabase) batchRetrieveDocumentMetadata(keys []string) ([]*docMetadata, error) {
	couchdbLogger.Debugf("[%s] Entering BatchRetrieveDocumentMetadata()  keys=%s", dbclient.dbName, keys)

	batchRetrieveURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	queryParms := batchRetrieveURL.Query()

	// While BatchRetrieveDocumentMetadata() does not return the entire document,
	// for reads/writes, we do need to get document so that we can get the ledger version of the key.
	// TODO For blind writes we do not need to get the version, therefore when we bulk get
	// the revision numbers for the write keys that were not represented in read set
	// (the second time BatchRetrieveDocumentMetadata is called during block processing),
	// we could set include_docs to false to optimize the response.
	queryParms.Add("include_docs", "true")

	keymap := make(map[string]interface{})

	keymap["keys"] = keys

	jsonKeys, err := json.Marshal(keymap)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling json data")
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodPost, "BatchRetrieveDocumentMetadata", batchRetrieveURL, jsonKeys, "", "", maxRetries, true, &queryParms, "_all_docs")
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if couchdbLogger.IsEnabledFor(zapcore.DebugLevel) {
		dump, _ := httputil.DumpResponse(resp, false)
		// compact debug log by replacing carriage return / line feed with dashes to separate http headers
		couchdbLogger.Debugf("[%s] HTTP Response: %s", dbclient.dbName, bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
	}

	// handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	jsonResponse := &batchRetrieveDocMetadataResponse{}

	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, errors.Wrap(err2, "error unmarshalling json data")
	}

	docMetadataArray := []*docMetadata{}

	for _, row := range jsonResponse.Rows {
		docMetadata := &docMetadata{ID: row.ID, Rev: row.DocMetadata.Rev, Version: row.DocMetadata.Version}
		docMetadataArray = append(docMetadataArray, docMetadata)
	}

	couchdbLogger.Debugf("[%s] Exiting BatchRetrieveDocumentMetadata()", dbclient.dbName)

	return docMetadataArray, nil
}

func (dbclient *couchDatabase) insertDocuments(docs []*couchDoc) error {
	responses, err := dbclient.batchUpdateDocuments(docs)
	if err != nil {
		return errors.WithMessage(err, "error while updating docs in bulk")
	}

	for i, resp := range responses {
		if resp.Ok {
			continue
		}
		if _, err := dbclient.saveDoc(resp.ID, "", docs[i]); err != nil {
			return errors.WithMessagef(err, "error while storing doc with ID %s", resp.ID)
		}
	}
	return nil
}

// batchUpdateDocuments - batch method to batch update documents
func (dbclient *couchDatabase) batchUpdateDocuments(documents []*couchDoc) ([]*batchUpdateResponse, error) {
	dbName := dbclient.dbName

	if couchdbLogger.IsEnabledFor(zapcore.DebugLevel) {
		documentIdsString, err := printDocumentIds(documents)
		if err == nil {
			couchdbLogger.Debugf("[%s] Entering BatchUpdateDocuments()  document ids=[%s]", dbName, documentIdsString)
		} else {
			couchdbLogger.Debugf("[%s] Entering BatchUpdateDocuments()  Could not print document ids due to error: %+v", dbName, err)
		}
	}

	batchUpdateURL, err := url.Parse(dbclient.couchInstance.url())
	if err != nil {
		couchdbLogger.Errorf("URL parse error: %s", err)
		return nil, errors.Wrapf(err, "error parsing CouchDB URL: %s", dbclient.couchInstance.url())
	}

	documentMap := make(map[string]interface{})

	var jsonDocumentMap []interface{}

	for _, jsonDocument := range documents {

		// create a document map
		document := make(map[string]interface{})

		// unmarshal the JSON component of the couchDoc into the document
		err = json.Unmarshal(jsonDocument.jsonValue, &document)
		if err != nil {
			return nil, errors.Wrap(err, "error unmarshalling json data")
		}

		// iterate through any attachments
		if len(jsonDocument.attachments) > 0 {

			// create a file attachment map
			fileAttachment := make(map[string]interface{})

			// for each attachment, create a base64Attachment, name the attachment,
			// add the content type and base64 encode the attachment
			for _, attachment := range jsonDocument.attachments {
				fileAttachment[attachment.Name] = base64Attachment{
					attachment.ContentType,
					base64.StdEncoding.EncodeToString(attachment.AttachmentBytes),
				}
			}

			// add attachments to the document
			document["_attachments"] = fileAttachment

		}

		// Append the document to the map of documents
		jsonDocumentMap = append(jsonDocumentMap, document)

	}

	// Add the documents to the "docs" item
	documentMap["docs"] = jsonDocumentMap

	bulkDocsJSON, err := json.Marshal(documentMap)
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling json data")
	}

	// get the number of retries
	maxRetries := dbclient.couchInstance.conf.MaxRetries

	resp, _, err := dbclient.handleRequest(http.MethodPost, "BatchUpdateDocuments", batchUpdateURL, bulkDocsJSON, "", "", maxRetries, true, nil, "_bulk_docs")
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp)

	if couchdbLogger.IsEnabledFor(zapcore.DebugLevel) {
		dump, _ := httputil.DumpResponse(resp, false)
		// compact debug log by replacing carriage return / line feed with dashes to separate http headers
		couchdbLogger.Debugf("[%s] HTTP Response: %s", dbclient.dbName, bytes.Replace(dump, []byte{0x0d, 0x0a}, []byte{0x20, 0x7c, 0x20}, -1))
	}

	// handle as JSON document
	jsonResponseRaw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	jsonResponse := []*batchUpdateResponse{}
	err2 := json.Unmarshal(jsonResponseRaw, &jsonResponse)
	if err2 != nil {
		return nil, errors.Wrap(err2, "error unmarshalling json data")
	}

	couchdbLogger.Debugf("[%s] Exiting BatchUpdateDocuments() _bulk_docs response=[%s]", dbclient.dbName, string(jsonResponseRaw))

	return jsonResponse, nil
}

// handleRequestWithRevisionRetry method is a generic http request handler with
// a retry for document revision conflict errors,
// which may be detected during saves or deletes that timed out from client http perspective,
// but which eventually succeeded in couchdb
func (dbclient *couchDatabase) handleRequestWithRevisionRetry(id, method, dbName, functionName string, connectURL *url.URL, data []byte, rev string,
	multipartBoundary string, maxRetries int, keepConnectionOpen bool, queryParms *url.Values) (*http.Response, *dbReturn, error) {
	// Initialize a flag for the revision conflict
	revisionConflictDetected := false
	var resp *http.Response
	var couchDBReturn *dbReturn
	var errResp error

	// attempt the http request for the max number of retries
	// In this case, the retry is to catch problems where a client timeout may miss a
	// successful CouchDB update and cause a document revision conflict on a retry in handleRequest
	for attempts := 0; attempts <= maxRetries; attempts++ {

		// if the revision was not passed in, or if a revision conflict is detected on prior attempt,
		// query CouchDB for the document revision
		if rev == "" || revisionConflictDetected {
			rev = dbclient.getDocumentRevision(id)
		}

		// handle the request for saving/deleting the couchdb data
		resp, couchDBReturn, errResp = dbclient.couchInstance.handleRequest(context.Background(), method, dbName, functionName, connectURL,
			data, rev, multipartBoundary, maxRetries, keepConnectionOpen, queryParms, id)

		// If there was a 409 conflict error during the save/delete, log it and retry it.
		// Otherwise, break out of the retry loop
		if couchDBReturn != nil && couchDBReturn.StatusCode == 409 {
			couchdbLogger.Warningf("CouchDB document revision conflict detected, retrying. Attempt:%v", attempts+1)
			revisionConflictDetected = true
		} else {
			break
		}
	}

	// return the handleRequest results
	return resp, couchDBReturn, errResp
}

func (dbclient *couchDatabase) handleRequest(method, functionName string, connectURL *url.URL, data []byte, rev, multipartBoundary string,
	maxRetries int, keepConnectionOpen bool, queryParms *url.Values, pathElements ...string) (*http.Response, *dbReturn, error) {
	return dbclient.couchInstance.handleRequest(context.Background(),
		method, dbclient.dbName, functionName, connectURL, data, rev, multipartBoundary,
		maxRetries, keepConnectionOpen, queryParms, pathElements...,
	)
}

// handleRequest method is a generic http request handler.
// If it returns an error, it ensures that the response body is closed, else it is the
// callee's responsibility to close response correctly.
// Any http error or CouchDB error (4XX or 500) will result in a golang error getting returned
func (couchInstance *couchInstance) handleRequest(ctx context.Context, method, dbName, functionName string, connectURL *url.URL, data []byte, rev string,
	multipartBoundary string, maxRetries int, keepConnectionOpen bool, queryParms *url.Values, pathElements ...string) (*http.Response, *dbReturn, error) {
	couchdbLogger.Debugf("Entering handleRequest()  method=%s  url=%v  dbName=%s", method, connectURL, dbName)

	// create the return objects for couchDB
	var resp *http.Response
	var errResp error
	couchDBReturn := &dbReturn{}
	defer couchInstance.recordMetric(time.Now(), dbName, functionName, couchDBReturn)

	// set initial wait duration for retries
	waitDuration := retryWaitTime * time.Millisecond

	if maxRetries < 0 {
		return nil, nil, errors.New("number of retries must be zero or greater")
	}

	requestURL := constructCouchDBUrl(connectURL, dbName, pathElements...)

	if queryParms != nil {
		requestURL.RawQuery = queryParms.Encode()
	}

	couchdbLogger.Debugf("Request URL: %s", requestURL)

	// attempt the http request for the max number of retries
	// if maxRetries is 0, the database creation will be attempted once and will
	//    return an error if unsuccessful
	// if maxRetries is 3 (default), a maximum of 4 attempts (one attempt with 3 retries)
	//    will be made with warning entries for unsuccessful attempts
	for attempts := 0; attempts <= maxRetries; attempts++ {

		// Set up a buffer for the payload data
		payloadData := new(bytes.Buffer)

		payloadData.ReadFrom(bytes.NewReader(data))

		// Create request based on URL for couchdb operation
		req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), payloadData)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error creating http request")
		}

		// set the request to close on completion if shared connections are not allowSharedConnection
		// Current CouchDB has a problem with zero length attachments, do not allow the connection to be reused.
		// Apache JIRA item for CouchDB   https://issues.apache.org/jira/browse/COUCHDB-3394
		if !keepConnectionOpen {
			req.Close = true
		}

		// add content header for PUT
		if method == http.MethodPut || method == http.MethodPost || method == http.MethodDelete {

			// If the multipartBoundary is not set, then this is a JSON and content-type should be set
			// to application/json.   Else, this is contains an attachment and needs to be multipart
			if multipartBoundary == "" {
				req.Header.Set("Content-Type", "application/json")
			} else {
				req.Header.Set("Content-Type", "multipart/related;boundary=\""+multipartBoundary+"\"")
			}

			// check to see if the revision is set,  if so, pass as a header
			if rev != "" {
				req.Header.Set("If-Match", rev)
			}
		}

		// add content header for PUT
		if method == http.MethodPut || method == http.MethodPost {
			req.Header.Set("Accept", "application/json")
		}

		// add content header for GET
		if method == http.MethodGet {
			req.Header.Set("Accept", "multipart/related")
		}

		// If username and password are set the use basic auth
		if couchInstance.conf.Username != "" && couchInstance.conf.Password != "" {
			// req.Header.Set("Authorization", "Basic YWRtaW46YWRtaW5w")
			req.SetBasicAuth(couchInstance.conf.Username, couchInstance.conf.Password)
		}

		// Execute http request
		resp, errResp = couchInstance.client.Do(req)

		// check to see if the return from CouchDB is valid
		if invalidCouchDBReturn(resp, errResp) {
			continue
		}

		// if there is no golang http error and no CouchDB 500 error, then drop out of the retry
		if errResp == nil && resp != nil && resp.StatusCode < 500 {
			// if this is an error, then populate the couchDBReturn
			if resp.StatusCode >= 400 {
				// Read the response body and close it for next attempt
				jsonError, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, errors.Wrap(err, "error reading response body")
				}
				defer closeResponseBody(resp)

				errorBytes := []byte(jsonError)
				// Unmarshal the response
				err = json.Unmarshal(errorBytes, &couchDBReturn)
				if err != nil {
					return nil, nil, errors.Wrap(err, "error unmarshalling json data")
				}
			}

			break
		}

		// If the maxRetries is greater than 0, then log the retry info
		if maxRetries > 0 {

			retryMessage := fmt.Sprintf("Retrying couchdb request in %s", waitDuration)
			if attempts == maxRetries {
				retryMessage = "Retries exhausted"
			}

			// if this is an unexpected golang http error, log the error and retry
			if errResp != nil {
				// Log the error with the retry count and continue
				couchdbLogger.Warningf("Attempt %d of %d returned error: %s. %s", attempts+1, maxRetries+1, errResp.Error(), retryMessage)

				// otherwise this is an unexpected 500 error from CouchDB. Log the error and retry.
			} else {
				// Read the response body and close it for next attempt
				jsonError, err := ioutil.ReadAll(resp.Body)
				defer closeResponseBody(resp)
				if err != nil {
					return nil, nil, errors.Wrap(err, "error reading response body")
				}

				errorBytes := []byte(jsonError)
				// Unmarshal the response
				err = json.Unmarshal(errorBytes, &couchDBReturn)
				if err != nil {
					return nil, nil, errors.Wrap(err, "error unmarshalling json data")
				}

				// Log the 500 error with the retry count and continue
				couchdbLogger.Warningf("Attempt %d of %d returned Couch DB Error:%s,  Status Code:%v  Reason:%s. %s",
					attempts+1, maxRetries+1, couchDBReturn.Error, resp.Status, couchDBReturn.Reason, retryMessage)

			}
			// if there are more retries remaining, sleep for specified sleep time, then retry
			if attempts < maxRetries {
				time.Sleep(waitDuration)
			}

			// backoff, doubling the retry time for next attempt
			waitDuration *= 2

		}

	} // end retry loop

	// if a golang http error is still present after retries are exhausted, return the error
	if errResp != nil {
		return nil, couchDBReturn, errors.Wrap(errResp, "http error calling couchdb")
	}

	// This situation should not occur according to the golang spec.
	// if this error returned (errResp) from an http call, then the resp should be not nil,
	// this is a structure and StatusCode is an int
	// This is meant to provide a more graceful error if this should occur
	if invalidCouchDBReturn(resp, errResp) {
		return nil, nil, errors.New("unable to connect to CouchDB, check the hostname and port")
	}

	// set the return code for the couchDB request
	couchDBReturn.StatusCode = resp.StatusCode

	// check to see if the status code from couchdb is 400 or higher
	// response codes 4XX and 500 will be treated as errors -
	// golang error will be created from the couchDBReturn contents and both will be returned
	if resp.StatusCode >= 400 {

		// if the status code is 400 or greater, log and return an error
		couchdbLogger.Debugf("Error handling CouchDB request. Error:%s,  Status Code:%v,  Reason:%s",
			couchDBReturn.Error, resp.StatusCode, couchDBReturn.Reason)

		return nil, couchDBReturn, errors.Errorf("error handling CouchDB request. Error:%s,  Status Code:%v,  Reason:%s",
			couchDBReturn.Error, resp.StatusCode, couchDBReturn.Reason)

	}

	couchdbLogger.Debugf("Exiting handleRequest()")

	// If no errors, then return the http response and the couchdb return object
	return resp, couchDBReturn, nil
}

func (couchInstance *couchInstance) recordMetric(startTime time.Time, dbName, api string, couchDBReturn *dbReturn) {
	couchInstance.stats.observeProcessingTime(startTime, dbName, api, strconv.Itoa(couchDBReturn.StatusCode))
}

// invalidCouchDBResponse checks to make sure either a valid response or error is returned
func invalidCouchDBReturn(resp *http.Response, errResp error) bool {
	if resp == nil && errResp == nil {
		return true
	}
	return false
}

// isJSON tests a string to determine if a valid JSON
func isJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

// encodePathElement uses Golang for url path encoding, additionally:
// '/' is replaced by %2F, otherwise path encoding will treat as path separator and ignore it
// '+' is replaced by %2B, otherwise path encoding will ignore it, while CouchDB will unencode the plus as a space
// Note that all other URL special characters have been tested successfully without need for special handling
func encodePathElement(str string) string {
	u := &url.URL{}
	u.Path = str
	encodedStr := u.EscapedPath() // url encode using golang url path encoding rules
	encodedStr = strings.Replace(encodedStr, "/", "%2F", -1)
	encodedStr = strings.Replace(encodedStr, "+", "%2B", -1)

	return encodedStr
}

func encodeForJSON(str string) (string, error) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(str); err != nil {
		return "", errors.Wrap(err, "error encoding json data")
	}
	// Encode adds double quotes to string and terminates with \n - stripping them as bytes as they are all ascii(0-127)
	buffer := buf.Bytes()
	return string(buffer[1 : len(buffer)-2]), nil
}

// printDocumentIds is a convenience method to print readable log entries for arrays of pointers
// to couch document IDs
func printDocumentIds(documentPointers []*couchDoc) (string, error) {
	documentIds := []string{}

	for _, documentPointer := range documentPointers {
		docMetadata := &docMetadata{}
		err := json.Unmarshal(documentPointer.jsonValue, &docMetadata)
		if err != nil {
			return "", errors.Wrap(err, "error unmarshalling json data")
		}
		documentIds = append(documentIds, docMetadata.ID)
	}
	return strings.Join(documentIds, ","), nil
}
