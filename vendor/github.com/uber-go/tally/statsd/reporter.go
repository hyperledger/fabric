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

package statsd

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/uber-go/tally"

	"github.com/cactus/go-statsd-client/statsd"
)

const (
	// DefaultHistogramBucketNamePrecision is the default
	// precision to use when formatting the metric name
	// with the histogram bucket bound values.
	DefaultHistogramBucketNamePrecision = uint(6)
)

type cactusStatsReporter struct {
	statter    statsd.Statter
	sampleRate float32
	bucketFmt  string
}

// Options is a set of options for the tally reporter.
type Options struct {
	// SampleRate is the metrics emission sample rate. If you
	// do not set this value it will be set to 1.
	SampleRate float32

	// HistogramBucketNamePrecision is the precision to use when
	// formatting the metric name with the histogram bucket bound values.
	// By default this will be set to the const DefaultHistogramBucketPrecision.
	HistogramBucketNamePrecision uint
}

// NewReporter wraps a statsd.Statter for use with tally. Use either
// statsd.NewClient or statsd.NewBufferedClient.
func NewReporter(statsd statsd.Statter, opts Options) tally.StatsReporter {
	var nilSampleRate float32
	if opts.SampleRate == nilSampleRate {
		opts.SampleRate = 1.0
	}
	if opts.HistogramBucketNamePrecision == 0 {
		opts.HistogramBucketNamePrecision = DefaultHistogramBucketNamePrecision
	}
	return &cactusStatsReporter{
		statter:    statsd,
		sampleRate: opts.SampleRate,
		bucketFmt:  "%." + strconv.Itoa(int(opts.HistogramBucketNamePrecision)) + "f",
	}
}

func (r *cactusStatsReporter) ReportCounter(name string, tags map[string]string, value int64) {
	r.statter.Inc(name, value, r.sampleRate)
}

func (r *cactusStatsReporter) ReportGauge(name string, tags map[string]string, value float64) {
	r.statter.Gauge(name, int64(value), r.sampleRate)
}

func (r *cactusStatsReporter) ReportTimer(name string, tags map[string]string, interval time.Duration) {
	r.statter.TimingDuration(name, interval, r.sampleRate)
}

func (r *cactusStatsReporter) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound float64,
	samples int64,
) {
	r.statter.Inc(
		fmt.Sprintf("%s.%s-%s", name,
			r.valueBucketString(bucketLowerBound),
			r.valueBucketString(bucketUpperBound)),
		samples, r.sampleRate)
}

func (r *cactusStatsReporter) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound time.Duration,
	samples int64,
) {
	r.statter.Inc(
		fmt.Sprintf("%s.%s-%s", name,
			r.durationBucketString(bucketLowerBound),
			r.durationBucketString(bucketUpperBound)),
		samples, r.sampleRate)
}

func (r *cactusStatsReporter) valueBucketString(
	upperBound float64,
) string {
	if upperBound == math.MaxFloat64 {
		return "infinity"
	}
	if upperBound == -math.MaxFloat64 {
		return "-infinity"
	}
	return fmt.Sprintf(r.bucketFmt, upperBound)
}

func (r *cactusStatsReporter) durationBucketString(
	upperBound time.Duration,
) string {
	if upperBound == time.Duration(math.MaxInt64) {
		return "infinity"
	}
	if upperBound == time.Duration(math.MinInt64) {
		return "-infinity"
	}
	return upperBound.String()
}

func (r *cactusStatsReporter) Capabilities() tally.Capabilities {
	return r
}

func (r *cactusStatsReporter) Reporting() bool {
	return true
}

func (r *cactusStatsReporter) Tagging() bool {
	return false
}

func (r *cactusStatsReporter) Flush() {
	// no-op
}
