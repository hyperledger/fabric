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
	"fmt"

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
	policyManager     *policies.ManagerImpl
	channelConfig     *configtxchannel.SharedConfigImpl
	ordererConfig     *configtxorderer.ManagerImpl
	applicationConfig *configtxapplication.SharedConfigImpl
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

func newResources() *resources {
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

	ordererConfig := configtxorderer.NewManagerImpl(mspConfigHandler)
	applicationConfig := configtxapplication.NewSharedConfigImpl(mspConfigHandler)

	return &resources{
		policyManager:     policies.NewManagerImpl(policyProviderMap),
		channelConfig:     configtxchannel.NewSharedConfigImpl(ordererConfig, applicationConfig),
		ordererConfig:     ordererConfig,
		applicationConfig: applicationConfig,
		mspConfigHandler:  mspConfigHandler,
	}

}

type initializer struct {
	*resources
	is map[string]api.Initializer
}

// NewInitializer creates a chain initializer for the basic set of common chain resources
func NewInitializer() api.Initializer {
	return &initializer{
		resources: newResources(),
	}
}

// BeginConfig is used to start a new config proposal
func (i *initializer) BeginConfig() {
	i.policyManager.BeginConfig()
	i.channelConfig.BeginConfig()
	i.ordererConfig.BeginConfig()
	i.applicationConfig.BeginConfig()
	i.mspConfigHandler.BeginConfig()
}

// RollbackConfig is used to abandon a new config proposal
func (i *initializer) RollbackConfig() {
	i.policyManager.RollbackConfig()
	i.channelConfig.RollbackConfig()
	i.ordererConfig.RollbackConfig()
	i.applicationConfig.RollbackConfig()
	i.mspConfigHandler.RollbackConfig()
}

// CommitConfig is used to commit a new config proposal
func (i *initializer) CommitConfig() {
	i.policyManager.CommitConfig()
	i.channelConfig.CommitConfig()
	i.ordererConfig.CommitConfig()
	i.applicationConfig.CommitConfig()
	i.mspConfigHandler.CommitConfig()
}

func (i *initializer) PolicyHandler() api.PolicyHandler {
	return i.policyManager
}

func (i *initializer) Handler(path []string) (api.Handler, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("Empty path")
	}

	switch path[0] {
	case RootGroupKey:
		return i.channelConfig.Handler(path[1:])
	default:
		return nil, fmt.Errorf("Unknown root group: %s", path[0])
	}

}
