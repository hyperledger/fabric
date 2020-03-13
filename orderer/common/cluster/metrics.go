/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster

import (
	"time"

	"github.com/hyperledger/fabric/common/metrics"
)

var (
	EgressQueueLengthOpts = metrics.GaugeOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "egress_queue_length",
		Help:         "Length of the egress queue.",
		LabelNames:   []string{"host", "msg_type", "channel"},
		StatsdFormat: "%{#fqname}.%{host}.%{msg_type}.%{channel}",
	}

	EgressQueueCapacityOpts = metrics.GaugeOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "egress_queue_capacity",
		Help:         "Capacity of the egress queue.",
		LabelNames:   []string{"host", "msg_type", "channel"},
		StatsdFormat: "%{#fqname}.%{host}.%{msg_type}.%{channel}",
	}

	EgressWorkersOpts = metrics.GaugeOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "egress_queue_workers",
		Help:         "Count of egress queue workers.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	IngressStreamsCountOpts = metrics.GaugeOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "ingress_stream_count",
		Help:         "Count of streams from other nodes.",
		StatsdFormat: "%{#fqname}",
	}

	EgressStreamsCountOpts = metrics.GaugeOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "egress_stream_count",
		Help:         "Count of streams to other nodes.",
		LabelNames:   []string{"channel"},
		StatsdFormat: "%{#fqname}.%{channel}",
	}

	EgressTLSConnectionCountOpts = metrics.GaugeOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "egress_tls_connection_count",
		Help:         "Count of TLS connections to other nodes.",
		StatsdFormat: "%{#fqname}",
	}

	MessageSendTimeOpts = metrics.HistogramOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "msg_send_time",
		Help:         "The time it takes to send a message in seconds.",
		LabelNames:   []string{"host", "channel"},
		StatsdFormat: "%{#fqname}.%{host}.%{channel}",
	}

	MessagesDroppedCountOpts = metrics.CounterOpts{
		Namespace:    "cluster",
		Subsystem:    "comm",
		Name:         "msg_dropped_count",
		Help:         "Count of messages dropped.",
		LabelNames:   []string{"host", "channel"},
		StatsdFormat: "%{#fqname}.%{host}.%{channel}",
	}
)

// Metrics defines the metrics for the cluster.
type Metrics struct {
	EgressQueueLength        metrics.Gauge
	EgressQueueCapacity      metrics.Gauge
	EgressWorkerCount        metrics.Gauge
	IngressStreamsCount      metrics.Gauge
	EgressStreamsCount       metrics.Gauge
	EgressTLSConnectionCount metrics.Gauge
	MessageSendTime          metrics.Histogram
	MessagesDroppedCount     metrics.Counter
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
func NewMetrics(provider MetricsProvider) *Metrics {
	return &Metrics{
		EgressQueueLength:        provider.NewGauge(EgressQueueLengthOpts),
		EgressQueueCapacity:      provider.NewGauge(EgressQueueCapacityOpts),
		EgressStreamsCount:       provider.NewGauge(EgressStreamsCountOpts),
		EgressTLSConnectionCount: provider.NewGauge(EgressTLSConnectionCountOpts),
		EgressWorkerCount:        provider.NewGauge(EgressWorkersOpts),
		IngressStreamsCount:      provider.NewGauge(IngressStreamsCountOpts),
		MessagesDroppedCount:     provider.NewCounter(MessagesDroppedCountOpts),
		MessageSendTime:          provider.NewHistogram(MessageSendTimeOpts),
	}
}

func (m *Metrics) reportMessagesDropped(host, channel string) {
	m.MessagesDroppedCount.With("host", host, "channel", channel).Add(1)
}

func (m *Metrics) reportQueueOccupancy(host string, msgType string, channel string, length, capacity int) {
	m.EgressQueueLength.With("host", host, "msg_type", msgType, "channel", channel).Set(float64(length))
	m.EgressQueueCapacity.With("host", host, "msg_type", msgType, "channel", channel).Set(float64(capacity))
}

func (m *Metrics) reportWorkerCount(channel string, count uint32) {
	m.EgressWorkerCount.With("channel", channel).Set(float64(count))
}

func (m *Metrics) reportMsgSendTime(host string, channel string, duration time.Duration) {
	m.MessageSendTime.With("host", host, "channel", channel).Observe(float64(duration.Seconds()))
}

func (m *Metrics) reportEgressStreamCount(channel string, count uint32) {
	m.EgressStreamsCount.With("channel", channel).Set(float64(count))
}

func (m *Metrics) reportStreamCount(count uint32) {
	m.IngressStreamsCount.Set(float64(count))
}
