/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package broadcast_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/orderer/common/broadcast"
	"github.com/hyperledger/fabric/orderer/common/broadcast/mock"
)

var _ = Describe("Metrics", func() {
	var (
		fakeProvider *mock.MetricsProvider
	)

	BeforeEach(func() {
		fakeProvider = &mock.MetricsProvider{}
		fakeProvider.NewHistogramReturns(&mock.MetricsHistogram{})
		fakeProvider.NewCounterReturns(&mock.MetricsCounter{})
	})

	It("uses the provider to initialize all fields", func() {
		metrics := broadcast.NewMetrics(fakeProvider)
		Expect(metrics).NotTo(BeNil())
		Expect(metrics.ValidateDuration).To(Equal(&mock.MetricsHistogram{}))
		Expect(metrics.EnqueueDuration).To(Equal(&mock.MetricsHistogram{}))
		Expect(metrics.ProcessedCount).To(Equal(&mock.MetricsCounter{}))

		Expect(fakeProvider.NewHistogramCallCount()).To(Equal(2))
		Expect(fakeProvider.NewCounterCallCount()).To(Equal(1))
	})
})
