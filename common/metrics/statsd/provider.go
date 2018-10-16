/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statsd

import (
	metrics "github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/statsd"
)

type Provider struct {
	Statsd *statsd.Statsd
}

func (p *Provider) NewCounter(name string) metrics.Counter {
	return p.Statsd.NewCounter(name, 1.0)
}

func (p *Provider) NewGauge(name string) metrics.Gauge {
	return p.Statsd.NewGauge(name)
}

func (p *Provider) NewHistogram(name string) metrics.Histogram {
	return p.Statsd.NewTiming(name, 1.0)
}
