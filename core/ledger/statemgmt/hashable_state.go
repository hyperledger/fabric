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

package statemgmt

import (
	"github.com/tecbot/gorocksdb"
)

// HashableState - Interface that is be implemented by state management
// Different state management implementation can be effiecient for computing crypto-hash for
// state under different workload conditions.
type HashableState interface {

	// Initialize this gives a chance to initialize. For instance, state implementation can load some data from DB
	Initialize(configs map[string]interface{}) error

	// Get get the value from DB
	Get(chaincodeID string, key string) ([]byte, error)

	// PrepareWorkingSet passes a stateDelta that captures the changes that needs to be applied to the state
	PrepareWorkingSet(stateDelta *StateDelta) error

	// ComputeCryptoHash state implementation to compute crypto-hash of state
	// assuming the stateDelta (passed in PrepareWorkingSet method) is to be applied
	ComputeCryptoHash() ([]byte, error)

	// AddChangesForPersistence state implementation to add all the key-value pair that it needs
	// to persist for committing the  stateDelta (passed in PrepareWorkingSet method) to DB.
	// In addition to the information in the StateDelta, the implementation may also want to
	// persist intermediate results for faster crypto-hash computation
	AddChangesForPersistence(writeBatch *gorocksdb.WriteBatch) error

	// ClearWorkingSet state implementation may clear any data structures that it may have constructed
	// for computing cryptoHash and persisting the changes for the stateDelta (passed in PrepareWorkingSet method)
	ClearWorkingSet(changesPersisted bool)

	// GetStateSnapshotIterator state implementation to provide an iterator that is supposed to give
	// All the key-value of global state. A particular implementation may need to remove additional information
	// that the implementation keeps for faster crypto-hash computation. For instance, filter a few of the
	// key-values or remove some data from particular key-values.
	GetStateSnapshotIterator(snapshot *gorocksdb.Snapshot) (StateSnapshotIterator, error)

	// GetRangeScanIterator - state implementation to provide an iterator that is supposed to give
	// All the key-values for a given chaincodeID such that a return key should be lexically greater than or
	// equal to startKey and less than or equal to endKey. If the value for startKey parameter is an empty string
	// startKey is assumed to be the smallest key available in the db for the chaincodeID. Similarly, an empty string
	// for endKey parameter assumes the endKey to be the greatest key available in the db for the chaincodeID
	GetRangeScanIterator(chaincodeID string, startKey string, endKey string) (RangeScanIterator, error)

	// PerfHintKeyChanged state implementation may be provided with some hints before (e.g., during tx execution)
	// the StateDelta is prepared and passed in PrepareWorkingSet method.
	// A state implementation may use this hint for prefetching relevant data so as if this could improve
	// the performance of ComputeCryptoHash method (when gets called at a later time)
	PerfHintKeyChanged(chaincodeID string, key string)
}

// StateSnapshotIterator An interface that is to be implemented by the return value of
// GetStateSnapshotIterator method in the implementation of HashableState interface
type StateSnapshotIterator interface {

	// Next moves to next key-value. Returns true if next key-value exists
	Next() bool

	// GetRawKeyValue returns next key-value
	GetRawKeyValue() ([]byte, []byte)

	// Close releases resources occupied by the iterator
	Close()
}

// RangeScanIterator - is to be implemented by the return value of
// GetRangeScanIterator method in the implementation of HashableState interface
type RangeScanIterator interface {

	// Next moves to next key-value. Returns true if next key-value exists
	Next() bool

	// GetKeyValue returns next key-value
	GetKeyValue() (string, []byte)

	// Close releases resources occupied by the iterator
	Close()
}
