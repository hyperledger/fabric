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
	"bytes"
	"fmt"

	ab "github.com/hyperledger/fabric/orderer/atomicbroadcast"
	"github.com/hyperledger/fabric/orderer/common/policies"

	"github.com/golang/protobuf/proto"
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
	ProposeConfig(configItem *ab.ConfigurationItem) error
}

// Manager provides a mechanism to query and update configuration
type Manager interface {
	// Apply attempts to apply a configtx to become the new configuration
	Apply(configtx *ab.ConfigurationEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	Validate(configtx *ab.ConfigurationEnvelope) error
}

// DefaultModificationPolicyID is the ID of the policy used when no other policy can be resolved, for instance when attempting to create a new config item
const DefaultModificationPolicyID = "DefaultModificationPolicy"

type acceptAllPolicy struct{}

func (ap *acceptAllPolicy) Evaluate(msg []byte, sigs []*ab.Envelope) error {
	return nil
}

type configurationManager struct {
	sequence      uint64
	chainID       []byte
	pm            policies.Manager
	configuration map[ab.ConfigurationItem_ConfigurationType]map[string]*ab.ConfigurationItem
	handlers      map[ab.ConfigurationItem_ConfigurationType]Handler
}

// NewConfigurationManager creates a new Manager unless an error is encountered
func NewConfigurationManager(configtx *ab.ConfigurationEnvelope, pm policies.Manager, handlers map[ab.ConfigurationItem_ConfigurationType]Handler) (Manager, error) {
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		if _, ok := handlers[ab.ConfigurationItem_ConfigurationType(ctype)]; !ok {
			return nil, fmt.Errorf("Must supply a handler for all known types")
		}
	}

	cm := &configurationManager{
		sequence:      configtx.Sequence - 1,
		chainID:       configtx.ChainID,
		pm:            pm,
		handlers:      handlers,
		configuration: makeConfigMap(),
	}

	err := cm.Apply(configtx)

	if err != nil {
		return nil, err
	}

	return cm, nil
}

func makeConfigMap() map[ab.ConfigurationItem_ConfigurationType]map[string]*ab.ConfigurationItem {
	configMap := make(map[ab.ConfigurationItem_ConfigurationType]map[string]*ab.ConfigurationItem)
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		configMap[ab.ConfigurationItem_ConfigurationType(ctype)] = make(map[string]*ab.ConfigurationItem)
	}
	return configMap
}

func (cm *configurationManager) beginHandlers() {
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		cm.handlers[ab.ConfigurationItem_ConfigurationType(ctype)].BeginConfig()
	}
}

func (cm *configurationManager) rollbackHandlers() {
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		cm.handlers[ab.ConfigurationItem_ConfigurationType(ctype)].RollbackConfig()
	}
}

func (cm *configurationManager) commitHandlers() {
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		cm.handlers[ab.ConfigurationItem_ConfigurationType(ctype)].CommitConfig()
	}
}

func (cm *configurationManager) processConfig(configtx *ab.ConfigurationEnvelope) (configMap map[ab.ConfigurationItem_ConfigurationType]map[string]*ab.ConfigurationItem, err error) {
	// Verify config is a sequential update to prevent exhausting sequence numbers
	if configtx.Sequence != cm.sequence+1 {
		return nil, fmt.Errorf("Config sequence number jumped from %d to %d", cm.sequence, configtx.Sequence)
	}

	// Verify config is intended for this globally unique chain ID
	if !bytes.Equal(configtx.ChainID, cm.chainID) {
		return nil, fmt.Errorf("Config is for the wrong chain, expected %x, got %x", cm.chainID, configtx.ChainID)
	}

	defaultModificationPolicy, defaultPolicySet := cm.pm.GetPolicy(DefaultModificationPolicyID)

	// If the default modification policy is not set, it indicates this is an uninitialized chain, so be permissive of modification
	if !defaultPolicySet {
		defaultModificationPolicy = &acceptAllPolicy{}
	}

	configMap = makeConfigMap()

	for _, entry := range configtx.Items {
		// Verify every entry is well formed
		config := &ab.ConfigurationItem{}
		err = proto.Unmarshal(entry.Configuration, config)
		if err != nil {
			return nil, err
		}

		// Ensure this configuration was intended for this chain
		if !bytes.Equal(config.ChainID, cm.chainID) {
			return nil, fmt.Errorf("Config item %v for type %v was not meant for a different chain %x", config.Key, config.Type, config.ChainID)
		}

		// Get the modification policy for this config item if one was previously specified
		// or the default if this is a new config item
		var policy policies.Policy
		oldItem, ok := cm.configuration[config.Type][config.Key]
		if ok {
			policy, _ = cm.pm.GetPolicy(oldItem.ModificationPolicy)
		} else {
			policy = defaultModificationPolicy
		}

		// Ensure the policy is satisfied
		if err = policy.Evaluate(entry.Configuration, entry.Signatures); err != nil {
			return nil, err
		}

		// Ensure the config sequence numbers are correct to prevent replay attacks
		isModified := false

		if val, ok := cm.configuration[config.Type][config.Key]; ok {
			// Config was modified if the LastModified or the Data contents changed
			isModified = (val.LastModified != config.LastModified) || !bytes.Equal(config.Value, val.Value)
		} else {
			if config.LastModified != configtx.Sequence {
				return nil, fmt.Errorf("Key %v for type %v was new, but had an older Sequence %d set", config.Key, config.Type, config.LastModified)
			}
			isModified = true
		}

		// If a config item was modified, its LastModified must be set correctly
		if isModified {
			if config.LastModified != configtx.Sequence {
				return nil, fmt.Errorf("Key %v for type %v was modified, but its LastModified %d does not equal current configtx Sequence %d", config.Key, config.Type, config.LastModified, configtx.Sequence)
			}
		}

		// Ensure the type handler agrees the config is well formed
		err = cm.handlers[config.Type].ProposeConfig(config)
		if err != nil {
			return nil, err
		}

		configMap[config.Type][config.Key] = config
	}

	// Ensure that any config items which used to exist still exist, to prevent implicit deletion
	for ctype := range ab.ConfigurationItem_ConfigurationType_name {
		curMap := cm.configuration[ab.ConfigurationItem_ConfigurationType(ctype)]
		newMap := configMap[ab.ConfigurationItem_ConfigurationType(ctype)]
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
func (cm *configurationManager) Validate(configtx *ab.ConfigurationEnvelope) error {
	cm.beginHandlers()
	_, err := cm.processConfig(configtx)
	cm.rollbackHandlers()
	return err
}

// Apply attempts to apply a configtx to become the new configuration
func (cm *configurationManager) Apply(configtx *ab.ConfigurationEnvelope) error {
	cm.beginHandlers()
	configMap, err := cm.processConfig(configtx)
	if err != nil {
		cm.rollbackHandlers()
		return err
	}
	cm.configuration = configMap
	cm.sequence = configtx.Sequence
	cm.commitHandlers()
	return nil
}
