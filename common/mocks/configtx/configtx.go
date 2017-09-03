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
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Transactional implements the configtxapi.Transactional
type Transactional struct{}

// PreCommit returns nil
func (t *Transactional) PreCommit(tx interface{}) error { return nil }

// CommitConfig does nothing
func (t *Transactional) CommitProposals(tx interface{}) {}

// RollbackConfig does nothing
func (t *Transactional) RollbackProposals(tx interface{}) {}

// Initializer mocks the configtxapi.Initializer interface
type Initializer struct {
	// PolicyManagerVal is returned by PolicyManager
	PolicyManagerVal *mockpolicies.Manager

	// RootGroupKeyVal is returned by RootGroupKey
	RootGroupKeyVal string
}

// RootGroupKeyreturns RootGroupKeyVal
func (i *Initializer) RootGroupKey() string {
	return i.RootGroupKeyVal
}

// PolicyManager returns PolicyManagerVal
func (i *Initializer) PolicyManager() policies.Manager {
	return i.PolicyManagerVal
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

	// ConfigEnvelopeVal is returned as the value for ConfigEnvelope()
	ConfigEnvelopeVal *cb.ConfigEnvelope
}

// ConfigEnvelope returns the ConfigEnvelopeVal
func (cm *Manager) ConfigEnvelope() *cb.ConfigEnvelope {
	return cm.ConfigEnvelopeVal
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
