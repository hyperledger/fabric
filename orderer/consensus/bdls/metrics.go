/*
Copyright LFX-CLP-2023 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bdls

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
	proposalFailuresOpts = metrics.CounterOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "proposal_failures",
		Help:         "The number of proposal failures.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
	ActiveNodesOpts = metrics.GaugeOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "active_nodes",
		Help:         "Number of active nodes in this channel.",
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
	normalProposalsReceivedOpts = metrics.CounterOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "normal_proposals_received",
		Help:         "The total number of proposals received for normal type transactions.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
	configProposalsReceivedOpts = metrics.CounterOpts{
		Namespace:    "consensus",
		Subsystem:    "BFT",
		Name:         "config_proposals_received",
		Help:         "The total number of proposals received for config type transactions.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)

// Metrics defines the metrics for the cluster.
type Metrics struct {
	ClusterSize             metrics.Gauge
	CommittedBlockNumber    metrics.Gauge
	ActiveNodes             metrics.Gauge
	IsLeader                metrics.Gauge
	LeaderID                metrics.Gauge
	ProposalFailures        metrics.Counter
	NormalProposalsReceived metrics.Counter
	ConfigProposalsReceived metrics.Counter
}

// NewMetrics creates the Metrics
func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		ClusterSize:             p.NewGauge(clusterSizeOpts),
		CommittedBlockNumber:    p.NewGauge(committedBlockNumberOpts),
		ActiveNodes:             p.NewGauge(ActiveNodesOpts),
		ProposalFailures:        p.NewCounter(proposalFailuresOpts),
		IsLeader:                p.NewGauge(isLeaderOpts),
		LeaderID:                p.NewGauge(leaderIDOpts),
		NormalProposalsReceived: p.NewCounter(normalProposalsReceivedOpts),
		ConfigProposalsReceived: p.NewCounter(configProposalsReceivedOpts),
	}
}
