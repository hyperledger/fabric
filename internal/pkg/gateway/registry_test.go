/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	dp "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/stretchr/testify/require"
)

const channelName = "mychannel"

func TestOrdererCache(t *testing.T) {
	def := &testDef{
		config: buildConfig(t, []string{"orderer1"}),
	}
	test := prepareTest(t, def)

	orderers, err := test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 1)

	// add 2 more orderer nodes
	test.discovery.ConfigReturns(buildConfig(t, []string{"orderer1", "orderer2", "orderer3"}), nil)
	// the config is cached in the registry - so will still return the single orderer
	orderers, err = test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 1)

	// the config update callback is triggered, which will invalidate the cache
	bundle, err := createChannelConfigBundle()
	require.NoError(t, err)
	test.server.registry.configUpdate(bundle)
	orderers, err = test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 3)
}

func buildConfig(t *testing.T, orderers []string) *dp.ConfigResult {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	var endpoints []*dp.Endpoint
	for _, o := range orderers {
		endpoints = append(endpoints, &dp.Endpoint{Host: o, Port: 7050})
	}

	return &dp.ConfigResult{
		Orderers: map[string]*dp.Endpoints{
			"msp1": {
				Endpoint: endpoints,
			},
		},
		Msps: map[string]*msp.FabricMSPConfig{
			"msp1": {
				TlsRootCerts: [][]byte{ca.CertBytes()},
			},
		},
	}
}

func createChannelConfigBundle() (*channelconfig.Bundle, error) {
	conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
	conf.Capabilities = map[string]bool{"V2_0": true}

	cg, err := encoder.NewChannelGroup(conf)
	if err != nil {
		return nil, err
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}

	return channelconfig.NewBundle(channelName, &cb.Config{ChannelGroup: cg}, cryptoProvider)
}
