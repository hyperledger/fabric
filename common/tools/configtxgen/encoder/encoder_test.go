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
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

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

func TestMakeChannelCreationTransactionWithSigner(t *testing.T) {
	channelID := "foo"

	signer, err := mmsp.NewNoopMsp().GetDefaultSigningIdentity()
	assert.NoError(t, err, "Creating noop MSP")

	cct, err := MakeChannelCreationTransaction(channelID, "test", signer, nil)
	assert.NoError(t, err, "Making chain creation tx")

	assert.NotEmpty(t, cct.Signature, "Should have signature")

	payload, err := utils.UnmarshalPayload(cct.Payload)
	assert.NoError(t, err, "Unmarshaling payload")

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	assert.NoError(t, err, "Unmarshaling ConfigUpdateEnvelope")

	assert.NotEmpty(t, configUpdateEnv.Signatures, "Should have config env sigs")

	sigHeader, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	assert.NoError(t, err, "Unmarshaling SignatureHeader")
	assert.NotEmpty(t, sigHeader.Creator, "Creator specified")
}

func TestMakeChannelCreationTransactionNoSigner(t *testing.T) {
	channelID := "foo"
	cct, err := MakeChannelCreationTransaction(channelID, "test", nil, nil)
	assert.NoError(t, err, "Making chain creation tx")

	assert.Empty(t, cct.Signature, "Should have empty signature")

	payload, err := utils.UnmarshalPayload(cct.Payload)
	assert.NoError(t, err, "Unmarshaling payload")

	configUpdateEnv, err := configtx.UnmarshalConfigUpdateEnvelope(payload.Data)
	assert.NoError(t, err, "Unmarshaling ConfigUpdateEnvelope")

	assert.Empty(t, configUpdateEnv.Signatures, "Should have no config env sigs")

	sigHeader, err := utils.GetSignatureHeader(payload.Header.SignatureHeader)
	assert.NoError(t, err, "Unmarshaling SignatureHeader")
	assert.Empty(t, sigHeader.Creator, "No creator specified")
}
