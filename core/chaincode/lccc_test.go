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
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container"
	pb "github.com/hyperledger/fabric/protos/peer"
	"google.golang.org/grpc"
)

func register(stub *shim.MockStub, ccname string) error {
	args := [][]byte{[]byte("register"), []byte(ccname)}
	if _, err := stub.MockInvoke("1", args); err != nil {
		return err
	}
	return nil
}

func constructDeploymentSpec(name string, path string, initArgs [][]byte) (*pb.ChaincodeDeploymentSpec, error) {
	spec := &pb.ChaincodeSpec{Type: 1, ChaincodeID: &pb.ChaincodeID{Name: name, Path: path}, CtorMsg: &pb.ChaincodeInput{Args: initArgs}}
	codePackageBytes, err := container.GetChaincodePackageBytes(spec)
	if err != nil {
		return nil, err
	}
	chaincodeDeploymentSpec := &pb.ChaincodeDeploymentSpec{ChaincodeSpec: spec, CodePackage: codePackageBytes}
	return chaincodeDeploymentSpec, nil
}

func initialize() {
	//use a different address than what we usually use for "peer"
	//we override the peerAddress set in chaincode_support.go
	// FIXME: Use peer.GetLocalAddress()
	peerAddress := "0.0.0.0:21212"

	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "testpeer"}, Address: peerAddress}, nil
	}

	ccStartupTimeout := time.Duration(30000) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout))
}

//TestDeploy tests the deploy function (stops short of actually running the chaincode)
func TestDeploy(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}
}

//TestInvalidCodeDeploy tests the deploy function with invalid code package
func TestInvalidCodeDeploy(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	baddepspec := []byte("bad deploy spec")
	args := [][]byte{[]byte(DEPLOY), []byte("test"), baddepspec}
	_, err := stub.MockInvoke("1", args)
	if _, ok := err.(InvalidDeploymentSpecErr); !ok {
		t.FailNow()
	}
}

//TestInvalidChaincodeName tests the deploy function with invalid chaincode name
func TestInvalidChaincodeName(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})

	//change name to empty
	cds.ChaincodeSpec.ChaincodeID.Name = ""

	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	_, err = stub.MockInvoke("1", args)
	if _, ok := err.(InvalidChaincodeNameErr); !ok {
		t.FailNow()
	}
}

//TestRedeploy tests the redeploying will fail function(and fail with "exists" error)
func TestRedeploy(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}

	//this should fail with exists error
	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	_, err = stub.MockInvoke("1", args)
	if _, ok := err.(ChaincodeExistsErr); !ok {
		t.FailNow()
	}
}

//TestCheckCC invokes the GETCCINFO function to get status of deployed chaincode
func TestCheckCC(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}
}

//TestMultipleDeploy tests deploying multiple chaincodes
func TestMultipleDeploy(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args := [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}

	//deploy 01
	cds, err = constructDeploymentSpec("example01", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example01", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}

	args = [][]byte{[]byte(GETCCINFO), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}
}

//TestRetryFailedDeploy tests re-deploying after a failure
func TestRetryFailedDeploy(t *testing.T) {
	initialize()

	scc := new(LifeCycleSysCC)
	stub := shim.NewMockStub("lccc", scc)

	//deploy 02
	cds, err := constructDeploymentSpec("example02", "github.com/hyperledger/fabric/examples/chaincode/go/chaincode_example02", [][]byte{[]byte("init"), []byte("a"), []byte("100"), []byte("b"), []byte("200")})
	var b []byte
	if b, err = proto.Marshal(cds); err != nil || b == nil {
		t.FailNow()
	}

	//send invalid chain name name that should fail
	args := [][]byte{[]byte(DEPLOY), []byte(""), b}
	if _, err = stub.MockInvoke("1", args); err == nil {
		//expected error but got success
		t.FailNow()
	}

	if _, ok := err.(InvalidChainNameErr); !ok {
		//expected invalid chain name
		t.FailNow()
	}

	//deploy correctly now
	args = [][]byte{[]byte(DEPLOY), []byte("test"), b}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.FailNow()
	}

	//get the deploymentspec
	args = [][]byte{[]byte(GETDEPSPEC), []byte("test"), []byte(cds.ChaincodeSpec.ChaincodeID.Name)}
	if depspec, err := stub.MockInvoke("1", args); err != nil || depspec == nil {
		t.FailNow()
	}
}
