// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package localconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"
)

func TestLoadGoodConfig(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	cc := &configCache{}
	cfg, err := cc.load()
	require.NoError(t, err)
	require.NotNil(t, cfg, "Could not load config")
	require.Nil(t, err, "Load good config returned unexpected error")
}

func TestMissingConfigValueOverridden(t *testing.T) {
	t.Run("when the value is missing and not overridden", func(t *testing.T) {
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cc := &configCache{}
		cfg, err := cc.load()
		require.NotNil(t, cfg, "Could not load config")
		require.NoError(t, err, "Load good config returned unexpected error")
		require.Nil(t, cfg.General.TLS.ClientRootCAs)
	})

	t.Run("when the value is missing and is overridden", func(t *testing.T) {
		os.Setenv("ORDERER_GENERAL_TLS_CLIENTROOTCAS", "msp/tlscacerts/tlsroot.pem")
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cache := &configCache{}
		cfg, err := cache.load()
		require.NotNil(t, cfg, "Could not load config")
		require.NoError(t, err, "Load good config returned unexpected error")
		require.NotNil(t, cfg.Admin.TLS.ClientRootCAs)
	})
}

func TestLoadCached(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	// Load the initial config, update the environment, and load again.
	// With the caching behavior, the update should not be reflected
	initial, err := Load()
	require.NoError(t, err)
	os.Setenv("ORDERER_GENERAL_KEEPALIVE_SERVERTIMEOUT", "120s")
	updated, err := Load()
	require.NoError(t, err)
	require.Equal(t, initial, updated, "expected %#v to equal %#v", updated, initial)

	// Change the configuration we got back and load again.
	// The new value should not contain the update to the initial
	initial.General.LocalMSPDir = "/test/bad/mspDir"
	updated, err = Load()
	require.NoError(t, err)
	require.NotEqual(t, initial, updated, "expected %#v to not equal %#v", updated, initial)
}

func TestLoadMissingConfigFile(t *testing.T) {
	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := "invalid fabric cfg path"
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	cc := &configCache{}
	cfg, err := cc.load()
	require.Nil(t, cfg, "Loaded missing config file")
	require.NotNil(t, err, "Loaded missing config file without error")
}

func TestLoadMalformedConfigFile(t *testing.T) {
	name := t.TempDir()

	// Create a malformed orderer.yaml file in temp dir
	f, err := os.OpenFile(filepath.Join(name, "orderer.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	require.Nil(t, err, "Error creating file: %s", err)
	f.WriteString("General: 42")
	require.NoError(t, f.Close(), "Error closing file")

	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := name
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	cc := &configCache{}
	cfg, err := cc.load()
	require.Nil(t, cfg, "Loaded missing config file")
	require.NotNil(t, err, "Loaded missing config file without error")
}

// TestEnvInnerVar verifies that with the Unmarshal function that
// the environmental overrides still work on internal vars.  This was
// a bug in the original viper implementation that is worked around in
// the Load codepath for now
func TestEnvInnerVar(t *testing.T) {
	envVar1 := "ORDERER_GENERAL_LISTENPORT"
	envVal1 := uint16(80)
	envVar2 := "ORDERER_GENERAL_KEEPALIVE_SERVERTIMEOUT"
	envVal2 := "42s"
	os.Setenv(envVar1, fmt.Sprintf("%d", envVal1))
	os.Setenv(envVar2, envVal2)
	defer os.Unsetenv(envVar1)
	defer os.Unsetenv(envVar2)
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	cc := &configCache{}
	config, err := cc.load()
	require.NoError(t, err)

	require.NotNil(t, config, "Could not load config")
	require.Equal(t, config.General.ListenPort, envVal1, "Environmental override of inner config test 1 did not work")

	v2, _ := time.ParseDuration(envVal2)
	require.Equal(t, config.General.Keepalive.ServerTimeout, v2, "Environmental override of inner config test 2 did not work")
}

func TestAdminTLSConfig(t *testing.T) {
	testCases := []struct {
		name        string
		tls         TLS
		shouldPanic bool
	}{
		{
			name: "no TLS",
			tls: TLS{
				Enabled:            false,
				ClientAuthRequired: false,
			},
			shouldPanic: false,
		},
		{
			name: "TLS enabled and ClientAuthRequired",
			tls: TLS{
				Enabled:            true,
				ClientAuthRequired: true,
			},
			shouldPanic: false,
		},
		{
			name: "TLS enabled and ClientAuthRequired set to false",
			tls: TLS{
				Enabled:            true,
				ClientAuthRequired: false,
			},
			shouldPanic: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uconf := &TopLevel{Admin: Admin{TLS: tc.tls}}
			if tc.shouldPanic {
				require.PanicsWithValue(t, "Admin.TLS.ClientAuthRequired must be set to true if Admin.TLS.Enabled is set to true", func() { uconf.completeInitialization("/dummy/path") })
			} else {
				require.NotPanics(t, func() { uconf.completeInitialization("/dummy/path") }, "Should not panic")
			}
		})
	}
}

func TestClusterDefaults(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	cc := &configCache{}
	cfg, err := cc.load()
	require.NoError(t, err)
	require.Equal(t, cfg.General.Cluster.ReplicationMaxRetries, Defaults.General.Cluster.ReplicationMaxRetries)
}

func TestConsensusConfig(t *testing.T) {
	name := t.TempDir()

	content := `---
Consensus:
  Foo: bar
  Hello:
    World: 42
`

	f, err := os.OpenFile(filepath.Join(name, "orderer.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	require.Nil(t, err, "Error creating file: %s", err)
	f.WriteString(content)
	require.NoError(t, f.Close(), "Error closing file")

	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := name
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	cc := &configCache{}
	conf, err := cc.load()
	require.NoError(t, err, "Load good config returned unexpected error")
	require.NotNil(t, conf, "Could not load config")

	consensus := conf.Consensus
	require.IsType(t, map[string]interface{}{}, consensus, "Expected Consensus to be of type map[string]interface{}")

	foo := &struct {
		Foo   string
		Hello struct {
			World int
		}
	}{}
	err = mapstructure.Decode(consensus, foo)
	require.NoError(t, err, "Failed to decode Consensus to struct")
	require.Equal(t, foo.Foo, "bar")
	require.Equal(t, foo.Hello.World, 42)
}

func TestConnectionTimeout(t *testing.T) {
	t.Run("without connection timeout overridden", func(t *testing.T) {
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cc := &configCache{}
		cfg, err := cc.load()
		require.NotNil(t, cfg, "Could not load config")
		require.NoError(t, err, "Load good config returned unexpected error")
		require.Equal(t, cfg.General.ConnectionTimeout, time.Duration(0))
	})

	t.Run("with connection timeout overridden", func(t *testing.T) {
		os.Setenv("ORDERER_GENERAL_CONNECTIONTIMEOUT", "10s")
		defer os.Unsetenv("ORDERER_GENERAL_CONNECTIONTIMEOUT")
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()

		cc := &configCache{}
		cfg, err := cc.load()
		require.NotNil(t, cfg, "Could not load config")
		require.NoError(t, err, "Load good config returned unexpected error")
		require.Equal(t, cfg.General.ConnectionTimeout, 10*time.Second)
	})
}

func TestChannelParticipationDefaults(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	cc := &configCache{}
	cfg, err := cc.load()
	require.NoError(t, err)
	require.Equal(t, cfg.ChannelParticipation.Enabled, Defaults.ChannelParticipation.Enabled)
	require.Equal(t, cfg.ChannelParticipation.MaxRequestBodySize, Defaults.ChannelParticipation.MaxRequestBodySize)
}
