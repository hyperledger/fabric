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
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestMockStateRangeQueryIterator(t *testing.T) {
	stub := NewMockStub("rangeTest", nil)
	stub.MockTransactionStart("init")
	stub.PutState("1", []byte{61})
	stub.PutState("0", []byte{62})
	stub.PutState("5", []byte{65})
	stub.PutState("3", []byte{63})
	stub.PutState("4", []byte{64})
	stub.PutState("6", []byte{66})
	stub.MockTransactionEnd("init")

	expectKeys := []string{"3", "4"}
	expectValues := [][]byte{{63}, {64}}

	rqi := NewMockStateRangeQueryIterator(stub, "2", "4")

	fmt.Println("Running loop")
	for i := 0; i < 2; i++ {
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

// TestSetChaincodeLoggingLevel uses the utlity function defined in chaincode.go to
// set the chaincodeLogger's logging level
func TestSetChaincodeLoggingLevel(t *testing.T) {
	// set log level to a non-default level
	testLogLevelString := "debug"
	viper.Set("logging.chaincode", testLogLevelString)

	SetChaincodeLoggingLevel()

	if !IsEnabledForLogLevel(testLogLevelString) {
		t.FailNow()
	}
}

type Marble struct {
	ObjectType string `json:"docType"` //docType is used to distinguish the various types of objects in state database
	Name       string `json:"name"`    //the fieldtags are needed to keep case from bouncing around
	Color      string `json:"color"`
	Size       int    `json:"size"`
	Owner      string `json:"owner"`
}

// JSONBytesEqual compares the JSON in two byte slices.
func jsonBytesEqual(expected []byte, actual []byte) bool {
	var infExpected, infActual interface{}
	if err := json.Unmarshal(expected, &infExpected); err != nil {
		return false
	}
	if err := json.Unmarshal(actual, &infActual); err != nil {
		return false
	}
	return reflect.DeepEqual(infActual, infExpected)
}

func TestPartialCompositeKeyQuery(t *testing.T) {
	stub := NewMockStub("PartialCompositeKeyQueryTest", nil)
	stub.MockTransactionStart("init")

	marble1 := &Marble{"marble", "set-1", "red", 5, "tom"}
	// Convert marble1 to JSON with Color and Name as composite key
	compositeKey1, _ := stub.CreateCompositeKey(marble1.ObjectType, []string{marble1.Name, marble1.Color})
	marbleJSONBytes1, _ := json.Marshal(marble1)
	// Add marble1 JSON to state
	stub.PutState(compositeKey1, marbleJSONBytes1)

	marble2 := &Marble{"marble", "set-1", "blue", 5, "jerry"}
	compositeKey2, _ := stub.CreateCompositeKey(marble2.ObjectType, []string{marble2.Name, marble2.Color})
	marbleJSONBytes2, _ := json.Marshal(marble2)
	stub.PutState(compositeKey2, marbleJSONBytes2)

	marble3 := &Marble{"marble", "set-2", "red", 5, "tom-jerry"}
	compositeKey3, _ := stub.CreateCompositeKey(marble3.ObjectType, []string{marble3.Name, marble3.Color})
	marbleJSONBytes3, _ := json.Marshal(marble3)
	stub.PutState(compositeKey3, marbleJSONBytes3)

	stub.MockTransactionEnd("init")
	expectKeys := []string{compositeKey1, compositeKey2}
	expectKeysAttributes := [][]string{{"set-1", "red"}, {"set-1", "blue"}}
	expectValues := [][]byte{marbleJSONBytes1, marbleJSONBytes2}

	rqi, _ := stub.PartialCompositeKeyQuery("marble", []string{"set-1"})
	fmt.Println("Running loop")
	for i := 0; i < 2; i++ {
		key, value, err := rqi.Next()
		fmt.Println("Loop", i, "got", key, value, err)
		if expectKeys[i] != key {
			fmt.Println("Expected key", expectKeys[i], "got", key)
			t.FailNow()
		}
		objectType, attributes, _ := stub.SplitCompositeKey(key)
		if objectType != "marble" {
			fmt.Println("Expected objectType", "marble", "got", objectType)
			t.FailNow()
		}
		fmt.Println(attributes)
		for index, attr := range attributes {
			if expectKeysAttributes[i][index] != attr {
				fmt.Println("Expected keys attribute", expectKeysAttributes[index][i], "got", attr)
				t.FailNow()
			}
		}
		if jsonBytesEqual(expectValues[i], value) != true {
			fmt.Println("Expected value", expectValues[i], "got", value)
			t.FailNow()
		}
	}
}
