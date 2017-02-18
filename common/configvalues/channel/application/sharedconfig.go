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

package application

import (
	api "github.com/hyperledger/fabric/common/configvalues"
	"github.com/hyperledger/fabric/common/configvalues/channel/common/organization"
	"github.com/hyperledger/fabric/common/configvalues/msp"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/op/go-logging"
)

const (
	// GroupKey is the group name for the Application config
	GroupKey = "Application"
)

var orgSchema = &cb.ConfigGroupSchema{
	Groups: map[string]*cb.ConfigGroupSchema{},
	Values: map[string]*cb.ConfigValueSchema{
		AnchorPeersKey:      nil,
		organization.MSPKey: nil, // TODO, consolidate into a constant once common org code exists
	},
	Policies: map[string]*cb.ConfigPolicySchema{
	// TODO, set appropriately once hierarchical policies are implemented
	},
}

var Schema = &cb.ConfigGroupSchema{
	Groups: map[string]*cb.ConfigGroupSchema{
		"": orgSchema,
	},
	Values:   map[string]*cb.ConfigValueSchema{},
	Policies: map[string]*cb.ConfigPolicySchema{
	// TODO, set appropriately once hierarchical policies are implemented
	},
}

var logger = logging.MustGetLogger("common/configtx/handlers/application")

type sharedConfig struct {
	orgs map[string]api.ApplicationOrg
}

// SharedConfigImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.Manager
type SharedConfigImpl struct {
	pendingConfig *sharedConfig
	config        *sharedConfig

	mspConfig *msp.MSPConfigHandler
}

// NewSharedConfigImpl creates a new SharedConfigImpl with the given CryptoHelper
func NewSharedConfigImpl(mspConfig *msp.MSPConfigHandler) *SharedConfigImpl {
	return &SharedConfigImpl{
		config:    &sharedConfig{},
		mspConfig: mspConfig,
	}
}

// BeginValueProposals is used to start a new config proposal
func (di *SharedConfigImpl) BeginValueProposals(groups []string) ([]api.ValueProposer, error) {
	logger.Debugf("Beginning a possible new peer shared config")
	if di.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	di.pendingConfig = &sharedConfig{
		orgs: make(map[string]api.ApplicationOrg),
	}
	orgHandlers := make([]api.ValueProposer, len(groups))
	for i, group := range groups {
		org, ok := di.pendingConfig.orgs[group]
		if !ok {
			org = NewApplicationOrgConfig(group, di.mspConfig)
			di.pendingConfig.orgs[group] = org
		}
		orgHandlers[i] = org.(*ApplicationOrgConfig)
	}
	return orgHandlers, nil
}

// RollbackProposals is used to abandon a new config proposal
func (di *SharedConfigImpl) RollbackProposals() {
	logger.Debugf("Rolling back proposed peer shared config")
	di.pendingConfig = nil
}

// CommitProposals is used to commit a new config proposal
func (di *SharedConfigImpl) CommitProposals() {
	logger.Debugf("Committing new peer shared config")
	if di.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	di.config = di.pendingConfig
	di.pendingConfig = nil
}

// ProposeValue is used to add new config to the config proposal
func (di *SharedConfigImpl) ProposeValue(key string, configValue *cb.ConfigValue) error {
	logger.Warningf("Uknown Peer config item with key %s", key)
	return nil
}

// Organizations returns a map of org ID to ApplicationOrg
func (di *SharedConfigImpl) Organizations() map[string]api.ApplicationOrg {
	return di.config.orgs
}
