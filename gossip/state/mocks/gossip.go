/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/stretchr/testify/mock"
)

type GossipMock struct {
	mock.Mock
}

func (g *GossipMock) SelfMembershipInfo() discovery.NetworkMember {
	panic("implement me")
}

func (g *GossipMock) SelfChannelInfo(common.ChannelID) *protoext.SignedGossipMessage {
	panic("implement me")
}

func (*GossipMock) PeerFilter(channel common.ChannelID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error) {
	panic("implement me")
}

func (g *GossipMock) SuspectPeers(s api.PeerSuspector) {
	g.Called(s)
}

// UpdateLedgerHeight updates the ledger height the peer
// publishes to other peers in the channel
func (g *GossipMock) UpdateLedgerHeight(height uint64, channelID common.ChannelID) {
}

// UpdateChaincodes updates the chaincodes the peer publishes
// to other peers in the channel
func (g *GossipMock) UpdateChaincodes(chaincode []*proto.Chaincode, channelID common.ChannelID) {
}

func (g *GossipMock) LeaveChan(_ common.ChannelID) {
	panic("implement me")
}

func (g *GossipMock) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	g.Called(msg, peers)
}

func (g *GossipMock) Peers() []discovery.NetworkMember {
	return g.Called().Get(0).([]discovery.NetworkMember)
}

func (g *GossipMock) PeersOfChannel(channelID common.ChannelID) []discovery.NetworkMember {
	args := g.Called(channelID)
	return args.Get(0).([]discovery.NetworkMember)
}

func (g *GossipMock) UpdateMetadata(metadata []byte) {
	g.Called(metadata)
}

func (g *GossipMock) Gossip(msg *proto.GossipMessage) {
	g.Called(msg)
}

func (g *GossipMock) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan protoext.ReceivedMessage) {
	args := g.Called(acceptor, passThrough)
	if args.Get(0) == nil {
		return nil, args.Get(1).(chan protoext.ReceivedMessage)
	}
	return args.Get(0).(<-chan *proto.GossipMessage), nil
}

func (g *GossipMock) JoinChan(joinMsg api.JoinChannelMessage, channelID common.ChannelID) {
}

// IdentityInfo returns information known peer identities
func (g *GossipMock) IdentityInfo() api.PeerIdentitySet {
	panic("not implemented")
}

func (g *GossipMock) IsInMyOrg(member discovery.NetworkMember) bool {
	panic("not implemented")
}

func (g *GossipMock) Stop() {
}

func (g *GossipMock) SendByCriteria(*protoext.SignedGossipMessage, gossip.SendCriteria) error {
	return nil
}
