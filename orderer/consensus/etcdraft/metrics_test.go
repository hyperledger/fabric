/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft_test

import (
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics", func() {
	Context("NewMetrics", func() {
		var (
			fakeProvider *metricsfakes.Provider
			fakeGauge    *metricsfakes.Gauge
			fakeCounter  *metricsfakes.Counter
		)

		BeforeEach(func() {
			fakeProvider = &metricsfakes.Provider{}
			fakeGauge = &metricsfakes.Gauge{}
			fakeCounter = &metricsfakes.Counter{}

			fakeProvider.NewGaugeReturns(fakeGauge)
			fakeProvider.NewCounterReturns(fakeCounter)
		})

		It("uses the provider to initialize a new Metrics object", func() {
			metrics := etcdraft.NewMetrics(fakeProvider)

			Expect(metrics).NotTo(BeNil())
			Expect(fakeProvider.NewGaugeCallCount()).To(Equal(4))
			Expect(fakeProvider.NewCounterCallCount()).To(Equal(2))

			Expect(metrics.ClusterSize).To(Equal(fakeGauge))
			Expect(metrics.IsLeader).To(Equal(fakeGauge))
			Expect(metrics.CommittedBlockNumber).To(Equal(fakeGauge))
			Expect(metrics.SnapshotBlockNumber).To(Equal(fakeGauge))
			Expect(metrics.LeaderChanges).To(Equal(fakeCounter))
			Expect(metrics.ProposalFailures).To(Equal(fakeCounter))
		})
	})
})
