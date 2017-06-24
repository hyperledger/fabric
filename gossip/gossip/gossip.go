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

	"crypto/tls"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	proto "github.com/hyperledger/fabric/protos/gossip"
)

// Gossip is the interface of the gossip component
type Gossip interface {

	// Send sends a message to remote peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// GetPeers returns the NetworkMembers considered alive
	Peers() []discovery.NetworkMember

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChainID) []discovery.NetworkMember

	// UpdateMetadata updates the self metadata of the discovery layer
	// the peer publishes to other peers
	UpdateMetadata(metadata []byte)

	// UpdateChannelMetadata updates the self metadata the peer
	// publishes to other peers about its channel-related state
	UpdateChannelMetadata(metadata []byte, chainID common.ChainID)

	// Gossip sends a message to other peers to the network
	Gossip(msg *proto.GossipMessage)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan proto.ReceivedMessage)

	// JoinChan makes the Gossip instance join a channel
	JoinChan(joinMsg api.JoinChannelMessage, chainID common.ChainID)

	// SuspectPeers makes the gossip instance validate identities of suspected peers, and close
	// any connections to peers with identities that are found invalid
	SuspectPeers(s api.PeerSuspector)

	// Stop stops the gossip component
	Stop()
}

// Config is the configuration of the gossip component
type Config struct {
	BindPort            int      // Port we bind to, used only for tests
	ID                  string   // ID of this instance
	BootstrapPeers      []string // Peers we connect to at startup
	PropagateIterations int      // Number of times a message is pushed to remote peers
	PropagatePeerNum    int      // Number of peers selected to push messages to

	MaxBlockCountToStore int // Maximum count of blocks we store in memory

	MaxPropagationBurstSize    int           // Max number of messages stored until it triggers a push to remote peers
	MaxPropagationBurstLatency time.Duration // Max time between consecutive message pushes

	PullInterval time.Duration // Determines frequency of pull phases
	PullPeerNum  int           // Number of peers to pull from

	SkipBlockVerification bool // Should we skip verifying block messages or not

	PublishCertPeriod        time.Duration    // Time from startup certificates are included in Alive messages
	PublishStateInfoInterval time.Duration    // Determines frequency of pushing state info messages to peers
	RequestStateInfoInterval time.Duration    // Determines frequency of pulling state info messages from peers
	TLSServerCert            *tls.Certificate // TLS certificate of the peer

	InternalEndpoint string // Endpoint we publish to peers in our organization
	ExternalEndpoint string // Peer publishes this endpoint instead of SelfEndpoint to foreign organizations
}
