/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package disabled_test

import (
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provider", func() {
	var p metrics.Provider

	BeforeEach(func() {
		p = &disabled.Provider{}
	})

	Describe("NewCounter", func() {
		It("creates a no-op counter that doesn't blow up", func() {
			c := p.NewCounter(metrics.CounterOpts{})
			Expect(c).NotTo(BeNil())

			c.Add(1)
			c.With("whatever").Add(2)
		})
	})

	Describe("NewGauge", func() {
		It("creates a no-op gauge that doesn't blow up", func() {
			g := p.NewGauge(metrics.GaugeOpts{})
			Expect(g).NotTo(BeNil())

			g.Set(1)
			g.Add(1)
			g.With("whatever").Set(2)
			g.With("whatever").Add(2)
		})
	})

	Describe("NewHistogram", func() {
		It("creates a no-op histogram that doesn't blow up", func() {
			h := p.NewHistogram(metrics.HistogramOpts{})
			Expect(h).NotTo(BeNil())

			h.Observe(1)
			h.With("whatever").Observe(2)
		})
	})
})
