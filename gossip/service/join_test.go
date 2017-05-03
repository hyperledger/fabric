/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type secAdvMock struct {
}

func init() {
	util.SetupTestLogging()
}

func (s *secAdvMock) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return api.OrgIdentityType(identity)
}

type gossipMock struct {
	mock.Mock
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

func (*gossipMock) PeersOfChannel(common.ChainID) []discovery.NetworkMember {
	panic("implement me")
}

func (*gossipMock) UpdateMetadata(metadata []byte) {
	panic("implement me")
}

func (*gossipMock) UpdateChannelMetadata(metadata []byte, chainID common.ChainID) {
	panic("implement me")
}

func (*gossipMock) Gossip(msg *proto.GossipMessage) {
	panic("implement me")
}

func (*gossipMock) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage) {
	panic("implement me")
}

func (g *gossipMock) JoinChan(joinMsg api.JoinChannelMessage, chainID common.ChainID) {
	g.Called(joinMsg, chainID)
}

func (*gossipMock) Stop() {
	panic("implement me")
}

type appOrgMock struct {
	id string
}

func (*appOrgMock) Name() string {
	panic("implement me")
}

func (ao *appOrgMock) MSPID() string {
	return ao.id
}

func (ao *appOrgMock) AnchorPeers() []*peer.AnchorPeer {
	return []*peer.AnchorPeer{}
}

type configMock struct {
	orgs2AppOrgs map[string]config.ApplicationOrg
}

func (*configMock) ChainID() string {
	return "A"
}

func (c *configMock) Organizations() map[string]config.ApplicationOrg {
	return c.orgs2AppOrgs
}

func (*configMock) Sequence() uint64 {
	return 0
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
	g1 := &gossipServiceImpl{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("OrgMSP0"), gossipSvc: g1SvcMock}
	g1.configUpdated(&configMock{
		orgs2AppOrgs: map[string]config.ApplicationOrg{
			"Org0": &appOrgMock{id: "Org0"},
		},
	})
	select {
	case <-time.After(time.Second):
	case <-failChan:
		assert.Fail(t, "Joined a badly configured channel")
	}

	succChan := make(chan struct{}, 1)
	g2SvcMock := &gossipMock{}
	g2SvcMock.On("JoinChan", mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		succChan <- struct{}{}
	})
	g2 := &gossipServiceImpl{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("Org0"), gossipSvc: g2SvcMock}
	g2.configUpdated(&configMock{
		orgs2AppOrgs: map[string]config.ApplicationOrg{
			"Org0": &appOrgMock{id: "Org0"},
		},
	})
	select {
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't join a channel (should have done so within the time period)")
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
		channel := args.Get(1).(common.ChainID)
		assert.Len(t, jcm.Members(), 2)
		assert.Contains(t, jcm.Members(), api.OrgIdentityType("Org0"))
		assert.Contains(t, jcm.Members(), api.OrgIdentityType("Org1"))
		assert.Equal(t, "A", string(channel))
	})

	g := &gossipServiceImpl{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("Org0"), gossipSvc: gMock}

	appOrg0 := &appOrgMock{id: "Org0"}
	appOrg1 := &appOrgMock{id: "Org1"}

	// Make sure the ApplicationOrgs really have no anchor peers
	assert.Empty(t, appOrg0.AnchorPeers())
	assert.Empty(t, appOrg1.AnchorPeers())

	g.configUpdated(&configMock{
		orgs2AppOrgs: map[string]config.ApplicationOrg{
			"Org0": appOrg0,
			"Org1": appOrg1,
		},
	})
	joinChanCalled.Wait()
}
