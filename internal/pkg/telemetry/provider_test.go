/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// collector wires a provider to a manual reader so a test can assert on exactly
// what would be exported.
type collector struct {
	provider *MetricsProvider
	reader   *sdkmetric.ManualReader
}

func newCollector(t *testing.T) *collector {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	return &collector{provider: newMetricsProvider(mp), reader: reader}
}

// dataPoints collects the exported points for a metric by name, flattened into
// value plus attributes so assertions stay readable.
func (c *collector) dataPoints(t *testing.T, name string) []point {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, c.reader.Collect(context.Background(), &rm))

	var points []point
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			switch agg := m.Data.(type) {
			case metricdata.Sum[float64]:
				for _, dp := range agg.DataPoints {
					points = append(points, point{value: dp.Value, attrs: dp.Attributes})
				}
			case metricdata.Gauge[float64]:
				for _, dp := range agg.DataPoints {
					points = append(points, point{value: dp.Value, attrs: dp.Attributes})
				}
			case metricdata.Histogram[float64]:
				for _, dp := range agg.DataPoints {
					points = append(points, point{
						value:  dp.Sum,
						count:  dp.Count,
						bounds: dp.Bounds,
						attrs:  dp.Attributes,
					})
				}
			}
		}
	}
	return points
}

type point struct {
	value  float64
	count  uint64
	bounds []float64
	attrs  attribute.Set
}

func TestCounterRecordsWithLabels(t *testing.T) {
	c := newCollector(t)

	counter := c.provider.NewCounter(metrics.CounterOpts{
		Namespace:  "fabric",
		Subsystem:  "endorser",
		Name:       "proposals_received",
		LabelNames: []string{"channel"},
	})
	counter.With("channel", "mychannel").Add(2)
	counter.With("channel", "mychannel").Add(3)
	counter.With("channel", "other").Add(1)

	points := c.dataPoints(t, "fabric_endorser_proposals_received")
	require.Len(t, points, 2)

	byChannel := map[string]float64{}
	for _, p := range points {
		channel, ok := p.attrs.Value("channel")
		require.True(t, ok)
		byChannel[channel.AsString()] = p.value
	}
	require.Equal(t, map[string]float64{"mychannel": 5, "other": 1}, byChannel)
}

// Fabric's Gauge supports both Set and Add, but OTLP only carries an absolute
// value, so Add has to be resolved against the previous reading.
func TestGaugeSetAndAddResolveToAbsoluteValue(t *testing.T) {
	c := newCollector(t)

	gauge := c.provider.NewGauge(metrics.GaugeOpts{
		Namespace: "fabric",
		Name:      "ledger_height",
	})
	gauge.Set(10)
	gauge.Add(5)
	gauge.Add(-2)

	points := c.dataPoints(t, "fabric_ledger_height")
	require.Len(t, points, 1)
	require.Equal(t, float64(13), points[0].value)

	// A later Set discards the accumulated value rather than adding to it.
	gauge.Set(1)
	points = c.dataPoints(t, "fabric_ledger_height")
	require.Len(t, points, 1)
	require.Equal(t, float64(1), points[0].value)
}

// Each label set is its own series, so Add on one must not disturb another.
func TestGaugeTracksLabelSetsIndependently(t *testing.T) {
	c := newCollector(t)

	gauge := c.provider.NewGauge(metrics.GaugeOpts{
		Name:       "fabric_channel_height",
		LabelNames: []string{"channel"},
	})
	gauge.With("channel", "a").Set(5)
	gauge.With("channel", "b").Set(100)
	gauge.With("channel", "a").Add(1)

	points := c.dataPoints(t, "fabric_channel_height")
	require.Len(t, points, 2)

	byChannel := map[string]float64{}
	for _, p := range points {
		channel, ok := p.attrs.Value("channel")
		require.True(t, ok)
		byChannel[channel.AsString()] = p.value
	}
	require.Equal(t, map[string]float64{"a": 6, "b": 100}, byChannel)
}

func TestHistogramUsesFabricBucketBoundaries(t *testing.T) {
	c := newCollector(t)

	buckets := []float64{0.1, 0.5, 1}
	histogram := c.provider.NewHistogram(metrics.HistogramOpts{
		Name:    "fabric_proposal_duration",
		Buckets: buckets,
	})
	histogram.Observe(0.2)
	histogram.Observe(0.7)

	points := c.dataPoints(t, "fabric_proposal_duration")
	require.Len(t, points, 1)
	require.Equal(t, uint64(2), points[0].count)
	require.InDelta(t, 0.9, points[0].value, 1e-9)
	require.Equal(t, buckets, points[0].bounds)
}

// With must derive a new metric rather than mutating the one it was called on,
// otherwise labels would leak between unrelated observations.
func TestWithDoesNotAliasParentAttributes(t *testing.T) {
	c := newCollector(t)

	counter := c.provider.NewCounter(metrics.CounterOpts{Name: "fabric_calls"})
	base := counter.With("channel", "a")
	derived := base.With("chaincode", "cc")

	base.Add(1)
	derived.Add(1)

	points := c.dataPoints(t, "fabric_calls")
	require.Len(t, points, 2)

	for _, p := range points {
		if _, hasChaincode := p.attrs.Value("chaincode"); hasChaincode {
			require.Equal(t, 2, p.attrs.Len())
			continue
		}
		// The parent must still carry only its own label.
		require.Equal(t, 1, p.attrs.Len())
	}
}

func TestFullyQualifiedName(t *testing.T) {
	require.Equal(t, "fabric_endorser_proposals", fullyQualifiedName("fabric", "endorser", "proposals"))
	require.Equal(t, "fabric_proposals", fullyQualifiedName("fabric", "", "proposals"))
	require.Equal(t, "proposals", fullyQualifiedName("", "", "proposals"))
}

// An unpaired label would otherwise be recorded with an empty value, silently
// creating a second series that looks legitimate.
func TestAppendLabelsDropsUnpairedLabel(t *testing.T) {
	attrs := appendLabels(nil, []string{"channel", "a", "dangling"})
	require.Equal(t, []attribute.KeyValue{attribute.String("channel", "a")}, attrs)
}

func TestProviderIsUsableWhenTelemetryDisabled(t *testing.T) {
	// NewMetricsProvider against the global no-op provider must still hand back
	// working metrics, because the operations subsystem builds them at startup
	// regardless of whether an OTLP endpoint was configured.
	provider := NewMetricsProvider()

	require.NotPanics(t, func() {
		provider.NewCounter(metrics.CounterOpts{Name: "c"}).With("a", "b").Add(1)
		provider.NewGauge(metrics.GaugeOpts{Name: "g"}).With("a", "b").Set(1)
		provider.NewHistogram(metrics.HistogramOpts{Name: "h"}).With("a", "b").Observe(1)
	})
}
