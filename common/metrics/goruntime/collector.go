/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package goruntime

import (
	"runtime"
	"time"

	kitmetrics "github.com/go-kit/kit/metrics"
	"github.com/hyperledger/fabric/common/metrics"
)

type Collector struct {
	CgoCalls       kitmetrics.Gauge
	GoRoutines     kitmetrics.Gauge
	ThreadsCreated kitmetrics.Gauge
	HeapAlloc      kitmetrics.Gauge
	TotalAlloc     kitmetrics.Gauge
	Mallocs        kitmetrics.Gauge
	Frees          kitmetrics.Gauge
	HeapSys        kitmetrics.Gauge
	HeapIdle       kitmetrics.Gauge
	HeapInuse      kitmetrics.Gauge
	HeapReleased   kitmetrics.Gauge
	HeapObjects    kitmetrics.Gauge
	StackInuse     kitmetrics.Gauge
	StackSys       kitmetrics.Gauge
	MSpanInuse     kitmetrics.Gauge
	MSpanSys       kitmetrics.Gauge
	MCacheInuse    kitmetrics.Gauge
	MCacheSys      kitmetrics.Gauge
	BuckHashSys    kitmetrics.Gauge
	GCSys          kitmetrics.Gauge
	OtherSys       kitmetrics.Gauge
	NextGC         kitmetrics.Gauge
	LastGC         kitmetrics.Gauge
	PauseTotalNs   kitmetrics.Gauge
	PauseNs        kitmetrics.Gauge
	NumGC          kitmetrics.Gauge
	NumForcedGC    kitmetrics.Gauge
}

func NewCollector(p metrics.Provider) *Collector {
	return &Collector{
		CgoCalls:       p.NewGauge("runtime.go.cgo.calls"),
		GoRoutines:     p.NewGauge("runtime.go.goroutine.count"),
		ThreadsCreated: p.NewGauge("runtime.go.threads.created"),
		HeapAlloc:      p.NewGauge("runtime.go.mem.heap.alloc.bytes"),
		TotalAlloc:     p.NewGauge("runtime.go.mem.heap.total.alloc.bytes"),
		Mallocs:        p.NewGauge("runtime.go.mem.heap.malloc.count"),
		Frees:          p.NewGauge("runtime.go.mem.heap.free.count"),
		HeapSys:        p.NewGauge("runtime.go.mem.heap.sys.bytes"),
		HeapIdle:       p.NewGauge("runtime.go.mem.heap.idle.bytes"),
		HeapInuse:      p.NewGauge("runtime.go.mem.heap.inuse.bytes"),
		HeapReleased:   p.NewGauge("runtime.go.mem.heap.released.bytes"),
		HeapObjects:    p.NewGauge("runtime.go.mem.heap.objects"),
		StackInuse:     p.NewGauge("runtime.go.mem.stack.inuse.bytes"),
		StackSys:       p.NewGauge("runtime.go.mem.stack.sys.bytes"),
		MSpanInuse:     p.NewGauge("runtime.go.mem.mspan.inuse.bytes"),
		MSpanSys:       p.NewGauge("runtime.go.mem.mspan.sys.bytes"),
		MCacheInuse:    p.NewGauge("runtime.go.mem.mcache.inuse.bytes"),
		MCacheSys:      p.NewGauge("runtime.go.mem.mcache.sys.bytes"),
		BuckHashSys:    p.NewGauge("runtime.go.mem.buckethash.sys.bytes"),
		GCSys:          p.NewGauge("runtime.go.mem.gc.sys.bytes"),
		OtherSys:       p.NewGauge("runtime.go.mem.other.sys.bytes"),
		NextGC:         p.NewGauge("runtime.go.mem.gc.next.bytes"),
		LastGC:         p.NewGauge("runtime.go.mem.gc.last.epoch_nanotime"),
		PauseTotalNs:   p.NewGauge("runtime.go.mem.gc.pause.total_ns"),
		PauseNs:        p.NewGauge("runtime.go.mem.gc.pause.last_ns"),
		NumGC:          p.NewGauge("runtime.go.mem.gc.completed.count"),
		NumForcedGC:    p.NewGauge("runtime.go.mem.gc.forced.count"),
	}
}

func (c *Collector) CollectAndPublish(ticks <-chan time.Time) {
	for range ticks {
		stats := CollectStats()
		c.Publish(stats)
	}
}

func (c *Collector) Publish(stats Stats) {
	c.CgoCalls.Set(float64(stats.CgoCalls))
	c.GoRoutines.Set(float64(stats.GoRoutines))
	c.ThreadsCreated.Set(float64(stats.ThreadsCreated))
	c.HeapAlloc.Set(float64(stats.MemStats.HeapAlloc))
	c.TotalAlloc.Set(float64(stats.MemStats.TotalAlloc))
	c.Mallocs.Set(float64(stats.MemStats.Mallocs))
	c.Frees.Set(float64(stats.MemStats.Frees))
	c.HeapSys.Set(float64(stats.MemStats.HeapSys))
	c.HeapIdle.Set(float64(stats.MemStats.HeapIdle))
	c.HeapInuse.Set(float64(stats.MemStats.HeapInuse))
	c.HeapReleased.Set(float64(stats.MemStats.HeapReleased))
	c.HeapObjects.Set(float64(stats.MemStats.HeapObjects))
	c.StackInuse.Set(float64(stats.MemStats.StackInuse))
	c.StackSys.Set(float64(stats.MemStats.StackSys))
	c.MSpanInuse.Set(float64(stats.MemStats.MSpanInuse))
	c.MSpanSys.Set(float64(stats.MemStats.MSpanSys))
	c.MCacheInuse.Set(float64(stats.MemStats.MCacheInuse))
	c.MCacheSys.Set(float64(stats.MemStats.MCacheSys))
	c.BuckHashSys.Set(float64(stats.MemStats.BuckHashSys))
	c.GCSys.Set(float64(stats.MemStats.GCSys))
	c.OtherSys.Set(float64(stats.MemStats.OtherSys))
	c.NextGC.Set(float64(stats.MemStats.NextGC))
	c.LastGC.Set(float64(stats.MemStats.LastGC))
	c.PauseTotalNs.Set(float64(stats.MemStats.PauseTotalNs))
	c.PauseNs.Set(float64(stats.MemStats.PauseNs[(stats.MemStats.NumGC+255)%256]))
	c.NumGC.Set(float64(stats.MemStats.NumGC))
	c.NumForcedGC.Set(float64(stats.MemStats.NumForcedGC))
}

type Stats struct {
	CgoCalls       int64
	GoRoutines     int
	ThreadsCreated int
	MemStats       runtime.MemStats
}

func CollectStats() Stats {
	stats := Stats{
		CgoCalls:   runtime.NumCgoCall(),
		GoRoutines: runtime.NumGoroutine(),
	}
	stats.ThreadsCreated, _ = runtime.ThreadCreateProfile(nil)
	runtime.ReadMemStats(&stats.MemStats)
	return stats
}
