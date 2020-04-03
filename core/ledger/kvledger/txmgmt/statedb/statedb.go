/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statedb

import (
	"github.com/hyperledger/fabric/core/ledger/internal/state"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
)

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
	GetState(namespace string, key string) (*state.VersionedValue, error)
	// GetVersion gets the version for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetVersion(namespace string, key string) (*version.Height, error)
	// GetStateMultipleKeys gets the values for multiple keys in a single call
	GetStateMultipleKeys(namespace string, keys []string) ([]*state.VersionedValue, error)
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// startKey is inclusive
	// endKey is exclusive
	// The returned ResultsIterator contains results of type *VersionedKV
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (state.ResultsIterator, error)
	// GetStateRangeScanIteratorWithMetadata returns an iterator that contains all the key-values between given key ranges.
	// startKey is inclusive
	// endKey is exclusive
	// options is a map of additional query parameters
	// The returned ResultsIterator contains results of type *VersionedKV
	GetStateRangeScanIteratorWithOptions(namespace string, startKey string, endKey string, options map[string]interface{}) (state.QueryResultsIterator, error)
	// ExecuteQuery executes the given query and returns an iterator that contains results of type *VersionedKV.
	ExecuteQuery(namespace, query string) (state.ResultsIterator, error)
	// ExecuteQueryWithMetadata executes the given query with associated query options and
	// returns an iterator that contains results of type *VersionedKV.
	// metadata is a map of additional query parameters
	ExecuteQueryWithMetadata(namespace, query string, metadata map[string]interface{}) (state.QueryResultsIterator, error)
	// ApplyUpdates applies the batch to the underlying db.
	// height is the height of the highest transaction in the Batch that
	// a state db implementation is expected to ues as a save point
	ApplyUpdates(batch *state.UpdateBatch, height *version.Height) error
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
	LoadCommittedVersions(keys []*state.CompositeKey) error
	GetCachedVersion(namespace, key string) (*version.Height, bool)
	ClearCachedVersions()
}

//IndexCapable interface provides additional functions for
//databases capable of index operations
type IndexCapable interface {
	GetDBType() string
	ProcessIndexesForChaincodeDeploy(namespace string, indexFilesData map[string][]byte) error
}
