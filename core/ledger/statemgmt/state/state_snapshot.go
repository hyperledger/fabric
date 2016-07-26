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

package state

import (
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/tecbot/gorocksdb"
)

// StateSnapshot encapsulates StateSnapshotIterator given by actual state implementation and the db snapshot
type StateSnapshot struct {
	blockNumber  uint64
	stateImplItr statemgmt.StateSnapshotIterator
	dbSnapshot   *gorocksdb.Snapshot
}

// newStateSnapshot creates a new snapshot of the global state for the current block.
func newStateSnapshot(blockNumber uint64, dbSnapshot *gorocksdb.Snapshot) (*StateSnapshot, error) {
	itr, err := stateImpl.GetStateSnapshotIterator(dbSnapshot)
	if err != nil {
		return nil, err
	}
	snapshot := &StateSnapshot{blockNumber, itr, dbSnapshot}
	return snapshot, nil
}

// Release the snapshot. This MUST be called when you are done with this resouce.
func (ss *StateSnapshot) Release() {
	ss.stateImplItr.Close()
	ss.dbSnapshot.Release()
}

// Next moves the iterator to the next key/value pair in the state
func (ss *StateSnapshot) Next() bool {
	return ss.stateImplItr.Next()
}

// GetRawKeyValue returns the raw bytes for the key and value at the current iterator position
func (ss *StateSnapshot) GetRawKeyValue() ([]byte, []byte) {
	return ss.stateImplItr.GetRawKeyValue()
}

// GetBlockNumber returns the blocknumber associated with this global state snapshot
func (ss *StateSnapshot) GetBlockNumber() uint64 {
	return ss.blockNumber
}
