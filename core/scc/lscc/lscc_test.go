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
package lscc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccpackage"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	cutil "github.com/hyperledger/fabric/core/container/util"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policy"
	policymocks "github.com/hyperledger/fabric/core/policy/mocks"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	putils "github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

var lscctestpath = "/tmp/lscctest"

func constructDeploymentSpec(name string, path string, version string, initArgs [][]byte, createFS bool) (*pb.ChaincodeDeploymentSpec, error) {
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: name, Path: path, Version: version}, Input: &pb.ChaincodeInput{Args: initArgs}}

	codePackageBytes := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(codePackageBytes)
	tw := tar.NewWriter(gz)

	err := cutil.WriteBytesToPackage("src/garbage.go", []byte(name+path+version), tw)
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

//TestInstall tests the install function with various inputs
func TestInstall(t *testing.T) {
	path := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	testInstall(t, "example02", "0", path, "", "Alice")
	testInstall(t, "example02-2", "1.0", path, "", "Alice")
	testInstall(t, "example02.go", "0", path, InvalidChaincodeNameErr("example02.go").Error(), "Alice")
	testInstall(t, "", "0", path, EmptyChaincodeNameErr("").Error(), "Alice")
	testInstall(t, "example02", "1{}0", path, InvalidVersionErr("1{}0").Error(), "Alice")
	testInstall(t, "example02", "0", path, "Authorization for INSTALL has been denied", "Bob")
}

func testInstall(t *testing.T, ccname string, version string, path string, expectedErrorMsg string, caller string) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	if err != nil {
		t.FailNow()
	}
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//constructDeploymentSpec puts the depspec on the FS. This should succeed
	args := [][]byte{[]byte(INSTALL), b}

	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte(caller), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	if expectedErrorMsg == "" {
		defer os.Remove(lscctestpath + "/" + ccname + "." + version)
		if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
			t.FailNow()
		}
	} else {
		if res := stub.MockInvokeWithSignedProposal("1", args, sProp); !strings.HasPrefix(string(res.Message), expectedErrorMsg) {
			t.Logf("Received error: [%s]", res.Message)
			t.FailNow()
		}
	}

	args = [][]byte{[]byte(GETINSTALLEDCHAINCODES)}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	if expectedErrorMsg == "" {
		// installed one chaincode so query should return an array with one chaincode
		if len(cqr.GetChaincodes()) != 1 {
			t.Logf("Expected 1 chaincode, found %d\n", len(cqr.GetChaincodes()))
			t.FailNow()
		}

		// check that the ChaincodeInfo values match the input values
		if cqr.GetChaincodes()[0].Name != ccname || cqr.GetChaincodes()[0].Version != version || cqr.GetChaincodes()[0].Path != path {
			t.FailNow()
		}
	} else {
		// we expected an error so no chaincodes should have installed
		if len(cqr.GetChaincodes()) != 0 {
			t.Logf("Expected 0 chaincodes, found %d\n", len(cqr.GetChaincodes()))
			t.FailNow()
		}
	}
}

//TestReinstall tests the install function
func TestReinstall(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	//note that this puts the code on the filesyste....
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/example02.0")
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
	stub := shim.NewMockStub("lscc", scc)

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

// TestDeploy tests the deploy function with various inputs for basic use cases
// (and stops short of actually running the chaincode). More advanced tests like
// redeploying, multiple deployments, and other failure cases that don't match
// this standard test case pattern are handled in other test cases below.
// Note: the forceBlankCCName and forceBlankVersion flags are necessary because
// constructDeploymentSpec() with the createFS flag set to true places the
// chaincode onto the filesystem to install it before it then attempts to
// instantiate the chaincode
// A default instantiation policy is used automatically because the cc package
// comes without a policy.
func TestDeploy(t *testing.T) {
	path := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	testDeploy(t, "example02", "0", path, false, false, "")
	testDeploy(t, "example02", "1.0", path, false, false, "")
	testDeploy(t, "example02", "0", path, true, false, EmptyChaincodeNameErr("").Error())
	testDeploy(t, "example02", "0", path, false, true, EmptyVersionErr("example02").Error())
	testDeploy(t, "example02.go", "0", path, false, false, InvalidChaincodeNameErr("example02.go").Error())
	testDeploy(t, "example02", "1{}0", path, false, false, InvalidVersionErr("1{}0").Error())
	testDeploy(t, "example02", "0", path, true, true, EmptyChaincodeNameErr("").Error())
}

func testDeploy(t *testing.T, ccname string, version string, path string, forceBlankCCName bool, forceBlankVersion bool, expectedErrorMsg string) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		t.Logf("Init failed: %s", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/" + ccname + "." + version)
	if forceBlankCCName {
		cds.ChaincodeSpec.ChaincodeId.Name = ""
	}
	if forceBlankVersion {
		cds.ChaincodeSpec.ChaincodeId.Version = ""
	}
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp2)

	if expectedErrorMsg == "" {
		if res.Status != shim.OK {
			t.FailNow()
		}
	} else {
		if string(res.Message) != expectedErrorMsg {
			t.Logf("Get error: %s", res.Message)
			t.FailNow()
		}
	}

	args = [][]byte{[]byte(GETCHAINCODES)}
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.FailNow()
	}

	cqr := &pb.ChaincodeQueryResponse{}
	err = proto.Unmarshal(res.Payload, cqr)
	if err != nil {
		t.FailNow()
	}

	if expectedErrorMsg == "" {
		// instantiated one chaincode so query should return an array with one chaincode
		if len(cqr.GetChaincodes()) != 1 {
			t.Logf("Expected 1 chaincode, found %d\n", len(cqr.GetChaincodes()))
			t.FailNow()
		}

		// check that the ChaincodeInfo values match the input values
		if cqr.GetChaincodes()[0].Name != ccname || cqr.GetChaincodes()[0].Version != version || cqr.GetChaincodes()[0].Path != path {
			t.FailNow()
		}

		args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
		if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
			t.FailNow()
		}
	} else {
		// instantiated zero chaincodes so query should return a zero-length array
		if len(cqr.GetChaincodes()) != 0 {
			t.Logf("Expected 0 chaincodes, found %d\n", len(cqr.GetChaincodes()))
			t.FailNow()
		}
	}
}

//TestRedeploy tests the redeploying will fail function(and fail with "exists" error)
func TestRedeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.FailNow()
	}

	//this should fail with exists error
	sProp, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	if string(res.Message) != ExistsErr("example02").Error() {
		t.FailNow()
	}
}

//TestMultipleDeploy tests deploying multiple chaincodeschaincodes
func TestMultipleDeploy(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		t.FailNow()
	}

	//deploy 01
	cds, err = constructDeploymentSpec("example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/example01.0")
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCHAINCODES)}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
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
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/example02.0")
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//send invalid chain name name that should fail
	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte(""), b}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp2)
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
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.FailNow()
	}

	//get the deploymentspec
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK || res.Payload == nil {
		t.FailNow()
	}
}

//TestTamperChaincode modifies the chaincode on the FS after deploy
func TestTamperChaincode(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	//deploy 01
	cds, err := constructDeploymentSpec("example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01", "0", [][]byte{[]byte("init"), []byte("a"), []byte("1"), []byte("b"), []byte("2")}, true)
	if err != nil {
		t.Logf("Could not construct example01.0 [%s]", err)
		t.FailNow()
	}

	defer os.Remove(lscctestpath + "/example01.0")

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.Logf("Could not construct example01.0")
		t.FailNow()
	}

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp2)
	if res.Status != shim.OK {
		t.Logf("Could not deploy example01.0")
		t.FailNow()
	}

	//deploy 02
	cds, err = constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}

	defer os.Remove(lscctestpath + "/example02.0")

	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//deploy correctly now
	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res = stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.Logf("Could not deploy example02.0")
		t.FailNow()
	}

	//remove the old file...
	os.Remove(lscctestpath + "/example02.0")

	//read 01 and ...
	if b, err = ioutil.ReadFile(lscctestpath + "/example01.0"); err != nil {
		t.Logf("Could not read back example01.0")
		t.FailNow()
	}

	//...brute force replace 02 with bytes from 01
	if err = ioutil.WriteFile(lscctestpath+"/example02.0", b, 0644); err != nil {
		t.Logf("Could not write to example02.0")
		t.FailNow()
	}

	//get the deploymentspec
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res = stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status == shim.OK {
		t.Logf("Expected error on tampering files but succeeded")
		t.FailNow()
	}

	//look specifically for Invalid error
	expectedErr := InvalidCCOnFSError("").Error()
	if strings.Index(res.Message, expectedErr) < 0 {
		t.Logf("Expected prefix %s on error but appeared to have got a different error : %+v", expectedErr, res)
		t.FailNow()
	}
}

//TestIPolDeployFail tests chaincode deploy with an instantiation default policy if the cc package comes without a policy
func TestIPolDeployDefaultFail(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		t.Fatalf("Init failed: %s", string(res.Message))
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			chainid: &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	assert.NoError(t, err)
	defer os.Remove(lscctestpath + "/example02.0")

	cdsbytes, err := proto.Marshal(cds)
	assert.NoError(t, err)

	// invoke deploy with a signed proposal that is not satisfied by the default policy
	args := [][]byte{[]byte(DEPLOY), []byte(chainid), cdsbytes}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status == shim.OK {
		t.Fatalf("Deploy must not succeed!")
	}
}

//TestIPolDeploy tests chaincode deploy with an instantiation policy
func TestIPolDeploy(t *testing.T) {
	// default test policy, this should succeed
	testIPolDeploy(t, "", true)
	// policy involving an unknown ORG, this should fail
	testIPolDeploy(t, "AND('ORG.admin')", false)
}

func testIPolDeploy(t *testing.T, iPol string, successExpected bool) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		t.Fatalf("Init failed [%s]", string(res.Message))
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			chainid: &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	// create deployment spec, don't write to disk, just marshal it to be used in a signed dep spec
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	assert.NoError(t, err)
	// create an instantiation policy
	var ip *common.SignaturePolicyEnvelope
	ip = cauthdsl.SignedByMspAdmin(mspid)
	if iPol != "" {
		ip, err = cauthdsl.FromString(iPol)
		if err != nil {
			t.Fatalf("Error creating instantiation policy %s: [%s]", iPol, err)
		}
	}
	// create signed dep spec
	cdsbytes, err := proto.Marshal(cds)
	assert.NoError(t, err)
	objToWrite, err := ccpackage.OwnerCreateSignedCCDepSpec(cds, ip, nil)
	assert.NoError(t, err)
	// write it to disk
	bytesToWrite, err := proto.Marshal(objToWrite)
	assert.NoError(t, err)
	fileToWrite := lscctestpath + "/example02.0"
	err = ioutil.WriteFile(fileToWrite, bytesToWrite, 0700)
	assert.NoError(t, err)
	defer os.Remove(lscctestpath + "/example02.0")

	// invoke deploy with a signed proposal that will be evaluated based on the policy
	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte(chainid), cdsbytes}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		if successExpected {
			t.Fatalf("Deploy failed %s", res)
		}
	}

	args = [][]byte{[]byte(GETCCINFO), []byte(chainid), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		if successExpected {
			t.Fatalf("GetCCInfo failed %s", res)
		}
	}
}

// TestUpgrade tests the upgrade function with various inputs for basic use cases
func TestUpgrade(t *testing.T) {
	path := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	testUpgrade(t, "example02", "0", "example02", "1", path, "")
	testUpgrade(t, "example02", "0", "example02", "", path, EmptyVersionErr("example02").Error())
	testUpgrade(t, "example02", "0", "example02", "0", path, IdenticalVersionErr("example02").Error())
	testUpgrade(t, "example02", "0", "example03", "1", path, NotFoundErr("test").Error())
	testUpgrade(t, "example02", "0", "example02", "1{}0", path, InvalidVersionErr("1{}0").Error())
	testUpgrade(t, "example02", "0", "example*02", "1{}0", path, InvalidChaincodeNameErr("example*02").Error())
	testUpgrade(t, "example02", "0", "", "1", path, EmptyChaincodeNameErr("").Error())
}

func testUpgrade(t *testing.T, ccname string, version string, newccname string, newversion string, path string, expectedErrorMsg string) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	cds, err := constructDeploymentSpec(ccname, path, version, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/" + ccname + "." + version)
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	sProp, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		t.Fatalf("Deploy chaincode error: %v", err)
	}

	var newCds *pb.ChaincodeDeploymentSpec
	// check to see if we've already created the deployment spec on the filesystem
	// in the above step for the upgrade version
	if ccname == newccname && version == newversion {
		newCds, err = constructDeploymentSpec(newccname, path, newversion, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	} else {
		newCds, err = constructDeploymentSpec(newccname, path, newversion, [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)
	}
	if err != nil {
		t.FailNow()
	}
	defer os.Remove(lscctestpath + "/" + newccname + "." + newversion)
	var newb []byte
	if newb, err = proto.Marshal(newCds); err != nil || newb == nil {
		t.Fatalf("Marshal DeploymentSpec failed")
	}

	args = [][]byte{[]byte(UPGRADE), []byte("test"), newb}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	if expectedErrorMsg == "" {
		if res.Status != shim.OK {
			t.Fatalf("Upgrade chaincode error: %v", err)
		}

		cd := &ccprovider.ChaincodeData{}
		if err = proto.Unmarshal(res.Payload, cd); err != nil {
			t.Fatalf("Upgrade chaincode could not unmarshal response")
		}

		newVer := cd.Version

		expectVer := "1"
		if newVer != expectVer {
			t.Fatalf("Upgrade chaincode version error, expected %s, got %s", expectVer, newVer)
		}
	} else {
		if string(res.Message) != expectedErrorMsg {
			t.Logf("Received error message: %s", res.Message)
			t.FailNow()
		}
	}
}

//TestIPolUpgrade tests chaincode deploy with an instantiation policy
func TestIPolUpgrade(t *testing.T) {
	// default policy, this should succeed
	testIPolUpgrade(t, "", true)
	// policy involving an unknown ORG, this should fail
	testIPolUpgrade(t, "AND('ORG.admin')", false)
}

func testIPolUpgrade(t *testing.T, iPol string, successExpected bool) {
	// deploy version 0 with a default instantiation policy, this should succeed in any case
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)
	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		t.Fatalf("Init failed %s", string(res.Message))
	}
	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			chainid: &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	// create deployment spec, don't write to disk, just marshal it to be used in a signed dep spec
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	assert.NoError(t, err)
	// create an instantiation policy
	ip := cauthdsl.SignedByMspAdmin(mspid)
	// create signed dep spec
	cdsbytes, err := proto.Marshal(cds)
	assert.NoError(t, err)
	objToWrite, err := ccpackage.OwnerCreateSignedCCDepSpec(cds, ip, nil)
	assert.NoError(t, err)
	// write it to disk
	bytesToWrite, err := proto.Marshal(objToWrite)
	assert.NoError(t, err)
	fileToWrite := lscctestpath + "/example02.0"
	err = ioutil.WriteFile(fileToWrite, bytesToWrite, 0700)
	assert.NoError(t, err)
	defer os.Remove(lscctestpath + "/example02.0")
	// invoke deploy with a signed proposal that will be evaluated based on the policy
	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	assert.NoError(t, err)
	args := [][]byte{[]byte(DEPLOY), []byte(chainid), cdsbytes}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.Fatalf("Deploy failed %s", res)
	}
	args = [][]byte{[]byte(GETCCINFO), []byte(chainid), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		t.Fatalf("GetCCInfo after deploy failed %s", res)
	}

	// here starts the interesting part for upgrade
	// create deployment spec
	cds, err = constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "1", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, false)
	assert.NoError(t, err)
	cdsbytes, err = proto.Marshal(cds)
	assert.NoError(t, err)
	// create the instantiation policy
	if iPol != "" {
		ip, err = cauthdsl.FromString(iPol)
		assert.NoError(t, err)
	}
	// create the signed ccpackage of the new version
	objToWrite, err = ccpackage.OwnerCreateSignedCCDepSpec(cds, ip, nil)
	assert.NoError(t, err)
	bytesToWrite, err = proto.Marshal(objToWrite)
	assert.NoError(t, err)
	fileToWrite = lscctestpath + "/example02.1"
	err = ioutil.WriteFile(fileToWrite, bytesToWrite, 0700)
	assert.NoError(t, err)
	defer os.Remove(lscctestpath + "/example02.1")

	// invoke upgrade with a signed proposal that will be evaluated based on the policy
	args = [][]byte{[]byte(UPGRADE), []byte(chainid), cdsbytes}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		if successExpected {
			t.Fatalf("Upgrade failed %s", res)
		}
	}
	args = [][]byte{[]byte(GETCCINFO), []byte(chainid), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		if successExpected {
			t.Fatalf("GetCCInfo failed")
		}
	}
}

//TestGetAPIsWithoutInstall get functions should return the right responses when chaicode is on
//ledger but not on FS
func TestGetAPIsWithoutInstall(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.FailNow()
	}

	//Force remove CC
	os.Remove(lscctestpath + "/example02.0")

	//GETCCINFO should still work
	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		t.FailNow()
	}

	//GETCCDATA should still work
	args = [][]byte{[]byte(GETCCDATA), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status != shim.OK {
		t.FailNow()
	}

	//GETDEPSPEC should not work
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp); res.Status == shim.OK {
		t.FailNow()
	}

	// get instantiated chaincodes
	args = [][]byte{[]byte(GETCHAINCODES)}
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
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
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
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

// TestGetInstalledChaincodesAccessRights verifies that only authorized parties can call
// the GETINSTALLEDCHAINCODES function
func TestGetInstalledChaincodesAccessRights(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Should pass
	args := [][]byte{[]byte(GETINSTALLEDCHAINCODES)}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.FailNow()
	}

	// Should fail
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status == shim.OK {
		t.FailNow()
	}
}

// TestGetChaincodesAccessRights verifies that only authorized parties can call
// the GETCHAINCODES function
func TestGetChaincodesAccessRights(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	// Should pass
	args := [][]byte{[]byte(GETCHAINCODES)}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.FailNow()
	}

	// Should fail
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status == shim.OK {
		t.FailNow()
	}
}

// TestGetCCInfoAccessRights verifies that only authorized parties can call
// the GETCCINFO function
func TestGetCCAccessRights(t *testing.T) {
	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lscc", scc)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}

	// Init the policy checker
	identityDeserializer := &policymocks.MockIdentityDeserializer{[]byte("Alice"), []byte("msg1")}
	policyManagerGetter := &policymocks.MockChannelPolicyManagerGetter{
		Managers: map[string]policies.Manager{
			"test": &policymocks.MockChannelPolicyManager{MockPolicy: &policymocks.MockPolicy{Deserializer: identityDeserializer}},
		},
	}
	scc.policyChecker = policy.NewPolicyChecker(
		policyManagerGetter,
		identityDeserializer,
		&policymocks.MockMSPPrincipalGetter{Principal: []byte("Alice")},
	)

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", "0", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}, true)

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	sProp2, _ := putils.MockSignedEndorserProposal2OrPanic(chainid, &pb.ChaincodeSpec{}, id)
	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if res := stub.MockInvokeWithSignedProposal("1", args, sProp2); res.Status != shim.OK {
		t.FailNow()
	}

	//Force remove CC
	defer os.Remove(lscctestpath + "/example02.0")

	// GETCCINFO
	// Should pass
	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	sProp, _ := utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res := stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.Logf("This should pass [%s]", res.Message)
		t.FailNow()
	}

	// Should fail
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status == shim.OK {
		t.Logf("This should fail [%s]", res.Message)
		t.FailNow()
	}

	// GETDEPSPEC
	// Should pass
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.Logf("This should pass [%s]", res.Message)
		t.FailNow()
	}

	// Should fail
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status == shim.OK {
		t.Logf("This should fail [%s]", res.Message)
		t.FailNow()
	}

	// GETCCDATA
	// Should pass
	args = [][]byte{[]byte(GETCCDATA), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeId.Name)}
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Alice"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status != shim.OK {
		t.Logf("This should pass [%s]", res.Message)
		t.FailNow()
	}

	// Should fail
	sProp, _ = utils.MockSignedEndorserProposalOrPanic("", &pb.ChaincodeSpec{}, []byte("Bob"), []byte("msg1"))
	identityDeserializer.Msg = sProp.ProposalBytes
	sProp.Signature = sProp.ProposalBytes
	res = stub.MockInvokeWithSignedProposal("1", args, sProp)
	if res.Status == shim.OK {
		t.Logf("This should fail [%s]", res.Message)
		t.FailNow()
	}
}

var id msp.SigningIdentity
var sid []byte
var mspid string
var chainid string = util.GetTestChainID()

func TestMain(m *testing.M) {
	ccprovider.SetChaincodesPath(lscctestpath)
	sysccprovider.RegisterSystemChaincodeProviderFactory(&scc.MocksccProviderFactory{})

	mspGetter := func(cid string) []string {
		return []string{"DEFAULT"}
	}

	peer.MockSetMSPIDGetter(mspGetter)

	var err error

	// setup the MSP manager so that we can sign/verify
	msptesttools.LoadMSPSetupForTesting()

	id, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("GetSigningIdentity failed with err %s", err)
		os.Exit(-1)
	}

	sid, err = id.Serialize()
	if err != nil {
		fmt.Printf("Serialize failed with err %s", err)
		os.Exit(-1)
	}

	// determine the MSP identifier for the first MSP in the default chain
	var msp msp.MSP
	mspMgr := mspmgmt.GetManagerForChain(chainid)
	msps, err := mspMgr.GetMSPs()
	if err != nil {
		fmt.Printf("Could not retrieve the MSPs for the chain manager, err %s", err)
		os.Exit(-1)
	}
	if len(msps) == 0 {
		fmt.Printf("At least one MSP was expected")
		os.Exit(-1)
	}
	for _, m := range msps {
		msp = m
		break
	}
	mspid, err = msp.GetIdentifier()
	if err != nil {
		fmt.Printf("Failure getting the msp identifier, err %s", err)
		os.Exit(-1)
	}

	// also set the MSP for the "test" chain
	mspmgmt.XXXSetMSPManager("test", mspmgmt.GetManagerForChain(util.GetTestChainID()))

	os.Exit(m.Run())
}
