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

// this is the response to any successful Invoke() on chaincode_example04
var eventResponse = "{\"Name\":\"Event\",\"Amount\":\"1\"}"

func checkInit(t *testing.T, stub *shim.MockStub, args [][]byte) {
	res := stub.MockInit("1", args)
	if res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
}

func checkState(t *testing.T, stub *shim.MockStub, name string, value string) {
	bytes := stub.State[name]
	if bytes == nil {
		fmt.Println("State", name, "failed to get value")
		t.FailNow()
	}
	if string(bytes) != value {
		fmt.Println("State value", name, "was not", value, "as expected")
		t.FailNow()
	}
}

func checkQuery(t *testing.T, stub *shim.MockStub, name string, value string) {
	res := stub.MockInvoke("1", [][]byte{[]byte("query"), []byte(name)})
	if res.Status != shim.OK {
		fmt.Println("Query", name, "failed", string(res.Message))
		t.FailNow()
	}
	if res.Payload == nil {
		fmt.Println("Query", name, "failed to get value")
		t.FailNow()
	}
	if string(res.Payload) != value {
		fmt.Println("Query value", name, "was not", value, "as expected")
		t.FailNow()
	}
}

func checkInvoke(t *testing.T, stub *shim.MockStub, args [][]byte) {
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		fmt.Println("Invoke", args, "failed", string(res.Message))
		t.FailNow()
	}
}

func TestExample04_Init(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex04", scc)

	// Init A=123 B=234
	checkInit(t, stub, [][]byte{[]byte("init"), []byte("Event"), []byte("123")})

	checkState(t, stub, "Event", "123")
}

func TestExample04_Query(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex04", scc)

	// Init A=345 B=456
	checkInit(t, stub, [][]byte{[]byte("init"), []byte("Event"), []byte("1")})

	// Query A
	checkQuery(t, stub, "Event", eventResponse)
}

func TestExample04_Invoke(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex04", scc)

	chaincodeToInvoke := "ex02"

	ccEx2 := new(ex02.SimpleChaincode)
	stubEx2 := shim.NewMockStub(chaincodeToInvoke, ccEx2)
	checkInit(t, stubEx2, [][]byte{[]byte("init"), []byte("a"), []byte("111"), []byte("b"), []byte("222")})
	stub.MockPeerChaincode(chaincodeToInvoke, stubEx2)

	// Init A=567 B=678
	checkInit(t, stub, [][]byte{[]byte("init"), []byte("Event"), []byte("1")})

	// Invoke A->B for 10 via Example04's chaincode
	checkInvoke(t, stub, [][]byte{[]byte("invoke"), []byte(chaincodeToInvoke), []byte("Event"), []byte("1")})
	checkQuery(t, stub, "Event", eventResponse)
	checkQuery(t, stubEx2, "a", "101")
	checkQuery(t, stubEx2, "b", "232")

	// Invoke A->B for 10 via Example04's chaincode
	checkInvoke(t, stub, [][]byte{[]byte("invoke"), []byte(chaincodeToInvoke), []byte("Event"), []byte("1")})
	checkQuery(t, stub, "Event", eventResponse)
	checkQuery(t, stubEx2, "a", "91")
	checkQuery(t, stubEx2, "b", "242")
}
