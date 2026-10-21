/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"math"
	"slices"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	dp "github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/stretchr/testify/require"
)

const channelName = "mychannel"

func TestSorter(t *testing.T) {
	newState := func(address string, height uint64) *endorserState {
		return &endorserState{
			endorser: &endorser{endpointConfig: &endpointConfig{address: address}},
			height:   height,
		}
	}

	for _, test := range []struct {
		name     string
		host     string
		a, b     *endorserState
		expected int
	}{
		{
			name: "higher remote peer precedes lower host peer", host: "host",
			a: newState("remote", 5), b: newState("host", 4), expected: -1,
		},
		{
			name: "higher host peer precedes lower remote peer", host: "host",
			a: newState("host", 5), b: newState("remote", 4), expected: -1,
		},
		{
			name: "host peer wins at equal height", host: "host",
			a: newState("host", 4), b: newState("remote", 4), expected: -1,
		},
		{
			name: "remote peers at equal height compare equal", host: "host",
			a: newState("remote1", 4), b: newState("remote2", 4), expected: 0,
		},
		{
			name: "host peers at equal height compare equal", host: "host",
			a: newState("host", 4), b: newState("host", 4), expected: 0,
		},
		{
			name: "empty host orders by height",
			a:    newState("remote1", 5), b: newState("remote2", 4), expected: -1,
		},
		{
			name: "empty host leaves equal height remote peers equal",
			a:    newState("remote1", 4), b: newState("remote2", 4), expected: 0,
		},
		{
			name: "full uint64 height range", host: "host",
			a: newState("remote", math.MaxUint64), b: newState("host", 0), expected: -1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compare := sorter(test.host)
			require.Equal(t, test.expected, compare(test.a, test.b))
			require.Equal(t, -test.expected, compare(test.b, test.a))
			require.Zero(t, compare(test.a, test.a))
			require.Zero(t, compare(test.b, test.b))
		})
	}
}

func TestSorterPeerOrdering(t *testing.T) {
	host := &endorser{endpointConfig: &endpointConfig{address: "host"}}
	remote := &endorser{endpointConfig: &endpointConfig{address: "remote"}}
	higher := &endorserState{endorser: remote, height: 5}
	local := &endorserState{endorser: host, height: 4}
	equal := &endorserState{endorser: remote, height: 4}
	lower := &endorserState{endorser: remote, height: 3}
	want := []*endorserState{higher, local, equal, lower}

	// Check every input ordering so host preference cannot depend on its position.
	for a := range want {
		for b := range want {
			if b == a {
				continue
			}
			for c := range want {
				if c == a || c == b {
					continue
				}
				for d := range want {
					if d == a || d == b || d == c {
						continue
					}
					peers := []*endorserState{want[a], want[b], want[c], want[d]}
					slices.SortFunc(peers, sorter(host.address))
					require.Equal(t, want, peers, "input order: %v", []int{a, b, c, d})
				}
			}
		}
	}
}

func TestOrdererCache(t *testing.T) {
	def := &testDef{
		config: buildConfig(t, []string{"orderer1"}),
	}
	test := prepareTest(t, def)

	orderers, n, err := test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 1)
	require.Len(t, orderers[0].tlsRootCerts, 3) // 1 tlsrootCA + 2 tlsintermediateCAs
	require.Equal(t, 1, n)

	// trigger the config update callback, updating the orderers
	bundle, err := createChannelConfigBundle(channelName, []string{"orderer1:7050", "orderer2:7050", "orderer3:7050"})
	require.NoError(t, err)
	test.server.registry.configUpdate(bundle)
	orderers, n, err = test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 3)
	require.Len(t, orderers[2].tlsRootCerts, 2) // 1 tlsrootCA + 1 tlsintermediateCA from sampleconfig folder
	require.Equal(t, 3, n)
}

func TestStaleOrdererConnections(t *testing.T) {
	def := &testDef{
		config: buildConfig(t, []string{"orderer1", "orderer2", "orderer3"}),
	}
	test := prepareTest(t, def)

	orderers, n, err := test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 3)
	require.Equal(t, 3, n)

	closed := make([]bool, len(orderers))
	for i, o := range orderers {
		o.closeConnection = func(index int) func() error {
			return func() error {
				closed[index] = true
				return nil
			}
		}(i)
	}
	// trigger the config update callback, updating the orderers
	bundle, err := createChannelConfigBundle(channelName, []string{"orderer1:7050", "orderer3:7050"})
	require.NoError(t, err)
	test.server.registry.configUpdate(bundle)
	orderers, _, err = test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 2)
	require.False(t, closed[0])
	require.True(t, closed[1])
	require.False(t, closed[2])
}

func TestStaleMultiChannelOrdererConnections(t *testing.T) {
	channel1 := "channel1"

	def := &testDef{
		config: buildConfig(t, []string{"orderer1", "orderer2"}),
	}
	test := prepareTest(t, def)

	orderers, n, err := test.server.registry.orderers(channelName)
	require.NoError(t, err)
	require.Len(t, orderers, 2)
	require.Equal(t, 2, n)

	// trigger the config update callback, updating the orderers
	bundle, err := createChannelConfigBundle(channel1, []string{"orderer1:7050", "orderer3:7050", "orderer4:7050"})
	require.NoError(t, err)
	test.server.registry.configUpdate(bundle)
	orderers, n, err = test.server.registry.orderers(channel1)
	require.NoError(t, err)
	require.Len(t, orderers, 3)
	require.Equal(t, 3, n)

	closed := make([]bool, len(orderers))
	for i, o := range orderers {
		o.closeConnection = func(index int) func() error {
			return func() error {
				closed[index] = true
				return nil
			}
		}(i)
	}

	// new config update removes orderer1 and orderer3 from channel1 - should only trigger the closure of orderer3
	bundle, err = createChannelConfigBundle(channel1, []string{"orderer4:7050"})
	require.NoError(t, err)
	test.server.registry.configUpdate(bundle)
	orderers, n, err = test.server.registry.orderers(channel1)
	require.NoError(t, err)
	require.Len(t, orderers, 1)
	require.Equal(t, 1, n)

	require.False(t, closed[0]) // orderer1
	require.True(t, closed[1])  // orderer3
	require.False(t, closed[2]) // orderer4
}

func buildConfig(t *testing.T, orderers []string) *dp.ConfigResult {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	ica1, err := ca.NewIntermediateCA()
	require.NoError(t, err)
	ica2, err := ica1.NewIntermediateCA()
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
				TlsRootCerts:         [][]byte{ca.CertBytes()},
				TlsIntermediateCerts: [][]byte{ica1.CertBytes(), ica2.CertBytes()},
			},
		},
	}
}

func createChannelConfigBundle(channel string, endpoints []string) (*channelconfig.Bundle, error) {
	conf := genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile, configtest.GetDevConfigDir())
	conf.Capabilities = map[string]bool{"V2_0": true}
	conf.Orderer.Organizations[0].OrdererEndpoints = endpoints

	cg, err := encoder.NewChannelGroup(conf)
	if err != nil {
		return nil, err
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}

	return channelconfig.NewBundle(channel, &cb.Config{ChannelGroup: cg}, cryptoProvider)
}
