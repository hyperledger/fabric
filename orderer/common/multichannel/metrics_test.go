/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/orderer/common/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics", func() {
	Context("NewMetrics", func() {
		var (
			fakeProvider *metricsfakes.Provider
			fakeGauge    *metricsfakes.Gauge
		)

		BeforeEach(func() {
			fakeProvider = &metricsfakes.Provider{}
			fakeGauge = &metricsfakes.Gauge{}

			fakeProvider.NewGaugeReturns(fakeGauge)
		})

		It("uses the provider to initialize a new Metrics object", func() {
			metrics := NewMetrics(fakeProvider)

			Expect(metrics).NotTo(BeNil())
			Expect(fakeProvider.NewGaugeCallCount()).To(Equal(2))

			Expect(metrics.Relation).To(Equal(fakeGauge))
			Expect(metrics.Status).To(Equal(fakeGauge))
		})
	})

	Context("reportStatus", func() {
		var (
			fakeProvider *metricsfakes.Provider
			fakeGauge    *metricsfakes.Gauge
			metrics      *Metrics
		)

		BeforeEach(func() {
			fakeProvider = &metricsfakes.Provider{}
			fakeGauge = &metricsfakes.Gauge{}

			fakeProvider.NewGaugeReturns(fakeGauge)
			fakeGauge.WithReturns(fakeGauge)

			metrics = NewMetrics(fakeProvider)
			Expect(metrics.Status).To(Equal(fakeGauge))
		})

		It("reports status inactive as a gauge", func() {
			metrics.reportStatus("fake-channel", types.StatusInactive)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(0)))
		})

		It("reports status active as a gauge", func() {
			metrics.reportStatus("fake-channel", types.StatusActive)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(1)))
		})

		It("reports status onboarding as a gauge", func() {
			metrics.reportStatus("fake-channel", types.StatusOnBoarding)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(2)))
		})

		It("panics when reporting an unknown cluster status", func() {
			Expect(func() { metrics.reportStatus("fake-channel", "unknown") }).To(Panic())
		})
	})

	Context("reportRelation", func() {
		var (
			fakeProvider *metricsfakes.Provider
			fakeGauge    *metricsfakes.Gauge
			metrics      *Metrics
		)

		BeforeEach(func() {
			fakeProvider = &metricsfakes.Provider{}
			fakeGauge = &metricsfakes.Gauge{}

			fakeProvider.NewGaugeReturns(fakeGauge)
			fakeGauge.WithReturns(fakeGauge)

			metrics = NewMetrics(fakeProvider)
			Expect(metrics.Relation).To(Equal(fakeGauge))
		})

		It("reports relation none as a gauge", func() {
			metrics.reportRelation("fake-channel", types.ClusterRelationNone)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(0)))
		})

		It("reports relation consenter as a gauge", func() {
			metrics.reportRelation("fake-channel", types.ClusterRelationConsenter)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(1)))
		})

		It("reports relation follower as a gauge", func() {
			metrics.reportRelation("fake-channel", types.ClusterRelationFollower)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(2)))
		})

		It("reports relation config-tracker as a gauge", func() {
			metrics.reportRelation("fake-channel", types.ClusterRelationConfigTracker)
			Expect(fakeGauge.WithCallCount()).To(Equal(1))
			Expect(fakeGauge.WithArgsForCall(0)).To(Equal([]string{"channel", "fake-channel"}))
			Expect(fakeGauge.SetCallCount()).To(Equal(1))
			Expect(fakeGauge.SetArgsForCall(0)).To(Equal(float64(3)))
		})

		It("panics when reporting an unknown relation", func() {
			Expect(func() { metrics.reportRelation("fake-channel", "unknown") }).To(Panic())
		})
	})
})

func newFakeMetrics(fakeFields *fakeMetricsFields) *Metrics {
	return &Metrics{
		Relation: fakeFields.fakeRelation,
		Status:   fakeFields.fakeStatus,
	}
}

type fakeMetricsFields struct {
	fakeRelation *metricsfakes.Gauge
	fakeStatus   *metricsfakes.Gauge
}

func newFakeMetricsFields() *fakeMetricsFields {
	return &fakeMetricsFields{
		fakeRelation: newFakeGauge(),
		fakeStatus:   newFakeGauge(),
	}
}

func newFakeGauge() *metricsfakes.Gauge {
	fakeGauge := &metricsfakes.Gauge{}
	fakeGauge.WithReturns(fakeGauge)
	return fakeGauge
}
