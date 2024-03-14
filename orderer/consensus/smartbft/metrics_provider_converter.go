/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft

import (
	api "github.com/hyperledger-labs/SmartBFT/pkg/metrics"
	"github.com/hyperledger/fabric-lib-go/common/metrics"
)

type MetricProviderConverter struct {
	MetricsProvider metrics.Provider
}

func (m *MetricProviderConverter) NewCounter(opts api.CounterOpts) api.Counter {
	o := metrics.CounterOpts{
		Namespace:    opts.Namespace,
		Subsystem:    opts.Subsystem,
		Name:         opts.Name,
		Help:         opts.Help,
		LabelNames:   opts.LabelNames,
		LabelHelp:    opts.LabelHelp,
		StatsdFormat: opts.StatsdFormat,
	}
	return &CounterConverter{
		counter: m.MetricsProvider.NewCounter(o),
	}
}

func (m *MetricProviderConverter) NewGauge(opts api.GaugeOpts) api.Gauge {
	o := metrics.GaugeOpts{
		Namespace:    opts.Namespace,
		Subsystem:    opts.Subsystem,
		Name:         opts.Name,
		Help:         opts.Help,
		LabelNames:   opts.LabelNames,
		LabelHelp:    opts.LabelHelp,
		StatsdFormat: opts.StatsdFormat,
	}
	return &GaugeConverter{
		gauge: m.MetricsProvider.NewGauge(o),
	}
}

func (m *MetricProviderConverter) NewHistogram(opts api.HistogramOpts) api.Histogram {
	o := metrics.HistogramOpts{
		Namespace:    opts.Namespace,
		Subsystem:    opts.Subsystem,
		Name:         opts.Name,
		Help:         opts.Help,
		LabelNames:   opts.LabelNames,
		LabelHelp:    opts.LabelHelp,
		StatsdFormat: opts.StatsdFormat,
		Buckets:      opts.Buckets,
	}
	return &HistogramConverter{
		histogram: m.MetricsProvider.NewHistogram(o),
	}
}

type CounterConverter struct {
	counter metrics.Counter
}

func (c *CounterConverter) With(labelValues ...string) api.Counter {
	return &CounterConverter{
		counter: c.counter.With(labelValues...),
	}
}

func (c *CounterConverter) Add(delta float64) {
	c.counter.Add(delta)
}

type GaugeConverter struct {
	gauge metrics.Gauge
}

func (g *GaugeConverter) With(labelValues ...string) api.Gauge {
	return &GaugeConverter{
		gauge: g.gauge.With(labelValues...),
	}
}

func (g *GaugeConverter) Add(delta float64) {
	g.gauge.Add(delta)
}

func (g *GaugeConverter) Set(value float64) {
	g.gauge.Set(value)
}

type HistogramConverter struct {
	histogram metrics.Histogram
}

func (h *HistogramConverter) With(labelValues ...string) api.Histogram {
	return &HistogramConverter{
		histogram: h.histogram.With(labelValues...),
	}
}

func (h *HistogramConverter) Observe(value float64) {
	h.histogram.Observe(value)
}
