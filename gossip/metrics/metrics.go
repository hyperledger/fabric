/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import "github.com/hyperledger/fabric/common/metrics"

// GossipMetrics encapsulates all of gossip metrics
type GossipMetrics struct {
	StateMetrics    *StateMetrics
	ElectionMetrics *ElectionMetrics
}

func NewGossipMetrics(p metrics.Provider) *GossipMetrics {
	return &GossipMetrics{
		StateMetrics:    newStateMetrics(p),
		ElectionMetrics: newElectionMetrics(p),
	}
}

// StateMetrics encapsulates gossip state related metrics
type StateMetrics struct {
	Height            metrics.Gauge
	CommitDuration    metrics.Histogram
	PayloadBufferSize metrics.Gauge
}

func newStateMetrics(p metrics.Provider) *StateMetrics {
	return &StateMetrics{
		Height:            p.NewGauge(HeightOpts),
		CommitDuration:    p.NewHistogram(CommitDurationOpts),
		PayloadBufferSize: p.NewGauge(PayloadBufferSizeOpts),
	}
}

var (
	HeightOpts = metrics.GaugeOpts{
		Namespace:    "gossip",
		Subsystem:    "state",
		Name:         "height",
		Help:         "Current ledger height",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	CommitDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "state",
		Name:         "commit_duration",
		Help:         "Time it takes to commit a block in seconds",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	PayloadBufferSizeOpts = metrics.GaugeOpts{
		Namespace:    "gossip",
		Subsystem:    "payload_buffer",
		Name:         "size",
		Help:         "Size of the payload buffer",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)

// ElectionMetrics encapsulates gossip leader election related metrics
type ElectionMetrics struct {
	Declaration metrics.Gauge
}

func newElectionMetrics(p metrics.Provider) *ElectionMetrics {
	return &ElectionMetrics{
		Declaration: p.NewGauge(LeaderDeclerationOpts),
	}
}

var (
	LeaderDeclerationOpts = metrics.GaugeOpts{
		Namespace:    "gossip",
		Subsystem:    "leader_election",
		Name:         "leader",
		Help:         "Peer is leader (1) or follower (0)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)
