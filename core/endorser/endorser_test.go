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
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/peer"
	syscc "github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/testutil"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	pbutils "github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var endorserServer pb.EndorserServer
var signer msp.SigningIdentity

type testEnvironment struct {
	tempDir  string
	listener net.Listener
}

//initialize peer and start up. If security==enabled, login as vp
func initPeer(chainID string) (*testEnvironment, error) {
	//start clean
	// finitPeer(nil)
	var opts []grpc.ServerOption
	if viper.GetBool("peer.tls.enabled") {
		creds, err := credentials.NewServerTLSFromFile(config.GetPath("peer.tls.cert.file"), config.GetPath("peer.tls.key.file"))
		if err != nil {
			return nil, fmt.Errorf("Failed to generate credentials %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	tempDir := newTempDir()
	viper.Set("peer.fileSystemPath", filepath.Join(tempDir, "hyperledger", "production"))

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

	mspGetter := func(cid string) []string {
		return []string{"DEFAULT"}
	}

	peer.MockSetMSPIDGetter(mspGetter)

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{Id: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
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

	return &testEnvironment{tempDir: tempDir, listener: lis}, nil
}

func finitPeer(tev *testEnvironment) {
	closeListenerAndSleep(tev.listener)
	os.RemoveAll(tev.tempDir)
}

func closeListenerAndSleep(l net.Listener) {
	if l != nil {
		l.Close()
		time.Sleep(2 * time.Second)
	}
}

// getInvokeProposal gets the proposal for the chaincode invocation
// Currently supported only for Invokes
// It returns the proposal and the transaction id associated to the proposal
func getInvokeProposal(cis *pb.ChaincodeInvocationSpec, chainID string, creator []byte) (*pb.Proposal, string, error) {
	return pbutils.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, chainID, cis, creator)
}

// getInvokeProposalOverride allows to get a proposal for the chaincode invocation
// overriding transaction id and nonce which are by default auto-generated.
// It returns the proposal and the transaction id associated to the proposal
func getInvokeProposalOverride(txid string, cis *pb.ChaincodeInvocationSpec, chainID string, nonce, creator []byte) (*pb.Proposal, string, error) {
	return pbutils.CreateChaincodeProposalWithTxIDNonceAndTransient(txid, common.HeaderType_ENDORSER_TRANSACTION, chainID, cis, nonce, creator, nil)
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
	//we need to save off the chaincode as we have to instantiate with nil CodePackage
	var err error
	if err = ccprovider.PutChaincodeIntoFS(cds); err != nil {
		return nil, err
	}

	cds.CodePackage = nil

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
	sccver := util.GetSysCCVersion()
	//wrap the deployment in an invocation spec to lscc...
	lsccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: &pb.ChaincodeID{Name: "lscc", Version: sccver}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte(propType), []byte(chainID), b}}}}

	//...and get the proposal for it
	var prop *pb.Proposal
	if prop, _, err = getInvokeProposal(lsccSpec, chainID, creator); err != nil {
		return nil, err
	}

	return prop, nil
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

func invoke(chainID string, spec *pb.ChaincodeSpec) (*pb.Proposal, *pb.ProposalResponse, string, []byte, error) {
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, nil, "", nil, err
	}

	var prop *pb.Proposal
	prop, txID, err := getInvokeProposal(invocation, chainID, creator)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("Error creating proposal  %s: %s\n", spec.ChaincodeId, err)
	}

	nonce, err := pbutils.GetNonce(prop)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("Failed getting nonce  %s: %s\n", spec.ChaincodeId, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = getSignedProposal(prop, signer)
	if err != nil {
		return nil, nil, "", nil, err
	}

	resp, err := endorserServer.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, nil, "", nil, err
	}

	return prop, resp, txID, nonce, err
}

func invokeWithOverride(txid string, chainID string, spec *pb.ChaincodeSpec, nonce []byte) (*pb.ProposalResponse, error) {
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	var prop *pb.Proposal
	prop, _, err = getInvokeProposalOverride(txid, invocation, chainID, nonce, creator)
	if err != nil {
		return nil, fmt.Errorf("Error creating proposal with override  %s %s: %s\n", txid, spec.ChaincodeId, err)
	}

	var signedProp *pb.SignedProposal
	signedProp, err = getSignedProposal(prop, signer)
	if err != nil {
		return nil, err
	}

	resp, err := endorserServer.ProcessProposal(context.Background(), signedProp)
	if err != nil {
		return nil, fmt.Errorf("Error endorsing %s %s: %s\n", txid, spec.ChaincodeId, err)
	}

	return resp, err
}

func deleteChaincodeOnDisk(chaincodeID string) {
	os.RemoveAll(filepath.Join(config.GetPath("peer.fileSystemPath"), "chaincodes", chaincodeID))
}

//begin tests. Note that we rely upon the system chaincode and peer to be created
//once and be used for all the tests. In order to avoid dependencies / collisions
//due to deployed chaincodes, trying to use different chaincodes for different
//tests

//TestDeploy deploy chaincode example01
func TestDeploy(t *testing.T) {
	chainID := util.GetTestChainID()
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "ex01", Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}
	defer deleteChaincodeOnDisk("ex01.0")

	cccid := ccprovider.NewCCContext(chainID, "ex01", "0", "", false, nil, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("Deploy-error in deploy %s", err)
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//REMOVE WHEN JAVA CC IS ENABLED
func TestJavaDeploy(t *testing.T) {
	chainID := util.GetTestChainID()
	//pretend this is a java CC (type 4)
	spec := &pb.ChaincodeSpec{Type: 4, ChaincodeId: &pb.ChaincodeID{Name: "javacc", Path: "../../examples/chaincode/java/chaincode_example02", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}
	defer deleteChaincodeOnDisk("javacc.0")

	cccid := ccprovider.NewCCContext(chainID, "javacc", "0", "", false, nil, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err == nil {
		t.Fail()
		t.Logf("expected java CC deploy to fail")
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

func TestJavaCheckWithDifferentPackageTypes(t *testing.T) {
	//try SignedChaincodeDeploymentSpec with go chaincode (type 1)
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "gocc", Path: "path/to/cc", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("someargs")}}}
	cds := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: []byte("some code")}
	env := &common.Envelope{Payload: pbutils.MarshalOrPanic(&common.Payload{Data: pbutils.MarshalOrPanic(&pb.SignedChaincodeDeploymentSpec{ChaincodeDeploymentSpec: pbutils.MarshalOrPanic(cds)})})}
	//wrap the package in an invocation spec to lscc...
	b := pbutils.MarshalOrPanic(env)

	lsccCID := &pb.ChaincodeID{Name: "lscc", Version: util.GetSysCCVersion()}
	lsccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: lsccCID, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("install"), b}}}}

	e := &Endorser{}
	err := e.disableJavaCCInst(lsccCID, lsccSpec)
	assert.Nil(t, err)

	//now try plain ChaincodeDeploymentSpec...should succeed (go chaincode)
	b = pbutils.MarshalOrPanic(cds)

	lsccSpec = &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeId: lsccCID, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("install"), b}}}}
	err = e.disableJavaCCInst(lsccCID, lsccSpec)
	assert.Nil(t, err)
}

//TestRedeploy - deploy two times, second time should fail but example02 should remain deployed
func TestRedeploy(t *testing.T) {
	chainID := util.GetTestChainID()

	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: &pb.ChaincodeID{Name: "ex02", Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", Version: "0"}, Input: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	defer deleteChaincodeOnDisk("ex02.0")

	cccid := ccprovider.NewCCContext(chainID, "ex02", "0", "", false, nil, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("error in endorserServer.ProcessProposal %s", err)
		chaincode.GetChain().Stop(context.Background(), cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	deleteChaincodeOnDisk("ex02.0")

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
	chaincodeID := &pb.ChaincodeID{Path: url, Name: "ex01", Version: "0"}

	defer deleteChaincodeOnDisk("ex01.0")

	args := []string{"10"}

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: argsDeploy}}

	cccid := ccprovider.NewCCContext(chainID, "ex01", "0", "", false, nil, nil)

	resp, prop, err := deploy(endorserServer, chainID, spec, nil)
	chaincodeID1 := spec.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error deploying <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}
	var nextBlockNumber uint64 = 1 // first block needs to be block number = 1. Genesis block is block 0
	err = endorserServer.(*Endorser).commitTxSimulation(prop, chainID, signer, resp, nextBlockNumber)
	if err != nil {
		t.Fail()
		t.Logf("Error committing deploy <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	f = "invoke"
	invokeArgs := append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	prop, resp, txid, nonce, err := invoke(chainID, spec)
	if err != nil {
		t.Fail()
		t.Logf("Error invoking transaction: %s", err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}
	// Commit invoke
	nextBlockNumber++
	err = endorserServer.(*Endorser).commitTxSimulation(prop, chainID, signer, resp, nextBlockNumber)
	if err != nil {
		t.Fail()
		t.Logf("Error committing first invoke <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	// Now test for an invalid TxID
	f = "invoke"
	invokeArgs = append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	_, err = invokeWithOverride("invalid_tx_id", chainID, spec, nonce)
	if err == nil {
		t.Fail()
		t.Log("Replay attack protection faild. Transaction with invalid txid passed")
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	// Now test for duplicated TxID
	f = "invoke"
	invokeArgs = append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	_, err = invokeWithOverride(txid, chainID, spec, nonce)
	if err == nil {
		t.Fail()
		t.Log("Replay attack protection faild. Transaction with duplicaged txid passed")
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	// Test chaincode endorsement failure when invalid function name supplied
	f = "invokeinvalid"
	invokeArgs = append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	prop, resp, txid, nonce, err = invoke(chainID, spec)
	if err == nil {
		t.Fail()
		t.Logf("expecting fabric to report error from chaincode failure")
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	} else if _, ok := err.(*chaincodeError); !ok {
		t.Fail()
		t.Logf("expecting chaincode error but found %v", err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	if resp != nil {
		assert.Equal(t, int32(500), resp.Response.Status, "Unexpected response status")
	}

	t.Logf("Invoke test passed")

	chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
}

// TestUpgradeAndInvoke deploys chaincode_example01, upgrade it with chaincode_example02, then invoke it
func TestDeployAndUpgrade(t *testing.T) {
	chainID := util.GetTestChainID()
	var ctxt = context.Background()

	url1 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	url2 := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"
	chaincodeID1 := &pb.ChaincodeID{Path: url1, Name: "upgradeex01", Version: "0"}
	chaincodeID2 := &pb.ChaincodeID{Path: url2, Name: "upgradeex01", Version: "1"}

	defer deleteChaincodeOnDisk("upgradeex01.0")
	defer deleteChaincodeOnDisk("upgradeex01.1")

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID1, Input: &pb.ChaincodeInput{Args: argsDeploy}}

	cccid1 := ccprovider.NewCCContext(chainID, "upgradeex01", "0", "", false, nil, nil)
	cccid2 := ccprovider.NewCCContext(chainID, "upgradeex01", "1", "", false, nil, nil)

	resp, prop, err := deploy(endorserServer, chainID, spec, nil)

	chaincodeName := spec.ChaincodeId.Name
	if err != nil {
		t.Fail()
		chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID1}})
		chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID2}})
		t.Logf("Error deploying <%s>: %s", chaincodeName, err)
		return
	}

	var nextBlockNumber uint64 = 3 // something above created block 0
	err = endorserServer.(*Endorser).commitTxSimulation(prop, chainID, signer, resp, nextBlockNumber)
	if err != nil {
		t.Fail()
		chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID1}})
		chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID2}})
		t.Logf("Error committing <%s>: %s", chaincodeName, err)
		return
	}

	argsUpgrade := util.ToChaincodeArgs(f, "a", "150", "b", "300")
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID2, Input: &pb.ChaincodeInput{Args: argsUpgrade}}
	_, _, err = upgrade(endorserServer, chainID, spec, nil)
	if err != nil {
		t.Fail()
		chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID1}})
		chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID2}})
		t.Logf("Error upgrading <%s>: %s", chaincodeName, err)
		return
	}

	t.Logf("Upgrade test passed")

	chaincode.GetChain().Stop(ctxt, cccid1, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID1}})
	chaincode.GetChain().Stop(ctxt, cccid2, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID2}})
}

// TestWritersACLFail deploys a chaincode and then tries to invoke it;
// however we inject a special policy for writers to simulate
// the scenario in which the creator of this proposal is not among
// the writers for the chain
func TestWritersACLFail(t *testing.T) {
	//skip pending FAB-2457 fix
	t.Skip()
	chainID := util.GetTestChainID()
	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	chaincodeID := &pb.ChaincodeID{Path: url, Name: "ex01-fail", Version: "0"}

	defer deleteChaincodeOnDisk("ex01-fail.0")

	args := []string{"10"}

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: argsDeploy}}

	cccid := ccprovider.NewCCContext(chainID, "ex01-fail", "0", "", false, nil, nil)

	resp, prop, err := deploy(endorserServer, chainID, spec, nil)
	chaincodeID1 := spec.ChaincodeId.Name
	if err != nil {
		t.Fail()
		t.Logf("Error deploying <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}
	var nextBlockNumber uint64 = 3 // The tests that ran before this test created blocks 0-2
	err = endorserServer.(*Endorser).commitTxSimulation(prop, chainID, signer, resp, nextBlockNumber)
	if err != nil {
		t.Fail()
		t.Logf("Error committing deploy <%s>: %s", chaincodeID1, err)
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	// here we inject a reject policy for writers
	// to simulate the scenario in which the invoker
	// is not authorized to issue this proposal
	rejectpolicy := &mockpolicies.Policy{
		Err: errors.New("The creator of this proposal does not fulfil the writers policy of this chain"),
	}
	pm := peer.GetPolicyManager(chainID)
	pm.(*mockpolicies.Manager).PolicyMap = map[string]policies.Policy{policies.ChannelApplicationWriters: rejectpolicy}

	f = "invoke"
	invokeArgs := append([]string{f}, args...)
	spec = &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs(invokeArgs...)}}
	prop, resp, _, _, err = invoke(chainID, spec)
	if err == nil {
		t.Fail()
		t.Logf("Invocation should have failed")
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	t.Logf("TestWritersACLFail passed")

	chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
}

func TestHeaderExtensionNoChaincodeID(t *testing.T) {
	creator, _ := signer.Serialize()
	nonce := []byte{1, 2, 3}
	digest, err := factory.GetDefault().Hash(append(nonce, creator...), &bccsp.SHA256Opts{})
	txID := hex.EncodeToString(digest)
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: nil, Input: &pb.ChaincodeInput{Args: util.ToChaincodeArgs()}}
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}
	prop, _, _ := pbutils.CreateChaincodeProposalWithTxIDNonceAndTransient(txID, common.HeaderType_ENDORSER_TRANSACTION, util.GetTestChainID(), invocation, []byte{1, 2, 3}, creator, nil)
	signedProp, _ := getSignedProposal(prop, signer)
	_, err = endorserServer.ProcessProposal(context.Background(), signedProp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ChaincodeHeaderExtension.ChaincodeId is nil")
}

// TestAdminACLFail deploys tried to deploy a chaincode;
// however we inject a special policy for admins to simulate
// the scenario in which the creator of this proposal is not among
// the admins for the chain
func TestAdminACLFail(t *testing.T) {
	//skip pending FAB-2457 fix
	t.Skip()
	chainID := util.GetTestChainID()

	// here we inject a reject policy for admins
	// to simulate the scenario in which the invoker
	// is not authorized to issue this proposal
	rejectpolicy := &mockpolicies.Policy{
		Err: errors.New("The creator of this proposal does not fulfil the writers policy of this chain"),
	}
	pm := peer.GetPolicyManager(chainID)
	pm.(*mockpolicies.Manager).PolicyMap = map[string]policies.Policy{policies.ChannelApplicationAdmins: rejectpolicy}

	var ctxt = context.Background()

	url := "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"
	chaincodeID := &pb.ChaincodeID{Path: url, Name: "ex01-fail1", Version: "0"}

	defer deleteChaincodeOnDisk("ex01-fail1.0")

	f := "init"
	argsDeploy := util.ToChaincodeArgs(f, "a", "100", "b", "200")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: argsDeploy}}

	cccid := ccprovider.NewCCContext(chainID, "ex01-fail1", "0", "", false, nil, nil)

	_, _, err := deploy(endorserServer, chainID, spec, nil)
	if err == nil {
		t.Fail()
		t.Logf("Deploying chaincode should have failed!")
		chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
		return
	}

	t.Logf("TestATestAdminACLFailCLFail passed")

	chaincode.GetChain().Stop(ctxt, cccid, &pb.ChaincodeDeploymentSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: chaincodeID}})
}

// TestInvokeSccFail makes sure that invoking a system chaincode fails
func TestInvokeSccFail(t *testing.T) {
	chainID := util.GetTestChainID()

	chaincodeID := &pb.ChaincodeID{Name: "escc"}
	args := util.ToChaincodeArgs("someFunc", "someArg")
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeId: chaincodeID, Input: &pb.ChaincodeInput{Args: args}}
	_, _, _, _, err := invoke(chainID, spec)
	if err == nil {
		t.Logf("Invoking escc should have failed!")
		t.Fail()
		return
	}
}

func newTempDir() string {
	tempDir, err := ioutil.TempDir("", "fabric-")
	if err != nil {
		panic(err)
	}
	return tempDir
}

func TestMain(m *testing.M) {
	setupTestConfig()

	chainID := util.GetTestChainID()
	tev, err := initPeer(chainID)
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize tests")
		finitPeer(tev)
		return
	}

	endorserServer = NewEndorserServer()

	// setup the MSP manager so that we can sign/verify
	err = msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		fmt.Printf("Could not initialize msp/signer, err %s", err)
		finitPeer(tev)
		os.Exit(-1)
		return
	}
	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		fmt.Printf("Could not initialize msp/signer")
		finitPeer(tev)
		os.Exit(-1)
		return
	}

	retVal := m.Run()

	finitPeer(tev)

	os.Exit(retVal)
}

func setupTestConfig() {
	flag.Parse()

	// Now set the configuration file
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.SetConfigName("endorser_test") // name of config file (without extension)
	viper.AddConfigPath("./")            // path to look for the config file in
	err := viper.ReadInConfig()          // Find and read the config file
	if err != nil {                      // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	testutil.SetupTestLogging()

	// Set the number of maxprocs
	runtime.GOMAXPROCS(viper.GetInt("peer.gomaxprocs"))

	// Init the BCCSP
	var bccspConfig *factory.FactoryOpts
	err = viper.UnmarshalKey("peer.BCCSP", &bccspConfig)
	if err != nil {
		bccspConfig = nil
	}

	msp.SetupBCCSPKeystoreConfig(bccspConfig, viper.GetString("peer.mspConfigPath")+"/keystore")

	err = factory.InitFactories(bccspConfig)
	if err != nil {
		panic(fmt.Errorf("Could not initialize BCCSP Factories [%s]", err))
	}
}
