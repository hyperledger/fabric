/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"fmt"
	"time"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/filter"
	"github.com/hyperledger/fabric/gossip/protoext"
	proto "github.com/hyperledger/fabric/protos/gossip"
)

// Gossip is the interface of the gossip component
type Gossip interface {

	// SelfMembershipInfo returns the peer's membership information
	SelfMembershipInfo() discovery.NetworkMember

	// SelfChannelInfo returns the peer's latest StateInfo message of a given channel
	SelfChannelInfo(common.ChannelID) *protoext.SignedGossipMessage

	// Send sends a message to remote peers
	Send(msg *proto.GossipMessage, peers ...*comm.RemotePeer)

	// SendByCriteria sends a given message to all peers that match the given SendCriteria
	SendByCriteria(*protoext.SignedGossipMessage, SendCriteria) error

	// GetPeers returns the NetworkMembers considered alive
	Peers() []discovery.NetworkMember

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChannelID) []discovery.NetworkMember

	// UpdateMetadata updates the self metadata of the discovery layer
	// the peer publishes to other peers
	UpdateMetadata(metadata []byte)

	// UpdateLedgerHeight updates the ledger height the peer
	// publishes to other peers in the channel
	UpdateLedgerHeight(height uint64, channelID common.ChannelID)

	// UpdateChaincodes updates the chaincodes the peer publishes
	// to other peers in the channel
	UpdateChaincodes(chaincode []*proto.Chaincode, channelID common.ChannelID)

	// Gossip sends a message to other peers to the network
	Gossip(msg *proto.GossipMessage)

	// PeerFilter receives a SubChannelSelectionCriteria and returns a RoutingFilter that selects
	// only peer identities that match the given criteria, and that they published their channel participation
	PeerFilter(channel common.ChannelID, messagePredicate api.SubChannelSelectionCriteria) (filter.RoutingFilter, error)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// If passThrough is false, the messages are processed by the gossip layer beforehand.
	// If passThrough is true, the gossip layer doesn't intervene and the messages
	// can be used to send a reply back to the sender
	Accept(acceptor common.MessageAcceptor, passThrough bool) (<-chan *proto.GossipMessage, <-chan protoext.ReceivedMessage)

	// JoinChan makes the Gossip instance join a channel
	JoinChan(joinMsg api.JoinChannelMessage, channelID common.ChannelID)

	// LeaveChan makes the Gossip instance leave a channel.
	// It still disseminates stateInfo message, but doesn't participate
	// in block pulling anymore, and can't return anymore a list of peers
	// in the channel.
	LeaveChan(channelID common.ChannelID)

	// SuspectPeers makes the gossip instance validate identities of suspected peers, and close
	// any connections to peers with identities that are found invalid
	SuspectPeers(s api.PeerSuspector)

	// IdentityInfo returns information known peer identities
	IdentityInfo() api.PeerIdentitySet

	// IsInMyOrg checks whether a network member is in this peer's org
	IsInMyOrg(member discovery.NetworkMember) bool

	// Stop stops the gossip component
	Stop()
}

// emittedGossipMessage encapsulates signed gossip message to compose
// with routing filter to be used while message is forwarded
type emittedGossipMessage struct {
	*protoext.SignedGossipMessage
	filter func(id common.PKIidType) bool
}

// SendCriteria defines how to send a specific message
type SendCriteria struct {
	Timeout    time.Duration        // Timeout defines the time to wait for acknowledgements
	MinAck     int                  // MinAck defines the amount of peers to collect acknowledgements from
	MaxPeers   int                  // MaxPeers defines the maximum number of peers to send the message to
	IsEligible filter.RoutingFilter // IsEligible defines whether a specific peer is eligible of receiving the message
	Channel    common.ChannelID     // Channel specifies a channel to send this message on. \
	// Only peers that joined the channel would receive this message
}

// String returns a string representation of this SendCriteria
func (sc SendCriteria) String() string {
	return fmt.Sprintf("channel: %s, tout: %v, minAck: %d, maxPeers: %d", sc.Channel, sc.Timeout, sc.MinAck, sc.MaxPeers)
}
