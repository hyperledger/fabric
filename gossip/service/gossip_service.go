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
	"time"

	peerComm "github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/gossip/api"
	gossipCommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/gossip"
	"github.com/hyperledger/fabric/gossip/integration"
	"github.com/hyperledger/fabric/gossip/proto"
	"github.com/hyperledger/fabric/gossip/state"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

var (
	gossipServiceInstance *gossipServiceImpl
	once                  sync.Once
)

type gossipSvc gossip.Gossip

// TODO: This is a temporary join channel struct, to be removed once
// the structure of configuration block in terms of anchor peers will
// be defined and coded. Currently need it to allow the end-to-end
// skeleton to work, having gossip multi chain support.
type joinChanMsg struct {
}

// GetTimestamp returns the timestamp of the message's creation
func (*joinChanMsg) GetTimestamp() time.Time {
	return time.Now()
}

// AnchorPeers returns all the anchor peers that are in the channel
func (*joinChanMsg) AnchorPeers() []api.AnchorPeer {
	return []api.AnchorPeer{{Cert: api.PeerIdentityType(util.GetTestOrgID())}}
}

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
	gossipSvc
	chains map[string]state.GossipStateProvider
	lock   sync.RWMutex
}

var logger = logging.MustGetLogger("gossipService")

// InitGossipService initialize gossip service
func InitGossipService(endpoint string, s *grpc.Server, bootPeers ...string) {
	once.Do(func() {
		logger.Info("Initialize gossip with endpoint", endpoint, "and bootstrap set", bootPeers)
		dialOpts := []grpc.DialOption{}
		if peerComm.TLSEnabled() {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(peerComm.InitTLSForPeer()))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}

		gossip := integration.NewGossipComponent(endpoint, s, dialOpts, bootPeers...)
		gossipServiceInstance = &gossipServiceImpl{
			gossipSvc: gossip,
			chains:    make(map[string]state.GossipStateProvider),
		}
	})
}

// GetGossipService returns an instance of gossip service
func GetGossipService() GossipService {
	return gossipServiceInstance
}

// JoinChannel joins the channel and initialize gossip state with given committer
func (g *gossipServiceImpl) JoinChannel(commiter committer.Committer, block *common.Block) error {
	g.lock.Lock()
	defer g.lock.Unlock()

	if chainID, err := utils.GetChainIDFromBlock(block); err != nil {
		return err
	} else {
		// Initialize new state provider for given committer
		logger.Debug("Creating state provider for chainID", chainID)
		g.chains[chainID] = state.NewGossipStateProvider(chainID, g, commiter)
		g.JoinChan(&joinChanMsg{}, gossipCommon.ChainID(chainID))
	}

	return nil
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
		logger.Info("Stopping chain", ch)
		ch.Stop()
	}
	g.gossipSvc.Stop()
}
