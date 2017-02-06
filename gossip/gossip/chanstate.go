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

package gossip

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/gossip/msgstore"
	"github.com/hyperledger/fabric/gossip/gossip/pull"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/op/go-logging"
)

type channelState struct {
	stopping int32
	sync.RWMutex
	channels map[string]channel.GossipChannel
	g        *gossipServiceImpl
}

func (cs *channelState) stop() {
	if cs.isStopping() {
		return
	}
	atomic.StoreInt32(&cs.stopping, int32(1))
	cs.Lock()
	defer cs.Unlock()
	for _, gc := range cs.channels {
		gc.Stop()
	}
}

func (cs *channelState) isStopping() bool {
	return atomic.LoadInt32(&cs.stopping) == int32(1)
}

func (cs *channelState) getGossipChannelByChainID(chainID common.ChainID) channel.GossipChannel {
	if cs.isStopping() {
		return nil
	}
	cs.Lock()
	defer cs.Unlock()
	return cs.channels[string(chainID)]
}

func (cs *channelState) joinChannel(joinMsg api.JoinChannelMessage, chainID common.ChainID) {
	if cs.isStopping() {
		return
	}
	cs.Lock()
	defer cs.Unlock()
	if gc, exists := cs.channels[string(chainID)]; !exists {
		cs.channels[string(chainID)] = channel.NewGossipChannel(cs.g.mcs, chainID, &gossipAdapterImpl{gossipServiceImpl: cs.g, Discovery: cs.g.disc}, joinMsg)
	} else {
		gc.ConfigureChannel(joinMsg)
	}
}

type gossipAdapterImpl struct {
	*gossipServiceImpl
	discovery.Discovery
}

func (ga *gossipAdapterImpl) GetConf() channel.Config {
	return channel.Config{
		ID:                       ga.conf.ID,
		MaxBlockCountToStore:     ga.conf.MaxBlockCountToStore,
		PublishStateInfoInterval: ga.conf.PublishStateInfoInterval,
		PullInterval:             ga.conf.PullInterval,
		PullPeerNum:              ga.conf.PullPeerNum,
		RequestStateInfoInterval: ga.conf.RequestStateInfoInterval,
	}
}

// Gossip gossips a message
func (ga *gossipAdapterImpl) Gossip(msg *proto.GossipMessage) {
	ga.gossipServiceImpl.emitter.Add(msg)
}

// ValidateStateInfoMessage returns error if a message isn't valid
// nil otherwise
func (ga *gossipAdapterImpl) ValidateStateInfoMessage(msg *proto.GossipMessage) error {
	return ga.gossipServiceImpl.validateStateInfoMsg(msg)
}

// OrgByPeerIdentity extracts the organization identifier from a peer's identity
func (ga *gossipAdapterImpl) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return ga.gossipServiceImpl.secAdvisor.OrgByPeerIdentity(identity)
}

// GetOrgOfPeer returns the organization identifier of a certain peer
func (ga *gossipAdapterImpl) GetOrgOfPeer(PKIID common.PKIidType) api.OrgIdentityType {
	return ga.gossipServiceImpl.getOrgOfPeer(PKIID)
}

// Adapter enables the gossipChannel
// to communicate with gossipServiceImpl.

// Adapter connects a GossipChannel to the gossip implementation
type Adapter interface {

	// GetConf returns the configuration
	// of the GossipChannel
	GetConf() Config

	// Gossip gossips a message
	Gossip(*proto.GossipMessage)

	// DeMultiplex publishes a message to subscribers
	DeMultiplex(interface{})

	// GetMembership returns the peers that are considered alive
	GetMembership() []discovery.NetworkMember

	// Send sends a message to a list of peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// ValidateStateInfoMessage returns error if a message isn't valid
	// nil otherwise
	ValidateStateInfoMessage(*proto.GossipMessage) error

	// OrgByPeerIdentity extracts the organization identifier from a peer's identity
	OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType

	// GetOrgOfPeer returns the organization identifier of a certain peer
	GetOrgOfPeer(common.PKIidType) api.OrgIdentityType
}

type gossipChannel struct {
	Adapter
	sync.RWMutex
	shouldGossipStateInfo     int32
	stopChan                  chan struct{}
	stateInfoMsg              *proto.GossipMessage
	orgs                      []api.OrgIdentityType
	joinMsg                   api.JoinChannelMessage
	blockMsgStore             msgstore.MessageStore
	stateInfoMsgStore         msgstore.MessageStore
	chainID                   common.ChainID
	blocksPuller              pull.Mediator
	logger                    *logging.Logger
	stateInfoPublishScheduler *time.Ticker
	stateInfoRequestScheduler *time.Ticker
}
