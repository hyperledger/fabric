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
	"regexp"
	"strings"

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
	api.Resources
	sequence     uint64
	chainID      string
	config       map[string]comparable
	callOnUpdate []func(api.Manager)
	initializer  api.Initializer
}

func computeSequence(configGroup *cb.ConfigGroup) uint64 {
	max := uint64(0)
	for _, value := range configGroup.Values {
		if value.Version > max {
			max = value.Version
		}
	}

	for _, group := range configGroup.Groups {
		if groupMax := computeSequence(group); groupMax > max {
			max = groupMax
		}
	}

	return max
}

// computeChannelIdAndSequence returns the chain id and the sequence number for a config envelope
// or an error if there is a problem with the config envelope
func computeChannelIdAndSequence(config *cb.ConfigUpdate) (string, uint64, error) {
	if config.WriteSet == nil {
		return "", 0, errors.New("Empty envelope unsupported")
	}

	if config.Header == nil {
		return "", 0, fmt.Errorf("Header not set")
	}

	if config.Header.ChannelId == "" {
		return "", 0, fmt.Errorf("Header chainID was not set")
	}

	chainID := config.Header.ChannelId

	if err := validateChainID(chainID); err != nil {
		return "", 0, err
	}

	m := computeSequence(config.WriteSet)

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

func NewManagerImpl(configtx *cb.ConfigEnvelope, initializer api.Initializer, callOnUpdate []func(api.Manager)) (api.Manager, error) {
	// XXX as a temporary hack to get the new protos working, we assume entire config is always in the ConfigUpdate.WriteSet

	if configtx.LastUpdate == nil {
		return nil, fmt.Errorf("Must have ConfigEnvelope.LastUpdate set")
	}

	config, err := UnmarshalConfigUpdate(configtx.LastUpdate.ConfigUpdate)
	if err != nil {
		return nil, err
	}

	chainID, seq, err := computeChannelIdAndSequence(config)
	if err != nil {
		return nil, fmt.Errorf("Error computing chain ID and sequence: %s", err)
	}

	cm := &configManager{
		Resources:    initializer,
		initializer:  initializer,
		sequence:     seq - 1,
		chainID:      chainID,
		config:       make(map[string]comparable),
		callOnUpdate: callOnUpdate,
	}

	err = cm.Apply(configtx)

	if err != nil {
		return nil, fmt.Errorf("Error applying config transaction: %s", err)
	}

	return cm, nil
}

func (cm *configManager) beginHandlers() {
	logger.Debugf("Beginning new config for chain %s", cm.chainID)
	cm.initializer.BeginConfig()
}

func (cm *configManager) rollbackHandlers() {
	logger.Debugf("Rolling back config for chain %s", cm.chainID)
	cm.initializer.RollbackConfig()
}

func (cm *configManager) commitHandlers() {
	logger.Debugf("Committing config for chain %s", cm.chainID)
	cm.initializer.CommitConfig()
	for _, callback := range cm.callOnUpdate {
		callback(cm)
	}
}

func (cm *configManager) recurseConfig(result map[string]comparable, path []string, group *cb.ConfigGroup) error {

	for key, group := range group.Groups {
		// TODO rename validateChainID to validateConfigID
		if err := validateChainID(key); err != nil {
			return fmt.Errorf("Illegal characters in group key: %s", key)
		}

		if err := cm.recurseConfig(result, append(path, key), group); err != nil {
			return err
		}

		// TODO, uncomment to validate version validation on groups
		// result["[Groups]"+strings.Join(path, ".")+key] = comparable{path: path, ConfigGroup: group}
	}

	valueHandler, err := cm.initializer.Handler(path)
	if err != nil {
		return err
	}

	for key, value := range group.Values {
		if err := validateChainID(key); err != nil {
			return fmt.Errorf("Illegal characters in values key: %s", key)
		}

		err := valueHandler.ProposeConfig(key, &cb.ConfigValue{
			Value: value.Value,
		})
		if err != nil {
			return err
		}

		result["[Values]"+strings.Join(path, ".")+key] = comparable{path: path, ConfigValue: value}
	}

	logger.Debugf("Found %d policies", len(group.Policies))
	for key, policy := range group.Policies {
		if err := validateChainID(key); err != nil {
			return fmt.Errorf("Illegal characters in policies key: %s", key)
		}

		logger.Debugf("Proposing policy: %s", key)
		err := cm.initializer.PolicyProposer().ProposeConfig(key, &cb.ConfigValue{
			// TODO, fix policy interface to take the policy directly
			Value: utils.MarshalOrPanic(policy.Policy),
		})

		if err != nil {
			return err
		}

		// TODO, uncomment to validate version validation on policies
		//result["[Policies]"+strings.Join(path, ".")+key] = comparable{path: path, ConfigPolicy: policy}
	}

	return nil
}

func (cm *configManager) processConfig(configtx *cb.ConfigEnvelope) (configMap map[string]comparable, err error) {
	// XXX as a temporary hack to get the new protos working, we assume entire config is always in the ConfigUpdate.WriteSet

	if configtx.LastUpdate == nil {
		return nil, fmt.Errorf("Must have ConfigEnvelope.LastUpdate set")
	}

	config, err := UnmarshalConfigUpdate(configtx.LastUpdate.ConfigUpdate)
	if err != nil {
		return nil, err
	}

	chainID, seq, err := computeChannelIdAndSequence(config)
	if err != nil {
		return nil, err
	}

	signedData, err := configtx.LastUpdate.AsSignedData()
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

	configMap = make(map[string]comparable)

	if err := cm.recurseConfig(configMap, []string{}, config.WriteSet); err != nil {
		return nil, err
	}

	for key, value := range configMap {
		logger.Debugf("Processing key %s with value %v", key, value)

		// Ensure the config sequence numbers are correct to prevent replay attacks
		var isModified bool

		oldValue, ok := cm.config[key]
		if ok {
			isModified = !value.equals(oldValue)
		} else {
			if value.version() != seq {
				return nil, fmt.Errorf("Key %v was new, but had an older Sequence %d set", key, value.version())
			}
			isModified = true
		}

		// If a config item was modified, its Version must be set correctly, and it must satisfy the modification policy
		if isModified {
			logger.Debugf("Proposed config item %s on channel %s has been modified", key, chainID)

			if value.version() != seq {
				return nil, fmt.Errorf("Key %s was modified, but its Version %d does not equal current configtx Sequence %d", key, value.version(), seq)
			}

			// Get the modification policy for this config item if one was previously specified
			// or the default if this is a new config item
			var policy policies.Policy
			if ok {
				policy, _ = cm.PolicyManager().GetPolicy(oldValue.modPolicy())
			} else {
				policy = defaultModificationPolicy
			}

			// Ensure the policy is satisfied
			if err = policy.Evaluate(signedData); err != nil {
				return nil, err
			}
		}
	}

	// Ensure that any config items which used to exist still exist, to prevent implicit deletion
	for key, _ := range cm.config {
		_, ok := configMap[key]
		if !ok {
			return nil, fmt.Errorf("Missing key %v in new config", key)
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
