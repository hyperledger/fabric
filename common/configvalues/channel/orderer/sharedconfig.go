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

package orderer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	api "github.com/hyperledger/fabric/common/configvalues"
	"github.com/hyperledger/fabric/common/configvalues/channel/common/organization"
	"github.com/hyperledger/fabric/common/configvalues/msp"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
	"github.com/op/go-logging"
)

const (
	// GroupKey is the group name for the orderer config
	GroupKey = "Orderer"
)

var orgSchema = &cb.ConfigGroupSchema{
	Groups: map[string]*cb.ConfigGroupSchema{},
	Values: map[string]*cb.ConfigValueSchema{
		"MSP": nil, // TODO, consolidate into a constant once common org code exists
	},
	Policies: map[string]*cb.ConfigPolicySchema{
	// TODO, set appropriately once hierarchical policies are implemented
	},
}

var Schema = &cb.ConfigGroupSchema{
	Groups: map[string]*cb.ConfigGroupSchema{
		"": orgSchema,
	},
	Values: map[string]*cb.ConfigValueSchema{
		ConsensusTypeKey:            nil,
		BatchSizeKey:                nil,
		BatchTimeoutKey:             nil,
		ChainCreationPolicyNamesKey: nil,
		KafkaBrokersKey:             nil,
		IngressPolicyNamesKey:       nil,
		EgressPolicyNamesKey:        nil,
	},
	Policies: map[string]*cb.ConfigPolicySchema{
	// TODO, set appropriately once hierarchical policies are implemented
	},
}

const (
	// ConsensusTypeKey is the cb.ConfigItem type key name for the ConsensusType message
	ConsensusTypeKey = "ConsensusType"

	// BatchSizeKey is the cb.ConfigItem type key name for the BatchSize message
	BatchSizeKey = "BatchSize"

	// BatchTimeoutKey is the cb.ConfigItem type key name for the BatchTimeout message
	BatchTimeoutKey = "BatchTimeout"

	// ChainCreationPolicyNamesKey is the cb.ConfigItem type key name for the ChainCreationPolicyNames message
	ChainCreationPolicyNamesKey = "ChainCreationPolicyNames"

	// KafkaBrokersKey is the cb.ConfigItem type key name for the KafkaBrokers message
	KafkaBrokersKey = "KafkaBrokers"

	// IngressPolicyNamesKey is the cb.ConfigItem type key name for the IngressPolicyNames message
	IngressPolicyNamesKey = "IngressPolicyNames"

	// EgressPolicyNamesKey is the cb.ConfigItem type key name for the EgressPolicyNames message
	EgressPolicyNamesKey = "EgressPolicyNames"
)

var logger = logging.MustGetLogger("configtx/handlers/orderer")

type ordererConfig struct {
	consensusType            string
	batchSize                *ab.BatchSize
	batchTimeout             time.Duration
	chainCreationPolicyNames []string
	kafkaBrokers             []string
	ingressPolicyNames       []string
	egressPolicyNames        []string
	orgs                     map[string]*organization.OrgConfig
}

// ManagerImpl is an implementation of configtxapi.OrdererConfig and configtxapi.ValueProposer
type ManagerImpl struct {
	pendingConfig *ordererConfig
	config        *ordererConfig

	mspConfig *msp.MSPConfigHandler
}

// NewManagerImpl creates a new ManagerImpl
func NewManagerImpl(mspConfig *msp.MSPConfigHandler) *ManagerImpl {
	return &ManagerImpl{
		config:    &ordererConfig{},
		mspConfig: mspConfig,
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

// ChainCreationPolicyNames returns the policy names which are allowed for chain creation
// This field is only set for the system ordering chain
func (pm *ManagerImpl) ChainCreationPolicyNames() []string {
	return pm.config.chainCreationPolicyNames
}

// KafkaBrokers returns the addresses (IP:port notation) of a set of "bootstrap"
// Kafka brokers, i.e. this is not necessarily the entire set of Kafka brokers
// used for ordering
func (pm *ManagerImpl) KafkaBrokers() []string {
	return pm.config.kafkaBrokers
}

// IngressPolicyNames returns the name of the policy to validate incoming broadcast messages against
func (pm *ManagerImpl) IngressPolicyNames() []string {
	return pm.config.ingressPolicyNames
}

// EgressPolicyNames returns the name of the policy to validate incoming deliver seeks against
func (pm *ManagerImpl) EgressPolicyNames() []string {
	return pm.config.egressPolicyNames
}

// BeginValueProposals is used to start a new config proposal
func (pm *ManagerImpl) BeginValueProposals(groups []string) ([]api.ValueProposer, error) {
	logger.Debugf("Beginning a possible new orderer shared config")
	if pm.pendingConfig != nil {
		logger.Panicf("Programming error, cannot call begin in the middle of a proposal")
	}
	pm.pendingConfig = &ordererConfig{
		orgs: make(map[string]*organization.OrgConfig),
	}
	orgHandlers := make([]api.ValueProposer, len(groups))
	for i, group := range groups {
		org, ok := pm.pendingConfig.orgs[group]
		if !ok {
			org = organization.NewOrgConfig(group, pm.mspConfig)
			pm.pendingConfig.orgs[group] = org
		}
		orgHandlers[i] = org
	}
	return orgHandlers, nil
}

// RollbackProposals is used to abandon a new config proposal
func (pm *ManagerImpl) RollbackProposals() {
	logger.Debugf("Rolling back orderer config")
	pm.pendingConfig = nil
}

// CommitProposals is used to commit a new config proposal
func (pm *ManagerImpl) CommitProposals() {
	if pm.pendingConfig == nil {
		logger.Fatalf("Programming error, cannot call commit without an existing proposal")
	}
	pm.config = pm.pendingConfig
	pm.pendingConfig = nil
	if logger.IsEnabledFor(logging.DEBUG) {
		logger.Debugf("Adopting new orderer shared config: %+v", pm.config)
	}
}

// ProposeValue is used to add new config to the config proposal
func (pm *ManagerImpl) ProposeValue(key string, configValue *cb.ConfigValue) error {
	switch key {
	case ConsensusTypeKey:
		consensusType := &ab.ConsensusType{}
		if err := proto.Unmarshal(configValue.Value, consensusType); err != nil {
			return fmt.Errorf("Unmarshaling error for ConsensusType: %s", err)
		}
		if pm.config.consensusType == "" {
			// The first config we accept the consensus type regardless
			pm.config.consensusType = consensusType.Type
		}
		if consensusType.Type != pm.config.consensusType {
			return fmt.Errorf("Attempted to change the consensus type from %s to %s after init", pm.config.consensusType, consensusType.Type)
		}
		pm.pendingConfig.consensusType = consensusType.Type
	case BatchSizeKey:
		batchSize := &ab.BatchSize{}
		if err := proto.Unmarshal(configValue.Value, batchSize); err != nil {
			return fmt.Errorf("Unmarshaling error for BatchSize: %s", err)
		}
		if batchSize.MaxMessageCount == 0 {
			return fmt.Errorf("Attempted to set the batch size max message count to an invalid value: 0")
		}
		if batchSize.AbsoluteMaxBytes == 0 {
			return fmt.Errorf("Attempted to set the batch size absolute max bytes to an invalid value: 0")
		}
		if batchSize.PreferredMaxBytes == 0 {
			return fmt.Errorf("Attempted to set the batch size preferred max bytes to an invalid value: 0")
		}
		if batchSize.PreferredMaxBytes > batchSize.AbsoluteMaxBytes {
			return fmt.Errorf("Attempted to set the batch size preferred max bytes (%v) greater than the absolute max bytes (%v).", batchSize.PreferredMaxBytes, batchSize.AbsoluteMaxBytes)
		}
		pm.pendingConfig.batchSize = batchSize
	case BatchTimeoutKey:
		var timeoutValue time.Duration
		var err error
		batchTimeout := &ab.BatchTimeout{}
		if err = proto.Unmarshal(configValue.Value, batchTimeout); err != nil {
			return fmt.Errorf("Unmarshaling error for BatchTimeout: %s", err)
		}
		if timeoutValue, err = time.ParseDuration(batchTimeout.Timeout); err != nil {
			return fmt.Errorf("Attempted to set the batch timeout to a invalid value: %s", err)
		}
		if timeoutValue <= 0 {
			return fmt.Errorf("Attempted to set the batch timeout to a non-positive value: %s", timeoutValue.String())
		}
		pm.pendingConfig.batchTimeout = timeoutValue
	case ChainCreationPolicyNamesKey:
		chainCreationPolicyNames := &ab.ChainCreationPolicyNames{}
		if err := proto.Unmarshal(configValue.Value, chainCreationPolicyNames); err != nil {
			return fmt.Errorf("Unmarshaling error for ChainCreator: %s", err)
		}
		if chainCreationPolicyNames.Names == nil {
			// Proto unmarshals empty slices to nil, but this poses a problem for us in detecting the system chain
			// if it does not set this value, so explicitly set the policies to the empty string slice, if it is set
			pm.pendingConfig.chainCreationPolicyNames = []string{}
		} else {
			pm.pendingConfig.chainCreationPolicyNames = chainCreationPolicyNames.Names
		}
	case IngressPolicyNamesKey:
		ingressPolicyNames := &ab.IngressPolicyNames{}
		if err := proto.Unmarshal(configValue.Value, ingressPolicyNames); err != nil {
			return fmt.Errorf("Unmarshaling error for IngressPolicyNames: %s", err)
		}
		pm.pendingConfig.ingressPolicyNames = ingressPolicyNames.Names
	case EgressPolicyNamesKey:
		egressPolicyNames := &ab.EgressPolicyNames{}
		if err := proto.Unmarshal(configValue.Value, egressPolicyNames); err != nil {
			return fmt.Errorf("Unmarshaling error for EgressPolicyNames: %s", err)
		}
		pm.pendingConfig.egressPolicyNames = egressPolicyNames.Names
	case KafkaBrokersKey:
		kafkaBrokers := &ab.KafkaBrokers{}
		if err := proto.Unmarshal(configValue.Value, kafkaBrokers); err != nil {
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

// This does just a barebones sanity check.
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

// PreCommit returns nil
func (pm *ManagerImpl) PreCommit() error { return nil }
