/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"time"

	"github.com/spf13/viper"
)

const (
	reconcileSleepIntervalDefault = time.Minute
	reconcileBatchSizeDefault     = 10
)

// PrivdataConfig is the struct that defines the Gossip Privdata configurations.
type PrivdataConfig struct {
	// ReconcileSleepInterval determines the time reconciler sleeps from end of an interation until the beginning of the next
	// reconciliation iteration.
	ReconcileSleepInterval time.Duration
	// ReconcileBatchSize determines the maximum batch size of missing private data that will be reconciled in a single iteration.
	ReconcileBatchSize int
	// ReconciliationEnabled is a flag that indicates whether private data reconciliation is enabled or not.
	ReconciliationEnabled bool
}

// GlobalConfig obtains a set of configuration from viper, build and returns the config struct.
func GlobalConfig() *PrivdataConfig {
	c := &PrivdataConfig{}
	c.loadPrivDataConfig()
	return c
}

func (c *PrivdataConfig) loadPrivDataConfig() {
	c.ReconcileSleepInterval = viper.GetDuration("peer.gossip.pvtData.reconcileSleepInterval")
	if c.ReconcileSleepInterval == 0 {
		logger.Warning("Configuration key peer.gossip.pvtData.reconcileSleepInterval isn't set, defaulting to", reconcileSleepIntervalDefault)
		c.ReconcileSleepInterval = reconcileSleepIntervalDefault
	}

	c.ReconcileBatchSize = viper.GetInt("peer.gossip.pvtData.reconcileBatchSize")
	if c.ReconcileBatchSize == 0 {
		logger.Warning("Configuration key peer.gossip.pvtData.reconcileBatchSize isn't set, defaulting to", reconcileBatchSizeDefault)
		c.ReconcileBatchSize = reconcileBatchSizeDefault
	}

	c.ReconciliationEnabled = viper.GetBool("peer.gossip.pvtData.reconciliationEnabled")

}
