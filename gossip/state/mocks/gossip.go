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

package mocks

import (
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	proto "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/mock"
)

type GossipMock struct {
	mock.Mock
}

func (*GossipMock) SuspectPeers(s api.PeerSuspector) {
	panic("implement me")
}

func (g *GossipMock) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	g.Called(msg, peers)
}

func (*GossipMock) Peers() []discovery.NetworkMember {
	panic("implement me")
}

func (g *GossipMock) PeersOfChannel(chainID common.ChainID) []discovery.NetworkMember {
	args := g.Called(chainID)
	return args.Get(0).([]discovery.NetworkMember)
}

func (*GossipMock) UpdateMetadata(metadata []byte) {
	panic("implement me")
}

func (*GossipMock) UpdateChannelMetadata(metadata []byte, chainID common.ChainID) {

}

func (*GossipMock) Gossip(msg *proto.GossipMessage) {
	panic("implement me")
}

func (g *GossipMock) Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage) {
	args := g.Called(acceptor, passThrough)
	if args.Get(0) == nil {
		return nil, args.Get(1).(chan proto.ReceivedMessage)
	}
	return args.Get(0).(<-chan *proto.GossipMessage), nil
}

func (g *GossipMock) JoinChan(joinMsg api.JoinChannelMessage, chainID common.ChainID) {
}

func (*GossipMock) Stop() {
}
