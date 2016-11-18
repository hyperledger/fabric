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

package comm

import (
	"fmt"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/proto"
)

// Comm is an object that enables to communicate with other peers
// that also embed a CommModule.
type Comm interface {

	// GetPKIid returns this instance's PKI id
	GetPKIid() common.PKIidType

	// Send sends a message to remote peers
	Send(msg *proto.GossipMessage, peers ...*RemotePeer)

	// Probe probes a remote node and returns nil if its responsive
	Probe(peer *RemotePeer) error

	// Accept returns a dedicated read-only channel for messages sent by other nodes that match a certain predicate.
	// Each message from the channel can be used to send a reply back to the sender
	Accept(common.MessageAcceptor) <-chan ReceivedMessage

	// PresumedDead returns a read-only channel for node endpoints that are suspected to be offline
	PresumedDead() <-chan common.PKIidType

	// CloseConn closes a connection to a certain endpoint
	CloseConn(peer *RemotePeer)

	// Stop stops the module
	Stop()

	// BlackListPKIid prohibits the module communicating with the given PKIid
	BlackListPKIid(PKIid common.PKIidType)
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

// SecurityProvider enables the communication module to perform
// a handshake that authenticates the client to the server and vice versa
type SecurityProvider interface {

	// isEnabled returns whether this
	IsEnabled() bool

	// Sign signs msg with this peers signing key and outputs
	// the signature if no error occurred.
	Sign(msg []byte) ([]byte, error)

	// Verify checks that signature if a valid signature of message under vkID's verification key.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If vkID is nil, then the signature is verified against this validator's verification key.
	Verify(vkID, signature, message []byte) error
}

// ReceivedMessage is a GossipMessage wrapper that
// enables the user to send a message to the origin from which
// the ReceivedMessage was sent from
type ReceivedMessage interface {

	// Respond sends a GossipMessage to the origin from which this ReceivedMessage was sent from
	Respond(msg *proto.GossipMessage)

	// GetGossipMessage returns the underlying GossipMessage
	GetGossipMessage() *proto.GossipMessage
}
