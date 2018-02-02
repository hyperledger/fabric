/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encoder

import (
	"testing"

	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/localmsp"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
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
		genesisconfig.SampleSingleMSPSoloV11Profile,
		genesisconfig.SampleDevModeSoloProfile,
		genesisconfig.SampleInsecureKafkaProfile,
		genesisconfig.SampleSingleMSPKafkaProfile,
		genesisconfig.SampleSingleMSPKafkaV11Profile,
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
	systemChannel, err := NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, systemChannel)

	createConfig := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile)

	configUpdate, err := NewChannelCreateConfigUpdate("channel.id", nil, createConfig)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)

	defaultConfigUpdate, err := NewChannelCreateConfigUpdate("channel.id", systemChannel, createConfig)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)

	assert.True(t, proto.Equal(configUpdate, defaultConfigUpdate), "the config used has had no updates, so should equal default")
}

func TestChannelCreateWithResources(t *testing.T) {
	t.Run("AtV1.0", func(t *testing.T) {
		createConfig := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile)

		configUpdate, err := NewChannelCreateConfigUpdate("channel.id", nil, createConfig)
		assert.NoError(t, err)
		assert.NotNil(t, configUpdate)
		assert.Nil(t, configUpdate.IsolatedData)
	})

	t.Run("AtV1.1", func(t *testing.T) {
		createConfig := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelV11Profile)
		createConfig.Application.Capabilities[capabilities.ApplicationResourcesTreeExperimental] = true

		configUpdate, err := NewChannelCreateConfigUpdate("channel.id", nil, createConfig)
		assert.NoError(t, err)
		assert.NotNil(t, configUpdate)
		assert.NotNil(t, configUpdate.IsolatedData)
		assert.NotEmpty(t, configUpdate.IsolatedData[pb.ResourceConfigSeedDataKey])
	})

}

func TestNegativeChannelCreateConfigUpdate(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
	channelConfig := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile)
	group, err := NewChannelGroup(config)
	assert.NoError(t, err)
	assert.NotNil(t, group)

	t.Run("NoGroups", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		channelGroup.Groups = nil
		_, err := NewChannelCreateConfigUpdate("channel.id", &cb.ConfigGroup{}, channelConfig)
		assert.Error(t, err)
		assert.Regexp(t, "missing all channel groups", err.Error())
	})

	t.Run("NoConsortiumsGroup", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		delete(channelGroup.Groups, channelconfig.ConsortiumsGroupKey)
		_, err := NewChannelCreateConfigUpdate("channel.id", channelGroup, channelConfig)
		assert.Error(t, err)
		assert.Regexp(t, "bad consortiums group", err.Error())
	})

	t.Run("NoConsortiums", func(t *testing.T) {
		channelGroup := proto.Clone(group).(*cb.ConfigGroup)
		delete(channelGroup.Groups[channelconfig.ConsortiumsGroupKey].Groups, genesisconfig.SampleConsortiumName)
		_, err := NewChannelCreateConfigUpdate("channel.id", channelGroup, channelConfig)
		assert.Error(t, err)
		assert.Regexp(t, "bad consortium:", err.Error())
	})
}

func TestMakeChannelCreationTransactionWithSigner(t *testing.T) {
	channelID := "foo"

	mspmgmt.LoadDevMsp()
	signer := localmsp.NewSigner()

	cct, err := MakeChannelCreationTransaction(channelID, signer, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelV11Profile))
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
	cct, err := MakeChannelCreationTransaction(channelID, nil, nil, genesisconfig.Load(genesisconfig.SampleSingleMSPChannelProfile))
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

func TestNewApplicationGroup(t *testing.T) {
	t.Run("Application with capabilities", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelV11Profile)
		group, err := NewApplicationGroup(config.Application)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Application unknown MSP", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleSingleMSPChannelV11Profile)
		config.Application.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
		group, err := NewApplicationGroup(config.Application)
		assert.Error(t, err)
		assert.Nil(t, group)
	})
}

func TestNewChannelGroup(t *testing.T) {
	t.Run("Nil orderer", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer = nil
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Add test consortium", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Consortium = "Test"
		group, err := NewChannelGroup(config)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Add application unknown MSP", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Application = &genesisconfig.Application{Organizations: []*genesisconfig.Organization{{Name: "FakeOrg"}}}
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Add consortiums unknown MSP", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Consortiums["fakeorg"] = &genesisconfig.Consortium{Organizations: []*genesisconfig.Organization{{Name: "FakeOrg"}}}
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Add orderer unknown MSP", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer = &genesisconfig.Orderer{Organizations: []*genesisconfig.Organization{{Name: "FakeOrg"}}}
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})
}

func TestNewOrdererGroup(t *testing.T) {
	t.Run("Unknown orderer type", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer.OrdererType = "TestOrderer"
		group, err := NewOrdererGroup(config.Orderer)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Unknown MSP org", func(t *testing.T) {
		config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
		group, err := NewOrdererGroup(config.Orderer)
		assert.Error(t, err)
		assert.Nil(t, group)
	})
}

func TestBootstrapper(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)
	t.Run("New bootstrapper", func(t *testing.T) {
		bootstrapper := New(config)
		assert.NotNil(t, bootstrapper.GenesisBlock(), "genesis block should not be nil")
		assert.NotNil(t, bootstrapper.GenesisBlockForChannel("channelID"), "genesis block for channel should not be nil")
	})

	t.Run("New bootstrapper nil orderer", func(t *testing.T) {
		config.Orderer = nil
		newBootstrapperNilOrderer := func() {
			New(config)
		}
		assert.Panics(t, newBootstrapperNilOrderer)
	})
}
