/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/core/deliverservice/blocksprovider"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/election"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/state"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type embeddingDeliveryService struct {
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
	eds.startSignal.Done()
	return eds.DeliverService.StartDeliverForChannel(chainID, ledgerInfo, finalizer)
}

func (eds *embeddingDeliveryService) StopDeliverForChannel(chainID string) error {
	eds.stopSignal.Done()
	return eds.DeliverService.StopDeliverForChannel(chainID)
}

func (eds *embeddingDeliveryService) Stop() {
	eds.DeliverService.Stop()
}

type embeddingDeliveryServiceFactory struct {
	DeliveryServiceFactory
}

func (edsf *embeddingDeliveryServiceFactory) Service(g GossipService, endpoints []string, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	ds, _ := edsf.DeliveryServiceFactory.Service(g, endpoints, mcs)
	return newEmbeddingDeliveryService(ds), nil
}

func TestLeaderYield(t *testing.T) {
	// Scenario: Spawn 2 peers and wait for the first one to be the leader
	// There isn't any orderer present so the leader peer won't be able to
	// connect to the orderer, and should relinquish its leadership after a while.
	// Make sure the other peer declares itself as the leader soon after.
	deliverclient.SetReconnectTotalTimeThreshold(time.Second * 5)
	viper.Set("peer.gossip.useLeaderElection", true)
	viper.Set("peer.gossip.orgLeader", false)
	n := 2
	portPrefix := 30000
	gossips := startPeers(t, n, portPrefix)
	defer stopPeers(gossips)
	channelName := "channelA"
	peerIndexes := []int{0, 1}
	// Add peers to the channel
	addPeersToChannel(t, n, portPrefix, channelName, gossips, peerIndexes)
	// Prime the membership view of the peers
	waitForFullMembership(t, gossips, n, time.Second*30, time.Second*2)
	mcs := &naiveCryptoService{}
	// Helper function that creates a gossipService instance
	newGossipService := func(i int) *gossipServiceImpl {
		peerIdentity := api.PeerIdentityType(fmt.Sprintf("localhost:%d", portPrefix+i))
		gs := &gossipServiceImpl{
			mcs:             mcs,
			gossipSvc:       gossips[i],
			chains:          make(map[string]state.GossipStateProvider),
			leaderElection:  make(map[string]election.LeaderElectionService),
			deliveryFactory: &embeddingDeliveryServiceFactory{&deliveryFactoryImpl{}},
			idMapper:        identity.NewIdentityMapper(mcs, peerIdentity),
			peerIdentity:    peerIdentity,
			secAdv:          &secAdvMock{},
		}
		gossipServiceInstance = gs
		gs.InitializeChannel(channelName, &mockLedgerInfo{1}, []string{"localhost:7050"})
		return gs
	}

	p0 := newGossipService(0)
	p1 := newGossipService(1)

	// Returns index of the leader or -1 if no leader elected
	getLeader := func() int {
		if p0.leaderElection[channelName].IsLeader() {
			// Ensure p1 isn't a leader at the same time
			assert.False(t, p1.leaderElection[channelName].IsLeader())
			return 0
		}
		if p1.leaderElection[channelName].IsLeader() {
			return 1
		}
		return -1
	}

	ds0 := p0.deliveryService.(*embeddingDeliveryService)
	ds1 := p1.deliveryService.(*embeddingDeliveryService)

	// Wait for p0 to connect to the ordering service
	ds0.waitForDeliveryServiceActivation()
	t.Log("p0 started its delivery service")
	// Ensure it's a leader
	assert.Equal(t, 0, getLeader())
	// Wait for p0 to lose its leadership
	ds0.waitForDeliveryServiceTermination()
	t.Log("p0 stopped its delivery service")
	// Ensure there is no leader
	assert.Equal(t, -1, getLeader())
	// Wait for p1 to take over
	ds1.waitForDeliveryServiceActivation()
	t.Log("p1 started its delivery service")
	// Ensure it's a leader now
	assert.Equal(t, 1, getLeader())
	p0.chains[channelName].Stop()
	p1.chains[channelName].Stop()
	p0.deliveryService.Stop()
	p1.deliveryService.Stop()
}
