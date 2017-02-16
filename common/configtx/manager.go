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

type configResult struct {
	handler    api.Transactional
	subResults []*configResult
}

func (cr *configResult) commit() {
	for _, subResult := range cr.subResults {
		subResult.commit()
	}
	cr.handler.CommitConfig()
}

func (cr *configResult) rollback() {
	for _, subResult := range cr.subResults {
		subResult.rollback()
	}
	cr.handler.RollbackConfig()
}

type configManager struct {
	api.Resources
	sequence     uint64
	chainID      string
	config       map[string]comparable
	callOnUpdate []func(api.Manager)
	initializer  api.Initializer
	configEnv    *cb.ConfigEnvelope
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

func NewManagerImpl(configEnv *cb.ConfigEnvelope, initializer api.Initializer, callOnUpdate []func(api.Manager)) (api.Manager, error) {
	if configEnv == nil {
		return nil, fmt.Errorf("Nil config envelope")
	}

	if configEnv.Config == nil {
		return nil, fmt.Errorf("Nil config envelope Config")
	}

	if configEnv.Config.Header == nil {
		return nil, fmt.Errorf("Nil config envelop Config Header")
	}

	if err := validateChainID(configEnv.Config.Header.ChannelId); err != nil {
		return nil, fmt.Errorf("Bad channel id: %s", err)
	}

	configMap, err := mapConfig(configEnv.Config.Channel)
	if err != nil {
		return nil, fmt.Errorf("Error converting config to map: %s", err)
	}

	cm := &configManager{
		Resources:    initializer,
		initializer:  initializer,
		sequence:     computeSequence(configEnv.Config.Channel),
		chainID:      configEnv.Config.Header.ChannelId,
		config:       configMap,
		callOnUpdate: callOnUpdate,
	}

	result, err := cm.processConfig(configEnv.Config.Channel)
	if err != nil {
		return nil, err
	}
	result.commit()
	cm.commitCallbacks()

	return cm, nil
}

func (cm *configManager) commitCallbacks() {
	for _, callback := range cm.callOnUpdate {
		callback(cm)
	}
}

// proposeGroup proposes a group configuration with a given handler
// it will in turn recursively call itself until all groups have been exhausted
// at each call, it returns the handler that was passed in, plus any handlers returned
// by recursive calls into proposeGroup
func (cm *configManager) proposeGroup(name string, group *cb.ConfigGroup, handler api.Handler) (*configResult, error) {
	subGroups := make([]string, len(group.Groups))
	i := 0
	for subGroup := range group.Groups {
		subGroups[i] = subGroup
		i++
	}

	logger.Debugf("Beginning new config for channel %s and group %s", cm.chainID, name)
	subHandlers, err := handler.BeginConfig(subGroups)
	if err != nil {
		return nil, err
	}

	if len(subHandlers) != len(subGroups) {
		return nil, fmt.Errorf("Programming error, did not return as many handlers as groups %d vs %d", len(subHandlers), len(subGroups))
	}

	result := &configResult{
		handler:    handler,
		subResults: make([]*configResult, 0, len(subGroups)),
	}

	for i, subGroup := range subGroups {
		subResult, err := cm.proposeGroup(name+"/"+subGroup, group.Groups[subGroup], subHandlers[i])
		if err != nil {
			result.rollback()
			return nil, err
		}
		result.subResults = append(result.subResults, subResult)
	}

	for key, value := range group.Values {
		if err := handler.ProposeConfig(key, value); err != nil {
			result.rollback()
			return nil, err
		}
	}

	return result, nil
}

func (cm *configManager) proposePolicies(rootGroup *cb.ConfigGroup) (*configResult, error) {
	cm.initializer.PolicyHandler().BeginConfig(nil) // XXX temporary workaround until policy manager is adapted with sub-policies

	for key, policy := range rootGroup.Policies {
		logger.Debugf("Proposing policy: %s", key)
		if err := cm.initializer.PolicyHandler().ProposePolicy(key, []string{RootGroupKey}, policy); err != nil {
			cm.initializer.PolicyHandler().RollbackConfig()
			return nil, err
		}
	}

	return &configResult{handler: cm.initializer.PolicyHandler()}, nil
}

// authorizeUpdate validates that all modified config has the corresponding modification policies satisfied by the signature set
// it returns a map of the modified config
func (cm *configManager) authorizeUpdate(configUpdateEnv *cb.ConfigUpdateEnvelope) (map[string]comparable, error) {
	if configUpdateEnv == nil {
		return nil, fmt.Errorf("Cannot process nil ConfigUpdateEnvelope")
	}

	config, err := UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, err
	}

	if config.Header == nil {
		return nil, fmt.Errorf("Must have header set")
	}

	seq := computeSequence(config.WriteSet)
	if err != nil {
		return nil, err
	}

	signedData, err := configUpdateEnv.AsSignedData()
	if err != nil {
		return nil, err
	}

	// Verify config is a sequential update to prevent exhausting sequence numbers
	if seq != cm.sequence+1 {
		return nil, fmt.Errorf("Config sequence number jumped from %d to %d", cm.sequence, seq)
	}

	// Verify config is intended for this globally unique chain ID
	if config.Header.ChannelId != cm.chainID {
		return nil, fmt.Errorf("Config is for the wrong chain, expected %s, got %s", cm.chainID, config.Header.ChannelId)
	}

	configMap, err := mapConfig(config.WriteSet)
	if err != nil {
		return nil, err
	}
	for key, value := range configMap {
		logger.Debugf("Processing key %s with value %v", key, value)
		if key == "[Groups] /Channel" {
			// XXX temporary hack to prevent group evaluation for modification
			continue
		}

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
			logger.Debugf("Proposed config item %s on channel %s has been modified", key, cm.chainID)

			if value.version() != seq {
				return nil, fmt.Errorf("Key %s was modified, but its Version %d does not equal current configtx Sequence %d", key, value.version(), seq)
			}

			// Get the modification policy for this config item if one was previously specified
			// or accept it if it is new, as the group policy will be evaluated for its inclusion
			var policy policies.Policy
			if ok {
				policy, _ = cm.PolicyManager().GetPolicy(oldValue.modPolicy())
				// Ensure the policy is satisfied
				if err = policy.Evaluate(signedData); err != nil {
					return nil, err
				}
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

	return cm.computeUpdateResult(configMap), nil
}

// computeUpdateResult takes a configMap generated by an update and produces a new configMap overlaying it onto the old config
func (cm *configManager) computeUpdateResult(updatedConfig map[string]comparable) map[string]comparable {
	newConfigMap := make(map[string]comparable)
	for key, value := range cm.config {
		newConfigMap[key] = value
	}

	for key, value := range updatedConfig {
		newConfigMap[key] = value
	}
	return newConfigMap
}

func (cm *configManager) processConfig(channelGroup *cb.ConfigGroup) (*configResult, error) {
	helperGroup := cb.NewConfigGroup()
	helperGroup.Groups[RootGroupKey] = channelGroup
	groupResult, err := cm.proposeGroup("", helperGroup, cm.initializer)
	if err != nil {
		return nil, err
	}

	policyResult, err := cm.proposePolicies(channelGroup)
	if err != nil {
		groupResult.rollback()
		return nil, err
	}
	policyResult.subResults = []*configResult{groupResult}

	return policyResult, nil
}

func envelopeToConfigUpdate(configtx *cb.Envelope) (*cb.ConfigUpdateEnvelope, error) {
	payload, err := utils.UnmarshalPayload(configtx.Payload)
	if err != nil {
		return nil, err
	}

	if payload.Header == nil || payload.Header.ChannelHeader == nil {
		return nil, fmt.Errorf("Envelope must have ChannelHeader")
	}

	if payload.Header == nil || payload.Header.ChannelHeader.Type != int32(cb.HeaderType_CONFIG_UPDATE) {
		return nil, fmt.Errorf("Not a tx of type CONFIG_UPDATE")
	}

	configUpdateEnv, err := UnmarshalConfigUpdateEnvelope(payload.Data)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling ConfigUpdateEnvelope: %s", err)
	}

	return configUpdateEnv, nil
}

// Validate attempts to validate a new configtx against the current config state
func (cm *configManager) Validate(configtx *cb.Envelope) error {
	configUpdateEnv, err := envelopeToConfigUpdate(configtx)
	if err != nil {
		return err
	}

	configMap, err := cm.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return err
	}

	channelGroup, err := configMapToConfig(configMap)
	if err != nil {
		return fmt.Errorf("Could not turn configMap back to channelGroup: %s", err)
	}

	result, err := cm.processConfig(channelGroup)
	if err != nil {
		return err
	}

	result.rollback()
	return nil
}

// Apply attempts to apply a configtx to become the new config
func (cm *configManager) Apply(configtx *cb.Envelope) error {
	configUpdateEnv, err := envelopeToConfigUpdate(configtx)
	if err != nil {
		return err
	}

	configMap, err := cm.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return err
	}

	channelGroup, err := configMapToConfig(configMap)
	if err != nil {
		return fmt.Errorf("Could not turn configMap back to channelGroup: %s", err)
	}

	result, err := cm.processConfig(channelGroup)
	if err != nil {
		return err
	}

	result.commit()
	cm.commitCallbacks()

	cm.config = configMap
	cm.sequence++

	cm.configEnv = &cb.ConfigEnvelope{
		Config: &cb.Config{
			// XXX add header
			Channel: channelGroup,
		},
		LastUpdate: configtx,
	}
	return nil
}

// ConfigEnvelope retrieve the current ConfigEnvelope, generated after the last successfully applied configuration
func (cm *configManager) ConfigEnvelope() *cb.ConfigEnvelope {
	return cm.configEnv
}

// ChainID retrieves the chain ID associated with this manager
func (cm *configManager) ChainID() string {
	return cm.chainID
}

// Sequence returns the current sequence number of the config
func (cm *configManager) Sequence() uint64 {
	return cm.sequence
}
