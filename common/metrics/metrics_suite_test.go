/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

//go:generate counterfeiter -o metricsfakes/provider.go -fake-name Provider . Provider
//go:generate counterfeiter -o metricsfakes/counter.go -fake-name Counter . Counter
//go:generate counterfeiter -o metricsfakes/gauge.go -fake-name Gauge . Gauge
//go:generate counterfeiter -o metricsfakes/histogram.go -fake-name Histogram . Histogram
