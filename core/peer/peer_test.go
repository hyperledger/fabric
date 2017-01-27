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

package peer

import (
	"fmt"
	"net"
	"os"
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
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

func TestInitialize(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")

	// we mock this because we can't import the chaincode package lest we create an import cycle
	ccp.RegisterChaincodeProviderFactory(&ccprovider.MockCcProviderFactory{})

	Initialize(nil)
}

func TestCreateChainFromBlock(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")
	defer os.RemoveAll("/var/hyperledger/test/")
	testChainID := "mytestchainid"
	block, err := configtxtest.MakeGenesisBlock(testChainID)
	if err != nil {
		fmt.Printf("Failed to create a config block, err %s\n", err)
		t.FailNow()
	}

	// Initialize gossip service
	grpcServer := grpc.NewServer()
	socket, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "", 13611))
	assert.NoError(t, err)
	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	mgmt.LoadFakeSetupWithLocalMspAndTestChainMsp("../../msp/sampleconfig")

	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()
	service.InitGossipServiceCustomDeliveryFactory(identity, "localhost:13611", grpcServer, &mockDeliveryClientFactory{})

	err = CreateChainFromBlock(block)
	if err != nil {
		t.Fatalf("failed to create chain %s", err)
	}

	// Correct ledger
	ledger := GetLedger(testChainID)
	if ledger == nil {
		t.Fatalf("failed to get correct ledger")
	}

	// Bad ledger
	ledger = GetLedger("BogusChain")
	if ledger != nil {
		t.Fatalf("got a bogus ledger")
	}

	// Correct block
	block = GetCurrConfigBlock(testChainID)
	if block == nil {
		t.Fatalf("failed to get correct block")
	}

	// Bad block
	block = GetCurrConfigBlock("BogusBlock")
	if block != nil {
		t.Fatalf("got a bogus block")
	}

	// Chaos monkey test
	Initialize(nil)

	SetCurrConfigBlock(block, testChainID)
}

func TestNewPeerClientConnection(t *testing.T) {
	if _, err := NewPeerClientConnection(); err != nil {
		t.Log(err)
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	t.Log(ip)
}
