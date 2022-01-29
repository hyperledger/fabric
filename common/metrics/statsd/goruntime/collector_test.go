/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package goruntime_test

import (
	"strings"
	"time"

	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/common/metrics/statsd/goruntime"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Collector", func() {
	var (
		fakeProvider *metricsfakes.Provider
		fakeGauges   map[string]*metricsfakes.Gauge

		collector *goruntime.Collector
	)

	BeforeEach(func() {
		fakeGauges = map[string]*metricsfakes.Gauge{}
		fakeProvider = &metricsfakes.Provider{}
		fakeProvider.NewGaugeStub = func(o metrics.GaugeOpts) metrics.Gauge {
			name := strings.Join([]string{o.Namespace, o.Subsystem, o.Name}, ".")
			_, ok := fakeGauges[name]
			if !ok {
				fakeGauges[name] = &metricsfakes.Gauge{}
			}
			return fakeGauges[name]
		}

		collector = goruntime.NewCollector(fakeProvider)
	})

	It("constructs a collector with the approriate gauges", func() {
		Expect(fakeProvider.NewGaugeCallCount()).To(Equal(27))
	})

	It("acquires runtime statistics", func() {
		// TODO: statistics are a bit difficult to test
		stats := goruntime.CollectStats()
		Expect(stats).NotTo(BeZero())
	})

	It("collects and publishes statistics", func() {
		ticks := make(chan time.Time, 3)
		ticks <- time.Now()
		ticks <- time.Now().Add(time.Second)
		ticks <- time.Now().Add(2 * time.Second)
		close(ticks)

		collector.CollectAndPublish(ticks)
		for _, gauge := range fakeGauges {
			Expect(gauge.SetCallCount()).To(Equal(3))
		}
	})

	Describe("Publish", func() {
		var stats goruntime.Stats

		BeforeEach(func() {
			stats = goruntime.Stats{}
		})

		It("publishes CgoCalls as go.cgo_calls", func() {
			stats.CgoCalls = 1
			collector.Publish(stats)

			Expect(fakeGauges["go..cgo_calls"]).NotTo(BeNil())
			Expect(fakeGauges["go..cgo_calls"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go..cgo_calls"].SetArgsForCall(0)).To(Equal(float64(1)))
		})

		It("publishes GoRoutines as go.goroutine_count", func() {
			stats.GoRoutines = 2
			collector.Publish(stats)

			Expect(fakeGauges["go..goroutine_count"]).NotTo(BeNil())
			Expect(fakeGauges["go..goroutine_count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go..goroutine_count"].SetArgsForCall(0)).To(Equal(float64(2)))
		})

		It("publishes ThreadsCreated as go.threads_created", func() {
			stats.ThreadsCreated = 3
			collector.Publish(stats)

			Expect(fakeGauges["go..threads_created"]).NotTo(BeNil())
			Expect(fakeGauges["go..threads_created"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go..threads_created"].SetArgsForCall(0)).To(Equal(float64(3)))
		})

		It("publishes HeapAlloc as go.mem.heap_alloc_bytes", func() {
			stats.MemStats.HeapAlloc = 4
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_alloc_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_alloc_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_alloc_bytes"].SetArgsForCall(0)).To(Equal(float64(4)))
		})

		It("publishes TotalAlloc as go.mem.heap.total_alloc_bytes", func() {
			stats.MemStats.TotalAlloc = 5
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_total_alloc_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_total_alloc_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_total_alloc_bytes"].SetArgsForCall(0)).To(Equal(float64(5)))
		})

		It("publishes Mallocs as go.mem.heap.heap_malloc_count", func() {
			stats.MemStats.Mallocs = 6
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_malloc_count"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_malloc_count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_malloc_count"].SetArgsForCall(0)).To(Equal(float64(6)))
		})

		It("publishes Frees as go.mem.heap_free_count", func() {
			stats.MemStats.Frees = 7
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_free_count"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_free_count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_free_count"].SetArgsForCall(0)).To(Equal(float64(7)))
		})

		It("publishes HeapSys as go.mem.heap_sys_bytes", func() {
			stats.MemStats.HeapSys = 8
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(8)))
		})

		It("publishes HeapSys as go.mem.heap_idle_bytes", func() {
			stats.MemStats.HeapIdle = 9
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_idle_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_idle_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_idle_bytes"].SetArgsForCall(0)).To(Equal(float64(9)))
		})

		It("publishes HeapInuse as go.mem.heap_inuse_bytes", func() {
			stats.MemStats.HeapInuse = 10
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_inuse_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_inuse_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_inuse_bytes"].SetArgsForCall(0)).To(Equal(float64(10)))
		})

		It("publishes HeapReleased as go.mem.heap_released_bytes", func() {
			stats.MemStats.HeapReleased = 11
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_released_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_released_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_released_bytes"].SetArgsForCall(0)).To(Equal(float64(11)))
		})

		It("publishes HeapObjects as go.mem.heap_objects", func() {
			stats.MemStats.HeapObjects = 12
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.heap_objects"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.heap_objects"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.heap_objects"].SetArgsForCall(0)).To(Equal(float64(12)))
		})

		It("publishes StackInuse as go.mem.stack_inuse_bytes", func() {
			stats.MemStats.StackInuse = 13
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.stack_inuse_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.stack_inuse_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.stack_inuse_bytes"].SetArgsForCall(0)).To(Equal(float64(13)))
		})

		It("publishes StackSys as go.mem.stack_sys_bytes", func() {
			stats.MemStats.StackSys = 14
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.stack_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.stack_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.stack_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(14)))
		})

		It("publishes MSpanInuse as go.mem.mspan_inuse_bytes", func() {
			stats.MemStats.MSpanInuse = 15
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.mspan_inuse_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.mspan_inuse_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.mspan_inuse_bytes"].SetArgsForCall(0)).To(Equal(float64(15)))
		})

		It("publishes MSpanSys as go.mem.mspan_sys_bytes", func() {
			stats.MemStats.MSpanSys = 16
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.mspan_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.mspan_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.mspan_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(16)))
		})

		It("publishes MCacheInuse as go.mem.mcache_inuse_bytes", func() {
			stats.MemStats.MCacheInuse = 17
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.mcache_inuse_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.mcache_inuse_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.mcache_inuse_bytes"].SetArgsForCall(0)).To(Equal(float64(17)))
		})

		It("publishes MCacheInuse as go.mem.mcache_sys_bytes", func() {
			stats.MemStats.MCacheSys = 18
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.mcache_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.mcache_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.mcache_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(18)))
		})

		It("publishes BuckHashSys as go.mem.buckethash_sys_bytes", func() {
			stats.MemStats.BuckHashSys = 19
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.buckethash_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.buckethash_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.buckethash_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(19)))
		})

		It("publishes GCSys as go.mem.gc_sys_bytes", func() {
			stats.MemStats.GCSys = 20
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(20)))
		})

		It("publishes OtherSys as go.mem.other_sys_bytes", func() {
			stats.MemStats.OtherSys = 21
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.other_sys_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.other_sys_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.other_sys_bytes"].SetArgsForCall(0)).To(Equal(float64(21)))
		})

		It("publishes NextGC as go.mem.gc_next_bytes", func() {
			stats.MemStats.NextGC = 22
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_next_bytes"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_next_bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_next_bytes"].SetArgsForCall(0)).To(Equal(float64(22)))
		})

		It("publishes LastGC as go.mem.gc.last_epoch_nanotime", func() {
			stats.MemStats.LastGC = 23
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_last_epoch_nanotime"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_last_epoch_nanotime"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_last_epoch_nanotime"].SetArgsForCall(0)).To(Equal(float64(23)))
		})

		It("publishes PauseTotalNs as go.mem.gc_pause_total_ns", func() {
			stats.MemStats.PauseTotalNs = 24
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_pause_total_ns"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_pause_total_ns"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_pause_total_ns"].SetArgsForCall(0)).To(Equal(float64(24)))
		})

		It("publishes PauseNs as go.mem.gc_pause_last_ns", func() {
			stats.MemStats.NumGC = 3
			stats.MemStats.PauseNs[2] = 25
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_pause_last_ns"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_pause_last_ns"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_pause_last_ns"].SetArgsForCall(0)).To(Equal(float64(25)))
		})

		It("publishes NumGC as go.mem.gc_completed_count", func() {
			stats.MemStats.NumGC = 27
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_completed_count"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_completed_count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_completed_count"].SetArgsForCall(0)).To(Equal(float64(27)))
		})

		It("publishes NumForcedGC as go.mem.gc_forced_count", func() {
			stats.MemStats.NumForcedGC = 28
			collector.Publish(stats)

			Expect(fakeGauges["go.mem.gc_forced_count"]).NotTo(BeNil())
			Expect(fakeGauges["go.mem.gc_forced_count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["go.mem.gc_forced_count"].SetArgsForCall(0)).To(Equal(float64(28)))
		})
	})
})
