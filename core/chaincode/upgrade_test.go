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
	putils "github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

//getUpgradeLSCCSpec gets the spec for the chaincode upgrade to be sent to LSCC
func getUpgradeLSCCSpec(chainID string, cds *pb.ChaincodeDeploymentSpec) (*pb.ChaincodeInvocationSpec, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	//wrap the deployment in an invocation spec to lscc...
	lsccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: "lscc"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("upgrade"), []byte(chainID), b}}}}

	return lsccSpec, nil
}

// upgrade a chaincode - i.e., build and initialize.
func upgrade(ctx context.Context, cccid *ccprovider.CCContext, spec *pb.ChaincodeSpec, blockNumber uint64) (*ccprovider.CCContext, error) {
	// First build and get the deployment spec
	chaincodeDeploymentSpec, err := getDeploymentSpec(ctx, spec)
	if err != nil {
		return nil, err
	}

	return upgrade2(ctx, cccid, chaincodeDeploymentSpec, blockNumber)
}

func upgrade2(ctx context.Context, cccid *ccprovider.CCContext,
	chaincodeDeploymentSpec *pb.ChaincodeDeploymentSpec, blockNumber uint64) (newcccid *ccprovider.CCContext, err error) {
	cis, err := getUpgradeLSCCSpec(cccid.ChainID, chaincodeDeploymentSpec)
	if err != nil {
		return nil, fmt.Errorf("Error creating lscc spec : %s\n", err)
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
			err = endTxSimulationCDS(cccid.ChainID, uuid, txsim, []byte("upgraded"), true, chaincodeDeploymentSpec, blockNumber)
		} else {
			//there was an error, just close simulation and return that
			endTxSimulationCDS(cccid.ChainID, uuid, txsim, []byte("upgraded"), false, chaincodeDeploymentSpec, blockNumber)
		}
	}()

	//ignore existence errors
	ccprovider.PutChaincodeIntoFS(chaincodeDeploymentSpec)

	sysCCVers := util.GetSysCCVersion()
	sprop, prop := putils.MockSignedEndorserProposal2OrPanic(cccid.ChainID, cis.ChaincodeSpec, signer)
	lsccid := ccprovider.NewCCContext(cccid.ChainID, cis.ChaincodeSpec.ChaincodeId.Name, sysCCVers, uuid, true, sprop, prop)

	var cdbytes []byte
	//write to lscc
	if cdbytes, _, err = ExecuteWithErrorFilter(ctx, lsccid, cis); err != nil {
		return nil, fmt.Errorf("Error executing LSCC for upgrade: %s", err)
	}

	if cdbytes == nil {
		return nil, fmt.Errorf("Expected ChaincodeData back from LSCC but got nil")
	}

	cd := &ccprovider.ChaincodeData{}
	if err = proto.Unmarshal(cdbytes, cd); err != nil {
		return nil, fmt.Errorf("getting  ChaincodeData failed")
	}

	newVersion := string(cd.Version)
	if newVersion == cccid.Version {
		return nil, fmt.Errorf("Expected new version from LSCC but got same %s(%s)", newVersion, cccid.Version)
	}

	newcccid = ccprovider.NewCCContext(cccid.ChainID, chaincodeDeploymentSpec.ChaincodeSpec.ChaincodeId.Name, newVersion, uuid, false, nil, nil)

	if _, _, err = ExecuteWithErrorFilter(ctx, newcccid, chaincodeDeploymentSpec); err != nil {
		return nil, fmt.Errorf("Error deploying chaincode for upgrade: %s", err)
	}
	return
}

//TestUpgradeCC - test basic upgrade
//     deploy example01
//     do a query against 01 that'll fail
//     upgrade to exampl02
//     show the upgrade worked using the same query successfully
//This test a variety of things in addition to basic upgrade
//     uses next version from lscc
//     re-initializtion of the same chaincode "mycc"
//     upgrade when "mycc" is up and running (test version based namespace)
func TestUpgradeCC(t *testing.T) {
	testForSkip(t)
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
	chaincodeID := &pb.ChaincodeID{Name: ccName, Path: url, Version: "0"}

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	cccid := ccprovider.NewCCContext(chainID, ccName, "0", "", false, nil, nil)
	var nextBlockNumber uint64 = 1
	_, err = deploy(ctxt, cccid, spec, nextBlockNumber)

	if err != nil {
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		t.Fail()
		t.Logf("Error deploying chaincode %s(%s)", chaincodeID, err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	// Query example01, which should fail
	qArgs := util.ToChaincodeArgs("query", "a")

	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: qArgs}}

	//Do not increment block number here because, the block will not be committted because of error
	_, _, _, err = invoke(ctxt, chainID, spec, nextBlockNumber, nil)
	if err == nil {
		t.Fail()
		t.Logf("querying chaincode exampl01 should fail transaction: %s", err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	} else if !strings.Contains(err.Error(), "Invalid invoke function name. Expecting \"invoke\"") {
		t.Fail()
		t.Logf("expected <Invalid invoke function name. Expecting \"invoke\"> found <%s>", err)
		theChaincodeSupport.Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	//now upgrade to example02 which takes the same args as example01 but inits state vars
	//and also allows query.
	url = "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"

	//Note ccName hasn't changed...
	chaincodeID = &pb.ChaincodeID{Name: ccName, Path: url, Version: "1"}
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	//...and get back the ccid with the new version
	nextBlockNumber++
	cccid2, err := upgrade(ctxt, cccid, spec, nextBlockNumber)
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
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: qArgs}}
	nextBlockNumber++
	_, _, _, err = invokeWithVersion(ctxt, chainID, cccid2.Version, spec, nextBlockNumber, nil)

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
	testForSkip(t)
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

	f := "init"
	args := util.ToChaincodeArgs(f, "a", "100", "b", "200")

	cccid := ccprovider.NewCCContext(chainID, ccName, "0", "", false, nil, nil)

	//Note ccName hasn't changed...
	chaincodeID := &pb.ChaincodeID{Name: ccName, Path: url, Version: "1"}
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}

	//...and get back the ccid with the new version
	var nextBlockNumber uint64
	cccid2, err := upgrade(ctxt, cccid, spec, nextBlockNumber)
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
