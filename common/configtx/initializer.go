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
	configvaluesapi "github.com/hyperledger/fabric/common/configvalues"
	configvalueschannel "github.com/hyperledger/fabric/common/configvalues/channel"
	configtxapplication "github.com/hyperledger/fabric/common/configvalues/channel/application"
	configtxorderer "github.com/hyperledger/fabric/common/configvalues/channel/orderer"
	configtxmsp "github.com/hyperledger/fabric/common/configvalues/msp"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

type resources struct {
	policyManager     *policies.ManagerImpl
	channelConfig     *configvalueschannel.Config
	ordererConfig     *configtxorderer.ManagerImpl
	applicationConfig *configtxapplication.SharedConfigImpl
	mspConfigHandler  *configtxmsp.MSPConfigHandler
}

// PolicyManager returns the policies.Manager for the chain
func (r *resources) PolicyManager() policies.Manager {
	return r.policyManager
}

// ChannelConfig returns the api.ChannelConfig for the chain
func (r *resources) ChannelConfig() configvalueschannel.ConfigReader {
	return r.channelConfig
}

// OrdererConfig returns the api.OrdererConfig for the chain
func (r *resources) OrdererConfig() configvaluesapi.Orderer {
	return r.ordererConfig
}

// ApplicationConfig returns the api.ApplicationConfig for the chain
func (r *resources) ApplicationConfig() configvaluesapi.Application {
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
		policyManager:     policies.NewManagerImpl(RootGroupKey, policyProviderMap),
		channelConfig:     configvalueschannel.NewConfig(ordererConfig, applicationConfig),
		ordererConfig:     ordererConfig,
		applicationConfig: applicationConfig,
		mspConfigHandler:  mspConfigHandler,
	}
}

type valueProposerRoot struct {
	channelConfig    *configvalueschannel.Config
	mspConfigHandler *configtxmsp.MSPConfigHandler
}

type policyProposerRoot struct {
	policyManager policies.Proposer
}

// BeginValueProposals is used to start a new config proposal
func (v *valueProposerRoot) BeginValueProposals(groups []string) ([]configvaluesapi.ValueProposer, error) {
	if len(groups) != 1 {
		logger.Panicf("Initializer only supports having one root group")
	}
	logger.Debugf("Calling begin for MSP manager")
	v.mspConfigHandler.BeginConfig()
	return []configvaluesapi.ValueProposer{v.channelConfig}, nil
}

// RollbackConfig is used to abandon a new config proposal
func (i *valueProposerRoot) RollbackProposals() {
	logger.Debugf("Calling rollback for MSP manager")
	i.mspConfigHandler.RollbackProposals()
}

// CommitConfig is used to commit a new config proposal
func (i *valueProposerRoot) CommitProposals() {
	logger.Debugf("Calling commit for MSP manager")
	i.mspConfigHandler.CommitProposals()
}

func (i *valueProposerRoot) ProposeValue(key string, value *cb.ConfigValue) error {
	return fmt.Errorf("Programming error, this should never be invoked")
}

// BeginPolicyProposals is used to start a new config proposal
func (p *policyProposerRoot) BeginPolicyProposals(groups []string) ([]policies.Proposer, error) {
	if len(groups) != 1 {
		logger.Panicf("Initializer only supports having one root group")
	}
	return []policies.Proposer{p.policyManager}, nil
}

func (i *policyProposerRoot) ProposePolicy(key string, policy *cb.ConfigPolicy) error {
	return fmt.Errorf("Programming error, this should never be invoked")
}

// RollbackConfig is used to abandon a new config proposal
func (i *policyProposerRoot) RollbackProposals() {}

// CommitConfig is used to commit a new config proposal
func (i *policyProposerRoot) CommitProposals() {}

type initializer struct {
	*resources
	vpr *valueProposerRoot
	ppr *policyProposerRoot
}

// NewInitializer creates a chain initializer for the basic set of common chain resources
func NewInitializer() api.Initializer {
	resources := newResources()
	return &initializer{
		resources: resources,
		vpr: &valueProposerRoot{
			channelConfig:    resources.channelConfig,
			mspConfigHandler: resources.mspConfigHandler,
		},
		ppr: &policyProposerRoot{
			policyManager: resources.policyManager,
		},
	}
}

func (i *initializer) PolicyProposer() policies.Proposer {
	return i.ppr
}

func (i *initializer) ValueProposer() configvaluesapi.ValueProposer {
	return i.vpr
}
