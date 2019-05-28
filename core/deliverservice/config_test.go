/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/core/deliverservice"
	"github.com/spf13/viper"
)

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("peer.tls.enabled", true)
	viper.Set("peer.deliveryclient.reConnectBackoffThreshold", 25.5)
	viper.Set("peer.deliveryclient.reconnectTotalTimeThreshold", "20s")
	viper.Set("peer.deliveryclient.connTimeout", "10s")

	coreConfig := deliverservice.GlobalConfig()

	expectedConfig := &deliverservice.DeliverServiceConfig{
		PeerTLSEnabled:              true,
		ReConnectBackoffThreshold:   25.5,
		ReconnectTotalTimeThreshold: 20 * time.Second,
		ConnectionTimeout:           10 * time.Second,
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
	}

	assert.Equal(t, expectedConfig, coreConfig)
}
