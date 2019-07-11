/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"math/rand"
	"net"
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/mocks/config"
	mscc "github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	deliverclient "github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	ledgermocks "github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/mocks/ccprovider"
	fakeconfig "github.com/hyperledger/fabric/core/peer/mocks"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	peergossip "github.com/hyperledger/fabric/peer/gossip"
	"github.com/hyperledger/fabric/peer/gossip/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockDeliveryClient struct {
}

func (ds *mockDeliveryClient) UpdateEndpoints(chainID string, _ deliverclient.ConnectionCriteria) error {
	return nil
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

func (*mockDeliveryClientFactory) Service(g service.GossipService, _ service.OrdererAddressConfig, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	return &mockDeliveryClient{}, nil
}

func TestNewPeerServer(t *testing.T) {
	server, err := NewPeerServer(":4050", comm.ServerConfig{})
	assert.NoError(t, err, "NewPeerServer returned unexpected error")
	assert.Equal(t, "[::]:4050", server.Address(), "NewPeerServer returned the wrong address")
	server.Stop()

	_, err = NewPeerServer("", comm.ServerConfig{})
	assert.Error(t, err, "expected NewPeerServer to return error with missing address")
}

func TestInitChain(t *testing.T) {
	chainId := "testChain"
	chainInitializer = func(cid string) {
		assert.Equal(t, chainId, cid, "chainInitializer received unexpected cid")
	}
	InitChain(chainId)
}

func TestInitialize(t *testing.T) {
	cleanup := setupPeerFS(t)
	defer cleanup()

	Initialize(nil, &ccprovider.MockCcProviderImpl{}, (&mscc.MocksccProviderFactory{}).NewSystemChaincodeProvider(), txvalidator.MapBasedPluginMapper(map[string]validation.PluginFactory{}), nil, &ledgermocks.DeployedChaincodeInfoProvider{}, nil, &disabled.Provider{})
}

func TestCreateChainFromBlock(t *testing.T) {
	cleanup := setupPeerFS(t)
	defer cleanup()

	Initialize(nil, &ccprovider.MockCcProviderImpl{}, (&mscc.MocksccProviderFactory{}).NewSystemChaincodeProvider(), txvalidator.MapBasedPluginMapper(map[string]validation.PluginFactory{}), &platforms.Registry{}, &ledgermocks.DeployedChaincodeInfoProvider{}, nil, &disabled.Provider{})
	testChainID := fmt.Sprintf("mytestchainid-%d", rand.Int())
	block, err := configtxtest.MakeGenesisBlock(testChainID)
	if err != nil {
		fmt.Printf("Failed to create a config block, err %s\n", err)
		t.FailNow()
	}

	// Initialize gossip service
	grpcServer := grpc.NewServer()
	socket, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

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
		identity, &disabled.Provider{}, socket.Addr().String(), grpcServer, nil, &mockDeliveryClientFactory{},
		messageCryptoService, secAdv, defaultSecureDialOpts)

	assert.NoError(t, err)

	go grpcServer.Serve(socket)
	defer grpcServer.Stop()

	err = CreateChainFromBlock(block, nil, nil)
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

	cfgSupport := configSupport{}
	chCfg := cfgSupport.GetChannelConfig(testChainID)
	assert.NotNil(t, chCfg, "failed to get channel config")

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

	SetCurrConfigBlock(block, testChainID)

	channels := GetChannelsInfo()
	if len(channels) != 1 {
		t.Fatalf("incorrect number of channels")
	}

	// cleanup the chain referenes to enable execution with -count n
	chains.Lock()
	chains.list = map[string]*chain{}
	chains.Unlock()
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	t.Log(ip)
}

func TestDeliverSupportManager(t *testing.T) {
	// reset chains for testing
	MockInitialize()

	manager := &DeliverChainManager{}
	chainSupport := manager.GetChain("fake")
	assert.Nil(t, chainSupport, "chain support should be nil")

	MockCreateChain("testchain")
	chainSupport = manager.GetChain("testchain")
	assert.NotNil(t, chainSupport, "chain support should not be nil")
}

func TestGossipChannelConfigOrdererAddressesByOrgs(t *testing.T) {
	ordererOrg1 := &fakeconfig.OrdererOrg{}
	ordererOrg2 := &fakeconfig.OrdererOrg{}

	ordererOrg1.On("MSPID").Return("Org1")
	ordererOrg2.On("MSPID").Return("Org2")
	ordererOrg1.On("Endpoints").Return([]string{"o1"})
	ordererOrg2.On("Endpoints").Return([]string{"o2"})

	gcp := &gossipChannelConfig{
		oc: &config.Orderer{
			OrganizationsVal: map[string]channelconfig.OrdererOrg{
				"Org1": ordererOrg1,
				"Org2": ordererOrg2,
			},
		},
	}

	assert.Equal(t, map[string][]string{
		"Org1": {"o1"},
		"Org2": {"o2"},
	}, gcp.OrdererAddressesByOrgs())
}
