/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

// Counter is the interface for emitting Counter type metrics.
type Counter interface {
	// Inc increments the Counter by a delta.
	Inc(delta int64)
}

// Gauge is the interface for emitting Gauge metrics.
type Gauge interface {
	// Update sets the gauges absolute value.
	Update(value float64)
}

// Scope is a namespace wrapper around a stats reporter, ensuring that
// all emitted values have a given prefix or set of tags.
type Scope interface {
	// Counter returns the Counter object corresponding to the name.
	Counter(name string) Counter

	// Gauge returns the Gauge object corresponding to the name.
	Gauge(name string) Gauge

	// Tagged returns a new child Scope with the given tags and current tags.
	Tagged(tags map[string]string) Scope

	// SubScope returns a new child Scope appending a further name prefix.
	SubScope(name string) Scope
}
