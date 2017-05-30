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

package container

import (
	"flag"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/testutil"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestMain(m *testing.M) {
	flag.BoolVar(&runTests, "run-controller-tests", true, "run tests")
	flag.Parse()
	testutil.SetupTestConfig()
	os.Exit(m.Run())
}

func TestVM_ListImages(t *testing.T) {
	vm, err := NewVM()
	if err != nil {
		t.Fail()
		t.Logf("Error getting VM: %s", err)
	}
	err = vm.ListImages(context.TODO())
	assert.NoError(t, err, "Error listing images")
}

func TestVM_BuildImage_ChaincodeLocal(t *testing.T) {
	vm, err := NewVM()
	if err != nil {
		t.Fail()
		t.Logf("Error getting VM: %s", err)
		return
	}
	// Build the spec
	chaincodePath := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
		ChaincodeId: &pb.ChaincodeID{Name: "ex01", Path: chaincodePath},
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")}}
	err = vm.BuildChaincodeContainer(spec)
	assert.NoError(t, err)
}

func TestVM_BuildImage_ChaincodeRemote(t *testing.T) {
	t.Skip("Works but needs user credentials. Not suitable for automated unit tests as is")
	vm, err := NewVM()
	if err != nil {
		t.Fail()
		t.Logf("Error getting VM: %s", err)
		return
	}
	// Build the spec
	chaincodePath := "https://github.com/prjayach/chaincode_examples/chaincode_example02"
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
		ChaincodeId: &pb.ChaincodeID{Name: "ex02", Path: chaincodePath},
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")}}
	err = vm.BuildChaincodeContainer(spec)
	assert.NoError(t, err)
}

func TestVM_GetChaincodePackageBytes(t *testing.T) {
	_, err := GetChaincodePackageBytes(nil)
	assert.Error(t, err,
		"GetChaincodePackageBytes did not return error when chaincode spec is nil")

	spec := &pb.ChaincodeSpec{ChaincodeId: nil}
	_, err = GetChaincodePackageBytes(spec)
	assert.Error(t, err, "Error expected when GetChaincodePackageBytes is called with nil chaincode ID")
	assert.Contains(t, err.Error(), "invalid chaincode spec")

	spec = &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
		ChaincodeId: nil,
		Input:       &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")}}
	_, err = GetChaincodePackageBytes(spec)
	assert.Error(t, err,
		"GetChaincodePackageBytes did not return error when chaincode ID is nil")
}

func TestVM_BuildChaincodeContainer(t *testing.T) {
	vm, err := NewVM()
	assert.NoError(t, err)
	err = vm.BuildChaincodeContainer(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error getting chaincode package bytes")
}

func TestVM_Chaincode_Compile(t *testing.T) {
	// vm, err := NewVM()
	// if err != nil {
	// 	t.Fail()
	// 	t.Logf("Error getting VM: %s", err)
	// 	return
	// }

	// if err := vm.BuildPeerContainer(); err != nil {
	// 	t.Fail()
	// 	t.Log(err)
	// }
	t.Skip("NOT IMPLEMENTED")
}
