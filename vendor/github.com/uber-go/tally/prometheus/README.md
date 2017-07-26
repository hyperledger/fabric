# A buffered Prometheus reporter

See `examples/prometheus_main.go` for an end to end example.

## Options

You can use a specific Prometheus registry, and you can use
either summaries or histograms for timers.

The reporter options are:

```go
// Options is a set of options for the tally reporter.
type Options struct {
	// Registerer is the prometheus registerer to register
	// metrics with. Use nil to specify the default registerer.
	Registerer prom.Registerer

	// DefaultTimerType is the default type timer type to create
	// when using timers. It's default value is a histogram timer type.
	DefaultTimerType TimerType

	// DefaultHistogramBuckets is the default histogram buckets
	// to use. Use nil to specify the default histogram buckets.
	DefaultHistogramBuckets []float64

	// DefaultSummaryObjectives is the default summary objectives
	// to use. Use nil to specify the default summary objectives.
	DefaultSummaryObjectives map[float64]float64

	// OnRegisterError defines a method to call to when registering
	// a metric with the registerer fails. Use nil to specify
	// to panic by default when registering a metric fails.
	OnRegisterError func(err error)
}
```

The timer types are:

```go
// TimerType describes a type of timer
type TimerType int

const (
	// SummaryTimerType is a timer type that reports into a summary
	SummaryTimerType TimerType = iota

	// HistogramTimerType is a timer type that reports into a histogram
	HistogramTimerType
)
```

You can also pre-register help description text ahead of using a metric
that will be named and tagged identically with `tally`. You can also
access the Prometheus HTTP handler directly.

The returned reporter interface:

```go
// Reporter is a Prometheus backed tally reporter.
type Reporter interface {
	tally.CachedStatsReporter

	// HTTPHandler provides the Prometheus HTTP scrape handler.
	HTTPHandler() http.Handler

	// RegisterCounter is a helper method to initialize a counter
	// in the Prometheus backend with a given help text.
	// If not called explicitly, the Reporter will create one for
	// you on first use, with a not super helpful HELP string.
	RegisterCounter(
		name string,
		tagKeys []string,
		desc string,
	) (*prom.CounterVec, error)

	// RegisterGauge is a helper method to initialize a gauge
	// in the prometheus backend with a given help text.
	// If not called explicitly, the Reporter will create one for
	// you on first use, with a not super helpful HELP string.
	RegisterGauge(
		name string,
		tagKeys []string,
		desc string,
	) (*prom.GaugeVec, error)

	// RegisterTimer is a helper method to initialize a timer
	// summary or histogram vector in the prometheus backend
	// with a given help text.
	// If not called explicitly, the Reporter will create one for
	// you on first use, with a not super helpful HELP string.
	// You may pass opts as nil to get the default timer type
	// and objectives/buckets.
	// You may also pass objectives/buckets as nil in opts to
	// get the default objectives/buckets for the specified
	// timer type.
	RegisterTimer(
		name string,
		tagKeys []string,
		desc string,
		opts *RegisterTimerOptions,
	) (TimerUnion, error)
}
```

The register timer options:

```go
// RegisterTimerOptions provides options when registering a timer on demand.
// By default you can pass nil for the options to get the reporter defaults.
type RegisterTimerOptions struct {
	TimerType         TimerType
	HistogramBuckets  []float64
	SummaryObjectives map[float64]float64
}
```


