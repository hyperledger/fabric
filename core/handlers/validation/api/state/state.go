/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
)

// State defines interaction with the world state
type State interface {
	// GetStateMultipleKeys gets the values for multiple keys in a single call
	GetStateMultipleKeys(namespace string, keys []string) ([][]byte, error)

	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// startKey is included in the results and endKey is excluded. An empty startKey refers to the first available key
	// and an empty endKey refers to the last available key. For scanning all the keys, both the startKey and the endKey
	// can be supplied as empty strings. However, a full scan should be used judiciously for performance reasons.
	// The returned ResultsIterator contains results of type *KV which is defined in fabric-protos/ledger/queryresult.
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)

	// GetStateMetadata returns the metadata for given namespace and key
	GetStateMetadata(namespace, key string) (map[string][]byte, error)

	// GetPrivateDataMetadataByHash gets the metadata of a private data item identified by a tuple <namespace, collection, keyhash>
	GetPrivateDataMetadataByHash(namespace, collection string, keyhash []byte) (map[string][]byte, error)

	// Done releases resources occupied by the State
	Done()
}

// StateFetcher retrieves an instance of a state
type StateFetcher interface {
	validation.Dependency

	// FetchState fetches state
	FetchState() (State, error)
}

// ResultsIterator - an iterator for query result set
type ResultsIterator interface {
	// Next returns the next item in the result set. The `QueryResult` is expected to be nil when
	// the iterator gets exhausted
	Next() (QueryResult, error)
	// Close releases resources occupied by the iterator
	Close()
}

// QueryResult - a general interface for supporting different types of query results. Actual types differ for different queries
type QueryResult interface{}
