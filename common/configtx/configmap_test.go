/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"strconv"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
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
	require.Equal(t, comparable{key: "0DeepGroup", path: []string{"Channel"}, ConfigGroup: config.GetGroups()["0DeepGroup"]},
		confMap["[Group]  /Channel/0DeepGroup"])
	require.Equal(t, comparable{key: "0DeepValue1", path: []string{"Channel"}, ConfigValue: config.GetValues()["0DeepValue1"]},
		confMap["[Value]  /Channel/0DeepValue1"])
	require.Equal(t, comparable{key: "0DeepValue2", path: []string{"Channel"}, ConfigValue: config.GetValues()["0DeepValue2"]},
		confMap["[Value]  /Channel/0DeepValue2"])
	require.Equal(t, comparable{key: "1DeepPolicy", path: []string{"Channel", "0DeepGroup"}, ConfigPolicy: config.GetGroups()["0DeepGroup"].GetPolicies()["1DeepPolicy"]},
		confMap["[Policy] /Channel/0DeepGroup/1DeepPolicy"])
	require.Equal(t, comparable{key: "1DeepGroup", path: []string{"Channel", "0DeepGroup"}, ConfigGroup: config.GetGroups()["0DeepGroup"].GetGroups()["1DeepGroup"]},
		confMap["[Group]  /Channel/0DeepGroup/1DeepGroup"])
	require.Equal(t, comparable{key: "2DeepValue", path: []string{"Channel", "0DeepGroup", "1DeepGroup"}, ConfigValue: config.GetGroups()["0DeepGroup"].GetGroups()["1DeepGroup"].GetValues()["2DeepValue"]},
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
		require.NotEmpty(t, cg.GetModPolicy(), "empty group mod_policy")

		for key, value := range cg.GetValues() {
			require.NotEmpty(t, value.GetModPolicy(), "empty value mod_policy %s", key)
		}

		for key, policy := range cg.GetPolicies() {
			require.NotEmpty(t, policy.GetModPolicy(), "empty policy mod_policy %s", key)
		}

		for _, group := range cg.GetGroups() {
			checkModPolicy(group)
		}
	}

	checkModPolicy(newConfig)
}

// nestedChain builds a config of `chains` sibling group chains, each `depth`
// groups deep, beneath an empty root group.
func nestedChain(depth, chains int) *cb.ConfigGroup {
	root := protoutil.NewConfigGroup()
	for c := range chains {
		cur := root
		for d := range depth {
			next := protoutil.NewConfigGroup()
			cur.Groups[strconv.Itoa(c)+"x"+strconv.Itoa(d)] = next
			cur = next
		}
	}
	return root
}

func TestMapConfigNestingBounds(t *testing.T) {
	t.Run("AtMaxDepthSucceeds", func(t *testing.T) {
		// root plus maxConfigGroupDepth-1 nested groups: deepest path is exactly maxConfigGroupDepth
		confMap, err := mapConfig(nestedChain(maxConfigGroupDepth-1, 1), "Channel")
		require.NoError(t, err)
		require.Len(t, confMap, maxConfigGroupDepth)
	})

	t.Run("OverMaxDepthFails", func(t *testing.T) {
		_, err := mapConfig(nestedChain(maxConfigGroupDepth, 1), "Channel")
		require.ErrorContains(t, err, "exceeds maximum supported depth")
	})

	t.Run("DeepSiblingChainsFailFast", func(t *testing.T) {
		// far beyond what a config update can carry on the wire; must be
		// rejected at the depth cap instead of allocating quadratically
		start := time.Now()
		_, err := mapConfig(nestedChain(2000, 8), "Channel")
		require.ErrorContains(t, err, "exceeds maximum supported depth")
		t.Logf("rejected 8 sibling chains x 2000 depth in %s", time.Since(start))
	})

	t.Run("MaxEntriesSucceeds", func(t *testing.T) {
		config := protoutil.NewConfigGroup()
		for i := range maxConfigMapEntries - 1 {
			config.Values[strconv.Itoa(i)] = &cb.ConfigValue{}
		}
		confMap, err := mapConfig(config, "Channel")
		require.NoError(t, err)
		require.Len(t, confMap, maxConfigMapEntries)
	})

	t.Run("OverMaxEntriesFails", func(t *testing.T) {
		config := protoutil.NewConfigGroup()
		for i := range maxConfigMapEntries {
			config.Values[strconv.Itoa(i)] = &cb.ConfigValue{}
		}
		_, err := mapConfig(config, "Channel")
		require.ErrorContains(t, err, "more than the maximum supported")
	})
}

func TestMapConfigRealisticChannelConfig(t *testing.T) {
	// mirrors the deepest nesting a legitimate channel config reaches:
	// /Channel/Application/Org, /Channel/Orderer/Org and
	// /Channel/Consortiums/Consortium/Org
	config := protoutil.NewConfigGroup()
	app := protoutil.NewConfigGroup()
	appOrg := protoutil.NewConfigGroup()
	appOrg.Values["MSP"] = &cb.ConfigValue{}
	appOrg.Values["Endpoints"] = &cb.ConfigValue{}
	appOrg.Policies["Admins"] = &cb.ConfigPolicy{}
	app.Groups["Org1"] = appOrg
	app.Values["Capabilities"] = &cb.ConfigValue{}
	app.Values["ACLs"] = &cb.ConfigValue{}
	orderer := protoutil.NewConfigGroup()
	ordererOrg := protoutil.NewConfigGroup()
	ordererOrg.Values["MSP"] = &cb.ConfigValue{}
	ordererOrg.Values["Endpoints"] = &cb.ConfigValue{}
	orderer.Groups["OrdererOrg"] = ordererOrg
	orderer.Values["BatchSize"] = &cb.ConfigValue{}
	orderer.Values["ConsensusType"] = &cb.ConfigValue{}
	orderer.Values["Capabilities"] = &cb.ConfigValue{}
	consortiums := protoutil.NewConfigGroup()
	consortium := protoutil.NewConfigGroup()
	consortiumOrg := protoutil.NewConfigGroup()
	consortiumOrg.Values["MSP"] = &cb.ConfigValue{}
	consortium.Groups["Org1"] = consortiumOrg
	consortiums.Groups["SampleConsortium"] = consortium
	config.Groups["Application"] = app
	config.Groups["Orderer"] = orderer
	config.Groups["Consortiums"] = consortiums
	config.Policies["Admins"] = &cb.ConfigPolicy{}

	confMap, err := mapConfig(config, "Channel")
	require.NoError(t, err)
	require.Equal(t, []string{"Channel", "Consortiums", "SampleConsortium"},
		confMap["[Group]  /Channel/Consortiums/SampleConsortium/Org1"].path)

	newConfig, err := configMapToConfig(confMap, "Channel")
	require.NoError(t, err)
	require.Contains(t, newConfig.GetGroups()["Consortiums"].GetGroups()["SampleConsortium"].GetGroups(), "Org1")
	require.Contains(t, newConfig.GetGroups()["Application"].GetGroups(), "Org1")
	require.Contains(t, newConfig.GetGroups()["Orderer"].GetGroups(), "OrdererOrg")
}
