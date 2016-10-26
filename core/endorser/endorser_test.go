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
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/crypto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	u "github.com/hyperledger/fabric/core/util"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var testDBWrapper = db.NewTestDBWrapper()
var endorserServer pb.EndorserServer

//initialize peer and start up. If security==enabled, login as vp
func initPeer() (net.Listener, error) {
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

	peerAddress := viper.GetString("peer.address")
	lis, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return nil, fmt.Errorf("Error starting peer listener %s", err)
	}

	//initialize ledger
	lpath := viper.GetString("peer.fileSystemPath")
	kvledger.Initialize(lpath)

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	// Install security object for peer
	var secHelper crypto.Peer
	if viper.GetBool("security.enabled") {
		enrollID := viper.GetString("security.enrollID")
		enrollSecret := viper.GetString("security.enrollSecret")
		if err = crypto.RegisterValidator(enrollID, nil, enrollID, enrollSecret); nil != err {
			return nil, err
		}
		secHelper, err = crypto.InitValidator(enrollID, nil)
		if nil != err {
			return nil, err
		}
	}

	ccStartupTimeout := time.Duration(30000) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, chaincode.NewChaincodeSupport(chaincode.DefaultChain, getPeerEndpoint, false, ccStartupTimeout, secHelper))

	chaincode.RegisterSysCCs()

	chaincodeID := &pb.ChaincodeID{Path: "github.com/hyperledger/fabric/core/chaincode/lccc", Name: "lccc"}
	spec := pb.ChaincodeSpec{Type: pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value["GOLANG"]), ChaincodeID: chaincodeID, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("")}}}

	chaincode.DeploySysCC(context.Background(), &spec)

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
func getProposal(cis *pb.ChaincodeInvocationSpec) (*pb.Proposal, error) {
	b, err := proto.Marshal(cis)
	if err != nil {
		return nil, err
	}

	prop := &pb.Proposal{Type: pb.Proposal_CHAINCODE, Id: u.GenerateUUID(), Payload: b}

	return prop, nil
}

//getDeployProposal gets the proposal for the chaincode deployment
//the payload is a ChaincodeDeploymentSpec
func getDeployProposal(cds *pb.ChaincodeDeploymentSpec) (*pb.Proposal, error) {
	b, err := proto.Marshal(cds)
	if err != nil {
		return nil, err
	}

	//wrap the deployment in an invocation spec to lccc...
	lcccSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG, ChaincodeID: &pb.ChaincodeID{Name: "lccc"}, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("deploy"), []byte("default"), b}}}}

	//...and get the proposal for it
	return getProposal(lcccSpec)
}

func getDeploymentSpec(context context.Context, spec *pb.ChaincodeSpec) (*pb.ChaincodeDeploymentSpec, error) {
	codePackageBytes, err := container.GetChaincodePackageBytes(spec)
	if err != nil {
		return nil, err
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func deploy(endorserServer pb.EndorserServer, spec *pb.ChaincodeSpec, f func(*pb.ChaincodeDeploymentSpec)) (*pb.ProposalResponse, error) {
	var err error
	var depSpec *pb.ChaincodeDeploymentSpec

	ctxt := context.Background()
	depSpec, err = getDeploymentSpec(ctxt, spec)
	if err != nil {
		return nil, err
	}

	if f != nil {
		f(depSpec)
	}

	var prop *pb.Proposal
	prop, err = getDeployProposal(depSpec)
	if err != nil {
		return nil, err
	}

	var resp *pb.ProposalResponse
	resp, err = endorserServer.ProcessProposal(context.Background(), prop)

	return resp, err
}

//begin tests. Note that we rely upon the system chaincode and peer to be created
//once and be used for all the tests. In order to avoid dependencies / collisions
//due to deployed chaincodes, trying to use different chaincodes for different
//tests

//TestDeploy deploy chaincode example01
func TestDeploy(t *testing.T) {
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01"}, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	_, err := deploy(endorserServer, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("Deploy-error in deploy %s", err)
		chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestDeployBadArgs sets bad args on deploy. It should fail, and example02 should not be deployed
func TestDeployBadArgs(t *testing.T) {
	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b")}}}

	_, err := deploy(endorserServer, spec, nil)
	if err == nil {
		t.Fail()
		t.Log("DeployBadArgs-expected error in deploy but succeeded")
		chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestDeployBadPayload set payload to nil and do a deploy. It should fail and example02 should not be deployed
func TestDeployBadPayload(t *testing.T) {
	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	f := func(cds *pb.ChaincodeDeploymentSpec) {
		cds.CodePackage = nil
	}
	_, err := deploy(endorserServer, spec, f)
	if err == nil {
		t.Fail()
		t.Log("DeployBadPayload-expected error in deploy but succeeded")
		chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

//TestRedeploy - deploy two times, second time should fail but example02 should remain deployed
func TestRedeploy(t *testing.T) {

	//invalid arguments
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Path: "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02"}, CtorMsg: &pb.ChaincodeInput{Args: [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")}}}

	_, err := deploy(endorserServer, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("error in endorserServer.ProcessProposal %s", err)
		chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}

	//second time should not fail as we are just simulating
	_, err = deploy(endorserServer, spec, nil)
	if err != nil {
		t.Fail()
		t.Logf("error in endorserServer.ProcessProposal %s", err)
		chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
		return
	}
	chaincode.GetChain(chaincode.DefaultChain).Stop(context.Background(), &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec})
}

func TestMain(m *testing.M) {
	SetupTestConfig()
	testDBWrapper.CleanDB(nil)
	viper.Set("peer.fileSystemPath", filepath.Join(os.TempDir(), "hyperledger", "production"))
	lis, err := initPeer()
	if err != nil {
		os.Exit(-1)
		fmt.Printf("Could not initialize tests")
		finitPeer(lis)
		return
	}

	endorserServer = NewEndorserServer(nil)
	retVal := m.Run()

	finitPeer(lis)

	os.Exit(retVal)
}
