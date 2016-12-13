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

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

func TestInit(t *testing.T) {
	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	if _, err := stub.MockInit("1", nil); err != nil {
		fmt.Println("Init failed", err)
		t.FailNow()
	}
}

func TestInvokeJoinChainMissingParams(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	// Failed path: Not enough parameters
	args := [][]byte{[]byte("JoinChain")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("cscc invoke JoinChain should have failed with invalid number of args: %v", args)
	}
}

func TestInvokeJoinChainWrongParams(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	// Failed path: wrong parameter type
	args := [][]byte{[]byte("JoinChain"), []byte("action")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		fmt.Println("Invoke", args, "failed", err)
		t.Fatalf("cscc invoke JoinChain should have failed with null genesis block.  args: %v", args)
	}
}

func TestInvokeJoinChainCorrectParams(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")

	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	// Initialize gossip service
	grpcServer := grpc.NewServer()
	socket, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 13611))
	assert.NoError(t, err)
	go grpcServer.Serve(socket)
	defer grpcServer.Stop()
	service.InitGossipService("localhost:13611", grpcServer)

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke JoinChain failed because invalid block")
	}
	args := [][]byte{[]byte("JoinChain"), blockBytes}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := getChainID(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke JoinChain failed with: %v", err)
	}
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", err)
	}
}

func TestInvokeUpdateConfigBlock(t *testing.T) {
	e := new(PeerConfiger)
	stub := shim.NewMockStub("PeerConfiger", e)

	// Failed path: Not enough parameters
	args := [][]byte{[]byte("UpdateConfigBlock")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		t.Fatalf("cscc invoke UpdateConfigBlock should have failed with invalid number of args: %v", args)
	}

	// Failed path: wrong parameter type
	args = [][]byte{[]byte("UpdateConfigBlock"), []byte("action")}
	if _, err := stub.MockInvoke("1", args); err == nil {
		fmt.Println("Invoke", args, "failed", err)
		t.Fatalf("cscc invoke UpdateConfigBlock should have failed with null genesis block - args: %v", args)
	}

	// Successful path for JoinChain
	blockBytes := mockConfigBlock()
	if blockBytes == nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed because invalid block")
	}
	args = [][]byte{[]byte("UpdateConfigBlock"), blockBytes}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed with: %v", err)
	}

	// Query the configuration block
	//chainID := []byte{143, 222, 22, 192, 73, 145, 76, 110, 167, 154, 118, 66, 132, 204, 113, 168}
	chainID, err := getChainID(blockBytes)
	if err != nil {
		t.Fatalf("cscc invoke UpdateConfigBlock failed with: %v", err)
	}
	args = [][]byte{[]byte("GetConfigBlock"), []byte(chainID)}
	if _, err := stub.MockInvoke("1", args); err != nil {
		t.Fatalf("cscc invoke GetConfigBlock failed with: %v", err)
	}

}

func mockConfigBlock() []byte {
	var blockBytes []byte
	block, err := utils.MakeConfigurationBlock("mytestchainid")
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
