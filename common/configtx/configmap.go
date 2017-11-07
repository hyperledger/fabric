/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"fmt"
	"strings"

	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
)

const (
	groupPrefix  = "[Group]  "
	valuePrefix  = "[Value]  "
	policyPrefix = "[Policy] "

	pathSeparator = "/"

	// Hacky fix constants, used in recurseConfigMap
	hackyFixOrdererCapabilities = "[Value]  /Channel/Orderer/Capabilities"
	hackyFixNewModPolicy        = "Admins"
)

// mapConfig is intended to be called outside this file
// it takes a ConfigGroup and generates a map of fqPath to comparables (or error on invalid keys)
func mapConfig(channelGroup *cb.ConfigGroup, rootGroupKey string) (map[string]comparable, error) {
	result := make(map[string]comparable)
	if channelGroup != nil {
		err := recurseConfig(result, []string{rootGroupKey}, channelGroup)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// addToMap is used only internally by mapConfig
func addToMap(cg comparable, result map[string]comparable) error {
	var fqPath string

	switch {
	case cg.ConfigGroup != nil:
		fqPath = groupPrefix
	case cg.ConfigValue != nil:
		fqPath = valuePrefix
	case cg.ConfigPolicy != nil:
		fqPath = policyPrefix
	}

	if err := validateConfigID(cg.key); err != nil {
		return fmt.Errorf("Illegal characters in key: %s", fqPath)
	}

	if len(cg.path) == 0 {
		fqPath += pathSeparator + cg.key
	} else {
		fqPath += pathSeparator + strings.Join(cg.path, pathSeparator) + pathSeparator + cg.key
	}

	logger.Debugf("Adding to config map: %s", fqPath)

	result[fqPath] = cg

	return nil
}

// recurseConfig is used only internally by mapConfig
func recurseConfig(result map[string]comparable, path []string, group *cb.ConfigGroup) error {
	if err := addToMap(comparable{key: path[len(path)-1], path: path[:len(path)-1], ConfigGroup: group}, result); err != nil {
		return err
	}

	for key, group := range group.Groups {
		nextPath := make([]string, len(path)+1)
		copy(nextPath, path)
		nextPath[len(nextPath)-1] = key
		if err := recurseConfig(result, nextPath, group); err != nil {
			return err
		}
	}

	for key, value := range group.Values {
		if err := addToMap(comparable{key: key, path: path, ConfigValue: value}, result); err != nil {
			return err
		}
	}

	for key, policy := range group.Policies {
		if err := addToMap(comparable{key: key, path: path, ConfigPolicy: policy}, result); err != nil {
			return err
		}
	}

	return nil
}

// configMapToConfig is intended to be called from outside this file
// It takes a configMap and converts it back into a *cb.ConfigGroup structure
func configMapToConfig(configMap map[string]comparable, rootGroupKey string) (*cb.ConfigGroup, error) {
	rootPath := pathSeparator + rootGroupKey
	return recurseConfigMap(rootPath, configMap)
}

// recurseConfigMap is used only internally by configMapToConfig
// Note, this function no longer mutates the cb.Config* entries within configMap
func recurseConfigMap(path string, configMap map[string]comparable) (*cb.ConfigGroup, error) {
	groupPath := groupPrefix + path
	group, ok := configMap[groupPath]
	if !ok {
		return nil, fmt.Errorf("Missing group at path: %s", groupPath)
	}

	if group.ConfigGroup == nil {
		return nil, fmt.Errorf("ConfigGroup not found at group path: %s", groupPath)
	}

	newConfigGroup := cb.NewConfigGroup()
	proto.Merge(newConfigGroup, group.ConfigGroup)

	for key := range group.Groups {
		updatedGroup, err := recurseConfigMap(path+pathSeparator+key, configMap)
		if err != nil {
			return nil, err
		}
		newConfigGroup.Groups[key] = updatedGroup
	}

	for key := range group.Values {
		valuePath := valuePrefix + path + pathSeparator + key
		value, ok := configMap[valuePath]
		if !ok {
			return nil, fmt.Errorf("Missing value at path: %s", valuePath)
		}
		if value.ConfigValue == nil {
			return nil, fmt.Errorf("ConfigValue not found at value path: %s", valuePath)
		}
		newConfigGroup.Values[key] = proto.Clone(value.ConfigValue).(*cb.ConfigValue)
	}

	for key := range group.Policies {
		policyPath := policyPrefix + path + pathSeparator + key
		policy, ok := configMap[policyPath]
		if !ok {
			return nil, fmt.Errorf("Missing policy at path: %s", policyPath)
		}
		if policy.ConfigPolicy == nil {
			return nil, fmt.Errorf("ConfigPolicy not found at policy path: %s", policyPath)
		}
		newConfigGroup.Policies[key] = proto.Clone(policy.ConfigPolicy).(*cb.ConfigPolicy)
		logger.Debugf("Setting policy for key %s to %+v", key, group.Policies[key])
	}

	// This is a really very hacky fix to facilitate upgrading channels which were constructed
	// using the channel generation from v1.0 with bugs FAB-5309, and FAB-6080.
	// In summary, these channels were constructed with a bug which left mod_policy unset in some cases.
	// If mod_policy is unset, it's impossible to modify the element, and current code disallows
	// unset mod_policy values.  This hack 'fixes' existing config with empty mod_policy values.
	// If the capabilities framework is on, it sets any unset mod_policy to 'Admins'.
	// This code needs to sit here until validation of v1.0 chains is deprecated from the codebase.
	if _, ok := configMap[hackyFixOrdererCapabilities]; ok {
		// Hacky fix constants, used in recurseConfigMap
		if newConfigGroup.ModPolicy == "" {
			logger.Debugf("Performing upgrade of group %s empty mod_policy", groupPath)
			newConfigGroup.ModPolicy = hackyFixNewModPolicy
		}

		for key, value := range newConfigGroup.Values {
			if value.ModPolicy == "" {
				logger.Debugf("Performing upgrade of value %s empty mod_policy", valuePrefix+path+pathSeparator+key)
				value.ModPolicy = hackyFixNewModPolicy
			}
		}

		for key, policy := range newConfigGroup.Policies {
			if policy.ModPolicy == "" {
				logger.Debugf("Performing upgrade of policy %s empty mod_policy", policyPrefix+path+pathSeparator+key)

				policy.ModPolicy = hackyFixNewModPolicy
			}
		}
	}

	return newConfigGroup, nil
}
