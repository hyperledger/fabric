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

package chaincode

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

//getUpgradeLCCCSpec gets the spec for the chaincode upgrade to be sent to LCCC
func getUpgradeLCCCSpec(chainID string, cds *pb.ChaincodeDeploymentSpec) (*pb.ChaincodeInvocationSpec, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	//wrap the deployment in an invocation spec to lccc...
	lcccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Name: "lccc"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("upgrade"), []byte(chainID), b}}}}

	return lcccSpec, nil
}

// upgrade a chaincode - i.e., build and initialize.
func upgrade(ctx context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeSpec) (*ccprovider.CCContext, error) {
	// First build and get the deployment spec
	chaincodeDeploymentSpec, err := getDeploymentSpec(ctx, spec)
	if err != nil {
		return nil, err
	}

	return upgrade2(ctx, cccid, chaincodeDeploymentSpec)
}

func upgrade2(ctx context.Context, cccid *ccprovider.CCContext, chaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec) (*ccprovider.CCContext, error) {
	cis, err := getUpgradeLCCCSpec(cccid.ChainID, chaincodeDeploymentSpec)
	if err != nil {
		return nil, fmt.Errorf("Error creating lccc spec : %s\n", err)
	}

	ctx, txsim, err := startTxSimulation(ctx, cccid.ChainID)
	if err != nil {
		return nil, fmt.Errorf("Failed to get handle to simulator: %s ", err)
	}

	uuid := util.GenerateUUID()

	cccid.TxID = uuid

	defer func() {
		//no error, lets try commit
		if err == nil {
			//capture returned error from commit
			err = endTxSimulationCDS(cccid.ChainID, uuid, txsim, []byte("upgraded"), true, chaincodeDeploymentSpec)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCDS(cccid.ChainID, uuid, txsim, []byte("upgraded"), false, chaincodeDeploymentSpec)
		}
	}()

	sysCCVers := util.GetSysCCVersion()
	lcccid := ccprovider.NewCCContext(cccid.ChainID, cis.ChaincodeSpec.ChaincodeID.Name, sysCCVers, uuid, true, nil)

	var versionBytes []byte
	//write to lccc
	if versionBytes, _, err = ExecuteWithErrorFilter(ctx, lcccid, cis); err != nil {
		return nil, fmt.Errorf("Error executing LCCC for upgrade: %s", err)
	}

	if versionBytes == nil {
		return nil, fmt.Errorf("Expected version back from LCCC but got nil")
	}

	newVersion := string(versionBytes)
	if newVersion == cccid.Version {
		return nil, fmt.Errorf("Expected new version from LCCC but got same %s(%s)", newVersion, cccid.Version)
	}

	newcccid := ccprovider.NewCCContext(cccid.ChainID, chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeID.Name, newVersion, uuid, false, nil)

	if _, _, err = ExecuteWithErrorFilter(ctx, newcccid, chaincodeDeploymentSpec); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode for upgrade: %s", err)
	}

	return newcccid, nil
}

//TestUpgradeCC - test basic upgrade
//     deploy example01
//     do a query against 01 that'll fail
//     upgrade to exampl02
//     show the upgrade worked using the same query successfully
//This test a variety of things in addition to basic upgrade
//     uses next version from lccc
//     re-initializtion of the same chaincode "mycc"
//     upgrade when "mycc" is up and running (test version based namespace)
func TestUpgradeCC(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	ccName := "mycc"
	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	chaincodeID := &pb.ChaincodeID{Name: ccName, Path: url}

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	cccid := ccprovider.NewCCContext(chainID, ccName, "0", "", false, nil)
	_, err = deploy(ctxt, cccid, spec)

	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		t.Fail()
		t.Logf("Error deploying chaincode %s(%s)", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
		return
	}

	// Query example01, which should fail
	qArgs := util.ToChaincodeArgs("query", "a")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: qArgs}}

	_, _, _, err = invoke(ctxt, chainID, spec)
	if err == nil {
		t.Fail()
		t.Logf("querying chaincode exampl01 should fail transaction: %s", err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
		return
	} else if !strings.Contains(err.Error(), "Invalid invoke function name. Expecting \"invoke\"") {
		t.Fail()
		t.Logf("expected <Invalid invoke function name. Expecting \"invoke\"> found <%s>", err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
		return
	}

	//now upgrade to example02 which takes the same args as example01 but inits state vars
	//and also allows query.
	url = "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	//Note ccName hasn't changed...
	chaincodeID = &pb.ChaincodeID{Name: ccName, Path: url}
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	//...and get back the ccid with the new version
	cccid2, err := upgrade(ctxt, cccid, spec)
	if err != nil {
		t.Fail()
		t.Logf("Error upgrading chaincode %s(%s)", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		if cccid2 != nil {
			theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		}
		return
	}

	//go back and do the same query now
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: qArgs}}

	_, _, _, err = invokeWithVersion(ctxt, chainID, cccid2.Version, spec)
	if err != nil {
		t.Fail()
		t.Logf("querying chaincode exampl02 did not succeed: %s", err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
	theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestInvalUpgradeCC - test basic upgrade
//     upgrade to exampl02 when "mycc" is not deployed
//     look for "not found" failure
func TestInvalUpgradeCC(t *testing.T) {
	chainID := util.GetTestChainID()

	lis, err := initPeer(chainID)
	if err != nil {
		t.Fail()
		t.Logf("Error creating peer: %s", err)
	}

	defer finitPeer(lis, chainID)

	var ctxt = context.Background()

	ccName := "mycc"
	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID := &pb.ChaincodeID{Name: ccName, Path: url}

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	cccid := ccprovider.NewCCContext(chainID, ccName, "0", "", false, nil)

	//Note ccName hasn't changed...
	chaincodeID = &pb.ChaincodeID{Name: ccName, Path: url}
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	//...and get back the ccid with the new version
	cccid2, err := upgrade(ctxt, cccid, spec)
	if err == nil {
		t.Fail()
		t.Logf("Error expected upgrading to fail but it succeeded%s(%s)", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		if cccid2 != nil {
			theChaincodeSupport.Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		}
		return
	}

	theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}
