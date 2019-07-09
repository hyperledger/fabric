/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deliverservice_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/core/comm"
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
