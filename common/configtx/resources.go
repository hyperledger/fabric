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
	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/chainconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Resources is the common set of configuration resources for all chains
// Depending on whether chain is used at the orderer or at the peer, other
// configuration resources may be available
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
	Handlers() map[cb.ConfigurationItem_ConfigurationType]Handler
}

type resources struct {
	handlers         map[cb.ConfigurationItem_ConfigurationType]Handler
	policyManager    policies.Manager
	chainConfig      chainconfig.Descriptor
	mspConfigHandler *mspmgmt.MSPConfigHandler
}

// PolicyManager returns the policies.Manager for the chain
func (r *resources) PolicyManager() policies.Manager {
	return r.policyManager
}

// ChainConfig returns the chainconfig.Descriptor for the chain
func (r *resources) ChainConfig() chainconfig.Descriptor {
	return r.chainConfig
}

// MSPManager returns the msp.MSPManager for the chain
func (r *resources) MSPManager() msp.MSPManager {
	return r.mspConfigHandler.GetMSPManager()
}

// Handlers returns the handlers to be used when initializing the configtx.Manager
func (r *resources) Handlers() map[cb.ConfigurationItem_ConfigurationType]Handler {
	return r.handlers
}

// NewInitializer creates a chain initializer for the basic set of common chain resources
func NewInitializer() Initializer {
	mspConfigHandler := &mspmgmt.MSPConfigHandler{}
	policyProviderMap := make(map[int32]policies.Provider)
	for pType := range cb.Policy_PolicyType_name {
		rtype := cb.Policy_PolicyType(pType)
		switch rtype {
		case cb.Policy_UNKNOWN:
			// Do not register a handler
		case cb.Policy_SIGNATURE:
			policyProviderMap[pType] = cauthdsl.NewPolicyProvider(mspConfigHandler)
		case cb.Policy_MSP:
			// Add hook for MSP Handler here
		}
	}

	policyManager := policies.NewManagerImpl(policyProviderMap)
	chainConfig := chainconfig.NewDescriptorImpl()
	handlers := make(map[cb.ConfigurationItem_ConfigurationType]Handler)

	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		rtype := cb.ConfigurationItem_ConfigurationType(ctype)
		switch rtype {
		case cb.ConfigurationItem_Chain:
			handlers[rtype] = chainConfig
		case cb.ConfigurationItem_Policy:
			handlers[rtype] = policyManager
		case cb.ConfigurationItem_MSP:
			handlers[rtype] = mspConfigHandler
		default:
			handlers[rtype] = NewBytesHandler()
		}
	}

	return &resources{
		handlers:         handlers,
		policyManager:    policyManager,
		chainConfig:      chainConfig,
		mspConfigHandler: mspConfigHandler,
	}
}
