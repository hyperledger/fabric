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
	"github.com/hyperledger/fabric/gossip/discovery"
	"google.golang.org/grpc"
)

type GossipEmitterFactory interface {
	NewGossipEmitter(id string, discSvc discovery.DiscoveryService) GossipService
}

// GossipService is used to publish new blocks to the gossip network
type GossipService interface {
	// payload: Holds the block's content, hash and seqNum
	Publish(payload Payload) error
}

type BindAddress struct {
	Host string
	Port int16
}

// Payload defines an object that contains a ledger block
type Payload struct {
	Data   []byte // The content of the message, possibly encrypted or signed
	Hash   string // The message hash
	SeqNum uint64 // The message sequence number
}

type GossipMemberFactory interface {
	NewGossipMember(discovery.DiscoveryService, ReplicationProvider, MessageCryptoService, MessagePolicyVerifier, *grpc.Server) GossipMember

	NewGossipMemberWithRPCServer(discovery.DiscoveryService, ReplicationProvider, MessageCryptoService, MessagePolicyVerifier, BindAddress) (GossipMember, error)
}

// GossipMember is used to obtain new blocks from the gossip network
type GossipMember interface {
	// RegisterCallback registers a callback that is invoked on messages
	// from startSeq to endSeq and invokes the callback when they arrive
	RegisterCallback(startSeq uint64, endSeq uint64, callback func([]Payload))
}

// ReplicationProvider used by the GossipMember in order to obtain Blocks of
// certain seqNum range to be sent to the requester
type ReplicationProvider interface {
	// GetData used by the gossip component to obtain certain blocks from the ledger.
	// Returns the blocks requested with the given sequence numbers, or an error if
	// some block requested is not available.
	GetData(startSeqNum uint64, endSeqNum uint64) ([]Payload, error)

	// LastBlockSeq used by the gossip component to obtain the last sequence of a block the ledger has.
	LastBlockSeq() uint64
}

// MessageCryptoVerifier verifies the message's authenticity,
// if messages are cryptographically signed
type MessageCryptoService interface {
	// Verify returns nil whether the message and its identifier are authentic,
	// otherwise returns an error
	Verify(seqNum uint64, sender string, payload Payload) error

	// Sign signs the payload
	Sign(sender string, Payload Payload) Payload

	// SignBlob signs a blob
	SignBlob([]byte) []byte

	// VerifyBlob verifies a blob, returns error on failure
	// and nil if the blob is correctly signed
	VerifyBlob(sender string, blob []byte) error
}

// MessagePolicyVerifier verifies whether the message conforms to all required policies,
// and can be safely delivered to the user.
type MessagePolicyVerifier interface {
	Verify(seqNum uint64, sender string, payload Payload) error
}
