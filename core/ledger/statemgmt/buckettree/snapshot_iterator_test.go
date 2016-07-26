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

package buckettree

import (
	"testing"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestStateSnapshotIterator(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateImplTestWrapper := newStateImplTestWrapper(t)
	stateDelta := statemgmt.NewStateDelta()

	// insert keys
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID2", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID3", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID4", "key4", []byte("value4"), nil)
	stateDelta.Set("chaincodeID5", "key5", []byte("value5"), nil)
	stateDelta.Set("chaincodeID6", "key6", []byte("value6"), nil)
	stateImplTestWrapper.prepareWorkingSet(stateDelta)
	stateImplTestWrapper.persistChangesAndResetInMemoryChanges()
	//check that the key is persisted
	testutil.AssertEquals(t, stateImplTestWrapper.get("chaincodeID5", "key5"), []byte("value5"))

	// take db snapeshot
	dbSnapshot := db.GetDBHandle().GetSnapshot()

	// delete keys
	stateDelta.Delete("chaincodeID1", "key1", nil)
	stateDelta.Delete("chaincodeID2", "key2", nil)
	stateDelta.Delete("chaincodeID3", "key3", nil)
	stateDelta.Delete("chaincodeID4", "key4", nil)
	stateDelta.Delete("chaincodeID5", "key5", nil)
	stateDelta.Delete("chaincodeID6", "key6", nil)
	stateImplTestWrapper.prepareWorkingSet(stateDelta)
	stateImplTestWrapper.persistChangesAndResetInMemoryChanges()
	//check that the key is deleted
	testutil.AssertNil(t, stateImplTestWrapper.get("chaincodeID5", "key5"))

	itr, err := newStateSnapshotIterator(dbSnapshot)
	testutil.AssertNoError(t, err, "Error while getting state snapeshot iterator")
	numKeys := 0
	for itr.Next() {
		key, value := itr.GetRawKeyValue()
		t.Logf("key=[%s], value=[%s]", string(key), string(value))
		numKeys++
	}
	testutil.AssertEquals(t, numKeys, 6)
}
