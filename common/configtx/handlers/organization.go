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

package handlers

import (
	"fmt"

	"github.com/hyperledger/fabric/common/configtx/api"
	mspconfig "github.com/hyperledger/fabric/common/configtx/handlers/msp"
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"

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

// BeginConfig is used to start a new config proposal
func (oc *OrgConfig) BeginConfig() {
	logger.Debugf("Beginning a possible new org config")
	if oc.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	oc.pendingConfig = &orgConfig{}
}

// RollbackConfig is used to abandon a new config proposal
func (oc *OrgConfig) RollbackConfig() {
	logger.Debugf("Rolling back proposed org config")
	oc.pendingConfig = nil
}

// CommitConfig is used to commit a new config proposal
func (oc *OrgConfig) CommitConfig() {
	logger.Debugf("Committing new org config")
	if oc.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	oc.config = oc.pendingConfig
	oc.pendingConfig = nil
}

// ProposeConfig is used to add new config to the config proposal
func (oc *OrgConfig) ProposeConfig(key string, configValue *cb.ConfigValue) error {
	switch key {
	case MSPKey:
		logger.Debugf("Initializing org MSP for id %s", oc.id)
		return oc.mspConfig.ProposeConfig(key, configValue)
	default:
		logger.Warningf("Uknown org config item with key %s", key)
	}
	return nil
}

// Handler returns the associated api.Handler for the given path
func (oc *OrgConfig) Handler(path []string) (api.Handler, error) {
	if len(path) == 0 {
		return oc, nil
	}

	return nil, fmt.Errorf("Organizations do not further nesting")
}
