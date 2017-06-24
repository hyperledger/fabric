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
	"github.com/hyperledger/fabric/gossip/common"
)

func init() {
	// This is just to satisfy the code coverage tool
	// miss any methods
	switch true {

	}
}

// SecurityAdvisor defines an external auxiliary object
// that provides security and identity related capabilities
type SecurityAdvisor interface {
	// OrgByPeerIdentity returns the OrgIdentityType
	// of a given peer identity.
	// If any error occurs, nil is returned.
	// This method does not validate peerIdentity.
	// This validation is supposed to be done appropriately during the execution flow.
	OrgByPeerIdentity(PeerIdentityType) OrgIdentityType
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

	// SequenceNumber returns the sequence number of the configuration block
	// the JoinChannelMessage originated from
	SequenceNumber() uint64

	// Members returns the organizations of the channel
	Members() []OrgIdentityType

	// AnchorPeersOf returns the anchor peers of the given organization
	AnchorPeersOf(org OrgIdentityType) []AnchorPeer
}

// AnchorPeer is an anchor peer's certificate and endpoint (host:port)
type AnchorPeer struct {
	Host string // Host is the hostname/ip address of the remote peer
	Port int    // Port is the port the remote peer is listening on
}

// OrgIdentityType defines the identity of an organization
type OrgIdentityType []byte
