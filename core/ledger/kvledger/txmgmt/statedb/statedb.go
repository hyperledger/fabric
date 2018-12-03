/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedb

import (
	"fmt"
	"sort"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
)

//go:generate counterfeiter -o mock/results_iterator.go -fake-name ResultsIterator . ResultsIterator
//go:generate counterfeiter -o mock/versioned_db.go -fake-name VersionedDB . VersionedDB

// VersionedDBProvider provides an instance of an versioned DB
type VersionedDBProvider interface {
	// GetDBHandle returns a handle to a VersionedDB
	GetDBHandle(id string) (VersionedDB, error)
	// Close closes all the VersionedDB instances and releases any resources held by VersionedDBProvider
	Close()
}

// VersionedDB lists methods that a db is supposed to implement
type VersionedDB interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) (*VersionedValue, error)
	// GetVersion gets the version for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetVersion(namespace string, key string) (*version.Height, error)
	// GetStateMultipleKeys gets the values for multiple keys in a single call
	GetStateMultipleKeys(namespace string, keys []string) ([]*VersionedValue, error)
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// startKey is inclusive
	// endKey is exclusive
	// The returned ResultsIterator contains results of type *VersionedKV
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)
	// GetStateRangeScanIteratorWithMetadata returns an iterator that contains all the key-values between given key ranges.
	// startKey is inclusive
	// endKey is exclusive
	// metadata is a map of additional query parameters
	// The returned ResultsIterator contains results of type *VersionedKV
	GetStateRangeScanIteratorWithMetadata(namespace string, startKey string, endKey string, metadata map[string]interface{}) (QueryResultsIterator, error)
	// ExecuteQuery executes the given query and returns an iterator that contains results of type *VersionedKV.
	ExecuteQuery(namespace, query string) (ResultsIterator, error)
	// ExecuteQueryWithMetadata executes the given query with associated query options and
	// returns an iterator that contains results of type *VersionedKV.
	// metadata is a map of additional query parameters
	ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (QueryResultsIterator, error)
	// ApplyUpdates applies the batch to the underlying db.
	// height is the height of the highest transaction in the Batch that
	// a state db implementation is expected to ues as a save point
	ApplyUpdates(batch *UpdateBatch, height *version.Height) error
	// GetLatestSavePoint returns the height of the highest transaction upto which
	// the state db is consistent
	GetLatestSavePoint() (*version.Height, error)
	// ValidateKeyValue tests whether the key and value is supported by the db implementation.
	// For instance, leveldb supports any bytes for the key while the couchdb supports only valid utf-8 string
	// TODO make the function ValidateKeyValue return a specific error say ErrInvalidKeyValue
	// However, as of now, the both implementations of this function (leveldb and couchdb) are deterministic in returing an error
	// i.e., an error is returned only if the key-value are found to be invalid for the underlying db
	ValidateKeyValue(key string, value []byte) error
	// BytesKeySupported returns true if the implementation (underlying db) supports the any bytes to be used as key.
	// For instance, leveldb supports any bytes for the key while the couchdb supports only valid utf-8 string
	BytesKeySupported() bool
	// Open opens the db
	Open() error
	// Close closes the db
	Close()
}

//BulkOptimizable interface provides additional functions for
//databases capable of batch operations
type BulkOptimizable interface {
	LoadCommittedVersions(keys []*CompositeKey) error
	GetCachedVersion(namespace, key string) (*version.Height, bool)
	ClearCachedVersions()
}

//IndexCapable interface provides additional functions for
//databases capable of index operations
type IndexCapable interface {
	GetDBType() string
	ProcessIndexesForChaincodeDeploy(namespace string, fileEntries []*ccprovider.TarFileEntry) error
}

// CompositeKey encloses Namespace and Key components
type CompositeKey struct {
	Namespace string
	Key       string
}

// VersionedValue encloses value and corresponding version
type VersionedValue struct {
	Value    []byte
	Metadata []byte
	Version  *version.Height
}

// IsDelete returns true if this update indicates delete of a key
func (vv *VersionedValue) IsDelete() bool {
	return vv.Value == nil
}

// VersionedKV encloses key and corresponding VersionedValue
type VersionedKV struct {
	CompositeKey
	VersionedValue
}

// ResultsIterator iterates over query results
type ResultsIterator interface {
	Next() (QueryResult, error)
	Close()
}

// QueryResultsIterator adds GetBookmarkAndClose method
type QueryResultsIterator interface {
	ResultsIterator
	GetBookmarkAndClose() string
}

// QueryResult - a general interface for supporting different types of query results. Actual types differ for different queries
type QueryResult interface{}

type nsUpdates struct {
	m map[string]*VersionedValue
}

func newNsUpdates() *nsUpdates {
	return &nsUpdates{make(map[string]*VersionedValue)}
}

// UpdateBatch encloses the details of multiple `updates`
type UpdateBatch struct {
	updates map[string]*nsUpdates
}

// NewUpdateBatch constructs an instance of a Batch
func NewUpdateBatch() *UpdateBatch {
	return &UpdateBatch{make(map[string]*nsUpdates)}
}

// Get returns the VersionedValue for the given namespace and key
func (batch *UpdateBatch) Get(ns string, key string) *VersionedValue {
	nsUpdates, ok := batch.updates[ns]
	if !ok {
		return nil
	}
	vv, ok := nsUpdates.m[key]
	if !ok {
		return nil
	}
	return vv
}

// Put adds a key with value only. The metadata is assumed to be nil
func (batch *UpdateBatch) Put(ns string, key string, value []byte, version *version.Height) {
	batch.PutValAndMetadata(ns, key, value, nil, version)
}

// PutValAndMetadata adds a key with value and metadata
// TODO introducing a new function to limit the refactoring. Later in a separate CR, the 'Put' function above should be removed
func (batch *UpdateBatch) PutValAndMetadata(ns string, key string, value []byte, metadata []byte, version *version.Height) {
	if value == nil {
		panic("Nil value not allowed. Instead call 'Delete' function")
	}
	batch.Update(ns, key, &VersionedValue{value, metadata, version})
}

// Delete deletes a Key and associated value
func (batch *UpdateBatch) Delete(ns string, key string, version *version.Height) {
	batch.Update(ns, key, &VersionedValue{nil, nil, version})
}

// Exists checks whether the given key exists in the batch
func (batch *UpdateBatch) Exists(ns string, key string) bool {
	nsUpdates, ok := batch.updates[ns]
	if !ok {
		return false
	}
	_, ok = nsUpdates.m[key]
	return ok
}

// GetUpdatedNamespaces returns the names of the namespaces that are updated
func (batch *UpdateBatch) GetUpdatedNamespaces() []string {
	namespaces := make([]string, len(batch.updates))
	i := 0
	for ns := range batch.updates {
		namespaces[i] = ns
		i++
	}
	return namespaces
}

// Update updates the batch with a latest entry for a namespace and a key
func (batch *UpdateBatch) Update(ns string, key string, vv *VersionedValue) {
	batch.getOrCreateNsUpdates(ns).m[key] = vv
}

// GetUpdates returns all the updates for a namespace
func (batch *UpdateBatch) GetUpdates(ns string) map[string]*VersionedValue {
	nsUpdates, ok := batch.updates[ns]
	if !ok {
		return nil
	}
	return nsUpdates.m
}

// GetRangeScanIterator returns an iterator that iterates over keys of a specific namespace in sorted order
// In other word this gives the same functionality over the contents in the `UpdateBatch` as
// `VersionedDB.GetStateRangeScanIterator()` method gives over the contents in the statedb
// This function can be used for querying the contents in the updateBatch before they are committed to the statedb.
// For instance, a validator implementation can used this to verify the validity of a range query of a transaction
// where the UpdateBatch represents the union of the modifications performed by the preceding valid transactions in the same block
// (Assuming Group commit approach where we commit all the updates caused by a block together).
func (batch *UpdateBatch) GetRangeScanIterator(ns string, startKey string, endKey string) QueryResultsIterator {
	return newNsIterator(ns, startKey, endKey, batch)
}

func (batch *UpdateBatch) getOrCreateNsUpdates(ns string) *nsUpdates {
	nsUpdates := batch.updates[ns]
	if nsUpdates == nil {
		nsUpdates = newNsUpdates()
		batch.updates[ns] = nsUpdates
	}
	return nsUpdates
}

type nsIterator struct {
	ns         string
	nsUpdates  *nsUpdates
	sortedKeys []string
	nextIndex  int
	lastIndex  int
}

func newNsIterator(ns string, startKey string, endKey string, batch *UpdateBatch) *nsIterator {
	nsUpdates, ok := batch.updates[ns]
	if !ok {
		return &nsIterator{}
	}
	sortedKeys := util.GetSortedKeys(nsUpdates.m)
	var nextIndex int
	var lastIndex int
	if startKey == "" {
		nextIndex = 0
	} else {
		nextIndex = sort.SearchStrings(sortedKeys, startKey)
	}
	if endKey == "" {
		lastIndex = len(sortedKeys)
	} else {
		lastIndex = sort.SearchStrings(sortedKeys, endKey)
	}
	return &nsIterator{ns, nsUpdates, sortedKeys, nextIndex, lastIndex}
}

// Next gives next key and versioned value. It returns a nil when exhausted
func (itr *nsIterator) Next() (QueryResult, error) {
	if itr.nextIndex >= itr.lastIndex {
		return nil, nil
	}
	key := itr.sortedKeys[itr.nextIndex]
	vv := itr.nsUpdates.m[key]
	itr.nextIndex++
	return &VersionedKV{CompositeKey{itr.ns, key}, VersionedValue{vv.Value, vv.Metadata, vv.Version}}, nil
}

// Close implements the method from QueryResult interface
func (itr *nsIterator) Close() {
	// do nothing
}

// GetBookmarkAndClose implements the method from QueryResult interface
func (itr *nsIterator) GetBookmarkAndClose() string {
	// do nothing
	return ""
}

const optionLimit = "limit"

// ValidateRangeMetadata validates the JSON containing attributes for the range query
func ValidateRangeMetadata(metadata map[string]interface{}) error {
	for key, keyVal := range metadata {
		switch key {

		case optionLimit:
			//Verify the pageSize is an integer
			if _, ok := keyVal.(int32); ok {
				continue
			}
			return fmt.Errorf("Invalid entry, \"limit\" must be a int32")

		default:
			return fmt.Errorf("Invalid entry, option %s not recognized", key)
		}
	}
	return nil
}
