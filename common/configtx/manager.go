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

package configtx

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

	"errors"

	"github.com/golang/protobuf/proto"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("common/configtx")

// Constraints for valid chain IDs
var (
	allowedChars = "[a-zA-Z0-9.-]+"
	maxLength    = 249
	illegalNames = map[string]struct{}{
		".":  struct{}{},
		"..": struct{}{},
	}
)

// Handler provides a hook which allows other pieces of code to participate in config proposals
type Handler interface {
	// BeginConfig called when a config proposal is begun
	BeginConfig()

	// RollbackConfig called when a config proposal is abandoned
	RollbackConfig()

	// CommitConfig called when a config proposal is committed
	CommitConfig()

	// ProposeConfig called when config is added to a proposal
	ProposeConfig(configItem *cb.ConfigurationItem) error
}

// Manager provides a mechanism to query and update configuration
type Manager interface {
	Resources

	// Apply attempts to apply a configtx to become the new configuration
	Apply(configtx *cb.ConfigurationEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	Validate(configtx *cb.ConfigurationEnvelope) error

	// ChainID retrieves the chain ID associated with this manager
	ChainID() string

	// Sequence returns the current sequence number of the configuration
	Sequence() uint64
}

// NewConfigurationItemPolicyKey is the ID of the policy used when no other policy can be resolved, for instance when attempting to create a new config item
const NewConfigurationItemPolicyKey = "NewConfigurationItemPolicy"

type acceptAllPolicy struct{}

func (ap *acceptAllPolicy) Evaluate(signedData []*cb.SignedData) error {
	return nil
}

type configurationManager struct {
	Initializer
	sequence      uint64
	chainID       string
	configuration map[cb.ConfigurationItem_ConfigurationType]map[string]*cb.ConfigurationItem
	callOnUpdate  []func(Manager)
}

// computeChainIDAndSequence returns the chain id and the sequence number for a configuration envelope
// or an error if there is a problem with the configuration envelope
func computeChainIDAndSequence(configtx *cb.ConfigurationEnvelope) (string, uint64, error) {
	if len(configtx.Items) == 0 {
		return "", 0, errors.New("Empty envelope unsupported")
	}

	m := uint64(0)

	if configtx.Header == nil {
		return "", 0, fmt.Errorf("Header not set")
	}

	if configtx.Header.ChainID == "" {
		return "", 0, fmt.Errorf("Header chainID was not set")
	}

	chainID := configtx.Header.ChainID

	if err := validateChainID(chainID); err != nil {
		return "", 0, err
	}

	for _, signedItem := range configtx.Items {
		item := &cb.ConfigurationItem{}
		if err := proto.Unmarshal(signedItem.ConfigurationItem, item); err != nil {
			return "", 0, fmt.Errorf("Error unmarshaling signedItem.ConfigurationItem: %s", err)
		}

		if item.LastModified > m {
			m = item.LastModified
		}
	}

	return chainID, m, nil
}

// validateChainID makes sure that proposed chain IDs (i.e. channel names)
// comply with the following restrictions:
//      1. Contain only ASCII alphanumerics, dots '.', dashes '-'
//      2. Are shorter than 250 characters.
//      3. Are not the strings "." or "..".
//
// Our hand here is forced by:
// https://github.com/apache/kafka/blob/trunk/core/src/main/scala/kafka/common/Topic.scala#L29
func validateChainID(chainID string) error {
	re, _ := regexp.Compile(allowedChars)
	// Length
	if len(chainID) <= 0 {
		return fmt.Errorf("chain ID illegal, cannot be empty")
	}
	if len(chainID) > maxLength {
		return fmt.Errorf("chain ID illegal, cannot be longer than %d", maxLength)
	}
	// Illegal name
	if _, ok := illegalNames[chainID]; ok {
		return fmt.Errorf("name '%s' for chain ID is not allowed", chainID)
	}
	// Illegal characters
	matched := re.FindString(chainID)
	if len(matched) != len(chainID) {
		return fmt.Errorf("Chain ID '%s' contains illegal characters", chainID)
	}

	return nil
}

// NewManagerImpl creates a new Manager unless an error is encountered, each element of the callOnUpdate slice
// is invoked when a new configuration is committed
func NewManagerImpl(configtx *cb.ConfigurationEnvelope, initializer Initializer, callOnUpdate []func(Manager)) (Manager, error) {
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		if _, ok := initializer.Handlers()[cb.ConfigurationItem_ConfigurationType(ctype)]; !ok {
			return nil, errors.New("Must supply a handler for all known types")
		}
	}

	chainID, seq, err := computeChainIDAndSequence(configtx)
	if err != nil {
		return nil, fmt.Errorf("Error computing chain ID and sequence: %s", err)
	}

	cm := &configurationManager{
		Initializer:   initializer,
		sequence:      seq - 1,
		chainID:       chainID,
		configuration: makeConfigMap(),
		callOnUpdate:  callOnUpdate,
	}

	err = cm.Apply(configtx)

	if err != nil {
		return nil, fmt.Errorf("Error applying config transaction: %s", err)
	}

	return cm, nil
}

func makeConfigMap() map[cb.ConfigurationItem_ConfigurationType]map[string]*cb.ConfigurationItem {
	configMap := make(map[cb.ConfigurationItem_ConfigurationType]map[string]*cb.ConfigurationItem)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		configMap[cb.ConfigurationItem_ConfigurationType(ctype)] = make(map[string]*cb.ConfigurationItem)
	}
	return configMap
}

func (cm *configurationManager) beginHandlers() {
	logger.Debugf("Beginning new configuration for chain %s", cm.chainID)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		cm.Initializer.Handlers()[cb.ConfigurationItem_ConfigurationType(ctype)].BeginConfig()
	}
}

func (cm *configurationManager) rollbackHandlers() {
	logger.Debugf("Rolling back configuration for chain %s", cm.chainID)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		cm.Initializer.Handlers()[cb.ConfigurationItem_ConfigurationType(ctype)].RollbackConfig()
	}
}

func (cm *configurationManager) commitHandlers() {
	logger.Debugf("Committing configuration for chain %s", cm.chainID)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		cm.Initializer.Handlers()[cb.ConfigurationItem_ConfigurationType(ctype)].CommitConfig()
	}
	for _, callback := range cm.callOnUpdate {
		callback(cm)
	}
}

func (cm *configurationManager) processConfig(configtx *cb.ConfigurationEnvelope) (configMap map[cb.ConfigurationItem_ConfigurationType]map[string]*cb.ConfigurationItem, err error) {
	chainID, seq, err := computeChainIDAndSequence(configtx)
	if err != nil {
		return nil, err
	}

	// Verify config is a sequential update to prevent exhausting sequence numbers
	if seq != cm.sequence+1 {
		return nil, fmt.Errorf("Config sequence number jumped from %d to %d", cm.sequence, seq)
	}

	// Verify config is intended for this globally unique chain ID
	if chainID != cm.chainID {
		return nil, fmt.Errorf("Config is for the wrong chain, expected %s, got %s", cm.chainID, chainID)
	}

	defaultModificationPolicy, defaultPolicySet := cm.PolicyManager().GetPolicy(NewConfigurationItemPolicyKey)

	// If the default modification policy is not set, it indicates this is an uninitialized chain, so be permissive of modification
	if !defaultPolicySet {
		defaultModificationPolicy = &acceptAllPolicy{}
	}

	configMap = makeConfigMap()

	for _, entry := range configtx.Items {
		// Verify every entry is well formed
		config := &cb.ConfigurationItem{}
		err = proto.Unmarshal(entry.ConfigurationItem, config)
		if err != nil {
			// Note that this is not reachable by test coverage because the unmarshal error would have already been found when computing the chainID and seqNo
			return nil, fmt.Errorf("Error unmarshaling ConfigurationItem: %s", err)
		}

		// Ensure the config sequence numbers are correct to prevent replay attacks
		var isModified bool

		if val, ok := cm.configuration[config.Type][config.Key]; ok {
			// Config was modified if any of the contents changed
			isModified = !reflect.DeepEqual(val, config)
		} else {
			if config.LastModified != seq {
				return nil, fmt.Errorf("Key %v for type %v was new, but had an older Sequence %d set", config.Key, config.Type, config.LastModified)
			}
			isModified = true
		}

		// If a config item was modified, its LastModified must be set correctly, and it must satisfy the modification policy
		if isModified {
			logger.Debugf("Proposed configuration item of type %v and key %s on chain %s has been modified", config.Type, config.Key, cm.chainID)

			if config.LastModified != seq {
				return nil, fmt.Errorf("Key %v for type %v was modified, but its LastModified %d does not equal current configtx Sequence %d", config.Key, config.Type, config.LastModified, seq)
			}

			// Get the modification policy for this config item if one was previously specified
			// or the default if this is a new config item
			var policy policies.Policy
			oldItem, ok := cm.configuration[config.Type][config.Key]
			if ok {
				policy, _ = cm.PolicyManager().GetPolicy(oldItem.ModificationPolicy)
			} else {
				policy = defaultModificationPolicy
			}

			// Get signatures
			signedData, err := entry.AsSignedData()
			if err != nil {
				return nil, err
			}

			// Ensure the policy is satisfied
			if err = policy.Evaluate(signedData); err != nil {
				return nil, err
			}
		}

		// Ensure the type handler agrees the config is well formed
		logger.Debugf("Proposing configuration item of type %v for key %s on chain %s", config.Type, config.Key, cm.chainID)
		err = cm.Initializer.Handlers()[config.Type].ProposeConfig(config)
		if err != nil {
			return nil, fmt.Errorf("Error proposing configuration item of type %v for key %s on chain %s: %s", config.Type, config.Key, chainID, err)
		}

		configMap[config.Type][config.Key] = config
	}

	// Ensure that any config items which used to exist still exist, to prevent implicit deletion
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		curMap := cm.configuration[cb.ConfigurationItem_ConfigurationType(ctype)]
		newMap := configMap[cb.ConfigurationItem_ConfigurationType(ctype)]
		for id := range curMap {
			_, ok := newMap[id]
			if !ok {
				return nil, fmt.Errorf("Missing key %v for type %v in new configuration", id, ctype)
			}

		}
	}

	return configMap, nil

}

// Validate attempts to validate a new configtx against the current config state
func (cm *configurationManager) Validate(configtx *cb.ConfigurationEnvelope) error {
	cm.beginHandlers()
	_, err := cm.processConfig(configtx)
	cm.rollbackHandlers()
	return err
}

// Apply attempts to apply a configtx to become the new configuration
func (cm *configurationManager) Apply(configtx *cb.ConfigurationEnvelope) error {
	cm.beginHandlers()
	configMap, err := cm.processConfig(configtx)
	if err != nil {
		cm.rollbackHandlers()
		return err
	}
	cm.configuration = configMap
	cm.sequence++
	cm.commitHandlers()
	return nil
}

// ChainID retrieves the chain ID associated with this manager
func (cm *configurationManager) ChainID() string {
	return cm.chainID
}

// Sequence returns the current sequence number of the configuration
func (cm *configurationManager) Sequence() uint64 {
	return cm.sequence
}
