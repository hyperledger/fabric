/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoext

import (
	"fmt"

	"github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
)

// ReceivedMessage is a GossipMessage wrapper that
// enables the user to send a message to the origin from which
// the ReceivedMessage was sent from.
// It also allows to know the identity of the sender,
// to obtain the raw bytes the GossipMessage was un-marshaled from,
// and the signature over these raw bytes.
type ReceivedMessage interface {
	// Respond sends a GossipMessage to the origin from which this ReceivedMessage was sent from
	Respond(msg *gossip.GossipMessage)

	// GetGossipMessage returns the underlying GossipMessage
	GetGossipMessage() *SignedGossipMessage

	// GetSourceMessage Returns the Envelope the ReceivedMessage was
	// constructed with
	GetSourceEnvelope() *gossip.Envelope

	// GetConnectionInfo returns information about the remote peer
	// that sent the message
	GetConnectionInfo() *ConnectionInfo

	// Ack returns to the sender an acknowledgement for the message
	// An ack can receive an error that indicates that the operation related
	// to the message has failed
	Ack(err error)
}

// ConnectionInfo represents information about
// the remote peer that sent a certain ReceivedMessage
type ConnectionInfo struct {
	ID       common.PKIidType
	Auth     *AuthInfo
	Identity api.PeerIdentityType
	Endpoint string
}

// String returns a string representation of this ConnectionInfo
func (c *ConnectionInfo) String() string {
	return fmt.Sprintf("%s %v", c.Endpoint, c.ID)
}

// AuthInfo represents the authentication
// data that was provided by the remote peer
// at the connection time
type AuthInfo struct {
	SignedData []byte
	Signature  []byte
}
