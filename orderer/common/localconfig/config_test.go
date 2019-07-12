// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package localconfig

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/stretchr/testify/assert"
)

func TestLoadGoodConfig(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	cfg, err := Load()
	assert.NotNil(t, cfg, "Could not load config")
	assert.Nil(t, err, "Load good config returned unexpected error")
}

func TestMissingConfigValueOverridden(t *testing.T) {
	t.Run("when the value is missing and not overridden", func(t *testing.T) {
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cfg, err := Load()
		assert.NotNil(t, cfg, "Could not load config")
		assert.NoError(t, err, "Load good config returned unexpected error")
		assert.Nil(t, cfg.Kafka.TLS.ClientRootCAs)
	})

	t.Run("when the value is missing and is overridden", func(t *testing.T) {
		os.Setenv("ORDERER_KAFKA_TLS_CLIENTROOTCAS", "msp/tlscacerts/tlsroot.pem")
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cfg, err := Load()
		assert.NotNil(t, cfg, "Could not load config")
		assert.NoError(t, err, "Load good config returned unexpected error")
		assert.NotNil(t, cfg.Kafka.TLS.ClientRootCAs)
	})
}

func TestLoadMissingConfigFile(t *testing.T) {
	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := "invalid fabric cfg path"
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	cfg, err := Load()
	assert.Nil(t, cfg, "Loaded missing config file")
	assert.NotNil(t, err, "Loaded missing config file without error")
}

func TestLoadMalformedConfigFile(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer func() {
		err = os.RemoveAll(name)
		assert.Nil(t, os.RemoveAll(name), "Error removing temp dir: %s", err)
	}()

	{
		// Create a malformed orderer.yaml file in temp dir
		f, err := os.OpenFile(filepath.Join(name, "orderer.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		assert.Nil(t, err, "Error creating file: %s", err)
		f.WriteString("General: 42")
		assert.NoError(t, f.Close(), "Error closing file")
	}

	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := name
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	cfg, err := Load()
	assert.Nil(t, cfg, "Loaded missing config file")
	assert.NotNil(t, err, "Loaded missing config file without error")
}

// TestEnvInnerVar verifies that with the Unmarshal function that
// the environmental overrides still work on internal vars.  This was
// a bug in the original viper implementation that is worked around in
// the Load codepath for now
func TestEnvInnerVar(t *testing.T) {
	envVar1 := "ORDERER_GENERAL_LISTENPORT"
	envVal1 := uint16(80)
	envVar2 := "ORDERER_KAFKA_RETRY_SHORTINTERVAL"
	envVal2 := "42s"
	os.Setenv(envVar1, fmt.Sprintf("%d", envVal1))
	os.Setenv(envVar2, envVal2)
	defer os.Unsetenv(envVar1)
	defer os.Unsetenv(envVar2)
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	config, _ := Load()

	assert.NotNil(t, config, "Could not load config")
	assert.Equal(t, config.General.ListenPort, envVal1, "Environmental override of inner config test 1 did not work")

	v2, _ := time.ParseDuration(envVal2)
	assert.Equal(t, config.Kafka.Retry.ShortInterval, v2, "Environmental override of inner config test 2 did not work")
}

func TestKafkaTLSConfig(t *testing.T) {
	testCases := []struct {
		name        string
		tls         TLS
		shouldPanic bool
	}{
		{"Disabled", TLS{Enabled: false}, false},
		{"EnabledNoPrivateKey", TLS{Enabled: true, Certificate: "public.key"}, true},
		{"EnabledNoPublicKey", TLS{Enabled: true, PrivateKey: "private.key"}, true},
		{"EnabledNoTrustedRoots", TLS{Enabled: true, PrivateKey: "private.key", Certificate: "public.key"}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uconf := &TopLevel{Kafka: Kafka{TLS: tc.tls}}
			if tc.shouldPanic {
				assert.Panics(t, func() { uconf.completeInitialization("/dummy/path") }, "Should panic")
			} else {
				assert.NotPanics(t, func() { uconf.completeInitialization("/dummy/path") }, "Should not panic")
			}
		})
	}
}

func TestKafkaSASLPlain(t *testing.T) {
	testCases := []struct {
		name        string
		sasl        SASLPlain
		shouldPanic bool
	}{
		{"Disabled", SASLPlain{Enabled: false}, false},
		{"EnabledUserPassword", SASLPlain{Enabled: true, User: "user", Password: "pwd"}, false},
		{"EnabledNoUserPassword", SASLPlain{Enabled: true}, true},
		{"EnabledNoUser", SASLPlain{Enabled: true, Password: "pwd"}, true},
		{"EnabledNoPassword", SASLPlain{Enabled: true, User: "user"}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uconf := &TopLevel{Kafka: Kafka{SASLPlain: tc.sasl}}
			if tc.shouldPanic {
				assert.Panics(t, func() { uconf.completeInitialization("/dummy/path") }, "Should panic")
			} else {
				assert.NotPanics(t, func() { uconf.completeInitialization("/dummy/path") }, "Should not panic")
			}
		})
	}
}

func TestClusterDefaults(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	cfg, err := Load()

	assert.NoError(t, err)
	assert.Equal(t, cfg.General.Cluster.ReplicationMaxRetries, Defaults.General.Cluster.ReplicationMaxRetries)
}

func TestSystemChannel(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	conf, _ := Load()
	assert.Equal(t, Defaults.General.SystemChannel, conf.General.SystemChannel,
		"Expected default system channel ID to be '%s', got '%s' instead", Defaults.General.SystemChannel, conf.General.SystemChannel)
}

func TestConsensusConfig(t *testing.T) {
	name, err := ioutil.TempDir("", "hyperledger_fabric")
	assert.Nil(t, err, "Error creating temp dir: %s", err)
	defer func() {
		err = os.RemoveAll(name)
		assert.Nil(t, os.RemoveAll(name), "Error removing temp dir: %s", err)
	}()

	content := `---
Consensus:
  Foo: bar
  Hello:
    World: 42
`

	f, err := os.OpenFile(filepath.Join(name, "orderer.yaml"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	assert.Nil(t, err, "Error creating file: %s", err)
	f.WriteString(content)
	assert.NoError(t, f.Close(), "Error closing file")

	envVar1 := "FABRIC_CFG_PATH"
	envVal1 := name
	os.Setenv(envVar1, envVal1)
	defer os.Unsetenv(envVar1)

	conf, err := Load()
	assert.NoError(t, err, "Load good config returned unexpected error")
	assert.NotNil(t, conf, "Could not load config")

	consensus := conf.Consensus
	assert.IsType(t, map[string]interface{}{}, consensus, "Expected Consensus to be of type map[string]interface{}")

	foo := &struct {
		Foo   string
		Hello struct {
			World int
		}
	}{}
	err = viperutil.Decode(consensus, foo)
	assert.NoError(t, err, "Failed to decode Consensus to struct")
	assert.Equal(t, foo.Foo, "bar")
	assert.Equal(t, foo.Hello.World, 42)
}

func TestConnectionTimeout(t *testing.T) {
	t.Run("without connection timeout overridden", func(t *testing.T) {
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cfg, err := Load()
		assert.NotNil(t, cfg, "Could not load config")
		assert.NoError(t, err, "Load good config returned unexpected error")
		assert.Equal(t, cfg.General.ConnectionTimeout, 0*time.Second)
	})

	t.Run("with connection timeout overridden", func(t *testing.T) {
		os.Setenv("ORDERER_GENERAL_CONNECTIONTIMEOUT", "10s")
		defer os.Unsetenv("ORDERER_GENERAL_CONNECTIONTIMEOUT")
		cleanup := configtest.SetDevFabricConfigPath(t)
		defer cleanup()
		cfg, err := Load()
		assert.NotNil(t, cfg, "Could not load config")
		assert.NoError(t, err, "Load good config returned unexpected error")
		assert.Equal(t, cfg.General.ConnectionTimeout, 10*time.Second)
	})
}
