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
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"

	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestBadKey(t *testing.T) {
	assert.Error(t, addToMap(comparable{key: "[Label]", path: []string{}}, make(map[string]comparable)),
		"Should have errored on key with illegal characters")
}

func TestConfigMapMultiGroup(t *testing.T) {
	config := cb.NewConfigGroup()
	config.Groups["0"] = cb.NewConfigGroup()
	config.Groups["0"].Groups["1"] = cb.NewConfigGroup()
	config.Groups["0"].Groups["1"].Groups["2.1"] = cb.NewConfigGroup()
	config.Groups["0"].Groups["1"].Groups["2.1"].Values["Value"] = &cb.ConfigValue{}
	config.Groups["0"].Groups["1"].Groups["2.2"] = cb.NewConfigGroup()
	config.Groups["0"].Groups["1"].Groups["2.2"].Values["Value"] = &cb.ConfigValue{}

	confMap, err := MapConfig(config)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Channel", "0", "1", "2.1"}, confMap["[Values] /Channel/0/1/2.1/Value"].path)
	assert.Equal(t, []string{"Channel", "0", "1", "2.2"}, confMap["[Values] /Channel/0/1/2.2/Value"].path)
}

func TestConfigMap(t *testing.T) {
	config := cb.NewConfigGroup()
	config.Groups["0DeepGroup"] = cb.NewConfigGroup()
	config.Values["0DeepValue1"] = &cb.ConfigValue{}
	config.Values["0DeepValue2"] = &cb.ConfigValue{}
	config.Groups["0DeepGroup"].Policies["1DeepPolicy"] = &cb.ConfigPolicy{}
	config.Groups["0DeepGroup"].Groups["1DeepGroup"] = cb.NewConfigGroup()
	config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"] = &cb.ConfigValue{}

	confMap, err := MapConfig(config)
	assert.NoError(t, err, "Should not have errored building map")

	assert.Len(t, confMap, 7, "There should be 7 entries in the config map")

	assert.Equal(t, comparable{key: "Channel", path: []string{}, ConfigGroup: config},
		confMap["[Groups] /Channel"])
	assert.Equal(t, comparable{key: "0DeepGroup", path: []string{"Channel"}, ConfigGroup: config.Groups["0DeepGroup"]},
		confMap["[Groups] /Channel/0DeepGroup"])
	assert.Equal(t, comparable{key: "0DeepValue1", path: []string{"Channel"}, ConfigValue: config.Values["0DeepValue1"]},
		confMap["[Values] /Channel/0DeepValue1"])
	assert.Equal(t, comparable{key: "0DeepValue2", path: []string{"Channel"}, ConfigValue: config.Values["0DeepValue2"]},
		confMap["[Values] /Channel/0DeepValue2"])
	assert.Equal(t, comparable{key: "1DeepPolicy", path: []string{"Channel", "0DeepGroup"}, ConfigPolicy: config.Groups["0DeepGroup"].Policies["1DeepPolicy"]},
		confMap["[Policy] /Channel/0DeepGroup/1DeepPolicy"])
	assert.Equal(t, comparable{key: "1DeepGroup", path: []string{"Channel", "0DeepGroup"}, ConfigGroup: config.Groups["0DeepGroup"].Groups["1DeepGroup"]},
		confMap["[Groups] /Channel/0DeepGroup/1DeepGroup"])
	assert.Equal(t, comparable{key: "2DeepValue", path: []string{"Channel", "0DeepGroup", "1DeepGroup"}, ConfigValue: config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"]},
		confMap["[Values] /Channel/0DeepGroup/1DeepGroup/2DeepValue"])
}

func TestMapConfigBack(t *testing.T) {
	config := cb.NewConfigGroup()
	config.Groups["0DeepGroup"] = cb.NewConfigGroup()
	config.Values["0DeepValue1"] = &cb.ConfigValue{}
	config.Values["0DeepValue2"] = &cb.ConfigValue{}
	config.Groups["0DeepGroup"].Policies["1DeepPolicy"] = &cb.ConfigPolicy{}
	config.Groups["0DeepGroup"].Groups["1DeepGroup"] = cb.NewConfigGroup()
	config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"] = &cb.ConfigValue{}

	confMap, err := MapConfig(config)
	assert.NoError(t, err, "Should not have errored building map")

	newConfig, err := configMapToConfig(confMap)
	assert.NoError(t, err, "Should not have errored building config")

	assert.Equal(t, config, newConfig, "Should have transformed config map back from confMap")

	newConfig.Values["Value"] = &cb.ConfigValue{}
	assert.NotEqual(t, config, newConfig, "Mutating the new config should not mutate the existing config")
}
