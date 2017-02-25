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
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	//"github.com/hyperledger/fabric/core/container"
	"archive/tar"
	"bytes"
	"compress/gzip"

	"github.com/hyperledger/fabric/core/container/util"
	pb "github.com/hyperledger/fabric/protos/peer"
)

var lccctestpath = "/tmp/lccctest"

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

func constructDeploymentSpec(name string, path string, version string, initArgs [][]byte, createFS bool) (*pb.ChaincodeDeploymentSpec, error) {
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: name, Path: path, Version: version}, Input: &pb.ChaincodeInput{Args: initArgs}}

	codePackageBytes := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(codePackageBytes)
	tw := tar.NewWriter(gz)

	err := util.WriteBytesToPackage("src/garbage.go", []byte(name+path+version), tw)
	if err != nil {
		return nil, err
	}

	tw.Close()
	gz.Close()

	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes.Bytes()}

	if createFS {
		err := ccprovider.PutChaincodeIntoFS(chaincodeDeploymentSpec)
		if err != nil {
			return nil, err
		}
	}

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

	ccname := "example02"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	version := "0"
	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lccctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCHAINCODES)}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}
	// deployed one chaincode so query should return an array with one chaincode
	if len(cqr.GetChaincodes()) != 1 {
		t.FailNow()
	}

	// check that the ChaincodeInfo values match the input values
	if cqr.GetChaincodes()[0].Name != ccname || cqr.GetChaincodes()[0].Version != version || cqr.GetChaincodes()[0].Path != path {
		t.FailNow()
	}
}

//TestInstall tests the install function
func TestInstall(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
	ccname := "example02"
	path := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	version := "0"
	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//constructDeploymentSpec puts the depspec on the FS. This should succeed
	args := [][]byte{[]byte(INSTALL), b}
	defer os.Remove(lccctestpath + "/example02.0")
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETINSTALLEDCHAINCODES)}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	// installed one chaincode so query should return an array with one chaincode
	if len(cqr.GetChaincodes()) != 1 {
		t.FailNow()
	}

	// check that the ChaincodeInfo values match the input values
	if cqr.GetChaincodes()[0].Name != ccname || cqr.GetChaincodes()[0].Version != version || cqr.GetChaincodes()[0].Path != path {
		t.FailNow()
	}
}

//TestReinstall tests the install function
func TestReinstall(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	//note that this puts the code on the filesyste....
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//constructDeploymentSpec puts the depspec on the FS. This should fail
	args := [][]byte{[]byte(INSTALL), b}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
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
	if res.Status == shim.OK {
		t.Logf("Expected failure")
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

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
	if err != nil {
		t.FailNow()
	}

	//change name to empty
	cds.ChaincodeSpec.ChaincodeId.Name = ""

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

//TestEmptyChaincodeVersion tests the deploy function without a version name
func TestEmptyChaincodeVersion(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
	if err != nil {
		t.FailNow()
	}

	//change version to empty
	cds.ChaincodeSpec.ChaincodeId.Version = ""

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	res := stub.MockInvoke("1", args)
	if string(res.Message) != EmptyVersionErr("example02").Error() {
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

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
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

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}
}

//TestMultipleDeploy tests deploying multiple chaincodeschaincodes
func TestMultipleDeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//deploy 01
	cds, err = constructDeploymentSpec("example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example01.0")
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCHAINCODES)}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	// deployed two chaincodes so query should return an array with two chaincodes
	if len(cqr.GetChaincodes()) != 2 {
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
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
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
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
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

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("Deploy chaincode error: %v", err)
	}

	newCds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "1", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.1")
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

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("Deploy chaincode error: %s", res.Message)
	}

	newCds, err := constructDeploymentSpec("example03", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "1", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	defer os.Remove(lccctestpath + "/example03.1")
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

//TestGetAPIsWithoutInstall get functions should return the right responses when chaicode is on
//ledger but not on FS
func TestGetAPIsWithoutInstall(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//Force remove CC
	os.Remove(lccctestpath + "/example02.0")

	//GETCCINFO should still work
	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//GETCCDATA should still work
	args = [][]byte{[]byte(GETCCDATA), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.FailNow()
	}

	//GETDEPSPEC should not work
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.FailNow()
	}

	// get instantiated chaincodes
	args = [][]byte{[]byte(GETCHAINCODES)}
	res := stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	// one chaincode instantiated so query should return an array with one
	// chaincode
	if len(cqr.GetChaincodes()) != 1 {
		t.FailNow()
	}

	// get installed chaincodes
	args = [][]byte{[]byte(GETINSTALLEDCHAINCODES)}
	res = stub.MockInvoke("1", args)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr = &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	// no chaincodes installed to FS so query should return an array with zero
	// chaincodes
	if len(cqr.GetChaincodes()) != 0 {
		t.FailNow()
	}

}

func TestMain(m *testing.M) {
	ccprovider.SetChaincodesPath(lccctestpath)
	sysccprovider.RegisterSystemChaincodeProviderFactory(&mocksccProviderFactory{})
	os.Exit(m.Run())
}
