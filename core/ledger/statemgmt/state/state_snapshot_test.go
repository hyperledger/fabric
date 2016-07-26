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
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestStateSnapshot(t *testing.T) {
	stateTestWrapper, state := createFreshDBAndConstructState(t)
	// insert keys
	state.TxBegin("txUuid")
	state.Set("chaincodeID1", "key1", []byte("value1"))
	state.Set("chaincodeID2", "key2", []byte("value2"))
	state.Set("chaincodeID3", "key3", []byte("value3"))
	state.Set("chaincodeID4", "key4", []byte("value4"))
	state.Set("chaincodeID5", "key5", []byte("value5"))
	state.Set("chaincodeID6", "key6", []byte("value6"))
	state.TxFinish("txUuid", true)

	stateTestWrapper.persistAndClearInMemoryChanges(0)
	testutil.AssertEquals(t, stateTestWrapper.get("chaincodeID5", "key5", true), []byte("value5"))

	// take db snapeshot
	stateSnapshot := stateTestWrapper.getSnapshot()

	// delete keys
	state.TxBegin("txUuid")
	state.Delete("chaincodeID1", "key1")
	state.Delete("chaincodeID2", "key2")
	state.Delete("chaincodeID3", "key3")
	state.Delete("chaincodeID4", "key4")
	state.Delete("chaincodeID5", "key5")
	state.Delete("chaincodeID6", "key6")
	state.TxFinish("txUuid", true)
	stateTestWrapper.persistAndClearInMemoryChanges(0)
	//check that the key is deleted
	testutil.AssertNil(t, stateTestWrapper.get("chaincodeID5", "key5", true))

	numKeys := 0
	for stateSnapshot.Next() {
		key, value := stateSnapshot.GetRawKeyValue()
		t.Logf("key=[%s], value=[%s]", string(key), string(value))
		numKeys++
	}
	testutil.AssertEquals(t, numKeys, 6)
}
