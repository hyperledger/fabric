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

package organization

import (
	"fmt"

	"github.com/hyperledger/fabric/common/configvalues"
	mspconfig "github.com/hyperledger/fabric/common/configvalues/msp"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	mspprotos "github.com/hyperledger/fabric/protos/msp"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

// Org config keys
const (
	// MSPKey is value key for marshaled *mspconfig.MSPConfig
	MSPKey = "MSP"
)

var logger = logging.MustGetLogger("common/configtx/handlers")

type orgConfig struct {
	msp msp.MSP
}

// SharedConfigImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.Manager
type OrgConfig struct {
	id            string
	mspID         string
	pendingConfig *orgConfig
	config        *orgConfig

	mspConfig *mspconfig.MSPConfigHandler
}

// NewSharedConfigImpl creates a new SharedConfigImpl with the given CryptoHelper
func NewOrgConfig(id string, mspConfig *mspconfig.MSPConfigHandler) *OrgConfig {
	return &OrgConfig{
		id:        id,
		config:    &orgConfig{},
		mspConfig: mspConfig,
	}
}

// BeginValueProposals is used to start a new config proposal
func (oc *OrgConfig) BeginValueProposals(groups []string) ([]api.ValueProposer, error) {
	logger.Debugf("Beginning a possible new org config")
	if len(groups) != 0 {
		return nil, fmt.Errorf("Orgs do not support sub-groups")
	}

	if oc.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	oc.pendingConfig = &orgConfig{}
	return nil, nil
}

// RollbackProposals is used to abandon a new config proposal
func (oc *OrgConfig) RollbackProposals() {
	logger.Debugf("Rolling back proposed org config")
	oc.pendingConfig = nil
}

// CommitProposals is used to commit a new config proposal
func (oc *OrgConfig) CommitProposals() {
	logger.Debugf("Committing new org config")
	if oc.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	oc.config = oc.pendingConfig
	oc.pendingConfig = nil
}

// Name returns the name this org is referred to in config
func (oc *OrgConfig) Name() string {
	return oc.id
}

// MSPID returns the MSP ID associated with this org
func (oc *OrgConfig) MSPID() string {
	return oc.mspID
}

// ProposeValue is used to add new config to the config proposal
func (oc *OrgConfig) ProposeValue(key string, configValue *cb.ConfigValue) error {
	switch key {
	case MSPKey:
		logger.Debugf("Initializing org MSP for id %s", oc.id)

		mspconfig := &mspprotos.MSPConfig{}
		err := proto.Unmarshal(configValue.Value, mspconfig)
		if err != nil {
			return fmt.Errorf("Error unmarshalling msp config for org %s, err %s", oc.id, err)
		}

		logger.Debugf("Setting up MSP")
		msp, err := oc.mspConfig.ProposeMSP(mspconfig)
		if err != nil {
			return err
		}

		mspID, err := msp.GetIdentifier()
		if err != nil {
			return fmt.Errorf("Could not extract msp identifier for org %s, err %s", oc.id, err)
		}

		if mspID == "" {
			return fmt.Errorf("MSP for org %s has empty MSP ID", oc.id)
		}

		// If the mspID has never been initialized, store it to ensure immutability
		if oc.mspID == "" {
			oc.mspID = mspID
		}

		if mspID != oc.mspID {
			return fmt.Errorf("MSP for org %s attempted to change its identifier from %s to %s", oc.id, oc.mspID, mspID)
		}

		oc.pendingConfig.msp = msp
	default:
		logger.Warningf("Uknown org config item with key %s", key)
	}
	return nil
}
