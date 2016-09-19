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

package config

import (
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

func getKeysRecursively(base string, v *viper.Viper, nodeKeys map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key := range nodeKeys {
		fqKey := base + key
		val := v.Get(fqKey)
		if m, ok := val.(map[interface{}]interface{}); ok {
			logger.Debugf("Found map value for %s", fqKey)
			tmp := make(map[string]interface{})
			for ik, iv := range m {
				cik, ok := ik.(string)
				if !ok {
					panic("Non string key-entry")
				}
				tmp[cik] = iv
			}
			result[key] = getKeysRecursively(fqKey+".", v, tmp)
		} else {
			logger.Debugf("Found real value for %s setting to %T %v", fqKey, val, val)
			result[key] = val
		}
	}
	return result
}

// ExactWithDateUnmarshal is intended to unmarshal a config file into a structure
// producing error when extraneous variables are introduced and supporting
// the time.Duration type
func ExactWithDateUnmarshal(v *viper.Viper, output interface{}) error {
	baseKeys := v.AllSettings() // AllKeys doesn't actually return all keys, it only returns the base ones
	leafKeys := getKeysRecursively("", v, baseKeys)

	logger.Infof("%+v", leafKeys)
	config := &mapstructure.DecoderConfig{
		ErrorUnused:      true,
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	return decoder.Decode(leafKeys)
}
