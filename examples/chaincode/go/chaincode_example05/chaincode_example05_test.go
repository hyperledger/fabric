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
package main

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	ex02 "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
)

// chaincode_example02's hash is used here and must be updated if the example is changed
var example02Url = "github.com/hyperledger/fabric/core/example/chaincode/chaincode_example02"

// chaincode_example05 looks like it wanted to return a JSON response to Query()
// it doesn't actually do this though, it just returns the sum value
func jsonResponse(name string, value string) string {
	return fmt.Sprintf("jsonResponse = \"{\"Name\":\"%v\",\"Value\":\"%v\"}", name, value)
}

func checkInit(t *testing.T, stub *shim.MockStub, args []string) {
	_, err := stub.MockInit("1", "init", args)
	if err != nil {
		fmt.Println("Init failed", err)
		t.FailNow()
	}
}

func checkState(t *testing.T, stub *shim.MockStub, name string, expect string) {
	bytes := stub.State[name]
	if bytes == nil {
		fmt.Println("State", name, "failed to get value")
		t.FailNow()
	}
	if string(bytes) != expect {
		fmt.Println("State value", name, "was not", expect, "as expected")
		t.FailNow()
	}
}

func checkQuery(t *testing.T, stub *shim.MockStub, args []string, expect string) {
	bytes, err := stub.MockQuery("query", args)
	if err != nil {
		fmt.Println("Query", args, "failed", err)
		t.FailNow()
	}
	if bytes == nil {
		fmt.Println("Query", args, "failed to get result")
		t.FailNow()
	}
	if string(bytes) != expect {
		fmt.Println("Query result ", string(bytes), "was not", expect, "as expected")
		t.FailNow()
	}
}

func checkInvoke(t *testing.T, stub *shim.MockStub, args []string) {
	_, err := stub.MockInvoke("1", "query", args)
	if err != nil {
		fmt.Println("Invoke", args, "failed", err)
		t.FailNow()
	}
}

func TestExample04_Init(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex05", scc)

	// Init A=123 B=234
	checkInit(t, stub, []string{"sumStoreName", "432"})

	checkState(t, stub, "sumStoreName", "432")
}

func TestExample04_Query(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex05", scc)

	ccEx2 := new(ex02.SimpleChaincode)
	stubEx2 := shim.NewMockStub("ex02", ccEx2)
	checkInit(t, stubEx2, []string{"a", "111", "b", "222"})
	stub.MockPeerChaincode(example02Url, stubEx2)

	checkInit(t, stub, []string{"sumStoreName", "0"})

	// a + b = 111 + 222 = 333
	checkQuery(t, stub, []string{example02Url, "sumStoreName"}, "333") // example05 doesn't return JSON?
}

func TestExample04_Invoke(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex05", scc)

	ccEx2 := new(ex02.SimpleChaincode)
	stubEx2 := shim.NewMockStub("ex02", ccEx2)
	checkInit(t, stubEx2, []string{"a", "222", "b", "333"})
	stub.MockPeerChaincode(example02Url, stubEx2)

	checkInit(t, stub, []string{"sumStoreName", "0"})

	// a + b = 222 + 333 = 555
	checkInvoke(t, stub, []string{example02Url, "sumStoreName"})
	checkQuery(t, stub, []string{example02Url, "sumStoreName"}, "555") // example05 doesn't return JSON?
	checkQuery(t, stubEx2, []string{"a"}, "222")
	checkQuery(t, stubEx2, []string{"b"}, "333")

	// update A-=10 and B+=10
	checkInvoke(t, stubEx2, []string{"a", "b", "10"})

	// a + b = 212 + 343 = 555
	checkInvoke(t, stub, []string{example02Url, "sumStoreName"})
	checkQuery(t, stub, []string{example02Url, "sumStoreName"}, "555") // example05 doesn't return JSON?
	checkQuery(t, stubEx2, []string{"a"}, "212")
	checkQuery(t, stubEx2, []string{"b"}, "343")
}
