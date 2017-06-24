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

package discovery

import (
	"fmt"

	"github.com/hyperledger/fabric/gossip/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
)

// CryptoService is an interface that the discovery expects to be implemented and passed on creation
type CryptoService interface {
	// ValidateAliveMsg validates that an Alive message is authentic
	ValidateAliveMsg(message *proto.SignedGossipMessage) bool

	// SignMessage signs a message
	SignMessage(m *proto.GossipMessage, internalEndpoint string) *proto.Envelope
}

// EnvelopeFilter may or may not remove part of the Envelope
// that the given SignedGossipMessage originates from.
type EnvelopeFilter func(message *proto.SignedGossipMessage) *proto.Envelope

// Sieve defines the messages that are allowed to be sent to some remote peer,
// based on some criteria.
// Returns whether the sieve permits sending a given message.
type Sieve func(message *proto.SignedGossipMessage) bool

// DisclosurePolicy defines which messages a given remote peer
// is eligible of knowing about, and also what is it eligible
// to know about out of a given SignedGossipMessage.
// Returns:
// 1) A Sieve for a given remote peer.
//    The Sieve is applied for each peer in question and outputs
//    whether the message should be disclosed to the remote peer.
// 2) A EnvelopeFilter for a given SignedGossipMessage, which may remove
//    part of the Envelope the SignedGossipMessage originates from
type DisclosurePolicy func(remotePeer *NetworkMember) (Sieve, EnvelopeFilter)

// CommService is an interface that the discovery expects to be implemented and passed on creation
type CommService interface {
	// Gossip gossips a message
	Gossip(msg *proto.SignedGossipMessage)

	// SendToPeer sends to a given peer a message.
	// The nonce can be anything since the communication module handles the nonce itself
	SendToPeer(peer *NetworkMember, msg *proto.SignedGossipMessage)

	// Ping probes a remote peer and returns if it's responsive or not
	Ping(peer *NetworkMember) bool

	// Accept returns a read-only channel for membership messages sent from remote peers
	Accept() <-chan *proto.SignedGossipMessage

	// PresumedDead returns a read-only channel for peers that are presumed to be dead
	PresumedDead() <-chan common.PKIidType

	// CloseConn orders to close the connection with a certain peer
	CloseConn(peer *NetworkMember)
}

// NetworkMember is a peer's representation
type NetworkMember struct {
	Endpoint         string
	Metadata         []byte
	PKIid            common.PKIidType
	InternalEndpoint string
}

// String returns a string representation of the NetworkMember
func (n *NetworkMember) String() string {
	return fmt.Sprintf("Endpoint: %s, InternalEndpoint: %s, PKI-ID: %v, Metadata: %v", n.Endpoint, n.InternalEndpoint, n.PKIid, n.Metadata)
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
