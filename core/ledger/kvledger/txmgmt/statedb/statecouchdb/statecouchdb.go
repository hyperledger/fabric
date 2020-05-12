/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/dataformat"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("statecouchdb")

const (
	// savepointDocID is used as a key for maintaining savepoint (maintained in metadatadb for a channel)
	savepointDocID = "statedb_savepoint"
	// fabricInternalDBName is used to create a db in couch that would be used for internal data such as the version of the data format
	// a double underscore ensures that the dbname does not clash with the dbnames created for the chaincodes
	fabricInternalDBName = "fabric__internal"
	// dataformatVersionDocID is used as a key for maintaining version of the data format (maintained in fabric internal db)
	dataformatVersionDocID = "dataformatVersion"
)

// VersionedDBProvider implements interface VersionedDBProvider
type VersionedDBProvider struct {
	couchInstance      *couchInstance
	databases          map[string]*VersionedDB
	mux                sync.Mutex
	openCounts         uint64
	redoLoggerProvider *redoLoggerProvider
	cache              *cache
}

// NewVersionedDBProvider instantiates VersionedDBProvider
func NewVersionedDBProvider(config *ledger.CouchDBConfig, metricsProvider metrics.Provider, sysNamespaces []string) (*VersionedDBProvider, error) {
	logger.Debugf("constructing CouchDB VersionedDBProvider")
	couchInstance, err := createCouchInstance(config, metricsProvider)
	if err != nil {
		return nil, err
	}
	if err := checkExpectedDataformatVersion(couchInstance); err != nil {
		return nil, err
	}
	p, err := newRedoLoggerProvider(config.RedoLogPath)
	if err != nil {
		return nil, err
	}

	cache := newCache(config.UserCacheSizeMBs, sysNamespaces)
	return &VersionedDBProvider{
			couchInstance:      couchInstance,
			databases:          make(map[string]*VersionedDB),
			mux:                sync.Mutex{},
			openCounts:         0,
			redoLoggerProvider: p,
			cache:              cache,
		},
		nil
}

func checkExpectedDataformatVersion(couchInstance *couchInstance) error {
	databasesToIgnore := []string{fabricInternalDBName}
	isEmpty, err := couchInstance.isEmpty(databasesToIgnore)
	if err != nil {
		return err
	}
	if isEmpty {
		logger.Debugf("couch instance is empty. Setting dataformat version to %s", dataformat.CurrentFormat)
		return writeDataFormatVersion(couchInstance, dataformat.CurrentFormat)
	}
	dataformatVersion, err := readDataformatVersion(couchInstance)
	if err != nil {
		return err
	}
	if dataformatVersion != dataformat.CurrentFormat {
		return &dataformat.ErrFormatMismatch{
			DBInfo:         "CouchDB for state database",
			ExpectedFormat: dataformat.CurrentFormat,
			Format:         dataformatVersion,
		}
	}
	return nil
}

func readDataformatVersion(couchInstance *couchInstance) (string, error) {
	db, err := createCouchDatabase(couchInstance, fabricInternalDBName)
	if err != nil {
		return "", err
	}
	doc, _, err := db.readDoc(dataformatVersionDocID)
	logger.Debugf("dataformatVersionDoc = %s", doc)
	if err != nil || doc == nil {
		return "", err
	}
	return decodeDataformatInfo(doc)
}

func writeDataFormatVersion(couchInstance *couchInstance, dataformatVersion string) error {
	db, err := createCouchDatabase(couchInstance, fabricInternalDBName)
	if err != nil {
		return err
	}
	doc, err := encodeDataformatInfo(dataformatVersion)
	if err != nil {
		return err
	}
	if _, err := db.saveDoc(dataformatVersionDocID, "", doc); err != nil {
		return err
	}
	dbResponse, err := db.ensureFullCommit()

	if err != nil {
		return err
	}
	if !dbResponse.Ok {
		logger.Errorf("failed to perform full commit while writing dataformat version")
		return errors.New("failed to perform full commit while writing dataformat version")
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
		vdb, err = newVersionedDB(
			provider.couchInstance,
			provider.redoLoggerProvider.newRedoLogger(dbName),
			dbName,
			provider.cache,
		)
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
	provider.redoLoggerProvider.close()
}

// HealthCheck checks to see if the couch instance of the peer is healthy
func (provider *VersionedDBProvider) HealthCheck(ctx context.Context) error {
	return provider.couchInstance.healthCheck(ctx)
}

// VersionedDB implements VersionedDB interface
type VersionedDB struct {
	couchInstance      *couchInstance
	metadataDB         *couchDatabase            // A database per channel to store metadata such as savepoint.
	chainName          string                    // The name of the chain/channel.
	namespaceDBs       map[string]*couchDatabase // One database per deployed chaincode.
	committedDataCache *versionsCache            // Used as a local cache during bulk processing of a block.
	verCacheLock       sync.RWMutex
	mux                sync.RWMutex
	redoLogger         *redoLogger
	cache              *cache
}

// newVersionedDB constructs an instance of VersionedDB
func newVersionedDB(couchInstance *couchInstance, redoLogger *redoLogger, dbName string, cache *cache) (*VersionedDB, error) {
	// CreateCouchDatabase creates a CouchDB database object, as well as the underlying database if it does not exist
	chainName := dbName
	dbName = constructMetadataDBName(dbName)

	metadataDB, err := createCouchDatabase(couchInstance, dbName)
	if err != nil {
		return nil, err
	}
	namespaceDBMap := make(map[string]*couchDatabase)
	vdb := &VersionedDB{
		couchInstance:      couchInstance,
		metadataDB:         metadataDB,
		chainName:          chainName,
		namespaceDBs:       namespaceDBMap,
		committedDataCache: newVersionCache(),
		redoLogger:         redoLogger,
		cache:              cache,
	}
	logger.Debugf("chain [%s]: checking for redolog record", chainName)
	redologRecord, err := redoLogger.load()
	if err != nil {
		return nil, err
	}
	savepoint, err := vdb.GetLatestSavePoint()
	if err != nil {
		return nil, err
	}

	// in normal circumstances, redolog is expected to be either equal to the last block
	// committed to the statedb or one ahead (in the event of a crash). However, either of
	// these or both could be nil on first time start (fresh start/rebuild)
	if redologRecord == nil || savepoint == nil {
		logger.Debugf("chain [%s]: No redo-record or save point present", chainName)
		return vdb, nil
	}

	logger.Debugf("chain [%s]: save point = %#v, version of redolog record = %#v",
		chainName, savepoint, redologRecord.Version)

	if redologRecord.Version.BlockNum-savepoint.BlockNum == 1 {
		logger.Debugf("chain [%s]: Re-applying last batch", chainName)
		if err := vdb.applyUpdates(redologRecord.UpdateBatch, redologRecord.Version); err != nil {
			return nil, err
		}
	}
	return vdb, nil
}

// getNamespaceDBHandle gets the handle to a named chaincode database
func (vdb *VersionedDB) getNamespaceDBHandle(namespace string) (*couchDatabase, error) {
	vdb.mux.RLock()
	db := vdb.namespaceDBs[namespace]
	vdb.mux.RUnlock()
	if db != nil {
		return db, nil
	}
	namespaceDBName := constructNamespaceDBName(vdb.chainName, namespace)
	vdb.mux.Lock()
	defer vdb.mux.Unlock()
	db = vdb.namespaceDBs[namespace]
	if db == nil {
		var err error
		db, err = createCouchDatabase(vdb.couchInstance, namespaceDBName)
		if err != nil {
			return nil, err
		}
		vdb.namespaceDBs[namespace] = db
	}
	return db, nil
}

// ProcessIndexesForChaincodeDeploy creates indexes for a specified namespace
func (vdb *VersionedDB) ProcessIndexesForChaincodeDeploy(namespace string, indexFilesData map[string][]byte) error {
	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return err
	}
	// We need to satisfy two requirements while processing the index files.
	// R1: all valid indexes should be processed.
	// R2: the order of index creation must be the same in all peers. For example, if user
	// passes two index files with the same index name but different index fields and we
	// process these files in different orders in different peers, each peer would
	// have different indexes (as one index definion could replace another if the index names
	// are the same).
	// To satisfy R1, we log the error and continue to process the next index file.
	// To satisfy R2, we sort the indexFilesData map based on the filenames and process
	// each index as per the sorted order.
	var indexFilesName []string
	for fileName := range indexFilesData {
		indexFilesName = append(indexFilesName, fileName)
	}
	sort.Strings(indexFilesName)
	for _, fileName := range indexFilesName {
		_, err = db.createIndex(string(indexFilesData[fileName]))
		switch {
		case err != nil:
			logger.Errorf("error creating index from file [%s] for chaincode [%s] on channel [%s]: %+v",
				fileName, namespace, vdb.chainName, err)
		default:
			logger.Infof("successfully submitted index creation request present in the file [%s] for chaincode [%s] on channel [%s]",
				fileName, namespace, vdb.chainName)
		}
	}
	return nil
}

// GetDBType returns the hosted stateDB
func (vdb *VersionedDB) GetDBType() string {
	return "couchdb"
}

// LoadCommittedVersions populates committedVersions and revisionNumbers into cache.
// A bulk retrieve from couchdb is used to populate the cache.
// committedVersions cache will be used for state validation of readsets
// revisionNumbers cache will be used during commit phase for couchdb bulk updates
func (vdb *VersionedDB) LoadCommittedVersions(keys []*statedb.CompositeKey) error {
	missingKeys := map[string][]string{}
	committedDataCache := newVersionCache()
	for _, compositeKey := range keys {
		ns, key := compositeKey.Namespace, compositeKey.Key
		committedDataCache.setVerAndRev(ns, key, nil, "")
		logger.Debugf("Load into version cache: %s~%s", ns, key)

		if !vdb.cache.enabled(ns) {
			missingKeys[ns] = append(missingKeys[ns], key)
			continue
		}
		cv, err := vdb.cache.getState(vdb.chainName, ns, key)
		if err != nil {
			return err
		}
		if cv == nil {
			missingKeys[ns] = append(missingKeys[ns], key)
			continue
		}
		vv, err := constructVersionedValue(cv)
		if err != nil {
			return err
		}
		rev := string(cv.AdditionalInfo)
		committedDataCache.setVerAndRev(ns, key, vv.Version, rev)
	}

	nsMetadataMap, err := vdb.retrieveMetadata(missingKeys)
	logger.Debugf("missingKeys=%s", missingKeys)
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
	version, keyFound := vdb.GetCachedVersion(namespace, key)
	if !keyFound {
		// This if block get executed only during simulation because during commit
		// we always call `LoadCommittedVersions` before calling `GetVersion`
		vv, err := vdb.GetState(namespace, key)
		if err != nil || vv == nil {
			return nil, err
		}
		version = vv.Version
	}
	return version, nil
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

// BytesKeySupported implements method in VersionvdbedDB interface
func (vdb *VersionedDB) BytesKeySupported() bool {
	return false
}

// GetState implements method in VersionedDB interface
func (vdb *VersionedDB) GetState(namespace string, key string) (*statedb.VersionedValue, error) {
	logger.Debugf("GetState(). ns=%s, key=%s", namespace, key)

	// (1) read the KV from the cache if available
	cacheEnabled := vdb.cache.enabled(namespace)
	if cacheEnabled {
		cv, err := vdb.cache.getState(vdb.chainName, namespace, key)
		if err != nil {
			return nil, err
		}
		if cv != nil {
			vv, err := constructVersionedValue(cv)
			if err != nil {
				return nil, err
			}
			return vv, nil
		}
	}

	// (2) read from the database if cache miss occurs
	kv, err := vdb.readFromDB(namespace, key)
	if err != nil {
		return nil, err
	}
	if kv == nil {
		return nil, nil
	}

	// (3) if the value is not nil, store in the cache
	if cacheEnabled {
		cacheValue := constructCacheValue(kv.VersionedValue, kv.revision)
		if err := vdb.cache.putState(vdb.chainName, namespace, key, cacheValue); err != nil {
			return nil, err
		}
	}

	return kv.VersionedValue, nil
}

func (vdb *VersionedDB) readFromDB(namespace, key string) (*keyValue, error) {
	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	couchDoc, _, err := db.readDoc(key)
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
	return kv, nil
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
	return vdb.GetStateRangeScanIteratorWithPagination(namespace, startKey, endKey, 0)
}

// GetStateRangeScanIteratorWithPagination implements method in VersionedDB interface
// startKey is inclusive
// endKey is exclusive
// pageSize limits the number of results returned
func (vdb *VersionedDB) GetStateRangeScanIteratorWithPagination(namespace string, startKey string, endKey string, pageSize int32) (statedb.QueryResultsIterator, error) {
	logger.Debugf("Entering GetStateRangeScanIteratorWithPagination namespace: %s  startKey: %s  endKey: %s  pageSize: %d", namespace, startKey, endKey, pageSize)
	internalQueryLimit := vdb.couchInstance.internalQueryLimit()
	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	return newQueryScanner(namespace, db, "", internalQueryLimit, pageSize, "", startKey, endKey)
}

func (scanner *queryScanner) getNextStateRangeScanResults() error {
	queryLimit := scanner.queryDefinition.internalQueryLimit
	if scanner.paginationInfo.requestedLimit > 0 {
		moreResultsNeeded := scanner.paginationInfo.requestedLimit - scanner.resultsInfo.totalRecordsReturned
		if moreResultsNeeded < scanner.queryDefinition.internalQueryLimit {
			queryLimit = moreResultsNeeded
		}
	}
	queryResult, nextStartKey, err := rangeScanFilterCouchInternalDocs(scanner.db,
		scanner.queryDefinition.startKey, scanner.queryDefinition.endKey, queryLimit)
	if err != nil {
		return err
	}
	scanner.resultsInfo.results = queryResult
	scanner.queryDefinition.startKey = nextStartKey
	scanner.paginationInfo.cursor = 0
	return nil
}

func rangeScanFilterCouchInternalDocs(db *couchDatabase,
	startKey, endKey string, queryLimit int32,
) ([]*queryResult, string, error) {
	var finalResults []*queryResult
	var finalNextStartKey string
	for {
		results, nextStartKey, err := db.readDocRange(startKey, endKey, queryLimit)
		if err != nil {
			logger.Debugf("Error calling ReadDocRange(): %s\n", err.Error())
			return nil, "", err
		}
		var filteredResults []*queryResult
		for _, doc := range results {
			if !isCouchInternalKey(doc.id) {
				filteredResults = append(filteredResults, doc)
			}
		}

		finalResults = append(finalResults, filteredResults...)
		finalNextStartKey = nextStartKey
		queryLimit = int32(len(results) - len(filteredResults))
		if queryLimit == 0 || finalNextStartKey == "" {
			break
		}
		startKey = finalNextStartKey
	}
	var err error
	for i := 0; isCouchInternalKey(finalNextStartKey); i++ {
		_, finalNextStartKey, err = db.readDocRange(finalNextStartKey, endKey, 1)
		logger.Debugf("i=%d, finalNextStartKey=%s", i, finalNextStartKey)
		if err != nil {
			return nil, "", err
		}
	}
	return finalResults, finalNextStartKey, nil
}

func isCouchInternalKey(key string) bool {
	return len(key) != 0 && key[0] == '_'
}

// ExecuteQuery implements method in VersionedDB interface
func (vdb *VersionedDB) ExecuteQuery(namespace, query string) (statedb.ResultsIterator, error) {
	queryResult, err := vdb.ExecuteQueryWithPagination(namespace, query, "", 0)
	if err != nil {
		return nil, err
	}
	return queryResult, nil
}

// ExecuteQueryWithPagination implements method in VersionedDB interface
func (vdb *VersionedDB) ExecuteQueryWithPagination(namespace, query, bookmark string, pageSize int32) (statedb.QueryResultsIterator, error) {
	logger.Debugf("Entering ExecuteQueryWithPagination namespace: %s,  query: %s,  bookmark: %s, pageSize: %d", namespace, query, bookmark, pageSize)
	internalQueryLimit := vdb.couchInstance.internalQueryLimit()
	queryString, err := applyAdditionalQueryOptions(query, internalQueryLimit, bookmark)
	if err != nil {
		logger.Errorf("Error calling applyAdditionalQueryOptions(): %s", err.Error())
		return nil, err
	}
	db, err := vdb.getNamespaceDBHandle(namespace)
	if err != nil {
		return nil, err
	}
	return newQueryScanner(namespace, db, queryString, internalQueryLimit, pageSize, bookmark, "", "")
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
	queryResult, bookmark, err := scanner.db.queryDocuments(queryString)
	if err != nil {
		logger.Debugf("Error calling QueryDocuments(): %s\n", err.Error())
		return err
	}
	scanner.resultsInfo.results = queryResult
	scanner.paginationInfo.bookmark = bookmark
	scanner.paginationInfo.cursor = 0
	return nil
}

// ApplyUpdates implements method in VersionedDB interface
func (vdb *VersionedDB) ApplyUpdates(updates *statedb.UpdateBatch, height *version.Height) error {
	if height != nil && updates.ContainsPostOrderWrites {
		// height is passed nil when committing missing private data for previously committed blocks
		r := &redoRecord{
			UpdateBatch: updates,
			Version:     height,
		}
		if err := vdb.redoLogger.persist(r); err != nil {
			return err
		}
	}
	return vdb.applyUpdates(updates, height)
}

func (vdb *VersionedDB) applyUpdates(updates *statedb.UpdateBatch, height *version.Height) error {
	// TODO a note about https://jira.hyperledger.org/browse/FAB-8622
	// The write lock is needed only for the stage 2.

	// stage 1 - buildCommitters builds committers per namespace (per DB). Each committer transforms the
	// given batch in the form of underlying db and keep it in memory.
	committers, err := vdb.buildCommitters(updates)
	if err != nil {
		return err
	}

	// stage 2 -- executeCommitter executes each committer to push the changes to the DB
	if err = vdb.executeCommitter(committers); err != nil {
		return err
	}

	// Stgae 3 - postCommitProcessing - flush and record savepoint.
	namespaces := updates.GetUpdatedNamespaces()
	if err := vdb.postCommitProcessing(committers, namespaces, height); err != nil {
		return err
	}

	return nil
}

func (vdb *VersionedDB) postCommitProcessing(committers []*committer, namespaces []string, height *version.Height) error {
	var wg sync.WaitGroup

	wg.Add(1)
	errChan := make(chan error, 1)
	defer close(errChan)
	go func() {
		defer wg.Done()

		cacheUpdates := make(cacheUpdates)
		for _, c := range committers {
			if !c.cacheEnabled {
				continue
			}
			cacheUpdates.add(c.namespace, c.cacheKVs)
		}

		if len(cacheUpdates) == 0 {
			return
		}

		// update the cache
		if err := vdb.cache.UpdateStates(vdb.chainName, cacheUpdates); err != nil {
			vdb.cache.Reset()
			errChan <- err
		}

	}()

	// Record a savepoint at a given height
	if err := vdb.ensureFullCommitAndRecordSavepoint(height, namespaces); err != nil {
		logger.Errorf("Error during recordSavepoint: %s", err.Error())
		return err
	}

	wg.Wait()
	select {
	case err := <-errChan:
		return errors.WithStack(err)
	default:
		return nil
	}
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

// ensureFullCommitAndRecordSavepoint flushes all the dbs (corresponding to `namespaces`) to disk
// and Record a savepoint in the metadata db.
// Couch parallelizes writes in cluster or sharded setup and ordering is not guaranteed.
// Hence we need to fence the savepoint with sync. So ensure_full_commit on all updated
// namespace DBs is called before savepoint to ensure all block writes are flushed. Savepoint
// itself is flushed to the metadataDB.
func (vdb *VersionedDB) ensureFullCommitAndRecordSavepoint(height *version.Height, namespaces []string) error {
	// ensure full commit to flush all changes on updated namespaces until now to disk
	// namespace also includes empty namespace which is nothing but metadataDB
	errsChan := make(chan error, len(namespaces))
	defer close(errsChan)
	var commitWg sync.WaitGroup
	commitWg.Add(len(namespaces))

	for _, ns := range namespaces {
		go func(ns string) {
			defer commitWg.Done()
			db, err := vdb.getNamespaceDBHandle(ns)
			if err != nil {
				errsChan <- err
				return
			}
			_, err = db.ensureFullCommit()
			if err != nil {
				errsChan <- err
				return
			}
		}(ns)
	}

	commitWg.Wait()

	select {
	case err := <-errsChan:
		logger.Errorf("Failed to perform full commit")
		return errors.Wrap(err, "failed to perform full commit")
	default:
		logger.Debugf("All changes have been flushed to the disk")
	}

	// If a given height is nil, it denotes that we are committing pvt data of old blocks.
	// In this case, we should not store a savepoint for recovery. The lastUpdatedOldBlockList
	// in the pvtstore acts as a savepoint for pvt data.
	if height == nil {
		return nil
	}

	// construct savepoint document and save
	savepointCouchDoc, err := encodeSavepoint(height)
	if err != nil {
		return err
	}
	_, err = vdb.metadataDB.saveDoc(savepointDocID, "", savepointCouchDoc)
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
	couchDoc, _, err := vdb.metadataDB.readDoc(savepointDocID)
	if err != nil {
		logger.Errorf("Failed to read savepoint data %s", err.Error())
		return nil, err
	}
	// ReadDoc() not found (404) will result in nil response, in these cases return height nil
	if couchDoc == nil || couchDoc.jsonValue == nil {
		return nil, nil
	}
	return decodeSavepoint(couchDoc)
}

func (vdb *VersionedDB) GetFullScanIterator(skipNamespace func(string) bool) (statedb.FullScanIterator, byte, error) {
	return nil, byte(0), errors.New("Not yet implemented")
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
	db              *couchDatabase
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
	results              []*queryResult
}

func newQueryScanner(namespace string, db *couchDatabase, query string, internalQueryLimit,
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
	key := selectedResultRecord.id
	// remove the reserved fields from CouchDB JSON and return the value and version
	kv, err := couchDocToKeyValue(&couchDoc{jsonValue: selectedResultRecord.value, attachments: selectedResultRecord.attachments})
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

func constructCacheValue(v *statedb.VersionedValue, rev string) *CacheValue {
	return &CacheValue{
		Version:        v.Version.ToBytes(),
		Value:          v.Value,
		Metadata:       v.Metadata,
		AdditionalInfo: []byte(rev),
	}
}

func constructVersionedValue(cv *CacheValue) (*statedb.VersionedValue, error) {
	height, _, err := version.NewHeightFromBytes(cv.Version)
	if err != nil {
		return nil, err
	}

	return &statedb.VersionedValue{
		Value:    cv.Value,
		Version:  height,
		Metadata: cv.Metadata,
	}, nil
}
