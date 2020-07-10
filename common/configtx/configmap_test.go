/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestBadKey(t *testing.T) {
	require.Error(t, addToMap(comparable{key: "[Label]", path: []string{}}, make(map[string]comparable)),
		"Should have errored on key with illegal characters")
}

func TestConfigMapMultiGroup(t *testing.T) {
	config := protoutil.NewConfigGroup()
	config.Groups["0"] = protoutil.NewConfigGroup()
	config.Groups["0"].Groups["1"] = protoutil.NewConfigGroup()
	config.Groups["0"].Groups["1"].Groups["2.1"] = protoutil.NewConfigGroup()
	config.Groups["0"].Groups["1"].Groups["2.1"].Values["Value"] = &cb.ConfigValue{}
	config.Groups["0"].Groups["1"].Groups["2.2"] = protoutil.NewConfigGroup()
	config.Groups["0"].Groups["1"].Groups["2.2"].Values["Value"] = &cb.ConfigValue{}

	confMap, err := mapConfig(config, "Channel")
	require.NoError(t, err)
	require.Equal(t, []string{"Channel", "0", "1", "2.1"}, confMap["[Value]  /Channel/0/1/2.1/Value"].path)
	require.Equal(t, []string{"Channel", "0", "1", "2.2"}, confMap["[Value]  /Channel/0/1/2.2/Value"].path)
}

func TestConfigMap(t *testing.T) {
	config := protoutil.NewConfigGroup()
	config.Groups["0DeepGroup"] = protoutil.NewConfigGroup()
	config.Values["0DeepValue1"] = &cb.ConfigValue{}
	config.Values["0DeepValue2"] = &cb.ConfigValue{}
	config.Groups["0DeepGroup"].Policies["1DeepPolicy"] = &cb.ConfigPolicy{}
	config.Groups["0DeepGroup"].Groups["1DeepGroup"] = protoutil.NewConfigGroup()
	config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"] = &cb.ConfigValue{}

	confMap, err := mapConfig(config, "Channel")
	require.NoError(t, err, "Should not have errored building map")

	require.Len(t, confMap, 7, "There should be 7 entries in the config map")

	require.Equal(t, comparable{key: "Channel", path: []string{}, ConfigGroup: config},
		confMap["[Group]  /Channel"])
	require.Equal(t, comparable{key: "0DeepGroup", path: []string{"Channel"}, ConfigGroup: config.Groups["0DeepGroup"]},
		confMap["[Group]  /Channel/0DeepGroup"])
	require.Equal(t, comparable{key: "0DeepValue1", path: []string{"Channel"}, ConfigValue: config.Values["0DeepValue1"]},
		confMap["[Value]  /Channel/0DeepValue1"])
	require.Equal(t, comparable{key: "0DeepValue2", path: []string{"Channel"}, ConfigValue: config.Values["0DeepValue2"]},
		confMap["[Value]  /Channel/0DeepValue2"])
	require.Equal(t, comparable{key: "1DeepPolicy", path: []string{"Channel", "0DeepGroup"}, ConfigPolicy: config.Groups["0DeepGroup"].Policies["1DeepPolicy"]},
		confMap["[Policy] /Channel/0DeepGroup/1DeepPolicy"])
	require.Equal(t, comparable{key: "1DeepGroup", path: []string{"Channel", "0DeepGroup"}, ConfigGroup: config.Groups["0DeepGroup"].Groups["1DeepGroup"]},
		confMap["[Group]  /Channel/0DeepGroup/1DeepGroup"])
	require.Equal(t, comparable{key: "2DeepValue", path: []string{"Channel", "0DeepGroup", "1DeepGroup"}, ConfigValue: config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"]},
		confMap["[Value]  /Channel/0DeepGroup/1DeepGroup/2DeepValue"])
}

func TestMapConfigBack(t *testing.T) {
	config := protoutil.NewConfigGroup()
	config.Groups["0DeepGroup"] = protoutil.NewConfigGroup()
	config.Values["0DeepValue1"] = &cb.ConfigValue{}
	config.Values["0DeepValue2"] = &cb.ConfigValue{}
	config.Groups["0DeepGroup"].Policies["1DeepPolicy"] = &cb.ConfigPolicy{}
	config.Groups["0DeepGroup"].Groups["1DeepGroup"] = protoutil.NewConfigGroup()
	config.Groups["0DeepGroup"].Groups["1DeepGroup"].Values["2DeepValue"] = &cb.ConfigValue{}

	confMap, err := mapConfig(config, "Channel")
	require.NoError(t, err, "Should not have errored building map")

	newConfig, err := configMapToConfig(confMap, "Channel")
	require.NoError(t, err, "Should not have errored building config")

	require.Equal(t, config, newConfig, "Should have transformed config map back from confMap")

	newConfig.Values["Value"] = &cb.ConfigValue{}
	require.NotEqual(t, config, newConfig, "Mutating the new config should not mutate the existing config")
}

func TestHackInmapConfigBack(t *testing.T) {
	config := protoutil.NewConfigGroup()
	config.Values["ChannelValue1"] = &cb.ConfigValue{}
	config.Values["ChannelValue2"] = &cb.ConfigValue{}
	config.Groups["Orderer"] = protoutil.NewConfigGroup()
	config.Groups["Orderer"].Values["Capabilities"] = &cb.ConfigValue{}
	config.Groups["Orderer"].Policies["OrdererPolicy"] = &cb.ConfigPolicy{}
	config.Groups["Orderer"].Groups["OrdererOrg"] = protoutil.NewConfigGroup()
	config.Groups["Orderer"].Groups["OrdererOrg"].Values["OrdererOrgValue"] = &cb.ConfigValue{}
	config.Groups["Application"] = protoutil.NewConfigGroup()
	config.Groups["Application"].Policies["ApplicationPolicy"] = &cb.ConfigPolicy{}
	config.Groups["Application"].Policies["ApplicationValue"] = &cb.ConfigPolicy{}

	confMap, err := mapConfig(config, "Channel")
	require.NoError(t, err, "Should not have errored building map")

	newConfig, err := configMapToConfig(confMap, "Channel")
	require.NoError(t, err, "Should not have errored building config")

	var checkModPolicy func(cg *cb.ConfigGroup)
	checkModPolicy = func(cg *cb.ConfigGroup) {
		require.NotEmpty(t, cg.ModPolicy, "empty group mod_policy")

		for key, value := range cg.Values {
			require.NotEmpty(t, value.ModPolicy, "empty value mod_policy %s", key)
		}

		for key, policy := range cg.Policies {
			require.NotEmpty(t, policy.ModPolicy, "empty policy mod_policy %s", key)
		}

		for _, group := range cg.Groups {
			checkModPolicy(group)
		}
	}

	checkModPolicy(newConfig)
}
