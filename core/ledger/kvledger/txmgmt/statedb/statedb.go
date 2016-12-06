/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package statedb

import "github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"

// VersionedDBProvider provides an instance of an versioned DB
type VersionedDBProvider interface {
	GetDBHandle(id string) VersionedDB
}

// VersionedDB lists methods that a db is supposed to implement
type VersionedDB interface {
	// GetState gets the value for given namespace and key. For a chaincode, the namespace corresponds to the chaincodeId
	GetState(namespace string, key string) (*VersionedValue, error)
	// GetStateMultipleKeys gets the values for multiple keys in a single call
	GetStateMultipleKeys(namespace string, keys []string) ([]*VersionedValue, error)
	// GetStateRangeScanIterator returns an iterator that contains all the key-values between given key ranges.
	// The returned ResultsIterator contains results of type *VersionedKV
	GetStateRangeScanIterator(namespace string, startKey string, endKey string) (ResultsIterator, error)
	// ExecuteQuery executes the given query and returns an iterator that contains results of type *VersionedKV.
	ExecuteQuery(query string) (ResultsIterator, error)
	// ApplyUpdates applies the batch to the underlying db.
	// height is the height of the highest transaction in the Batch that
	// a state db implementation is expected to ues as a save point
	ApplyUpdates(batch *UpdateBatch, height *version.Height) error
	// GetLatestSavePoint returns the height of the highest transaction upto which
	// the state db is consistent
	GetLatestSavePoint() (*version.Height, error)
	// Open opens the db
	Open() error
	// Close closes the db
	Close()
}

// CompositeKey encloses Namespace and Key components
type CompositeKey struct {
	Namespace string
	Key       string
}

// VersionedValue encloses value and corresponding version
type VersionedValue struct {
	Value   []byte
	Version *version.Height
}

// VersionedKV encloses key and corresponding VersionedValue
type VersionedKV struct {
	CompositeKey
	VersionedValue
}

// ResultsIterator hepls in iterates over query results
type ResultsIterator interface {
	Next() (*VersionedKV, error)
	Close()
}

// UpdateBatch encloses the details of multiple `updates`
type UpdateBatch struct {
	KVs map[CompositeKey]*VersionedValue
}

// NewUpdateBatch constructs an instance of a Batch
func NewUpdateBatch() *UpdateBatch {
	return &UpdateBatch{make(map[CompositeKey]*VersionedValue)}
}

// Put adds a VersionedKV
func (batch *UpdateBatch) Put(ns string, key string, value []byte, version *version.Height) {
	if value == nil {
		panic("Nil value not allowed")
	}
	batch.KVs[CompositeKey{ns, key}] = &VersionedValue{value, version}
}

// Delete deletes a Key and associated value
func (batch *UpdateBatch) Delete(ns string, key string, version *version.Height) {
	batch.KVs[CompositeKey{ns, key}] = &VersionedValue{nil, version}
}

// Exists checks whether the given key exists in the batch
func (batch *UpdateBatch) Exists(ns string, key string) bool {
	_, ok := batch.KVs[CompositeKey{ns, key}]
	return ok
}
