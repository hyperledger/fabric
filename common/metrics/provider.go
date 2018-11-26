/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

// A Provider is an abstraction for a metrics provider. It is a factory for
// Counter, Gauge, and Histogram meters.
type Provider interface {
	// NewCounter creates a new instance of a Counter.
	NewCounter(CounterOpts) Counter
	// NewGauge creates a new instance of a Gauge.
	NewGauge(GaugeOpts) Gauge
	// NewHistogram creates a new instance of a Histogram.
	NewHistogram(HistogramOpts) Histogram
}

// A Counter represents a monotonically increasing value.
type Counter interface {
	// With is used to provide label values when updating a Counter. This must be
	// used to provide values for all LabelNames provided to CounterOpts.
	With(labelValues ...string) Counter

	// Add increments a counter value.
	Add(delta float64)
}

// CounterOpts is used to provide basic information about a counter to the
// metrics subsystem.
type CounterOpts struct {
	// Namespace, Subsystem, and Name are components of the fully-qualified name
	// of the Metric. The fully-qualified aneme is created by joining these
	// components with an appropriate separator. Only Name is mandatory, the
	// others merely help structuring the name.
	Namespace string
	Subsystem string
	Name      string

	// Help provides information about this metric.
	Help string

	// LabelNames provides the names of the labels that can be attached to this
	// metric. When a metric is recorded, label values must be provided for each
	// of these label names.
	LabelNames []string

	// StatsdFormat determines how the fully-qualified statsd bucket name is
	// constructed from Namespace, Subsystem, Name, and Labels. This is done by
	// including field references in `%{reference}` escape sequences.
	//
	// The following reference names are supported:
	// - #namespace   - the value of Namespace
	// - #subsystem   - the value of Subsystem
	// - #name        - the value of Name
	// - #fqname      - the fully-qualified metric name
	// - label_name   - the value associated with the named label
	//
	// The result of the formatting must be a valid statsd bucket name.
	StatsdFormat string
}

// A Gauge is a meter that expresses the current value of some metric.
type Gauge interface {
	// With is used to provide label values when recording a Gauge value. This
	// must be used to provide values for all LabelNames provided to GaugeOpts.
	With(labelValues ...string) Gauge

	// Add increments a Gauge value.
	Add(delta float64) // TODO: consider removing

	// Set is used to update the current value associted with a Gauge.
	Set(value float64)
}

// GaugeOpts is used to provide basic information about a gauge to the
// metrics subsystem.
type GaugeOpts struct {
	// Namespace, Subsystem, and Name are components of the fully-qualified name
	// of the Metric. The fully-qualified aneme is created by joining these
	// components with an appropriate separator. Only Name is mandatory, the
	// others merely help structuring the name.
	Namespace string
	Subsystem string
	Name      string

	// Help provides information about this metric.
	Help string

	// LabelNames provides the names of the labels that can be attached to this
	// metric. When a metric is recorded, label values must be provided for each
	// of these label names.
	LabelNames []string

	// StatsdFormat determines how the fully-qualified statsd bucket name is
	// constructed from Namespace, Subsystem, Name, and Labels. This is done by
	// including field references in `%{reference}` escape sequences.
	//
	// The following reference names are supported:
	// - #namespace   - the value of Namespace
	// - #subsystem   - the value of Subsystem
	// - #name        - the value of Name
	// - #fqname      - the fully-qualified metric name
	// - label_name   - the value associated with the named label
	//
	// The result of the formatting must be a valid statsd bucket name.
	StatsdFormat string
}

// A Histogram is a meter that records an observed value into quantized
// buckets.
type Histogram interface {
	// With is used to provide label values when recording a Histogram
	// observation. This must be used to provide values for all LabelNames
	// provided to HistogramOpts.
	With(labelValues ...string) Histogram
	Observe(value float64)
}

// HistogramOpts is used to provide basic information about a histogram to the
// metrics subsystem.
type HistogramOpts struct {
	// Namespace, Subsystem, and Name are components of the fully-qualified name
	// of the Metric. The fully-qualified aneme is created by joining these
	// components with an appropriate separator. Only Name is mandatory, the
	// others merely help structuring the name.
	Namespace string
	Subsystem string
	Name      string

	// Help provides information about this metric.
	Help string

	// Buckets can be used to provide the bucket boundaries for Prometheus. When
	// omitted, the default Prometheus bucket values are used.
	Buckets []float64

	// LabelNames provides the names of the labels that can be attached to this
	// metric. When a metric is recorded, label values must be provided for each
	// of these label names.
	LabelNames []string

	// StatsdFormat determines how the fully-qualified statsd bucket name is
	// constructed from Namespace, Subsystem, Name, and Labels. This is done by
	// including field references in `%{reference}` escape sequences.
	//
	// The following reference names are supported:
	// - #namespace   - the value of Namespace
	// - #subsystem   - the value of Subsystem
	// - #name        - the value of Name
	// - #fqname      - the fully-qualified metric name
	// - label_name   - the value associated with the named label
	//
	// The result of the formatting must be a valid statsd bucket name.
	StatsdFormat string
}
