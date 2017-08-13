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

package config

import (
	"fmt"

	"github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/hyperledger/fabric/common/config"
	configtxmsp "github.com/hyperledger/fabric/common/config/channel/msp"
	"github.com/hyperledger/fabric/common/configtx"
	configtxapi "github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

const RootGroupKey = "Channel"

type resources struct {
	policyManager    *policies.ManagerImpl
	configRoot       *Root
	mspConfigHandler *configtxmsp.MSPConfigHandler
	configtxManager  configtxapi.Manager
}

// PolicyManager returns the policies.Manager for the chain
func (r *resources) PolicyManager() policies.Manager {
	return r.policyManager
}

// ConfigtxManager returns the configtxapi.Manager for the chain
func (r *resources) ConfigtxManager() configtxapi.Manager {
	return r.configtxManager
}

// ChannelConfig returns the api.ChannelConfig for the chain
func (r *resources) ChannelConfig() Channel {
	return r.configRoot.Channel()
}

// OrdererConfig returns the api.OrdererConfig for the chain
func (r *resources) OrdererConfig() (Orderer, bool) {
	result := r.configRoot.Orderer()
	if result == nil {
		return nil, false
	}
	return result, true
}

// ApplicationConfig returns the api.ApplicationConfig for the chain
func (r *resources) ApplicationConfig() (Application, bool) {
	result := r.configRoot.Application()
	if result == nil {
		return nil, false
	}
	return result, true
}

// ConsortiumsConfig returns the api.ConsortiumsConfig for the chain and whether or not
// this channel contains consortiums config
func (r *resources) ConsortiumsConfig() (Consortiums, bool) {
	result := r.configRoot.Consortiums()
	if result == nil {
		return nil, false
	}

	return result, true
}

// MSPManager returns the msp.MSPManager for the chain
func (r *resources) MSPManager() msp.MSPManager {
	return r.mspConfigHandler
}

func newResources() *resources {
	mspConfigHandler := configtxmsp.NewMSPConfigHandler()

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

	return &resources{
		policyManager:    policies.NewManagerImpl(RootGroupKey, policyProviderMap),
		configRoot:       NewRoot(mspConfigHandler),
		mspConfigHandler: mspConfigHandler,
	}
}

type policyProposerRoot struct {
	policyManager                *policies.ManagerImpl
	initializationLogSuppression bool
}

// BeginPolicyProposals is used to start a new config proposal
func (ppr *policyProposerRoot) BeginPolicyProposals(tx interface{}, groups []string) ([]policies.Proposer, error) {
	if len(groups) != 1 {
		logger.Panicf("policyProposerRoot only supports having one root group")
	}
	return []policies.Proposer{ppr.policyManager}, nil
}

func (ppr *policyProposerRoot) ProposePolicy(tx interface{}, key string, policy *cb.ConfigPolicy) (proto.Message, error) {
	return nil, fmt.Errorf("Programming error, this should never be invoked")
}

// PreCommit is a no-op and returns nil
func (ppr *policyProposerRoot) PreCommit(tx interface{}) error {
	return nil
}

// RollbackConfig is used to abandon a new config proposal
func (ppr *policyProposerRoot) RollbackProposals(tx interface{}) {}

// CommitConfig is used to commit a new config proposal
func (ppr *policyProposerRoot) CommitProposals(tx interface{}) {
	if ppr.initializationLogSuppression {
		ppr.initializationLogSuppression = false
	}

	pm := ppr.policyManager
	for _, policyName := range []string{policies.ChannelReaders, policies.ChannelWriters} {
		_, ok := pm.GetPolicy(policyName)
		if !ok {
			logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
		} else {
			logger.Debugf("As expected, current configuration has policy '%s'", policyName)
		}
	}
	if _, ok := pm.Manager([]string{policies.ApplicationPrefix}); ok {
		// Check for default application policies if the application component is defined
		for _, policyName := range []string{
			policies.ChannelApplicationReaders,
			policies.ChannelApplicationWriters,
			policies.ChannelApplicationAdmins} {
			_, ok := pm.GetPolicy(policyName)
			if !ok {
				logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
			} else {
				logger.Debugf("As expected, current configuration has policy '%s'", policyName)
			}
		}
	}
	if _, ok := pm.Manager([]string{policies.OrdererPrefix}); ok {
		for _, policyName := range []string{policies.BlockValidation} {
			_, ok := pm.GetPolicy(policyName)
			if !ok {
				logger.Warningf("Current configuration has no policy '%s', this will likely cause problems in production systems", policyName)
			} else {
				logger.Debugf("As expected, current configuration has policy '%s'", policyName)
			}
		}
	}
}

type Initializer struct {
	*resources
	configtxapi.Manager
	ppr *policyProposerRoot
}

func NewWithOneTimeSuppressedPolicyWarnings(envConfig *cb.Envelope, callOnUpdate []func(*Initializer)) (*Initializer, error) {
	return commonNew(envConfig, callOnUpdate, true)
}

// New creates a new channel config, complete with backing configtx manager
func New(envConfig *cb.Envelope, callOnUpdate []func(*Initializer)) (*Initializer, error) {
	return commonNew(envConfig, callOnUpdate, false)
}

func commonNew(envConfig *cb.Envelope, callOnUpdate []func(*Initializer), suppressFirstPolicyWarnings bool) (*Initializer, error) {
	resources := newResources()
	i := &Initializer{
		resources: resources,
		ppr: &policyProposerRoot{
			policyManager:                resources.policyManager,
			initializationLogSuppression: suppressFirstPolicyWarnings,
		},
	}
	var err error
	initialized := false
	i.Manager, err = configtx.NewManagerImpl(envConfig, i, []func(cm configtxapi.Manager){
		func(cm configtxapi.Manager) {
			// This gets invoked once at instantiation before we are ready for it
			// we manually do this right after
			if !initialized {
				return
			}
			for _, callback := range callOnUpdate {
				callback(i)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	initialized = true
	i.resources.configtxManager = i.Manager
	for _, callback := range callOnUpdate {
		callback(i)
	}
	return i, err
}

func (i *Initializer) RootGroupKey() string {
	return RootGroupKey
}

func (i *Initializer) PolicyProposer() policies.Proposer {
	return i.ppr
}

func (i *Initializer) ValueProposer() config.ValueProposer {
	return i.resources.configRoot
}
