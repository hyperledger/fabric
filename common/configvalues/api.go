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

package api

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

// Orderer stores the common shared orderer config
type Orderer interface {
	// ConsensusType returns the configured consensus type
	ConsensusType() string

	// BatchSize returns the maximum number of messages to include in a block
	BatchSize() *ab.BatchSize

	// BatchTimeout returns the amount of time to wait before creating a batch
	BatchTimeout() time.Duration

	// ChainCreationPolicyNames returns the policy names which are allowed for chain creation
	// This field is only set for the system ordering chain
	ChainCreationPolicyNames() []string

	// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
	// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
	// used for ordering
	KafkaBrokers() []string

	// IngressPolicyNames returns the name of the policy to validate incoming broadcast messages against
	IngressPolicyNames() []string

	// EgressPolicyNames returns the name of the policy to validate incoming broadcast messages against
	EgressPolicyNames() []string
}

type ValueProposer interface {
	// BeginValueProposals called when a config proposal is begun
	BeginValueProposals(groups []string) ([]ValueProposer, error)

	// ProposeValue called when config is added to a proposal
	ProposeValue(key string, configValue *cb.ConfigValue) error

	// RollbackProposals called when a config proposal is abandoned
	RollbackProposals()

	// PreCommit is invoked before committing the config to catch
	// any errors which cannot be caught on a per proposal basis
	// TODO, rename other methods to remove Value/Proposal references
	PreCommit() error

	// CommitProposals called when a config proposal is committed
	CommitProposals()
}
