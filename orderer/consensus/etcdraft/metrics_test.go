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
			fakeProvider  *metricsfakes.Provider
			fakeGauge     *metricsfakes.Gauge
			fakeCounter   *metricsfakes.Counter
			fakeHistogram *metricsfakes.Histogram
		)

		BeforeEach(func() {
			fakeProvider = &metricsfakes.Provider{}
			fakeGauge = &metricsfakes.Gauge{}
			fakeCounter = &metricsfakes.Counter{}
			fakeHistogram = &metricsfakes.Histogram{}

			fakeProvider.NewGaugeReturns(fakeGauge)
			fakeProvider.NewCounterReturns(fakeCounter)
			fakeProvider.NewHistogramReturns(fakeHistogram)
		})

		It("uses the provider to initialize a new Metrics object", func() {
			metrics := etcdraft.NewMetrics(fakeProvider)

			Expect(metrics).NotTo(BeNil())
			Expect(fakeProvider.NewGaugeCallCount()).To(Equal(4))
			Expect(fakeProvider.NewCounterCallCount()).To(Equal(4))
			Expect(fakeProvider.NewHistogramCallCount()).To(Equal(1))

			Expect(metrics.ClusterSize).To(Equal(fakeGauge))
			Expect(metrics.IsLeader).To(Equal(fakeGauge))
			Expect(metrics.CommittedBlockNumber).To(Equal(fakeGauge))
			Expect(metrics.SnapshotBlockNumber).To(Equal(fakeGauge))
			Expect(metrics.LeaderChanges).To(Equal(fakeCounter))
			Expect(metrics.ProposalFailures).To(Equal(fakeCounter))
			Expect(metrics.DataPersistDuration).To(Equal(fakeHistogram))
			Expect(metrics.NormalProposalsReceived).To(Equal(fakeCounter))
			Expect(metrics.ConfigProposalsReceived).To(Equal(fakeCounter))
		})
	})
})

func newFakeMetrics(fakeFields *fakeMetricsFields) *etcdraft.Metrics {
	return &etcdraft.Metrics{
		ClusterSize:             fakeFields.fakeClusterSize,
		IsLeader:                fakeFields.fakeIsLeader,
		CommittedBlockNumber:    fakeFields.fakeCommittedBlockNumber,
		SnapshotBlockNumber:     fakeFields.fakeSnapshotBlockNumber,
		LeaderChanges:           fakeFields.fakeLeaderChanges,
		ProposalFailures:        fakeFields.fakeProposalFailures,
		DataPersistDuration:     fakeFields.fakeDataPersistDuration,
		NormalProposalsReceived: fakeFields.fakeNormalProposalsReceived,
		ConfigProposalsReceived: fakeFields.fakeConfigProposalsReceived,
	}
}

type fakeMetricsFields struct {
	fakeClusterSize             *metricsfakes.Gauge
	fakeIsLeader                *metricsfakes.Gauge
	fakeCommittedBlockNumber    *metricsfakes.Gauge
	fakeSnapshotBlockNumber     *metricsfakes.Gauge
	fakeLeaderChanges           *metricsfakes.Counter
	fakeProposalFailures        *metricsfakes.Counter
	fakeDataPersistDuration     *metricsfakes.Histogram
	fakeNormalProposalsReceived *metricsfakes.Counter
	fakeConfigProposalsReceived *metricsfakes.Counter
}

func newFakeMetricsFields() *fakeMetricsFields {
	return &fakeMetricsFields{
		fakeClusterSize:             newFakeGauge(),
		fakeIsLeader:                newFakeGauge(),
		fakeCommittedBlockNumber:    newFakeGauge(),
		fakeSnapshotBlockNumber:     newFakeGauge(),
		fakeLeaderChanges:           newFakeCounter(),
		fakeProposalFailures:        newFakeCounter(),
		fakeDataPersistDuration:     newFakeHistogram(),
		fakeNormalProposalsReceived: newFakeCounter(),
		fakeConfigProposalsReceived: newFakeCounter(),
	}
}

func newFakeGauge() *metricsfakes.Gauge {
	fakeGauge := &metricsfakes.Gauge{}
	fakeGauge.WithReturns(fakeGauge)
	return fakeGauge
}

func newFakeCounter() *metricsfakes.Counter {
	fakeCounter := &metricsfakes.Counter{}
	fakeCounter.WithReturns(fakeCounter)
	return fakeCounter
}

func newFakeHistogram() *metricsfakes.Histogram {
	fakeHistogram := &metricsfakes.Histogram{}
	fakeHistogram.WithReturns(fakeHistogram)
	return fakeHistogram
}
