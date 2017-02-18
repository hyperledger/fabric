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
	configvalues "github.com/hyperledger/fabric/common/configvalues"
	configvalueschannel "github.com/hyperledger/fabric/common/configvalues/channel"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Manager provides a mechanism to query and update config
type Manager interface {
	Resources

	// Apply attempts to apply a configtx to become the new config
	Apply(configEnv *cb.ConfigEnvelope) error

	// Validate attempts to apply a configtx to become the new config
	Validate(configEnv *cb.ConfigEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)

	// ConfigEnvelope returns the *cb.ConfigEnvelope from the last successful Apply
	ConfigEnvelope() *cb.ConfigEnvelope

	// ChainID retrieves the chain ID associated with this manager
	ChainID() string

	// Sequence returns the current sequence number of the config
	Sequence() uint64
}

// Resources is the common set of config resources for all channels
// Depending on whether chain is used at the orderer or at the peer, other
// config resources may be available
type Resources interface {
	// PolicyManager returns the policies.Manager for the channel
	PolicyManager() policies.Manager

	// ChannelConfig returns the ChannelConfig for the chain
	ChannelConfig() configvalueschannel.ConfigReader

	// OrdererConfig returns the configtxorderer.SharedConfig for the channel
	OrdererConfig() configvalues.Orderer

	// ApplicationConfig returns the configtxapplication.SharedConfig for the channel
	ApplicationConfig() configvalues.Application

	// MSPManager returns the msp.MSPManager for the chain
	MSPManager() msp.MSPManager
}

// Transactional is an interface which allows for an update to be proposed and rolled back
type Transactional interface {
	// RollbackConfig called when a config proposal is abandoned
	RollbackProposals()

	// CommitConfig called when a config proposal is committed
	CommitProposals()
}

// PolicyHandler is used for config updates to policy
type PolicyHandler interface {
	Transactional

	BeginConfig(groups []string) ([]PolicyHandler, error)

	ProposePolicy(key string, path []string, policy *cb.ConfigPolicy) error
}

// Initializer is used as indirection between Manager and Handler to allow
// for single Handlers to handle multiple paths
type Initializer interface {
	// ValueProposer return the root value proposer
	ValueProposer() configvalues.ValueProposer

	// PolicyProposer return the root policy proposer
	PolicyProposer() policies.Proposer

	Resources
}
