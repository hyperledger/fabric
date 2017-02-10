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
	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Handler provides a hook which allows other pieces of code to participate in config proposals
type Handler interface {
	// BeginConfig called when a config proposal is begun
	BeginConfig()

	// RollbackConfig called when a config proposal is abandoned
	RollbackConfig()

	// CommitConfig called when a config proposal is committed
	CommitConfig()

	// ProposeConfig called when config is added to a proposal
	ProposeConfig(configItem *cb.ConfigItem) error
}

// Manager provides a mechanism to query and update config
type Manager interface {
	Resources

	// Apply attempts to apply a configtx to become the new config
	Apply(configtx *cb.ConfigEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	Validate(configtx *cb.ConfigEnvelope) error

	// ChainID retrieves the chain ID associated with this manager
	ChainID() string

	// Sequence returns the current sequence number of the config
	Sequence() uint64
}

// Resources is the common set of config resources for all chains
// Depending on whether chain is used at the orderer or at the peer, other
// config resources may be available
type Resources interface {
	// PolicyManager returns the policies.Manager for the chain
	PolicyManager() policies.Manager

	// ChainConfig returns the chainconfig.Descriptor for the chain
	ChainConfig() chainconfig.Descriptor

	// MSPManager returns the msp.MSPManager for the chain
	MSPManager() msp.MSPManager
}

// Initializer is a structure which is only useful before a configtx.Manager
// has been instantiated for a chain, afterwards, it is of no utility, which
// is why it embeds the Resources interface
type Initializer interface {
	Resources
	// Handlers returns the handlers to be used when initializing the configtx.Manager
	Handlers() map[cb.ConfigItem_ConfigType]Handler
}
