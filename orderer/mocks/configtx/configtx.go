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
	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

type Initializer struct {
	// HandlersVal is returned as the result of Handlers()
	HandlersVal map[cb.ConfigurationItem_ConfigurationType]configtx.Handler

	// PolicyManagerVal is returned as the result of PolicyManager()
	PolicyManagerVal policies.Manager

	// ChainConfigVal is returned as the result of ChainConfig()
	ChainConfigVal chainconfig.Descriptor

	// MSPManagerVal is returned as the result of MSPManager()
	MSPManagerVal msp.MSPManager
}

// Returns the HandlersVal
func (i *Initializer) Handlers() map[cb.ConfigurationItem_ConfigurationType]configtx.Handler {
	return i.HandlersVal
}

// Returns the PolicyManagerVal
func (i *Initializer) PolicyManager() policies.Manager {
	return i.PolicyManagerVal
}

// Returns the ChainConfigVal
func (i *Initializer) ChainConfig() chainconfig.Descriptor {
	return i.ChainConfigVal
}

// Returns the MSPManagerVal
func (i *Initializer) MSPManager() msp.MSPManager {
	return i.MSPManagerVal
}

// Manager is a mock implementation of configtx.Manager
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
func (cm *Manager) Apply(configtx *cb.ConfigurationEnvelope) error {
	panic("Unimplemented")
}

// Validate panics
func (cm *Manager) Validate(configtx *cb.ConfigurationEnvelope) error {
	panic("Unimplemented")

}
