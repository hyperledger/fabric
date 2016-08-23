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
)

func checkInit(t *testing.T, scc *SimpleChaincode, stub *shim.MockStub, args []string) {
	_, err := stub.MockInit("1", "init", args)
	if err != nil {
		fmt.Println("Init failed", err)
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

func checkQuery(t *testing.T, scc *SimpleChaincode, stub *shim.MockStub, args []string, value string) {
	bytes, err := scc.Query(stub, "query", args)
	if err != nil {
		// expected failure
		fmt.Println("Query below is expected to fail")
		fmt.Println("Query failed", err)
		fmt.Println("Query above is expected to fail")

		if err.Error() != "{\"Error\":\"Cannot put state within chaincode query\"}" {
			fmt.Println("Failure was not the expected \"Cannot put state within chaincode query\" : ", err)
			t.FailNow()
		}

	} else {
		fmt.Println("Query did not fail as expected (PutState within Query)!", bytes, err)
		t.FailNow()
	}
}

func checkInvoke(t *testing.T, scc *SimpleChaincode, stub *shim.MockStub, args []string) {
	_, err := stub.MockInvoke("1", "query", args)
	if err != nil {
		fmt.Println("Invoke", args, "failed", err)
		t.FailNow()
	}
}

func TestExample03_Init(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex03", scc)

	// Init A=123 B=234
	checkInit(t, scc, stub, []string{"A", "123"})

	checkState(t, stub, "A", "123")
}

func TestExample03_Query(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex03", scc)

	// Init A=345 B=456
	checkInit(t, scc, stub, []string{"A", "345"})

	// Query A
	checkQuery(t, scc, stub, []string{"A", "345"}, "345")
}

func TestExample03_Invoke(t *testing.T) {
	// Example03 does not implement Invoke()
}
