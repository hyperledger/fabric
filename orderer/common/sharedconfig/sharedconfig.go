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
	"regexp"
	"strconv"
	"strings"
	"time"

	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

const (
	// ConsensusTypeKey is the cb.ConfigurationItem type key name for the ConsensusType message
	ConsensusTypeKey = "ConsensusType"

	// BatchSizeKey is the cb.ConfigurationItem type key name for the BatchSize message
	BatchSizeKey = "BatchSize"

	// BatchTimeoutKey is the cb.ConfigurationItem type key name for the BatchTimeout message
	BatchTimeoutKey = "BatchTimeout"

	// ChainCreatorsKey is the cb.ConfigurationItem type key name for the ChainCreators message
	ChainCreatorsKey = "ChainCreators"

	// KafkaBrokersKey is the cb.ConfigurationItem type key name for the KafkaBrokers message
	KafkaBrokersKey = "KafkaBrokers"
)

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
	BatchSize() *ab.BatchSize

	// BatchTimeout returns the amount of time to wait before creating a batch
	BatchTimeout() time.Duration

	// ChainCreators returns the policy names which are allowed for chain creation
	// This field is only set for the system ordering chain
	ChainCreators() []string

	// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
	// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
	// used for ordering
	KafkaBrokers() []string
}

type ordererConfig struct {
	consensusType string
	batchSize     *ab.BatchSize
	batchTimeout  time.Duration
	chainCreators []string
	kafkaBrokers  []string
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
func (pm *ManagerImpl) BatchSize() *ab.BatchSize {
	return pm.config.batchSize
}

// BatchTimeout returns the amount of time to wait before creating a batch
func (pm *ManagerImpl) BatchTimeout() time.Duration {
	return pm.config.batchTimeout
}

// ChainCreators returns the policy names which are allowed for chain creation
// This field is only set for the system ordering chain
func (pm *ManagerImpl) ChainCreators() []string {
	return pm.config.chainCreators
}

// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
// used for ordering
func (pm *ManagerImpl) KafkaBrokers() []string {
	return pm.config.kafkaBrokers
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
		if err := proto.Unmarshal(configItem.Value, consensusType); err != nil {
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
		if err := proto.Unmarshal(configItem.Value, batchSize); err != nil {
			return fmt.Errorf("Unmarshaling error for BatchSize: %s", err)
		}
		if batchSize.MaxMessageCount <= 0 {
			return fmt.Errorf("Attempted to set the batch size max message count to %d which is less than or equal to 0", batchSize.MaxMessageCount)
		}
		pm.pendingConfig.batchSize = batchSize
	case BatchTimeoutKey:
		var timeoutValue time.Duration
		var err error
		batchTimeout := &ab.BatchTimeout{}
		if err = proto.Unmarshal(configItem.Value, batchTimeout); err != nil {
			return fmt.Errorf("Unmarshaling error for BatchTimeout: %s", err)
		}
		if timeoutValue, err = time.ParseDuration(batchTimeout.Timeout); err != nil {
			return fmt.Errorf("Attempted to set the batch timeout to a invalid value: %s", err)
		}
		if timeoutValue <= 0 {
			return fmt.Errorf("Attempted to set the batch timeout to a non-positive value: %s", timeoutValue.String())
		}
		pm.pendingConfig.batchTimeout = timeoutValue
	case ChainCreatorsKey:
		chainCreators := &ab.ChainCreators{}
		if err := proto.Unmarshal(configItem.Value, chainCreators); err != nil {
			return fmt.Errorf("Unmarshaling error for ChainCreator: %s", err)
		}
		pm.pendingConfig.chainCreators = chainCreators.Policies
	case KafkaBrokersKey:
		kafkaBrokers := &ab.KafkaBrokers{}
		if err := proto.Unmarshal(configItem.Value, kafkaBrokers); err != nil {
			return fmt.Errorf("Unmarshaling error for KafkaBrokers: %s", err)
		}
		if len(kafkaBrokers.Brokers) == 0 {
			return fmt.Errorf("Kafka broker set cannot be nil")
		}
		for _, broker := range kafkaBrokers.Brokers {
			if !brokerEntrySeemsValid(broker) {
				return fmt.Errorf("Invalid broker entry: %s", broker)
			}
		}
		pm.pendingConfig.kafkaBrokers = kafkaBrokers.Brokers
	}
	return nil
}

// This does just a barebones sanitfy check.
func brokerEntrySeemsValid(broker string) bool {
	if !strings.Contains(broker, ":") {
		return false
	}

	parts := strings.Split(broker, ":")
	if len(parts) > 2 {
		return false
	}

	host := parts[0]
	port := parts[1]

	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return false
	}

	// Valid hostnames may contain only the ASCII letters 'a' through 'z' (in a
	// case-insensitive manner), the digits '0' through '9', and the hyphen. IP
	// v4 addresses are  represented in dot-decimal notation, which consists of
	// four decimal numbers, each ranging from 0 to 255, separated by dots,
	// e.g., 172.16.254.1
	// The following regular expression:
	// 1. allows just a-z (case-insensitive), 0-9, and the dot and hyphen characters
	// 2. does not allow leading trailing dots or hyphens
	re, _ := regexp.Compile("^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9.-]*[a-zA-Z0-9])$")
	matched := re.FindString(host)
	return len(matched) == len(host)
}
