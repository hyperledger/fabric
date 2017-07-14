/*
Copyright IBM Corp. 2016-2017 All Rights Reserved.

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
	"strings"

	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

func (c *configSet) verifyReadSet(readSet map[string]comparable) error {
	for key, value := range readSet {
		existing, ok := c.configMap[key]
		if !ok {
			return fmt.Errorf("Existing config does not contain element for %s but was in the read set", key)
		}

		if existing.version() != value.version() {
			return fmt.Errorf("Readset expected key %s at version %d, but got version %d", key, value.version(), existing.version())
		}
	}
	return nil
}

func ComputeDeltaSet(readSet, writeSet map[string]comparable) map[string]comparable {
	result := make(map[string]comparable)
	for key, value := range writeSet {
		readVal, ok := readSet[key]

		if ok && readVal.version() == value.version() {
			continue
		}

		// If the key in the readset is a different version, we include it
		// Error checking on the sanity of the update is done against the current config
		result[key] = value
	}
	return result
}

func validateModPolicy(modPolicy string) error {
	if modPolicy == "" {
		return fmt.Errorf("mod_policy not set")
	}

	trimmed := modPolicy
	if modPolicy[0] == '/' {
		trimmed = modPolicy[1:]
	}

	for i, pathElement := range strings.Split(trimmed, PathSeparator) {
		err := validateConfigID(pathElement)
		if err != nil {
			return fmt.Errorf("path element at %d is invalid: %s", i, err)
		}
	}
	return nil

}

func (cm *configManager) verifyDeltaSet(deltaSet map[string]comparable, signedData []*cb.SignedData) error {
	if len(deltaSet) == 0 {
		return fmt.Errorf("Delta set was empty.  Update would have no effect.")
	}

	for key, value := range deltaSet {
		if err := validateModPolicy(value.modPolicy()); err != nil {
			return fmt.Errorf("invalid mod_policy for element %s: %s", key, err)
		}

		existing, ok := cm.current.configMap[key]
		if !ok {
			if value.version() != 0 {
				return fmt.Errorf("Attempted to set key %s to version %d, but key does not exist", key, value.version())
			} else {
				continue
			}

		}
		if value.version() != existing.version()+1 {
			return fmt.Errorf("Attempt to set key %s to version %d, but key is at version %d", key, value.version(), existing.version())
		}

		policy, ok := cm.policyForItem(existing)
		if !ok {
			return fmt.Errorf("Unexpected missing policy %s for item %s", existing.modPolicy(), key)
		}

		// Ensure the policy is satisfied
		if err := policy.Evaluate(signedData); err != nil {
			return fmt.Errorf("Policy for %s not satisfied: %s", key, err)
		}
	}
	return nil
}

func verifyFullProposedConfig(writeSet, fullProposedConfig map[string]comparable) error {
	for key, _ := range writeSet {
		if _, ok := fullProposedConfig[key]; !ok {
			return fmt.Errorf("Writeset contained key %s which did not appear in proposed config", key)
		}
	}
	return nil
}

// authorizeUpdate validates that all modified config has the corresponding modification policies satisfied by the signature set
// it returns a map of the modified config
func (cm *configManager) authorizeUpdate(configUpdateEnv *cb.ConfigUpdateEnvelope) (map[string]comparable, error) {
	if configUpdateEnv == nil {
		return nil, fmt.Errorf("Cannot process nil ConfigUpdateEnvelope")
	}

	configUpdate, err := UnmarshalConfigUpdate(configUpdateEnv.ConfigUpdate)
	if err != nil {
		return nil, err
	}

	if configUpdate.ChannelId != cm.current.channelID {
		return nil, fmt.Errorf("Update not for correct channel: %s for %s", configUpdate.ChannelId, cm.current.channelID)
	}

	readSet, err := MapConfig(configUpdate.ReadSet)
	if err != nil {
		return nil, fmt.Errorf("Error mapping ReadSet: %s", err)
	}
	err = cm.current.verifyReadSet(readSet)
	if err != nil {
		return nil, fmt.Errorf("Error validating ReadSet: %s", err)
	}

	writeSet, err := MapConfig(configUpdate.WriteSet)
	if err != nil {
		return nil, fmt.Errorf("Error mapping WriteSet: %s", err)
	}

	deltaSet := ComputeDeltaSet(readSet, writeSet)
	signedData, err := configUpdateEnv.AsSignedData()
	if err != nil {
		return nil, err
	}

	if err = cm.verifyDeltaSet(deltaSet, signedData); err != nil {
		return nil, fmt.Errorf("Error validating DeltaSet: %s", err)
	}

	fullProposedConfig := cm.computeUpdateResult(deltaSet)
	if err := verifyFullProposedConfig(writeSet, fullProposedConfig); err != nil {
		return nil, fmt.Errorf("Full config did not verify: %s", err)
	}

	return fullProposedConfig, nil
}

func (cm *configManager) policyForItem(item comparable) (policies.Policy, bool) {
	// path is always at least of length 1
	manager, ok := cm.PolicyManager().Manager(item.path[1:])
	if !ok {
		return nil, ok
	}

	// In the case of the group type, its key is part of its path for the purposes of finding the policy manager
	if item.ConfigGroup != nil {
		manager, ok = manager.Manager([]string{item.key})
	}
	if !ok {
		return nil, ok
	}
	return manager.GetPolicy(item.modPolicy())
}

// computeUpdateResult takes a configMap generated by an update and produces a new configMap overlaying it onto the old config
func (cm *configManager) computeUpdateResult(updatedConfig map[string]comparable) map[string]comparable {
	newConfigMap := make(map[string]comparable)
	for key, value := range cm.current.configMap {
		newConfigMap[key] = value
	}

	for key, value := range updatedConfig {
		newConfigMap[key] = value
	}
	return newConfigMap
}

func envelopeToConfigUpdate(configtx *cb.Envelope) (*cb.ConfigUpdateEnvelope, error) {
	configUpdateEnv := &cb.ConfigUpdateEnvelope{}
	_, err := utils.UnmarshalEnvelopeOfType(configtx, cb.HeaderType_CONFIG_UPDATE, configUpdateEnv)
	if err != nil {
		return nil, err
	}
	return configUpdateEnv, nil
}
