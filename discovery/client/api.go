/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"github.com/hyperledger/fabric/protos/discovery"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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

// Client defines the client-side API of the discovery service
type Client interface {
	// Send sends the Request and returns the response, or error on failure
	Send(context.Context, *Request) (Response, error)
}

// Response aggregates several responses from the discovery service
type Response interface {
	// ForChannel returns a ChannelResponse in the context of a given channel
	ForChannel(string) ChannelResponse
}

// ChannelResponse aggregates responses for a given channel
type ChannelResponse interface {
	// Config returns a response for a config query, or error if something went wrong
	Config() (*discovery.ConfigResult, error)

	// Peers returns a response for a peer membership query, or error if something went wrong
	Peers() ([]*Peer, error)

	// Endorsers returns the response for an endorser query for a given
	// chaincode in a given channel context, or error if something went wrong.
	// The method returns a random set of endorsers, such that signatures from all of them
	// combined, satisfy the endorsement policy.
	Endorsers(string) (Endorsers, error)
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
