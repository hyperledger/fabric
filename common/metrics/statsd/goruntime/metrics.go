/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package goruntime

import (
	"github.com/hyperledger/fabric/common/metrics"
)

//gendoc:ignore

var (
	cgoCallsGaugeOpts       = metrics.GaugeOpts{Namespace: "go", Name: "cgo_calls"}
	goRoutinesGaugeOpts     = metrics.GaugeOpts{Namespace: "go", Name: "goroutine_count"}
	threadsCreatedGaugeOpts = metrics.GaugeOpts{Namespace: "go", Name: "threads_created"}
	heapAllocGaugeOpts      = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_alloc_bytes"}
	totalAllocGaugeOpts     = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_total_alloc_bytes"}
	mallocsGaugeOpts        = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_malloc_count"}
	freesGaugeOpts          = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_free_count"}
	heapSysGaugeOpts        = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_sys_bytes"}
	heapIdleGaugeOpts       = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_idle_bytes"}
	heapInuseGaugeOpts      = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_inuse_bytes"}
	heapReleasedGaugeOpts   = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_released_bytes"}
	heapObjectsGaugeOpts    = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "heap_objects"}
	stackInuseGaugeOpts     = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "stack_inuse_bytes"}
	stackSysGaugeOpts       = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "stack_sys_bytes"}
	mSpanInuseGaugeOpts     = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "mspan_inuse_bytes"}
	mSpanSysGaugeOpts       = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "mspan_sys_bytes"}
	mCacheInuseGaugeOpts    = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "mcache_inuse_bytes"}
	mCacheSysGaugeOpts      = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "mcache_sys_bytes"}
	buckHashSysGaugeOpts    = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "buckethash_sys_bytes"}
	gCSysGaugeOpts          = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_sys_bytes"}
	otherSysGaugeOpts       = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "other_sys_bytes"}
	nextGCGaugeOpts         = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_next_bytes"}
	lastGCGaugeOpts         = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_last_epoch_nanotime"}
	pauseTotalNsGaugeOpts   = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_pause_total_ns"}
	pauseNsGaugeOpts        = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_pause_last_ns"}
	numGCGaugeOpts          = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_completed_count"}
	numForcedGCGaugeOpts    = metrics.GaugeOpts{Namespace: "go", Subsystem: "mem", Name: "gc_forced_count"}
)
