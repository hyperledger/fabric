/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package example03

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

func checkInit(t *testing.T, scc *SimpleChaincode, stub *shim.MockStub, args [][]byte) {
	res := stub.MockInit("1", args)
	if res.Status != shim.OK {
		fmt.Println("Init failed", res.Message)
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

func checkInvoke(t *testing.T, scc *SimpleChaincode, stub *shim.MockStub, args [][]byte) {
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		fmt.Println("Query failed", string(res.Message))
		t.FailNow()
	}
}

func TestExample03_Init(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex03", scc)

	// Init A=123 B=234
	checkInit(t, scc, stub, [][]byte{[]byte("init"), []byte("A"), []byte("123")})

	checkState(t, stub, "A", "123")
}

func TestExample03_Invoke(t *testing.T) {
	scc := new(SimpleChaincode)
	stub := shim.NewMockStub("ex03", scc)

	// Init A=345 B=456
	checkInit(t, scc, stub, [][]byte{[]byte("init"), []byte("A"), []byte("345")})

	// Invoke "query"
	checkInvoke(t, scc, stub, [][]byte{[]byte("query"), []byte("A"), []byte("345")})
}
