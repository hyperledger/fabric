/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import "github.com/hyperledger/fabric/common/metrics"

// GossipMetrics encapsulates all of gossip metrics
type GossipMetrics struct {
	StateMetrics      *StateMetrics
	ElectionMetrics   *ElectionMetrics
	CommMetrics       *CommMetrics
	MembershipMetrics *MembershipMetrics
}

func NewGossipMetrics(p metrics.Provider) *GossipMetrics {
	return &GossipMetrics{
		StateMetrics:      newStateMetrics(p),
		ElectionMetrics:   newElectionMetrics(p),
		CommMetrics:       newCommMetrics(p),
		MembershipMetrics: newMembershipMetrics(p),
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

// CommMetrics encapsulates gossip communication related metrics
type CommMetrics struct {
	SentMessages     metrics.Counter
	BufferOverflow   metrics.Counter
	ReceivedMessages metrics.Counter
}

func newCommMetrics(p metrics.Provider) *CommMetrics {
	return &CommMetrics{
		SentMessages:     p.NewCounter(SentMessagesOpts),
		BufferOverflow:   p.NewCounter(BufferOverflowOpts),
		ReceivedMessages: p.NewCounter(ReceivedMessagesOpts),
	}
}

var (
	SentMessagesOpts = metrics.CounterOpts{
		Namespace:    "gossip",
		Subsystem:    "comm",
		Name:         "messages_sent",
		Help:         "Number of messages sent",
		StatsdFormat: "%{#fqname}",
	}

	BufferOverflowOpts = metrics.CounterOpts{
		Namespace:    "gossip",
		Subsystem:    "comm",
		Name:         "overflow_count",
		Help:         "Number of outgoing queue buffer overflows",
		StatsdFormat: "%{#fqname}",
	}

	ReceivedMessagesOpts = metrics.CounterOpts{
		Namespace:    "gossip",
		Subsystem:    "comm",
		Name:         "messages_received",
		Help:         "Number of messages received",
		StatsdFormat: "%{#fqname}",
	}
)

// MembershipMetrics encapsulates gossip channel membership related metrics
type MembershipMetrics struct {
	Total metrics.Gauge
}

func newMembershipMetrics(p metrics.Provider) *MembershipMetrics {
	return &MembershipMetrics{
		Total: p.NewGauge(TotalOpts),
	}
}

var (
	TotalOpts = metrics.GaugeOpts{
		Namespace:    "gossip",
		Subsystem:    "membership",
		Name:         "total_peers_known",
		Help:         "Total known peers",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)
