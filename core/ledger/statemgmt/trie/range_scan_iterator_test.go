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

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestRangeScanIterator(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrieTestWrapper := newStateTrieTestWrapper(t)
	stateTrie := stateTrieTestWrapper.stateTrie
	stateDelta := statemgmt.NewStateDelta()

	// insert keys
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)

	stateDelta.Set("chaincodeID2", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID2", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID2", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID2", "key4", []byte("value4"), nil)
	stateDelta.Set("chaincodeID2", "key5", []byte("value5"), nil)
	stateDelta.Set("chaincodeID2", "key6", []byte("value6"), nil)
	stateDelta.Set("chaincodeID2", "key7", []byte("value7"), nil)

	stateDelta.Set("chaincodeID3", "key1", []byte("value1"), nil)

	stateDelta.Set("chaincodeID4", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID4", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID4", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID4", "key4", []byte("value4"), nil)
	stateDelta.Set("chaincodeID4", "key5", []byte("value5"), nil)
	stateDelta.Set("chaincodeID4", "key6", []byte("value6"), nil)
	stateDelta.Set("chaincodeID4", "key7", []byte("value7"), nil)

	stateDelta.Set("chaincodeID5", "key1", []byte("value5"), nil)
	stateDelta.Set("chaincodeID6", "key1", []byte("value6"), nil)

	stateTrie.PrepareWorkingSet(stateDelta)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	// test range scan for chaincodeID2
	rangeScanItr, _ := stateTrie.GetRangeScanIterator("chaincodeID2", "key2", "key5")

	var results = make(map[string][]byte)
	for rangeScanItr.Next() {
		key, value := rangeScanItr.GetKeyValue()
		results[key] = value
	}
	t.Logf("Results = %s", results)
	testutil.AssertEquals(t, len(results), 4)
	testutil.AssertEquals(t, results["key2"], []byte("value2"))
	testutil.AssertEquals(t, results["key3"], []byte("value3"))
	testutil.AssertEquals(t, results["key4"], []byte("value4"))
	testutil.AssertEquals(t, results["key5"], []byte("value5"))
	rangeScanItr.Close()

	// test range scan for chaincodeID4
	rangeScanItr, _ = stateTrie.GetRangeScanIterator("chaincodeID2", "key3", "key6")
	results = make(map[string][]byte)
	for rangeScanItr.Next() {
		key, value := rangeScanItr.GetKeyValue()
		results[key] = value
	}
	t.Logf("Results = %s", results)
	testutil.AssertEquals(t, len(results), 4)
	testutil.AssertEquals(t, results["key3"], []byte("value3"))
	testutil.AssertEquals(t, results["key4"], []byte("value4"))
	testutil.AssertEquals(t, results["key5"], []byte("value5"))
	testutil.AssertEquals(t, results["key6"], []byte("value6"))
	rangeScanItr.Close()

	// test range scan for chaincodeID2 starting from first key
	rangeScanItr, _ = stateTrie.GetRangeScanIterator("chaincodeID2", "", "key5")
	results = make(map[string][]byte)
	for rangeScanItr.Next() {
		key, value := rangeScanItr.GetKeyValue()
		results[key] = value
	}
	t.Logf("Results = %s", results)
	testutil.AssertEquals(t, len(results), 5)
	testutil.AssertEquals(t, results["key1"], []byte("value1"))
	testutil.AssertEquals(t, results["key2"], []byte("value2"))
	testutil.AssertEquals(t, results["key3"], []byte("value3"))
	testutil.AssertEquals(t, results["key4"], []byte("value4"))
	testutil.AssertEquals(t, results["key5"], []byte("value5"))
	rangeScanItr.Close()

	// test range scan for all the keys in chaincodeID2 starting from first key
	rangeScanItr, _ = stateTrie.GetRangeScanIterator("chaincodeID2", "", "")
	results = make(map[string][]byte)
	for rangeScanItr.Next() {
		key, value := rangeScanItr.GetKeyValue()
		results[key] = value
	}
	t.Logf("Results = %s", results)
	testutil.AssertEquals(t, len(results), 7)
	testutil.AssertEquals(t, results["key1"], []byte("value1"))
	testutil.AssertEquals(t, results["key2"], []byte("value2"))
	testutil.AssertEquals(t, results["key3"], []byte("value3"))
	testutil.AssertEquals(t, results["key4"], []byte("value4"))
	testutil.AssertEquals(t, results["key5"], []byte("value5"))
	testutil.AssertEquals(t, results["key6"], []byte("value6"))
	testutil.AssertEquals(t, results["key7"], []byte("value7"))
	rangeScanItr.Close()
}
