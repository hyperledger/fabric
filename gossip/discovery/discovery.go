/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"fmt"

	protolib "github.com/golang/protobuf/proto"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/protoext"
)

// CryptoService is an interface that the discovery expects to be implemented and passed on creation
type CryptoService interface {
	// ValidateAliveMsg validates that an Alive message is authentic
	ValidateAliveMsg(message *protoext.SignedGossipMessage) bool

	// SignMessage signs a message
	SignMessage(m *proto.GossipMessage, internalEndpoint string) *proto.Envelope
}

// EnvelopeFilter may or may not remove part of the Envelope
// that the given SignedGossipMessage originates from.
type EnvelopeFilter func(message *protoext.SignedGossipMessage) *proto.Envelope

// Sieve defines the messages that are allowed to be sent to some remote peer,
// based on some criteria.
// Returns whether the sieve permits sending a given message.
type Sieve func(message *protoext.SignedGossipMessage) bool

// DisclosurePolicy defines which messages a given remote peer
// is eligible of knowing about, and also what is it eligible
// to know about out of a given SignedGossipMessage.
// Returns:
//  1. A Sieve for a given remote peer.
//     The Sieve is applied for each peer in question and outputs
//     whether the message should be disclosed to the remote peer.
//  2. A EnvelopeFilter for a given SignedGossipMessage, which may remove
//     part of the Envelope the SignedGossipMessage originates from
type DisclosurePolicy func(remotePeer *NetworkMember) (Sieve, EnvelopeFilter)

// CommService is an interface that the discovery expects to be implemented and passed on creation
type CommService interface {
	// Gossip gossips a message
	Gossip(msg *protoext.SignedGossipMessage)

	// SendToPeer sends to a given peer a message.
	// The nonce can be anything since the communication module handles the nonce itself
	SendToPeer(peer *NetworkMember, msg *protoext.SignedGossipMessage)

	// Ping probes a remote peer and returns if it's responsive or not
	Ping(peer *NetworkMember) bool

	// Accept returns a read-only channel for membership messages sent from remote peers
	Accept() <-chan protoext.ReceivedMessage

	// PresumedDead returns a read-only channel for peers that are presumed to be dead
	PresumedDead() <-chan common.PKIidType

	// CloseConn orders to close the connection with a certain peer
	CloseConn(peer *NetworkMember)

	// Forward sends message to the next hop, excluding the hop
	// from which message was initially received
	Forward(msg protoext.ReceivedMessage)

	// IdentitySwitch returns a read-only channel about identity change events
	IdentitySwitch() <-chan common.PKIidType
}

// AnchorPeerTracker is an interface that is passed to discovery to check if an endpoint is an anchor peer
type AnchorPeerTracker interface {
	IsAnchorPeer(endpoint string) bool
}

// NetworkMember is a peer's representation
type NetworkMember struct {
	Endpoint         string
	Metadata         []byte
	PKIid            common.PKIidType
	InternalEndpoint string
	Properties       *proto.Properties
	*proto.Envelope
}

// Clone clones the NetworkMember
func (n NetworkMember) Clone() NetworkMember {
	pkiIDClone := make(common.PKIidType, len(n.PKIid))
	copy(pkiIDClone, n.PKIid)
	nmClone := NetworkMember{
		Endpoint:         n.Endpoint,
		Metadata:         n.Metadata,
		InternalEndpoint: n.InternalEndpoint,
		PKIid:            pkiIDClone,
	}

	if n.Properties != nil {
		nmClone.Properties = protolib.Clone(n.Properties).(*proto.Properties)
	}

	if n.Envelope != nil {
		nmClone.Envelope = protolib.Clone(n.Envelope).(*proto.Envelope)
	}

	return nmClone
}

// String returns a string representation of the NetworkMember
func (n NetworkMember) String() string {
	return fmt.Sprintf("Endpoint: %s, InternalEndpoint: %s, PKI-ID: %s, Metadata: %x", n.Endpoint, n.InternalEndpoint, n.PKIid, n.Metadata)
}

// PreferredEndpoint computes the endpoint to connect to,
// while preferring internal endpoint over the standard
// endpoint
func (n NetworkMember) PreferredEndpoint() string {
	if n.InternalEndpoint != "" {
		return n.InternalEndpoint
	}
	return n.Endpoint
}

// PeerIdentification encompasses a remote peer's
// PKI-ID and whether its in the same org as the current
// peer or not
type PeerIdentification struct {
	ID      common.PKIidType
	SelfOrg bool
}

type identifier func() (*PeerIdentification, error)

// Discovery is the interface that represents a discovery module
type Discovery interface {
	// Lookup returns a network member, or nil if not found
	Lookup(PKIID common.PKIidType) *NetworkMember

	// Self returns this instance's membership information
	Self() NetworkMember

	// UpdateMetadata updates this instance's metadata
	UpdateMetadata([]byte)

	// UpdateEndpoint updates this instance's endpoint
	UpdateEndpoint(string)

	// Stops this instance
	Stop()

	// GetMembership returns the alive members in the view
	GetMembership() []NetworkMember

	// InitiateSync makes the instance ask a given number of peers
	// for their membership information
	InitiateSync(peerNum int)

	// Connect makes this instance to connect to a remote instance
	// The identifier param is a function that can be used to identify
	// the peer, and to assert its PKI-ID, whether its in the peer's org or not,
	// and whether the action was successful or not
	Connect(member NetworkMember, id identifier)
}

// Members represents an aggregation of NetworkMembers
type Members []NetworkMember

// ByID returns a mapping from the PKI-IDs (in string form)
// to NetworkMember
func (members Members) ByID() map[string]NetworkMember {
	res := make(map[string]NetworkMember, len(members))
	for _, peer := range members {
		res[string(peer.PKIid)] = peer
	}
	return res
}

// Intersect returns the intersection of 2 Members
func (members Members) Intersect(otherMembers Members) Members {
	var res Members
	m := otherMembers.ByID()
	for _, member := range members {
		if _, exists := m[string(member.PKIid)]; exists {
			res = append(res, member)
		}
	}
	return res
}

// Filter returns only members that satisfy the given filter
func (members Members) Filter(filter func(member NetworkMember) bool) Members {
	var res Members
	for _, member := range members {
		if filter(member) {
			res = append(res, member)
		}
	}
	return res
}

// Map invokes the given function to every NetworkMember among the Members
func (members Members) Map(f func(member NetworkMember) NetworkMember) Members {
	var res Members
	for _, m := range members {
		res = append(res, f(m))
	}
	return res
}

// HaveExternalEndpoints selects network members that have external endpoints
func HasExternalEndpoint(member NetworkMember) bool {
	return member.Endpoint != ""
}
