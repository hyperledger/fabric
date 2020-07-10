/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state_test

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/state"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	viper.Set("peer.gossip.state.checkInterval", "1s")
	viper.Set("peer.gossip.state.responseTimeout", "2s")
	viper.Set("peer.gossip.state.batchSize", 3)
	viper.Set("peer.gossip.state.maxRetries", 4)
	viper.Set("peer.gossip.state.blockBufferSize", 5)
	viper.Set("peer.gossip.state.channelSize", 6)
	viper.Set("peer.gossip.state.enabled", true)

	coreConfig := state.GlobalConfig()

	expectedConfig := &state.StateConfig{
		StateCheckInterval:   time.Second,
		StateResponseTimeout: 2 * time.Second,
		StateBatchSize:       uint64(3),
		StateMaxRetries:      4,
		StateBlockBufferSize: 5,
		StateChannelSize:     6,
		StateEnabled:         true,
	}

	require.Equal(t, expectedConfig, coreConfig)
}

func TestGlobalConfigDefaults(t *testing.T) {
	viper.Reset()

	coreConfig := state.GlobalConfig()

	expectedConfig := &state.StateConfig{
		StateCheckInterval:   10 * time.Second,
		StateResponseTimeout: 3 * time.Second,
		StateBatchSize:       uint64(10),
		StateMaxRetries:      3,
		StateBlockBufferSize: 20,
		StateChannelSize:     100,
		StateEnabled:         false,
	}

	require.Equal(t, expectedConfig, coreConfig)
}
