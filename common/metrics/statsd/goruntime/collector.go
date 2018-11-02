/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package goruntime

import (
	"runtime"
	"time"

	"github.com/hyperledger/fabric/common/metrics"
)

type Collector struct {
	CgoCalls       metrics.Gauge
	GoRoutines     metrics.Gauge
	ThreadsCreated metrics.Gauge
	HeapAlloc      metrics.Gauge
	TotalAlloc     metrics.Gauge
	Mallocs        metrics.Gauge
	Frees          metrics.Gauge
	HeapSys        metrics.Gauge
	HeapIdle       metrics.Gauge
	HeapInuse      metrics.Gauge
	HeapReleased   metrics.Gauge
	HeapObjects    metrics.Gauge
	StackInuse     metrics.Gauge
	StackSys       metrics.Gauge
	MSpanInuse     metrics.Gauge
	MSpanSys       metrics.Gauge
	MCacheInuse    metrics.Gauge
	MCacheSys      metrics.Gauge
	BuckHashSys    metrics.Gauge
	GCSys          metrics.Gauge
	OtherSys       metrics.Gauge
	NextGC         metrics.Gauge
	LastGC         metrics.Gauge
	PauseTotalNs   metrics.Gauge
	PauseNs        metrics.Gauge
	NumGC          metrics.Gauge
	NumForcedGC    metrics.Gauge
}

func NewCollector(p metrics.Provider) *Collector {
	return &Collector{
		CgoCalls:       p.NewGauge(cgoCallsGaugeOpts),
		GoRoutines:     p.NewGauge(goRoutinesGaugeOpts),
		ThreadsCreated: p.NewGauge(threadsCreatedGaugeOpts),
		HeapAlloc:      p.NewGauge(heapAllocGaugeOpts),
		TotalAlloc:     p.NewGauge(totalAllocGaugeOpts),
		Mallocs:        p.NewGauge(mallocsGaugeOpts),
		Frees:          p.NewGauge(freesGaugeOpts),
		HeapSys:        p.NewGauge(heapSysGaugeOpts),
		HeapIdle:       p.NewGauge(heapIdleGaugeOpts),
		HeapInuse:      p.NewGauge(heapInuseGaugeOpts),
		HeapReleased:   p.NewGauge(heapReleasedGaugeOpts),
		HeapObjects:    p.NewGauge(heapObjectsGaugeOpts),
		StackInuse:     p.NewGauge(stackInuseGaugeOpts),
		StackSys:       p.NewGauge(stackSysGaugeOpts),
		MSpanInuse:     p.NewGauge(mSpanInuseGaugeOpts),
		MSpanSys:       p.NewGauge(mSpanSysGaugeOpts),
		MCacheInuse:    p.NewGauge(mCacheInuseGaugeOpts),
		MCacheSys:      p.NewGauge(mCacheSysGaugeOpts),
		BuckHashSys:    p.NewGauge(buckHashSysGaugeOpts),
		GCSys:          p.NewGauge(gCSysGaugeOpts),
		OtherSys:       p.NewGauge(otherSysGaugeOpts),
		NextGC:         p.NewGauge(nextGCGaugeOpts),
		LastGC:         p.NewGauge(lastGCGaugeOpts),
		PauseTotalNs:   p.NewGauge(pauseTotalNsGaugeOpts),
		PauseNs:        p.NewGauge(pauseNsGaugeOpts),
		NumGC:          p.NewGauge(numGCGaugeOpts),
		NumForcedGC:    p.NewGauge(numForcedGCGaugeOpts),
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
