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
package cscc

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type mockDeliveryClient struct {
}

// JoinChain once peer joins the chain it should need to check whenever
// it has been selected as a leader and open connection to the configured
// ordering service endpoint
func (*mockDeliveryClient) JoinChain(chainID string, ledgerInfo blocksprovider.LedgerInfo) error {
	return nil
}

// Stop terminates delivery service and closes the connection
func (*mockDeliveryClient) Stop() {

}

type mockDeliveryClientFactory struct {
}

func (*mockDeliveryClientFactory) Service(g service.GossipService) (deliverclient.DeliverService, error) {
	return &mockDeliveryClient{}, nil
}

func TestConfigerInit(t *testing.T) {
	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	if res := stub.MockInit("1", nil); res.Status != shim.OK {
		fmt.Println("Init failed", string(res.Message))
		t.FailNow()
	}
}

func setupEndpoint(t *testing.T) {
	peerAddress := peer.GetLocalIP()
	if peerAddress == "" {
		peerAddress = "0.0.0.0"
	}
	peerAddress = peerAddress + ":21213"
	t.Logf("Local peer IP address: %s", peerAddress)
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	getPeerEndpoint := func() (*pb.PeerEndpoint, error) {
		return &pb.PeerEndpoint{ID: &pb.PeerID{Name: "cscctestpeer"}, Address: peerAddress}, nil
	}
	ccStartupTimeout := time.Duration(30000) * time.Millisecond
	pb.RegisterChaincodeSupportServer(grpcServer, chaincode.NewChaincodeSupport(getPeerEndpoint, false, ccStartupTimeout))
}

func TestConfigerInvokeJoinChainMissingParams(t *testing.T) {
	//t.Skip("Test CI build")
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	setupEndpoint(t)
	// Failed path: Not enough parameters
	args := [][]byte{[]byte("JoinChain")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with invalid number of args: %v", args)
	}
}

func TestConfigerInvokeJoinChainWrongParams(t *testing.T) {
	//t.Skip("Test CI build")
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	setupEndpoint(t)

	// Failed path: wrong parameter type
	args := [][]byte{[]byte("JoinChain"), []byte("action")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke JoinChain should have failed with null genesis block.  args: %v", args)
	}
}

func TestConfigerInvokeJoinChainCorrectParams(t *testing.T) {
	//t.Skip("Test CI build")
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	peer.MockInitialize()
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	defer os.RemoveAll("/var/hyperledger/test/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	setupEndpoint(t)

	// Initialize gossip service
	grpcServer := grpc.NewServer()
	socket, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 13611))
	assert.NoError(t, err)
	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	mgmt.LoadFakeSetupWithLocalMspAndTestChainMsp("../../../msp/sampleconfig")
	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()

	service.InitGossipServiceCustomDeliveryFactory(identity, "localhost:13611", grpcServer, &mockDeliveryClientFactory{})

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke JoinChain failed because invalid block")
	}
	args := [][]byte{[]byte("JoinChain"), blockBytes}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := getChainID(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", err)
	}
}

func TestConfigerInvokeUpdateConfigBlock(t *testing.T) {
	//t.Skip("Test CI build")
	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	setupEndpoint(t)

	// Failed path: Not enough parameters
	args := [][]byte{[]byte("UpdateConfigBlock")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke UpdateConfigBlock should have failed with invalid number of args: %v", args)
	}

	// Failed path: wrong parameter type
	args = [][]byte{[]byte("UpdateConfigBlock"), []byte("action")}
	if res := stub.MockInvoke("1", args); res.Status == shim.OK {
		t.Fatalf("cscc invoke UpdateConfigBlock should have failed with null genesis block - args: %v", args)
	}

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed because invalid block")
	}
	args = [][]byte{[]byte("UpdateConfigBlock"), blockBytes}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("cscc invoke UpdateConfigBlock failed with: %v", res.Message)
	}

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := getChainID(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed with: %v", err)
	}
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	if res := stub.MockInvoke("1", args); res.Status != shim.OK {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", err)
	}

}

func mockConfigBlock() []byte {
	var blockBytes []byte
	block, err := configtxtest.MakeGenesisBlock("mytestchainid")
	if err != nil {
		blockBytes = nil
	} else {
		blockBytes = utils.MarshalOrPanic(block)
	}
	return blockBytes
}

func getChainID(blockBytes []byte) (string, error) {
	block := &common.Block{}
	if err := proto.Unmarshal(blockBytes, block); err != nil {
		return "", err
	}
	envelope := &common.Envelope{}
	if err := proto.Unmarshal(block.Data.Data[0], envelope); err != nil {
		return "", err
	}
	payload := &common.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return "", err
	}
	fmt.Printf("chain id: %v\n", payload.Header.ChainHeader.ChainID)
	return payload.Header.ChainHeader.ChainID, nil
}
