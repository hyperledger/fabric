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
	"testing"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestStateSnapshotIterator(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrieTestWrapper := newStateTrieTestWrapper(t)
	stateTrie := stateTrieTestWrapper.stateTrie
	stateDelta := statemgmt.NewStateDelta()

	// insert keys
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID2", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID3", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID4", "key4", []byte("value4"), nil)
	stateDelta.Set("chaincodeID5", "key5", []byte("value5"), nil)
	stateDelta.Set("chaincodeID6", "key6", []byte("value6"), nil)
	stateTrie.PrepareWorkingSet(stateDelta)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()
	//check that the key is persisted
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID1", "key1"), []byte("value1"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID2", "key2"), []byte("value2"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID3", "key3"), []byte("value3"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID4", "key4"), []byte("value4"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID5", "key5"), []byte("value5"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID6", "key6"), []byte("value6"))

	// take db snapeshot
	dbSnapshot := db.GetDBHandle().GetSnapshot()

	stateDelta1 := statemgmt.NewStateDelta()
	// delete a few keys
	stateDelta1.Delete("chaincodeID1", "key1", nil)
	stateDelta1.Delete("chaincodeID3", "key3", nil)
	stateDelta1.Delete("chaincodeID4", "key4", nil)
	stateDelta1.Delete("chaincodeID6", "key6", nil)

	// update remaining keys
	stateDelta1.Set("chaincodeID2", "key2", []byte("value2_new"), nil)
	stateDelta1.Set("chaincodeID5", "key5", []byte("value5_new"), nil)

	stateTrie.PrepareWorkingSet(stateDelta1)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()
	//check that the keys are updated
	testutil.AssertNil(t, stateTrieTestWrapper.Get("chaincodeID1", "key1"))
	testutil.AssertNil(t, stateTrieTestWrapper.Get("chaincodeID3", "key3"))
	testutil.AssertNil(t, stateTrieTestWrapper.Get("chaincodeID4", "key4"))
	testutil.AssertNil(t, stateTrieTestWrapper.Get("chaincodeID6", "key6"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID2", "key2"), []byte("value2_new"))
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID5", "key5"), []byte("value5_new"))

	itr, err := newStateSnapshotIterator(dbSnapshot)
	testutil.AssertNoError(t, err, "Error while getting state snapeshot iterator")

	stateDeltaFromSnapshot := statemgmt.NewStateDelta()
	for itr.Next() {
		keyBytes, valueBytes := itr.GetRawKeyValue()
		t.Logf("key=[%s], value=[%s]", string(keyBytes), string(valueBytes))
		chaincodeID, key := statemgmt.DecodeCompositeKey(keyBytes)
		stateDeltaFromSnapshot.Set(chaincodeID, key, valueBytes, nil)
	}
	testutil.AssertEquals(t, stateDelta, stateDeltaFromSnapshot)
}
