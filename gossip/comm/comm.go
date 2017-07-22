/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"fmt"

	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	proto "github.com/hyperledger/fabric/protos/gossip"
)

// Comm is an object that enables to communicate with other peers
// that also embed a CommModule.
type Comm interface {

	// GetPKIid returns this instance's PKI id
	GetPKIid() common.PKIidType

	// Send sends a message to remote peers
	Send(msg *proto.SignedGossipMessage, peers ...*RemotePeer)

	// Probe probes a remote node and returns nil if its responsive,
	// and an error if it's not.
	Probe(peer *RemotePeer) error

	// Handshake authenticates a remote peer and returns
	// (its identity, nil) on success and (nil, error)
	Handshake(peer *RemotePeer) (api.PeerIdentityType, error)

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// Each message from the channel can be used to send a reply back to the sender
	Accept(common.MessageAcceptor) <-chan proto.ReceivedMessage

	// PresumedDead returns a read-only channel for node endpoints that are suspected to be offline
	PresumedDead() <-chan common.PKIidType

	// CloseConn closes a connection to a certain endpoint
	CloseConn(peer *RemotePeer)

	// Stop stops the module
	Stop()
}

// RemotePeer defines a peer's endpoint and its PKIid
type RemotePeer struct {
	Endpoint string
	PKIID    common.PKIidType
}

// String converts a RemotePeer to a string
func (p *RemotePeer) String() string {
	return fmt.Sprintf("%s, PKIid:%v", p.Endpoint, p.PKIID)
}
