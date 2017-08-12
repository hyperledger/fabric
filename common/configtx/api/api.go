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
	"github.com/hyperledger/fabric/common/config"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

// Manager provides a mechanism to query and update config
type Manager interface {
	// Apply attempts to apply a configtx to become the new config
	Apply(configEnv *cb.ConfigEnvelope) error

	// Validate attempts to apply a configtx to become the new config
	Validate(configEnv *cb.ConfigEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)

	// ChainID retrieves the chain ID associated with this manager
	ChainID() string

	// ConfigEnvelope returns the current config envelope
	ConfigEnvelope() *cb.ConfigEnvelope

	// Sequence returns the current sequence number of the config
	Sequence() uint64
}

// Transactional is an interface which allows for an update to be proposed and rolled back
type Transactional interface {
	// RollbackConfig called when a config proposal is abandoned
	RollbackProposals(tx interface{})

	// PreCommit verifies that the transaction can be committed successfully
	PreCommit(tx interface{}) error

	// CommitConfig called when a config proposal is committed
	CommitProposals(tx interface{})
}

// PolicyHandler is used for config updates to policy
type PolicyHandler interface {
	Transactional

	BeginConfig(tx interface{}, groups []string) ([]PolicyHandler, error)

	ProposePolicy(tx interface{}, key string, path []string, policy *cb.ConfigPolicy) (proto.Message, error)
}

// Proposer contains the references necessary to appropriately unmarshal
// a cb.ConfigGroup
type Proposer interface {
	// ValueProposer return the root value proposer
	ValueProposer() config.ValueProposer

	// PolicyProposer return the root policy proposer
	PolicyProposer() policies.Proposer

	// RootGroupKey is the string to use to namespace the root group
	RootGroupKey() string

	// PolicyManager() returns the policy manager for considering config changes
	PolicyManager() policies.Manager
}
