/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestWithRealConfigtx(t *testing.T) {
	conf := configtxgentest.Load(genesisconfig.SampleSingleMSPSoloProfile)

	// None of the sample profiles define an application config section
	// in a genesis block (as this is a bad idea), but we combine them
	// here to better exercise the code.
	conf.Application = &genesisconfig.Application{
		Organizations: []*genesisconfig.Organization{
			conf.Orderer.Organizations[0],
		},
	}
	conf.Application.Organizations[0].AnchorPeers = []*genesisconfig.AnchorPeer{
		{
			Host: "foo",
			Port: 7,
		},
	}
	gb := encoder.New(conf).GenesisBlockForChannel("foo")
	env := utils.ExtractEnvelopeOrPanic(gb, 0)
	_, err := channelconfig.NewBundleFromEnvelope(env)
	assert.NoError(t, err)
}

func TestOrgSpecificOrdererEndpoints(t *testing.T) {
	t.Run("Without_Capability", func(t *testing.T) {
		conf := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		conf.Capabilities = map[string]bool{"V1_3": true}

		cg, err := encoder.NewChannelGroup(conf)
		assert.NoError(t, err)

		_, err = channelconfig.NewChannelConfig(cg)
		assert.EqualError(t, err, "could not create channel Orderer sub-group config: Orderer Org SampleOrg cannot contain endpoints value until V1_4_2+ capabilities have been enabled")
	})

	t.Run("Without_Capability_NoOSNs", func(t *testing.T) {
		conf := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		conf.Capabilities = map[string]bool{"V1_3": true}
		conf.Orderer.Organizations[0].OrdererEndpoints = nil
		conf.Orderer.Addresses = []string{}

		cg, err := encoder.NewChannelGroup(conf)
		assert.NoError(t, err)

		_, err = channelconfig.NewChannelConfig(cg)
		assert.EqualError(t, err, "Must set some OrdererAddresses")
	})

	t.Run("With_Capability", func(t *testing.T) {
		conf := configtxgentest.Load(genesisconfig.SampleDevModeSoloProfile)
		conf.Capabilities = map[string]bool{"V1_4_2": true}
		assert.NotEmpty(t, conf.Orderer.Organizations[0].OrdererEndpoints)

		cg, err := encoder.NewChannelGroup(conf)
		assert.NoError(t, err)

		cc, err := channelconfig.NewChannelConfig(cg)
		assert.NoError(t, err)

		err = cc.Validate(cc.Capabilities())
		assert.NoError(t, err)

		assert.NotEmpty(t, cc.OrdererConfig().Organizations()["SampleOrg"].Endpoints)
	})
}
