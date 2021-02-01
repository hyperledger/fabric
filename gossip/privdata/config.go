/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

const (
	reconcileSleepIntervalDefault         = time.Minute
	reconcileBatchSizeDefault             = 10
	implicitCollectionMaxPeerCountDefault = 1
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
	// ImplicitCollectionDisseminationPolicy specifies the dissemination  policy for the peer's own implicit collection.
	ImplicitCollDisseminationPolicy ImplicitCollectionDisseminationPolicy
}

// ImplicitCollectionDisseminationPolicy specifies the dissemination  policy for the peer's own implicit collection.
// It is not applicable to private data for other organizations' implicit collections.
type ImplicitCollectionDisseminationPolicy struct {
	// RequiredPeerCount defines the minimum number of eligible peers to which each endorsing peer must successfully
	// disseminate private data for its own implicit collection. Default is 0.
	RequiredPeerCount int
	// MaxPeerCount defines the maximum number of eligible peers to which each endorsing peer will attempt to
	// disseminate private data for its own implicit collection. Default is 1.
	MaxPeerCount int
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

	requiredPeerCount := viper.GetInt("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount")

	maxPeerCount := implicitCollectionMaxPeerCountDefault
	if viper.Get("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.maxPeerCount") != nil {
		// allow override maxPeerCount to 0 that will effectively disable dissemination on the peer
		maxPeerCount = viper.GetInt("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.maxPeerCount")
	}

	if requiredPeerCount < 0 {
		panic(fmt.Sprintf("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.requiredPeerCount (%d) cannot be less than zero",
			requiredPeerCount))
	}
	if maxPeerCount < requiredPeerCount {
		panic(fmt.Sprintf("peer.gossip.pvtData.implicitCollectionDisseminationPolicy.maxPeerCount (%d) cannot be less than requiredPeerCount (%d)",
			maxPeerCount, requiredPeerCount))
	}

	c.ImplicitCollDisseminationPolicy.RequiredPeerCount = requiredPeerCount
	c.ImplicitCollDisseminationPolicy.MaxPeerCount = maxPeerCount
}
