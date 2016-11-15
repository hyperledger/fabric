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

package api

import (
	"time"

	"github.com/hyperledger/fabric/gossip/common"
)

// SecurityAdvisor defines an external auxiliary object
// that provides security and identity related capabilities
type SecurityAdvisor interface {
	// IsInMyOrg returns whether the given peer's certificate represents
	// a peer in the invoker's organization
	IsInMyOrg(PeerCert) bool

	// Verify verifies a JoinChannelMessage, returns nil on success,
	// and an error on failure
	Verify(JoinChannelMessage) error
}

// ChannelNotifier is implemented by the gossip component and is used for the peer
// layer to notify the gossip component of a JoinChannel event
type ChannelNotifier interface {
	JoinChannel(joinMsg JoinChannelMessage, chainID common.ChainID)
}

// JoinChannelMessage is the message that asserts a creation or mutation
// of a channel's membership list, and is the message that is gossipped
// among the peers
type JoinChannelMessage interface {

	// GetTimestamp returns the timestamp of the message's creation
	GetTimestamp() time.Time

	// Members returns all the peers that are in the channel
	Members() []ChannelMember
}

// ChannelMember is a peer's certificate and endpoint (host:port)
type ChannelMember struct {
	Cert PeerCert // Cert defines the certificate of the remote peer
	Host string   // Host is the hostname/ip address of the remote peer
	Port int      // Port is the port the remote peer is listening on
}

// PeerCert defines the cryptographic identity of a peer
type PeerCert []byte
