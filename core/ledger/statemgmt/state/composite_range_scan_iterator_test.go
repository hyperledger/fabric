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

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
)

func TestCompositeRangeScanIterator(t *testing.T) {
	stateTestWrapper, state := createFreshDBAndConstructState(t)

	// commit initial test state to db
	state.TxBegin("txUuid")
	state.Set("chaincode1", "key1", []byte("value1"))
	state.Set("chaincode1", "key2", []byte("value2"))
	state.Set("chaincode1", "key3", []byte("value3"))
	state.Set("chaincode1", "key4", []byte("value4"))
	state.Set("chaincode1", "key5", []byte("value5"))
	state.Set("chaincode1", "key6", []byte("value6"))
	state.Set("chaincode1", "key7", []byte("value7"))

	state.Set("chaincode2", "key1", []byte("value1"))
	state.Set("chaincode2", "key2", []byte("value2"))
	state.Set("chaincode2", "key3", []byte("value3"))
	state.TxFinish("txUuid", true)
	stateTestWrapper.persistAndClearInMemoryChanges(0)

	// change and delete a few existing keys and add a new key
	state.TxBegin("txUUID")
	state.Set("chaincode1", "key3", []byte("value3_new"))
	state.Set("chaincode1", "key4", []byte("value4_new"))
	state.Set("chaincode1", "key5", []byte("value5_new"))
	state.Delete("chaincode1", "key6")
	state.Set("chaincode1", "key8", []byte("value8_new"))
	state.TxFinish("txUUID", true)

	// change and delete a few existing keys and add a new key, in the on-going tx
	state.TxBegin("txUUID")
	state.Set("chaincode1", "key3", []byte("value3_new_new"))
	state.Delete("chaincode1", "key4")
	state.Set("chaincode1", "key9", []byte("value9_new_new"))

	// Test with committed=false //////////////////////////
	/////////////////////////////////////////////////////
	// test keys between key2 and key8
	itr, _ := state.GetRangeScanIterator("chaincode1", "key2", "key8", false)
	statemgmt.AssertIteratorContains(t, itr,
		map[string][]byte{
			// from current tx
			"key3": []byte("value3_new_new"),

			// from current batch
			"key5": []byte("value5_new"),
			"key8": []byte("value8_new"),

			// from committed results
			"key2": []byte("value2"),
			"key7": []byte("value7"),
		})
	itr.Close()

	// test with an empty startKey
	itr, _ = state.GetRangeScanIterator("chaincode1", "", "key8", false)
	statemgmt.AssertIteratorContains(t, itr,
		map[string][]byte{
			// from current tx
			"key3": []byte("value3_new_new"),

			// from current batch
			"key5": []byte("value5_new"),
			"key8": []byte("value8_new"),

			// from committed results
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key7": []byte("value7"),
		})
	itr.Close()

	// test with an empty endKey
	itr, _ = state.GetRangeScanIterator("chaincode1", "", "", false)
	statemgmt.AssertIteratorContains(t, itr,
		map[string][]byte{
			// from current tx
			"key3": []byte("value3_new_new"),
			"key9": []byte("value9_new_new"),

			// from current batch
			"key5": []byte("value5_new"),
			"key8": []byte("value8_new"),

			// from committed results
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key7": []byte("value7"),
		})
	itr.Close()

	// Test with committed=true //////////////////////////
	/////////////////////////////////////////////////////
	// test keys between key2 and key8
	itr, _ = state.GetRangeScanIterator("chaincode1", "key2", "key8", true)
	statemgmt.AssertIteratorContains(t, itr,
		map[string][]byte{
			// from committed results
			"key2": []byte("value2"),
			"key3": []byte("value3"),
			"key4": []byte("value4"),
			"key5": []byte("value5"),
			"key6": []byte("value6"),
			"key7": []byte("value7"),
		})
	itr.Close()

	// test with an empty startKey
	itr, _ = state.GetRangeScanIterator("chaincode1", "", "key8", true)
	statemgmt.AssertIteratorContains(t, itr,
		map[string][]byte{
			// from committed results
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key3": []byte("value3"),
			"key4": []byte("value4"),
			"key5": []byte("value5"),
			"key6": []byte("value6"),
			"key7": []byte("value7"),
		})
	itr.Close()

	// test with an empty endKey
	itr, _ = state.GetRangeScanIterator("chaincode1", "", "", true)
	statemgmt.AssertIteratorContains(t, itr,
		map[string][]byte{
			// from committed results
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key3": []byte("value3"),
			"key4": []byte("value4"),
			"key5": []byte("value5"),
			"key6": []byte("value6"),
			"key7": []byte("value7"),
		})
	itr.Close()
}
