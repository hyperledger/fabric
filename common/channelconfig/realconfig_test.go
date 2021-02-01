/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig_test

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestWithRealConfigtx(t *testing.T) {
	conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())

	gb := encoder.New(conf).GenesisBlockForChannel("foo")
	env := protoutil.ExtractEnvelopeOrPanic(gb, 0)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	_, err = channelconfig.NewBundleFromEnvelope(env, cryptoProvider)
	require.NoError(t, err)
}

func TestOrgSpecificOrdererEndpoints(t *testing.T) {
	t.Run("Without_Capability", func(t *testing.T) {
		conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
		conf.Orderer.Addresses = []string{"127.0.0.1:7050"}
		conf.Capabilities = map[string]bool{"V1_3": true}

		cg, err := encoder.NewChannelGroup(conf)
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = channelconfig.NewChannelConfig(cg, cryptoProvider)
		require.EqualError(t, err, "could not create channel Orderer sub-group config: Orderer Org SampleOrg cannot contain endpoints value until V1_4_2+ capabilities have been enabled")
	})

	t.Run("Without_Capability_NoOSNs", func(t *testing.T) {
		conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
		conf.Capabilities = map[string]bool{"V1_3": true}
		conf.Orderer.Organizations[0].OrdererEndpoints = nil
		conf.Orderer.Addresses = []string{}

		cg, err := encoder.NewChannelGroup(conf)
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = channelconfig.NewChannelConfig(cg, cryptoProvider)
		require.EqualError(t, err, "Must set some OrdererAddresses")
	})

	t.Run("With_Capability", func(t *testing.T) {
		conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
		conf.Capabilities = map[string]bool{"V2_0": true}
		require.NotEmpty(t, conf.Orderer.Organizations[0].OrdererEndpoints)

		cg, err := encoder.NewChannelGroup(conf)
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		cc, err := channelconfig.NewChannelConfig(cg, cryptoProvider)
		require.NoError(t, err)

		err = cc.Validate(cc.Capabilities())
		require.NoError(t, err)

		require.NotEmpty(t, cc.OrdererConfig().Organizations()["SampleOrg"].Endpoints)
	})
}
