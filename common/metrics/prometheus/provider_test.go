/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	commonmetrics "github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	prom "github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Provider", func() {
	var (
		server *httptest.Server
		client *http.Client
		p      *prometheus.Provider
	)

	BeforeEach(func() {
		// Note: These tests can't be run in parallel because go-kit uses
		// the global registry to manage metrics. This is something to revisit
		// in the future.
		registry := prom.NewRegistry()
		prom.DefaultRegisterer = registry
		prom.DefaultGatherer = registry

		server = httptest.NewServer(prom.UninstrumentedHandler())
		client = server.Client()

		p = &prometheus.Provider{}
	})

	AfterEach(func() {
		server.Close()
	})

	It("implements metrics.Provider", func() {
		var p commonmetrics.Provider = &prometheus.Provider{}
		Expect(p).NotTo(BeNil())
	})

	Describe("NewCounter", func() {
		var counterOpts commonmetrics.CounterOpts

		BeforeEach(func() {
			counterOpts = commonmetrics.CounterOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "counter_name",
				Help:       "This is some help text for the counter",
				LabelNames: []string{"alpha", "beta"},
			}
		})

		It("creates counters that support labels", func() {
			counter := p.NewCounter(counterOpts)
			counter.With("alpha", "a", "beta", "b").Add(1)
			counter.With("alpha", "aardvark", "beta", "b").Add(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			bytes, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_counter_name This is some help text for the counter`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_counter_name counter`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="aardvark",beta="b"} 2`))
		})

		Context("when the counter is defined without labels", func() {
			BeforeEach(func() {
				counterOpts.LabelNames = nil
			})

			It("With does not need to be called", func() {
				counter := p.NewCounter(counterOpts)
				counter.Add(1)

				resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				bytes, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name 1`))
			})
		})
	})

	Describe("NewGauge", func() {
		var gaugeOpts commonmetrics.GaugeOpts

		BeforeEach(func() {
			gaugeOpts = commonmetrics.GaugeOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "gauge_name",
				Help:       "This is some help text for the gauge",
				LabelNames: []string{"alpha", "beta"},
			}
		})

		It("creates gauges that support labels", func() {
			gauge := p.NewGauge(gaugeOpts)
			gauge.With("alpha", "a", "beta", "b").Add(1)
			gauge.With("alpha", "a", "beta", "b").Add(1)
			gauge.With("alpha", "aardvark", "beta", "b").Add(1)
			gauge.With("alpha", "aardvark", "beta", "bob").Set(99)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			bytes, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_gauge_name This is some help text for the gauge`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_gauge_name gauge`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="a",beta="b"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="aardvark",beta="b"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="aardvark",beta="bob"} 99`))
		})
	})

	Describe("NewHistogram", func() {
		var histogramOpts commonmetrics.HistogramOpts

		BeforeEach(func() {
			histogramOpts = commonmetrics.HistogramOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "histogram_name",
				Help:       "This is some help text for the gauge",
				LabelNames: []string{"alpha", "beta"},
			}
		})

		It("creates histogram that support labels", func() {
			histogram := p.NewHistogram(histogramOpts)
			for _, limit := range prom.DefBuckets {
				histogram.With("alpha", "a", "beta", "b").Observe(limit)
			}
			histogram.With("alpha", "a", "beta", "b").Observe(prom.DefBuckets[len(prom.DefBuckets)-1] + 1)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			bytes, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_histogram_name This is some help text for the gauge`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_histogram_name histogram`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.005"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.01"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.025"} 3`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.05"} 4`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.1"} 5`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.25"} 6`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.5"} 7`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="1"} 8`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="2.5"} 9`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="5"} 10`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="10"} 11`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="+Inf"} 12`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_sum{alpha="a",beta="b"} `))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_count{alpha="a",beta="b"} 12`))
		})

		It("creates histogram with buckets that support labels", func() {
			histogramOpts.Buckets = []float64{1, 5}
			histogram := p.NewHistogram(histogramOpts)

			histogram.With("alpha", "a", "beta", "b").Observe(0.5)
			histogram.With("alpha", "a", "beta", "b").Observe(4.5)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			bytes, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_histogram_name This is some help text for the gauge`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_histogram_name histogram`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="1"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="5"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_sum{alpha="a",beta="b"} 5`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_count{alpha="a",beta="b"} 2`))
		})
	})

	// This helps ensure the label cardinality behavior matches what was implemented
	// for statsd. If these tests fail, correspending updates will be needed in the label
	// processing used for statsd.
	Describe("edge case behavior", func() {
		var counterOpts commonmetrics.CounterOpts

		BeforeEach(func() {
			counterOpts = commonmetrics.CounterOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "counter_name",
				Help:       "This is some help text for the counter",
				LabelNames: []string{"alpha", "beta"},
			}
		})
		Context("when With is called without a label value", func() {
			It("uses unknown for the missing value", func() {
				counter := p.NewCounter(counterOpts)
				counter.With("alpha", "a", "beta").Add(1)
				resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				bytes, err := ioutil.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_counter_name This is some help text for the counter`))
				Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_counter_name counter`))
				Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="unknown"} 1`))
			})
		})

		Context("when With is called with an extra label", func() {
			It("panics", func() {
				counter := p.NewCounter(counterOpts)
				panicMessage := func() (panicMessage interface{}) {
					defer func() { panicMessage = recover() }()
					counter.With("alpha", "a", "beta", "b", "charlie", "c").Add(1)
					return
				}()
				Expect(panicMessage).To(MatchError("inconsistent label cardinality"))
			})
		})

		Context("when label values are not provided", func() {
			It("it panics with a cardinaility message", func() {
				counter := p.NewCounter(counterOpts)
				panicMessage := func() (panicMessage interface{}) {
					defer func() { panicMessage = recover() }()
					counter.Add(1)
					return
				}()
				Expect(panicMessage).To(MatchError("inconsistent label cardinality"))
			})
		})
	})
})
