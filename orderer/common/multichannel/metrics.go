/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/orderer/common/types"
)

var (
	statusOpts = metrics.GaugeOpts{
		Namespace:    "participation",
		Subsystem:    "",
		Name:         "status",
		Help:         "The channel participation status of the node: 0 if inactive, 1 if active, 2 if onboarding, 3 if failed.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	consensusRelationOpts = metrics.GaugeOpts{
		Namespace:    "participation",
		Subsystem:    "",
		Name:         "consensus_relation",
		Help:         "The channel participation consensus relation of the node: 0 if other, 1 if consenter, 2 if follower, 3 if config-tracker.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)

// Metrics defines the metrics for the cluster.
type Metrics struct {
	Status            metrics.Gauge
	ConsensusRelation metrics.Gauge
}

// A MetricsProvider is an abstraction for a metrics provider. It is a factory for
// Counter, Gauge, and Histogram meters.
type MetricsProvider interface {
	// NewCounter creates a new instance of a Counter.
	NewCounter(opts metrics.CounterOpts) metrics.Counter
	// NewGauge creates a new instance of a Gauge.
	NewGauge(opts metrics.GaugeOpts) metrics.Gauge
	// NewHistogram creates a new instance of a Histogram.
	NewHistogram(opts metrics.HistogramOpts) metrics.Histogram
}

//go:generate mockery -dir . -name MetricsProvider -case underscore -output ./mocks/

// NewMetrics initializes new metrics for the channel participation API.
func NewMetrics(m MetricsProvider) *Metrics {
	return &Metrics{
		Status:            m.NewGauge(statusOpts),
		ConsensusRelation: m.NewGauge(consensusRelationOpts),
	}
}

func (m *Metrics) reportStatus(channel string, status types.Status) {
	var s int
	switch status {
	case types.StatusInactive:
		s = 0
	case types.StatusActive:
		s = 1
	case types.StatusOnBoarding:
		s = 2
	case types.StatusFailed:
		s = 3
	default:
		logger.Panicf("Programming error: unexpected status %s", status)
	}
	m.Status.With("channel", channel).Set(float64(s))
}

func (m *Metrics) reportConsensusRelation(channel string, relation types.ConsensusRelation) {
	var r int
	switch relation {
	case types.ConsensusRelationOther:
		r = 0
	case types.ConsensusRelationConsenter:
		r = 1
	case types.ConsensusRelationFollower:
		r = 2
	case types.ConsensusRelationConfigTracker:
		r = 3
	default:
		logger.Panicf("Programming error: unexpected relation %s", relation)

	}
	m.ConsensusRelation.With("channel", channel).Set(float64(r))
}
