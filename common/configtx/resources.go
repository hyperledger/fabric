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
	"github.com/hyperledger/fabric/common/configtx/api"
	configtxapplication "github.com/hyperledger/fabric/common/configtx/handlers/application"
	configtxchannel "github.com/hyperledger/fabric/common/configtx/handlers/channel"
	configtxmsp "github.com/hyperledger/fabric/common/configtx/handlers/msp"
	configtxorderer "github.com/hyperledger/fabric/common/configtx/handlers/orderer"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

type resources struct {
	handlers          map[cb.ConfigItem_ConfigType]api.Handler
	policyManager     policies.Manager
	channelConfig     api.ChannelConfig
	ordererConfig     api.OrdererConfig
	applicationConfig api.ApplicationConfig
	mspConfigHandler  *configtxmsp.MSPConfigHandler
}

// PolicyManager returns the policies.Manager for the chain
func (r *resources) PolicyManager() policies.Manager {
	return r.policyManager
}

// ChannelConfig returns the api.ChannelConfig for the chain
func (r *resources) ChannelConfig() api.ChannelConfig {
	return r.channelConfig
}

// OrdererConfig returns the api.OrdererConfig for the chain
func (r *resources) OrdererConfig() api.OrdererConfig {
	return r.ordererConfig
}

// ApplicationConfig returns the api.ApplicationConfig for the chain
func (r *resources) ApplicationConfig() api.ApplicationConfig {
	return r.applicationConfig
}

// MSPManager returns the msp.MSPManager for the chain
func (r *resources) MSPManager() msp.MSPManager {
	return r.mspConfigHandler
}

// Handlers returns the handlers to be used when initializing the configtx.Manager
func (r *resources) Handlers() map[cb.ConfigItem_ConfigType]api.Handler {
	return r.handlers
}

// NewInitializer creates a chain initializer for the basic set of common chain resources
func NewInitializer() api.Initializer {
	mspConfigHandler := &configtxmsp.MSPConfigHandler{}
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
	channelConfig := configtxchannel.NewSharedConfigImpl()
	ordererConfig := configtxorderer.NewManagerImpl()
	applicationConfig := configtxapplication.NewSharedConfigImpl()
	handlers := make(map[cb.ConfigItem_ConfigType]api.Handler)

	for ctype := range cb.ConfigItem_ConfigType_name {
		rtype := cb.ConfigItem_ConfigType(ctype)
		switch rtype {
		case cb.ConfigItem_CHAIN:
			handlers[rtype] = channelConfig
		case cb.ConfigItem_ORDERER:
			handlers[rtype] = ordererConfig
		case cb.ConfigItem_PEER:
			handlers[rtype] = applicationConfig
		case cb.ConfigItem_POLICY:
			handlers[rtype] = policyManager
		case cb.ConfigItem_MSP:
			handlers[rtype] = mspConfigHandler
		default:
			handlers[rtype] = NewBytesHandler()
		}
	}

	return &resources{
		handlers:          handlers,
		policyManager:     policyManager,
		channelConfig:     channelConfig,
		ordererConfig:     ordererConfig,
		applicationConfig: applicationConfig,
		mspConfigHandler:  mspConfigHandler,
	}
}
