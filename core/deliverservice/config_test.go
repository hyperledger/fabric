/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/spf13/viper"
)

func TestSecureOptsConfig(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	certPath := filepath.Join(cwd, "testdata", "cert.pem")
	keyPath := filepath.Join(cwd, "testdata", "key.pem")

	certBytes, err := ioutil.ReadFile(filepath.Join("testdata", "cert.pem"))
	require.NoError(t, err)

	keyBytes, err := ioutil.ReadFile(filepath.Join("testdata", "key.pem"))
	require.NoError(t, err)

	t.Run("client specific cert", func(t *testing.T) {
		viper.Reset()
		defer viper.Reset()

		viper.Set("peer.tls.enabled", true)
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.clientCert.file", certPath)
		viper.Set("peer.tls.clientKey.file", keyPath)

		coreConfig := deliverservice.GlobalConfig()

		assert.True(t, coreConfig.SecOpts.UseTLS)
		assert.True(t, coreConfig.SecOpts.RequireClientCert)
		assert.Equal(t, keyBytes, coreConfig.SecOpts.Key)
		assert.Equal(t, certBytes, coreConfig.SecOpts.Certificate)
	})

	t.Run("fallback cert", func(t *testing.T) {
		viper.Reset()
		defer viper.Reset()

		viper.Set("peer.tls.enabled", true)
		viper.Set("peer.tls.clientAuthRequired", true)
		viper.Set("peer.tls.cert.file", certPath)
		viper.Set("peer.tls.key.file", keyPath)

		coreConfig := deliverservice.GlobalConfig()

		assert.True(t, coreConfig.SecOpts.UseTLS)
		assert.True(t, coreConfig.SecOpts.RequireClientCert)
		assert.Equal(t, keyBytes, coreConfig.SecOpts.Key)
		assert.Equal(t, certBytes, coreConfig.SecOpts.Certificate)
	})

	t.Run("no cert", func(t *testing.T) {
		viper.Reset()
		defer viper.Reset()

		viper.Set("peer.tls.enabled", true)
		viper.Set("peer.tls.clientAuthRequired", true)

		assert.Panics(t, func() { deliverservice.GlobalConfig() })
	})
}

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.deliveryclient.reConnectBackoffThreshold", 25.5)
	viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", "20s")
	viper.Set("peer.deliveryclient.connTimeout", "10s")
	viper.Set("peer.keepalive.deliveryClient.interval", "5s")
	viper.Set("peer.keepalive.deliveryClient.timeout", "2s")

	coreConfig := deliverservice.GlobalConfig()

	expectedConfig := &deliverservice.DeliverServiceConfig{
		PeerTLSEnabled:              true,
		ReConnectBackoffThreshold:   25.5,
		ReconnectTotalTimeThreshold: 20 * time.Second,
		ConnectionTimeout:           10 * time.Second,
		KeepaliveOptions: comm.KeepaliveOptions{
			ClientInterval:    time.Second * 5,
			ClientTimeout:     time.Second * 2,
			ServerInterval:    time.Hour * 2,
			ServerTimeout:     time.Second * 20,
			ServerMinInterval: time.Minute,
		},
		SecOpts: comm.SecureOptions{
			UseTLS: true,
		},
	}

	assert.Equal(t, expectedConfig, coreConfig)
}

func TestGlobalConfigDefault(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	coreConfig := deliverservice.GlobalConfig()

	expectedConfig := &deliverservice.DeliverServiceConfig{
		PeerTLSEnabled:              false,
		ReConnectBackoffThreshold:   deliverservice.DefaultReConnectBackoffThreshold,
		ReconnectTotalTimeThreshold: deliverservice.DefaultReConnectTotalTimeThreshold,
		ConnectionTimeout:           deliverservice.DefaultConnectionTimeout,
		KeepaliveOptions:            comm.DefaultKeepaliveOptions,
	}

	assert.Equal(t, expectedConfig, coreConfig)
}
