/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/discovery/support/config"
	"github.com/hyperledger/fabric/discovery/support/mocks"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
)

func TestMSPIDMapping(t *testing.T) {
	randString := func() string {
		buff := make([]byte, 10)
		rand.Read(buff)
		return hex.EncodeToString(buff)
	}

	dir := filepath.Join(os.TempDir(), fmt.Sprintf("TestMSPIDMapping_%s", randString()))
	os.Mkdir(dir, 0o700)
	defer os.RemoveAll(dir)

	cryptogen, err := gexec.Build("github.com/hyperledger/fabric/cmd/cryptogen")
	require.NoError(t, err)
	defer os.Remove(cryptogen)

	idemixgen, err := gexec.Build("github.com/IBM/idemix/tools/idemixgen", "-mod=mod")
	require.NoError(t, err)
	defer os.Remove(idemixgen)

	cryptoConfigDir := filepath.Join(dir, "crypto-config")
	b, err := exec.Command(cryptogen, "generate", fmt.Sprintf("--output=%s", cryptoConfigDir)).CombinedOutput()
	require.NoError(t, err, string(b))

	idemixConfigDir := filepath.Join(dir, "crypto-config", "idemix")
	b, err = exec.Command(idemixgen, "ca-keygen", fmt.Sprintf("--output=%s", idemixConfigDir)).CombinedOutput()
	require.NoError(t, err, string(b))

	profileConfig := genesisconfig.Load("TwoOrgsChannel", "testdata/")
	ordererConfig := genesisconfig.Load("TwoOrgsOrdererGenesis", "testdata/")
	profileConfig.Orderer = ordererConfig.Orderer

	// Override the MSP directory with our randomly generated and populated path
	for _, org := range ordererConfig.Orderer.Organizations {
		org.MSPDir = filepath.Join(cryptoConfigDir, "ordererOrganizations", "example.com", "msp")
		org.Name = randString()
	}

	// Randomize organization names
	for _, org := range profileConfig.Application.Organizations {
		org.Name = randString()
		// Non bccsp-msp orgs don't have the crypto material produced by cryptogen,
		// we need to use the idemix crypto folder instead.
		if org.MSPType != "bccsp" {
			org.MSPDir = filepath.Join(idemixConfigDir)
			continue
		}
		org.MSPDir = filepath.Join(cryptoConfigDir, "peerOrganizations", "org1.example.com", "msp")
	}

	channelGroup, err := encoder.NewChannelGroup(profileConfig)
	require.NoError(t, err)
	fakeConfigGetter := &mocks.ConfigGetter{}
	fakeConfigGetter.GetCurrConfigReturnsOnCall(
		0,
		&common.Config{
			ChannelGroup: channelGroup,
		},
	)

	cs := config.NewDiscoverySupport(fakeConfigGetter)
	res, err := cs.Config("mychannel")
	require.NoError(t, err)

	actualKeys := make(map[string]struct{})
	for key := range res.Orderers {
		actualKeys[key] = struct{}{}
	}

	for key := range res.Msps {
		actualKeys[key] = struct{}{}
	}

	// Note that Org3MSP is an idemix org, but it shouldn't be listed here
	// because peers can't have idemix credentials
	expected := map[string]struct{}{
		"OrdererMSP": {},
		"Org1MSP":    {},
		"Org2MSP":    {},
	}
	require.Equal(t, expected, actualKeys)
}

func TestSupportGreenPath(t *testing.T) {
	fakeConfigGetter := &mocks.ConfigGetter{}
	fakeConfigGetter.GetCurrConfigReturnsOnCall(0, nil)

	cs := config.NewDiscoverySupport(fakeConfigGetter)
	res, err := cs.Config("test")
	require.Nil(t, res)
	require.Equal(t, "could not get last config for channel test", err.Error())

	config, err := test.MakeChannelConfig("test")
	require.NoError(t, err)
	require.NotNil(t, config)

	fakeConfigGetter.GetCurrConfigReturnsOnCall(1, config)
	res, err = cs.Config("test")
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        *common.Config
		containsError string
	}{
		{
			name:          "nil Config field",
			config:        &common.Config{},
			containsError: "field Config.ChannelGroup is nil",
		},
		{
			name: "nil Groups field",
			config: &common.Config{
				ChannelGroup: &common.ConfigGroup{},
			},
			containsError: "field Config.ChannelGroup.Groups is nil",
		},
		{
			name: "no orderer group key",
			config: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						channelconfig.ApplicationGroupKey: {},
					},
				},
			},
			containsError: "key Config.ChannelGroup.Groups[Orderer] is missing",
		},
		{
			name: "no application group key",
			config: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						channelconfig.OrdererGroupKey: {
							Groups: map[string]*common.ConfigGroup{},
						},
					},
				},
			},
			containsError: "key Config.ChannelGroup.Groups[Application] is missing",
		},
		{
			name: "no groups key in orderer group",
			config: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						channelconfig.ApplicationGroupKey: {
							Groups: map[string]*common.ConfigGroup{},
						},
						channelconfig.OrdererGroupKey: {},
					},
				},
			},
			containsError: "key Config.ChannelGroup.Groups[Orderer].Groups is nil",
		},
		{
			name: "no groups key in application group",
			config: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						channelconfig.ApplicationGroupKey: {},
						channelconfig.OrdererGroupKey: {
							Groups: map[string]*common.ConfigGroup{},
						},
					},
				},
			},
			containsError: "key Config.ChannelGroup.Groups[Application].Groups is nil",
		},
		{
			name: "no Values in ChannelGroup",
			config: &common.Config{
				ChannelGroup: &common.ConfigGroup{
					Groups: map[string]*common.ConfigGroup{
						channelconfig.ApplicationGroupKey: {
							Groups: map[string]*common.ConfigGroup{},
						},
						channelconfig.OrdererGroupKey: {
							Groups: map[string]*common.ConfigGroup{},
						},
					},
				},
			},
			containsError: "field Config.ChannelGroup.Values is nil",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := config.ValidateConfig(test.config)
			require.Contains(t, test.containsError, err.Error())
		})
	}
}

func TestOrdererEndpoints(t *testing.T) {
	t.Run("Global endpoints", func(t *testing.T) {
		channelConfig, err := test.MakeChannelConfig("mychannel")
		require.NoError(t, err)

		fakeConfigGetter := &mocks.ConfigGetter{}
		cs := config.NewDiscoverySupport(fakeConfigGetter)

		fakeConfigGetter.GetCurrConfigReturnsOnCall(0, channelConfig)

		injectGlobalOrdererEndpoint(t, channelConfig, "globalEndpoint:7050")

		res, err := cs.Config("test")
		require.NoError(t, err)
		require.Equal(t, map[string]*discovery.Endpoints{
			"SampleOrg": {Endpoint: []*discovery.Endpoint{{Host: "globalEndpoint", Port: 7050}}},
		}, res.Orderers)
	})

	t.Run("Per org endpoints alongside global endpoints", func(t *testing.T) {
		channelConfig, err := test.MakeChannelConfig("mychannel")
		require.NoError(t, err)

		fakeConfigGetter := &mocks.ConfigGetter{}
		cs := config.NewDiscoverySupport(fakeConfigGetter)

		fakeConfigGetter.GetCurrConfigReturnsOnCall(0, channelConfig)

		injectAdditionalEndpointPair(t, channelConfig, "perOrgEndpoint:7050", "anotherOrg")
		injectAdditionalEndpointPair(t, channelConfig, "endpointWithoutAPortName", "aBadOrg")

		res, err := cs.Config("test")
		require.NoError(t, err)
		require.Equal(t, map[string]*discovery.Endpoints{
			"SampleOrg":  {Endpoint: []*discovery.Endpoint{{Host: "127.0.0.1", Port: 7050}}},
			"anotherOrg": {Endpoint: []*discovery.Endpoint{{Host: "perOrgEndpoint", Port: 7050}}},
			"aBadOrg":    {},
		}, res.Orderers)
	})

	t.Run("Per org endpoints without global endpoints", func(t *testing.T) {
		channelConfig, err := test.MakeChannelConfig("mychannel")
		require.NoError(t, err)

		fakeConfigGetter := &mocks.ConfigGetter{}
		cs := config.NewDiscoverySupport(fakeConfigGetter)

		fakeConfigGetter.GetCurrConfigReturnsOnCall(0, channelConfig)

		removeGlobalEndpoints(t, channelConfig)
		injectAdditionalEndpointPair(t, channelConfig, "perOrgEndpoint:7050", "SampleOrg")
		injectAdditionalEndpointPair(t, channelConfig, "endpointWithoutAPortName", "aBadOrg")

		res, err := cs.Config("test")
		require.NoError(t, err)
		require.Equal(t, map[string]*discovery.Endpoints{
			"SampleOrg": {Endpoint: []*discovery.Endpoint{{Host: "perOrgEndpoint", Port: 7050}}},
			"aBadOrg":   {},
		}, res.Orderers)
	})
}

func removeGlobalEndpoints(t *testing.T, config *common.Config) {
	// Remove the orderer addresses
	delete(config.ChannelGroup.Values, channelconfig.OrdererAddressesKey)
}

func injectGlobalOrdererEndpoint(t *testing.T, config *common.Config, endpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{endpoint})
	// Replace the orderer addresses
	config.ChannelGroup.Values[ordererAddresses.Key()] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(ordererAddresses.Value()),
		ModPolicy: "/Channel/Orderer/Admins",
	}
	// Remove the per org addresses, if applicable
	ordererGrps := config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	for _, grp := range ordererGrps {
		if grp.Values[channelconfig.EndpointsKey] == nil {
			continue
		}
		grp.Values[channelconfig.EndpointsKey].Value = nil
	}
}

func injectAdditionalEndpointPair(t *testing.T, config *common.Config, endpoint string, orgName string) {
	ordererGrp := config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	// Get the first orderer org config
	var firstOrdererConfig *common.ConfigGroup
	for _, grp := range ordererGrp {
		firstOrdererConfig = grp
		break
	}
	// Duplicate it.
	secondOrdererConfig := proto.Clone(firstOrdererConfig).(*common.ConfigGroup)
	ordererGrp[orgName] = secondOrdererConfig
	// Reach the FabricMSPConfig buried in it.
	mspConfig := &msp.MSPConfig{}
	err := proto.Unmarshal(secondOrdererConfig.Values[channelconfig.MSPKey].Value, mspConfig)
	require.NoError(t, err)

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	require.NoError(t, err)

	// Rename it.
	fabricConfig.Name = orgName

	// Pack the MSP config back into the config
	secondOrdererConfig.Values[channelconfig.MSPKey].Value = protoutil.MarshalOrPanic(&msp.MSPConfig{
		Config: protoutil.MarshalOrPanic(fabricConfig),
		Type:   mspConfig.Type,
	})

	// Inject the endpoint
	ordererOrgProtos := &common.OrdererAddresses{
		Addresses: []string{endpoint},
	}
	secondOrdererConfig.Values[channelconfig.EndpointsKey].Value = protoutil.MarshalOrPanic(ordererOrgProtos)
}
