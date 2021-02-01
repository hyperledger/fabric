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
	PrivdataMetrics   *PrivdataMetrics
}

func NewGossipMetrics(p metrics.Provider) *GossipMetrics {
	return &GossipMetrics{
		StateMetrics:      newStateMetrics(p),
		ElectionMetrics:   newElectionMetrics(p),
		CommMetrics:       newCommMetrics(p),
		MembershipMetrics: newMembershipMetrics(p),
		PrivdataMetrics:   newPrivdataMetrics(p),
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

var LeaderDeclerationOpts = metrics.GaugeOpts{
	Namespace:    "gossip",
	Subsystem:    "leader_election",
	Name:         "leader",
	Help:         "Peer is leader (1) or follower (0)",
	LabelNames:   []string{"channel"},
	StatsdFormat: "%{#fqname}.%{channel}",
}

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

var TotalOpts = metrics.GaugeOpts{
	Namespace:    "gossip",
	Subsystem:    "membership",
	Name:         "total_peers_known",
	Help:         "Total known peers",
	LabelNames:   []string{"channel"},
	StatsdFormat: "%{#fqname}.%{channel}",
}

// PrivdataMetrics encapsulates gossip private data related metrics
type PrivdataMetrics struct {
	ValidationDuration             metrics.Histogram
	ListMissingPrivateDataDuration metrics.Histogram
	FetchDuration                  metrics.Histogram
	CommitPrivateDataDuration      metrics.Histogram
	PurgeDuration                  metrics.Histogram
	SendDuration                   metrics.Histogram
	ReconciliationDuration         metrics.Histogram
	PullDuration                   metrics.Histogram
	RetrieveDuration               metrics.Histogram
}

func newPrivdataMetrics(p metrics.Provider) *PrivdataMetrics {
	return &PrivdataMetrics{
		ValidationDuration:             p.NewHistogram(ValidationDurationOpts),
		ListMissingPrivateDataDuration: p.NewHistogram(ListMissingPrivateDataDurationOpts),
		FetchDuration:                  p.NewHistogram(FetchDurationOpts),
		CommitPrivateDataDuration:      p.NewHistogram(CommitPrivateDataDurationOpts),
		PurgeDuration:                  p.NewHistogram(PurgeDurationOpts),
		SendDuration:                   p.NewHistogram(SendDurationOpts),
		ReconciliationDuration:         p.NewHistogram(ReconciliationDurationOpts),
		PullDuration:                   p.NewHistogram(PullDurationOpts),
		RetrieveDuration:               p.NewHistogram(RetrieveDurationOpts),
	}
}

var (
	ValidationDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "validation_duration",
		Help:         "Time it takes to validate a block (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	ListMissingPrivateDataDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "list_missing_duration",
		Help:         "Time it takes to list the missing private data (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	FetchDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "fetch_duration",
		Help:         "Time it takes to fetch missing private data from peers (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	CommitPrivateDataDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "commit_block_duration",
		Help:         "Time it takes to commit private data and the corresponding block (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	PurgeDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "purge_duration",
		Help:         "Time it takes to purge private data (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	SendDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "send_duration",
		Help:         "Time it takes to send a missing private data element (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	ReconciliationDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "reconciliation_duration",
		Help:         "Time it takes for reconciliation to complete (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	PullDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "pull_duration",
		Help:         "Time it takes to pull a missing private data element (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	RetrieveDurationOpts = metrics.HistogramOpts{
		Namespace:    "gossip",
		Subsystem:    "privdata",
		Name:         "retrieve_duration",
		Help:         "Time it takes to retrieve missing private data elements from the ledger (in seconds)",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)
