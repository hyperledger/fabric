/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package goruntime_test

import (
	"time"

	kitmetrics "github.com/go-kit/kit/metrics"
	"github.com/hyperledger/fabric/common/metrics/goruntime"
	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	. "github.com/onsi/ginkgo"
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
		fakeProvider.NewGaugeStub = func(name string) kitmetrics.Gauge {
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
		Expect(collector).To(Equal(&goruntime.Collector{
			CgoCalls:       fakeGauges["runtime.go.cgo.calls"],
			GoRoutines:     fakeGauges["runtime.go.goroutine.count"],
			ThreadsCreated: fakeGauges["runtime.go.threads.created"],
			HeapAlloc:      fakeGauges["runtime.go.mem.heap.alloc.bytes"],
			TotalAlloc:     fakeGauges["runtime.go.mem.heap.total.alloc.bytes"],
			Mallocs:        fakeGauges["runtime.go.mem.heap.malloc.count"],
			Frees:          fakeGauges["runtime.go.mem.heap.free.count"],
			HeapSys:        fakeGauges["runtime.go.mem.heap.sys.bytes"],
			HeapIdle:       fakeGauges["runtime.go.mem.heap.idle.bytes"],
			HeapInuse:      fakeGauges["runtime.go.mem.heap.inuse.bytes"],
			HeapReleased:   fakeGauges["runtime.go.mem.heap.released.bytes"],
			HeapObjects:    fakeGauges["runtime.go.mem.heap.objects"],
			StackInuse:     fakeGauges["runtime.go.mem.stack.inuse.bytes"],
			StackSys:       fakeGauges["runtime.go.mem.stack.sys.bytes"],
			MSpanInuse:     fakeGauges["runtime.go.mem.mspan.inuse.bytes"],
			MSpanSys:       fakeGauges["runtime.go.mem.mspan.sys.bytes"],
			MCacheInuse:    fakeGauges["runtime.go.mem.mcache.inuse.bytes"],
			MCacheSys:      fakeGauges["runtime.go.mem.mcache.sys.bytes"],
			BuckHashSys:    fakeGauges["runtime.go.mem.buckethash.sys.bytes"],
			GCSys:          fakeGauges["runtime.go.mem.gc.sys.bytes"],
			OtherSys:       fakeGauges["runtime.go.mem.other.sys.bytes"],
			NextGC:         fakeGauges["runtime.go.mem.gc.next.bytes"],
			LastGC:         fakeGauges["runtime.go.mem.gc.last.epoch_nanotime"],
			PauseTotalNs:   fakeGauges["runtime.go.mem.gc.pause.total_ns"],
			PauseNs:        fakeGauges["runtime.go.mem.gc.pause.last_ns"],
			NumGC:          fakeGauges["runtime.go.mem.gc.completed.count"],
			NumForcedGC:    fakeGauges["runtime.go.mem.gc.forced.count"],
		}))
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

		It("publishes CgoCalls as runtime.go.cgo.calls", func() {
			stats.CgoCalls = 1
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.cgo.calls"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.cgo.calls"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.cgo.calls"].SetArgsForCall(0)).To(Equal(float64(1)))
		})

		It("publishes GoRoutines as runtime.go.cgo.calls", func() {
			stats.GoRoutines = 2
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.goroutine.count"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.goroutine.count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.goroutine.count"].SetArgsForCall(0)).To(Equal(float64(2)))
		})

		It("publishes ThreadsCreated as runtime.go.threads.created", func() {
			stats.ThreadsCreated = 3
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.threads.created"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.threads.created"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.threads.created"].SetArgsForCall(0)).To(Equal(float64(3)))
		})

		It("publishes HeapAlloc as runtime.go.mem.heap.alloc.bytes", func() {
			stats.MemStats.HeapAlloc = 4
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.alloc.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.alloc.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.alloc.bytes"].SetArgsForCall(0)).To(Equal(float64(4)))
		})

		It("publishes TotalAlloc as runtime.go.mem.heap.total.alloc.bytes", func() {
			stats.MemStats.TotalAlloc = 5
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.total.alloc.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.total.alloc.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.total.alloc.bytes"].SetArgsForCall(0)).To(Equal(float64(5)))
		})

		It("publishes Mallocs as runtime.go.mem.heap.total.alloc.bytes", func() {
			stats.MemStats.Mallocs = 6
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.malloc.count"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.malloc.count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.malloc.count"].SetArgsForCall(0)).To(Equal(float64(6)))
		})

		It("publishes Frees as runtime.go.mem.heap.total.alloc.bytes", func() {
			stats.MemStats.Frees = 7
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.free.count"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.free.count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.free.count"].SetArgsForCall(0)).To(Equal(float64(7)))
		})

		It("publishes HeapSys as runtime.go.mem.heap.sys.bytes", func() {
			stats.MemStats.HeapSys = 8
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(8)))
		})

		It("publishes HeapSys as runtime.go.mem.heap.idle.bytes", func() {
			stats.MemStats.HeapIdle = 9
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.idle.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.idle.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.idle.bytes"].SetArgsForCall(0)).To(Equal(float64(9)))
		})

		It("publishes HeapInuse as runtime.go.mem.heap.inuse.bytes", func() {
			stats.MemStats.HeapInuse = 10
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.inuse.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.inuse.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.inuse.bytes"].SetArgsForCall(0)).To(Equal(float64(10)))
		})

		It("publishes HeapReleased as runtime.go.mem.heap.released.bytes", func() {
			stats.MemStats.HeapReleased = 11
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.released.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.released.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.released.bytes"].SetArgsForCall(0)).To(Equal(float64(11)))
		})

		It("publishes HeapObjects as runtime.go.mem.heap.objects", func() {
			stats.MemStats.HeapObjects = 12
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.heap.objects"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.heap.objects"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.heap.objects"].SetArgsForCall(0)).To(Equal(float64(12)))
		})

		It("publishes StackInuse as runtime.go.mem.stack.inuse.bytes", func() {
			stats.MemStats.StackInuse = 13
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.stack.inuse.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.stack.inuse.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.stack.inuse.bytes"].SetArgsForCall(0)).To(Equal(float64(13)))
		})

		It("publishes StackSys as runtime.go.mem.stack.sys.bytes", func() {
			stats.MemStats.StackSys = 14
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.stack.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.stack.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.stack.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(14)))
		})

		It("publishes MSpanInuse as runtime.go.mem.mspan.inuse.bytes", func() {
			stats.MemStats.MSpanInuse = 15
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.mspan.inuse.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.mspan.inuse.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.mspan.inuse.bytes"].SetArgsForCall(0)).To(Equal(float64(15)))
		})

		It("publishes MSpanSys as runtime.go.mem.mspan.sys.bytes", func() {
			stats.MemStats.MSpanSys = 16
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.mspan.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.mspan.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.mspan.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(16)))
		})

		It("publishes MCacheInuse as runtime.go.mem.mcache.inuse.bytes", func() {
			stats.MemStats.MCacheInuse = 17
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.mcache.inuse.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.mcache.inuse.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.mcache.inuse.bytes"].SetArgsForCall(0)).To(Equal(float64(17)))
		})

		It("publishes MCacheInuse as runtime.go.mem.mcache.sys.bytes", func() {
			stats.MemStats.MCacheSys = 18
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.mcache.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.mcache.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.mcache.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(18)))
		})

		It("publishes BuckHashSys as runtime.go.mem.buckethash.sys.bytes", func() {
			stats.MemStats.BuckHashSys = 19
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.buckethash.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.buckethash.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.buckethash.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(19)))
		})

		It("publishes GCSys as runtime.go.mem.gc.sys.bytes", func() {
			stats.MemStats.GCSys = 20
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(20)))
		})

		It("publishes OtherSys as runtime.go.mem.other.sys.bytes", func() {
			stats.MemStats.OtherSys = 21
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.other.sys.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.other.sys.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.other.sys.bytes"].SetArgsForCall(0)).To(Equal(float64(21)))
		})

		It("publishes NextGC as runtime.go.mem.gc.next.bytes", func() {
			stats.MemStats.NextGC = 22
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.next.bytes"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.next.bytes"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.next.bytes"].SetArgsForCall(0)).To(Equal(float64(22)))
		})

		It("publishes LastGC as runtime.go.mem.gc.last.epoch_nanotime", func() {
			stats.MemStats.LastGC = 23
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.last.epoch_nanotime"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.last.epoch_nanotime"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.last.epoch_nanotime"].SetArgsForCall(0)).To(Equal(float64(23)))
		})

		It("publishes PauseTotalNs as runtime.go.mem.gc.pause.total_ns", func() {
			stats.MemStats.PauseTotalNs = 24
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.pause.total_ns"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.pause.total_ns"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.pause.total_ns"].SetArgsForCall(0)).To(Equal(float64(24)))
		})

		It("publishes PauseNs as runtime.go.mem.gc.pause.last_ns", func() {
			stats.MemStats.NumGC = 3
			stats.MemStats.PauseNs[2] = 25
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.pause.last_ns"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.pause.last_ns"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.pause.last_ns"].SetArgsForCall(0)).To(Equal(float64(25)))
		})

		It("publishes NumGC as runtime.go.mem.gc.completed.count", func() {
			stats.MemStats.NumGC = 27
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.completed.count"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.completed.count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.completed.count"].SetArgsForCall(0)).To(Equal(float64(27)))
		})

		It("publishes NumForcedGC as runtime.go.mem.gc.forced.count", func() {
			stats.MemStats.NumForcedGC = 28
			collector.Publish(stats)

			Expect(fakeGauges["runtime.go.mem.gc.forced.count"]).NotTo(BeNil())
			Expect(fakeGauges["runtime.go.mem.gc.forced.count"].SetCallCount()).To(Equal(1))
			Expect(fakeGauges["runtime.go.mem.gc.forced.count"].SetArgsForCall(0)).To(Equal(float64(28)))
		})
	})
})
