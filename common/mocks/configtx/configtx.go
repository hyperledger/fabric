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

package configtx

import (
	configvaluesapi "github.com/hyperledger/fabric/common/configvalues"
	configvalueschannel "github.com/hyperledger/fabric/common/configvalues/channel"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

type Resources struct {
	// PolicyManagerVal is returned as the result of PolicyManager()
	PolicyManagerVal *mockpolicies.Manager

	// ChannelConfigVal is returned as the result of ChannelConfig()
	ChannelConfigVal configvalueschannel.ConfigReader

	// OrdererConfigVal is returned as the result of OrdererConfig()
	OrdererConfigVal configvaluesapi.Orderer

	// ApplicationConfigVal is returned as the result of ApplicationConfig()
	ApplicationConfigVal configvaluesapi.Application

	// MSPManagerVal is returned as the result of MSPManager()
	MSPManagerVal msp.MSPManager
}

// Returns the PolicyManagerVal
func (r *Resources) PolicyManager() policies.Manager {
	return r.PolicyManagerVal
}

// Returns the ChannelConfigVal
func (r *Resources) ChannelConfig() configvalueschannel.ConfigReader {
	return r.ChannelConfigVal
}

// Returns the OrdererConfigVal
func (r *Resources) OrdererConfig() configvaluesapi.Orderer {
	return r.OrdererConfigVal
}

// Returns the ApplicationConfigVal
func (r *Resources) ApplicationConfig() configvaluesapi.Application {
	return r.ApplicationConfigVal
}

// Returns the MSPManagerVal
func (r *Resources) MSPManager() msp.MSPManager {
	return r.MSPManagerVal
}

// Transactional implements the configtxapi.Transactional
type Transactional struct{}

// PreCommit returns nil
func (t *Transactional) PreCommit() error { return nil }

// CommitConfig does nothing
func (t *Transactional) CommitProposals() {}

// RollbackConfig does nothing
func (t *Transactional) RollbackProposals() {}

// Initializer mocks the configtxapi.Initializer interface
type Initializer struct {
	Resources

	// PolicyProposerVal is returned by PolicyProposers
	PolicyProposerVal *PolicyProposer

	// ValueProposerVal is returned by ValueProposers
	ValueProposerVal *ValueProposer
}

// PolicyProposers returns PolicyProposerVal
func (i *Initializer) PolicyProposer() policies.Proposer {
	return i.PolicyProposerVal
}

// ValueProposers returns ValueProposerVal
func (i *Initializer) ValueProposer() configvaluesapi.ValueProposer {
	return i.ValueProposerVal
}

// PolicyProposer mocks the policies.Proposer interface
type PolicyProposer struct {
	Transactional
	LastKey               string
	LastPolicy            *cb.ConfigPolicy
	ErrorForProposePolicy error
}

// ProposeConfig sets LastKey to key, LastPath to path, and LastPolicy to configPolicy, returning ErrorForProposedConfig
func (pp *PolicyProposer) ProposePolicy(key string, configPolicy *cb.ConfigPolicy) error {
	pp.LastKey = key
	pp.LastPolicy = configPolicy
	return pp.ErrorForProposePolicy
}

// BeginConfig will be removed in the future
func (pp *PolicyProposer) BeginPolicyProposals(groups []string) ([]policies.Proposer, error) {
	handlers := make([]policies.Proposer, len(groups))
	for i := range handlers {
		handlers[i] = pp
	}
	return handlers, nil
}

// Handler mocks the configtxapi.Handler interface
type ValueProposer struct {
	Transactional
	LastKey               string
	LastValue             *cb.ConfigValue
	ErrorForProposeConfig error
}

// ProposeConfig sets LastKey to key, and LastValue to configValue, returning ErrorForProposedConfig
func (vp *ValueProposer) ProposeValue(key string, configValue *cb.ConfigValue) error {
	vp.LastKey = key
	vp.LastValue = configValue
	return vp.ErrorForProposeConfig
}

// BeginConfig returns slices populated by self
func (vp *ValueProposer) BeginValueProposals(groups []string) ([]configvaluesapi.ValueProposer, error) {
	handlers := make([]configvaluesapi.ValueProposer, len(groups))
	for i := range handlers {
		handlers[i] = vp
	}
	return handlers, nil
}

// Manager is a mock implementation of configtxapi.Manager
type Manager struct {
	Initializer

	// ChainIDVal is returned as the result of ChainID()
	ChainIDVal string

	// SequenceVal is returned as the result of Sequence()
	SequenceVal uint64

	// ApplyVal is returned by Apply
	ApplyVal error

	// AppliedConfigUpdateEnvelope is set by Apply
	AppliedConfigUpdateEnvelope *cb.ConfigEnvelope

	// ValidateVal is returned by Validate
	ValidateVal error

	// ProposeConfigUpdateError is returned as the error value for ProposeConfigUpdate
	ProposeConfigUpdateError error

	// ProposeConfigUpdateVal is returns as the value for ProposeConfigUpdate
	ProposeConfigUpdateVal *cb.ConfigEnvelope
}

// ConfigEnvelope is currently unimplemented
func (cm *Manager) ConfigEnvelope() *cb.ConfigEnvelope {
	panic("Unimplemented")
}

// ConsensusType returns the ConsensusTypeVal
func (cm *Manager) ChainID() string {
	return cm.ChainIDVal
}

// BatchSize returns the BatchSizeVal
func (cm *Manager) Sequence() uint64 {
	return cm.SequenceVal
}

// ProposeConfigUpdate
func (cm *Manager) ProposeConfigUpdate(update *cb.Envelope) (*cb.ConfigEnvelope, error) {
	return cm.ProposeConfigUpdateVal, cm.ProposeConfigUpdateError
}

// Apply returns ApplyVal
func (cm *Manager) Apply(configEnv *cb.ConfigEnvelope) error {
	cm.AppliedConfigUpdateEnvelope = configEnv
	return cm.ApplyVal
}

// Validate returns ValidateVal
func (cm *Manager) Validate(configEnv *cb.ConfigEnvelope) error {
	return cm.ValidateVal
}
