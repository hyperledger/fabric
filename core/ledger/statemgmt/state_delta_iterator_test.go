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
	"testing"
)

func TestStateDeltaIteratorTest(t *testing.T) {
	delta := NewStateDelta()
	delta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	delta.Set("chaincodeID1", "key2", []byte("value2"), nil)
	delta.Set("chaincodeID1", "key3", []byte("value3"), nil)
	delta.Set("chaincodeID1", "key4", []byte("value4"), nil)
	delta.Set("chaincodeID1", "key5", []byte("value5"), nil)
	delta.Set("chaincodeID1", "key6", []byte("value6"), nil)
	delta.Delete("chaincodeID1", "key3", nil)

	delta.Set("chaincodeID2", "key7", []byte("value7"), nil)
	delta.Set("chaincodeID2", "key8", []byte("value8"), nil)

	// test a range
	itr := NewStateDeltaRangeScanIterator(delta, "chaincodeID1", "key2", "key5")
	AssertIteratorContains(t, itr,
		map[string][]byte{
			"key2": []byte("value2"),
			"key4": []byte("value4"),
			"key5": []byte("value5"),
		})

	// test with empty start key
	itr = NewStateDeltaRangeScanIterator(delta, "chaincodeID1", "", "key5")
	AssertIteratorContains(t, itr,
		map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key4": []byte("value4"),
			"key5": []byte("value5"),
		})

	// test with empty end key
	itr = NewStateDeltaRangeScanIterator(delta, "chaincodeID1", "", "")
	AssertIteratorContains(t, itr,
		map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
			"key4": []byte("value4"),
			"key5": []byte("value5"),
			"key6": []byte("value6"),
		})
}
