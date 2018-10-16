/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statsd_test

import (
	"bytes"
	"fmt"

	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger/fabric/common/metrics/statsd"
	. "github.com/onsi/ginkgo"
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

	It("creates counters", func() {
		counter := provider.NewCounter("name")
		for i := 0; i < 10; i++ {
			counter.Add(float64(i))

			buf := &bytes.Buffer{}
			s.WriteTo(buf)
			Expect(buf.String()).To(Equal(fmt.Sprintf("name:%f|c\n", float64(i))))
		}
	})

	It("creates gauges", func() {
		gauge := provider.NewGauge("name")
		for i := 0; i < 10; i++ {
			gauge.Set(float64(i))

			buf := &bytes.Buffer{}
			s.WriteTo(buf)
			Expect(buf.String()).To(Equal(fmt.Sprintf("name:%f|g\n", float64(i))))
		}
	})

	It("creates histograms", func() {
		hist := provider.NewHistogram("name")
		for i := 0; i < 10; i++ {
			hist.Observe(float64(i))

			buf := &bytes.Buffer{}
			s.WriteTo(buf)
			Expect(buf.String()).To(Equal(fmt.Sprintf("name:%f|ms\n", float64(i))))
		}
	})
})
