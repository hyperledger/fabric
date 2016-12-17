/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/gossip/comm"
	gossipCommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/integration"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/state"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"google.golang.org/grpc"
)

var (
	gossipServiceInstance *gossipServiceImpl
	once                  sync.Once
)

// GossipService encapsulates gossip and state capabilities into single interface
type GossipService interface {
	gossip.Gossip

	// JoinChannel joins new chain given the configuration block and initialized committer service
	JoinChannel(committer committer.Committer, block *common.Block) error
	// GetBlock returns block for given chain
	GetBlock(chainID string, index uint64) *common.Block
	// AddPayload appends message payload to for given chain
	AddPayload(chainID string, payload *proto.Payload) error
}

type gossipServiceImpl struct {
	gossip gossip.Gossip
	chains map[string]state.GossipStateProvider
	lock   sync.RWMutex
}

// InitGossipService initialize gossip service
func InitGossipService(endpoint string, s *grpc.Server, bootPeers ...string) {
	once.Do(func() {
		gossip := integration.NewGossipComponent(endpoint, s, []grpc.DialOption{}, bootPeers...)
		gossipServiceInstance = &gossipServiceImpl{
			gossip: gossip,
			chains: make(map[string]state.GossipStateProvider),
		}
	})
}

// GetGossipService returns an instance of gossip service
func GetGossipService() GossipService {
	return gossipServiceInstance
}

// Send sends a message to remote peers
func (g *gossipServiceImpl) Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer) {
	g.gossip.Send(msg, peers...)
}

// JoinChannel joins the channel and initialize gossip state with given committer
func (g *gossipServiceImpl) JoinChannel(commiter committer.Committer, block *common.Block) error {
	g.lock.Lock()
	defer g.lock.Unlock()

	if chainID, err := utils.GetChainIDFromBlock(block); err != nil {
		return err
	} else {
		// Initialize new state provider for given committer
		g.chains[chainID] = state.NewGossipStateProvider(g.gossip, commiter)
	}

	return nil
}

// GetPeers returns a mapping of endpoint --> []discovery.NetworkMember
func (g *gossipServiceImpl) GetPeers() []discovery.NetworkMember {
	return g.gossip.GetPeers()
}

// UpdateMetadata updates the self metadata of the discovery layer
func (g *gossipServiceImpl) UpdateMetadata(data []byte) {
	g.gossip.UpdateMetadata(data)
}

// Gossip sends a message to other peers to the network
func (g *gossipServiceImpl) Gossip(msg *proto.GossipMessage) {
	g.gossip.Gossip(msg)
}

// Accept returns a channel that outputs messages from other peers
func (g *gossipServiceImpl) Accept(acceptor gossipCommon.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan comm.ReceivedMessage) {
	return g.gossip.Accept(acceptor, false)
}

// GetBlock returns block for given chain
func (g *gossipServiceImpl) GetBlock(chainID string, index uint64) *common.Block {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.chains[chainID].GetBlock(index)
}

// AddPayload appends message payload to for given chain
func (g *gossipServiceImpl) AddPayload(chainID string, payload *proto.Payload) error {
	g.lock.RLock()
	defer g.lock.RUnlock()
	return g.chains[chainID].AddPayload(payload)
}

// Stop stops the gossip component
func (g *gossipServiceImpl) Stop() {
	for _, ch := range g.chains {
		ch.Stop()
	}
	g.gossip.Stop()
}
