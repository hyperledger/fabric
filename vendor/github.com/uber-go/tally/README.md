# :heavy_check_mark: tally [![GoDoc][doc-img]][doc] [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

Fast, buffered, hierarchical stats collection in Go.

## Installation
`go get -u github.com/uber-go/tally`

## Abstract

Tally provides a common interface for emitting metrics, while letting you not worry about the velocity of metrics emission.

By default it buffers counters, gauges and histograms at a specified interval but does not buffer timer values.  This is primarily so timer values can have all their values sampled if desired and if not they can be sampled as summaries or histograms independently by a reporter.

## Structure

- Scope: Keeps track of metrics, and their common metadata.
- Metrics: Counters, Gauges, Timers and Histograms.
- Reporter: Implemented by you. Accepts aggregated values from the scope. Forwards the aggregated values to your metrics ingestion pipeline.
  - The reporters already available listed alphabetically are:
	 - `github.com/uber-go/tally/m3`: Report m3 metrics, timers are not sampled and forwarded directly.
	 - `github.com/uber-go/tally/multi`: Report to multiple reporters, you can multi-write metrics to other reporters simply.
	 - `github.com/uber-go/tally/prometheus`: Report prometheus metrics, timers by default are made summaries with an option to make them histograms instead.
	 - `github.com/uber-go/tally/statsd`: Report statsd metrics, no support for tags.

### Acquire a Scope ###
```go
reporter = NewMyStatsReporter()  // Implement as you will
tags := map[string]string{
	"dc": "east-1",
	"type": "master",
}
reportEvery := time.Second

scope := tally.NewRootScope(tally.ScopeOptions{
	Tags: tags,
	Reporter: reporter,
}, reportEvery)
```

### Get/Create a metric, use it ###
```go
// Get a counter, increment a counter
reqCounter := scope.Counter("requests")  // cache me
reqCounter.Inc(1)

queueGauge := scope.Gauge("queue_length")  // cache me
queueGauge.Update(42)
```

### Report your metrics ###
Use the inbuilt statsd reporter:

```go
import (
	"io"
	"github.com/cactus/go-statsd-client/statsd"
	"github.com/uber-go/tally"
	tallystatsd "github.com/uber-go/tally/statsd"
	// ...
)

func newScope() (tally.Scope, io.Closer) {
	statter, _ := statsd.NewBufferedClient("127.0.0.1:8125",
		"stats", 100*time.Millisecond, 1440)
	
	reporter := tallystatsd.NewReporter(statter, tallystatsd.Options{
		SampleRate: 1.0,
	})
	
	scope, closer := tally.NewRootScope(tally.ScopeOptions{
		Prefix:   "my-service",
		Tags:     map[string]string{},
		Reporter: r,
	}, time.Second)

	return scope, closer
}
```

Implement your own reporter using the `StatsReporter` interface:

```go

// BaseStatsReporter implements the shared reporter methods.
type BaseStatsReporter interface {
	Capabilities() Capabilities
	Flush()
}

// StatsReporter is a backend for Scopes to report metrics to.
type StatsReporter interface {
	BaseStatsReporter

	// ReportCounter reports a counter value
	ReportCounter(
		name string,
		tags map[string]string,
		value int64,
	)

	// ReportGauge reports a gauge value
	ReportGauge(
		name string,
		tags map[string]string,
		value float64,
	)

	// ReportTimer reports a timer value
	ReportTimer(
		name string,
		tags map[string]string,
		interval time.Duration,
	)

	// ReportHistogramValueSamples reports histogram samples for a bucket
	ReportHistogramValueSamples(
		name string,
		tags map[string]string,
		buckets Buckets,
		bucketLowerBound,
		bucketUpperBound float64,
		samples int64,
	)

	// ReportHistogramDurationSamples reports histogram samples for a bucket
	ReportHistogramDurationSamples(
		name string,
		tags map[string]string,
		buckets Buckets,
		bucketLowerBound,
		bucketUpperBound time.Duration,
		samples int64,
	)
}
```

Or implement your own metrics implementation that matches the tally `Scope` interface to use different buffering semantics:

```go
type Scope interface {
	// Counter returns the Counter object corresponding to the name.
	Counter(name string) Counter

	// Gauge returns the Gauge object corresponding to the name.
	Gauge(name string) Gauge

	// Timer returns the Timer object corresponding to the name.
	Timer(name string) Timer

	// Histogram returns the Histogram object corresponding to the name.
	// To use default value and duration buckets configured for the scope
	// simply pass tally.DefaultBuckets or nil.
	// You can use tally.ValueBuckets{x, y, ...} for value buckets.
	// You can use tally.DurationBuckets{x, y, ...} for duration buckets.
	// You can use tally.MustMakeLinearValueBuckets(start, width, count) for linear values.
	// You can use tally.MustMakeLinearDurationBuckets(start, width, count) for linear durations.
	// You can use tally.MustMakeExponentialValueBuckets(start, factor, count) for exponential values.
	// You can use tally.MustMakeExponentialDurationBuckets(start, factor, count) for exponential durations.
	Histogram(name string, buckets Buckets) Histogram

	// Tagged returns a new child scope with the given tags and current tags.
	Tagged(tags map[string]string) Scope

	// SubScope returns a new child scope appending a further name prefix.
	SubScope(name string) Scope

	// Capabilities returns a description of metrics reporting capabilities.
	Capabilities() Capabilities
}

// Capabilities is a description of metrics reporting capabilities.
type Capabilities interface {
	// Reporting returns whether the reporter has the ability to actively report.
	Reporting() bool

	// Tagging returns whether the reporter has the capability for tagged metrics.
	Tagging() bool
}
```

## Performance

This stuff needs to be fast. With that in mind, we avoid locks and unnecessary memory allocations.

```
BenchmarkCounterInc-8               	200000000	         7.68 ns/op
BenchmarkReportCounterNoData-8      	300000000	         4.88 ns/op
BenchmarkReportCounterWithData-8    	100000000	        21.6 ns/op
BenchmarkGaugeSet-8                 	100000000	        16.0 ns/op
BenchmarkReportGaugeNoData-8        	100000000	        10.4 ns/op
BenchmarkReportGaugeWithData-8      	50000000	        27.6 ns/op
BenchmarkTimerInterval-8            	50000000	        37.7 ns/op
BenchmarkTimerReport-8              	300000000	         5.69 ns/op
```

<hr>

Released under the [MIT License](LICENSE).

[doc-img]: https://godoc.org/github.com/uber-go/tally?status.svg
[doc]: https://godoc.org/github.com/uber-go/tally
[ci-img]: https://travis-ci.org/uber-go/tally.svg?branch=master
[ci]: https://travis-ci.org/uber-go/tally
[cov-img]: https://coveralls.io/repos/github/uber-go/tally/badge.svg?branch=master
[cov]: https://coveralls.io/github/uber-go/tally?branch=master
[glide.lock]: https://github.com/uber-go/tally/blob/master/glide.lock
[v1]: https://github.com/uber-go/tally/milestones
