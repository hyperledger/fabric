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
	"github.com/hyperledger/fabric/gossip/election"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/identity"
	"github.com/hyperledger/fabric/gossip/integration"
	"github.com/hyperledger/fabric/gossip/state"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/peer/gossip/sa"
	"github.com/hyperledger/fabric/protos/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/spf13/viper"
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
	InitializeChannel(chainID string, committer committer.Committer, endpoints []string)
	// GetBlock returns block for given chain
	GetBlock(chainID string, index uint64) *common.Block
	// AddPayload appends message payload to for given chain
	AddPayload(chainID string, payload *proto.Payload) error
}

// DeliveryServiceFactory factory to create and initialize delivery service instance
type DeliveryServiceFactory interface {
	// Returns an instance of delivery client
	Service(g GossipService, endpoints []string, msc api.MessageCryptoService) (deliverclient.DeliverService, error)
}

type deliveryFactoryImpl struct {
}

// Returns an instance of delivery client
func (*deliveryFactoryImpl) Service(g GossipService, endpoints []string, mcs api.MessageCryptoService) (deliverclient.DeliverService, error) {
	return deliverclient.NewDeliverService(g, endpoints, mcs)
}

type gossipServiceImpl struct {
	gossipSvc
	chains          map[string]state.GossipStateProvider
	leaderElection  map[string]election.LeaderElectionService
	deliveryService deliverclient.DeliverService
	deliveryFactory DeliveryServiceFactory
	lock            sync.RWMutex
	idMapper        identity.Mapper
	mcs             api.MessageCryptoService
	peerIdentity    []byte
	secAdv          api.SecurityAdvisor
}

// This is an implementation of api.JoinChannelMessage.
type joinChannelMessage struct {
	seqNum              uint64
	members2AnchorPeers map[string][]api.AnchorPeer
}

func (jcm *joinChannelMessage) SequenceNumber() uint64 {
	return jcm.seqNum
}

// Members returns the organizations of the channel
func (jcm *joinChannelMessage) Members() []api.OrgIdentityType {
	members := make([]api.OrgIdentityType, len(jcm.members2AnchorPeers))
	i := 0
	for org := range jcm.members2AnchorPeers {
		members[i] = api.OrgIdentityType(org)
		i++
	}
	return members
}

// AnchorPeersOf returns the anchor peers of the given organization
func (jcm *joinChannelMessage) AnchorPeersOf(org api.OrgIdentityType) []api.AnchorPeer {
	return jcm.members2AnchorPeers[string(org)]
}

var logger = util.GetLogger(util.LoggingServiceModule, "")

// InitGossipService initialize gossip service
func InitGossipService(peerIdentity []byte, endpoint string, s *grpc.Server, mcs api.MessageCryptoService, bootPeers ...string) {
	InitGossipServiceCustomDeliveryFactory(peerIdentity, endpoint, s, &deliveryFactoryImpl{}, mcs, bootPeers...)
}

// InitGossipService initialize gossip service with customize delivery factory
// implementation, might be useful for testing and mocking purposes
func InitGossipServiceCustomDeliveryFactory(peerIdentity []byte, endpoint string, s *grpc.Server, factory DeliveryServiceFactory, mcs api.MessageCryptoService, bootPeers ...string) {
	once.Do(func() {
		logger.Info("Initialize gossip with endpoint", endpoint, "and bootstrap set", bootPeers)
		dialOpts := []grpc.DialOption{}
		if peerComm.TLSEnabled() {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(peerComm.InitTLSForPeer()))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}

		secAdv := sa.NewSecurityAdvisor()

		if overrideEndpoint := viper.GetString("peer.gossip.endpoint"); overrideEndpoint != "" {
			endpoint = overrideEndpoint
		}

		if viper.GetBool("peer.gossip.ignoreSecurity") {
			logger.Info("This peer ignoring security in gossip")
			sec := &secImpl{[]byte(endpoint)}
			mcs = sec
			secAdv = sec
			peerIdentity = []byte(endpoint)
		}

		idMapper := identity.NewIdentityMapper(mcs)
		idMapper.Put(mcs.GetPKIidOfCert(peerIdentity), peerIdentity)

		gossip := integration.NewGossipComponent(peerIdentity, endpoint, s, secAdv, mcs, idMapper, dialOpts, bootPeers...)
		gossipServiceInstance = &gossipServiceImpl{
			mcs:             mcs,
			gossipSvc:       gossip,
			chains:          make(map[string]state.GossipStateProvider),
			leaderElection:  make(map[string]election.LeaderElectionService),
			deliveryFactory: factory,
			idMapper:        idMapper,
			peerIdentity:    peerIdentity,
			secAdv:          secAdv,
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
func (g *gossipServiceImpl) InitializeChannel(chainID string, committer committer.Committer, endpoints []string) {
	g.lock.Lock()
	defer g.lock.Unlock()
	// Initialize new state provider for given committer
	logger.Debug("Creating state provider for chainID", chainID)
	g.chains[chainID] = state.NewGossipStateProvider(chainID, g, committer, g.mcs)
	if g.deliveryService == nil {
		var err error
		g.deliveryService, err = g.deliveryFactory.Service(gossipServiceInstance, endpoints, g.mcs)
		if err != nil {
			logger.Warning("Cannot create delivery client, due to", err)
		}
	}

	if g.deliveryService != nil {
		leaderElection := viper.GetBool("peer.gossip.useLeaderElection")
		staticOrderConnection := viper.GetBool("peer.gossip.orgLeader")

		if leaderElection && staticOrderConnection {
			msg := "Setting both orgLeader and useLeaderElection to true isn't supported, aborting execution"
			logger.Panic(msg)
		} else if leaderElection {
			logger.Debug("Delivery uses dynamic leader election mechanism, channel", chainID)
			connector := &leaderElectionDeliverConnector{
				deliverer: g.deliveryService,
				committer: committer,
				chainID:   chainID,
			}
			electionService := g.newLeaderElectionComponent(gossipCommon.ChainID(connector.chainID), connector.leadershipStatusChange)
			g.leaderElection[chainID] = electionService
		} else if staticOrderConnection {
			logger.Debug("This peer is configured to connect to ordering service for blocks delivery, channel", chainID)
			g.deliveryService.StartDeliverForChannel(chainID, committer)
		} else {
			logger.Debug("This peer is not configured to connect to ordering service for blocks delivery, channel", chainID)
		}
	} else {
		logger.Warning("Delivery client is down won't be able to pull blocks for chain", chainID)
	}
}

// configUpdated constructs a joinChannelMessage and sends it to the gossipSvc
func (g *gossipServiceImpl) configUpdated(config Config) {
	myOrg := string(g.secAdv.OrgByPeerIdentity(api.PeerIdentityType(g.peerIdentity)))
	if !g.amIinChannel(myOrg, config) {
		logger.Error("Tried joining channel", config.ChainID(), "but our org(", myOrg, "), isn't "+
			"among the orgs of the channel:", orgListFromConfig(config), ", aborting.")
		return
	}
	jcm := &joinChannelMessage{seqNum: config.Sequence(), members2AnchorPeers: map[string][]api.AnchorPeer{}}
	for orgID, appOrg := range config.Organizations() {
		for _, ap := range appOrg.AnchorPeers() {
			anchorPeer := api.AnchorPeer{
				Host: ap.Host,
				Port: int(ap.Port),
			}
			jcm.members2AnchorPeers[orgID] = append(jcm.members2AnchorPeers[orgID], anchorPeer)
		}
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

	for chainID, electionService := range g.leaderElection {
		logger.Info("Stopping leader election for %s", chainID)
		electionService.Stop()
	}
	g.gossipSvc.Stop()
	if g.deliveryService != nil {
		g.deliveryService.Stop()
	}
}

func (g *gossipServiceImpl) newLeaderElectionComponent(channel gossipCommon.ChainID, callback func(bool)) election.LeaderElectionService {
	PKIid := g.idMapper.GetPKIidOfCert(g.peerIdentity)
	adapter := election.NewAdapter(g, PKIid, channel)
	return election.NewLeaderElectionService(adapter, string(PKIid), callback)
}

func (g *gossipServiceImpl) amIinChannel(myOrg string, config Config) bool {
	for _, orgName := range orgListFromConfig(config) {
		if orgName == myOrg {
			return true
		}
	}
	return false
}

func orgListFromConfig(config Config) []string {
	var orgList []string
	for orgName := range config.Organizations() {
		orgList = append(orgList, orgName)
	}
	return orgList
}

type secImpl struct {
	identity []byte
}

func (*secImpl) OrgByPeerIdentity(api.PeerIdentityType) api.OrgIdentityType {
	return api.OrgIdentityType("DEFAULT")
}

func (s *secImpl) GetPKIidOfCert(peerIdentity api.PeerIdentityType) gossipCommon.PKIidType {
	return gossipCommon.PKIidType(peerIdentity)
}

func (s *secImpl) VerifyBlock(chainID gossipCommon.ChainID, signedBlock []byte) error {
	return nil
}

func (s *secImpl) Sign(msg []byte) ([]byte, error) {
	return msg, nil
}

func (s *secImpl) Verify(peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (s *secImpl) VerifyByChannel(chainID gossipCommon.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error {
	return nil
}

func (s *secImpl) ValidateIdentity(peerIdentity api.PeerIdentityType) error {
	return nil
}

type leaderElectionDeliverConnector struct {
	deliverer deliverclient.DeliverService
	chainID   string
	committer committer.Committer
}

func (ledc *leaderElectionDeliverConnector) leadershipStatusChange(isLeader bool) {
	if isLeader {
		if err := ledc.deliverer.StartDeliverForChannel(ledc.chainID, ledc.committer); err != nil {
			logger.Error("Delivery service is not able to start blocks delivery for chain, due to", err)
		}
	} else {
		if err := ledc.deliverer.StopDeliverForChannel(ledc.chainID); err != nil {
			logger.Error("Delivery service is not able to stop blocks delivery for chain, due to", err)
		}

	}
}
