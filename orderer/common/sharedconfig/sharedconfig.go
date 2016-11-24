/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package sharedconfig

import (
	"fmt"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

// ConsensusTypeKey is the cb.ConfigurationItem type key name for the ConsensusType message
const ConsensusTypeKey = "ConsensusType"

// BatchSizeKey is the cb.ConfigurationItem type key name for the BatchSize message
const BatchSizeKey = "BatchSize"

var logger = logging.MustGetLogger("orderer/common/sharedconfig")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

// Manager stores the common shared orderer configuration
// It is intended to be the primary accessor of ManagerImpl
// It is intended to discourage use of the other exported ManagerImpl methods
// which are used for updating the orderer configuration by the ConfigManager
type Manager interface {
	// ConsensusType returns the configured consensus type
	ConsensusType() string

	// BatchSize returns the maximum number of messages to include in a block
	BatchSize() int
}

type ordererConfig struct {
	consensusType string
	batchSize     int
}

// ManagerImpl is an implementation of Manager and configtx.ConfigHandler
// In general, it should only be referenced as an Impl for the configtx.ConfigManager
type ManagerImpl struct {
	pendingConfig *ordererConfig
	config        *ordererConfig
}

// NewManagerImpl creates a new ManagerImpl with the given CryptoHelper
func NewManagerImpl() *ManagerImpl {
	return &ManagerImpl{
		config: &ordererConfig{},
	}
}

// ConsensusType returns the configured consensus type
func (pm *ManagerImpl) ConsensusType() string {
	return pm.config.consensusType
}

// BatchSize returns the maximum number of messages to include in a block
func (pm *ManagerImpl) BatchSize() int {
	return pm.config.batchSize
}

// BeginConfig is used to start a new configuration proposal
func (pm *ManagerImpl) BeginConfig() {
	if pm.pendingConfig != nil {
		logger.Fatalf("Programming error, cannot call begin in the middle of a proposal")
	}
	pm.pendingConfig = &ordererConfig{}
}

// RollbackConfig is used to abandon a new configuration proposal
func (pm *ManagerImpl) RollbackConfig() {
	pm.pendingConfig = nil
}

// CommitConfig is used to commit a new configuration proposal
func (pm *ManagerImpl) CommitConfig() {
	if pm.pendingConfig == nil {
		logger.Fatalf("Programming error, cannot call commit without an existing proposal")
	}
	pm.config = pm.pendingConfig
	pm.pendingConfig = nil
}

// ProposeConfig is used to add new configuration to the configuration proposal
func (pm *ManagerImpl) ProposeConfig(configItem *cb.ConfigurationItem) error {
	if configItem.Type != cb.ConfigurationItem_Orderer {
		return fmt.Errorf("Expected type of ConfigurationItem_Orderer, got %v", configItem.Type)
	}

	switch configItem.Key {
	case ConsensusTypeKey:
		consensusType := &ab.ConsensusType{}
		err := proto.Unmarshal(configItem.Value, consensusType)
		if err != nil {
			return fmt.Errorf("Unmarshaling error for ConsensusType: %s", err)
		}
		if pm.config.consensusType == "" {
			// The first configuration we accept the consensus type regardless
			pm.config.consensusType = consensusType.Type
		}
		if consensusType.Type != pm.config.consensusType {
			return fmt.Errorf("Attempted to change the consensus type from %s to %s after init", pm.config.consensusType, consensusType.Type)
		}

		pm.pendingConfig.consensusType = consensusType.Type
	case BatchSizeKey:
		batchSize := &ab.BatchSize{}
		err := proto.Unmarshal(configItem.Value, batchSize)
		if err != nil {
			return fmt.Errorf("Unmarshaling error for BatchSize: %s", err)
		}

		if batchSize.Messages <= 0 {
			return fmt.Errorf("Attempted to set the batch size to %d which is less than or equal to  0", batchSize.Messages)
		}

		pm.pendingConfig.batchSize = int(batchSize.Messages)
	}

	return nil
}
