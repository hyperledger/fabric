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
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type secAdvMock struct {
}

func (s *secAdvMock) OrgByPeerIdentity(identity api.PeerIdentityType) api.OrgIdentityType {
	return api.OrgIdentityType(identity)
}

type gossipMock struct {
	mock.Mock
}

func (*gossipMock) Send(msg *gossip.GossipMessage, peers ...*comm.RemotePeer) {
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

func (*gossipMock) Gossip(msg *gossip.GossipMessage) {
	panic("implement me")
}

func (*gossipMock) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *gossip.GossipMessage, <-chan gossip.ReceivedMessage) {
	panic("implement me")
}

func (g *gossipMock) JoinChan(joinMsg api.JoinChannelMessage, chainID common.ChainID) {
	g.Called()
}

func (*gossipMock) Stop() {
	panic("implement me")
}

type appOrgMock struct {
}

func (*appOrgMock) Name() string {
	panic("implement me")
}

func (*appOrgMock) MSPID() string {
	panic("implement me")
}

func (*appOrgMock) AnchorPeers() []*peer.AnchorPeer {
	return []*peer.AnchorPeer{{Host: "1.2.3.4", Port: 5611}}
}

type configMock struct {
}

func (*configMock) ChainID() string {
	return "A"
}

func (*configMock) Organizations() map[string]config.ApplicationOrg {
	return map[string]config.ApplicationOrg{
		"Org0": &appOrgMock{},
	}
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
	g1SvcMock.On("JoinChan", mock.Anything).Run(func(_ mock.Arguments) {
		failChan <- struct{}{}
	})
	g1 := &gossipServiceImpl{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("OrgMSP0"), gossipSvc: g1SvcMock}
	g1.configUpdated(&configMock{})
	select {
	case <-time.After(time.Second):
	case <-failChan:
		assert.Fail(t, "Joined a badly configured channel")
	}

	succChan := make(chan struct{}, 1)
	g2SvcMock := &gossipMock{}
	g2SvcMock.On("JoinChan", mock.Anything).Run(func(_ mock.Arguments) {
		succChan <- struct{}{}
	})
	g2 := &gossipServiceImpl{secAdv: &secAdvMock{}, peerIdentity: api.PeerIdentityType("Org0"), gossipSvc: g2SvcMock}
	g2.configUpdated(&configMock{})
	select {
	case <-time.After(time.Second):
		assert.Fail(t, "Didn't join a channel (should have done so within the time period)")
	case <-succChan:

	}
}
