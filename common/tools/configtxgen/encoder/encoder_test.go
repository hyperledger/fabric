/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
}

func hasModPolicySet(t *testing.T, groupName string, cg *cb.ConfigGroup) {
	assert.NotEmpty(t, cg.ModPolicy, "group %s has empty mod_policy", groupName)

	for valueName, value := range cg.Values {
		assert.NotEmpty(t, value.ModPolicy, "group %s has value %s with empty mod_policy", groupName, valueName)
	}

	for policyName, policy := range cg.Policies {
		assert.NotEmpty(t, policy.ModPolicy, "group %s has policy %s with empty mod_policy", groupName, policyName)
	}

	for groupName, group := range cg.Groups {
		hasModPolicySet(t, groupName, group)
	}
}

func TestConfigParsing(t *testing.T) {
	for _, profile := range []string{
		genesisconfig.SampleInsecureSoloProfile,
		genesisconfig.SampleSingleMSPSoloProfile,
		genesisconfig.SampleDevModeSoloProfile,
		genesisconfig.SampleInsecureKafkaProfile,
		genesisconfig.SampleSingleMSPKafkaProfile,
		genesisconfig.SampleDevModeKafkaProfile,
	} {
		t.Run(profile, func(t *testing.T) {
			config := genesisconfig.Load(profile)
			group, err := NewChannelGroup(config)
			assert.NoError(t, err)
			assert.NotNil(t, group)

			_, err = channelconfig.NewBundle("test", &cb.Config{
				ChannelGroup: group,
			})
			assert.NoError(t, err)

			hasModPolicySet(t, "Channel", group)
		})
	}
}

func TestGoodChannelCreateConfigUpdate(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
	group, err := NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, group)

	configUpdate, err := NewChannelCreateConfigUpdate("channel.id", genesisconfig.SampleConsortiumName, []string{genesisconfig.SampleOrgName}, group)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)

	defaultConfigUpdate, err := NewChannelCreateConfigUpdate("channel.id", genesisconfig.SampleConsortiumName, []string{genesisconfig.SampleOrgName}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)

	assert.True(t, proto.Equal(configUpdate, defaultConfigUpdate), "the config used has had no updates, so should equal default")
}

func TestNegativeChannelCreateConfigUpdate(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
	group, err := NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, group)

	t.Run("NoGroups", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		channelGroup.Groups = nil
		_, err := NewChannelCreateConfigUpdate("channel.id", genesisconfig.SampleConsortiumName, []string{genesisconfig.SampleOrgName}, channelGroup)
		assert.Error(t, err)
		assert.Regexp(t, "missing all channel groups", err.Error())
	})

	t.Run("NoConsortiumsGroup", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		delete(channelGroup.Groups, channelconfig.ConsortiumsGroupKey)
		_, err := NewChannelCreateConfigUpdate("channel.id", genesisconfig.SampleConsortiumName, []string{genesisconfig.SampleOrgName}, channelGroup)
		assert.Error(t, err)
		assert.Regexp(t, "bad consortiums group", err.Error())
	})

	t.Run("NoConsortiums", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		delete(channelGroup.Groups[channelconfig.ConsortiumsGroupKey].Groups, genesisconfig.SampleConsortiumName)
		_, err := NewChannelCreateConfigUpdate("channel.id", genesisconfig.SampleConsortiumName, []string{genesisconfig.SampleOrgName}, channelGroup)
		assert.Error(t, err)
		assert.Regexp(t, "bad consortium:", err.Error())
	})

	t.Run("MissingOrg", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		_, err := NewChannelCreateConfigUpdate("channel.id", genesisconfig.SampleConsortiumName, []string{genesisconfig.SampleOrgName + ".wrong"}, channelGroup)
		assert.Error(t, err)
		assert.Regexp(t, "missing organization:", err.Error())
	})
}

// This is a temporary test to make sure that the newly implement channel creation method properly replicates
// the old behavior
func TestCompatability(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
	group, err := NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, group)

	channelID := "channel.id"
	orgs := []string{genesisconfig.SampleOrgName}
	configUpdate, err := NewChannelCreateConfigUpdate(channelID, genesisconfig.SampleConsortiumName, orgs, group)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)

	template := channelconfig.NewChainCreationTemplate(genesisconfig.SampleConsortiumName, orgs)
	configEnv, err := template.Envelope(channelID)
	assert.NoError(t, err)
	oldUpdate := configtx.UnmarshalConfigUpdateOrPanic(configEnv.ConfigUpdate)
	oldUpdate.IsolatedData = nil
	assert.True(t, proto.Equal(oldUpdate, configUpdate))
}
