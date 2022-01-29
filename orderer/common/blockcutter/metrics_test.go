/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockcutter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/blockcutter/mock"
)

var _ = Describe("Metrics", func() {
	Describe("NewMetrics", func() {
		var fakeProvider *mock.MetricsProvider

		BeforeEach(func() {
			fakeProvider = &mock.MetricsProvider{}
			fakeProvider.NewHistogramReturns(&mock.MetricsHistogram{})
		})

		It("uses the provider to initialize its field", func() {
			metrics := blockcutter.NewMetrics(fakeProvider)
			Expect(metrics).NotTo(BeNil())
			Expect(metrics.BlockFillDuration).To(Equal(&mock.MetricsHistogram{}))

			Expect(fakeProvider.NewHistogramCallCount()).To(Equal(1))
		})
	})
})
