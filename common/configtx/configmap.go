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
	"strings"

	cb "github.com/hyperledger/fabric/protos/common"
)

const (
	RootGroupKey = "Channel"

	GroupPrefix  = "[Groups] "
	ValuePrefix  = "[Values] "
	PolicyPrefix = "[Policy] " // The plurarility doesn't match, but, it makes the logs much easier being the same lenght as "Groups" and "Values"

	PathSeparator = "/"
)

// mapConfig is the only method in this file intended to be called outside this file
// it takes a ConfigGroup and generates a map of fqPath to comparables (or error on invalid keys)
func mapConfig(channelGroup *cb.ConfigGroup) (map[string]comparable, error) {
	result := make(map[string]comparable)
	err := recurseConfig(result, []string{RootGroupKey}, channelGroup)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func addToMap(cg comparable, result map[string]comparable) error {
	var fqPath string

	switch {
	case cg.ConfigGroup != nil:
		fqPath = GroupPrefix
	case cg.ConfigValue != nil:
		fqPath = ValuePrefix
	case cg.ConfigPolicy != nil:
		fqPath = PolicyPrefix
	}

	// TODO rename validateChainID to validateConfigID
	if err := validateChainID(cg.key); err != nil {
		return fmt.Errorf("Illegal characters in key: %s", fqPath)
	}

	if len(cg.path) == 0 {
		fqPath += PathSeparator + cg.key
	} else {
		fqPath += PathSeparator + strings.Join(cg.path, PathSeparator) + PathSeparator + cg.key
	}

	logger.Debugf("Adding to config map: %s", fqPath)

	result[fqPath] = cg

	return nil
}

func recurseConfig(result map[string]comparable, path []string, group *cb.ConfigGroup) error {
	if err := addToMap(comparable{key: path[len(path)-1], path: path[:len(path)-1], ConfigGroup: group}, result); err != nil {
		return err
	}

	for key, group := range group.Groups {
		nextPath := append(path, key)
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
