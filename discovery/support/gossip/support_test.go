/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip_test

import (
	"testing"

	gp "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/discovery/support/gossip"
	"github.com/hyperledger/fabric/discovery/support/gossip/mocks"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/require"
)

func TestChannelExists(t *testing.T) {
	g := &mocks.Gossip{}
	sup := gossip.NewDiscoverySupport(g)
	require.False(t, sup.ChannelExists(""))
}

func TestPeers(t *testing.T) {
	g := &mocks.Gossip{}
	sup := gossip.NewDiscoverySupport(g)
	p1Envelope := &gp.Envelope{
		Payload: []byte{4, 5, 6},
		SecretEnvelope: &gp.SecretEnvelope{
			Payload: []byte{1, 2, 3},
		},
	}
	peers := []discovery.NetworkMember{
		{PKIid: common.PKIidType("p1"), Endpoint: "p1", Envelope: p1Envelope},
		{PKIid: common.PKIidType("p2")},
	}
	g.PeersReturnsOnCall(0, peers)
	g.SelfMembershipInfoReturnsOnCall(0, discovery.NetworkMember{PKIid: common.PKIidType("p0"), Endpoint: "p0"})
	p1ExpectedEnvelope := &gp.Envelope{
		Payload: []byte{4, 5, 6},
	}
	expected := discovery.Members{{PKIid: common.PKIidType("p1"), Endpoint: "p1", Envelope: p1ExpectedEnvelope}, {PKIid: common.PKIidType("p0"), Endpoint: "p0"}}
	actual := sup.Peers()
	require.Equal(t, expected, actual)
}

func TestPeersOfChannel(t *testing.T) {
	stateInfo := &gp.GossipMessage{
		Content: &gp.GossipMessage_StateInfo{
			StateInfo: &gp.StateInfo{
				PkiId: common.PKIidType("px"),
			},
		},
	}
	sMsg, _ := protoext.NoopSign(stateInfo)
	g := &mocks.Gossip{}
	g.SelfChannelInfoReturnsOnCall(0, nil)
	g.SelfChannelInfoReturnsOnCall(1, sMsg)
	g.PeersOfChannelReturnsOnCall(0, []discovery.NetworkMember{{PKIid: common.PKIidType("p1")}, {PKIid: common.PKIidType("p2")}})
	sup := gossip.NewDiscoverySupport(g)
	require.Empty(t, sup.PeersOfChannel(common.ChannelID("")))
	expected := discovery.Members{{PKIid: common.PKIidType("p1")}, {PKIid: common.PKIidType("p2")}, {PKIid: common.PKIidType("px"), Envelope: sMsg.Envelope}}
	require.Equal(t, expected, sup.PeersOfChannel(common.ChannelID("")))
}
