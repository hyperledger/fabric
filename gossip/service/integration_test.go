/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package service

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/election"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/peer/blocksprovider"
	"github.com/hyperledger/fabric/internal/pkg/peer/orderers"
	"github.com/stretchr/testify/require"
)

type embeddingDeliveryService struct {
	startOnce sync.Once
	stopOnce  sync.Once
	deliverservice.DeliverService
	startSignal sync.WaitGroup
	stopSignal  sync.WaitGroup
}

func newEmbeddingDeliveryService(ds deliverservice.DeliverService) *embeddingDeliveryService {
	eds := &embeddingDeliveryService{
		DeliverService: ds,
	}
	eds.startSignal.Add(1)
	eds.stopSignal.Add(1)
	return eds
}

func (eds *embeddingDeliveryService) waitForDeliveryServiceActivation() {
	eds.startSignal.Wait()
}

func (eds *embeddingDeliveryService) waitForDeliveryServiceTermination() {
	eds.stopSignal.Wait()
}

func (eds *embeddingDeliveryService) StartDeliverForChannel(chainID string, ledgerInfo blocksprovider.LedgerInfo, finalizer func()) error {
	eds.startOnce.Do(func() {
		eds.startSignal.Done()
	})
	return eds.DeliverService.StartDeliverForChannel(chainID, ledgerInfo, finalizer)
}

func (eds *embeddingDeliveryService) StopDeliverForChannel() error {
	eds.stopOnce.Do(func() {
		eds.stopSignal.Done()
	})
	return eds.DeliverService.StopDeliverForChannel()
}

func (eds *embeddingDeliveryService) Stop() {
	eds.DeliverService.Stop()
}

type embeddingDeliveryServiceFactory struct {
	DeliveryServiceFactory
}

func (edsf *embeddingDeliveryServiceFactory) Service(g GossipServiceAdapter, ordererSource *orderers.ConnectionSource, mcs api.MessageCryptoService, isStaticLead bool, channelConfig *common.Config, cryptoProvider bccsp.BCCSP) deliverservice.DeliverService {
	ds := edsf.DeliveryServiceFactory.Service(g, ordererSource, mcs, false, channelConfig, cryptoProvider)
	return newEmbeddingDeliveryService(ds)
}

func TestLeaderYield(t *testing.T) {
	// Scenario: Spawn 2 peers and wait for the first one to be the leader
	// There isn't any orderer present so the leader peer won't be able to
	// connect to the orderer, and should relinquish its leadership after a while.
	// Make sure the other peer declares itself as the leader soon after.
	takeOverMaxTimeout := time.Minute
	// It's enough to make single re-try
	// There is no ordering service available anyway, hence connection timeout
	// could be shorter
	serviceConfig := &ServiceConfig{
		UseLeaderElection:          true,
		OrgLeader:                  false,
		ElectionStartupGracePeriod: election.DefStartupGracePeriod,
		// Since we ensuring gossip has stable membership, there is no need for
		// leader election to wait for stabilization
		ElectionMembershipSampleInterval: time.Millisecond * 100,
		ElectionLeaderAliveThreshold:     time.Second * 5,
		// Test case has only two instance + making assertions only after membership view
		// is stable, hence election duration could be shorter
		ElectionLeaderElectionDuration: time.Millisecond * 500,
	}
	n := 2
	gossips := startPeers(serviceConfig, n, 0, 1)
	defer stopPeers(gossips)
	channelName := "test-channel"
	peerIndexes := []int{0, 1}
	// Add peers to the channel
	addPeersToChannel(channelName, gossips, peerIndexes)
	// Prime the membership view of the peers
	waitForFullMembershipOrFailNow(t, channelName, gossips, n, TIMEOUT, time.Millisecond*100)

	store := newTransientStore(t)
	defer store.tearDown()

	confAppRaft := genesisconfig.Load(genesisconfig.SampleAppChannelEtcdRaftProfile, configtest.GetDevConfigDir())
	certDir := t.TempDir()
	tlsCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	generateCertificates(t, confAppRaft, tlsCA, certDir)
	bootstrapper, err := encoder.NewBootstrapper(confAppRaft)
	require.NoError(t, err)
	channelConfigProto := &common.Config{ChannelGroup: bootstrapper.GenesisChannelGroup()}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	bundle, err := channelconfig.NewBundle(channelName, channelConfigProto, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, bundle)

	// Helper function that creates a gossipService instance
	newGossipService := func(i int) *GossipService {
		gs := gossips[i].GossipService
		gs.deliveryFactory = &embeddingDeliveryServiceFactory{
			&deliveryFactoryImpl{
				credentialSupport: comm.NewCredentialSupport(),
				deliverServiceConfig: &deliverservice.DeliverServiceConfig{
					PeerTLSEnabled:              false,
					ReConnectBackoffThreshold:   deliverservice.DefaultReConnectBackoffThreshold,
					ReconnectTotalTimeThreshold: time.Second,
					ConnectionTimeout:           time.Millisecond * 100,
				},
			},
		}

		gs.InitializeChannel(
			channelName,
			orderers.NewConnectionSource(flogging.MustGetLogger("peer.orderers"), nil),
			store.Store,
			Support{
				Committer: &mockLedgerInfo{1},
			},
			channelConfigProto,
			cryptoProvider,
		)
		return gs
	}

	// The first leader is determined by the peer with the lower PKIid (lower TCP port in this case).
	// We set p0 to be the peer with the lower PKIid to ensure it'll be elected as leader before p1 and spare time.
	pkiID0 := gossips[0].peerIdentity
	pkiID1 := gossips[1].peerIdentity
	var firstLeaderIdx, secondLeaderIdx int
	if bytes.Compare(pkiID0, pkiID1) < 0 {
		firstLeaderIdx = 0
		secondLeaderIdx = 1
	} else {
		firstLeaderIdx = 1
		secondLeaderIdx = 0
	}
	p0 := newGossipService(firstLeaderIdx)
	p1 := newGossipService(secondLeaderIdx)

	// Returns index of the leader or -1 if no leader elected
	getLeader := func() int {
		p0.lock.RLock()
		p1.lock.RLock()
		defer p0.lock.RUnlock()
		defer p1.lock.RUnlock()

		if p0.leaderElection[channelName].IsLeader() {
			return 0
		}
		if p1.leaderElection[channelName].IsLeader() {
			return 1
		}
		return -1
	}

	ds0 := p0.deliveryService[channelName].(*embeddingDeliveryService)

	// Wait for p0 to connect to the ordering service
	ds0.waitForDeliveryServiceActivation()
	t.Log("p0 started its delivery service")
	// Ensure it's a leader
	require.Equal(t, 0, getLeader())
	// Wait for p0 to lose its leadership
	ds0.waitForDeliveryServiceTermination()
	t.Log("p0 stopped its delivery service")
	// Ensure p0 is not a leader
	require.NotEqual(t, 0, getLeader())
	// Wait for p1 to take over. It should take over before time reaches timeLimit
	timeLimit := time.Now().Add(takeOverMaxTimeout)
	for getLeader() != 1 && time.Now().Before(timeLimit) {
		time.Sleep(100 * time.Millisecond)
	}
	if time.Now().After(timeLimit) && getLeader() != 1 {
		util.PrintStackTrace()
		t.Fatalf("p1 hasn't taken over leadership within %v: %d", takeOverMaxTimeout, getLeader())
	}
	t.Log("p1 has taken over leadership")
	p0.chains[channelName].Stop()
	p1.chains[channelName].Stop()
	p0.deliveryService[channelName].Stop()
	p1.deliveryService[channelName].Stop()
}

// TODO this pattern repeats itself in several places. Make it common in the 'genesisconfig' package to easily create
// Raft genesis blocks
func generateCertificates(t *testing.T, confAppRaft *genesisconfig.Profile, tlsCA tlsgen.CA, certDir string) {
	for i, c := range confAppRaft.Orderer.EtcdRaft.Consenters {
		srvC, err := tlsCA.NewServerCertKeyPair(c.Host)
		require.NoError(t, err)
		srvP := path.Join(certDir, fmt.Sprintf("server%d.crt", i))
		err = os.WriteFile(srvP, srvC.Cert, 0o644)
		require.NoError(t, err)

		clnC, err := tlsCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clnP := path.Join(certDir, fmt.Sprintf("client%d.crt", i))
		err = os.WriteFile(clnP, clnC.Cert, 0o644)
		require.NoError(t, err)

		c.ServerTlsCert = []byte(srvP)
		c.ClientTlsCert = []byte(clnP)
	}
}
