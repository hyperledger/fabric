/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kafka_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger/fabric/orderer/consensus/kafka"
	"github.com/hyperledger/fabric/orderer/consensus/kafka/mock"
)

var _ = Describe("Metrics", func() {
	var (
		fakeMetricsRegistry *mock.MetricsRegistry
	)

	BeforeEach(func() {
		fakeMetricsRegistry = &mock.MetricsRegistry{}
	})

	Describe("NewMetrics", func() {
		var (
			fakeMetricsProvider *mock.MetricsProvider
		)

		BeforeEach(func() {
			fakeMetricsProvider = &mock.MetricsProvider{}
		})

		It("initializes a new set of metrics", func() {
			m := kafka.NewMetrics(fakeMetricsProvider, fakeMetricsRegistry)
			Expect(m).NotTo(BeNil())
			Expect(fakeMetricsProvider.NewGaugeCallCount()).To(Equal(12))
			Expect(m.GoMetricsRegistry).To(Equal(fakeMetricsRegistry))
		})
	})

	Describe("PollGoMetrics", func() {
		var (
			// The fake gometrics sources
			fakeIncomingByteRateMeter      *mock.MetricsMeter
			fakeOutgoingByteRateMeter      *mock.MetricsMeter
			fakeRequestRateMeter           *mock.MetricsMeter
			fakeRequestSizeHistogram       *mock.MetricsHistogram
			fakeRequestLatencyHistogram    *mock.MetricsHistogram
			fakeResponseRateMeter          *mock.MetricsMeter
			fakeResponseSizeHistogram      *mock.MetricsHistogram
			fakeBatchSizeHistogram         *mock.MetricsHistogram
			fakeRecordSendRateMeter        *mock.MetricsMeter
			fakeRecordsPerRequestHistogram *mock.MetricsHistogram
			fakeCompressionRatioHistogram  *mock.MetricsHistogram

			// The fake go-kit metric sinks
			fakeIncomingByteRateGauge  *mock.MetricsGauge
			fakeOutgoingByteRateGauge  *mock.MetricsGauge
			fakeRequestRateGauge       *mock.MetricsGauge
			fakeRequestSizeGauge       *mock.MetricsGauge
			fakeRequestLatencyGauge    *mock.MetricsGauge
			fakeResponseRateGauge      *mock.MetricsGauge
			fakeResponseSizeGauge      *mock.MetricsGauge
			fakeBatchSizeGauge         *mock.MetricsGauge
			fakeRecordSendRateGauge    *mock.MetricsGauge
			fakeRecordsPerRequestGauge *mock.MetricsGauge
			fakeCompressionRatioGauge  *mock.MetricsGauge

			m *kafka.Metrics
		)

		BeforeEach(func() {
			fakeIncomingByteRateMeter = &mock.MetricsMeter{}
			fakeIncomingByteRateMeter.SnapshotReturns(fakeIncomingByteRateMeter)
			fakeIncomingByteRateMeter.Rate1Returns(1)

			fakeOutgoingByteRateMeter = &mock.MetricsMeter{}
			fakeOutgoingByteRateMeter.SnapshotReturns(fakeOutgoingByteRateMeter)
			fakeOutgoingByteRateMeter.Rate1Returns(2)

			fakeRequestRateMeter = &mock.MetricsMeter{}
			fakeRequestRateMeter.SnapshotReturns(fakeRequestRateMeter)
			fakeRequestRateMeter.Rate1Returns(3)

			fakeRequestSizeHistogram = &mock.MetricsHistogram{}
			fakeRequestSizeHistogram.SnapshotReturns(fakeRequestSizeHistogram)
			fakeRequestSizeHistogram.MeanReturns(4)

			fakeRequestLatencyHistogram = &mock.MetricsHistogram{}
			fakeRequestLatencyHistogram.SnapshotReturns(fakeRequestLatencyHistogram)
			fakeRequestLatencyHistogram.MeanReturns(5)

			fakeResponseRateMeter = &mock.MetricsMeter{}
			fakeResponseRateMeter.SnapshotReturns(fakeResponseRateMeter)
			fakeResponseRateMeter.Rate1Returns(6)

			fakeResponseSizeHistogram = &mock.MetricsHistogram{}
			fakeResponseSizeHistogram.SnapshotReturns(fakeResponseSizeHistogram)
			fakeResponseSizeHistogram.MeanReturns(7)

			fakeBatchSizeHistogram = &mock.MetricsHistogram{}
			fakeBatchSizeHistogram.SnapshotReturns(fakeBatchSizeHistogram)
			fakeBatchSizeHistogram.MeanReturns(8)

			fakeRecordSendRateMeter = &mock.MetricsMeter{}
			fakeRecordSendRateMeter.SnapshotReturns(fakeRecordSendRateMeter)
			fakeRecordSendRateMeter.Rate1Returns(9)

			fakeRecordsPerRequestHistogram = &mock.MetricsHistogram{}
			fakeRecordsPerRequestHistogram.SnapshotReturns(fakeRecordsPerRequestHistogram)
			fakeRecordsPerRequestHistogram.MeanReturns(10)

			fakeCompressionRatioHistogram = &mock.MetricsHistogram{}
			fakeCompressionRatioHistogram.SnapshotReturns(fakeCompressionRatioHistogram)
			fakeCompressionRatioHistogram.MeanReturns(11)

			fakeMetricsRegistry.EachStub = func(receiver func(name string, value interface{})) {
				receiver("incoming-byte-rate-for-broker-0", fakeIncomingByteRateMeter)
				receiver("incoming-byte-rate-for-broker-1", fakeIncomingByteRateMeter)
				receiver("outgoing-byte-rate-for-broker-0", fakeOutgoingByteRateMeter)
				receiver("outgoing-byte-rate-for-broker-1", fakeOutgoingByteRateMeter)
				receiver("request-rate-for-broker-0", fakeRequestRateMeter)
				receiver("request-size-for-broker-0", fakeRequestSizeHistogram)
				receiver("request-latency-in-ms-for-broker-0", fakeRequestLatencyHistogram)
				receiver("response-rate-for-broker-0", fakeResponseRateMeter)
				receiver("response-size-for-broker-0", fakeResponseSizeHistogram)
				receiver("batch-size-for-topic-mytopic", fakeBatchSizeHistogram)
				receiver("record-send-rate-for-topic-mytopic", fakeRecordSendRateMeter)
				receiver("records-per-request-for-topic-mytopic", fakeRecordsPerRequestHistogram)
				receiver("compression-ratio-for-topic-mytopic", fakeCompressionRatioHistogram)
			}

			fakeIncomingByteRateGauge = &mock.MetricsGauge{}
			fakeIncomingByteRateGauge.WithReturns(fakeIncomingByteRateGauge)

			fakeOutgoingByteRateGauge = &mock.MetricsGauge{}
			fakeOutgoingByteRateGauge.WithReturns(fakeOutgoingByteRateGauge)

			fakeRequestRateGauge = &mock.MetricsGauge{}
			fakeRequestRateGauge.WithReturns(fakeRequestRateGauge)

			fakeRequestSizeGauge = &mock.MetricsGauge{}
			fakeRequestSizeGauge.WithReturns(fakeRequestSizeGauge)

			fakeRequestLatencyGauge = &mock.MetricsGauge{}
			fakeRequestLatencyGauge.WithReturns(fakeRequestLatencyGauge)

			fakeResponseRateGauge = &mock.MetricsGauge{}
			fakeResponseRateGauge.WithReturns(fakeResponseRateGauge)

			fakeResponseSizeGauge = &mock.MetricsGauge{}
			fakeResponseSizeGauge.WithReturns(fakeResponseSizeGauge)

			fakeBatchSizeGauge = &mock.MetricsGauge{}
			fakeBatchSizeGauge.WithReturns(fakeBatchSizeGauge)

			fakeRecordSendRateGauge = &mock.MetricsGauge{}
			fakeRecordSendRateGauge.WithReturns(fakeRecordSendRateGauge)

			fakeRecordsPerRequestGauge = &mock.MetricsGauge{}
			fakeRecordsPerRequestGauge.WithReturns(fakeRecordsPerRequestGauge)

			fakeCompressionRatioGauge = &mock.MetricsGauge{}
			fakeCompressionRatioGauge.WithReturns(fakeCompressionRatioGauge)

			m = &kafka.Metrics{
				IncomingByteRate:  fakeIncomingByteRateGauge,
				OutgoingByteRate:  fakeOutgoingByteRateGauge,
				RequestRate:       fakeRequestRateGauge,
				RequestSize:       fakeRequestSizeGauge,
				RequestLatency:    fakeRequestLatencyGauge,
				ResponseRate:      fakeResponseRateGauge,
				ResponseSize:      fakeResponseSizeGauge,
				BatchSize:         fakeBatchSizeGauge,
				RecordSendRate:    fakeRecordSendRateGauge,
				RecordsPerRequest: fakeRecordsPerRequestGauge,
				CompressionRatio:  fakeCompressionRatioGauge,

				GoMetricsRegistry: fakeMetricsRegistry,
			}
		})

		It("converts the metrics in the registry to be gauages", func() {
			m.PollGoMetrics()

			Expect(fakeIncomingByteRateGauge.WithCallCount()).To(Equal(2))
			Expect(fakeIncomingByteRateGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeIncomingByteRateGauge.WithArgsForCall(1)).To(Equal([]string{"broker_id", "1"}))
			Expect(fakeIncomingByteRateGauge.SetCallCount()).To(Equal(2))
			Expect(fakeIncomingByteRateGauge.SetArgsForCall(0)).To(Equal(float64(1)))

			Expect(fakeOutgoingByteRateGauge.WithCallCount()).To(Equal(2))
			Expect(fakeOutgoingByteRateGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeOutgoingByteRateGauge.WithArgsForCall(1)).To(Equal([]string{"broker_id", "1"}))
			Expect(fakeOutgoingByteRateGauge.SetCallCount()).To(Equal(2))
			Expect(fakeOutgoingByteRateGauge.SetArgsForCall(0)).To(Equal(float64(2)))

			Expect(fakeRequestRateGauge.WithCallCount()).To(Equal(1))
			Expect(fakeRequestRateGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeRequestRateGauge.SetCallCount()).To(Equal(1))
			Expect(fakeRequestRateGauge.SetArgsForCall(0)).To(Equal(float64(3)))

			Expect(fakeRequestSizeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeRequestSizeGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeRequestSizeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeRequestSizeGauge.SetArgsForCall(0)).To(Equal(float64(4)))

			Expect(fakeRequestLatencyGauge.WithCallCount()).To(Equal(1))
			Expect(fakeRequestLatencyGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeRequestLatencyGauge.SetCallCount()).To(Equal(1))
			Expect(fakeRequestLatencyGauge.SetArgsForCall(0)).To(Equal(float64(5)))

			Expect(fakeResponseRateGauge.WithCallCount()).To(Equal(1))
			Expect(fakeResponseRateGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeResponseRateGauge.SetCallCount()).To(Equal(1))
			Expect(fakeResponseRateGauge.SetArgsForCall(0)).To(Equal(float64(6)))

			Expect(fakeResponseSizeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeResponseSizeGauge.WithArgsForCall(0)).To(Equal([]string{"broker_id", "0"}))
			Expect(fakeResponseSizeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeResponseSizeGauge.SetArgsForCall(0)).To(Equal(float64(7)))

			Expect(fakeBatchSizeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeBatchSizeGauge.WithArgsForCall(0)).To(Equal([]string{"topic", "mytopic"}))
			Expect(fakeBatchSizeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeBatchSizeGauge.SetArgsForCall(0)).To(Equal(float64(8)))

			Expect(fakeRecordSendRateGauge.WithCallCount()).To(Equal(1))
			Expect(fakeRecordSendRateGauge.WithArgsForCall(0)).To(Equal([]string{"topic", "mytopic"}))
			Expect(fakeRecordSendRateGauge.SetCallCount()).To(Equal(1))
			Expect(fakeRecordSendRateGauge.SetArgsForCall(0)).To(Equal(float64(9)))

			Expect(fakeRecordsPerRequestGauge.WithCallCount()).To(Equal(1))
			Expect(fakeRecordsPerRequestGauge.WithArgsForCall(0)).To(Equal([]string{"topic", "mytopic"}))
			Expect(fakeRecordsPerRequestGauge.SetCallCount()).To(Equal(1))
			Expect(fakeRecordsPerRequestGauge.SetArgsForCall(0)).To(Equal(float64(10)))

			Expect(fakeCompressionRatioGauge.WithCallCount()).To(Equal(1))
			Expect(fakeCompressionRatioGauge.WithArgsForCall(0)).To(Equal([]string{"topic", "mytopic"}))
			Expect(fakeCompressionRatioGauge.SetCallCount()).To(Equal(1))
			Expect(fakeCompressionRatioGauge.SetArgsForCall(0)).To(Equal(float64(11)))
		})

		Context("when the go-metrics source contains unknown metrics", func() {
			var (
				fakeMeter *mock.MetricsMeter
			)

			BeforeEach(func() {
				fakeMeter = &mock.MetricsMeter{}
				fakeMetricsRegistry.EachStub = func(receiver func(name string, value interface{})) {
					receiver("unknown-metric-name", fakeMeter)
					receiver("another-unknown-metric", fakeMeter)
				}
			})

			It("does nothing with them", func() {
				m.PollGoMetrics()
				Expect(fakeIncomingByteRateGauge.SetCallCount()).To(Equal(0))
				Expect(fakeOutgoingByteRateGauge.SetCallCount()).To(Equal(0))
				Expect(fakeRequestRateGauge.SetCallCount()).To(Equal(0))
				Expect(fakeRequestSizeGauge.SetCallCount()).To(Equal(0))
				Expect(fakeRequestLatencyGauge.SetCallCount()).To(Equal(0))
				Expect(fakeResponseRateGauge.SetCallCount()).To(Equal(0))
				Expect(fakeResponseSizeGauge.SetCallCount()).To(Equal(0))
				Expect(fakeBatchSizeGauge.SetCallCount()).To(Equal(0))
				Expect(fakeRecordSendRateGauge.SetCallCount()).To(Equal(0))
				Expect(fakeRecordsPerRequestGauge.SetCallCount()).To(Equal(0))
				Expect(fakeCompressionRatioGauge.SetCallCount()).To(Equal(0))

				Expect(fakeMeter.SnapshotCallCount()).To(Equal(0))
			})
		})

		Context("when a histogram metric does not have a histogram value", func() {
			var (
				fakeMeter *mock.MetricsMeter
			)

			BeforeEach(func() {
				fakeMeter = &mock.MetricsMeter{}
				fakeMetricsRegistry.EachStub = func(receiver func(name string, value interface{})) {
					receiver("request-size-for-broker-0", fakeMeter)
				}
			})

			It("panics", func() {
				Expect(m.PollGoMetrics).To(Panic())
				Expect(fakeRequestSizeGauge.SetCallCount()).To(Equal(0))
				Expect(fakeMeter.SnapshotCallCount()).To(Equal(0))
			})
		})

		Context("when a meter metric does not have a meter value", func() {
			var (
				fakeHistogram *mock.MetricsHistogram
			)

			BeforeEach(func() {
				fakeHistogram = &mock.MetricsHistogram{}
				fakeMetricsRegistry.EachStub = func(receiver func(name string, value interface{})) {
					receiver("incoming-byte-rate-for-broker-0", fakeHistogram)
				}
			})

			It("panics", func() {
				Expect(m.PollGoMetrics).To(Panic())
				Expect(fakeIncomingByteRateGauge.SetCallCount()).To(Equal(0))
				Expect(fakeHistogram.SnapshotCallCount()).To(Equal(0))
			})
		})
	})

	Describe("PollGoMetricsUntilStop", func() {
		var (
			m           *kafka.Metrics
			stopChannel chan struct{}
			iterations  int
		)

		BeforeEach(func() {
			fakeMetricsRegistry.EachStub = func(func(string, interface{})) {
				iterations++
				if iterations > 5 {
					close(stopChannel)
				}
			}
			m = &kafka.Metrics{
				GoMetricsRegistry: fakeMetricsRegistry,
			}
			stopChannel = make(chan struct{})
		})

		It("polls the go metrics until the stop channel closes", func() {
			m.PollGoMetricsUntilStop(time.Millisecond, stopChannel)
			Expect(iterations).To(BeNumerically(">", 5))
		})
	})
})
