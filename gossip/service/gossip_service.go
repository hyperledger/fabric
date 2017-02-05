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

package service

import (
	"sync"

	peerComm "github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/hyperledger/fabric/gossip/api"
	gossipCommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/integration"
	"github.com/hyperledger/fabric/gossip/state"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/gossip"
	"google.golang.org/grpc"
)

var (
	gossipServiceInstance *gossipServiceImpl
	once                  sync.Once
)

type gossipSvc gossip.Gossip

// GossipService encapsulates gossip and state capabilities into single interface
type GossipService interface {
	gossip.Gossip

	// NewConfigEventer creates a ConfigProcessor which the configtx.Manager can ultimately route config updates to
	NewConfigEventer() ConfigProcessor
	// InitializeChannel allocates the state provider and should be invoked once per channel per execution
	InitializeChannel(chainID string, committer committer.Committer)
	// GetBlock returns block for given chain
	GetBlock(chainID string, index uint64) *common.Block
	// AddPayload appends message payload to for given chain
	AddPayload(chainID string, payload *proto.Payload) error
}

// DeliveryServiceFactory factory to create and initialize delivery service instance
type DeliveryServiceFactory interface {
	// Returns an instance of delivery client
	Service(g GossipService) (deliverclient.DeliverService, error)
}

type deliveryFactoryImpl struct {
}

// Returns an instance of delivery client
func (*deliveryFactoryImpl) Service(g GossipService) (deliverclient.DeliverService, error) {
	return deliverclient.NewDeliverService(g)
}

type gossipServiceImpl struct {
	gossipSvc
	chains          map[string]state.GossipStateProvider
	deliveryService deliverclient.DeliverService
	deliveryFactory DeliveryServiceFactory
	lock            sync.RWMutex
}

// This is an implementation of api.JoinChannelMessage.
type joinChannelMessage struct {
	seqNum      uint64
	anchorPeers []api.AnchorPeer
}

func (jcm *joinChannelMessage) SequenceNumber() uint64 {
	return jcm.seqNum
}

func (jcm *joinChannelMessage) AnchorPeers() []api.AnchorPeer {
	return jcm.anchorPeers
}

var logger = util.GetLogger(util.LoggingServiceModule, "")

// InitGossipService initialize gossip service
func InitGossipService(identity []byte, endpoint string, s *grpc.Server, bootPeers ...string) {
	InitGossipServiceCustomDeliveryFactory(identity, endpoint, s, &deliveryFactoryImpl{}, bootPeers...)
}

// InitGossipService initialize gossip service with customize delivery factory
// implementation, might be useful for testing and mocking purposes
func InitGossipServiceCustomDeliveryFactory(identity []byte, endpoint string, s *grpc.Server, factory DeliveryServiceFactory, bootPeers ...string) {
	once.Do(func() {
		logger.Info("Initialize gossip with endpoint", endpoint, "and bootstrap set", bootPeers)
		dialOpts := []grpc.DialOption{}
		if peerComm.TLSEnabled() {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(peerComm.InitTLSForPeer()))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}

		gossip := integration.NewGossipComponent(identity, endpoint, s, dialOpts, bootPeers...)
		gossipServiceInstance = &gossipServiceImpl{
			gossipSvc:       gossip,
			chains:          make(map[string]state.GossipStateProvider),
			deliveryFactory: factory,
		}
	})
}

// GetGossipService returns an instance of gossip service
func GetGossipService() GossipService {
	return gossipServiceInstance
}

// NewConfigEventer creates a ConfigProcessor which the configtx.Manager can ultimately route config updates to
func (g *gossipServiceImpl) NewConfigEventer() ConfigProcessor {
	return newConfigEventer(g)
}

// InitializeChannel allocates the state provider and should be invoked once per channel per execution
func (g *gossipServiceImpl) InitializeChannel(chainID string, committer committer.Committer) {
	g.lock.Lock()
	defer g.lock.Unlock()
	// Initialize new state provider for given committer
	logger.Debug("Creating state provider for chainID", chainID)
	g.chains[chainID] = state.NewGossipStateProvider(chainID, g, committer)
	if g.deliveryService == nil {
		var err error
		g.deliveryService, err = g.deliveryFactory.Service(gossipServiceInstance)
		if err != nil {
			logger.Warning("Cannot create delivery client, due to", err)
		}
	}

	if g.deliveryService != nil {
		if err := g.deliveryService.JoinChain(chainID, committer); err != nil {
			logger.Error("Delivery service is not able to join the chain, due to", err)
		}
	} else {
		logger.Warning("Delivery client is down won't be able to pull blocks for chain", chainID)
	}
}

// configUpdated constructs a joinChannelMessage and sends it to the gossipSvc
func (g *gossipServiceImpl) configUpdated(config Config) {
	jcm := &joinChannelMessage{seqNum: config.Sequence(), anchorPeers: []api.AnchorPeer{}}
	for _, ap := range config.AnchorPeers() {
		anchorPeer := api.AnchorPeer{
			Host: ap.Host,
			Port: int(ap.Port),
			Cert: api.PeerIdentityType(ap.Cert),
		}
		jcm.anchorPeers = append(jcm.anchorPeers, anchorPeer)
	}

	// Initialize new state provider for given committer
	logger.Debug("Creating state provider for chainID", config.ChainID())
	g.JoinChan(jcm, gossipCommon.ChainID(config.ChainID()))
}

// GetBlock returns block for given chain
func (g *gossipServiceImpl) GetBlock(chainID string, index uint64) *common.Block {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.chains[chainID].GetBlock(index)
}

// AddPayload appends message payload to for given chain
func (g *gossipServiceImpl) AddPayload(chainID string, payload *proto.Payload) error {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.chains[chainID].AddPayload(payload)
}

// Stop stops the gossip component
func (g *gossipServiceImpl) Stop() {
	g.lock.Lock()
	defer g.lock.Unlock()
	for _, ch := range g.chains {
		logger.Info("Stopping chain", ch)
		ch.Stop()
	}
	g.gossipSvc.Stop()
	if g.deliveryService != nil {
		g.deliveryService.Stop()
	}
}
