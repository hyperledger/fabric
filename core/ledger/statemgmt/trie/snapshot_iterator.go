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

package trie

import (
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/tecbot/gorocksdb"
)

// StateSnapshotIterator implements the interface 'statemgmt.StateSnapshotIterator'
type StateSnapshotIterator struct {
	dbItr        *gorocksdb.Iterator
	currentKey   []byte
	currentValue []byte
}

func newStateSnapshotIterator(snapshot *gorocksdb.Snapshot) (*StateSnapshotIterator, error) {
	dbItr := db.GetDBHandle().GetStateCFSnapshotIterator(snapshot)
	dbItr.SeekToFirst()
	// skip the root key, because, the value test in Next method is misleading for root key as the value field
	dbItr.Next()
	return &StateSnapshotIterator{dbItr, nil, nil}, nil
}

// Next - see interface 'statemgmt.StateSnapshotIterator' for details
func (snapshotItr *StateSnapshotIterator) Next() bool {
	var available bool
	for ; snapshotItr.dbItr.Valid(); snapshotItr.dbItr.Next() {

		// making a copy of key-value bytes because, underlying key bytes are reused by itr.
		// no need to free slices as iterator frees memory when closed.
		trieKeyBytes := statemgmt.Copy(snapshotItr.dbItr.Key().Data())
		trieNodeBytes := statemgmt.Copy(snapshotItr.dbItr.Value().Data())
		value := unmarshalTrieNodeValue(trieNodeBytes)
		if value != nil {
			snapshotItr.currentKey = trieKeyEncoderImpl.decodeTrieKeyBytes(statemgmt.Copy(trieKeyBytes))
			snapshotItr.currentValue = value
			available = true
			snapshotItr.dbItr.Next()
			break
		}
	}
	return available
}

// GetRawKeyValue - see interface 'statemgmt.StateSnapshotIterator' for details
func (snapshotItr *StateSnapshotIterator) GetRawKeyValue() ([]byte, []byte) {
	return snapshotItr.currentKey, snapshotItr.currentValue
}

// Close - see interface 'statemgmt.StateSnapshotIterator' for details
func (snapshotItr *StateSnapshotIterator) Close() {
	snapshotItr.dbItr.Close()
}
