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
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

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

// NewConfigItemPolicyKey is the ID of the policy used when no other policy can be resolved, for instance when attempting to create a new config item
const NewConfigItemPolicyKey = "NewConfigItemPolicy"

type acceptAllPolicy struct{}

func (ap *acceptAllPolicy) Evaluate(signedData []*cb.SignedData) error {
	return nil
}

type configManager struct {
	api.Initializer
	sequence     uint64
	chainID      string
	config       map[cb.ConfigItem_ConfigType]map[string]*cb.ConfigItem
	callOnUpdate []func(api.Manager)
}

// computeChainIDAndSequence returns the chain id and the sequence number for a config envelope
// or an error if there is a problem with the config envelope
func computeChainIDAndSequence(config *cb.Config) (string, uint64, error) {
	if len(config.Items) == 0 {
		return "", 0, errors.New("Empty envelope unsupported")
	}

	m := uint64(0)

	if config.Header == nil {
		return "", 0, fmt.Errorf("Header not set")
	}

	if config.Header.ChainID == "" {
		return "", 0, fmt.Errorf("Header chainID was not set")
	}

	chainID := config.Header.ChainID

	if err := validateChainID(chainID); err != nil {
		return "", 0, err
	}

	for _, item := range config.Items {
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

func NewManagerImplNext(configtx *cb.ConfigEnvelope, initializer api.Initializer, callOnUpdate []func(api.Manager)) (api.Manager, error) {
	configNext, err := UnmarshalConfigNext(configtx.Config)
	if err != nil {
		return nil, err
	}

	config := ConfigNextToConfig(configNext)

	return NewManagerImpl(&cb.ConfigEnvelope{Config: utils.MarshalOrPanic(config), Signatures: configtx.Signatures}, initializer, callOnUpdate)
}

// NewManagerImpl creates a new Manager unless an error is encountered, each element of the callOnUpdate slice
// is invoked when a new config is committed
func NewManagerImpl(configtx *cb.ConfigEnvelope, initializer api.Initializer, callOnUpdate []func(api.Manager)) (api.Manager, error) {
	for ctype := range cb.ConfigItem_ConfigType_name {
		if _, ok := initializer.Handlers()[cb.ConfigItem_ConfigType(ctype)]; !ok {
			return nil, errors.New("Must supply a handler for all known types")
		}
	}

	config, err := UnmarshalConfig(configtx.Config)
	if err != nil {
		return nil, err
	}

	chainID, seq, err := computeChainIDAndSequence(config)
	if err != nil {
		return nil, fmt.Errorf("Error computing chain ID and sequence: %s", err)
	}

	cm := &configManager{
		Initializer:  initializer,
		sequence:     seq - 1,
		chainID:      chainID,
		config:       makeConfigMap(),
		callOnUpdate: callOnUpdate,
	}

	err = cm.Apply(configtx)

	if err != nil {
		return nil, fmt.Errorf("Error applying config transaction: %s", err)
	}

	return cm, nil
}

func makeConfigMap() map[cb.ConfigItem_ConfigType]map[string]*cb.ConfigItem {
	configMap := make(map[cb.ConfigItem_ConfigType]map[string]*cb.ConfigItem)
	for ctype := range cb.ConfigItem_ConfigType_name {
		configMap[cb.ConfigItem_ConfigType(ctype)] = make(map[string]*cb.ConfigItem)
	}
	return configMap
}

func (cm *configManager) beginHandlers() {
	logger.Debugf("Beginning new config for chain %s", cm.chainID)
	for ctype := range cb.ConfigItem_ConfigType_name {
		cm.Initializer.Handlers()[cb.ConfigItem_ConfigType(ctype)].BeginConfig()
	}
}

func (cm *configManager) rollbackHandlers() {
	logger.Debugf("Rolling back config for chain %s", cm.chainID)
	for ctype := range cb.ConfigItem_ConfigType_name {
		cm.Initializer.Handlers()[cb.ConfigItem_ConfigType(ctype)].RollbackConfig()
	}
}

func (cm *configManager) commitHandlers() {
	logger.Debugf("Committing config for chain %s", cm.chainID)
	for ctype := range cb.ConfigItem_ConfigType_name {
		cm.Initializer.Handlers()[cb.ConfigItem_ConfigType(ctype)].CommitConfig()
	}
	for _, callback := range cm.callOnUpdate {
		callback(cm)
	}
}

func (cm *configManager) processConfig(configtx *cb.ConfigEnvelope) (configMap map[cb.ConfigItem_ConfigType]map[string]*cb.ConfigItem, err error) {
	config, err := UnmarshalConfig(configtx.Config)
	if err != nil {
		return nil, err
	}

	chainID, seq, err := computeChainIDAndSequence(config)
	if err != nil {
		return nil, err
	}

	signedData, err := configtx.AsSignedData()
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

	defaultModificationPolicy, defaultPolicySet := cm.PolicyManager().GetPolicy(NewConfigItemPolicyKey)

	// If the default modification policy is not set, it indicates this is an uninitialized chain, so be permissive of modification
	if !defaultPolicySet {
		defaultModificationPolicy = &acceptAllPolicy{}
	}

	configMap = makeConfigMap()

	for _, item := range config.Items {
		// Ensure the config sequence numbers are correct to prevent replay attacks
		var isModified bool

		if val, ok := cm.config[item.Type][item.Key]; ok {
			// Config was modified if any of the contents changed
			isModified = !reflect.DeepEqual(val, item)
		} else {
			if item.LastModified != seq {
				return nil, fmt.Errorf("Key %v for type %v was new, but had an older Sequence %d set", item.Key, item.Type, item.LastModified)
			}
			isModified = true
		}

		// If a config item was modified, its LastModified must be set correctly, and it must satisfy the modification policy
		if isModified {
			logger.Debugf("Proposed config item of type %v and key %s on chain %s has been modified", item.Type, item.Key, chainID)

			if item.LastModified != seq {
				return nil, fmt.Errorf("Key %v for type %v was modified, but its LastModified %d does not equal current configtx Sequence %d", item.Key, item.Type, item.LastModified, seq)
			}

			// Get the modification policy for this config item if one was previously specified
			// or the default if this is a new config item
			var policy policies.Policy
			oldItem, ok := cm.config[item.Type][item.Key]
			if ok {
				policy, _ = cm.PolicyManager().GetPolicy(oldItem.ModificationPolicy)
			} else {
				policy = defaultModificationPolicy
			}

			// Ensure the policy is satisfied
			if err = policy.Evaluate(signedData); err != nil {
				return nil, err
			}
		}

		// Ensure the type handler agrees the config is well formed
		logger.Debugf("Proposing config item of type %v for key %s on chain %s", item.Type, item.Key, cm.chainID)
		err = cm.Initializer.Handlers()[item.Type].ProposeConfig(item)
		if err != nil {
			return nil, fmt.Errorf("Error proposing config item of type %v for key %s on chain %s: %s", item.Type, item.Key, chainID, err)
		}

		// Ensure the same key has not been specified multiple times
		_, ok := configMap[item.Type][item.Key]
		if ok {
			return nil, fmt.Errorf("Specified config item of type %v for key %s for chain %s multiple times", item.Type, item.Key, chainID)
		}

		configMap[item.Type][item.Key] = item
	}

	// Ensure that any config items which used to exist still exist, to prevent implicit deletion
	for ctype := range cb.ConfigItem_ConfigType_name {
		curMap := cm.config[cb.ConfigItem_ConfigType(ctype)]
		newMap := configMap[cb.ConfigItem_ConfigType(ctype)]
		for id := range curMap {
			_, ok := newMap[id]
			if !ok {
				return nil, fmt.Errorf("Missing key %v for type %v in new config", id, ctype)
			}

		}
	}

	return configMap, nil

}

// Validate attempts to validate a new configtx against the current config state
func (cm *configManager) Validate(configtx *cb.ConfigEnvelope) error {
	cm.beginHandlers()
	_, err := cm.processConfig(configtx)
	cm.rollbackHandlers()
	return err
}

// Apply attempts to apply a configtx to become the new config
func (cm *configManager) Apply(configtx *cb.ConfigEnvelope) error {
	cm.beginHandlers()
	configMap, err := cm.processConfig(configtx)
	if err != nil {
		cm.rollbackHandlers()
		return err
	}
	cm.config = configMap
	cm.sequence++
	cm.commitHandlers()
	return nil
}

// ChainID retrieves the chain ID associated with this manager
func (cm *configManager) ChainID() string {
	return cm.chainID
}

// Sequence returns the current sequence number of the config
func (cm *configManager) Sequence() uint64 {
	return cm.sequence
}
