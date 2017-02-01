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
package lccc

import (
	"fmt"
	"testing"

	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/container"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type mocksccProviderFactory struct {
}

func (c *mocksccProviderFactory) NewSystemChaincodeProvider() sysccprovider.SystemChaincodeProvider {
	return &mocksccProviderImpl{}
}

type mocksccProviderImpl struct {
}

func (c *mocksccProviderImpl) IsSysCC(name string) bool {
	return true
}

func register(stub *shim.MockStub, ccname string) error {
	args := [][]byte{[]byte("register"), []byte(ccname)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		return fmt.Errorf(string(res.Message))
	}
	return nil
}

func constructDeploymentSpec(name string, path string, initArgs [][]byte) (*pb.ChaincodeDeploymentSpec, error) {
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: name, Path: path}, Input: &pb.ChaincodeInput{Args: initArgs}}
	codePackageBytes, err := container.GetChaincodePackageBytes(spec)
	if err != nil {
		return nil, err
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

//TestDeploy tests the deploy function (stops short of actually running the chaincode)
func TestDeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}
}

//TestInvalidCodeDeploy tests the deploy function with invalid code package
func TestInvalidCodeDeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	baddepspec := []byte("bad deploy spec")
	args := [][]byte{[]byte(DEPLOY), []byte("test"), baddepspec}
	res := stub.MockInvoke("1", args)
	expectErr := InvalidDeploymentSpecErr("unexpected EOF")
	if string(res.Message) != expectErr.Error() {
		t.Logf("get result: %+v", res)
		t.FailNow()
	}
}

//TestInvalidChaincodeName tests the deploy function with invalid chaincode name
func TestInvalidChaincodeName(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})

	//change name to empty
	cds.ChaincodeSpec.ChaincodeID.Name = ""

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	res := stub.MockInvoke("1", args)
	if string(res.Message) != InvalidChaincodeNameErr("").Error() {
		t.Logf("Get error: %s", res.Message)
		t.FailNow()
	}
}

//TestRedeploy tests the redeploying will fail function(and fail with "exists" error)
func TestRedeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//this should fail with exists error
	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	res := stub.MockInvoke("1", args)
	if string(res.Message) != ExistsErr("example02").Error() {
		t.FailNow()
	}
}

//TestCheckCC invokes the GETCCINFO function to get status of deployed chaincode
func TestCheckCC(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}
}

//TestMultipleDeploy tests deploying multiple chaincodes
func TestMultipleDeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//deploy 01
	cds, err = constructDeploymentSpec("example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}
}

//TestRetryFailedDeploy tests re-deploying after a failure
func TestRetryFailedDeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//send invalid chain name name that should fail
	args := [][]byte{[]byte(DEPLOY), []byte(""), b}
	res := stub.MockInvoke("1", args)
	if res.Status == shim.OK {
		//expected error but got success
		t.FailNow()
	}

	if string(res.Message) != InvalidChainNameErr("").Error() {
		//expected invalid chain name
		t.FailNow()
	}

	//deploy correctly now
	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//get the deploymentspec
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK || res.Payload == nil {
		t.FailNow()
	}
}

//TestUpgrade tests the upgrade function
func TestUpgrade(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("Deploy chaincode error: %v", err)
	}

	newCds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var newb []byte
	if newb, err = proto.Marshal(newCds); err != nil || newb == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args = [][]byte{[]byte(UPGRADE), []byte("test"), newb}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.Fatalf("Upgrade chaincode error: %v", err)
	}

	expectVer := "1"
	newVer := string(res.Payload)
	if newVer != expectVer {
		t.Fatalf("Upgrade chaincode version error, expected %s, got %s", expectVer, newVer)
	}
}

//TestUpgradeNonExistChaincode tests upgrade non exist chaincode
func TestUpgradeNonExistChaincode(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("Deploy chaincode error: %s", res.Message)
	}

	newCds, err := constructDeploymentSpec("example03", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var newb []byte
	if newb, err = proto.Marshal(newCds); err != nil || newb == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args = [][]byte{[]byte(UPGRADE), []byte("test"), newb}
	res := stub.MockInvoke("1", args)
	if string(res.Message) != NotFoundErr("test").Error() {
		t.FailNow()
	}
}

func TestMain(m *testing.M) {
	sysccprovider.RegisterSystemChaincodeProviderFactory(&mocksccProviderFactory{})
	os.Exit(m.Run())
}
