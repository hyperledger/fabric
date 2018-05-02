/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("statecouchdb")

// querySkip is implemented for future use by query paging
// currently defaulted to 0 and is not used
const querySkip = 0

// VersionedDBProvider implements interface VersionedDBProvider
type VersionedDBProvider struct {
	couchInstance *couchdb.CouchInstance
	databases     map[string]*VersionedDB
	mux           sync.Mutex
	openCounts    uint64
}

// NewVersionedDBProvider instantiates VersionedDBProvider
func NewVersionedDBProvider() (*VersionedDBProvider, error) {
	logger.Debugf("constructing CouchDB VersionedDBProvider")
	couchDBDef := couchdb.GetCouchDBDefinition()
	couchInstance, err := couchdb.CreateCouchInstance(couchDBDef.URL, couchDBDef.Username, couchDBDef.Password,
		couchDBDef.MaxRetries, couchDBDef.MaxRetriesOnStartup, couchDBDef.RequestTimeout, couchDBDef.CreateGlobalChangesDB)
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
	couchInstance      *couchdb.CouchInstance
	metadataDB         *couchdb.CouchDatabase            // A database per channel to store metadata such as savepoint.
	chainName          string                            // The name of the chain/channel.
	namespaceDBs       map[string]*couchdb.CouchDatabase // One database per deployed chaincode.
	committedDataCache *versionsCache                    // Used as a local cache during bulk processing of a block.
	verCacheLock       sync.RWMutex
	mux                sync.RWMutex
}

// newVersionedDB constructs an instance of VersionedDB
func newVersionedDB(couchInstance *couchdb.CouchInstance, dbName string) (*VersionedDB, error) {
	// CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
	chainName := dbName
	dbName = couchdb.ConstructMetadataDBName(dbName)

	metadataDB, err := couchdb.CreateCouchDatabase(couchInstance, dbName)
	if err != nil {
		return nil, err
	}
	namespaceDBMap := make(map[string]*couchdb.CouchDatabase)
	return &VersionedDB{couchInstance: couchInstance, metadataDB: metadataDB, chainName: chainName, namespaceDBs: namespaceDBMap,
		committedDataCache: newVersionCache(), mux: sync.RWMutex{}}, nil
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
		db, err = couchdb.CreateCouchDatabase(vdb.couchInstance, namespaceDBName)
		if err != nil {
			return nil, err
		}
		vdb.namespaceDBs[namespace] = db
	}
	return db, nil
}

// ProcessIndexesForChaincodeDeploy creates indexes for a specified namespace
func (vdb *VersionedDB) ProcessIndexesForChaincodeDeploy(namespace string, fileEntries []*ccprovider.TarFileEntry) error {

	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return err
	}

	for _, fileEntry := range fileEntries {
		indexData := fileEntry.FileContent
		filename := fileEntry.FileHeader.Name
		_, err = db.CreateIndex(string(indexData))
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf(
				"error creating index from file [%s] for channel [%s]", filename, namespace))
		}
	}

	return nil

}

func (vdb *VersionedDB) GetDBType() string {
	return "couchdb"
}

// LoadCommittedVersions populates committedVersions and revisionNumbers into cache.
// A bulk retrieve from couchdb is used to populate the cache.
// committedVersions cache will be used for state validation of readsets
// revisionNumbers cache will be used during commit phase for couchdb bulk updates
func (vdb *VersionedDB) LoadCommittedVersions(keys []*statedb.CompositeKey) error {
	nsKeysMap := map[string][]string{}
	committedDataCache := newVersionCache()
	for _, compositeKey := range keys {
		ns, key := compositeKey.Namespace, compositeKey.Key
		committedDataCache.setVerAndRev(ns, key, nil, "")
		logger.Debugf("Load into version cache: %s~%s", ns, key)
		nsKeysMap[ns] = append(nsKeysMap[ns], key)
	}
	nsMetadataMap, err := vdb.retrieveMetadata(nsKeysMap)
	logger.Debugf("nsKeysMap=%s", nsKeysMap)
	logger.Debugf("nsMetadataMap=%s", nsMetadataMap)
	if err != nil {
		return err
	}
	for ns, nsMetadata := range nsMetadataMap {
		for _, keyMetadata := range nsMetadata {
			// TODO - why would version be ever zero if loaded from db?
			if len(keyMetadata.Version) != 0 {
				version, _, err := decodeVersionAndMetadata(keyMetadata.Version)
				if err != nil {
					return err
				}
				committedDataCache.setVerAndRev(ns, keyMetadata.ID, version, keyMetadata.Rev)
			}
		}
	}
	vdb.verCacheLock.Lock()
	defer vdb.verCacheLock.Unlock()
	vdb.committedDataCache = committedDataCache
	return nil
}

// GetVersion implements method in VersionedDB interface
func (vdb *VersionedDB) GetVersion(namespace string, key string) (*version.Height, error) {
	returnVersion, keyFound := vdb.GetCachedVersion(namespace, key)
	if !keyFound {
		// This if block get executed only during simulation because during commit
		// we always call `LoadCommittedVersions` before calling `GetVersion`
		vv, err := vdb.GetState(namespace, key)
		if err != nil || vv == nil {
			return nil, err
		}
		returnVersion = vv.Version
	}
	return returnVersion, nil
}

// GetCachedVersion returns version from cache. `LoadCommittedVersions` function populates the cache
func (vdb *VersionedDB) GetCachedVersion(namespace string, key string) (*version.Height, bool) {
	logger.Debugf("Retrieving cached version: %s~%s", key, namespace)
	vdb.verCacheLock.RLock()
	defer vdb.verCacheLock.RUnlock()
	return vdb.committedDataCache.getVersion(namespace, key)
}

// ValidateKeyValue implements method in VersionedDB interface
func (vdb *VersionedDB) ValidateKeyValue(key string, value []byte) error {
	err := validateKey(key)
	if err != nil {
		return err
	}
	return validateValue(value)
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
	kv, err := couchDocToKeyValue(couchDoc)
	if err != nil {
		return nil, err
	}
	return kv.VersionedValue, nil
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
	return vdb.GetStateRangeScanIteratorWithMetadata(namespace, startKey, endKey, nil)
}

const optionBookmark = "bookmark"
const optionLimit = "limit"
const returnCount = "count"

// GetStateRangeScanIteratorWithMetadata implements method in VersionedDB interface
// startKey is inclusive
// endKey is exclusive
// metadata contains a map of additional query options
func (vdb *VersionedDB) GetStateRangeScanIteratorWithMetadata(namespace string, startKey string, endKey string, metadata map[string]interface{}) (statedb.QueryResultsIterator, error) {

	logger.Debugf("Entering GetStateRangeScanIteratorWithMetadata  namespace: %s  startKey: %s  endKey: %s  metadata: %v", namespace, startKey, endKey, metadata)

	// Get the internalQueryLimit from core.yaml
	internalQueryLimit := int32(ledgerconfig.GetInternalQueryLimit())
	requestedLimit := int32(0)
	// if metadata is provided, validate and apply options
	if metadata != nil {
		//validate the metadata
		err := statedb.ValidateRangeMetadata(metadata)
		if err != nil {
			return nil, err
		}
		if limitOption, ok := metadata[optionLimit]; ok {
			requestedLimit = limitOption.(int32)
		}
	}
	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	return newQueryScanner(namespace, db, "", internalQueryLimit, requestedLimit, "", startKey, endKey)
}

func (scanner *queryScanner) getNextStateRangeScanResults() error {

	queryLimit := scanner.queryDefinition.internalQueryLimit
	if scanner.paginationInfo.requestedLimit > 0 {
		moreResultsNeeded := scanner.paginationInfo.requestedLimit - scanner.resultsInfo.totalRecordsReturned
		if moreResultsNeeded < scanner.queryDefinition.internalQueryLimit {
			queryLimit = moreResultsNeeded
		}
	}
	queryResult, nextStartKey, err := scanner.db.ReadDocRange(scanner.queryDefinition.startKey, scanner.queryDefinition.endKey,
		queryLimit)
	if err != nil {
		logger.Debugf("Error calling ReadDocRange(): %s\n", err.Error())
		return err
	}

	scanner.resultsInfo.results = queryResult
	scanner.queryDefinition.startKey = nextStartKey
	scanner.paginationInfo.cursor = 0

	return nil
}

// ExecuteQuery implements method in VersionedDB interface
func (vdb *VersionedDB) ExecuteQuery(namespace, query string) (statedb.ResultsIterator, error) {

	queryResult, err := vdb.ExecuteQueryWithMetadata(namespace, query, nil)
	if err != nil {
		return nil, err
	}

	return queryResult, nil
}

// ExecuteQueryWithMetadata implements method in VersionedDB interface
func (vdb *VersionedDB) ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (statedb.QueryResultsIterator, error) {

	logger.Debugf("Entering ExecuteQueryWithMetadata  namespace: %s,  query: %s,  metadata: %v", namespace, query, metadata)
	// Get the querylimit from core.yaml
	internalQueryLimit := int32(ledgerconfig.GetInternalQueryLimit())
	bookmark := ""
	requestedLimit := int32(0)
	// if metadata is provided, then validate and set provided options
	if metadata != nil {
		err := validateQueryMetadata(metadata)
		if err != nil {
			return nil, err
		}
		if limitOption, ok := metadata[optionLimit]; ok {
			requestedLimit = limitOption.(int32)
		}
		if bookmarkOption, ok := metadata[optionBookmark]; ok {
			bookmark = bookmarkOption.(string)
		}
	}
	queryString, err := applyAdditionalQueryOptions(query, internalQueryLimit, bookmark)
	if err != nil {
		logger.Errorf("Error calling applyAdditionalQueryOptions(): %s", err.Error())
		return nil, err
	}
	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	return newQueryScanner(namespace, db, queryString, internalQueryLimit, requestedLimit, bookmark, "", "")
}

// executeQueryWithBookmark executes a "paging" query with a bookmark, this method allows a
// paged query without returning a new query iterator
func (scanner *queryScanner) executeQueryWithBookmark() error {

	queryLimit := scanner.queryDefinition.internalQueryLimit
	if scanner.paginationInfo.requestedLimit > 0 {
		if scanner.paginationInfo.requestedLimit-scanner.resultsInfo.totalRecordsReturned < scanner.queryDefinition.internalQueryLimit {
			queryLimit = scanner.paginationInfo.requestedLimit - scanner.resultsInfo.totalRecordsReturned
		}
	}

	queryString, err := applyAdditionalQueryOptions(scanner.queryDefinition.query,
		queryLimit, scanner.paginationInfo.bookmark)
	if err != nil {
		logger.Debugf("Error calling applyAdditionalQueryOptions(): %s\n", err.Error())
		return err
	}

	queryResult, bookmark, err := scanner.db.QueryDocuments(queryString)
	if err != nil {
		logger.Debugf("Error calling QueryDocuments(): %s\n", err.Error())
		return err
	}

	scanner.resultsInfo.results = queryResult
	scanner.paginationInfo.bookmark = bookmark
	scanner.paginationInfo.cursor = 0

	return nil
}

func validateQueryMetadata(metadata map[string]interface{}) error {
	for key, keyVal := range metadata {
		switch key {

		case optionBookmark:
			//Verify the bookmark is a string
			if _, ok := keyVal.(string); ok {
				continue
			}
			return fmt.Errorf("Invalid entry, \"bookmark\" must be a string")

		case optionLimit:
			//Verify the limit is an integer
			if _, ok := keyVal.(int32); ok {
				continue
			}
			return fmt.Errorf("Invalid entry, \"limit\" must be an int32")

		default:
			return fmt.Errorf("Invalid entry, option %s not recognized", key)
		}
	}
	return nil
}

// ApplyUpdates implements method in VersionedDB interface
func (vdb *VersionedDB) ApplyUpdates(updates *statedb.UpdateBatch, height *version.Height) error {
	// TODO a note about https://jira.hyperledger.org/browse/FAB-8622
	// the function `Apply update can be split into three functions. Each carrying out one of the following three stages`.
	// The write lock is needed only for the stage 2.

	// stage 1 - PrepareForUpdates - db transforms the given batch in the form of underlying db
	// and keep it in memory
	var updateBatches []batch
	var err error
	if updateBatches, err = vdb.buildCommitters(updates); err != nil {
		return err
	}
	// stage 2 - ApplyUpdates push the changes to the DB
	if err = executeBatches(updateBatches); err != nil {
		return err
	}

	// Stgae 3 - PostUpdateProcessing - flush and record savepoint.
	namespaces := updates.GetUpdatedNamespaces()
	// Record a savepoint at a given height
	if err = vdb.ensureFullCommitAndRecordSavepoint(height, namespaces); err != nil {
		logger.Errorf("Error during recordSavepoint: %s", err.Error())
		return err
	}
	return nil
}

// ClearCachedVersions clears committedVersions and revisionNumbers
func (vdb *VersionedDB) ClearCachedVersions() {
	logger.Debugf("Clear Cache")
	vdb.verCacheLock.Lock()
	defer vdb.verCacheLock.Unlock()
	vdb.committedDataCache = newVersionCache()
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

// Savepoint docid (key) for couchdb
const savepointDocID = "statedb_savepoint"

// ensureFullCommitAndRecordSavepoint flushes all the dbs (corresponding to `namespaces`) to disk
// and Record a savepoint in the metadata db.
// Couch parallelizes writes in cluster or sharded setup and ordering is not guaranteed.
// Hence we need to fence the savepoint with sync. So ensure_full_commit on all updated
// namespace DBs is called before savepoint to ensure all block writes are flushed. Savepoint
// itself is flushed to the metadataDB.
func (vdb *VersionedDB) ensureFullCommitAndRecordSavepoint(height *version.Height, namespaces []string) error {
	// ensure full commit to flush all changes on updated namespaces until now to disk
	// namespace also includes empty namespace which is nothing but metadataDB
	var dbs []*couchdb.CouchDatabase
	for _, ns := range namespaces {
		db, err := vdb.getNamespaceDBHandle(ns)
		if err != nil {
			return err
		}
		dbs = append(dbs, db)
	}
	if err := vdb.ensureFullCommit(dbs); err != nil {
		return err
	}
	// construct savepoint document and save
	savepointCouchDoc, err := encodeSavepoint(height)
	if err != nil {
		return err
	}
	_, err = vdb.metadataDB.SaveDoc(savepointDocID, "", savepointCouchDoc)
	if err != nil {
		logger.Errorf("Failed to save the savepoint to DB %s", err.Error())
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
		logger.Errorf("Failed to read savepoint data %s", err.Error())
		return nil, err
	}
	// ReadDoc() not found (404) will result in nil response, in these cases return height nil
	if couchDoc == nil || couchDoc.JSONValue == nil {
		return nil, nil
	}
	return decodeSavepoint(couchDoc)
}

// applyAdditionalQueryOptions will add additional fields to the query required for query processing
func applyAdditionalQueryOptions(queryString string, queryLimit int32, queryBookmark string) (string, error) {
	const jsonQueryFields = "fields"
	const jsonQueryLimit = "limit"
	const jsonQueryBookmark = "bookmark"
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
			return "", errors.New("fields definition must be an array")
		}
	}
	// Add limit
	// This will override any limit passed in the query.
	// Explicit paging not yet supported.
	jsonQueryMap[jsonQueryLimit] = queryLimit
	// Add the bookmark if provided
	if queryBookmark != "" {
		jsonQueryMap[jsonQueryBookmark] = queryBookmark
	}
	//Marshal the updated json query
	editedQuery, err := json.Marshal(jsonQueryMap)
	if err != nil {
		return "", err
	}
	logger.Debugf("Rewritten query: %s", editedQuery)
	return string(editedQuery), nil
}

type queryScanner struct {
	namespace       string
	db              *couchdb.CouchDatabase
	queryDefinition *queryDefinition
	paginationInfo  *paginationInfo
	resultsInfo     *resultsInfo
}

type queryDefinition struct {
	startKey           string
	endKey             string
	query              string
	internalQueryLimit int32
}

type paginationInfo struct {
	cursor         int32
	requestedLimit int32
	bookmark       string
}

type resultsInfo struct {
	totalRecordsReturned int32
	results              []*couchdb.QueryResult
}

func newQueryScanner(namespace string, db *couchdb.CouchDatabase, query string, internalQueryLimit,
	limit int32, bookmark, startKey, endKey string) (*queryScanner, error) {

	scanner := &queryScanner{namespace, db, &queryDefinition{startKey, endKey, query, internalQueryLimit}, &paginationInfo{-1, limit, bookmark}, &resultsInfo{0, nil}}
	var err error

	// query is defined, then execute the query and return the records and bookmark
	if scanner.queryDefinition.query != "" {
		err = scanner.executeQueryWithBookmark()
	} else {
		err = scanner.getNextStateRangeScanResults()
	}
	if err != nil {
		return nil, err
	}
	scanner.paginationInfo.cursor = -1

	return scanner, nil
}

func (scanner *queryScanner) Next() (statedb.QueryResult, error) {

	//test for no results case
	if len(scanner.resultsInfo.results) == 0 {
		return nil, nil
	}

	// increment the cursor
	scanner.paginationInfo.cursor++

	// check to see if additional records are needed
	// requery if the cursor exceeds the internalQueryLimit
	if scanner.paginationInfo.cursor >= scanner.queryDefinition.internalQueryLimit {

		var err error
		// query is defined, then execute the query and return the records and bookmark
		if scanner.queryDefinition.query != "" {
			err = scanner.executeQueryWithBookmark()
		} else {
			err = scanner.getNextStateRangeScanResults()
		}
		if err != nil {
			return nil, err
		}

		//if no more results, then return
		if len(scanner.resultsInfo.results) == 0 {
			return nil, nil
		}

	}

	//If the cursor is greater than or equal to the number of result records, return
	if scanner.paginationInfo.cursor >= int32(len(scanner.resultsInfo.results)) {
		return nil, nil
	}

	selectedResultRecord := scanner.resultsInfo.results[scanner.paginationInfo.cursor]
	key := selectedResultRecord.ID

	// remove the reserved fields from CouchDB JSON and return the value and version
	kv, err := couchDocToKeyValue(&couchdb.CouchDoc{JSONValue: selectedResultRecord.Value, Attachments: selectedResultRecord.Attachments})
	if err != nil {
		return nil, err
	}

	scanner.resultsInfo.totalRecordsReturned++

	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: scanner.namespace, Key: key},
		VersionedValue: *kv.VersionedValue}, nil
}

func (scanner *queryScanner) Close() {
	scanner = nil
}

func (scanner *queryScanner) GetBookmarkAndClose() string {
	retval := ""
	if scanner.queryDefinition.query != "" {
		retval = scanner.paginationInfo.bookmark
	} else {
		retval = scanner.queryDefinition.startKey
	}
	scanner.Close()
	return retval
}
