/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	gmetrics "github.com/hyperledger/fabric/gossip/metrics"
)

type TestMetricProvider struct {
	FakeProvider *metricsfakes.Provider

	FakeHeightGauge            *metricsfakes.Gauge
	FakeCommitDurationHist     *metricsfakes.Histogram
	FakePayloadBufferSizeGauge *metricsfakes.Gauge

	FakeDeclarationGauge *metricsfakes.Gauge

	FakeSentMessages     *metricsfakes.Counter
	FakeBufferOverflow   *metricsfakes.Counter
	FakeReceivedMessages *metricsfakes.Counter

	FakeTotalGauge *metricsfakes.Gauge

	FakeValidationDuration             *metricsfakes.Histogram
	FakeListMissingPrivateDataDuration *metricsfakes.Histogram
	FakeFetchDuration                  *metricsfakes.Histogram
	FakeCommitPrivateDataDuration      *metricsfakes.Histogram
	FakePurgeDuration                  *metricsfakes.Histogram
	FakeSendDuration                   *metricsfakes.Histogram
	FakeReconciliationDuration         *metricsfakes.Histogram
	FakePullDuration                   *metricsfakes.Histogram
	FakeRetrieveDuration               *metricsfakes.Histogram
}

func TestUtilConstructMetricProvider() *TestMetricProvider {
	fakeProvider := &metricsfakes.Provider{}

	fakeHeightGauge := testUtilConstructGauge()
	fakeCommitDurationHist := testUtilConstructHist()
	fakePayloadBufferSizeGauge := testUtilConstructGauge()

	fakeDeclarationGauge := testUtilConstructGauge()

	fakeSentMessages := testUtilConstructCounter()
	fakeBufferOverflow := testUtilConstructCounter()
	fakeReceivedMessages := testUtilConstructCounter()

	fakeTotalGauge := testUtilConstructGauge()

	fakeValidationDuration := testUtilConstructHist()
	fakeListMissingPrivateDataDuration := testUtilConstructHist()
	fakeFetchDuration := testUtilConstructHist()
	fakeCommitPrivateDataDuration := testUtilConstructHist()
	fakePurgeDuration := testUtilConstructHist()
	fakeSendDuration := testUtilConstructHist()
	fakeReconciliationDuration := testUtilConstructHist()
	fakePullDuration := testUtilConstructHist()
	fakeRetrieveDuration := testUtilConstructHist()

	fakeProvider.NewCounterStub = func(opts metrics.CounterOpts) metrics.Counter {
		switch opts.Name {
		case gmetrics.BufferOverflowOpts.Name:
			return fakeBufferOverflow
		case gmetrics.SentMessagesOpts.Name:
			return fakeSentMessages
		case gmetrics.ReceivedMessagesOpts.Name:
			return fakeReceivedMessages
		}
		return nil
	}
	fakeProvider.NewHistogramStub = func(opts metrics.HistogramOpts) metrics.Histogram {
		switch opts.Name {
		case gmetrics.CommitDurationOpts.Name:
			return fakeCommitDurationHist
		case gmetrics.ValidationDurationOpts.Name:
			return fakeValidationDuration
		case gmetrics.ListMissingPrivateDataDurationOpts.Name:
			return fakeListMissingPrivateDataDuration
		case gmetrics.FetchDurationOpts.Name:
			return fakeFetchDuration
		case gmetrics.CommitPrivateDataDurationOpts.Name:
			return fakeCommitPrivateDataDuration
		case gmetrics.PurgeDurationOpts.Name:
			return fakePurgeDuration
		case gmetrics.SendDurationOpts.Name:
			return fakeSendDuration
		case gmetrics.ReconciliationDurationOpts.Name:
			return fakeReconciliationDuration
		case gmetrics.PullDurationOpts.Name:
			return fakePullDuration
		case gmetrics.RetrieveDurationOpts.Name:
			return fakeRetrieveDuration
		}
		return nil
	}
	fakeProvider.NewGaugeStub = func(opts metrics.GaugeOpts) metrics.Gauge {
		switch opts.Name {
		case gmetrics.PayloadBufferSizeOpts.Name:
			return fakePayloadBufferSizeGauge
		case gmetrics.HeightOpts.Name:
			return fakeHeightGauge
		case gmetrics.LeaderDeclerationOpts.Name:
			return fakeDeclarationGauge
		case gmetrics.TotalOpts.Name:
			return fakeTotalGauge
		}
		return nil
	}

	return &TestMetricProvider{
		fakeProvider,
		fakeHeightGauge,
		fakeCommitDurationHist,
		fakePayloadBufferSizeGauge,
		fakeDeclarationGauge,
		fakeSentMessages,
		fakeBufferOverflow,
		fakeReceivedMessages,
		fakeTotalGauge,
		fakeValidationDuration,
		fakeListMissingPrivateDataDuration,
		fakeFetchDuration,
		fakeCommitPrivateDataDuration,
		fakePurgeDuration,
		fakeSendDuration,
		fakeReconciliationDuration,
		fakePullDuration,
		fakeRetrieveDuration,
	}
}

func testUtilConstructGauge() *metricsfakes.Gauge {
	fakeGauge := &metricsfakes.Gauge{}
	fakeGauge.WithReturns(fakeGauge)
	return fakeGauge
}

func testUtilConstructHist() *metricsfakes.Histogram {
	fakeHist := &metricsfakes.Histogram{}
	fakeHist.WithReturns(fakeHist)
	return fakeHist
}

func testUtilConstructCounter() *metricsfakes.Counter {
	fakeCounter := &metricsfakes.Counter{}
	fakeCounter.WithReturns(fakeCounter)
	return fakeCounter
}
