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

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	proto "github.com/hyperledger/fabric/protos/gossip"
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
func (ga *gossipAdapterImpl) Gossip(msg *proto.SignedGossipMessage) {
	ga.gossipServiceImpl.emitter.Add(msg)
}

func (ga *gossipAdapterImpl) Send(msg *proto.SignedGossipMessage, peers ...*comm.RemotePeer) {
	ga.gossipServiceImpl.comm.Send(msg, peers...)
}

// ValidateStateInfoMessage returns error if a message isn't valid
// nil otherwise
func (ga *gossipAdapterImpl) ValidateStateInfoMessage(msg *proto.SignedGossipMessage) error {
	return ga.gossipServiceImpl.validateStateInfoMsg(msg)
}

// GetOrgOfPeer returns the organization identifier of a certain peer
func (ga *gossipAdapterImpl) GetOrgOfPeer(PKIID common.PKIidType) api.OrgIdentityType {
	return ga.gossipServiceImpl.getOrgOfPeer(PKIID)
}

// GetIdentityByPKIID returns an identity of a peer with a certain
// pkiID, or nil if not found
func (ga *gossipAdapterImpl) GetIdentityByPKIID(pkiID common.PKIidType) api.PeerIdentityType {
	identity, err := ga.idMapper.Get(pkiID)
	if err != nil {
		return nil
	}
	return identity
}
