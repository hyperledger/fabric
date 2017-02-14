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
	"fmt"

	"github.com/hyperledger/fabric/common/configtx/handlers"
	mspconfig "github.com/hyperledger/fabric/common/configtx/handlers/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
)

// Application org config keys
const (
	// AnchorPeersKey is the key name for the AnchorPeers ConfigValue
	AnchorPeersKey = "AnchorPeers"
)

type applicationOrgConfig struct {
	anchorPeers []*pb.AnchorPeer
}

// SharedConfigImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.Manager
type ApplicationOrgConfig struct {
	*handlers.OrgConfig
	pendingConfig *applicationOrgConfig
	config        *applicationOrgConfig

	mspConfig *mspconfig.MSPConfigHandler
}

// NewSharedConfigImpl creates a new SharedConfigImpl with the given CryptoHelper
func NewApplicationOrgConfig(id string, mspConfig *mspconfig.MSPConfigHandler) *ApplicationOrgConfig {
	return &ApplicationOrgConfig{
		OrgConfig: handlers.NewOrgConfig(id, mspConfig),
		config:    &applicationOrgConfig{},
	}
}

// AnchorPeers returns the list of valid orderer addresses to connect to to invoke Broadcast/Deliver
func (oc *ApplicationOrgConfig) AnchorPeers() []*pb.AnchorPeer {
	return oc.config.anchorPeers
}

// BeginConfig is used to start a new config proposal
func (oc *ApplicationOrgConfig) BeginConfig() {
	logger.Debugf("Beginning a possible new org config")
	if oc.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	oc.pendingConfig = &applicationOrgConfig{}
}

// RollbackConfig is used to abandon a new config proposal
func (oc *ApplicationOrgConfig) RollbackConfig() {
	logger.Debugf("Rolling back proposed org config")
	oc.pendingConfig = nil
}

// CommitConfig is used to commit a new config proposal
func (oc *ApplicationOrgConfig) CommitConfig() {
	logger.Debugf("Committing new org config")
	if oc.pendingConfig == nil {
		logger.Panicf("Programming error, cannot call commit without an existing proposal")
	}
	oc.config = oc.pendingConfig
	oc.pendingConfig = nil
}

// ProposeConfig is used to add new config to the config proposal
func (oc *ApplicationOrgConfig) ProposeConfig(key string, configValue *cb.ConfigValue) error {
	switch key {
	case AnchorPeersKey:
		anchorPeers := &pb.AnchorPeers{}
		if err := proto.Unmarshal(configValue.Value, anchorPeers); err != nil {
			return fmt.Errorf("Unmarshaling error for %s: %s", key, err)
		}
		if logger.IsEnabledFor(logging.DEBUG) {
			logger.Debugf("Setting %s to %v", key, anchorPeers.AnchorPeers)
		}
		oc.pendingConfig.anchorPeers = anchorPeers.AnchorPeers
	default:
		return oc.OrgConfig.ProposeConfig(key, configValue)
	}

	return nil
}
