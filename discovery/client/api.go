/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

var (
	// ErrNotFound defines an error that means that an element wasn't found
	ErrNotFound = errors.New("not found")
)

// Signer signs a message and returns the signature and nil,
// or nil and error on failure
type Signer func(msg []byte) ([]byte, error)

// Dialer connects to the server
type Dialer func() (*grpc.ClientConn, error)

// Response aggregates several responses from the discovery service
type Response interface {
	// ForChannel returns a ChannelResponse in the context of a given channel
	ForChannel(string) ChannelResponse

	// ForLocal returns a LocalResponse in the context of no channel
	ForLocal() LocalResponse
}

// ChannelResponse aggregates responses for a given channel
type ChannelResponse interface {
	// Config returns a response for a config query, or error if something went wrong
	Config() (*discovery.ConfigResult, error)

	// Peers returns a response for a peer membership query, or error if something went wrong
	Peers(invocationChain ...*discovery.ChaincodeCall) ([]*Peer, error)

	// Endorsers returns the response for an endorser query for a given
	// chaincode in a given channel context, or error if something went wrong.
	// The method returns a random set of endorsers, such that signatures from all of them
	// combined, satisfy the endorsement policy.
	// The selection is based on the given selection hints:
	// Filter: Filters and sorts the endorsers
	// The given InvocationChain specifies the chaincode calls (along with collections)
	// that the client passed during the construction of the request
	Endorsers(invocationChain InvocationChain, f Filter) (Endorsers, error)
}

// LocalResponse aggregates responses for a channel-less scope
type LocalResponse interface {
	// Peers returns a response for a local peer membership query, or error if something went wrong
	Peers() ([]*Peer, error)
}

// Endorsers defines a set of peers that are sufficient
// for satisfying some chaincode's endorsement policy
type Endorsers []*Peer

// Peer aggregates identity, membership and channel-scoped information
// of a certain peer.
type Peer struct {
	MSPID            string
	AliveMessage     *gossip.SignedGossipMessage
	StateInfoMessage *gossip.SignedGossipMessage
	Identity         []byte
}
