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
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/proto"
)

// Gossip is the interface of the gossip component
type Gossip interface {

	// GetPeers returns a mapping of endpoint --> []discovery.NetworkMember
	GetPeers() []discovery.NetworkMember

	// UpdateMetadata updates the self metadata of the discovery layer
	UpdateMetadata([]byte)

	// Gossip sends a message to other peers to the network
	Gossip(msg *proto.GossipMessage)

	// Accept returns a channel that outputs messages from other peers
	Accept(common.MessageAcceptor) <-chan *proto.GossipMessage

	// Stop stops the gossip component
	Stop()
}

// Config is the configuration of the gossip component
type Config struct {
	BindPort            int
	ID                  string
	SelfEndpoint        string
	BootstrapPeers      []string
	PropagateIterations int
	PropagatePeerNum    int

	MaxMessageCountToStore int

	MaxPropagationBurstSize    int
	MaxPropagationBurstLatency time.Duration

	PullInterval time.Duration
	PullPeerNum  int
}
