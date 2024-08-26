/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig_test

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
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
	t.Run("could not create channel orderer config with empty organization endpoints", func(t *testing.T) {
		conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())

		cg, err := encoder.NewChannelGroup(conf)
		require.NoError(t, err)

		cg.Groups["Orderer"].Groups["SampleOrg"].Values[channelconfig.EndpointsKey] = &common.ConfigValue{ModPolicy: channelconfig.AdminsPolicyKey}

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = channelconfig.NewChannelConfig(cg, cryptoProvider)
		require.EqualError(t, err, "could not create channel Orderer sub-group config: some orderer organizations endpoints are empty: [SampleOrg]")
	})

	t.Run("could not create channelgroup with empty organization endpoints", func(t *testing.T) {
		conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
		conf.Capabilities = map[string]bool{"V3_0": true}
		conf.Orderer.Organizations[0].OrdererEndpoints = nil
		conf.Orderer.Addresses = []string{}

		cg, err := encoder.NewChannelGroup(conf)
		require.Nil(t, cg)
		require.EqualError(t, err, "could not create orderer group: failed to create orderer org: orderer endpoints for organization SampleOrg are missing and must be configured when capability V3_0 is enabled")

		conf.Orderer.Organizations[0].OrdererEndpoints = []string{"127.0.0.1:7050"}
		cg, err = encoder.NewChannelGroup(conf)
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = channelconfig.NewChannelConfig(cg, cryptoProvider)
		require.NoError(t, err)
	})

	t.Run("With V2_0 Capability", func(t *testing.T) {
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

	t.Run("no global address With V3_0 Capability", func(t *testing.T) {
		conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
		conf.Orderer.Addresses = []string{"globalAddress"}
		conf.Capabilities = map[string]bool{"V3_0": true}
		require.NotEmpty(t, conf.Orderer.Organizations[0].OrdererEndpoints)
		require.NotEmpty(t, conf.Orderer.Addresses)

		_, err := encoder.NewChannelGroup(conf)
		require.EqualError(t, err, "could not create orderer group: global orderer endpoints exist, but can not be used with V3_0 capability: [globalAddress]")
	})
}
