/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package service

import (
	"sync"
	"testing"
	"time"

	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/protoext"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/msp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type secAdvMock struct{}

func init() {
	util.SetupTestLogging()
}

func (s *secAdvMock) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return api.OrgIdentityType(identity)
}

type gossipMock struct {
	mock.Mock
}

func (g *gossipMock) SelfChannelInfo(common.ChannelID) *protoext.SignedGossipMessage {
	panic("implement me")
}

func (g *gossipMock) SelfMembershipInfo() discovery.NetworkMember {
	panic("implement me")
}

func (*gossipMock) PeerFilter(channel common.ChannelID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error) {
	panic("implement me")
}

func (*gossipMock) SuspectPeers(s api.PeerSuspector) {
	panic("implement me")
}

func (*gossipMock) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	panic("implement me")
}

func (*gossipMock) Peers() []discovery.NetworkMember {
	panic("implement me")
}

func (*gossipMock) PeersOfChannel(common.ChannelID) []discovery.NetworkMember {
	panic("implement me")
}

func (*gossipMock) UpdateMetadata(metadata []byte) {
	panic("implement me")
}

// UpdateLedgerHeight updates the ledger height the peer
// publishes to other peers in the channel
func (*gossipMock) UpdateLedgerHeight(height uint64, channelID common.ChannelID) {
	panic("implement me")
}

// UpdateChaincodes updates the chaincodes the peer publishes
// to other peers in the channel
func (*gossipMock) UpdateChaincodes(chaincode []*proto.Chaincode, channelID common.ChannelID) {
	panic("implement me")
}

func (*gossipMock) Gossip(msg *proto.GossipMessage) {
	panic("implement me")
}

func (*gossipMock) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan protoext.ReceivedMessage) {
	panic("implement me")
}

func (g *gossipMock) JoinChan(joinMsg api.JoinChannelMessage, channelID common.ChannelID) {
	g.Called(joinMsg, channelID)
}

func (g *gossipMock) LeaveChan(channelID common.ChannelID) {
	panic("implement me")
}

func (g *gossipMock) IdentityInfo() api.PeerIdentitySet {
	panic("implement me")
}

func (*gossipMock) IsInMyOrg(member discovery.NetworkMember) bool {
	panic("implement me")
}

func (*gossipMock) Stop() {
	panic("implement me")
}

func (*gossipMock) SendByCriteria(*protoext.SignedGossipMessage, gossip.SendCriteria) error {
	panic("implement me")
}

type appOrgMock struct {
	id string
}

func (*appOrgMock) Name() string {
	panic("implement me")
}

func (*appOrgMock) MSP() msp.MSP {
	panic("generate this fake instead")
}

func (ao *appOrgMock) MSPID() string {
	return ao.id
}

func (ao *appOrgMock) AnchorPeers() []*peer.AnchorPeer {
	return []*peer.AnchorPeer{}
}

func TestJoinChannelConfig(t *testing.T) {
	// Scenarios: The channel we're joining has a single org - Org0
	// but our org ID is actually Org0MSP in the negative path
	// and Org0 in the positive path

	failChan := make(chan struct{}, 1)
	g1SvcMock := &gossipMock{}
	g1SvcMock.On("JoinChan", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		failChan <- struct{}{}
	})
	anchorPeerTracker := &anchorPeerTracker{allEndpoints: map[string]map[string]struct{}{}}
	g1 := &GossipService{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("OrgMSP0"), gossipSvc: g1SvcMock, anchorPeerTracker: anchorPeerTracker}
	g1.updateAnchors(ConfigUpdate{
		ChannelID:        "A",
		OrdererAddresses: []string{"localhost:7050"},
		Organizations: map[string]channelconfig.ApplicationOrg{
			"Org0": &appOrgMock{id: "Org0"},
		},
	})
	select {
	case <-time.After(time.Second):
	case <-failChan:
		require.Fail(t, "Joined a badly configured channel")
	}

	succChan := make(chan struct{}, 1)
	g2SvcMock := &gossipMock{}
	g2SvcMock.On("JoinChan", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		succChan <- struct{}{}
	})
	g2 := &GossipService{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("Org0"), gossipSvc: g2SvcMock, anchorPeerTracker: anchorPeerTracker}
	g2.updateAnchors(ConfigUpdate{
		ChannelID:        "A",
		OrdererAddresses: []string{"localhost:7050"},
		Organizations: map[string]channelconfig.ApplicationOrg{
			"Org0": &appOrgMock{id: "Org0"},
		},
	})
	select {
	case <-time.After(time.Second):
		require.Fail(t, "Didn't join a channel (should have done so within the time period)")
	case <-succChan:
	}
}

func TestJoinChannelNoAnchorPeers(t *testing.T) {
	// Scenario: The channel we're joining has 2 orgs but no anchor peers
	// The test ensures that JoinChan is called with a JoinChannelMessage with Members
	// that consist of the organizations of the configuration given.

	var joinChanCalled sync.WaitGroup
	joinChanCalled.Add(1)
	gMock := &gossipMock{}
	gMock.On("JoinChan", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		defer joinChanCalled.Done()
		jcm := args.Get(0).(api.JoinChannelMessage)
		channel := args.Get(1).(common.ChannelID)
		require.Len(t, jcm.Members(), 2)
		require.Contains(t, jcm.Members(), api.OrgIdentityType("Org0"))
		require.Contains(t, jcm.Members(), api.OrgIdentityType("Org1"))
		require.Equal(t, "A", string(channel))
	})

	anchorPeerTracker := &anchorPeerTracker{allEndpoints: map[string]map[string]struct{}{}}
	g := &GossipService{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("Org0"), gossipSvc: gMock, anchorPeerTracker: anchorPeerTracker}

	appOrg0 := &appOrgMock{id: "Org0"}
	appOrg1 := &appOrgMock{id: "Org1"}

	// Make sure the ApplicationOrgs really have no anchor peers
	require.Empty(t, appOrg0.AnchorPeers())
	require.Empty(t, appOrg1.AnchorPeers())

	g.updateAnchors(ConfigUpdate{
		ChannelID:        "A",
		OrdererAddresses: []string{"localhost:7050"},
		Organizations: map[string]channelconfig.ApplicationOrg{
			"Org0": appOrg0,
			"Org1": appOrg1,
		},
	})
	joinChanCalled.Wait()
}
