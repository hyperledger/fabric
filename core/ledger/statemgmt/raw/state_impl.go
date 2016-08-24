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

package raw

import (
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/tecbot/gorocksdb"
)

// StateImpl implements raw state management. This implementation does not support computation of crypto-hash of the state.
// It simply stores the compositeKey and value in the db
type StateImpl struct {
	stateDelta *statemgmt.StateDelta
}

// NewStateImpl constructs new instance of raw state
func NewStateImpl() *StateImpl {
	return &StateImpl{}
}

// Initialize - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) Initialize(configs map[string]interface{}) error {
	return nil
}

// Get - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) Get(chaincodeID string, key string) ([]byte, error) {
	compositeKey := statemgmt.ConstructCompositeKey(chaincodeID, key)
	openchainDB := db.GetDBHandle()
	return openchainDB.GetFromStateCF(compositeKey)
}

// PrepareWorkingSet - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) PrepareWorkingSet(stateDelta *statemgmt.StateDelta) error {
	impl.stateDelta = stateDelta
	return nil
}

// ClearWorkingSet - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) ClearWorkingSet(changesPersisted bool) {
	impl.stateDelta = nil
}

// ComputeCryptoHash - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) ComputeCryptoHash() ([]byte, error) {
	return nil, nil
}

// AddChangesForPersistence - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) AddChangesForPersistence(writeBatch *gorocksdb.WriteBatch) error {
	delta := impl.stateDelta
	if delta == nil {
		return nil
	}
	openchainDB := db.GetDBHandle()
	updatedChaincodeIds := delta.GetUpdatedChaincodeIds(false)
	for _, updatedChaincodeID := range updatedChaincodeIds {
		updates := delta.GetUpdates(updatedChaincodeID)
		for updatedKey, value := range updates {
			compositeKey := statemgmt.ConstructCompositeKey(updatedChaincodeID, updatedKey)
			if value.IsDeleted() {
				writeBatch.DeleteCF(openchainDB.StateCF, compositeKey)
			} else {
				writeBatch.PutCF(openchainDB.StateCF, compositeKey, value.GetValue())
			}
		}
	}
	return nil
}

// PerfHintKeyChanged - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) PerfHintKeyChanged(chaincodeID string, key string) {
}

// GetStateSnapshotIterator - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) GetStateSnapshotIterator(snapshot *gorocksdb.Snapshot) (statemgmt.StateSnapshotIterator, error) {
	panic("Not a full-fledged state implementation. Implemented only for measuring best-case performance benchmark")
}

// GetRangeScanIterator - method implementation for interface 'statemgmt.HashableState'
func (impl *StateImpl) GetRangeScanIterator(chaincodeID string, startKey string, endKey string) (statemgmt.RangeScanIterator, error) {
	panic("Not a full-fledged state implementation. Implemented only for measuring best-case performance benchmark")
}
