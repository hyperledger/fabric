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

func TestStateChanges(t *testing.T) {
	stateTestWrapper, state := createFreshDBAndConstructState(t)
	// add keys
	state.TxBegin("txUuid")
	state.Set("chaincode1", "key1", []byte("value1"))
	state.Set("chaincode1", "key2", []byte("value2"))
	state.TxFinish("txUuid", true)
	//chehck in-memory
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", false), []byte("value1"))
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key1", true))

	delta := state.getStateDelta()
	// save to db
	stateTestWrapper.persistAndClearInMemoryChanges(0)
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", true), []byte("value1"))
	testutil.AssertEquals(t, stateTestWrapper.fetchStateDeltaFromDB(0), delta)

	// make changes when data is already in db
	state.TxBegin("txUuid")
	state.Set("chaincode1", "key1", []byte("new_value1"))
	state.TxFinish("txUuid", true)
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", false), []byte("new_value1"))

	state.TxBegin("txUuid")
	state.Delete("chaincode1", "key2")
	state.TxFinish("txUuid", true)
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key2", false))

	state.TxBegin("txUuid")
	state.Set("chaincode2", "key3", []byte("value3"))
	state.Set("chaincode2", "key4", []byte("value4"))
	state.TxFinish("txUuid", true)

	delta = state.getStateDelta()
	stateTestWrapper.persistAndClearInMemoryChanges(1)
	testutil.AssertEquals(t, stateTestWrapper.fetchStateDeltaFromDB(1), delta)

	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", true), []byte("new_value1"))
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key2", true))
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode2", "key3", true), []byte("value3"))
}

func TestStateTxBehavior(t *testing.T) {
	stateTestWrapper, state := createFreshDBAndConstructState(t)
	if state.txInProgress() {
		t.Fatalf("No tx should be reported to be in progress")
	}

	// set state in a successful tx
	state.TxBegin("txUuid")
	state.Set("chaincode1", "key1", []byte("value1"))
	state.Set("chaincode2", "key2", []byte("value2"))
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", false), []byte("value1"))
	state.TxFinish("txUuid", true)
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", false), []byte("value1"))

	// set state in a failed tx
	state.TxBegin("txUuid1")
	state.Set("chaincode1", "key1", []byte("value1_new"))
	state.Set("chaincode2", "key2", []byte("value2_new"))
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", false), []byte("value1_new"))
	state.TxFinish("txUuid1", false)
	//older state should be available
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", false), []byte("value1"))

	// delete state in a successful tx
	state.TxBegin("txUuid2")
	state.Delete("chaincode1", "key1")
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key1", false))
	state.TxFinish("txUuid2", true)
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key1", false))

	// // delete state in a failed tx
	state.TxBegin("txUuid2")
	state.Delete("chaincode2", "key2")
	testutil.AssertNil(t, stateTestWrapper.get("chaincode2", "key2", false))
	state.TxFinish("txUuid2", false)
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode2", "key2", false), []byte("value2"))
}

func TestStateTxWrongCallCausePanic_1(t *testing.T) {
	_, state := createFreshDBAndConstructState(t)
	defer testutil.AssertPanic(t, "A panic should occur when a set state is invoked with out calling a tx-begin")
	state.Set("chaincodeID1", "key1", []byte("value1"))
}

func TestStateTxWrongCallCausePanic_2(t *testing.T) {
	_, state := createFreshDBAndConstructState(t)
	defer testutil.AssertPanic(t, "A panic should occur when a tx-begin is invoked before tx-finish for on-going tx")
	state.TxBegin("txUuid")
	state.TxBegin("anotherUuid")
}

func TestStateTxWrongCallCausePanic_3(t *testing.T) {
	_, state := createFreshDBAndConstructState(t)
	defer testutil.AssertPanic(t, "A panic should occur when Uuid for tx-begin and tx-finish ends")
	state.TxBegin("txUuid")
	state.TxFinish("anotherUuid", true)
}

func TestDeleteState(t *testing.T) {

	stateTestWrapper, state := createFreshDBAndConstructState(t)

	// Add keys
	state.TxBegin("txUuid")
	state.Set("chaincode1", "key1", []byte("value1"))
	state.Set("chaincode1", "key2", []byte("value2"))
	state.TxFinish("txUuid", true)
	state.getStateDelta()
	stateTestWrapper.persistAndClearInMemoryChanges(0)

	// confirm keys are present
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", true), []byte("value1"))
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key2", true), []byte("value2"))

	// Delete the State
	err := state.DeleteState()
	if err != nil {
		t.Fatalf("Error deleting the state: %s", err)
	}

	// confirm the values are empty
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key1", false))
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key2", false))
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key1", true))
	testutil.AssertNil(t, stateTestWrapper.get("chaincode1", "key2", true))

	// Confirm that we can now store new stuff in the state
	state.TxBegin("txUuid")
	state.Set("chaincode1", "key1", []byte("value1"))
	state.Set("chaincode1", "key2", []byte("value2"))
	state.TxFinish("txUuid", true)
	state.getStateDelta()
	stateTestWrapper.persistAndClearInMemoryChanges(1)

	// confirm keys are present
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key1", true), []byte("value1"))
	testutil.AssertEquals(t, stateTestWrapper.get("chaincode1", "key2", true), []byte("value2"))
}

func TestStateDeltaSizeSetting(t *testing.T) {
	_, state := createFreshDBAndConstructState(t)
	if state.historyStateDeltaSize != 500 {
		t.Fatalf("Error reading historyStateDeltaSize. Expected 500, but got %d", state.historyStateDeltaSize)
	}
}
