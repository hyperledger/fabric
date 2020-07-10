/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata_test

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/privdata"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	// Capture the configuration from viper
	viper.Set("peer.gossip.pvtData.reconcileSleepInterval", "10s")
	viper.Set("peer.gossip.pvtData.reconcileBatchSize", 10)
	viper.Set("peer.gossip.pvtData.reconciliationEnabled", true)
	viper.Set("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount", 2)
	viper.Set("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.maxPeerCount", 3)

	coreConfig := privdata.GlobalConfig()

	expectedConfig := &privdata.PrivdataConfig{
		ReconcileSleepInterval: 10 * time.Second,
		ReconcileBatchSize:     10,
		ReconciliationEnabled:  true,
		ImplicitCollDisseminationPolicy: privdata.ImplicitCollectionDisseminationPolicy{
			RequiredPeerCount: 2,
			MaxPeerCount:      3,
		},
	}

	require.Equal(t, coreConfig, expectedConfig)
}

func TestGlobalConfigDefaults(t *testing.T) {
	viper.Reset()

	coreConfig := privdata.GlobalConfig()

	expectedConfig := &privdata.PrivdataConfig{
		ReconcileSleepInterval: time.Minute,
		ReconcileBatchSize:     10,
		ReconciliationEnabled:  false,
		ImplicitCollDisseminationPolicy: privdata.ImplicitCollectionDisseminationPolicy{
			RequiredPeerCount: 0,
			MaxPeerCount:      1,
		},
	}

	require.Equal(t, coreConfig, expectedConfig)
}

func TestGlobalConfigPanic(t *testing.T) {
	viper.Reset()
	// Capture the configuration from viper
	viper.Set("peer.gossip.pvtData.reconcileSleepInterval", "10s")
	viper.Set("peer.gossip.pvtData.reconcileBatchSize", 10)
	viper.Set("peer.gossip.pvtData.reconciliationEnabled", true)
	viper.Set("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount", 2)
	viper.Set("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.maxPeerCount", 1)
	require.PanicsWithValue(
		t,
		"peer.gossip.pvtData.implicitCollectionDisseminationPolicy.maxPeerCount (1) cannot be less than requiredPeerCount (2)",
		func() { privdata.GlobalConfig() },
		"A panic should occur because maxPeerCount is less than requiredPeerCount",
	)

	viper.Set("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount", -1)
	require.PanicsWithValue(
		t,
		"peer.gossip.pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount (-1) cannot be less than zero",
		func() { privdata.GlobalConfig() },
		"A panic should occur because requiredPeerCount is less than zero",
	)
}
