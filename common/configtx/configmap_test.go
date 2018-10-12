/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"testing"

	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

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

	confMap, err := mapConfig(config, "Channel")
	assert.NoError(t, err)
	assert.Equal(t, []string{"Channel", "0", "1", "2.1"}, confMap["[Value]  /Channel/0/1/2.1/Value"].path)
	assert.Equal(t, []string{"Channel", "0", "1", "2.2"}, confMap["[Value]  /Channel/0/1/2.2/Value"].path)
}

func TestConfigMap(t *testing.T) {
	config := cb.NewConfigGroup()
	config.Groups["0DeepGroup"] = cb.NewConfigGroup()
	config.Values["0DeepValue1"] = &cb.ConfigValue{}
	config.Values["0DeepValue2"] = &cb.ConfigValue{}
	config.Groups["0DeepGroup"].Policies["1DeepPolicy"] = &cb.ConfigPolicy{}
	config.Groups["0DeepGroup"].Groups["1DeepGroup"] = cb.NewConfigGroup()
	config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"] = &cb.ConfigValue{}

	confMap, err := mapConfig(config, "Channel")
	assert.NoError(t, err, "Should not have errored building map")

	assert.Len(t, confMap, 7, "There should be 7 entries in the config map")

	assert.Equal(t, comparable{key: "Channel", path: []string{}, ConfigGroup: config},
		confMap["[Group]  /Channel"])
	assert.Equal(t, comparable{key: "0DeepGroup", path: []string{"Channel"}, ConfigGroup: config.Groups["0DeepGroup"]},
		confMap["[Group]  /Channel/0DeepGroup"])
	assert.Equal(t, comparable{key: "0DeepValue1", path: []string{"Channel"}, ConfigValue: config.Values["0DeepValue1"]},
		confMap["[Value]  /Channel/0DeepValue1"])
	assert.Equal(t, comparable{key: "0DeepValue2", path: []string{"Channel"}, ConfigValue: config.Values["0DeepValue2"]},
		confMap["[Value]  /Channel/0DeepValue2"])
	assert.Equal(t, comparable{key: "1DeepPolicy", path: []string{"Channel", "0DeepGroup"}, ConfigPolicy: config.Groups["0DeepGroup"].Policies["1DeepPolicy"]},
		confMap["[Policy] /Channel/0DeepGroup/1DeepPolicy"])
	assert.Equal(t, comparable{key: "1DeepGroup", path: []string{"Channel", "0DeepGroup"}, ConfigGroup: config.Groups["0DeepGroup"].Groups["1DeepGroup"]},
		confMap["[Group]  /Channel/0DeepGroup/1DeepGroup"])
	assert.Equal(t, comparable{key: "2DeepValue", path: []string{"Channel", "0DeepGroup", "1DeepGroup"}, ConfigValue: config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"]},
		confMap["[Value]  /Channel/0DeepGroup/1DeepGroup/2DeepValue"])
}

func TestMapConfigBack(t *testing.T) {
	config := cb.NewConfigGroup()
	config.Groups["0DeepGroup"] = cb.NewConfigGroup()
	config.Values["0DeepValue1"] = &cb.ConfigValue{}
	config.Values["0DeepValue2"] = &cb.ConfigValue{}
	config.Groups["0DeepGroup"].Policies["1DeepPolicy"] = &cb.ConfigPolicy{}
	config.Groups["0DeepGroup"].Groups["1DeepGroup"] = cb.NewConfigGroup()
	config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"] = &cb.ConfigValue{}

	confMap, err := mapConfig(config, "Channel")
	assert.NoError(t, err, "Should not have errored building map")

	newConfig, err := configMapToConfig(confMap, "Channel")
	assert.NoError(t, err, "Should not have errored building config")

	assert.Equal(t, config, newConfig, "Should have transformed config map back from confMap")

	newConfig.Values["Value"] = &cb.ConfigValue{}
	assert.NotEqual(t, config, newConfig, "Mutating the new config should not mutate the existing config")
}

func TestHackInmapConfigBack(t *testing.T) {
	config := cb.NewConfigGroup()
	config.Values["ChannelValue1"] = &cb.ConfigValue{}
	config.Values["ChannelValue2"] = &cb.ConfigValue{}
	config.Groups["Orderer"] = cb.NewConfigGroup()
	config.Groups["Orderer"].Values["Capabilities"] = &cb.ConfigValue{}
	config.Groups["Orderer"].Policies["OrdererPolicy"] = &cb.ConfigPolicy{}
	config.Groups["Orderer"].Groups["OrdererOrg"] = cb.NewConfigGroup()
	config.Groups["Orderer"].Groups["OrdererOrg"].Values["OrdererOrgValue"] = &cb.ConfigValue{}
	config.Groups["Application"] = cb.NewConfigGroup()
	config.Groups["Application"].Policies["ApplicationPolicy"] = &cb.ConfigPolicy{}
	config.Groups["Application"].Policies["ApplicationValue"] = &cb.ConfigPolicy{}

	confMap, err := mapConfig(config, "Channel")
	assert.NoError(t, err, "Should not have errored building map")

	newConfig, err := configMapToConfig(confMap, "Channel")
	assert.NoError(t, err, "Should not have errored building config")

	var checkModPolicy func(cg *cb.ConfigGroup)
	checkModPolicy = func(cg *cb.ConfigGroup) {
		assert.NotEmpty(t, cg.ModPolicy, "empty group mod_policy")

		for key, value := range cg.Values {
			assert.NotEmpty(t, value.ModPolicy, "empty value mod_policy %s", key)
		}

		for key, policy := range cg.Policies {
			assert.NotEmpty(t, policy.ModPolicy, "empty policy mod_policy %s", key)
		}

		for _, group := range cg.Groups {
			checkModPolicy(group)
		}
	}

	checkModPolicy(newConfig)
}
