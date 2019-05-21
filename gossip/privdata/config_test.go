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
	"github.com/stretchr/testify/assert"
)

func TestGlobalConfig(t *testing.T) {
	viper.Reset()
	// Capture the configuration from viper
	viper.Set("peer.gossip.pvtData.reconcileSleepInterval", "10s")
	viper.Set("peer.gossip.pvtData.reconcileBatchSize", 10)
	viper.Set("peer.gossip.pvtData.reconciliationEnabled", true)

	coreConfig := privdata.GlobalConfig()

	expectedConfig := &privdata.PrivdataConfig{
		ReconcileSleepInterval: 10 * time.Second,
		ReconcileBatchSize:     10,
		ReconciliationEnabled:  true,
	}

	assert.Equal(t, coreConfig, expectedConfig)
}

func TestGlobalConfigDefaults(t *testing.T) {
	viper.Reset()

	coreConfig := privdata.GlobalConfig()

	expectedConfig := &privdata.PrivdataConfig{
		ReconcileSleepInterval: time.Minute,
		ReconcileBatchSize:     10,
		ReconciliationEnabled:  false,
	}

	assert.Equal(t, coreConfig, expectedConfig)
}
