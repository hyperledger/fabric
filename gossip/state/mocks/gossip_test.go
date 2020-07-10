/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"testing"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGossipMock(t *testing.T) {
	g := GossipMock{}
	mkChan := func() <-chan *proto.GossipMessage {
		c := make(chan *proto.GossipMessage, 1)
		c <- &proto.GossipMessage{}
		return c
	}
	g.On("Accept", mock.Anything, false).Return(mkChan(), nil)
	a, b := g.Accept(func(o interface{}) bool {
		return true
	}, false)
	require.Nil(t, b)
	require.NotNil(t, a)
	require.Panics(t, func() {
		g.SuspectPeers(func(identity api.PeerIdentityType) bool { return false })
	})
	require.Panics(t, func() {
		g.Send(nil, nil)
	})
	require.Panics(t, func() {
		g.Peers()
	})
	g.On("PeersOfChannel", mock.Anything).Return([]discovery.NetworkMember{})
	require.Empty(t, g.PeersOfChannel(common.ChannelID("A")))

	require.Panics(t, func() {
		g.UpdateMetadata([]byte{})
	})
	require.Panics(t, func() {
		g.Gossip(nil)
	})
	require.NotPanics(t, func() {
		g.UpdateLedgerHeight(0, common.ChannelID("A"))
		g.Stop()
		g.JoinChan(nil, common.ChannelID("A"))
	})
}
