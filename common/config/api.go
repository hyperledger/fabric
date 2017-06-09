/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

// Note, the directory is still configvalues, but this is stuttery and config
// is a more accurate and better name, TODO, update directory
package config

import (
	"time"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Org stores the common organizational config
type Org interface {
	// Name returns the name this org is referred to in config
	Name() string

	// MSPID returns the MSP ID associated with this org
	MSPID() string
}

// ApplicationOrg stores the per org application config
type ApplicationOrg interface {
	Org

	// AnchorPeers returns the list of gossip anchor peers
	AnchorPeers() []*pb.AnchorPeer
}

// Application stores the common shared application config
type Application interface {
	// Organizations returns a map of org ID to ApplicationOrg
	Organizations() map[string]ApplicationOrg
}

// Channel gives read only access to the channel configuration
type Channel interface {
	// HashingAlgorithm returns the default algorithm to be used when hashing
	// such as computing block hashes, and CreationPolicy digests
	HashingAlgorithm() func(input []byte) []byte

	// BlockDataHashingStructureWidth returns the width to use when constructing the
	// Merkle tree to compute the BlockData hash
	BlockDataHashingStructureWidth() uint32

	// OrdererAddresses returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
	OrdererAddresses() []string
}

// Consortiums represents the set of consortiums serviced by an ordering service
type Consortiums interface {
	// Consortiums returns the set of consortiums
	Consortiums() map[string]Consortium
}

// Consortium represents a group of orgs which may create channels together
type Consortium interface {
	// ChannelCreationPolicy returns the policy to check when instantiating a channel for this consortium
	ChannelCreationPolicy() *cb.Policy
}

// Orderer stores the common shared orderer config
type Orderer interface {
	// ConsensusType returns the configured consensus type
	ConsensusType() string

	// BatchSize returns the maximum number of messages to include in a block
	BatchSize() *ab.BatchSize

	// BatchTimeout returns the amount of time to wait before creating a batch
	BatchTimeout() time.Duration

	// MaxChannelsCount returns the maximum count of channels to allow for an ordering network
	MaxChannelsCount() uint64

	// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
	// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
	// used for ordering
	KafkaBrokers() []string

	// Organizations returns the organizations for the ordering service
	Organizations() map[string]Org
}

type ValueProposer interface {
	// BeginValueProposals called when a config proposal is begun
	BeginValueProposals(tx interface{}, groups []string) (ValueDeserializer, []ValueProposer, error)

	// RollbackProposals called when a config proposal is abandoned
	RollbackProposals(tx interface{})

	// PreCommit is invoked before committing the config to catch
	// any errors which cannot be caught on a per proposal basis
	// TODO, rename other methods to remove Value/Proposal references
	PreCommit(tx interface{}) error

	// CommitProposals called when a config proposal is committed
	CommitProposals(tx interface{})
}
