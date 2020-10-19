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
		Help:         "The channel participation status of the node: 0 if inactive, 1 if active, 2 if onboarding.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	relationOpts = metrics.GaugeOpts{
		Namespace:    "participation",
		Subsystem:    "",
		Name:         "cluster_relation",
		Help:         "The channel participation cluster relation of the node: 0 if none, 1 if consenter, 2 if follower, 3 if config-tracker.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}
)

// Metrics defines the metrics for the cluster.
type Metrics struct {
	Status   metrics.Gauge
	Relation metrics.Gauge
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

// NewMetrics initializes new metrics for the cluster infrastructure.
func NewMetrics(m MetricsProvider) *Metrics {
	return &Metrics{
		Status:   m.NewGauge(statusOpts),
		Relation: m.NewGauge(relationOpts),
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
	default:
		logger.Panicf("Programming error: unexpected status %s", status)
	}
	m.Status.With("channel", channel).Set(float64(s))
}

func (m *Metrics) reportRelation(channel string, relation types.ClusterRelation) {
	var r int
	switch relation {
	case types.ClusterRelationNone:
		r = 0
	case types.ClusterRelationConsenter:
		r = 1
	case types.ClusterRelationFollower:
		r = 2
	case types.ClusterRelationConfigTracker:
		r = 3
	default:
		logger.Panicf("Programming error: unexpected relation %s", relation)

	}
	m.Relation.With("channel", channel).Set(float64(r))
}
