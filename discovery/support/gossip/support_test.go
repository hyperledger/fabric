/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip_test

import (
	"testing"

	gossipSupport "github.com/hyperledger/fabric/discovery/support/gossip"
	"github.com/hyperledger/fabric/discovery/support/mocks"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
)

func TestChannelExists(t *testing.T) {
	g := &mocks.Gossip{}
	sup := gossipSupport.NewDiscoverySupport(g)
	assert.False(t, sup.ChannelExists(""))
}

func TestPeers(t *testing.T) {
	g := &mocks.Gossip{}
	sup := gossipSupport.NewDiscoverySupport(g)
	p1Envelope := &gossip.Envelope{
		Payload: []byte{4, 5, 6},
		SecretEnvelope: &gossip.SecretEnvelope{
			Payload: []byte{1, 2, 3},
		},
	}
	peers := []discovery.NetworkMember{
		{PKIid: common.PKIidType("p1"), Endpoint: "p1", Envelope: p1Envelope},
		{PKIid: common.PKIidType("p2")},
	}
	g.PeersReturnsOnCall(0, peers)
	g.SelfMembershipInfoReturnsOnCall(0, discovery.NetworkMember{PKIid: common.PKIidType("p0"), Endpoint: "p0"})
	p1ExpectedEnvelope := &gossip.Envelope{
		Payload: []byte{4, 5, 6},
	}
	expected := discovery.Members{{PKIid: common.PKIidType("p1"), Endpoint: "p1", Envelope: p1ExpectedEnvelope}, {PKIid: common.PKIidType("p0"), Endpoint: "p0"}}
	actual := sup.Peers()
	assert.Equal(t, expected, actual)
}

func TestPeersOfChannel(t *testing.T) {
	stateInfo := &gossip.GossipMessage{
		Content: &gossip.GossipMessage_StateInfo{
			StateInfo: &gossip.StateInfo{
				PkiId: common.PKIidType("px"),
			},
		},
	}
	sMsg, _ := stateInfo.NoopSign()
	g := &mocks.Gossip{}
	g.SelfChannelInfoReturnsOnCall(0, nil)
	g.SelfChannelInfoReturnsOnCall(1, sMsg)
	g.PeersOfChannelReturnsOnCall(0, []discovery.NetworkMember{{PKIid: common.PKIidType("p1")}, {PKIid: common.PKIidType("p2")}})
	sup := gossipSupport.NewDiscoverySupport(g)
	assert.Empty(t, sup.PeersOfChannel(common.ChainID("")))
	expected := discovery.Members{{PKIid: common.PKIidType("p1")}, {PKIid: common.PKIidType("p2")}, {PKIid: common.PKIidType("px"), Envelope: sMsg.Envelope}}
	assert.Equal(t, expected, sup.PeersOfChannel(common.ChainID("")))
}
