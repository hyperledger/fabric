/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import "github.com/go-kit/kit/metrics"

type Provider interface {
	NewCounter(name string) metrics.Counter
	NewGauge(name string) metrics.Gauge
	NewHistogram(name string) metrics.Histogram
}

type Stopper interface {
	Stop()
}
