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

package test

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/testutil"
	pb "github.com/hyperledger/fabric/protos/peer"
)

func TestMain(m *testing.M) {
	testutil.SetupTestConfig()
	os.Exit(m.Run())
}

func TestCar_BuildImage(t *testing.T) {
	vm, err := container.NewVM()

	if err != nil {
		t.Errorf("Error getting VM: %s", err)
		return
	}
	// Build the spec
	cwd, err := os.Getwd()
	if err != nil {
		t.Errorf("Error getting CWD: %s", err)
		return
	}

	chaincodePath := cwd + "/org.hyperledger.chaincode.example02-0.1-SNAPSHOT.car"
	spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_CAR, ChaincodeId: &pb.ChaincodeID{Name: "cartest", Path: chaincodePath}, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs("f")}}
	if err := vm.BuildChaincodeContainer(spec); err != nil {
		t.Error(err)
	}
}
