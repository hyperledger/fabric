/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package service

import (
	"sync"
	"testing"
	"time"

	deliverclient "github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	transientstore2 "github.com/hyperledger/fabric/protos/transientstore"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type transientStoreMock struct {
}

func (*transientStoreMock) PurgeByHeight(maxBlockNumToRetain uint64) error {
	return nil
}

func (*transientStoreMock) Persist(txid string, blockHeight uint64, privateSimulationResults *rwset.TxPvtReadWriteSet) error {
	panic("implement me")
}

func (*transientStoreMock) PersistWithConfig(txid string, blockHeight uint64, privateSimulationResultsWithConfig *transientstore2.TxPvtReadWriteSetWithConfigInfo) error {
	panic("implement me")
}

func (*transientStoreMock) GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (transientstore.RWSetScanner, error) {
	panic("implement me")
}

func (*transientStoreMock) PurgeByTxids(txids []string) error {
	panic("implement me")
}

type embeddingDeliveryService struct {
	startOnce sync.Once
	stopOnce  sync.Once
	deliverclient.DeliverService
	startSignal sync.WaitGroup
	stopSignal  sync.WaitGroup
}

func newEmbeddingDeliveryService(ds deliverclient.DeliverService) *embeddingDeliveryService {
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

func (eds *embeddingDeliveryService) StopDeliverForChannel(chainID string) error {
	eds.stopOnce.Do(func() {
		eds.stopSignal.Done()
	})
	return eds.DeliverService.StopDeliverForChannel(chainID)
}

func (eds *embeddingDeliveryService) Stop() {
	eds.DeliverService.Stop()
}

type embeddingDeliveryServiceFactory struct {
	DeliveryServiceFactory
}

func (edsf *embeddingDeliveryServiceFactory) Service(g GossipService, ec OrdererAddressConfig, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	ds, _ := edsf.DeliveryServiceFactory.Service(g, ec, mcs)
	return newEmbeddingDeliveryService(ds), nil
}

func TestLeaderYield(t *testing.T) {
	// Scenario: Spawn 2 peers and wait for the first one to be the leader
	// There isn't any orderer present so the leader peer won't be able to
	// connect to the orderer, and should relinquish its leadership after a while.
	// Make sure the other peer declares itself as the leader soon after.
	takeOverMaxTimeout := time.Minute
	viper.Set("peer.gossip.election.leaderAliveThreshold", time.Second*5)
	// Test case has only two instance + making assertions only after membership view
	// is stable, hence election duration could be shorter
	viper.Set("peer.gossip.election.leaderElectionDuration", time.Millisecond*500)
	// It's enough to make single re-try
	viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", time.Second*1)
	// Since we ensuring gossip has stable membership, there is no need for
	// leader election to wait for stabilization
	viper.Set("peer.gossip.election.membershipSampleInterval", time.Millisecond*100)
	// There is no ordering service available anyway, hence connection timeout
	// could be shorter
	viper.Set("peer.deliveryclient.connTimeout", time.Millisecond*100)
	viper.Set("peer.gossip.useLeaderElection", true)
	viper.Set("peer.gossip.orgLeader", false)
	n := 2
	gossips := startPeers(t, n, 0, 1)
	defer stopPeers(gossips)
	channelName := "channelA"
	peerIndexes := []int{0, 1}
	// Add peers to the channel
	addPeersToChannel(t, n, channelName, gossips, peerIndexes)
	// Prime the membership view of the peers
	waitForFullMembership(t, gossips, n, time.Second*30, time.Millisecond*100)

	endpoint, socket := getAvailablePort(t)
	socket.Close()

	// Helper function that creates a gossipService instance
	newGossipService := func(i int) *gossipServiceImpl {
		gs := gossips[i].(*gossipGRPC).gossipServiceImpl
		gs.deliveryFactory = &embeddingDeliveryServiceFactory{&deliveryFactoryImpl{}}
		gossipServiceInstance = gs
		gs.InitializeChannel(channelName, OrdererAddressConfig{Addresses: []string{endpoint}}, Support{
			Committer: &mockLedgerInfo{1},
			Store:     &transientStoreMock{},
		})
		return gs
	}

	p0 := newGossipService(0)
	p1 := newGossipService(1)

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
	assert.Equal(t, 0, getLeader())
	// Wait for p0 to lose its leadership
	ds0.waitForDeliveryServiceTermination()
	t.Log("p0 stopped its delivery service")
	// Ensure p0 is not a leader
	assert.NotEqual(t, 0, getLeader())
	// Wait for p1 to take over. It should take over before time reaches timeLimit
	timeLimit := time.Now().Add(takeOverMaxTimeout)
	for getLeader() != 1 && time.Now().Before(timeLimit) {
		time.Sleep(100 * time.Millisecond)
	}
	if time.Now().After(timeLimit) {
		util.PrintStackTrace()
		t.Fatalf("p1 hasn't taken over leadership within %v: %d", takeOverMaxTimeout, getLeader())
	}
	t.Log("p1 has taken over leadership")
	p0.chains[channelName].Stop()
	p1.chains[channelName].Stop()
	p0.deliveryService[channelName].Stop()
	p1.deliveryService[channelName].Stop()
}
