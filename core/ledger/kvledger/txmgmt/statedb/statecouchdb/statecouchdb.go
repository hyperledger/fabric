/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	logging "github.com/op/go-logging"
)

var logger = flogging.MustGetLogger("statecouchdb")

var compositeKeySep = []byte{0x00}
var lastKeyIndicator = byte(0x01)

var binaryWrapper = "valueBytes"

// querySkip is implemented for future use by query paging
// currently defaulted to 0 and is not used
var querySkip = 0

// BatchDocument defines a document for a batch
type BatchableDocument struct {
	CouchDoc couchdb.CouchDoc
	Deleted  bool
}

// VersionedDBProvider implements interface VersionedDBProvider
type VersionedDBProvider struct {
	couchInstance *couchdb.CouchInstance
	databases     map[string]*VersionedDB
	mux           sync.Mutex
	openCounts    uint64
}

// CommittedVersions contains maps of committedVersions and revisionNumbers.
// Used as a local cache during bulk processing of a block.
// committedVersions - used for state validation of readsets
// revisionNumbers - used during commit phase for couchdb bulk updates
type CommittedVersions struct {
	committedVersions map[statedb.CompositeKey]*version.Height
	revisionNumbers   map[statedb.CompositeKey]string
}

// NewVersionedDBProvider instantiates VersionedDBProvider
func NewVersionedDBProvider() (*VersionedDBProvider, error) {
	logger.Debugf("constructing CouchDB VersionedDBProvider")
	couchDBDef := couchdb.GetCouchDBDefinition()
	couchInstance, err := couchdb.CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout)
	if err != nil {
		return nil, err
	}

	return &VersionedDBProvider{couchInstance, make(map[string]*VersionedDB), sync.Mutex{}, 0}, nil
}

// GetDBHandle gets the handle to a named database
func (provider *VersionedDBProvider) GetDBHandle(dbName string) (statedb.VersionedDB, error) {
	provider.mux.Lock()
	defer provider.mux.Unlock()

	vdb := provider.databases[dbName]
	if vdb == nil {
		var err error
		vdb, err = newVersionedDB(provider.couchInstance, dbName)
		if err != nil {
			return nil, err
		}
		provider.databases[dbName] = vdb
	}
	return vdb, nil
}

// Close closes the underlying db instance
func (provider *VersionedDBProvider) Close() {
	// No close needed on Couch
}

// VersionedDB implements VersionedDB interface
type VersionedDB struct {
	db                 *couchdb.CouchDatabase
	dbName             string
	committedDataCache *CommittedVersions // Used as a local cache during bulk processing of a block.
}

// newVersionedDB constructs an instance of VersionedDB
func newVersionedDB(couchInstance *couchdb.CouchInstance, dbName string) (*VersionedDB, error) {
	// CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
	db, err := couchdb.CreateCouchDatabase(*couchInstance, dbName)
	if err != nil {
		return nil, err
	}
	versionMap := make(map[statedb.CompositeKey]*version.Height)
	revMap := make(map[statedb.CompositeKey]string)

	committedDataCache := &CommittedVersions{committedVersions: versionMap, revisionNumbers: revMap}

	return &VersionedDB{db, dbName, committedDataCache}, nil
}

// Open implements method in VersionedDB interface
func (vdb *VersionedDB) Open() error {
	// no need to open db since a shared couch instance is used
	return nil
}

// Close implements method in VersionedDB interface
func (vdb *VersionedDB) Close() {
	// no need to close db since a shared couch instance is used
}

// ValidateKey implements method in VersionedDB interface
func (vdb *VersionedDB) ValidateKey(key string) error {
	if !utf8.ValidString(key) {
		return fmt.Errorf("Key should be a valid utf8 string: [%x]", key)
	}
	return nil
}

// BytesKeySuppoted implements method in VersionedDB interface
func (vdb *VersionedDB) BytesKeySuppoted() bool {
	return false
}

// GetState implements method in VersionedDB interface
func (vdb *VersionedDB) GetState(namespace string, key string) (*statedb.VersionedValue, error) {
	logger.Debugf("GetState(). ns=%s, key=%s", namespace, key)

	compositeKey := constructCompositeKey(namespace, key)

	couchDoc, _, err := vdb.db.ReadDoc(string(compositeKey))
	if err != nil {
		return nil, err
	}
	if couchDoc == nil {
		return nil, nil
	}

	// remove the data wrapper and return the value and version
	returnValue, returnVersion := removeDataWrapper(couchDoc.JSONValue, couchDoc.Attachments)

	return &statedb.VersionedValue{Value: returnValue, Version: returnVersion}, nil
}

// GetVersion implements method in VersionedDB interface
func (vdb *VersionedDB) GetCachedVersion(namespace string, key string) (*version.Height, bool) {

	logger.Debugf("Retrieving cached version: %s~%s", key, namespace)

	compositeKey := statedb.CompositeKey{Namespace: namespace, Key: key}

	// Retrieve the version from committed data cache.
	// Since the cache was populated based on block readsets,
	// checks during validation should find the version here
	version, keyFound := vdb.committedDataCache.committedVersions[compositeKey]

	if !keyFound {
		return nil, false
	}
	return version, true
}

// GetVersion implements method in VersionedDB interface
func (vdb *VersionedDB) GetVersion(namespace string, key string) (*version.Height, error) {

	returnVersion, keyFound := vdb.GetCachedVersion(namespace, key)

	// If the version was not found in the committed data cache, retrieve it from statedb.
	if !keyFound {

		couchDBCompositeKey := constructCompositeKey(namespace, key)
		couchDoc, _, err := vdb.db.ReadDoc(string(couchDBCompositeKey))
		if err != nil {
			return nil, err
		}
		if couchDoc == nil {
			return nil, nil
		}

		docMetadata := &couchdb.DocMetadata{}
		err = json.Unmarshal(couchDoc.JSONValue, &docMetadata)
		if err != nil {
			logger.Errorf("Failed to unmarshal couchdb doc header %s\n", err.Error())
			return nil, err
		}

		if docMetadata.Version == "" {
			return nil, nil
		}
		returnVersion = createVersionHeightFromVersionString(docMetadata.Version)
	}

	return returnVersion, nil
}

func removeDataWrapper(wrappedValue []byte, attachments []*couchdb.AttachmentInfo) ([]byte, *version.Height) {

	// initialize the return value
	returnValue := []byte{}

	// initialize a default return version
	returnVersion := version.NewHeight(0, 0)

	// create a generic map for the json
	jsonResult := make(map[string]interface{})

	// unmarshal the selected json into the generic map
	decoder := json.NewDecoder(bytes.NewBuffer(wrappedValue))
	decoder.UseNumber()
	_ = decoder.Decode(&jsonResult)

	// handle binary or json data
	if jsonResult[dataWrapper] == nil && attachments != nil { // binary attachment
		// get binary data from attachment
		for _, attachment := range attachments {
			if attachment.Name == binaryWrapper {
				returnValue = attachment.AttachmentBytes
			}
		}
	} else {
		// place the result json in the data key
		returnMap := jsonResult[dataWrapper]

		// marshal the mapped data.   this wrappers the result in a key named "data"
		returnValue, _ = json.Marshal(returnMap)

	}

	returnVersion = createVersionHeightFromVersionString(jsonResult["version"].(string))

	return returnValue, returnVersion

}

// GetStateMultipleKeys implements method in VersionedDB interface
func (vdb *VersionedDB) GetStateMultipleKeys(namespace string, keys []string) ([]*statedb.VersionedValue, error) {

	vals := make([]*statedb.VersionedValue, len(keys))
	for i, key := range keys {
		val, err := vdb.GetState(namespace, key)
		if err != nil {
			return nil, err
		}
		vals[i] = val
	}
	return vals, nil

}

// GetStateRangeScanIterator implements method in VersionedDB interface
// startKey is inclusive
// endKey is exclusive
func (vdb *VersionedDB) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (statedb.ResultsIterator, error) {

	// Get the querylimit from core.yaml
	queryLimit := ledgerconfig.GetQueryLimit()

	compositeStartKey := constructCompositeKey(namespace, startKey)
	compositeEndKey := constructCompositeKey(namespace, endKey)
	if endKey == "" {
		compositeEndKey[len(compositeEndKey)-1] = lastKeyIndicator
	}
	queryResult, err := vdb.db.ReadDocRange(string(compositeStartKey), string(compositeEndKey), queryLimit, querySkip)
	if err != nil {
		logger.Debugf("Error calling ReadDocRange(): %s\n", err.Error())
		return nil, err
	}
	logger.Debugf("Exiting GetStateRangeScanIterator")
	return newKVScanner(namespace, *queryResult), nil

}

// ExecuteQuery implements method in VersionedDB interface
func (vdb *VersionedDB) ExecuteQuery(namespace, query string) (statedb.ResultsIterator, error) {

	// Get the querylimit from core.yaml
	queryLimit := ledgerconfig.GetQueryLimit()

	queryString, err := ApplyQueryWrapper(namespace, query, queryLimit, 0)
	if err != nil {
		logger.Debugf("Error calling ApplyQueryWrapper(): %s\n", err.Error())
		return nil, err
	}

	queryResult, err := vdb.db.QueryDocuments(queryString)
	if err != nil {
		logger.Debugf("Error calling QueryDocuments(): %s\n", err.Error())
		return nil, err
	}
	logger.Debugf("Exiting ExecuteQuery")
	return newQueryScanner(*queryResult), nil
}

// ApplyUpdates implements method in VersionedDB interface
func (vdb *VersionedDB) ApplyUpdates(batch *statedb.UpdateBatch, height *version.Height) error {

	// STEP 1: GATHER DOCUMENT REVISION NUMBERS REQUIRED FOR THE COUCHDB BULK UPDATE
	//         ALSO BUILD PROCESS BATCHES OF UPDATE DOCUMENTS BASED ON THE MAX BATCH SIZE

	// initialize a missing key list
	var missingKeys []*statedb.CompositeKey

	//initialize a processBatch for updating bulk documents
	processBatch := statedb.NewUpdateBatch()

	//get the max size of the batch from core.yaml
	maxBatchSize := ledgerconfig.GetMaxBatchUpdateSize()

	//initialize a counter to track the batch size
	batchSizeCounter := 0

	// Iterate through the batch passed in and create process batches
	// using the max batch size
	namespaces := batch.GetUpdatedNamespaces()
	for _, ns := range namespaces {
		nsUpdates := batch.GetUpdates(ns)
		for k, vv := range nsUpdates {

			//increment the batch size counter
			batchSizeCounter++

			compositeKey := statedb.CompositeKey{Namespace: ns, Key: k}

			// Revision numbers are needed for couchdb updates.
			// vdb.committedDataCache.revisionNumbers is a cache of revision numbers based on ID
			// Document IDs and revision numbers will already be in the cache for read/writes,
			// but will be missing in the case of blind writes.
			// If the key is missing in the cache, then add the key to missingKeys
			_, keyFound := vdb.committedDataCache.revisionNumbers[compositeKey]

			if !keyFound {
				// Add the key to the missing key list
				// As there can be no duplicates in UpdateBatch, no need check for duplicates.
				missingKeys = append(missingKeys, &compositeKey)
			}

			//add the record to the process batch
			if vv.Value == nil {
				processBatch.Delete(ns, k, vv.Version)
			} else {
				processBatch.Put(ns, k, vv.Value, vv.Version)
			}

			//Check to see if the process batch exceeds the max batch size
			if batchSizeCounter >= maxBatchSize {

				//STEP 2:  PROCESS EACH BATCH OF UPDATE DOCUMENTS

				err := vdb.processUpdateBatch(processBatch, missingKeys)
				if err != nil {
					return err
				}

				//reset the batch size counter
				batchSizeCounter = 0

				//create a new process batch
				processBatch = statedb.NewUpdateBatch()

				// reset the missing key list
				missingKeys = []*statedb.CompositeKey{}

			}

		}
	}

	//STEP 3:  PROCESS ANY REMAINING DOCUMENTS
	err := vdb.processUpdateBatch(processBatch, missingKeys)
	if err != nil {
		return err
	}

	// STEP 4: IF THERE WAS SUCCESS UPDATING COUCHDB, THEN RECORD A SAVEPOINT FOR THIS BLOCK HEIGHT

	// Record a savepoint at a given height
	err = vdb.recordSavepoint(height)
	if err != nil {
		logger.Errorf("Error during recordSavepoint: %s\n", err.Error())
		return err
	}

	return nil
}

//ProcessUpdateBatch updates a batch
func (vdb *VersionedDB) processUpdateBatch(updateBatch *statedb.UpdateBatch, missingKeys []*statedb.CompositeKey) error {

	// An array of missing keys is passed in to the batch processing
	// A bulk read will then add the missing revisions to the cache
	if len(missingKeys) > 0 {

		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Retrieving keys with unknown revision numbers, keys= %s", printCompositeKeys(missingKeys))
		}

		vdb.LoadCommittedVersions(missingKeys)
	}

	// STEP 1: CREATE COUCHDB DOCS FROM UPDATE SET THEN DO A BULK UPDATE IN COUCHDB

	// Use the batchUpdateMap for tracking couchdb updates by ID
	// this will be used in case there are retries required
	batchUpdateMap := make(map[string]interface{})

	namespaces := updateBatch.GetUpdatedNamespaces()
	for _, ns := range namespaces {
		nsUpdates := updateBatch.GetUpdates(ns)
		for k, vv := range nsUpdates {
			compositeKey := constructCompositeKey(ns, k)

			// Create a document structure
			couchDoc := &couchdb.CouchDoc{}

			// retrieve the couchdb revision from the cache
			// Documents that do not exist in couchdb will not have revision numbers and will
			// exist in the cache with a revision value of nil
			revision := vdb.committedDataCache.revisionNumbers[statedb.CompositeKey{Namespace: ns, Key: k}]

			var isDelete bool // initialized to false
			if vv.Value == nil {
				isDelete = true
			}

			logger.Debugf("Channel [%s]: key(string)=[%s] key(bytes)=[%#v], prior revision=[%s], isDelete=[%t]",
				vdb.dbName, string(compositeKey), compositeKey, revision, isDelete)

			if isDelete {
				// this is a deleted record.  Set the _deleted property to true
				couchDoc.JSONValue = createCouchdbDocJSON(string(compositeKey), revision, nil, ns, vv.Version, true)

			} else {

				if couchdb.IsJSON(string(vv.Value)) {
					// Handle as json
					couchDoc.JSONValue = createCouchdbDocJSON(string(compositeKey), revision, vv.Value, ns, vv.Version, false)

				} else { // if value is not json, handle as a couchdb attachment

					attachment := &couchdb.AttachmentInfo{}
					attachment.AttachmentBytes = vv.Value
					attachment.ContentType = "application/octet-stream"
					attachment.Name = binaryWrapper
					attachments := append([]*couchdb.AttachmentInfo{}, attachment)

					couchDoc.Attachments = attachments
					couchDoc.JSONValue = createCouchdbDocJSON(string(compositeKey), revision, nil, ns, vv.Version, false)

				}
			}

			// Add the current docment to the update map
			batchUpdateMap[string(compositeKey)] = BatchableDocument{CouchDoc: *couchDoc, Deleted: isDelete}

		}
	}

	if len(batchUpdateMap) > 0 {

		//Add the documents to the batch update array
		batchUpdateDocs := []*couchdb.CouchDoc{}
		for _, updateDocument := range batchUpdateMap {
			batchUpdateDocument := updateDocument.(BatchableDocument)
			batchUpdateDocs = append(batchUpdateDocs, &batchUpdateDocument.CouchDoc)
		}

		// Do the bulk update into couchdb
		// Note that this will do retries if the entire bulk update fails or times out
		batchUpdateResp, err := vdb.db.BatchUpdateDocuments(batchUpdateDocs)
		if err != nil {
			return err
		}

		// STEP 2: IF INDIVIDUAL DOCUMENTS IN THE BULK UPDATE DID NOT SUCCEED, TRY THEM INDIVIDUALLY

		// iterate through the response from CouchDB by document
		for _, respDoc := range batchUpdateResp {

			// If the document returned an error, retry the individual document
			if respDoc.Ok != true {

				batchUpdateDocument := batchUpdateMap[respDoc.ID].(BatchableDocument)

				var err error

				// Check to see if the document was added to the batch as a delete type document
				if batchUpdateDocument.Deleted {
					// If this is a deleted document, then retry the delete
					// If the delete fails due to a document not being found (404 error),
					// the document has already been deleted and the DeleteDoc will not return an error
					err = vdb.db.DeleteDoc(respDoc.ID, "")
				} else {
					// Save the individual document to couchdb
					// Note that this will do retries as needed
					_, err = vdb.db.SaveDoc(respDoc.ID, "", &batchUpdateDocument.CouchDoc)
				}

				// If the single document update or delete returns an error, then throw the error
				if err != nil {

					errorString := fmt.Sprintf("Error occurred while saving document ID = %v  Error: %s  Reason: %s\n",
						respDoc.ID, respDoc.Error, respDoc.Reason)

					logger.Errorf(errorString)
					return fmt.Errorf(errorString)

				}
			}
		}

	}

	return nil
}

// printCompositeKeys is a convenience method to print readable log entries for arrays of pointers
// to composite keys
func printCompositeKeys(keys []*statedb.CompositeKey) string {

	compositeKeyString := []string{}
	for _, key := range keys {
		compositeKeyString = append(compositeKeyString, "["+key.Namespace+","+key.Key+"]")
	}
	return strings.Join(compositeKeyString, ",")
}

// LoadCommittedVersions populates committedVersions and revisionNumbers into cache.
// A bulk retrieve from couchdb is used to populate the cache.
// committedVersions cache will be used for state validation of readsets
// revisionNumbers cache will be used during commit phase for couchdb bulk updates
func (vdb *VersionedDB) LoadCommittedVersions(keys []*statedb.CompositeKey) {

	// initialize version and revision maps
	versionMap := vdb.committedDataCache.committedVersions
	revMap := vdb.committedDataCache.revisionNumbers

	keysToRetrieve := []string{}
	for _, key := range keys {

		logger.Debugf("Load into version cache: %s~%s", key.Key, key.Namespace)

		// create composite key for couchdb
		compositeDBKey := constructCompositeKey(key.Namespace, key.Key)

		// create the compositeKey
		compositeKey := statedb.CompositeKey{Namespace: key.Namespace, Key: key.Key}

		// initialize an entry for each key.  Set the version to nil
		_, versionFound := versionMap[compositeKey]
		if !versionFound {
			versionMap[compositeKey] = nil
		}

		// initialize empty values for each key (revision numbers will not be in couchdb for new creates)
		_, revFound := revMap[compositeKey]
		if !revFound {
			revMap[compositeKey] = ""
		}

		// if the compositeKey was not found in the revision or version part of the cache, then add the key to the
		// list of keys to be retrieved
		if !revFound || !versionFound {
			// add the composite key to the list of required keys
			keysToRetrieve = append(keysToRetrieve, string(compositeDBKey))
		}

	}

	// Call the batch retrieve if there is one or more keys to retrieve
	if len(keysToRetrieve) > 0 {

		documentMetadataArray, _ := vdb.db.BatchRetrieveDocumentMetadata(keysToRetrieve)

		for _, documentMetadata := range documentMetadataArray {

			if len(documentMetadata.Version) != 0 {
				ns, key := splitCompositeKey([]byte(documentMetadata.ID))
				compositeKey := statedb.CompositeKey{Namespace: ns, Key: key}

				versionMap[compositeKey] = createVersionHeightFromVersionString(documentMetadata.Version)
				revMap[compositeKey] = documentMetadata.Rev
			}
		}

	}

}

func createVersionHeightFromVersionString(encodedVersion string) *version.Height {

	versionArray := strings.Split(fmt.Sprintf("%s", encodedVersion), ":")

	// convert the blockNum from String to unsigned int
	blockNum, _ := strconv.ParseUint(versionArray[0], 10, 64)

	// convert the txNum from String to unsigned int
	txNum, _ := strconv.ParseUint(versionArray[1], 10, 64)

	return version.NewHeight(blockNum, txNum)

}

// ClearCachedVersions clears committedVersions and revisionNumbers
func (vdb *VersionedDB) ClearCachedVersions() {

	logger.Debugf("Clear Cache")

	versionMap := make(map[statedb.CompositeKey]*version.Height)
	revMap := make(map[statedb.CompositeKey]string)

	vdb.committedDataCache = &CommittedVersions{committedVersions: versionMap, revisionNumbers: revMap}

}

// createCouchdbDocJSON adds keys to the CouchDoc.JSON value for the following items:
// _id - couchdb document ID, need for all couchdb batch operations
// _rev - couchdb document revision, needed for updating or deleting existing documents
// _deleted - flag using in batch operations for deleting a couchdb document
// chaincodeID - chain code ID, added to header, used to scope couchdb queries
// version - version, added to header, used for state validation
// data wrapper - JSON from the chaincode goes here
// The return value is the CouchDoc.JSONValue with the header fields populated
func createCouchdbDocJSON(id, revision string, value []byte, chaincodeID string, version *version.Height, deleted bool) []byte {

	// create a version mapping
	jsonMap := map[string]interface{}{"version": fmt.Sprintf("%v:%v", version.BlockNum, version.TxNum)}

	// add the ID
	jsonMap["_id"] = id

	// add the revision
	if revision != "" {
		jsonMap["_rev"] = revision
	}

	// If this record is to be deleted, set the "_deleted" property to true
	if deleted {
		jsonMap["_deleted"] = true

	} else {

		// add the chaincodeID
		jsonMap["chaincodeid"] = chaincodeID

		// Add the wrapped data if the value is not null
		if value != nil {

			// create a new genericMap
			rawJSON := (*json.RawMessage)(&value)

			// add the rawJSON to the map
			jsonMap[dataWrapper] = rawJSON
		}
	}

	// documentJSON represents the JSON document that will be sent in the CouchDB bulk update
	documentJSON, _ := json.Marshal(jsonMap)
	return documentJSON
}

// Savepoint docid (key) for couchdb
const savepointDocID = "statedb_savepoint"

// Savepoint data for couchdb
type couchSavepointData struct {
	BlockNum  uint64 `json:"BlockNum"`
	TxNum     uint64 `json:"TxNum"`
	UpdateSeq string `json:"UpdateSeq"`
}

// recordSavepoint Record a savepoint in statedb.
// Couch parallelizes writes in cluster or sharded setup and ordering is not guaranteed.
// Hence we need to fence the savepoint with sync. So ensure_full_commit is called before
// savepoint to ensure all block writes are flushed. Savepoint itself does not need to be flushed,
// it will get flushed with next block if not yet committed.
func (vdb *VersionedDB) recordSavepoint(height *version.Height) error {
	var err error
	var savepointDoc couchSavepointData
	// ensure full commit to flush all changes until now to disk
	dbResponse, err := vdb.db.EnsureFullCommit()
	if err != nil || dbResponse.Ok != true {
		logger.Errorf("Failed to perform full commit\n")
		return errors.New("Failed to perform full commit")
	}

	// construct savepoint document
	// UpdateSeq would be useful if we want to get all db changes since a logical savepoint
	dbInfo, _, err := vdb.db.GetDatabaseInfo()
	if err != nil {
		logger.Errorf("Failed to get DB info %s\n", err.Error())
		return err
	}
	savepointDoc.BlockNum = height.BlockNum
	savepointDoc.TxNum = height.TxNum
	savepointDoc.UpdateSeq = dbInfo.UpdateSeq

	savepointDocJSON, err := json.Marshal(savepointDoc)
	if err != nil {
		logger.Errorf("Failed to create savepoint data %s\n", err.Error())
		return err
	}

	// SaveDoc using couchdb client and use JSON format
	_, err = vdb.db.SaveDoc(savepointDocID, "", &couchdb.CouchDoc{JSONValue: savepointDocJSON, Attachments: nil})
	if err != nil {
		logger.Errorf("Failed to save the savepoint to DB %s\n", err.Error())
		return err
	}

	return nil
}

// GetLatestSavePoint implements method in VersionedDB interface
func (vdb *VersionedDB) GetLatestSavePoint() (*version.Height, error) {

	var err error
	couchDoc, _, err := vdb.db.ReadDoc(savepointDocID)
	if err != nil {
		logger.Errorf("Failed to read savepoint data %s\n", err.Error())
		return nil, err
	}

	// ReadDoc() not found (404) will result in nil response, in these cases return height nil
	if couchDoc == nil || couchDoc.JSONValue == nil {
		return nil, nil
	}

	savepointDoc := &couchSavepointData{}
	err = json.Unmarshal(couchDoc.JSONValue, &savepointDoc)
	if err != nil {
		logger.Errorf("Failed to unmarshal savepoint data %s\n", err.Error())
		return nil, err
	}

	return &version.Height{BlockNum: savepointDoc.BlockNum, TxNum: savepointDoc.TxNum}, nil
}

func constructCompositeKey(ns string, key string) []byte {
	compositeKey := []byte(ns)
	compositeKey = append(compositeKey, compositeKeySep...)
	compositeKey = append(compositeKey, []byte(key)...)
	return compositeKey
}

func splitCompositeKey(compositeKey []byte) (string, string) {
	split := bytes.SplitN(compositeKey, compositeKeySep, 2)
	return string(split[0]), string(split[1])
}

type kvScanner struct {
	cursor    int
	namespace string
	results   []couchdb.QueryResult
}

func newKVScanner(namespace string, queryResults []couchdb.QueryResult) *kvScanner {
	return &kvScanner{-1, namespace, queryResults}
}

func (scanner *kvScanner) Next() (statedb.QueryResult, error) {

	scanner.cursor++

	if scanner.cursor >= len(scanner.results) {
		return nil, nil
	}

	selectedKV := scanner.results[scanner.cursor]

	_, key := splitCompositeKey([]byte(selectedKV.ID))

	// remove the data wrapper and return the value and version
	returnValue, returnVersion := removeDataWrapper(selectedKV.Value, selectedKV.Attachments)

	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: scanner.namespace, Key: key},
		VersionedValue: statedb.VersionedValue{Value: returnValue, Version: returnVersion}}, nil
}

func (scanner *kvScanner) Close() {
	scanner = nil
}

type queryScanner struct {
	cursor  int
	results []couchdb.QueryResult
}

func newQueryScanner(queryResults []couchdb.QueryResult) *queryScanner {
	return &queryScanner{-1, queryResults}
}

func (scanner *queryScanner) Next() (statedb.QueryResult, error) {

	scanner.cursor++

	if scanner.cursor >= len(scanner.results) {
		return nil, nil
	}

	selectedResultRecord := scanner.results[scanner.cursor]

	namespace, key := splitCompositeKey([]byte(selectedResultRecord.ID))

	// remove the data wrapper and return the value and version
	returnValue, returnVersion := removeDataWrapper(selectedResultRecord.Value, selectedResultRecord.Attachments)

	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: namespace, Key: key},
		VersionedValue: statedb.VersionedValue{Value: returnValue, Version: returnVersion}}, nil
}

func (scanner *queryScanner) Close() {
	scanner = nil
}
