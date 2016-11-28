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

	"github.com/hyperledger/fabric/orderer/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"

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
	ProposeConfig(configItem *cb.ConfigurationItem) error
}

// Manager provides a mechanism to query and update configuration
type Manager interface {
	// Apply attempts to apply a configtx to become the new configuration
	Apply(configtx *cb.ConfigurationEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	Validate(configtx *cb.ConfigurationEnvelope) error

	// ChainID retrieves the chain ID associated with this manager
	ChainID() string
}

// DefaultModificationPolicyID is the ID of the policy used when no other policy can be resolved, for instance when attempting to create a new config item
const DefaultModificationPolicyID = "DefaultModificationPolicy"

type acceptAllPolicy struct{}

func (ap *acceptAllPolicy) Evaluate(headers [][]byte, payload []byte, identities [][]byte, signatures [][]byte) error {
	return nil
}

type configurationManager struct {
	sequence      uint64
	chainID       string
	pm            policies.Manager
	configuration map[cb.ConfigurationItem_ConfigurationType]map[string]*cb.ConfigurationItem
	handlers      map[cb.ConfigurationItem_ConfigurationType]Handler
}

// computeChainIDAndSequence returns the chain id and the sequence number for a configuration envelope
// or an error if there is a problem with the configuration envelope
func computeChainIDAndSequence(configtx *cb.ConfigurationEnvelope) (string, uint64, error) {
	if len(configtx.Items) == 0 {
		return "", 0, fmt.Errorf("Empty envelope unsupported")
	}

	m := uint64(0)     //configtx.Items[0].LastModified
	var chainID string //:= configtx.Items[0].Header.ChainID

	for _, signedItem := range configtx.Items {
		item := &cb.ConfigurationItem{}
		err := proto.Unmarshal(signedItem.ConfigurationItem, item)
		if err != nil {
			return "", 0, fmt.Errorf("Error unmarshaling signedItem.ConfigurationItem: %s", err)
		}

		if item.LastModified > m {
			m = item.LastModified
		}

		if item.Header == nil {
			return "", 0, fmt.Errorf("Header not set: %v", item)
		}

		if item.Header.ChainID == "" {
			return "", 0, fmt.Errorf("Header chainID was not set: %v", item)
		}

		if chainID == "" {
			chainID = item.Header.ChainID
		} else {
			if chainID != item.Header.ChainID {
				return "", 0, fmt.Errorf("Mismatched chainIDs in envelope %s != %s", chainID, item.Header.ChainID)
			}
		}
	}

	return chainID, m, nil
}

// NewConfigurationManager creates a new Manager unless an error is encountered
func NewConfigurationManager(configtx *cb.ConfigurationEnvelope, pm policies.Manager, handlers map[cb.ConfigurationItem_ConfigurationType]Handler) (Manager, error) {
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		if _, ok := handlers[cb.ConfigurationItem_ConfigurationType(ctype)]; !ok {
			return nil, fmt.Errorf("Must supply a handler for all known types")
		}
	}

	chainID, seq, err := computeChainIDAndSequence(configtx)
	if err != nil {
		return nil, err
	}

	cm := &configurationManager{
		sequence:      seq - 1,
		chainID:       chainID,
		pm:            pm,
		handlers:      handlers,
		configuration: makeConfigMap(),
	}

	err = cm.Apply(configtx)

	if err != nil {
		return nil, err
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
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		cm.handlers[cb.ConfigurationItem_ConfigurationType(ctype)].BeginConfig()
	}
}

func (cm *configurationManager) rollbackHandlers() {
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		cm.handlers[cb.ConfigurationItem_ConfigurationType(ctype)].RollbackConfig()
	}
}

func (cm *configurationManager) commitHandlers() {
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		cm.handlers[cb.ConfigurationItem_ConfigurationType(ctype)].CommitConfig()
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

	defaultModificationPolicy, defaultPolicySet := cm.pm.GetPolicy(DefaultModificationPolicyID)

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
			return nil, err
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

		headers := make([][]byte, len(entry.Signatures))
		signatures := make([][]byte, len(entry.Signatures))
		identities := make([][]byte, len(entry.Signatures))

		for i, configSig := range entry.Signatures {
			headers[i] = configSig.Signature
			signatures[i] = configSig.SignatureHeader
			sigHeader := &cb.SignatureHeader{}
			err := proto.Unmarshal(configSig.SignatureHeader, sigHeader)
			if err != nil {
				return nil, err
			}
			identities[i] = sigHeader.Creator
		}

		// Ensure the policy is satisfied
		if err = policy.Evaluate(headers, entry.ConfigurationItem, identities, signatures); err != nil {
			return nil, err
		}

		// Ensure the config sequence numbers are correct to prevent replay attacks
		isModified := false

		if val, ok := cm.configuration[config.Type][config.Key]; ok {
			// Config was modified if the LastModified or the Data contents changed
			isModified = (val.LastModified != config.LastModified) || !bytes.Equal(config.Value, val.Value)
		} else {
			if config.LastModified != seq {
				return nil, fmt.Errorf("Key %v for type %v was new, but had an older Sequence %d set", config.Key, config.Type, config.LastModified)
			}
			isModified = true
		}

		// If a config item was modified, its LastModified must be set correctly
		if isModified {
			if config.LastModified != seq {
				return nil, fmt.Errorf("Key %v for type %v was modified, but its LastModified %d does not equal current configtx Sequence %d", config.Key, config.Type, config.LastModified, seq)
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
