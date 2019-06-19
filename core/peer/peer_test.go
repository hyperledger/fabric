/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	mscc "github.com/hyperledger/fabric/common/mocks/scc"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/mock"
	ledgermocks "github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/transientstore"
	gossipmetrics "github.com/hyperledger/fabric/gossip/metrics"
	gossipservice "github.com/hyperledger/fabric/gossip/service"
	peergossip "github.com/hyperledger/fabric/internal/peer/gossip"
	"github.com/hyperledger/fabric/internal/peer/gossip/mocks"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestMain(m *testing.M) {
	// TODO: remove the transient store and peer setup once we've completed the
	// transition to instances
	tempdir, err := ioutil.TempDir("", "scc-configure")
	if err != nil {
		panic(err)
	}

	msptesttools.LoadMSPSetupForTesting()

	// Initialize gossip service
	signer := mgmt.GetLocalSigningIdentityOrPanic()
	messageCryptoService := peergossip.NewMCS(&mocks.ChannelPolicyManagerGetter{}, signer, mgmt.NewDeserializersManager())
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	defaultSecureDialOpts := func() []grpc.DialOption { return []grpc.DialOption{grpc.WithInsecure()} }

	gossipService, err := gossipservice.New(
		signer,
		gossipmetrics.NewGossipMetrics(&disabled.Provider{}),
		"localhost:0",
		grpc.NewServer(),
		nil,
		messageCryptoService,
		secAdv,
		defaultSecureDialOpts,
		nil,
	)
	if err != nil {
		panic(err)
	}

	Default = &Peer{
		StoreProvider: transientstore.NewStoreProvider(
			filepath.Join(tempdir, "transientstore"),
		),
		GossipService: gossipService,
	}

	rc := m.Run()

	os.RemoveAll(tempdir)
	os.Exit(rc)
}

func TestInitialize(t *testing.T) {
	rootFSPath, err := ioutil.TempDir("", "ledgersData")
	if err != nil {
		t.Fatalf("Failed to create ledger directory: %s", err)
	}
	defer os.RemoveAll(rootFSPath)

	Default.Initialize(
		nil,
		(&mscc.MocksccProviderFactory{}).NewSystemChaincodeProvider(),
		plugin.MapBasedMapper(map[string]validation.PluginFactory{}),
		&ledgermocks.DeployedChaincodeInfoProvider{},
		nil,
		nil,
		nil,
		runtime.NumCPU(),
	)
}

func TestCreateChannel(t *testing.T) {
	peerFSPath, err := ioutil.TempDir("", "ledgersData")
	if err != nil {
		t.Fatalf("Failed to create peer directory: %s", err)
	}
	defer os.RemoveAll(peerFSPath)
	viper.Set("peer.fileSystemPath", peerFSPath)

	ledgermgmt.Initialize(&ledgermgmt.Initializer{
		CustomTxProcessors:            nil,
		PlatformRegistry:              &platforms.Registry{},
		DeployedChaincodeInfoProvider: &ledgermocks.DeployedChaincodeInfoProvider{},
		MembershipInfoProvider:        nil,
		MetricsProvider:               &disabled.Provider{},
		Config: &ledger.Config{
			RootFSPath:    filepath.Join(peerFSPath, "ledgersData"),
			StateDBConfig: &ledger.StateDBConfig{},
			PrivateDataConfig: &ledger.PrivateDataConfig{
				MaxBatchSize:    5000,
				BatchesInterval: 1000,
				PurgeInterval:   100,
			},
			HistoryDBConfig: &ledger.HistoryDBConfig{
				Enabled: true,
			},
		},
	})

	var initArg string
	Default.Initialize(
		func(cid string) { initArg = cid },
		(&mscc.MocksccProviderFactory{}).NewSystemChaincodeProvider(),
		plugin.MapBasedMapper(map[string]validation.PluginFactory{}),
		&ledgermocks.DeployedChaincodeInfoProvider{},
		nil,
		nil,
		nil,
		runtime.NumCPU(),
	)

	testChainID := fmt.Sprintf("mytestchainid-%d", rand.Int())
	block, err := configtxtest.MakeGenesisBlock(testChainID)
	if err != nil {
		fmt.Printf("Failed to create a config block, err %s\n", err)
		t.FailNow()
	}

	err = Default.CreateChannel(block, nil, &mock.DeployedChaincodeInfoProvider{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create chain %s", err)
	}

	assert.Equal(t, testChainID, initArg)

	// Correct ledger
	ledger := Default.GetLedger(testChainID)
	if ledger == nil {
		t.Fatalf("failed to get correct ledger")
	}

	// Get config block from ledger
	block, err = getCurrConfigBlockFromLedger(ledger)
	assert.NoError(t, err, "Failed to get config block from ledger")
	assert.NotNil(t, block, "Config block should not be nil")
	assert.Equal(t, uint64(0), block.Header.Number, "config block should have been block 0")

	// Bad ledger
	ledger = Default.GetLedger("BogusChain")
	if ledger != nil {
		t.Fatalf("got a bogus ledger")
	}

	// Correct block
	block = Default.GetCurrConfigBlock(testChainID)
	if block == nil {
		t.Fatalf("failed to get correct block")
	}

	cfgSupport := configSupport{}
	chCfg := cfgSupport.GetChannelConfig(testChainID)
	assert.NotNil(t, chCfg, "failed to get channel config")

	// Bad block
	block = Default.GetCurrConfigBlock("BogusBlock")
	if block != nil {
		t.Fatalf("got a bogus block")
	}

	// Correct PolicyManager
	pmgr := Default.GetPolicyManager(testChainID)
	if pmgr == nil {
		t.Fatal("failed to get PolicyManager")
	}

	// Bad PolicyManager
	pmgr = Default.GetPolicyManager("BogusChain")
	if pmgr != nil {
		t.Fatal("got a bogus PolicyManager")
	}

	channels := Default.GetChannelsInfo()
	if len(channels) != 1 {
		t.Fatalf("incorrect number of channels")
	}

	// cleanup the chain referenes to enable execution with -count n
	Default.mutex.Lock()
	Default.channels = map[string]*Channel{}
	Default.mutex.Unlock()
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
