/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics_test

import (
	kitmetrics "github.com/go-kit/kit/metrics"
)

//go:generate counterfeiter -o metricsfakes/provider.go -fake-name Provider . Provider

//go:generate counterfeiter -o metricsfakes/counter.go -fake-name Counter . counter
type counter interface {
	kitmetrics.Counter
}

//go:generate counterfeiter -o metricsfakes/gauge.go -fake-name Gauge . gauge
type gauge interface {
	kitmetrics.Gauge
}

//go:generate counterfeiter -o metricsfakes/histogram.go -fake-name Histogram . histogram
type histogram interface {
	kitmetrics.Histogram
}
