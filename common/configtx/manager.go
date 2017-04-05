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

package configtx

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/hyperledger/fabric/common/configtx/api"
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

type configSet struct {
	channelID string
	sequence  uint64
	configMap map[string]comparable
}

type configManager struct {
	api.Resources
	callOnUpdate []func(api.Manager)
	initializer  api.Initializer
	current      *configSet
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

func NewManagerImpl(envConfig *cb.Envelope, initializer api.Initializer, callOnUpdate []func(api.Manager)) (api.Manager, error) {
	if envConfig == nil {
		return nil, fmt.Errorf("Nil envelope")
	}

	configEnv := &cb.ConfigEnvelope{}
	header, err := utils.UnmarshalEnvelopeOfType(envConfig, cb.HeaderType_CONFIG, configEnv)
	if err != nil {
		return nil, fmt.Errorf("Bad envelope: %s", err)
	}

	if configEnv.Config == nil {
		return nil, fmt.Errorf("Nil config envelope Config")
	}

	if err := validateChainID(header.ChannelId); err != nil {
		return nil, fmt.Errorf("Bad channel id: %s", err)
	}

	configMap, err := mapConfig(configEnv.Config.ChannelGroup)
	if err != nil {
		return nil, fmt.Errorf("Error converting config to map: %s", err)
	}

	cm := &configManager{
		Resources:   initializer,
		initializer: initializer,
		current: &configSet{
			sequence:  configEnv.Config.Sequence,
			configMap: configMap,
			channelID: header.ChannelId,
		},
		callOnUpdate: callOnUpdate,
	}

	result, err := cm.processConfig(configEnv.Config.ChannelGroup)
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

// ProposeConfigUpdate takes in an Envelope of type CONFIG_UPDATE and produces a
// ConfigEnvelope to be used as the Envelope Payload Data of a CONFIG message
func (cm *configManager) ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error) {
	return cm.proposeConfigUpdate(configtx)
}

func (cm *configManager) proposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error) {
	configUpdateEnv, err := envelopeToConfigUpdate(configtx)
	if err != nil {
		return nil, err
	}

	configMap, err := cm.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return nil, err
	}

	channelGroup, err := configMapToConfig(configMap)
	if err != nil {
		return nil, fmt.Errorf("Could not turn configMap back to channelGroup: %s", err)
	}

	result, err := cm.processConfig(channelGroup)
	if err != nil {
		return nil, err
	}

	result.rollback()

	return &cb.ConfigEnvelope{
		Config: &cb.Config{
			Sequence:     cm.current.sequence + 1,
			ChannelGroup: channelGroup,
		},
		LastUpdate: configtx,
	}, nil
}

func (cm *configManager) prepareApply(configEnv *cb.ConfigEnvelope) (map[string]comparable, *configResult, error) {
	if configEnv == nil {
		return nil, nil, fmt.Errorf("Attempted to apply config with nil envelope")
	}

	if configEnv.Config == nil {
		return nil, nil, fmt.Errorf("Config cannot be nil")
	}

	if configEnv.Config.Sequence != cm.current.sequence+1 {
		return nil, nil, fmt.Errorf("Config at sequence %d, cannot prepare to update to %d", cm.current.sequence, configEnv.Config.Sequence)
	}

	configUpdateEnv, err := envelopeToConfigUpdate(configEnv.LastUpdate)
	if err != nil {
		return nil, nil, err
	}

	configMap, err := cm.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return nil, nil, err
	}

	channelGroup, err := configMapToConfig(configMap)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not turn configMap back to channelGroup: %s", err)
	}

	if !reflect.DeepEqual(channelGroup, configEnv.Config.ChannelGroup) {
		return nil, nil, fmt.Errorf("ConfigEnvelope LastUpdate did not produce the supplied config result")
	}

	result, err := cm.processConfig(channelGroup)
	if err != nil {
		return nil, nil, err
	}

	return configMap, result, nil
}

// Validate simulates applying a ConfigEnvelope to become the new config
func (cm *configManager) Validate(configEnv *cb.ConfigEnvelope) error {
	_, result, err := cm.prepareApply(configEnv)
	if err != nil {
		return err
	}

	result.rollback()

	return nil
}

// Apply attempts to apply a ConfigEnvelope to become the new config
func (cm *configManager) Apply(configEnv *cb.ConfigEnvelope) error {
	configMap, result, err := cm.prepareApply(configEnv)
	if err != nil {
		return err
	}

	result.commit()
	cm.commitCallbacks()

	cm.current = &configSet{
		configMap: configMap,
		channelID: cm.current.channelID,
		sequence:  configEnv.Config.Sequence,
	}

	return nil
}

// ChainID retrieves the chain ID associated with this manager
func (cm *configManager) ChainID() string {
	return cm.current.channelID
}

// Sequence returns the current sequence number of the config
func (cm *configManager) Sequence() uint64 {
	return cm.current.sequence
}
