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
package vscc

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos"
)

func TestInit(t *testing.T) {
	v := new(ValidatorOneValidSignature)
	stub := shim.NewMockStub("validatoronevalidsignature", v)

	if _, err := stub.MockInit("1", nil); err != nil {
		t.Fatalf("vscc init failed with %v", err)
	}
}

func TestInvoke(t *testing.T) {
	v := new(ValidatorOneValidSignature)
	stub := shim.NewMockStub("validatoronevalidsignature", v)

	// Failed path: Invalid arguments
	args := [][]byte{[]byte("dv")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("vscc invoke should have returned incorrect number of args: %v", args)
	}

	args = [][]byte{[]byte("dv"), []byte("tx")}
	args[1] = nil
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("vscc invoke should have returned no block to validate. Input args: %v", args)
	}

	// Successful path
	args = [][]byte{[]byte("dv"), mockBlock()}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("vscc invoke failed with: %v", err)
	}
}

func mockBlock() []byte {
	block := &pb.Block2{}
	payload, _ := proto.Marshal(block)
	return payload
}
