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
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
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
			config := configtxgentest.Load(profile)
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
	createConfig := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)

	configUpdate, err := NewChannelCreateConfigUpdate("channel.id", createConfig)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)
}

func TestGoodChannelCreateNoAnchorPeers(t *testing.T) {
	createConfig := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
	createConfig.Application.Organizations[0].AnchorPeers = nil

	configUpdate, err := NewChannelCreateConfigUpdate("channel.id", createConfig)
	assert.NoError(t, err)
	assert.NotNil(t, configUpdate)

	// Anchor peers should not be set
	assert.True(t, proto.Equal(
		configUpdate.WriteSet.Groups["Application"].Groups["SampleOrg"],
		&cb.ConfigGroup{},
	))
}

func TestChannelCreateWithResources(t *testing.T) {
	t.Run("AtV1.0", func(t *testing.T) {
		createConfig := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
		createConfig.Application.Capabilities = nil

		configUpdate, err := NewChannelCreateConfigUpdate("channel.id", createConfig)
		assert.NoError(t, err)
		assert.NotNil(t, configUpdate)
		assert.Nil(t, configUpdate.IsolatedData)
	})
}

func TestMakeChannelCreationTransactionWithSigner(t *testing.T) {
	channelID := "foo"

	msptesttools.LoadDevMsp()
	signer := localmsp.NewSigner()

	cct, err := MakeChannelCreationTransaction(channelID, signer, configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile))
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
	cct, err := MakeChannelCreationTransaction(channelID, nil, configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile))
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
		config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
		group, err := NewApplicationGroup(config.Application)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Application missing policies", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
		config.Application.Policies = nil
		for _, org := range config.Application.Organizations {
			org.Policies = nil
		}
		group, err := NewApplicationGroup(config.Application)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Application unknown MSP", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleSingleMSPChannelProfile)
		config.Application.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
		group, err := NewApplicationGroup(config.Application)
		assert.Error(t, err)
		assert.Nil(t, group)
	})
}

func TestNewChannelGroup(t *testing.T) {
	t.Run("Nil orderer", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer = nil
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Add test consortium", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Consortium = "Test"
		group, err := NewChannelGroup(config)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Channel missing policies", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Policies = nil
		group, err := NewChannelGroup(config)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Add application unknown MSP", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Application = &genesisconfig.Application{Organizations: []*genesisconfig.Organization{{Name: "FakeOrg"}}}
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Add consortiums unknown MSP", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Consortiums["fakeorg"] = &genesisconfig.Consortium{Organizations: []*genesisconfig.Organization{{Name: "FakeOrg"}}}
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Add orderer unknown MSP", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer = &genesisconfig.Orderer{Organizations: []*genesisconfig.Organization{{Name: "FakeOrg"}}}
		group, err := NewChannelGroup(config)
		assert.Error(t, err)
		assert.Nil(t, group)
	})
}

func TestNewOrdererGroup(t *testing.T) {
	t.Run("Unknown orderer type", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer.OrdererType = "TestOrderer"
		group, err := NewOrdererGroup(config.Orderer)
		assert.Error(t, err)
		assert.Nil(t, group)
	})

	t.Run("Orderer missing policies", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer.Policies = nil
		for _, org := range config.Orderer.Organizations {
			org.Policies = nil
		}
		group, err := NewOrdererGroup(config.Orderer)
		assert.NoError(t, err)
		assert.NotNil(t, group)
	})

	t.Run("Unknown MSP org", func(t *testing.T) {
		config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		config.Orderer.Organizations[0] = &genesisconfig.Organization{Name: "FakeOrg", ID: "FakeOrg"}
		group, err := NewOrdererGroup(config.Orderer)
		assert.Error(t, err)
		assert.Nil(t, group)
	})
}

func TestBootstrapper(t *testing.T) {
	config := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
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
