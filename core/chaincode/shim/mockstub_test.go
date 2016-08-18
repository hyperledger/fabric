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

package shim

import (
	"fmt"
	"testing"
)

func TestMockStateRangeQueryIterator(t *testing.T) {
	stub := NewMockStub("rangeTest", nil)
	stub.MockTransactionStart("init")
	stub.PutState("1", []byte{61})
	stub.PutState("2", []byte{62})
	stub.PutState("5", []byte{65})
	stub.PutState("3", []byte{63})
	stub.PutState("4", []byte{64})
	stub.PutState("6", []byte{66})
	stub.MockTransactionEnd("init")

	expectKeys := []string{"2", "3", "4"}
	expectValues := [][]byte{{62}, {63}, {64}}

	rqi := NewMockStateRangeQueryIterator(stub, "2", "4")

	fmt.Println("Running loop")
	for i := 0; i < 3; i++ {
		key, value, err := rqi.Next()
		fmt.Println("Loop", i, "got", key, value, err)
		if expectKeys[i] != key {
			fmt.Println("Expected key", expectKeys[i], "got", key)
			t.FailNow()
		}
		if expectValues[i][0] != value[0] {
			fmt.Println("Expected value", expectValues[i], "got", value)
		}
	}
}
