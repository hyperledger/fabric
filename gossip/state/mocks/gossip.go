/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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

func (*GossipMock) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	panic("implement me")
}

func (*GossipMock) Peers() []discovery.NetworkMember {
	panic("implement me")
}

func (*GossipMock) PeersOfChannel(common.ChainID) []discovery.NetworkMember {
	return nil
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
		return nil, args.Get(1).(<-chan proto.ReceivedMessage)
	}
	return args.Get(0).(<-chan *proto.GossipMessage), nil
}

func (g *GossipMock) JoinChan(joinMsg api.JoinChannelMessage, chainID common.ChainID) {
}

func (*GossipMock) Stop() {
}
