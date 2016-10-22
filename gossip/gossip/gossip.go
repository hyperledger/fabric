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

package gossip

import (
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/proto"
	"time"
)

type GossipService interface {

	// GetPeersMetadata returns a mapping of endpoint --> metadata
	GetPeersMetadata() map[string][]byte

	// UpdateMetadata updates the self metadata of the discovery layer
	UpdateMetadata([]byte)

	// Gossip sends a message to other peers to the network
	Gossip(msg *proto.GossipMessage)

	// Accept returns a channel that outputs messages from other peers
	Accept(MessageAcceptor) <-chan *proto.GossipMessage

	// Stop stops the gossip component
	Stop()
}

type MessageAcceptor func(*proto.GossipMessage) bool

type GossipConfig struct {
	BindPort            int
	Id                  string
	SelfEndpoint        string
	BootstrapPeers      []*discovery.NetworkMember
	PropagateIterations int
	PropagatePeerNum    int

	MaxMessageCountToStore int

	MaxPropagationBurstSize    int
	MaxPropagationBurstLatency time.Duration

	PullInterval time.Duration
	PullPeerNum  int
}
