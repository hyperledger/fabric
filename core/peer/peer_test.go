/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/channelconfig"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/deliverservice"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	ledgermocks "github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/gossip"
	gossipmetrics "github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/privdata"
	gossipservice "github.com/hyperledger/fabric/gossip/service"
	peergossip "github.com/hyperledger/fabric/internal/peer/gossip"
	"github.com/hyperledger/fabric/internal/peer/gossip/mocks"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestMain(m *testing.M) {
	msptesttools.LoadMSPSetupForTesting()
	rc := m.Run()
	os.Exit(rc)
}

func NewTestPeer(t *testing.T) (*Peer, func()) {
	tempdir := t.TempDir()

	// Initialize gossip service
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)
	signer, err := mgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	require.NoError(t, err)

	localMSP := mgmt.GetLocalMSP(cryptoProvider)
	deserManager := peergossip.NewDeserializersManager(localMSP)
	messageCryptoService := peergossip.NewMCS(
		&mocks.ChannelPolicyManagerGetter{},
		signer,
		deserManager,
		cryptoProvider,
		nil,
	)
	secAdv := peergossip.NewSecurityAdvisor(deserManager)
	defaultSecureDialOpts := func() []grpc.DialOption { return []grpc.DialOption{grpc.WithInsecure()} }
	gossipConfig, err := gossip.GlobalConfig("localhost:0", nil)
	require.NoError(t, err)

	gossipService, err := gossipservice.New(
		signer,
		gossipmetrics.NewGossipMetrics(&disabled.Provider{}),
		"localhost:0",
		grpc.NewServer(),
		messageCryptoService,
		secAdv,
		defaultSecureDialOpts,
		nil,
		gossipConfig,
		&gossipservice.ServiceConfig{},
		&privdata.PrivdataConfig{},
		&deliverservice.DeliverServiceConfig{
			ReConnectBackoffThreshold:   deliverservice.DefaultReConnectBackoffThreshold,
			ReconnectTotalTimeThreshold: deliverservice.DefaultReConnectTotalTimeThreshold,
		},
	)
	require.NoError(t, err, "failed to create gossip service")

	ledgerMgr, err := constructLedgerMgrWithTestDefaults(filepath.Join(tempdir, "ledgersData"))
	require.NoError(t, err, "failed to create ledger manager")

	require.NoError(t, err)
	transientStoreProvider, err := transientstore.NewStoreProvider(
		filepath.Join(tempdir, "transientstore"),
	)
	require.NoError(t, err)
	peerInstance := &Peer{
		GossipService:  gossipService,
		StoreProvider:  transientStoreProvider,
		LedgerMgr:      ledgerMgr,
		CryptoProvider: cryptoProvider,
	}

	cleanup := func() {
		ledgerMgr.Close()
	}
	return peerInstance, cleanup
}

func TestInitialize(t *testing.T) {
	peerInstance, cleanup := NewTestPeer(t)
	defer cleanup()

	org1CA, err := tlsgen.NewCA()
	require.NoError(t, err)
	org1Server1KeyPair, err := org1CA.NewServerCertKeyPair("localhost", "127.0.0.1", "::1")
	require.NoError(t, err)

	serverConfig := comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			UseTLS:            true,
			Certificate:       org1Server1KeyPair.Cert,
			Key:               org1Server1KeyPair.Key,
			ServerRootCAs:     [][]byte{org1CA.CertBytes()},
			RequireClientCert: true,
		},
	}

	server, err := comm.NewGRPCServer("localhost:0", serverConfig)
	require.NoError(t, err, "failed to create gRPC server")

	peerInstance.Initialize(
		nil,
		server,
		plugin.MapBasedMapper(map[string]validation.PluginFactory{}),
		&ledgermocks.DeployedChaincodeInfoProvider{},
		nil,
		nil,
		runtime.NumCPU(),
	)
	require.Equal(t, peerInstance.server, server)
}

func TestCreateChannel(t *testing.T) {
	peerInstance, cleanup := NewTestPeer(t)
	defer cleanup()

	var initArg string
	peerInstance.Initialize(
		func(cid string) { initArg = cid },
		nil,
		plugin.MapBasedMapper(map[string]validation.PluginFactory{}),
		&ledgermocks.DeployedChaincodeInfoProvider{},
		nil,
		nil,
		runtime.NumCPU(),
	)

	testChannelID := fmt.Sprintf("mytestchannelid-%d", rand.Int())
	block, err := configtxtest.MakeGenesisBlock(testChannelID)
	if err != nil {
		fmt.Printf("Failed to create a config block, err %s\n", err)
		t.FailNow()
	}

	err = peerInstance.CreateChannel(testChannelID, block, &ledgermocks.DeployedChaincodeInfoProvider{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create chain %s", err)
	}

	require.Equal(t, testChannelID, initArg)

	// Correct ledger
	ledger := peerInstance.GetLedger(testChannelID)
	if ledger == nil {
		t.Fatalf("failed to get correct ledger")
	}

	// Get config block from ledger
	block, err = ConfigBlockFromLedger(ledger)
	require.NoError(t, err, "Failed to get config block from ledger")
	require.NotNil(t, block, "Config block should not be nil")
	require.Equal(t, uint64(0), block.Header.Number, "config block should have been block 0")

	// Bad ledger
	ledger = peerInstance.GetLedger("BogusChain")
	if ledger != nil {
		t.Fatalf("got a bogus ledger")
	}

	// Correct PolicyManager
	pmgr := peerInstance.GetPolicyManager(testChannelID)
	if pmgr == nil {
		t.Fatal("failed to get PolicyManager")
	}

	// Bad PolicyManager
	pmgr = peerInstance.GetPolicyManager("BogusChain")
	if pmgr != nil {
		t.Fatal("got a bogus PolicyManager")
	}

	channels := peerInstance.GetChannelsInfo()
	if len(channels) != 1 {
		t.Fatalf("incorrect number of channels")
	}
}

func TestCreateChannelBySnapshot(t *testing.T) {
	peerInstance, cleanup := NewTestPeer(t)
	defer cleanup()

	var initArg string
	waitCh := make(chan struct{})
	peerInstance.Initialize(
		func(cid string) {
			<-waitCh
			initArg = cid
		},
		nil,
		plugin.MapBasedMapper(map[string]validation.PluginFactory{}),
		&ledgermocks.DeployedChaincodeInfoProvider{},
		nil,
		nil,
		runtime.NumCPU(),
	)

	testChannelID := "createchannelbysnapshot"

	// create a temp dir to store snapshot
	tempdir := t.TempDir()

	snapshotDir := ledgermgmttest.CreateSnapshotWithGenesisBlock(t, tempdir, testChannelID, &ConfigTxProcessor{})
	err := peerInstance.CreateChannelFromSnapshot(snapshotDir, &ledgermocks.DeployedChaincodeInfoProvider{}, nil, nil)
	require.NoError(t, err)

	expectedStatus := &pb.JoinBySnapshotStatus{InProgress: true, BootstrappingSnapshotDir: snapshotDir}
	require.Equal(t, expectedStatus, peerInstance.JoinBySnaphotStatus())

	// write a msg to waitCh to unblock channel init func
	waitCh <- struct{}{}

	// wait until ledger creation is done
	ledgerCreationDone := func() bool {
		return !peerInstance.JoinBySnaphotStatus().InProgress
	}
	require.Eventually(t, ledgerCreationDone, time.Minute, time.Second)

	// verify channel init func is called
	require.Equal(t, testChannelID, initArg)

	// verify ledger created
	ledger := peerInstance.GetLedger(testChannelID)
	require.NotNil(t, ledger)

	bcInfo, err := ledger.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, uint64(1), bcInfo.GetHeight())

	expectedStatus = &pb.JoinBySnapshotStatus{InProgress: false, BootstrappingSnapshotDir: ""}
	require.Equal(t, expectedStatus, peerInstance.JoinBySnaphotStatus())

	// Bad ledger
	ledger = peerInstance.GetLedger("BogusChain")
	require.Nil(t, ledger)

	// Correct PolicyManager
	pmgr := peerInstance.GetPolicyManager(testChannelID)
	require.NotNil(t, pmgr)

	// Bad PolicyManager
	pmgr = peerInstance.GetPolicyManager("BogusChain")
	require.Nil(t, pmgr)

	channels := peerInstance.GetChannelsInfo()
	require.Equal(t, 1, len(channels))
}

func TestDeliverSupportManager(t *testing.T) {
	peerInstance, cleanup := NewTestPeer(t)
	defer cleanup()

	manager := &DeliverChainManager{Peer: peerInstance}

	chainSupport := manager.GetChain("fake")
	require.Nil(t, chainSupport, "chain support should be nil")

	peerInstance.channels = map[string]*Channel{"testchain": {}}
	chainSupport = manager.GetChain("testchain")
	require.NotNil(t, chainSupport, "chain support should not be nil")
}

func TestConfigCallback(t *testing.T) {
	peerInstance, cleanup := NewTestPeer(t)
	defer cleanup()

	var callbackInvoked bool
	peerInstance.AddConfigCallbacks(func(bundle *channelconfig.Bundle) {
		callbackInvoked = true
		orderer, exists := bundle.OrdererConfig()
		require.True(t, exists)
		require.NotEmpty(t, orderer.Organizations()["SampleOrg"].Endpoints)
	})

	peerInstance.Initialize(
		nil,
		nil,
		plugin.MapBasedMapper(map[string]validation.PluginFactory{}),
		&ledgermocks.DeployedChaincodeInfoProvider{},
		nil,
		nil,
		runtime.NumCPU(),
	)

	testChannelID := fmt.Sprintf("mytestchannelid-%d", rand.Int())
	block, err := configtxtest.MakeGenesisBlock(testChannelID)
	require.NoError(t, err)

	// We expect the callback to be invoked when the channel is created
	require.False(t, callbackInvoked)

	err = peerInstance.CreateChannel(testChannelID, block, &ledgermocks.DeployedChaincodeInfoProvider{}, nil, nil)
	require.NoError(t, err)

	// the callback should have been invoked
	require.True(t, callbackInvoked)
}

func constructLedgerMgrWithTestDefaults(ledgersDataDir string) (*ledgermgmt.LedgerMgr, error) {
	ledgerInitializer := ledgermgmttest.NewInitializer(ledgersDataDir)

	ledgerInitializer.CustomTxProcessors = map[common.HeaderType]ledger.CustomTxProcessor{
		common.HeaderType_CONFIG: &ConfigTxProcessor{},
	}
	ledgerInitializer.Config.HistoryDBConfig = &ledger.HistoryDBConfig{
		Enabled: true,
	}
	return ledgermgmt.NewLedgerMgr(ledgerInitializer), nil
}

// SetServer sets the gRPC server for the peer.
// It should only be used in peer/pkg_test.
func (p *Peer) SetServer(server *comm.GRPCServer) {
	p.server = server
}
