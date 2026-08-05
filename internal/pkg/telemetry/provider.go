/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"
	"strings"
	"sync"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// meterName scopes every Fabric metric to a single instrumentation scope, which
// is how a backend can tell these apart from metrics emitted by the gRPC
// instrumentation in the same process.
const meterName = "github.com/hyperledger/fabric"

// MetricsProvider implements Fabric's metrics.Provider on top of OpenTelemetry.
//
// Fabric already defines well over a hundred metrics — endorsement duration,
// block commit time, validation outcomes, gossip, Raft — and every one of them
// is created through metrics.Provider. Implementing that interface therefore
// exports all of them over OTLP without adding a single new instrumentation call
// site, and without a parallel set of metric definitions that could drift from
// the ones the Prometheus endpoint serves.
//
// The trade-off to be aware of is cardinality. The Prometheus provider is
// scraped, so an unbounded label set costs memory on this process; the OTLP
// provider pushes, so it also costs ingest on the collector. Fabric's own labels
// are bounded (channel, chaincode, status, validation code), but anything added
// downstream should be checked against that.
type MetricsProvider struct {
	meter otelmetric.Meter

	// fallback serves metrics whose OTEL instrument could not be created, so a
	// single bad metric definition degrades to a no-op instead of crashing a
	// peer at startup or panicking on first use.
	fallback disabled.Provider
}

// NewMetricsProvider returns a metrics.Provider that records to the global
// MeterProvider installed by Initialize. When telemetry is disabled the global
// provider is a no-op, so this records nothing.
func NewMetricsProvider() *MetricsProvider {
	return newMetricsProvider(otel.GetMeterProvider())
}

// newMetricsProvider builds a provider against a specific MeterProvider, so
// tests can collect what was recorded without reaching through global state.
func newMetricsProvider(mp otelmetric.MeterProvider) *MetricsProvider {
	return &MetricsProvider{meter: mp.Meter(meterName)}
}

func (p *MetricsProvider) NewCounter(o metrics.CounterOpts) metrics.Counter {
	inst, err := p.meter.Float64Counter(
		fullyQualifiedName(o.Namespace, o.Subsystem, o.Name),
		otelmetric.WithDescription(o.Help),
	)
	if err != nil {
		logger.Warnw("Falling back to a disabled counter", "name", o.Name, "error", err.Error())
		return p.fallback.NewCounter(o)
	}
	return &otelCounter{instrument: inst}
}

func (p *MetricsProvider) NewGauge(o metrics.GaugeOpts) metrics.Gauge {
	inst, err := p.meter.Float64Gauge(
		fullyQualifiedName(o.Namespace, o.Subsystem, o.Name),
		otelmetric.WithDescription(o.Help),
	)
	if err != nil {
		logger.Warnw("Falling back to a disabled gauge", "name", o.Name, "error", err.Error())
		return p.fallback.NewGauge(o)
	}
	return &otelGauge{instrument: inst, state: &gaugeState{values: map[attribute.Distinct]float64{}}}
}

func (p *MetricsProvider) NewHistogram(o metrics.HistogramOpts) metrics.Histogram {
	opts := []otelmetric.Float64HistogramOption{otelmetric.WithDescription(o.Help)}
	if len(o.Buckets) > 0 {
		// Fabric chose these boundaries per metric to suit what it measures, so
		// they are carried over rather than left to the SDK default.
		opts = append(opts, otelmetric.WithExplicitBucketBoundaries(o.Buckets...))
	}

	inst, err := p.meter.Float64Histogram(fullyQualifiedName(o.Namespace, o.Subsystem, o.Name), opts...)
	if err != nil {
		logger.Warnw("Falling back to a disabled histogram", "name", o.Name, "error", err.Error())
		return p.fallback.NewHistogram(o)
	}
	return &otelHistogram{instrument: inst}
}

type otelCounter struct {
	instrument otelmetric.Float64Counter
	attributes []attribute.KeyValue
}

func (c *otelCounter) With(labelValues ...string) metrics.Counter {
	return &otelCounter{
		instrument: c.instrument,
		attributes: appendLabels(c.attributes, labelValues),
	}
}

func (c *otelCounter) Add(delta float64) {
	c.instrument.Add(context.Background(), delta, otelmetric.WithAttributes(c.attributes...))
}

// gaugeState holds the current value per label set.
//
// Fabric's Gauge supports both Set and Add, but OTLP carries only an absolute
// value, so Add has to be resolved against the previous reading before it can be
// recorded. The state is shared by every gauge derived through With, because
// they are all views onto the same instrument.
type gaugeState struct {
	mu     sync.Mutex
	values map[attribute.Distinct]float64
}

type otelGauge struct {
	instrument otelmetric.Float64Gauge
	state      *gaugeState
	attributes []attribute.KeyValue
}

func (g *otelGauge) With(labelValues ...string) metrics.Gauge {
	return &otelGauge{
		instrument: g.instrument,
		state:      g.state,
		attributes: appendLabels(g.attributes, labelValues),
	}
}

func (g *otelGauge) Set(value float64) { g.record(value, false) }

func (g *otelGauge) Add(delta float64) { g.record(delta, true) }

func (g *otelGauge) record(value float64, relative bool) {
	set := attribute.NewSet(g.attributes...)
	key := set.Equivalent()

	g.state.mu.Lock()
	if relative {
		value += g.state.values[key]
	}
	g.state.values[key] = value
	g.state.mu.Unlock()

	g.instrument.Record(context.Background(), value, otelmetric.WithAttributeSet(set))
}

type otelHistogram struct {
	instrument otelmetric.Float64Histogram
	attributes []attribute.KeyValue
}

func (h *otelHistogram) With(labelValues ...string) metrics.Histogram {
	return &otelHistogram{
		instrument: h.instrument,
		attributes: appendLabels(h.attributes, labelValues),
	}
}

func (h *otelHistogram) Observe(value float64) {
	h.instrument.Record(context.Background(), value, otelmetric.WithAttributes(h.attributes...))
}

// fullyQualifiedName joins the name components the way the Prometheus provider
// does, so a metric keeps the same name whichever backend is configured.
func fullyQualifiedName(namespace, subsystem, name string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{namespace, subsystem, name} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "_")
}

// appendLabels converts Fabric's alternating name/value label arguments into
// attributes, returning a new slice so that derived metrics never alias the
// parent's attributes.
func appendLabels(existing []attribute.KeyValue, labelValues []string) []attribute.KeyValue {
	if len(labelValues)%2 != 0 {
		// Dropping the unpaired trailing name is better than recording a label
		// with an empty value, which would create a second, misleading series.
		logger.Warnw("Ignoring unpaired metric label", "name", labelValues[len(labelValues)-1])
		labelValues = labelValues[:len(labelValues)-1]
	}

	attributes := make([]attribute.KeyValue, 0, len(existing)+len(labelValues)/2)
	attributes = append(attributes, existing...)
	for i := 0; i < len(labelValues); i += 2 {
		attributes = append(attributes, attribute.String(labelValues[i], labelValues[i+1]))
	}
	return attributes
}
