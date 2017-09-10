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
	"regexp"

	"github.com/hyperledger/fabric/common/configtx/api"
	"github.com/hyperledger/fabric/common/flogging"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/golang/protobuf/proto"
)

var logger = flogging.MustGetLogger("common/configtx")

// Constraints for valid channel and config IDs
var (
	channelAllowedChars = "[a-z][a-z0-9.-]*"
	configAllowedChars  = "[a-zA-Z0-9.-]+"
	maxLength           = 249
	illegalNames        = map[string]struct{}{
		".":  struct{}{},
		"..": struct{}{},
	}
)

type configSet struct {
	channelID string
	sequence  uint64
	configMap map[string]comparable
	configEnv *cb.ConfigEnvelope
}

type configManager struct {
	api.Resources
	callOnUpdate []func(api.Manager)
	initializer  api.Initializer
	current      *configSet
}

// validateConfigID makes sure that the config element names (ie map key of
// ConfigGroup) comply with the following restrictions
//      1. Contain only ASCII alphanumerics, dots '.', dashes '-'
//      2. Are shorter than 250 characters.
//      3. Are not the strings "." or "..".
func validateConfigID(configID string) error {
	re, _ := regexp.Compile(configAllowedChars)
	// Length
	if len(configID) <= 0 {
		return fmt.Errorf("config ID illegal, cannot be empty")
	}
	if len(configID) > maxLength {
		return fmt.Errorf("config ID illegal, cannot be longer than %d", maxLength)
	}
	// Illegal name
	if _, ok := illegalNames[configID]; ok {
		return fmt.Errorf("name '%s' for config ID is not allowed", configID)
	}
	// Illegal characters
	matched := re.FindString(configID)
	if len(matched) != len(configID) {
		return fmt.Errorf("config ID '%s' contains illegal characters", configID)
	}

	return nil
}

// validateChannelID makes sure that proposed channel IDs comply with the
// following restrictions:
//      1. Contain only lower case ASCII alphanumerics, dots '.', and dashes '-'
//      2. Are shorter than 250 characters.
//      3. Start with a letter
//
// This is the intersection of the Kafka restrictions and CouchDB restrictions
// with the following exception: '.' is converted to '_' in the CouchDB naming
// This is to accomodate existing channel names with '.', especially in the
// behave tests which rely on the dot notation for their sluggification.
func validateChannelID(channelID string) error {
	re, _ := regexp.Compile(channelAllowedChars)
	// Length
	if len(channelID) <= 0 {
		return fmt.Errorf("channel ID illegal, cannot be empty")
	}
	if len(channelID) > maxLength {
		return fmt.Errorf("channel ID illegal, cannot be longer than %d", maxLength)
	}

	// Illegal characters
	matched := re.FindString(channelID)
	if len(matched) != len(channelID) {
		return fmt.Errorf("channel ID '%s' contains illegal characters", channelID)
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

	if configEnv.Config.ChannelGroup == nil {
		return nil, fmt.Errorf("nil channel group")
	}

	if err := validateChannelID(header.ChannelId); err != nil {
		return nil, fmt.Errorf("Bad channel id: %s", err)
	}

	configMap, err := MapConfig(configEnv.Config.ChannelGroup)
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
			configEnv: configEnv,
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
		return nil, fmt.Errorf("Error converting envelope to config update: %s", err)
	}

	configMap, err := cm.authorizeUpdate(configUpdateEnv)
	if err != nil {
		return nil, fmt.Errorf("Error authorizing update: %s", err)
	}

	channelGroup, err := configMapToConfig(configMap)
	if err != nil {
		return nil, fmt.Errorf("Could not turn configMap back to channelGroup: %s", err)
	}

	result, err := cm.processConfig(channelGroup)
	if err != nil {
		return nil, fmt.Errorf("Error processing updated config: %s", err)
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

func (cm *configManager) prepareApply(configEnv *cb.ConfigEnvelope) (*configResult, error) {
	if configEnv == nil {
		return nil, fmt.Errorf("Attempted to apply config with nil envelope")
	}

	if configEnv.Config == nil {
		return nil, fmt.Errorf("Config cannot be nil")
	}

	if configEnv.Config.Sequence != cm.current.sequence+1 {
		return nil, fmt.Errorf("Config at sequence %d, cannot prepare to update to %d", cm.current.sequence, configEnv.Config.Sequence)
	}

	configUpdateEnv, err := envelopeToConfigUpdate(configEnv.LastUpdate)
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

	// reflect.Equal will not work here, because it considers nil and empty maps as different
	if !proto.Equal(channelGroup, configEnv.Config.ChannelGroup) {
		return nil, fmt.Errorf("ConfigEnvelope LastUpdate did not produce the supplied config result")
	}

	result, err := cm.processConfig(channelGroup)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Validate simulates applying a ConfigEnvelope to become the new config
func (cm *configManager) Validate(configEnv *cb.ConfigEnvelope) error {
	result, err := cm.prepareApply(configEnv)
	if err != nil {
		return err
	}

	result.rollback()

	return nil
}

// Apply attempts to apply a ConfigEnvelope to become the new config
func (cm *configManager) Apply(configEnv *cb.ConfigEnvelope) error {
	// Note, although prepareApply will necessarilly compute a config map
	// for the updated config, this config map will possibly contain unreachable
	// elements from a config graph perspective.  Therefore, it is not safe to use
	// as the config map after application.  Instead, we compute the config map
	// just like we would at startup.
	configMap, err := MapConfig(configEnv.Config.ChannelGroup)
	if err != nil {
		return err
	}

	result, err := cm.prepareApply(configEnv)
	if err != nil {
		return err
	}

	result.commit()

	cm.current = &configSet{
		configMap: configMap,
		channelID: cm.current.channelID,
		sequence:  configEnv.Config.Sequence,
		configEnv: configEnv,
	}

	cm.commitCallbacks()

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

// ConfigEnvelope returns the current config envelope
func (cm *configManager) ConfigEnvelope() *cb.ConfigEnvelope {
	return cm.current.configEnv
}
