/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/metrics"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	gometrics "github.com/rcrowley/go-metrics"
)

//go:generate counterfeiter -o mock/metrics_registry.go --fake-name MetricsRegistry . metricsRegistry
type metricsRegistry interface {
	gometrics.Registry
}

//go:generate counterfeiter -o mock/metrics_meter.go --fake-name MetricsMeter . metricsMeter
type metricsMeter interface {
	gometrics.Meter
}

//go:generate counterfeiter -o mock/metrics_histogram.go --fake-name MetricsHistogram . metricsHistogram
type metricsHistogram interface {
	gometrics.Histogram
}

//go:generate counterfeiter -o mock/metrics_provider.go --fake-name MetricsProvider . metricsProvider
type metricsProvider interface {
	metrics.Provider
}

//go:generate counterfeiter -o mock/metrics_gauge.go --fake-name MetricsGauge . metricsGauge
type metricsGauge interface {
	metrics.Gauge
}

func TestKafka(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kafka Suite")
}
