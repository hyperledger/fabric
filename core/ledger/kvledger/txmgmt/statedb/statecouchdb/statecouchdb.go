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
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	logging "github.com/op/go-logging"
)

var logger = flogging.MustGetLogger("statecouchdb")

const (
	binaryWrapper = "valueBytes"
	idField       = "_id"
	revField      = "_rev"
	versionField  = "~version"
	deletedField  = "_deleted"
)

var reservedFields = []string{idField, revField, versionField, deletedField}

var dbArtifactsDirFilter = map[string]bool{"META-INF/statedb/couchdb/indexes": true}

// querySkip is implemented for future use by query paging
// currently defaulted to 0 and is not used
const querySkip = 0

//BatchableDocument defines a document for a batch
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
	mux               sync.RWMutex
	// For now, we use one mutex for both versionNo and revisionNo. Having
	// two mutex might be a overkill.
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

//HandleChaincodeDeploy initializes database artifacts for the database associated with the namespace
// This function delibrately suppresses the errors that occur during the creation of the indexes on couchdb.
// This is because, in the present code, we do not differentiate between the errors because of couchdb interaction
// and the errors because of bad index files - the later being unfixable by the admin. Note that the error suppression
// is acceptable since peer can continue in the committing role without the indexes. However, executing chaincode queries
// may be affected, until a new chaincode with fixed indexes is installed and instantiated
func (vdb *VersionedDB) HandleChaincodeDeploy(chaincodeDefinition *cceventmgmt.ChaincodeDefinition, dbArtifactsTar []byte) error {
	logger.Debugf("Entering HandleChaincodeDeploy")
	if chaincodeDefinition == nil {
		return fmt.Errorf("chaincodeDefinition found nil while creating couchdb index on chain=%s", vdb.chainName)
	}
	db, err := vdb.getNamespaceDBHandle(chaincodeDefinition.Name)
	if err != nil {
		return err
	}

	fileEntries, err := ccprovider.ExtractFileEntries(dbArtifactsTar, dbArtifactsDirFilter)
	if err != nil {
		logger.Errorf("Error during extracting db artifacts from tar for chaincode=[%s] on chain=[%s]. Error=%s",
			chaincodeDefinition, vdb.chainName, err)
		return nil
	}

	for _, fileEntry := range fileEntries {
		indexData := fileEntry.FileContent
		filename := fileEntry.FileHeader.Name
		_, err = db.CreateIndex(string(indexData))
		if err != nil {
			logger.Errorf("Error during creation of index from file=[%s] for chaincode=[%s] on chain=[%s]. Error=%s",
				filename, chaincodeDefinition, vdb.chainName, err)
		}
	}

	return nil

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
	couchInstance *couchdb.CouchInstance
	metadataDB    *couchdb.CouchDatabase            // A database per channel to store metadata such as savepoint.
	chainName     string                            // The name of the chain/channel.
	namespaceDBs  map[string]*couchdb.CouchDatabase // One database per deployed chaincode.
	//TODO: Decide whether to split committedDataCache into multiple cahces, i.e., one per namespace.
	committedDataCache *CommittedVersions // Used as a local cache during bulk processing of a block.
	mux                sync.RWMutex
}

// newVersionedDB constructs an instance of VersionedDB
func newVersionedDB(couchInstance *couchdb.CouchInstance, dbName string) (*VersionedDB, error) {
	// CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
	chainName := dbName

	dbName = couchdb.ConstructMetadataDBName(dbName)

	metadataDB, err := couchdb.CreateCouchDatabase(*couchInstance, dbName)
	if err != nil {
		return nil, err
	}
	versionMap := make(map[statedb.CompositeKey]*version.Height)
	revMap := make(map[statedb.CompositeKey]string)
	namespaceDBMap := make(map[string]*couchdb.CouchDatabase)

	committedDataCache := &CommittedVersions{committedVersions: versionMap, revisionNumbers: revMap}

	return &VersionedDB{couchInstance, metadataDB, chainName, namespaceDBMap, committedDataCache, sync.RWMutex{}}, nil
}

// getNamespaceDBHandle gets the handle to a named chaincode database
func (vdb *VersionedDB) getNamespaceDBHandle(namespace string) (*couchdb.CouchDatabase, error) {

	vdb.mux.RLock()
	db := vdb.namespaceDBs[namespace]
	vdb.mux.RUnlock()

	if db != nil {
		return db, nil
	}

	namespaceDBName := couchdb.ConstructNamespaceDBName(vdb.chainName, namespace)

	vdb.mux.Lock()
	defer vdb.mux.Unlock()
	db = vdb.namespaceDBs[namespace]
	if db == nil {
		var err error
		db, err = couchdb.CreateCouchDatabase(*vdb.couchInstance, namespaceDBName)
		if err != nil {
			return nil, err
		}
		vdb.namespaceDBs[namespace] = db
	}
	return db, nil
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

// ValidateKeyValue implements method in VersionedDB interface
func (vdb *VersionedDB) ValidateKeyValue(key string, value []byte) error {
	if !utf8.ValidString(key) {
		return fmt.Errorf("Key should be a valid utf8 string: [%x]", key)
	}
	var jsonMap map[string]interface{}
	err := json.Unmarshal([]byte(value), &jsonMap)
	if err == nil {
		// the value is a proper json and hence perform a check that this json does not contain reserved field
		// if error is not nil then the value will be treated as a binary attachement.
		return checkReservedFieldsNotUsed(jsonMap)
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

	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	couchDoc, _, err := db.ReadDoc(key)
	if err != nil {
		return nil, err
	}
	if couchDoc == nil {
		return nil, nil
	}

	// remove the reserved fields from the CouchDB JSON and return the value and version
	returnValue, returnVersion, err := getValueAndVersionFromDoc(couchDoc.JSONValue, couchDoc.Attachments)
	if err != nil {
		return nil, err
	}

	return &statedb.VersionedValue{Value: returnValue, Version: returnVersion}, nil
}

//GetCachedVersion implements method in VersionedDB interface
func (vdb *VersionedDB) GetCachedVersion(namespace string, key string) (*version.Height, bool) {

	logger.Debugf("Retrieving cached version: %s~%s", key, namespace)

	compositeKey := statedb.CompositeKey{Namespace: namespace, Key: key}

	// Retrieve the version from committed data cache.
	// Since the cache was populated based on block readsets,
	// checks during validation should find the version here
	vdb.committedDataCache.mux.RLock()
	version, keyFound := vdb.committedDataCache.committedVersions[compositeKey]
	vdb.committedDataCache.mux.RUnlock()

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

		db, err := vdb.getNamespaceDBHandle(namespace)
		if err != nil {
			return nil, err
		}
		couchDoc, _, err := db.ReadDoc(key)

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

// remove the reserved fields from CouchDB JSON and return the value and version
func getValueAndVersionFromDoc(persistedValue []byte, attachments []*couchdb.AttachmentInfo) ([]byte, *version.Height, error) {

	// initialize the return value
	returnValue := []byte{}

	// create a generic map for the json
	jsonResult := make(map[string]interface{})

	// unmarshal the selected json into the generic map
	decoder := json.NewDecoder(bytes.NewBuffer(persistedValue))
	decoder.UseNumber()
	err := decoder.Decode(&jsonResult)
	if err != nil {
		return nil, nil, err
	}

	// verify the version field exists
	if _, fieldFound := jsonResult[versionField]; !fieldFound {
		return nil, nil, fmt.Errorf("The version field %s was not found", versionField)
	}

	// create the return version from the version field in the JSON
	returnVersion := createVersionHeightFromVersionString(jsonResult[versionField].(string))

	// remove the _id, _rev and version fields
	delete(jsonResult, idField)
	delete(jsonResult, revField)
	delete(jsonResult, versionField)

	// handle binary or json data
	if attachments != nil { // binary attachment
		// get binary data from attachment
		for _, attachment := range attachments {
			if attachment.Name == binaryWrapper {
				returnValue = attachment.AttachmentBytes
			}
		}
	} else {

		// marshal the returned JSON data.
		returnValue, err = json.Marshal(jsonResult)
		if err != nil {
			return nil, nil, err
		}

	}

	return returnValue, returnVersion, nil

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

	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}

	queryResult, err := db.ReadDocRange(startKey, endKey, queryLimit, querySkip)
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

	// Explicit paging not yet supported.
	// Use queryLimit from config and 0 skip.
	queryString, err := applyAdditionalQueryOptions(query, queryLimit, 0)
	if err != nil {
		logger.Debugf("Error calling applyAdditionalQueryOptions(): %s\n", err.Error())
		return nil, err
	}

	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	queryResult, err := db.QueryDocuments(queryString)
	if err != nil {
		logger.Debugf("Error calling QueryDocuments(): %s\n", err.Error())
		return nil, err
	}

	logger.Debugf("Exiting ExecuteQuery")
	return newQueryScanner(namespace, *queryResult), nil
}

// applyAdditionalQueryOptions will add additional fields to the query required for query processing
func applyAdditionalQueryOptions(queryString string, queryLimit, querySkip int) (string, error) {

	const jsonQueryFields = "fields"
	const jsonQueryLimit = "limit"
	const jsonQuerySkip = "skip"

	//create a generic map for the query json
	jsonQueryMap := make(map[string]interface{})

	//unmarshal the selector json into the generic map
	decoder := json.NewDecoder(bytes.NewBuffer([]byte(queryString)))
	decoder.UseNumber()
	err := decoder.Decode(&jsonQueryMap)
	if err != nil {
		return "", err
	}

	if fieldsJSONArray, ok := jsonQueryMap[jsonQueryFields]; ok {

		switch fieldsJSONArray.(type) {

		case []interface{}:

			//Add the "_id", and "version" fields,  these are needed by default
			jsonQueryMap[jsonQueryFields] = append(fieldsJSONArray.([]interface{}),
				idField, versionField)

		default:
			return "", fmt.Errorf("Fields definition must be an array.")

		}

	}

	// Add limit
	// This will override any limit passed in the query.
	// Explicit paging not yet supported.
	jsonQueryMap[jsonQueryLimit] = queryLimit

	// Add skip of 0.
	// This will override any skip passed in the query.
	// Explicit paging not yet supported.
	jsonQueryMap[jsonQuerySkip] = querySkip

	//Marshal the updated json query
	editedQuery, err := json.Marshal(jsonQueryMap)
	if err != nil {
		return "", err
	}

	logger.Debugf("Rewritten query: %s", editedQuery)

	return string(editedQuery), nil

}

// ApplyUpdates implements method in VersionedDB interface
func (vdb *VersionedDB) ApplyUpdates(batch *statedb.UpdateBatch, height *version.Height) error {

	// STEP 1: GATHER DOCUMENT REVISION NUMBERS REQUIRED FOR THE COUCHDB BULK UPDATE
	//         ALSO BUILD PROCESS BATCHES OF UPDATE DOCUMENTS PER NAMESPACE BASED ON
	// 	   THE MAX BATCH SIZE

	namespaces := batch.GetUpdatedNamespaces()

	// Create goroutine wait group.
	var processBatchGroup sync.WaitGroup
	processBatchGroup.Add(len(namespaces))

	// Collect error from each goroutine using buffered channel.
	errResponses := make(chan error, len(namespaces))

	for _, ns := range namespaces {
		// each namespace batch is processed and committed parallely.
		go func(ns string) {
			defer processBatchGroup.Done()

			// initialize a missing key list
			var missingKeys []*statedb.CompositeKey

			//initialize a processBatch for updating bulk documents
			processBatch := statedb.NewUpdateBatch()

			//get the max size of the batch from core.yaml
			maxBatchSize := ledgerconfig.GetMaxBatchUpdateSize()

			//initialize a counter to track the batch size
			batchSizeCounter := 0

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
				vdb.committedDataCache.mux.RLock()
				_, keyFound := vdb.committedDataCache.revisionNumbers[compositeKey]
				vdb.committedDataCache.mux.RUnlock()

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
						errResponses <- err
						return
					}

					//reset the batch size counter
					batchSizeCounter = 0

					//create a new process batch
					processBatch = statedb.NewUpdateBatch()

					// reset the missing key list
					missingKeys = []*statedb.CompositeKey{}
				}
			}

			//STEP 3:  PROCESS ANY REMAINING DOCUMENTS
			err := vdb.processUpdateBatch(processBatch, missingKeys)
			if err != nil {
				errResponses <- err
				return
			}
		}(ns)
	}

	// Wait for all goroutines to complete
	processBatchGroup.Wait()

	// Check if any goroutine resulted in error.
	// We can stop all goroutine as soon as any goutine resulted in error rather than
	// waiting for all goroutines to complete. As errors are very rare, current sub-optimal
	// approach (allowing each subroutine to complete) is adequate for now.
	// TODO: Currently, we are returing only one error. We need to create a new error type
	// that can encapsulate all the errors and return that type
	if len(errResponses) > 0 {
		return <-errResponses
	}

	// Record a savepoint at a given height
	err := vdb.recordSavepoint(height, namespaces)
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

		err := vdb.LoadCommittedVersions(missingKeys)
		if err != nil {
			return err
		}
	}

	// STEP 1: CREATE COUCHDB DOCS FROM UPDATE SET THEN DO A BULK UPDATE IN COUCHDB

	// Use the batchUpdateMap for tracking couchdb updates by ID
	// this will be used in case there are retries required
	batchUpdateMap := make(map[string]*BatchableDocument)

	//TODO: processUpdateBatch is called with updateBatch of a single namespace/chaincode at a time.
	// Hence, retrieving namespaces from updateBatch and looping over it is not required. Need to remove
	// only the outer for loop.
	namespaces := updateBatch.GetUpdatedNamespaces()
	for _, ns := range namespaces {
		nsUpdates := updateBatch.GetUpdates(ns)
		for key, vv := range nsUpdates {

			// Create a document structure
			couchDoc := &couchdb.CouchDoc{}
			var err error

			// retrieve the couchdb revision from the cache
			// Documents that do not exist in couchdb will not have revision numbers and will
			// exist in the cache with a revision value of nil
			vdb.committedDataCache.mux.RLock()
			revision := vdb.committedDataCache.revisionNumbers[statedb.CompositeKey{Namespace: ns, Key: key}]
			vdb.committedDataCache.mux.RUnlock()

			var isDelete bool // initialized to false
			if vv.Value == nil {
				isDelete = true
			}

			logger.Debugf("Channel [%s]: namespace=[%s] key=[%#v], prior revision=[%s], isDelete=[%t]",
				vdb.chainName, ns, key, revision, isDelete)

			if isDelete {
				// this is a deleted record.  Set the _deleted property to true
				couchDoc.JSONValue, err = createCouchdbDocJSON(key, revision, nil, vv.Version, true)
				if err != nil {
					return err
				}

			} else {

				if couchdb.IsJSON(string(vv.Value)) {
					// Handle as json
					couchDoc.JSONValue, err = createCouchdbDocJSON(key, revision, vv.Value, vv.Version, false)
					if err != nil {
						return err
					}

				} else { // if value is not json, handle as a couchdb attachment

					attachment := &couchdb.AttachmentInfo{}
					attachment.AttachmentBytes = vv.Value
					attachment.ContentType = "application/octet-stream"
					attachment.Name = binaryWrapper
					attachments := append([]*couchdb.AttachmentInfo{}, attachment)

					couchDoc.Attachments = attachments
					couchDoc.JSONValue, err = createCouchdbDocJSON(key, revision, nil, vv.Version, false)
					if err != nil {
						return err
					}

				}
			}

			// Add the current docment, revision and delete flag to the update map
			batchUpdateMap[key] = &BatchableDocument{CouchDoc: *couchDoc, Deleted: isDelete}

		}

		if len(batchUpdateMap) > 0 {

			//Add the documents to the batch update array
			batchUpdateDocs := []*couchdb.CouchDoc{}
			for _, updateDocument := range batchUpdateMap {
				batchUpdateDocument := updateDocument
				batchUpdateDocs = append(batchUpdateDocs, &batchUpdateDocument.CouchDoc)
			}

			// Do the bulk update into couchdb
			// Note that this will do retries if the entire bulk update fails or times out

			db, err := vdb.getNamespaceDBHandle(ns)
			if err != nil {
				return err
			}
			batchUpdateResp, err := db.BatchUpdateDocuments(batchUpdateDocs)
			if err != nil {
				return err
			}

			// STEP 2: IF INDIVIDUAL DOCUMENTS IN THE BULK UPDATE DID NOT SUCCEED, TRY THEM INDIVIDUALLY

			// iterate through the response from CouchDB by document
			for _, respDoc := range batchUpdateResp {

				// If the document returned an error, retry the individual document
				if respDoc.Ok != true {

					batchUpdateDocument := batchUpdateMap[respDoc.ID]

					var err error

					//Remove the "_rev" from the JSON before saving
					//this will allow the CouchDB retry logic to retry revisions without encountering
					//a mismatch between the "If-Match" and the "_rev" tag in the JSON
					if batchUpdateDocument.CouchDoc.JSONValue != nil {
						err = removeJSONRevision(&batchUpdateDocument.CouchDoc.JSONValue)
						if err != nil {
							return err
						}
					}

					// Check to see if the document was added to the batch as a delete type document
					if batchUpdateDocument.Deleted {

						//Log the warning message that a retry is being attempted for batch delete issue
						logger.Warningf("CouchDB batch document delete encountered an problem. Retrying delete for document ID:%s", respDoc.ID)

						// If this is a deleted document, then retry the delete
						// If the delete fails due to a document not being found (404 error),
						// the document has already been deleted and the DeleteDoc will not return an error
						err = db.DeleteDoc(respDoc.ID, "")
					} else {

						//Log the warning message that a retry is being attempted for batch update issue
						logger.Warningf("CouchDB batch document update encountered an problem. Retrying update for document ID:%s", respDoc.ID)

						// Save the individual document to couchdb
						// Note that this will do retries as needed
						_, err = db.SaveDoc(respDoc.ID, "", &batchUpdateDocument.CouchDoc)
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
func (vdb *VersionedDB) LoadCommittedVersions(keys []*statedb.CompositeKey) error {

	// initialize version and revision maps
	versionMap := vdb.committedDataCache.committedVersions
	revMap := vdb.committedDataCache.revisionNumbers

	missingKeys := make(map[string][]string) // for each namespace/chaincode, store the missingKeys

	vdb.committedDataCache.mux.Lock()
	for _, key := range keys {

		logger.Debugf("Load into version cache: %s~%s", key.Key, key.Namespace)

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
			// add the missing key to the list of required keys
			missingKeys[key.Namespace] = append(missingKeys[key.Namespace], key.Key)
		}

	}
	vdb.committedDataCache.mux.Unlock()

	//get the max size of the batch from core.yaml
	maxBatchSize := ledgerconfig.GetMaxBatchUpdateSize()

	// Call the batch retrieve if there is one or more keys to retrieve
	if len(missingKeys) > 0 {

		// Create goroutine wait group.
		var batchRetrieveGroup sync.WaitGroup
		batchRetrieveGroup.Add(len(missingKeys))

		// Collect error from each goroutine using buffered channel.
		errResponses := make(chan error, len(missingKeys))

		// For each namespace, we parallely load missing keys into the cache using goroutines
		for namespace := range missingKeys {
			go func(namespace string) {
				defer batchRetrieveGroup.Done()

				// Initialize the array of keys to be retrieved
				keysToRetrieve := []string{}

				// Iterate through the missingKeys and build a batch of keys for batch retrieval
				for _, key := range missingKeys[namespace] {

					keysToRetrieve = append(keysToRetrieve, key)

					// check to see if the number of keys is greater than the max batch size
					if len(keysToRetrieve) >= maxBatchSize {
						err := vdb.batchRetrieveMetaData(namespace, keysToRetrieve)
						if err != nil {
							errResponses <- err
							return
						}
						// reset the array
						keysToRetrieve = []string{}
					}

				}

				// If there are any remaining, retrieve the final batch
				if len(keysToRetrieve) > 0 {
					err := vdb.batchRetrieveMetaData(namespace, keysToRetrieve)
					if err != nil {
						errResponses <- err
						return
					}
				}
			}(namespace)
		}

		// Wait for all goroutines to complete
		batchRetrieveGroup.Wait()

		// Check if any goroutine resulted in error.
		// We can stop all goroutine as soon as any goutine resulted in error rather than
		// waiting for all goroutines to complete. As errors are very rare, current sub-optimal
		// approach (allowing each subroutine to complete) is adequate for now.
		// TODO: Currently, we are returing only one error. We need to create a new error type
		// that can encapsulate all the errors and return that type.
		if len(errResponses) > 0 {
			return <-errResponses
		}

	}

	return nil
}

// batchRetrieveMetaData retrieves a batch of keys and loads metadata into cache
func (vdb *VersionedDB) batchRetrieveMetaData(namespace string, keys []string) error {

	versionMap := vdb.committedDataCache.committedVersions
	revMap := vdb.committedDataCache.revisionNumbers

	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return err
	}

	documentMetadataArray, err := db.BatchRetrieveDocumentMetadata(keys)

	if err != nil {
		logger.Errorf("Batch retrieval of document metadata failed %s\n", err.Error())
		return err
	}

	for _, documentMetadata := range documentMetadataArray {

		if len(documentMetadata.Version) != 0 {
			compositeKey := statedb.CompositeKey{Namespace: namespace, Key: documentMetadata.ID}

			vdb.committedDataCache.mux.Lock()
			versionMap[compositeKey] = createVersionHeightFromVersionString(documentMetadata.Version)
			revMap[compositeKey] = documentMetadata.Rev
			vdb.committedDataCache.mux.Unlock()
		}
	}

	return nil
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
// _deleted - flag used in batch operations for deleting a couchdb document
// version - used for state validation
// The return value is the CouchDoc.JSONValue with the header fields populated
func createCouchdbDocJSON(id, revision string, value []byte, version *version.Height, deleted bool) ([]byte, error) {

	// create a new genericMap
	jsonMap := map[string]interface{}{}

	// if a JSON is provided, then unmarshal the JSON into the return mapping
	if value != nil {
		// Unmarshal the value into the return map
		err := json.Unmarshal(value, &jsonMap)
		if err != nil {
			return nil, err
		}
	}

	if err := checkReservedFieldsNotUsed(jsonMap); err != nil {
		return nil, err
	}

	// add the version
	jsonMap[versionField] = fmt.Sprintf("%v:%v", version.BlockNum, version.TxNum)

	// add the ID
	jsonMap[idField] = id

	// add the revision
	if revision != "" {
		// add the revision
		jsonMap[revField] = revision
	}

	// If this record is to be deleted, set the "_deleted" property to true
	if deleted {
		// add the deleted field
		jsonMap[deletedField] = true
	}

	// documentJSON represents the JSON document that will be sent in the CouchDB bulk update
	documentJSON, err := json.Marshal(jsonMap)
	if err != nil {
		return nil, err
	}

	return documentJSON, nil

}

// checkReservedFieldsNotUsed verifies that the reserve field was not included
func checkReservedFieldsNotUsed(jsonMap map[string]interface{}) error {
	for _, fieldName := range reservedFields {
		if _, fieldFound := jsonMap[fieldName]; fieldFound {
			return fmt.Errorf("The reserved field %s was found", fieldName)
		}
	}
	return nil
}

// removeJSONRevision removes the "_rev" if this is a JSON
func removeJSONRevision(jsonValue *[]byte) error {

	jsonMap := make(map[string]interface{})

	//Unmarshal the value into a map
	err := json.Unmarshal(*jsonValue, &jsonMap)
	if err != nil {
		logger.Errorf("Failed to unmarshal couchdb JSON data %s\n", err.Error())
		return err
	}

	//delete the "_rev" entry
	delete(jsonMap, revField)

	//marshal the updated map back into the byte array
	*jsonValue, err = json.Marshal(jsonMap)
	if err != nil {
		logger.Errorf("Failed to marshal couchdb JSON data %s\n", err.Error())
		return err
	}

	return nil

}

// Savepoint docid (key) for couchdb
const savepointDocID = "statedb_savepoint"

// Savepoint data for couchdb
type couchSavepointData struct {
	BlockNum uint64 `json:"BlockNum"`
	TxNum    uint64 `json:"TxNum"`
}

// recordSavepoint Record a savepoint in statedb.
// Couch parallelizes writes in cluster or sharded setup and ordering is not guaranteed.
// Hence we need to fence the savepoint with sync. So ensure_full_commit on all updated
// namespace DBs is called before savepoint to ensure all block writes are flushed. Savepoint
// itself is flushed to the metadataDB.
func (vdb *VersionedDB) recordSavepoint(height *version.Height, namespaces []string) error {
	var err error
	var savepointDoc couchSavepointData
	// ensure full commit to flush all changes on updated namespaces until now to disk
	// namespace also includes empty namespace which is nothing but metadataDB
	for _, ns := range namespaces {
		// TODO: Ensure full commit can be parallelized to improve performance
		db, err := vdb.getNamespaceDBHandle(ns)
		if err != nil {
			return err
		}
		dbResponse, err := db.EnsureFullCommit()

		if err != nil || dbResponse.Ok != true {
			logger.Errorf("Failed to perform full commit\n")
			return errors.New("Failed to perform full commit")
		}
	}

	// construct savepoint document
	savepointDoc.BlockNum = height.BlockNum
	savepointDoc.TxNum = height.TxNum

	savepointDocJSON, err := json.Marshal(savepointDoc)
	if err != nil {
		logger.Errorf("Failed to create savepoint data %s\n", err.Error())
		return err
	}

	// SaveDoc using couchdb client and use JSON format
	_, err = vdb.metadataDB.SaveDoc(savepointDocID, "", &couchdb.CouchDoc{JSONValue: savepointDocJSON, Attachments: nil})
	if err != nil {
		logger.Errorf("Failed to save the savepoint to DB %s\n", err.Error())
		return err
	}

	// Note: Ensure full commit on metadataDB after storing the savepoint is not necessary
	// as CouchDB syncs states to disk periodically (every 1 second). If peer fails before
	// syncing the savepoint to disk, ledger recovery process kicks in to ensure consistency
	// between CouchDB and block store on peer restart

	return nil
}

// GetLatestSavePoint implements method in VersionedDB interface
func (vdb *VersionedDB) GetLatestSavePoint() (*version.Height, error) {

	var err error
	couchDoc, _, err := vdb.metadataDB.ReadDoc(savepointDocID)
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

/*
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
*/

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

	key := selectedKV.ID

	// remove the reserved fields from CouchDB JSON and return the value and version
	returnValue, returnVersion, err := getValueAndVersionFromDoc(selectedKV.Value, selectedKV.Attachments)
	if err != nil {
		return nil, err
	}

	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: scanner.namespace, Key: key},
		VersionedValue: statedb.VersionedValue{Value: returnValue, Version: returnVersion}}, nil
}

func (scanner *kvScanner) Close() {
	scanner = nil
}

type queryScanner struct {
	cursor    int
	namespace string
	results   []couchdb.QueryResult
}

func newQueryScanner(namespace string, queryResults []couchdb.QueryResult) *queryScanner {
	return &queryScanner{-1, namespace, queryResults}
}

func (scanner *queryScanner) Next() (statedb.QueryResult, error) {

	scanner.cursor++

	if scanner.cursor >= len(scanner.results) {
		return nil, nil
	}

	selectedResultRecord := scanner.results[scanner.cursor]

	key := selectedResultRecord.ID

	// remove the reserved fields from CouchDB JSON and return the value and version
	returnValue, returnVersion, err := getValueAndVersionFromDoc(selectedResultRecord.Value, selectedResultRecord.Attachments)
	if err != nil {
		return nil, err
	}

	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: scanner.namespace, Key: key},
		VersionedValue: statedb.VersionedValue{Value: returnValue, Version: returnVersion}}, nil
}

func (scanner *queryScanner) Close() {
	scanner = nil
}
