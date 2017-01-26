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

package endorser

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"path/filepath"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/peer"
	syscc "github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	pbutils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var endorserServer pb.EndorserServer
var mspInstance msp.MSP
var signer msp.SigningIdentity

//initialize peer and start up. If security==enabled, login as vp
func initPeer(chainID string) (net.Listener, error) {
	//start clean
	finitPeer(nil)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(viper.GetString("peer.tls.cert.file"), viper.GetString("peer.tls.key.file"))
		if err != nil {
			return nil, fmt.Errorf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	viper.Set("peer.fileSystemPath", filepath.Join(os.TempDir(), "hyperledger", "production"))

	peerAddress, err := peer.GetLocalAddress()
	if err != nil {
		return nil, fmt.Errorf("Error obtaining peer address: %s", err)
	}
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return nil, fmt.Errorf("Error starting peer listener %s", err)
	}

	//initialize ledger
	peer.MockInitialize()

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(30000) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, chaincode.NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout))

	syscc.RegisterSysCCs()

	if err = peer.MockCreateChain(chainID); err != nil {
		closeListenerAndSleep(lis)
		return nil, err
	}

	syscc.DeploySysCCs(chainID)

	go grpcServer.Serve(lis)

	return lis, nil
}

func finitPeer(lis net.Listener) {
	closeListenerAndSleep(lis)
	os.RemoveAll(filepath.Join(os.TempDir(), "hyperledger"))
}

func closeListenerAndSleep(l net.Listener) {
	if l != nil {
		l.Close()
		time.Sleep(2 * time.Second)
	}
}

//getProposal gets the proposal for the chaincode invocation
//Currently supported only for Invokes (Queries still go through devops client)
func getInvokeProposal(cis *pb.ChaincodeInvocationSpec, chainID string, creator []byte) (*pb.Proposal, error) {
	uuid := util.GenerateUUID()
	return pbutils.CreateChaincodeProposal(uuid, common.HeaderType_ENDORSER_TRANSACTION, chainID, cis, creator)
}

func getDeployProposal(cds *pb.ChaincodeDeploymentSpec, chainID string, creator []byte) (*pb.Proposal, error) {
	return getDeployOrUpgradeProposal(cds, chainID, creator, false)
}

func getUpgradeProposal(cds *pb.ChaincodeDeploymentSpec, chainID string, creator []byte) (*pb.Proposal, error) {
	return getDeployOrUpgradeProposal(cds, chainID, creator, true)
}

//getDeployOrUpgradeProposal gets the proposal for the chaincode deploy or upgrade
//the payload is a ChaincodeDeploymentSpec
func getDeployOrUpgradeProposal(cds *pb.ChaincodeDeploymentSpec, chainID string, creator []byte, upgrade bool) (*pb.Proposal, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	var propType string
	if upgrade {
		propType = "upgrade"
	} else {
		propType = "deploy"
	}
	//wrap the deployment in an invocation spec to lccc...
	lcccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Name: "lccc"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte(propType), []byte(chainID), b}}}}

	//...and get the proposal for it
	return getInvokeProposal(lcccSpec, chainID, creator)
}

func getSignedProposal(prop *pb.Proposal, signer msp.SigningIdentity) (*pb.SignedProposal, error) {
	propBytes, err := pbutils.GetBytesProposal(prop)
	if err != nil {
		return nil, err
	}

	signature, err := signer.Sign(propBytes)
	if err != nil {
		return nil, err
	}

	return &pb.SignedProposal{ProposalBytes: propBytes, Signature: signature}, nil
}

func getDeploymentSpec(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	codePackageBytes, err := container.GetChaincodePackageBytes(spec)
	if err != nil {
		return nil, err
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func deploy(endorserServer pb.EndorserServer, chainID string, spec *pb.ChaincodeSpec, f func(*pb.ChaincodeDeploymentSpec)) (*pb.ProposalResponse, *pb.Proposal, error) {
	return deployOrUpgrade(endorserServer, chainID, spec, f, false)
}

func upgrade(endorserServer pb.EndorserServer, chainID string, spec *pb.ChaincodeSpec, f func(*pb.ChaincodeDeploymentSpec)) (*pb.ProposalResponse, *pb.Proposal, error) {
	return deployOrUpgrade(endorserServer, chainID, spec, f, true)
}

func deployOrUpgrade(endorserServer pb.EndorserServer, chainID string, spec *pb.ChaincodeSpec, f func(*pb.ChaincodeDeploymentSpec), upgrade bool) (*pb.ProposalResponse, *pb.Proposal, error) {
	var err error
	var depSpec *pb.ChaincodeDeploymentSpec

	ctxt := context.Background()
	depSpec, err = getDeploymentSpec(ctxt, spec)
	if err != nil {
		return nil, nil, err
	}

	if f != nil {
		f(depSpec)
	}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, nil, err
	}

	var prop *pb.Proposal
	if upgrade {
		prop, err = getUpgradeProposal(depSpec, chainID, creator)
	} else {
		prop, err = getDeployProposal(depSpec, chainID, creator)
	}
	if err != nil {
		return nil, nil, err
	}

	var signedProp *pb.SignedProposal
	signedProp, err = getSignedProposal(prop, signer)
	if err != nil {
		return nil, nil, err
	}

	var resp *pb.ProposalResponse
	resp, err = endorserServer.ProcessProposal(context.Background(), signedProp)

	return resp, prop, err
}

func invoke(chainID string, spec *pb.ChaincodeSpec) (*pb.ProposalResponse, error) {
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	var prop *pb.Proposal
	prop, err = getInvokeProposal(invocation, chainID, creator)
	if err != nil {
		return nil, fmt.Errorf("Error creating proposal  %s: %s\n", spec.ChaincodeID, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = getSignedProposal(prop, signer)
	if err != nil {
		return nil, err
	}

	resp, err := endorserServer.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing %s: %s\n", spec.ChaincodeID, err)
	}

	return resp, err
}

//begin tests. Note that we rely upon the system chaincode and peer to be created
//once and be used for all the tests. In order to avoid dependencies / collisions
//due to deployed chaincodes, trying to use different chaincodes for different
//tests

//TestDeploy deploy chaincode example01
func TestDeploy(t *testing.T) {
	chainID := util.GetTestChainID()
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "ex01", Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	cccid := ccprovider.NewCCContext(chainID, "ex01", "0", "", false, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("Deploy-error in deploy %s", err)
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestDeployBadArgs sets bad args on deploy. It should fail, and example02 should not be deployed
func TestDeployBadArgs(t *testing.T) {
	chainID := util.GetTestChainID()
	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "ex02", Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b")}}}

	cccid := ccprovider.NewCCContext(chainID, "ex02", "0", "", false, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err == nil {
		t.Fail()
		t.Log("DeployBadArgs-expected error in deploy but succeeded")
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestDeployBadPayload set payload to nil and do a deploy. It should fail and example02 should not be deployed
func TestDeployBadPayload(t *testing.T) {
	chainID := util.GetTestChainID()
	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "ex02", Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	cccid := ccprovider.NewCCContext(chainID, "ex02", "0", "", false, nil)

	f := func(cds *pb.ChaincodeDeploymentSpec) {
		cds.CodePackage = nil
	}
	_, _, err := deploy(endorserServer, chainID, spec, f)
	if err == nil {
		t.Fail()
		t.Log("DeployBadPayload-expected error in deploy but succeeded")
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestRedeploy - deploy two times, second time should fail but example02 should remain deployed
func TestRedeploy(t *testing.T) {
	chainID := util.GetTestChainID()

	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: "ex02", Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	cccid := ccprovider.NewCCContext(chainID, "ex02", "0", "", false, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("error in endorserServer.ProcessProposal %s", err)
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	//second time should not fail as we are just simulating
	_, _, err = deploy(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("error in endorserServer.ProcessProposal %s", err)
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

// TestDeployAndInvoke deploys and invokes chaincode_example01
func TestDeployAndInvoke(t *testing.T) {
	chainID := util.GetTestChainID()
	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	chaincodeID := &pb.ChaincodeID{Path: url, Name: "ex01"}

	args := []string{"10"}

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: argsDeploy}}

	cccid := ccprovider.NewCCContext(chainID, "ex01", "0", "", false, nil)

	resp, prop, err := deploy(endorserServer, chainID, spec, nil)
	chaincodeID1 := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		t.Logf("Error deploying <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
		return
	}

	err = endorserServer.(*Endorser).commitTxSimulation(prop, chainID, signer, resp)
	if err != nil {
		t.Fail()
		t.Logf("Error committing <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
		return
	}

	f = "invoke"
	invokeArgs := append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	resp, err = invoke(chainID, spec)
	if err != nil {
		t.Fail()
		t.Logf("Error invoking transaction: %s", err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
		return
	}

	fmt.Printf("Invoke test passed\n")
	t.Logf("Invoke test passed")

	chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID}})
}

// TestUpgradeAndInvoke deploys chaincode_example01, upgrade it with chaincode_example02, then invoke it
func TestDeployAndUpgrade(t *testing.T) {
	chainID := util.GetTestChainID()
	var ctxt = context.Background()

	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID1 := &pb.ChaincodeID{Path: url1, Name: "upgradeex01"}
	chaincodeID2 := &pb.ChaincodeID{Path: url2, Name: "upgradeex01"}

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID1, Input: &pb.ChaincodeInput{Args: argsDeploy}}

	cccid1 := ccprovider.NewCCContext(chainID, "upgradeex01", "0", "", false, nil)
	cccid2 := ccprovider.NewCCContext(chainID, "upgradeex01", "1", "", false, nil)

	resp, prop, err := deploy(endorserServer, chainID, spec, nil)

	chaincodeName := spec.ChaincodeID.Name
	if err != nil {
		t.Fail()
		chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID1}})
		chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID2}})
		t.Logf("Error deploying <%s>: %s", chaincodeName, err)
		return
	}

	err = endorserServer.(*Endorser).commitTxSimulation(prop, chainID, signer, resp)
	if err != nil {
		t.Fail()
		chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID1}})
		chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID2}})
		t.Logf("Error committing <%s>: %s", chaincodeName, err)
		return
	}

	argsUpgrade := util.ToChaincodeArgs(f, "a", "150", "b", "300")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeID: chaincodeID2, Input: &pb.ChaincodeInput{Args: argsUpgrade}}
	resp, prop, err = upgrade(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID1}})
		chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID2}})
		t.Logf("Error upgrading <%s>: %s", chaincodeName, err)
		return
	}

	fmt.Printf("Upgrade test passed\n")
	t.Logf("Upgrade test passed")

	chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID1}})
	chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeID: chaincodeID2}})
}

func TestMain(m *testing.M) {
	SetupTestConfig()
	viper.Set("peer.fileSystemPath", filepath.Join(os.TempDir(), "hyperledger", "production"))

	chainID := util.GetTestChainID()
	lis, err := initPeer(chainID)
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize tests")
		finitPeer(lis)
		return
	}

	endorserServer = NewEndorserServer()

	// setup the MSP manager so that we can sign/verify
	mspMgrConfigDir := "../../msp/sampleconfig/"
	err = mspmgmt.LoadFakeSetupWithLocalMspAndTestChainMsp(mspMgrConfigDir)
	if err != nil {
		fmt.Printf("Could not initialize msp/signer, err %s", err)
		os.Exit(-1)
		finitPeer(lis)
		return
	}
	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("Could not initialize msp/signer")
		os.Exit(-1)
		finitPeer(lis)
		return
	}

	retVal := m.Run()

	finitPeer(lis)

	os.Exit(retVal)
}
