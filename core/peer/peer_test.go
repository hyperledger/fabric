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
	"path/filepath"
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/localmsp"
	mscc "github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/core/comm"
	ccp "github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/msp/mgmt/testtools"
	peergossip "github.com/hyperledger/fabric/peer/gossip"
	"github.com/hyperledger/fabric/peer/gossip/mocks"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

type mockDeliveryClient struct {
}

// StartDeliverForChannel dynamically starts delivery of new blocks from ordering service
// to channel peers.
func (ds *mockDeliveryClient) StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, f func()) error {
	return nil
}

// StopDeliverForChannel dynamically stops delivery of new blocks from ordering service
// to channel peers.
func (ds *mockDeliveryClient) StopDeliverForChannel(chainID string) error {
	return nil
}

// Stop terminates delivery service and closes the connection
func (*mockDeliveryClient) Stop() {

}

type mockDeliveryClientFactory struct {
}

func (*mockDeliveryClientFactory) Service(g service.GossipService, endpoints []string, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	return &mockDeliveryClient{}, nil
}

func TestCreatePeerServer(t *testing.T) {

	server, err := CreatePeerServer(":4050", comm.SecureServerConfig{})
	assert.NoError(t, err, "CreatePeerServer returned unexpected error")
	assert.Equal(t, "[::]:4050", server.Address(),
		"CreatePeerServer returned the wrong address")
	server.Stop()

	_, err = CreatePeerServer("", comm.SecureServerConfig{})
	assert.Error(t, err, "expected CreatePeerServer to return error with missing address")

}

func TestGetSecureConfig(t *testing.T) {

	// good config without TLS
	viper.Set("peer.tls.enabled", false)
	sc, _ := GetSecureConfig()
	assert.Equal(t, false, sc.UseTLS, "SecureConfig.UseTLS should be false")

	// good config with TLS
	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.tls.cert.file", filepath.Join("testdata", "Org1-server1-cert.pem"))
	viper.Set("peer.tls.key.file", filepath.Join("testdata", "Org1-server1-key.pem"))
	viper.Set("peer.tls.rootcert.file", filepath.Join("testdata", "Org1-cert.pem"))
	sc, _ = GetSecureConfig()
	assert.Equal(t, true, sc.UseTLS, "SecureConfig.UseTLS should be true")

	// bad config with TLS
	viper.Set("peer.tls.rootcert.file", filepath.Join("testdata", "Org11-cert.pem"))
	_, err := GetSecureConfig()
	assert.Error(t, err, "GetSecureConfig should return error with bad root cert path")
	viper.Set("peer.tls.cert.file", filepath.Join("testdata", "Org11-cert.pem"))
	_, err = GetSecureConfig()
	assert.Error(t, err, "GetSecureConfig should return error with bad tls cert path")

	// disable TLS for remaining tests
	viper.Set("peer.tls.enabled", false)

}

func TestInitChain(t *testing.T) {

	chainId := "testChain"
	chainInitializer = func(cid string) {
		assert.Equal(t, chainId, cid, "chainInitializer received unexpected cid")
	}
	InitChain(chainId)
}

func TestInitialize(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/var/hyperledger/test/")

	// we mock this because we can't import the chaincode package lest we create an import cycle
	ccp.RegisterChaincodeProviderFactory(&ccprovider.MockCcProviderFactory{})
	sysccprovider.RegisterSystemChaincodeProviderFactory(&mscc.MocksccProviderFactory{})

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

	msptesttools.LoadMSPSetupForTesting()

	identity, _ := mgmt.GetLocalSigningIdentityOrPanic().Serialize()
	messageCryptoService := peergossip.NewMCS(&mocks.ChannelPolicyManagerGetter{}, localmsp.NewSigner(), mgmt.NewDeserializersManager())
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	var defaultSecureDialOpts = func() []grpc.DialOption {
		var dialOpts []grpc.DialOption
		dialOpts = append(dialOpts, grpc.WithInsecure())
		return dialOpts
	}
	err = service.InitGossipServiceCustomDeliveryFactory(
		identity, "localhost:13611", grpcServer,
		&mockDeliveryClientFactory{},
		messageCryptoService, secAdv, defaultSecureDialOpts)

	assert.NoError(t, err)

	err = CreateChainFromBlock(block)
	if err != nil {
		t.Fatalf("failed to create chain %s", err)
	}

	// Correct ledger
	ledger := GetLedger(testChainID)
	if ledger == nil {
		t.Fatalf("failed to get correct ledger")
	}

	// Get config block from ledger
	block, err = getCurrConfigBlockFromLedger(ledger)
	assert.NoError(t, err, "Failed to get config block from ledger")
	assert.NotNil(t, block, "Config block should not be nil")
	assert.Equal(t, uint64(0), block.Header.Number, "config block should have been block 0")

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

	// Correct PolicyManager
	pmgr := GetPolicyManager(testChainID)
	if pmgr == nil {
		t.Fatal("failed to get PolicyManager")
	}

	// Bad PolicyManager
	pmgr = GetPolicyManager("BogusChain")
	if pmgr != nil {
		t.Fatal("got a bogus PolicyManager")
	}

	// PolicyManagerGetter
	pmg := NewChannelPolicyManagerGetter()
	assert.NotNil(t, pmg, "PolicyManagerGetter should not be nil")

	pmgr, ok := pmg.Manager(testChainID)
	assert.NotNil(t, pmgr, "PolicyManager should not be nil")
	assert.Equal(t, true, ok, "expected Manage() to return true")

	// Chaos monkey test
	Initialize(nil)

	SetCurrConfigBlock(block, testChainID)

	channels := GetChannelsInfo()
	if len(channels) != 1 {
		t.Fatalf("incorrect number of channels")
	}
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
