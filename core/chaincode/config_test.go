/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/spf13/viper"
)

func TestGlobalConfig(t *testing.T) {
	cleanup := capture()
	defer cleanup()

	viper.Set("peer.networkId", "test-peer-network-id")
	viper.Set("peer.id", "test-peer-id")
	viper.Set("peer.tls.enabled", "true")
	viper.Set("chaincode.keepalive", "50")
	viper.Set("chaincode.executetimeout", "20h")
	viper.Set("chaincode.startuptimeout", "30h")
	viper.Set("chaincode.logging.format", "test-chaincode-logging-format")
	viper.Set("chaincode.logging.level", "WARNING")
	viper.Set("chaincode.logging.shim", "WARNING")

	config := chaincode.GlobalConfig()
	assert.Equal(t, "test-peer-network-id", config.PeerNetworkID)
	assert.Equal(t, "test-peer-id", config.PeerID)
	assert.Equal(t, true, config.TLSEnabled)
	assert.Equal(t, 50*time.Second, config.Keepalive)
	assert.Equal(t, 20*time.Hour, config.ExecuteTimeout)
	assert.Equal(t, 30*time.Hour, config.StartupTimeout)
	assert.Equal(t, "test-chaincode-logging-format", config.LogFormat)
	assert.Equal(t, "WARNING", config.LogLevel)
	assert.Equal(t, "WARNING", config.ShimLogLevel)
}

func TestGlobalConfigInvalidKeepalive(t *testing.T) {
	cleanup := capture()
	defer cleanup()

	viper.Set("chaincode.keepalive", "abc")

	config := chaincode.GlobalConfig()
	assert.Equal(t, time.Duration(0), config.Keepalive)
}

func TestGlobalConfigExecuteTimeoutTooLow(t *testing.T) {
	cleanup := capture()
	defer cleanup()

	viper.Set("chaincode.executetimeout", "15")

	config := chaincode.GlobalConfig()
	assert.Equal(t, 30*time.Second, config.ExecuteTimeout)
}

func TestGlobalConfigStartupTimeoutTooLow(t *testing.T) {
	cleanup := capture()
	defer cleanup()

	viper.Set("chaincode.startuptimeout", "15")

	config := chaincode.GlobalConfig()
	assert.Equal(t, 5*time.Second, config.StartupTimeout)
}

func TestGlobalConfigInvalidLogLevel(t *testing.T) {
	cleanup := capture()
	defer cleanup()

	viper.Set("chaincode.logging.level", "foo")
	viper.Set("chaincode.logging.shim", "bar")

	config := chaincode.GlobalConfig()
	assert.Equal(t, "INFO", config.LogLevel)
	assert.Equal(t, "INFO", config.ShimLogLevel)
}

func TestIsDevMode(t *testing.T) {
	cleanup := capture()
	defer cleanup()

	viper.Set("chaincode.mode", chaincode.DevModeUserRunsChaincode)
	assert.True(t, chaincode.IsDevMode())

	viper.Set("chaincode.mode", "empty")
	assert.False(t, chaincode.IsDevMode())
	viper.Set("chaincode.mode", "nonsense")
	assert.False(t, chaincode.IsDevMode())
}

func capture() (restore func()) {
	viper.SetEnvPrefix("CORE")
	viper.AutomaticEnv()
	config := map[string]string{
		"peer.networkId":           viper.GetString("peer.networkId"),
		"peer.id":                  viper.GetString("peer.id"),
		"peer.tls.enabled":         viper.GetString("peer.tls.enabled"),
		"chaincode.keepalive":      viper.GetString("chaincode.keepalive"),
		"chaincode.executetimeout": viper.GetString("chaincode.executetimeout"),
		"chaincode.startuptimeout": viper.GetString("chaincode.startuptimeout"),
		"chaincode.logging.format": viper.GetString("chaincode.logging.format"),
		"chaincode.logging.level":  viper.GetString("chaincode.logging.level"),
		"chaincode.logging.shim":   viper.GetString("chaincode.logging.shim"),
	}

	return func() {
		for k, val := range config {
			viper.Set(k, val)
		}
	}
}
