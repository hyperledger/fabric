/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statsd

import (
	"github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics"
)

const defaultFormat = "%{#fqname}"

type Provider struct {
	Statsd *statsd.Statsd
}

func (p *Provider) NewCounter(o metrics.CounterOpts) metrics.Counter {
	if o.StatsdFormat == "" {
		o.StatsdFormat = defaultFormat
	}
	counter := &Counter{
		namer: &namer{
			namespace:  o.Namespace,
			subsystem:  o.Subsystem,
			name:       o.Name,
			nameFormat: o.StatsdFormat,
			labelNames: sliceToSet(o.LabelNames),
		},
		statsdProvider: p.Statsd,
	}

	if len(o.LabelNames) == 0 {
		counter.Counter = p.Statsd.NewCounter(counter.namer.Format(), 1)
	}

	return counter
}

func (p *Provider) NewGauge(o metrics.GaugeOpts) metrics.Gauge {
	if o.StatsdFormat == "" {
		o.StatsdFormat = defaultFormat
	}
	gauge := &Gauge{
		namer: &namer{
			namespace:  o.Namespace,
			subsystem:  o.Subsystem,
			name:       o.Name,
			nameFormat: o.StatsdFormat,
			labelNames: sliceToSet(o.LabelNames),
		},
		statsdProvider: p.Statsd,
	}

	if len(o.LabelNames) == 0 {
		gauge.Gauge = p.Statsd.NewGauge(gauge.namer.Format())
	}

	return gauge
}

func (p *Provider) NewHistogram(o metrics.HistogramOpts) metrics.Histogram {
	if o.StatsdFormat == "" {
		o.StatsdFormat = defaultFormat
	}
	histogram := &Histogram{
		namer: &namer{
			namespace:  o.Namespace,
			subsystem:  o.Subsystem,
			name:       o.Name,
			nameFormat: o.StatsdFormat,
			labelNames: sliceToSet(o.LabelNames),
		},
		statsdProvider: p.Statsd,
	}

	if len(o.LabelNames) == 0 {
		histogram.Timing = p.Statsd.NewTiming(histogram.namer.Format(), 1.0)
	}

	return histogram
}

type Counter struct {
	Counter        *statsd.Counter
	namer          *namer
	statsdProvider *statsd.Statsd
}

func (c *Counter) Add(delta float64) {
	if c.Counter == nil {
		panic("label values must be provided by calling With")
	}
	c.Counter.Add(delta)
}

func (c *Counter) With(labelValues ...string) metrics.Counter {
	name := c.namer.Format(labelValues...)
	return &Counter{Counter: c.statsdProvider.NewCounter(name, 1)}
}

type Gauge struct {
	Gauge          *statsd.Gauge
	namer          *namer
	statsdProvider *statsd.Statsd
}

func (g *Gauge) Add(delta float64) {
	if g.Gauge == nil {
		panic("label values must be provided by calling With")
	}
	g.Gauge.Add(delta)
}

func (g *Gauge) Set(value float64) {
	if g.Gauge == nil {
		panic("label values must be provided by calling With")
	}
	g.Gauge.Set(value)
}

func (g *Gauge) With(labelValues ...string) metrics.Gauge {
	name := g.namer.Format(labelValues...)
	return &Gauge{Gauge: g.statsdProvider.NewGauge(name)}
}

type Histogram struct {
	Timing         *statsd.Timing
	namer          *namer
	statsdProvider *statsd.Statsd
}

func (h *Histogram) With(labelValues ...string) metrics.Histogram {
	name := h.namer.Format(labelValues...)
	return &Histogram{Timing: h.statsdProvider.NewTiming(name, 1)}
}

func (h *Histogram) Observe(value float64) {
	if h.Timing == nil {
		panic("label values must be provided by calling With")
	}
	h.Timing.Observe(value)
}

func sliceToSet(set []string) map[string]struct{} {
	labelSet := map[string]struct{}{}
	for _, s := range set {
		labelSet[s] = struct{}{}
	}
	return labelSet
}
