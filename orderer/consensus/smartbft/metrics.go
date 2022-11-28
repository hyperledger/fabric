/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import "github.com/hyperledger/fabric/common/metrics"

var (
	clusterSizeOpts = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "cluster_size",
		Help:         "Number of nodes in this channel.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
	committedBlockNumberOpts = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "committed_block_number",
		Help:         "The number of the latest committed block.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
	isLeaderOpts = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "is_leader",
		Help:         "The leadership status of the current node according to the latest committed block: 1 if it is the leader else 0.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
	leaderIDOpts = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "leader_id",
		Help:         "The id of the current leader according to the latest committed block.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)

// Metrics defines the metrics for the cluster.
type Metrics struct {
	ClusterSize          metrics.Gauge
	CommittedBlockNumber metrics.Gauge
	IsLeader             metrics.Gauge
	LeaderID             metrics.Gauge
}

// NewMetrics creates the Metrics
func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		ClusterSize:          p.NewGauge(clusterSizeOpts),
		CommittedBlockNumber: p.NewGauge(committedBlockNumberOpts),
		IsLeader:             p.NewGauge(isLeaderOpts),
		LeaderID:             p.NewGauge(leaderIDOpts),
	}
}
