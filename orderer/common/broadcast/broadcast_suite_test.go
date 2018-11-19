/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast_test

import (
	"testing"

	"github.com/hyperledger/fabric/common/metrics"
	ab "github.com/hyperledger/fabric/protos/orderer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o mock/ab_server.go --fake-name ABServer . abServer
type abServer interface {
	ab.AtomicBroadcast_BroadcastServer
}

//go:generate counterfeiter -o mock/metrics_histogram.go --fake-name MetricsHistogram . metricsHistogram
type metricsHistogram interface {
	metrics.Histogram
}

//go:generate counterfeiter -o mock/metrics_counter.go --fake-name MetricsCounter . metricsCounter
type metricsCounter interface {
	metrics.Counter
}

//go:generate counterfeiter -o mock/metrics_provider.go --fake-name MetricsProvider . metricsProvider
type metricsProvider interface {
	metrics.Provider
}

func TestBroadcast(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Broadcast Suite")
}
