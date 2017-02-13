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
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

type Resources struct {
	// PolicyManagerVal is returned as the result of PolicyManager()
	PolicyManagerVal *mockpolicies.Manager

	// ChannelConfigVal is returned as the result of ChannelConfig()
	ChannelConfigVal configtxapi.ChannelConfig

	// OrdererConfigVal is returned as the result of OrdererConfig()
	OrdererConfigVal configtxapi.OrdererConfig

	// ApplicationConfigVal is returned as the result of ApplicationConfig()
	ApplicationConfigVal configtxapi.ApplicationConfig

	// MSPManagerVal is returned as the result of MSPManager()
	MSPManagerVal msp.MSPManager
}

// Returns the PolicyManagerVal
func (r *Resources) PolicyManager() policies.Manager {
	return r.PolicyManagerVal
}

// Returns the ChannelConfigVal
func (r *Resources) ChannelConfig() configtxapi.ChannelConfig {
	return r.ChannelConfigVal
}

// Returns the OrdererConfigVal
func (r *Resources) OrdererConfig() configtxapi.OrdererConfig {
	return r.OrdererConfigVal
}

// Returns the ApplicationConfigVal
func (r *Resources) ApplicationConfig() configtxapi.ApplicationConfig {
	return r.ApplicationConfigVal
}

// Returns the MSPManagerVal
func (r *Resources) MSPManager() msp.MSPManager {
	return r.MSPManagerVal
}

// Initializer mocks the configtxapi.Initializer interface
type Initializer struct {
	Resources

	// HandlersVal is returned as the result of Handlers()
	HandlerVal configtxapi.Handler
}

// Returns the HandlersVal
func (i *Initializer) Handler(path []string) (configtxapi.Handler, error) {
	return i.HandlerVal, nil
}

func (i *Initializer) PolicyProposer() configtxapi.Handler {
	panic("Unimplemented")
}

// BeginConfig calls through to the HandlerVal
func (i *Initializer) BeginConfig() {}

// CommitConfig calls through to the HandlerVal
func (i *Initializer) CommitConfig() {}

// RollbackConfig calls through to the HandlerVal
func (i *Initializer) RollbackConfig() {}

// Handler mocks the configtxapi.Handler interface
type Handler struct {
	LastKey               string
	LastValue             *cb.ConfigValue
	ErrorForProposeConfig error
}

// ProposeConfig sets LastKey to key, and LastValue to configValue, returning ErrorForProposedConfig
func (h *Handler) ProposeConfig(key string, configValue *cb.ConfigValue) error {
	h.LastKey = key
	h.LastValue = configValue
	return h.ErrorForProposeConfig
}

// Manager is a mock implementation of configtxapi.Manager
type Manager struct {
	Initializer

	// ChainIDVal is returned as the result of ChainID()
	ChainIDVal string

	// SequenceVal is returned as the result of Sequence()
	SequenceVal uint64
}

// ConsensusType returns the ConsensusTypeVal
func (cm *Manager) ChainID() string {
	return cm.ChainIDVal
}

// BatchSize returns the BatchSizeVal
func (cm *Manager) Sequence() uint64 {
	return cm.SequenceVal
}

// Apply panics
func (cm *Manager) Apply(configtx *cb.ConfigEnvelope) error {
	panic("Unimplemented")
}

// Validate panics
func (cm *Manager) Validate(configtx *cb.ConfigEnvelope) error {
	panic("Unimplemented")

}
