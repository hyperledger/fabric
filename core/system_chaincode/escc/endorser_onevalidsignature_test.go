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
package escc

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos"
)

func TestInit(t *testing.T) {
	e := new(EndorserOneValidSignature)
	stub := shim.NewMockStub("endorseronevalidsignature", e)

	if _, err := stub.MockInit("1", nil); err != nil {
		fmt.Println("Init failed", err)
		t.FailNow()
	}
}

func TestInvoke(t *testing.T) {
	e := new(EndorserOneValidSignature)
	stub := shim.NewMockStub("endorseronevalidsignature", e)

	// Failed path: Not enough parameters
	args := [][]byte{[]byte("test")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("escc invoke should have failed with invalid number of args: %v", args)
	}

	// Failed path: action struct is null
	args = [][]byte{[]byte("test"), []byte("action"), []byte("proposal")}
	args[1] = nil
	if _, err := stub.MockInvoke("1", args); err == nil {
		fmt.Println("Invoke", args, "failed", err)
		t.Fatalf("escc invoke should have failed with Action object is null.  args: %v", args)
	}

	// Successful path
	args = [][]byte{[]byte("dv"), mockAction(), mockProposal()}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("escc invoke failed with: %v", err)
	}
	// TODO: Check sig here when we actually sign
}

func mockAction() []byte {
	action := &pb.Action{}
	action.ProposalHash = []byte("123")
	action.SimulationResult = []byte("read-write set")
	payload, _ := proto.Marshal(action)
	return payload
}
func mockProposal() []byte {
	return []byte("proposal")
}
