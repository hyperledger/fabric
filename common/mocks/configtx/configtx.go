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
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

type Initializer struct {
	// HandlersVal is returned as the result of Handlers()
	HandlersVal map[cb.ConfigItem_ConfigType]configtxapi.Handler

	// PolicyManagerVal is returned as the result of PolicyManager()
	PolicyManagerVal policies.Manager

	// ChannelConfigVal is returned as the result of ChannelConfig()
	ChannelConfigVal configtxapi.ChannelConfig

	// OrdererConfigVal is returned as the result of OrdererConfig()
	OrdererConfigVal configtxapi.OrdererConfig

	// ApplicationConfigVal is returned as the result of ApplicationConfig()
	ApplicationConfigVal configtxapi.ApplicationConfig

	// MSPManagerVal is returned as the result of MSPManager()
	MSPManagerVal msp.MSPManager
}

// Returns the HandlersVal
func (i *Initializer) Handlers() map[cb.ConfigItem_ConfigType]configtxapi.Handler {
	return i.HandlersVal
}

// Returns the PolicyManagerVal
func (i *Initializer) PolicyManager() policies.Manager {
	return i.PolicyManagerVal
}

// Returns the ChannelConfigVal
func (i *Initializer) ChannelConfig() configtxapi.ChannelConfig {
	return i.ChannelConfigVal
}

// Returns the OrdererConfigVal
func (i *Initializer) OrdererConfig() configtxapi.OrdererConfig {
	return i.OrdererConfigVal
}

// Returns the ApplicationConfigVal
func (i *Initializer) ApplicationConfig() configtxapi.ApplicationConfig {
	return i.ApplicationConfigVal
}

// Returns the MSPManagerVal
func (i *Initializer) MSPManager() msp.MSPManager {
	return i.MSPManagerVal
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
