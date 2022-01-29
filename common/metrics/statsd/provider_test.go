/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statsd_test

import (
	"bytes"
	"fmt"
	"strings"

	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provider", func() {
	var (
		s        *kitstatsd.Statsd
		provider *statsd.Provider
	)

	BeforeEach(func() {
		s = kitstatsd.New("", nil)
		provider = &statsd.Provider{Statsd: s}
	})

	It("implements metrics.Provider", func() {
		var p metrics.Provider = &statsd.Provider{}
		Expect(p).NotTo(BeNil())
	})

	Describe("NewCounter", func() {
		var counterOpts metrics.CounterOpts

		BeforeEach(func() {
			counterOpts = metrics.CounterOpts{
				Namespace:    "namespace",
				Subsystem:    "subsystem",
				Name:         "name",
				StatsdFormat: "%{#namespace}.%{#subsystem}.%{#name}.%{alpha}.%{beta}",
				LabelNames:   []string{"alpha", "beta"},
			}
		})

		It("creates counters that include label values in bucket names", func() {
			counter := provider.NewCounter(counterOpts)
			for _, alpha := range []string{"x", "y", "z"} {
				counter.With("alpha", alpha, "beta", "b").Add(1)
				buf := &bytes.Buffer{}
				s.WriteTo(buf)
				Expect(buf.String()).To(Equal(fmt.Sprintf("namespace.subsystem.name.%s.b:%f|c\n", alpha, float64(1))))
			}
		})

		Context("when a statsd format is not provided", func() {
			BeforeEach(func() {
				counterOpts.LabelNames = nil
				counterOpts.StatsdFormat = ""
			})

			It("uses the default format", func() {
				counter := provider.NewCounter(counterOpts)
				counter.Add(1)

				buf := &bytes.Buffer{}
				s.WriteTo(buf)
				Expect(buf.String()).To(Equal("namespace.subsystem.name:1.000000|c\n"))
			})
		})

		Context("when label values are not specified in counter options", func() {
			BeforeEach(func() {
				counterOpts.LabelNames = nil
				counterOpts.StatsdFormat = "%{#namespace}.%{#subsystem}.%{#name}"
			})

			It("creates counters that do not require calls to With", func() {
				counter := provider.NewCounter(counterOpts)
				for i := 0; i < 10; i++ {
					counter.Add(float64(i))

					buf := &bytes.Buffer{}
					s.WriteTo(buf)
					Expect(buf.String()).To(Equal(fmt.Sprintf("namespace.subsystem.name:%f|c\n", float64(i))))
				}
			})
		})

		Context("when labels are specified and with is not called", func() {
			It("panics with an appropriate message when Add is called", func() {
				counter := provider.NewCounter(counterOpts)
				panicMessage := func() (panicMessage interface{}) {
					defer func() { panicMessage = recover() }()
					counter.Add(1)
					return
				}()
				Expect(panicMessage).To(Equal("label values must be provided by calling With"))
			})
		})
	})

	Describe("NewGauge", func() {
		var gaugeOpts metrics.GaugeOpts

		BeforeEach(func() {
			gaugeOpts = metrics.GaugeOpts{
				Namespace:    "namespace",
				Subsystem:    "subsystem",
				Name:         "name",
				StatsdFormat: "%{#namespace}.%{#subsystem}.%{#name}.%{alpha}.%{beta}",
				LabelNames:   []string{"alpha", "beta"},
			}
		})

		It("creates gauges that support Set with label values in bucket names", func() {
			gauge := provider.NewGauge(gaugeOpts)
			for i := 1; i <= 5; i++ {
				for _, alpha := range []string{"x", "y", "z"} {
					gauge.With("alpha", alpha, "beta", "b").Set(float64(i))
					buf := &bytes.Buffer{}
					s.WriteTo(buf)
					Expect(buf.String()).To(Equal(fmt.Sprintf("namespace.subsystem.name.%s.b:%f|g\n", alpha, float64(i))))
				}
			}
		})

		It("creates gauges that support Add with label values in bucket names", func() {
			gauge := provider.NewGauge(gaugeOpts)

			for _, alpha := range []string{"x", "y", "z"} {
				gauge.With("alpha", alpha, "beta", "b").Set(float64(1.0))
			}
			for _, alpha := range []string{"x", "y", "z"} {
				for i := 0; i < 5; i++ {
					gauge.With("alpha", alpha, "beta", "b").Add(float64(1.0))
				}
			}
			buf := &bytes.Buffer{}
			s.WriteTo(buf)
			Expect(strings.SplitN(buf.String(), "\n", -1)).To(ConsistOf(
				Equal("namespace.subsystem.name.x.b:6.000000|g"),
				Equal("namespace.subsystem.name.y.b:6.000000|g"),
				Equal("namespace.subsystem.name.z.b:6.000000|g"),
				Equal(""),
			))
		})

		Context("when a statsd format is not provided", func() {
			BeforeEach(func() {
				gaugeOpts.LabelNames = nil
				gaugeOpts.StatsdFormat = ""
			})

			It("uses the default format", func() {
				gauge := provider.NewGauge(gaugeOpts)
				gauge.Set(1)

				buf := &bytes.Buffer{}
				s.WriteTo(buf)
				Expect(buf.String()).To(Equal("namespace.subsystem.name:1.000000|g\n"))
			})
		})

		Context("when label values are not specified in gauge options", func() {
			BeforeEach(func() {
				gaugeOpts.LabelNames = nil
				gaugeOpts.StatsdFormat = "%{#namespace}.%{#subsystem}.%{#name}"
			})

			It("creates gauges that do not require calls to With", func() {
				gauge := provider.NewGauge(gaugeOpts)
				for i := 0; i < 10; i++ {
					gauge.Add(float64(i))

					buf := &bytes.Buffer{}
					s.WriteTo(buf)
					Expect(buf.String()).To(Equal(fmt.Sprintf("namespace.subsystem.name:%f|g\n", float64(i))))
				}
			})
		})

		Context("when labels are specified and with is not called", func() {
			It("panics with an appropriate message when Add is called", func() {
				gauge := provider.NewGauge(gaugeOpts)
				panicMessage := func() (panicMessage interface{}) {
					defer func() { panicMessage = recover() }()
					gauge.Add(float64(1))
					return
				}()
				Expect(panicMessage).To(Equal("label values must be provided by calling With"))
			})

			It("panics with an appropriate message when Set is called", func() {
				gauge := provider.NewGauge(gaugeOpts)
				panicMessage := func() (panicMessage interface{}) {
					defer func() { panicMessage = recover() }()
					gauge.Set(float64(1))
					return
				}()
				Expect(panicMessage).To(Equal("label values must be provided by calling With"))
			})
		})
	})

	Describe("NewHistogram", func() {
		var histogramOpts metrics.HistogramOpts

		BeforeEach(func() {
			histogramOpts = metrics.HistogramOpts{
				Namespace:    "namespace",
				Subsystem:    "subsystem",
				Name:         "name",
				StatsdFormat: "%{#namespace}.%{#subsystem}.%{#name}.%{alpha}.%{beta}",
				LabelNames:   []string{"alpha", "beta"},
			}
		})

		It("creates histograms that support Observe with label values in bucket names", func() {
			histogram := provider.NewHistogram(histogramOpts)
			for i := 1; i <= 5; i++ {
				for _, alpha := range []string{"x", "y", "z"} {
					histogram.With("alpha", alpha, "beta", "b").Observe(float64(i))
					buf := &bytes.Buffer{}
					s.WriteTo(buf)
					Expect(buf.String()).To(Equal(fmt.Sprintf("namespace.subsystem.name.%s.b:%f|ms\n", alpha, float64(i))))
				}
			}
		})

		Context("when a statsd format is not provided", func() {
			BeforeEach(func() {
				histogramOpts.LabelNames = nil
				histogramOpts.StatsdFormat = ""
			})

			It("uses the default format", func() {
				histogram := provider.NewHistogram(histogramOpts)
				histogram.Observe(1)

				buf := &bytes.Buffer{}
				s.WriteTo(buf)
				Expect(buf.String()).To(Equal("namespace.subsystem.name:1.000000|ms\n"))
			})
		})

		Context("when label values are not specified in counter options", func() {
			BeforeEach(func() {
				histogramOpts.LabelNames = nil
				histogramOpts.StatsdFormat = "%{#namespace}.%{#subsystem}.%{#name}"
			})

			It("creates histograms that do not require calls to With", func() {
				histogram := provider.NewHistogram(histogramOpts)
				for i := 0; i < 10; i++ {
					histogram.Observe(float64(i))

					buf := &bytes.Buffer{}
					s.WriteTo(buf)
					Expect(buf.String()).To(Equal(fmt.Sprintf("namespace.subsystem.name:%f|ms\n", float64(i))))
				}
			})
		})

		Context("when labels are specified and with is not called", func() {
			It("panics with an appropriate message when Observe is called", func() {
				histogram := provider.NewHistogram(histogramOpts)
				panicMessage := func() (panicMessage interface{}) {
					defer func() { panicMessage = recover() }()
					histogram.Observe(float64(1))
					return
				}()
				Expect(panicMessage).To(Equal("label values must be provided by calling With"))
			})
		})
	})
})
