// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package prometheus

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/uber-go/tally"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// DefaultSeparator is the default separator that should be used with
	// a tally scope for a prometheus reporter.
	DefaultSeparator = "_"
)

var (
	errUnknownTimerType = errors.New("unknown metric timer type")
	ms                  = float64(time.Millisecond) / float64(time.Second)
)

// DefaultHistogramBuckets is the default histogram buckets used when
// creating a new Histogram in the prometheus registry.
// See: https://godoc.org/github.com/prometheus/client_golang/prometheus#HistogramOpts
func DefaultHistogramBuckets() []float64 {
	return []float64{
		ms,
		2 * ms,
		5 * ms,
		10 * ms,
		20 * ms,
		50 * ms,
		100 * ms,
		200 * ms,
		500 * ms,
		1000 * ms,
		2000 * ms,
		5000 * ms,
		10000 * ms,
	}
}

// DefaultSummaryObjectives is the default objectives used when
// creating a new Summary in the prometheus registry.
// See: https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts
func DefaultSummaryObjectives() map[float64]float64 {
	return map[float64]float64{
		0.5:   0.01,
		0.75:  0.001,
		0.95:  0.001,
		0.99:  0.001,
		0.999: 0.0001,
	}
}

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

// RegisterTimerOptions provides options when registering a timer on demand.
// By default you can pass nil for the options to get the reporter defaults.
type RegisterTimerOptions struct {
	TimerType         TimerType
	HistogramBuckets  []float64
	SummaryObjectives map[float64]float64
}

// TimerUnion is a representation of either a summary or a histogram
// described by the TimerType.
type TimerUnion struct {
	TimerType TimerType
	Histogram *prom.HistogramVec
	Summary   *prom.SummaryVec
}

type metricID string

type reporter struct {
	sync.RWMutex
	registerer      prom.Registerer
	timerType       TimerType
	objectives      map[float64]float64
	buckets         []float64
	onRegisterError func(e error)
	counters        map[metricID]*prom.CounterVec
	gauges          map[metricID]*prom.GaugeVec
	timers          map[metricID]*promTimerVec
	histograms      map[metricID]*prom.HistogramVec
}

type promTimerVec struct {
	summary   *prom.SummaryVec
	histogram *prom.HistogramVec
}

type cachedMetric struct {
	counter     prom.Counter
	gauge       prom.Gauge
	reportTimer func(d time.Duration)
	histogram   prom.Histogram
	summary     prom.Summary
}

func (m *cachedMetric) ReportCount(value int64) {
	m.counter.Add(float64(value))
}

func (m *cachedMetric) ReportGauge(value float64) {
	m.gauge.Set(value)
}

func (m *cachedMetric) ReportTimer(interval time.Duration) {
	m.reportTimer(interval)
}

func (m *cachedMetric) reportTimerHistogram(interval time.Duration) {
	m.histogram.Observe(float64(interval) / float64(time.Second))
}

func (m *cachedMetric) reportTimerSummary(interval time.Duration) {
	m.summary.Observe(float64(interval) / float64(time.Second))
}

func (m *cachedMetric) ValueBucket(
	bucketLowerBound, bucketUpperBound float64,
) tally.CachedHistogramBucket {
	return cachedHistogramBucket{m, bucketUpperBound}
}

func (m *cachedMetric) DurationBucket(
	bucketLowerBound, bucketUpperBound time.Duration,
) tally.CachedHistogramBucket {
	upperBound := float64(bucketUpperBound) / float64(time.Second)
	return cachedHistogramBucket{m, upperBound}
}

type cachedHistogramBucket struct {
	metric     *cachedMetric
	upperBound float64
}

func (b cachedHistogramBucket) ReportSamples(value int64) {
	for i := int64(0); i < value; i++ {
		b.metric.histogram.Observe(b.upperBound)
	}
}

type noopMetric struct{}

func (m noopMetric) ReportCount(value int64)            {}
func (m noopMetric) ReportGauge(value float64)          {}
func (m noopMetric) ReportTimer(interval time.Duration) {}
func (m noopMetric) ReportSamples(value int64)          {}
func (m noopMetric) ValueBucket(lower, upper float64) tally.CachedHistogramBucket {
	return m
}
func (m noopMetric) DurationBucket(lower, upper time.Duration) tally.CachedHistogramBucket {
	return m
}

func (r *reporter) HTTPHandler() http.Handler {
	return promhttp.Handler()
}

// TimerType describes a type of timer
type TimerType int

const (
	// SummaryTimerType is a timer type that reports into a summary
	SummaryTimerType TimerType = iota

	// HistogramTimerType is a timer type that reports into a histogram
	HistogramTimerType
)

// Options is a set of options for the tally reporter.
type Options struct {
	// Registerer is the prometheus registerer to register
	// metrics with. Use nil to specify the default registerer.
	Registerer prom.Registerer

	// DefaultTimerType is the default type timer type to create
	// when using timers. It's default value is a summary timer type.
	DefaultTimerType TimerType

	// DefaultHistogramBuckets is the default histogram buckets
	// to use. Use nil to specify the default histogram buckets.
	DefaultHistogramBuckets []float64

	// DefaultSummaryObjectives is the default summary objectives
	// to use. Use nil to specify the default summary objectives.
	DefaultSummaryObjectives map[float64]float64

	// OnRegisterError defines a method to call to when registering
	// a metric with the registerer fails. Use nil to specify
	// to panic by default when registering fails.
	OnRegisterError func(err error)
}

// NewReporter returns a new Reporter for Prometheus client backed metrics
// objectives is the objectives used when creating a new Summary histogram for Timers. See
// https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts for more details.
func NewReporter(opts Options) Reporter {
	if opts.Registerer == nil {
		opts.Registerer = prom.DefaultRegisterer
	}
	if opts.DefaultHistogramBuckets == nil {
		opts.DefaultHistogramBuckets = DefaultHistogramBuckets()
	}
	if opts.DefaultSummaryObjectives == nil {
		opts.DefaultSummaryObjectives = DefaultSummaryObjectives()
	}
	if opts.OnRegisterError == nil {
		opts.OnRegisterError = func(err error) {
			panic(err)
		}
	}
	return &reporter{
		registerer:      opts.Registerer,
		timerType:       opts.DefaultTimerType,
		buckets:         opts.DefaultHistogramBuckets,
		objectives:      opts.DefaultSummaryObjectives,
		onRegisterError: opts.OnRegisterError,
		counters:        make(map[metricID]*prom.CounterVec),
		gauges:          make(map[metricID]*prom.GaugeVec),
		timers:          make(map[metricID]*promTimerVec),
	}
}

func (r *reporter) RegisterCounter(
	name string,
	tagKeys []string,
	desc string,
) (*prom.CounterVec, error) {
	return r.counterVec(name, tagKeys, desc)
}

func (r *reporter) counterVec(
	name string,
	tagKeys []string,
	desc string,
) (*prom.CounterVec, error) {
	id := canonicalMetricID(name, tagKeys)

	r.Lock()
	defer r.Unlock()

	if ctr, ok := r.counters[id]; ok {
		return ctr, nil
	}

	ctr := prom.NewCounterVec(
		prom.CounterOpts{
			Name: name,
			Help: desc,
		},
		tagKeys,
	)

	if err := r.registerer.Register(ctr); err != nil {
		return nil, err
	}

	r.counters[id] = ctr
	return ctr, nil
}

// AllocateCounter implements tally.CachedStatsReporter.
func (r *reporter) AllocateCounter(name string, tags map[string]string) tally.CachedCount {
	tagKeys := keysFromMap(tags)
	counterVec, err := r.counterVec(name, tagKeys, name+" counter")
	if err != nil {
		r.onRegisterError(err)
		return noopMetric{}
	}
	return &cachedMetric{counter: counterVec.With(tags)}
}

func (r *reporter) RegisterGauge(
	name string,
	tagKeys []string,
	desc string,
) (*prom.GaugeVec, error) {
	return r.gaugeVec(name, tagKeys, desc)
}

func (r *reporter) gaugeVec(
	name string,
	tagKeys []string,
	desc string,
) (*prom.GaugeVec, error) {
	id := canonicalMetricID(name, tagKeys)

	r.Lock()
	defer r.Unlock()

	if g, ok := r.gauges[id]; ok {
		return g, nil
	}

	g := prom.NewGaugeVec(
		prom.GaugeOpts{
			Name: name,
			Help: desc,
		},
		tagKeys,
	)

	if err := r.registerer.Register(g); err != nil {
		return nil, err
	}

	r.gauges[id] = g
	return g, nil
}

// AllocateGauge implements tally.CachedStatsReporter.
func (r *reporter) AllocateGauge(name string, tags map[string]string) tally.CachedGauge {
	tagKeys := keysFromMap(tags)
	gaugeVec, err := r.gaugeVec(name, tagKeys, name+" gauge")
	if err != nil {
		r.onRegisterError(err)
		return noopMetric{}
	}
	return &cachedMetric{gauge: gaugeVec.With(tags)}
}

func (r *reporter) RegisterTimer(
	name string,
	tagKeys []string,
	desc string,
	opts *RegisterTimerOptions,
) (TimerUnion, error) {
	timerType, buckets, objectives := r.timerConfig(opts)
	switch timerType {
	case HistogramTimerType:
		h, err := r.histogramVec(name, tagKeys, desc, buckets)
		return TimerUnion{TimerType: timerType, Histogram: h}, err
	case SummaryTimerType:
		s, err := r.summaryVec(name, tagKeys, desc, objectives)
		return TimerUnion{TimerType: timerType, Summary: s}, err
	}
	return TimerUnion{}, errUnknownTimerType
}

func (r *reporter) timerConfig(
	opts *RegisterTimerOptions,
) (
	timerType TimerType,
	buckets []float64,
	objectives map[float64]float64,
) {
	timerType = r.timerType
	objectives = r.objectives
	buckets = r.buckets
	if opts != nil {
		timerType = opts.TimerType
		if opts.SummaryObjectives != nil {
			objectives = opts.SummaryObjectives
		}
		if opts.HistogramBuckets != nil {
			buckets = opts.HistogramBuckets
		}
	}
	return
}

func (r *reporter) summaryVec(
	name string,
	tagKeys []string,
	desc string,
	objectives map[float64]float64,
) (*prom.SummaryVec, error) {
	id := canonicalMetricID(name, tagKeys)

	r.Lock()
	defer r.Unlock()

	if s, ok := r.timers[id]; ok {
		return s.summary, nil
	}

	s := prom.NewSummaryVec(
		prom.SummaryOpts{
			Name:       name,
			Help:       desc,
			Objectives: objectives,
		},
		tagKeys,
	)

	if err := r.registerer.Register(s); err != nil {
		return nil, err
	}

	r.timers[id] = &promTimerVec{summary: s}
	return s, nil
}

func (r *reporter) histogramVec(
	name string,
	tagKeys []string,
	desc string,
	buckets []float64,
) (*prom.HistogramVec, error) {
	id := canonicalMetricID(name, tagKeys)

	r.Lock()
	defer r.Unlock()

	if h, ok := r.timers[id]; ok {
		return h.histogram, nil
	}

	h := prom.NewHistogramVec(
		prom.HistogramOpts{
			Name:    name,
			Help:    desc,
			Buckets: buckets,
		},
		tagKeys,
	)

	if err := r.registerer.Register(h); err != nil {
		return nil, err
	}

	r.timers[id] = &promTimerVec{histogram: h}
	return h, nil
}

// AllocateTimer implements tally.CachedStatsReporter.
func (r *reporter) AllocateTimer(name string, tags map[string]string) tally.CachedTimer {
	var (
		timer tally.CachedTimer
		err   error
	)
	tagKeys := keysFromMap(tags)
	timerType, buckets, objectives := r.timerConfig(nil)
	switch timerType {
	case HistogramTimerType:
		var histogramVec *prom.HistogramVec
		histogramVec, err = r.histogramVec(name, tagKeys, name+" histogram", buckets)
		if err == nil {
			t := &cachedMetric{histogram: histogramVec.With(tags)}
			t.reportTimer = t.reportTimerHistogram
			timer = t
		}
	case SummaryTimerType:
		var summaryVec *prom.SummaryVec
		summaryVec, err = r.summaryVec(name, tagKeys, name+" summary", objectives)
		if err == nil {
			t := &cachedMetric{summary: summaryVec.With(tags)}
			t.reportTimer = t.reportTimerSummary
			timer = t
		}
	default:
		err = errUnknownTimerType
	}
	if err != nil {
		r.onRegisterError(err)
		return noopMetric{}
	}
	return timer
}

func (r *reporter) AllocateHistogram(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
) tally.CachedHistogram {
	tagKeys := keysFromMap(tags)
	histogramVec, err := r.histogramVec(name, tagKeys, name+" histogram", buckets.AsValues())
	if err != nil {
		r.onRegisterError(err)
		return noopMetric{}
	}
	return &cachedMetric{histogram: histogramVec.With(tags)}
}

func (r *reporter) Capabilities() tally.Capabilities {
	return r
}

func (r *reporter) Reporting() bool {
	return true
}

func (r *reporter) Tagging() bool {
	return true
}

// Flush does nothing for prometheus
func (r *reporter) Flush() {}

var metricIDKeyValue = "1"

// NOTE: this generates a canonical MetricID for a given name+label keys,
// not values. This omits label values, as we track metrics as
// Vectors in order to support on-the-fly label changes.
func canonicalMetricID(name string, tagKeys []string) metricID {
	keySet := make(map[string]string, len(tagKeys))
	for _, key := range tagKeys {
		keySet[key] = metricIDKeyValue
	}
	return metricID(tally.KeyForPrefixedStringMap(name, keySet))
}

func keysFromMap(m map[string]string) []string {
	labelKeys := make([]string, len(m))
	i := 0
	for k := range m {
		labelKeys[i] = k
		i++
	}
	return labelKeys
}
