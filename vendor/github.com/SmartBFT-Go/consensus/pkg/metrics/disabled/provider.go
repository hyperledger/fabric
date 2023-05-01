/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package disabled

import (
	bft "github.com/SmartBFT-Go/consensus/pkg/api"
)

type Provider struct{}

func (p *Provider) NewCounter(bft.CounterOpts) bft.Counter       { return &Counter{} }
func (p *Provider) NewGauge(bft.GaugeOpts) bft.Gauge             { return &Gauge{} }
func (p *Provider) NewHistogram(bft.HistogramOpts) bft.Histogram { return &Histogram{} }

type Counter struct{}

func (c *Counter) Add(float64) {}
func (c *Counter) With(...string) bft.Counter {
	return c
}

type Gauge struct{}

func (g *Gauge) Add(float64) {}
func (g *Gauge) Set(float64) {}
func (g *Gauge) With(...string) bft.Gauge {
	return g
}

type Histogram struct{}

func (h *Histogram) Observe(float64) {}
func (h *Histogram) With(...string) bft.Histogram {
	return h
}
