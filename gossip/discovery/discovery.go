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

import "github.com/hyperledger/fabric/gossip/proto"

// CryptoService is an interface that the discovery expects to be implemented and passed on creation
type CryptoService interface {
	// validateAliveMsg validates that an Alive message is authentic
	ValidateAliveMsg(*proto.AliveMessage) bool

	// SignMessage signs an AliveMessage and updates its signature field
	SignMessage(*proto.AliveMessage) *proto.AliveMessage
}

// CommService is an interface that the discovery expects to be implemented and passed on creation
type CommService interface {
	// Gossip gossips a message
	Gossip(msg *proto.GossipMessage)

	// SendToPeer sends to a given peer a message.
	// The nonce can be anything since the communication module handles the nonce itself
	SendToPeer(peer *NetworkMember, msg *proto.GossipMessage)

	// Ping probes a remote peer and returns if it's responsive or not
	Ping(peer *NetworkMember) bool

	// Accept returns a read-only channel for membership messages sent from remote peers
	Accept() <-chan GossipMsg

	// PresumedDead returns a read-only channel for peers that are presumed to be dead
	PresumedDead() <-chan string

	// CloseConn orders to close the connection with a certain peer
	CloseConn(id string)
}

type GossipMsg interface {
	GetGossipMessage() *proto.GossipMessage
}

type NetworkMember struct {
	Id       string
	Endpoint string
	Metadata []byte
	PKIid	 []byte
}

type DiscoveryService interface {

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
}
