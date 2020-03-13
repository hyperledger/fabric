/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"bytes"
	"sync"
	"sync/atomic"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip/channel"
	"github.com/hyperledger/fabric/gossip/metrics"
	"github.com/hyperledger/fabric/gossip/protoext"
)

type channelState struct {
	stopping int32
	sync.RWMutex
	channels map[string]channel.GossipChannel
	g        *Node
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

func (cs *channelState) lookupChannelForMsg(msg protoext.ReceivedMessage) channel.GossipChannel {
	if protoext.IsStateInfoPullRequestMsg(msg.GetGossipMessage().GossipMessage) {
		sipr := msg.GetGossipMessage().GetStateInfoPullReq()
		mac := sipr.Channel_MAC
		pkiID := msg.GetConnectionInfo().ID
		return cs.getGossipChannelByMAC(mac, pkiID)
	}
	return cs.lookupChannelForGossipMsg(msg.GetGossipMessage().GossipMessage)
}

func (cs *channelState) lookupChannelForGossipMsg(msg *proto.GossipMessage) channel.GossipChannel {
	if !protoext.IsStateInfoMsg(msg) {
		// If we reached here then the message isn't:
		// 1) StateInfoPullRequest
		// 2) StateInfo
		// Hence, it was already sent to a peer (us) that has proved it knows the channel name, by
		// sending StateInfo messages in the past.
		// Therefore- we use the channel name from the message itself.
		return cs.getGossipChannelByChainID(msg.Channel)
	}

	// Else, it's a StateInfo message.
	stateInfMsg := msg.GetStateInfo()
	return cs.getGossipChannelByMAC(stateInfMsg.Channel_MAC, stateInfMsg.PkiId)
}

func (cs *channelState) getGossipChannelByMAC(receivedMAC []byte, pkiID common.PKIidType) channel.GossipChannel {
	// Iterate over the channels, and try to find a channel that the computation
	// of the MAC is equal to the MAC on the message.
	// If it is, then the peer that signed the message knows the name of the channel
	// because its PKI-ID was checked when the message was verified.
	cs.RLock()
	defer cs.RUnlock()
	for chanName, gc := range cs.channels {
		mac := channel.GenerateMAC(pkiID, common.ChannelID(chanName))
		if bytes.Equal(mac, receivedMAC) {
			return gc
		}
	}
	return nil
}

func (cs *channelState) getGossipChannelByChainID(channelID common.ChannelID) channel.GossipChannel {
	if cs.isStopping() {
		return nil
	}
	cs.RLock()
	defer cs.RUnlock()
	return cs.channels[string(channelID)]
}

func (cs *channelState) joinChannel(joinMsg api.JoinChannelMessage, channelID common.ChannelID,
	metrics *metrics.MembershipMetrics) {
	if cs.isStopping() {
		return
	}
	cs.Lock()
	defer cs.Unlock()
	if gc, exists := cs.channels[string(channelID)]; !exists {
		pkiID := cs.g.comm.GetPKIid()
		ga := &gossipAdapterImpl{Node: cs.g, Discovery: cs.g.disc}
		gc := channel.NewGossipChannel(pkiID, cs.g.selfOrg, cs.g.mcs, channelID, ga, joinMsg, metrics, nil)
		cs.channels[string(channelID)] = gc
	} else {
		gc.ConfigureChannel(joinMsg)
	}
}

type gossipAdapterImpl struct {
	*Node
	discovery.Discovery
}

func (ga *gossipAdapterImpl) GetConf() channel.Config {
	return channel.Config{
		ID:                          ga.conf.ID,
		MaxBlockCountToStore:        ga.conf.MaxBlockCountToStore,
		PublishStateInfoInterval:    ga.conf.PublishStateInfoInterval,
		PullInterval:                ga.conf.PullInterval,
		PullPeerNum:                 ga.conf.PullPeerNum,
		RequestStateInfoInterval:    ga.conf.RequestStateInfoInterval,
		BlockExpirationInterval:     ga.conf.PullInterval * 100,
		StateInfoCacheSweepInterval: ga.conf.PullInterval * 5,
		TimeForMembershipTracker:    ga.conf.TimeForMembershipTracker,
		DigestWaitTime:              ga.conf.DigestWaitTime,
		RequestWaitTime:             ga.conf.RequestWaitTime,
		ResponseWaitTime:            ga.conf.ResponseWaitTime,
		MsgExpirationTimeout:        ga.conf.MsgExpirationTimeout,
	}
}

func (ga *gossipAdapterImpl) Sign(msg *proto.GossipMessage) (*protoext.SignedGossipMessage, error) {
	signer := func(msg []byte) ([]byte, error) {
		return ga.mcs.Sign(msg)
	}
	sMsg := &protoext.SignedGossipMessage{
		GossipMessage: msg,
	}
	e, err := sMsg.Sign(signer)
	if err != nil {
		return nil, err
	}
	return &protoext.SignedGossipMessage{
		Envelope:      e,
		GossipMessage: msg,
	}, nil
}

// Gossip gossips a message
func (ga *gossipAdapterImpl) Gossip(msg *protoext.SignedGossipMessage) {
	ga.Node.emitter.Add(&emittedGossipMessage{
		SignedGossipMessage: msg,
		filter: func(_ common.PKIidType) bool {
			return true
		},
	})
}

// Forward sends message to the next hops
func (ga *gossipAdapterImpl) Forward(msg protoext.ReceivedMessage) {
	ga.Node.emitter.Add(&emittedGossipMessage{
		SignedGossipMessage: msg.GetGossipMessage(),
		filter:              msg.GetConnectionInfo().ID.IsNotSameFilter,
	})
}

func (ga *gossipAdapterImpl) Send(msg *protoext.SignedGossipMessage, peers ...*comm.RemotePeer) {
	ga.Node.comm.Send(msg, peers...)
}

// ValidateStateInfoMessage returns error if a message isn't valid
// nil otherwise
func (ga *gossipAdapterImpl) ValidateStateInfoMessage(msg *protoext.SignedGossipMessage) error {
	return ga.Node.validateStateInfoMsg(msg)
}

// GetOrgOfPeer returns the organization identifier of a certain peer
func (ga *gossipAdapterImpl) GetOrgOfPeer(PKIID common.PKIidType) api.OrgIdentityType {
	return ga.Node.getOrgOfPeer(PKIID)
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
