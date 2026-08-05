/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"go.opentelemetry.io/otel/attribute"
)

// shimStatSlots covers the ChaincodeMessage_Type enum, whose values are dense
// from UNDEFINED to GET_STATE_MULTIPLE. Indexing an array by the enum keeps
// recording a callback down to two atomic adds with no allocation and no map
// lookup, which matters because this runs on every state access a contract
// makes.
const shimStatSlots = 32

// Attribute keys are built once at startup rather than per transaction, since
// they are derived from a fixed enum and never change.
var (
	shimCountKeys    [shimStatSlots]attribute.Key
	shimDurationKeys [shimStatSlots]attribute.Key
)

func init() {
	for i := range shimStatSlots {
		name, ok := pb.ChaincodeMessage_Type_name[int32(i)]
		if !ok {
			continue
		}
		lower := strings.ToLower(name)
		shimCountKeys[i] = attribute.Key("fabric.shim." + lower + ".count")
		shimDurationKeys[i] = attribute.Key("fabric.shim." + lower + ".duration_ms")
	}
}

// Attribute keys summarising all callback types together.
const (
	AttrShimTotalCount    = attribute.Key("fabric.shim.total_count")
	AttrShimTotalDuration = attribute.Key("fabric.shim.total_duration_ms")
)

// ShimStats accumulates how many callbacks a chaincode made back into the peer
// during one invocation, and how long they took, broken down by type.
//
// This is the default alternative to emitting a span per callback. A contract
// that reads five hundred keys produces five hundred spans under the detailed
// setting, all of which have to be created, batched and shipped; here it
// produces a handful of attributes on the span that already exists. The
// question that actually gets asked — is this contract doing too much I/O, and
// how much of the transaction did it account for — is answered either way, at a
// fraction of the cost.
//
// What is lost is the individual timeline. If one read out of five hundred is
// slow, the totals show that reads dominated but not which one. That is what
// the per-call spans remain available for.
//
// A nil *ShimStats is safe to record into and is what the peer uses whenever
// the enclosing endorsement span is not recording, which makes the whole
// mechanism a single nil check on the hot path.
type ShimStats struct {
	counts    [shimStatSlots]atomic.Int64
	durations [shimStatSlots]atomic.Int64
}

// NewShimStats returns a fresh accumulator for one chaincode invocation.
func NewShimStats() *ShimStats {
	return &ShimStats{}
}

// Record adds one callback of the given type.
//
// Callbacks are handled on separate goroutines, so this is written to be safe
// under concurrency. In practice contention is negligible: a shim call is
// request/response and a contract blocks for the reply before issuing the next,
// so within a single invocation they arrive one at a time.
func (s *ShimStats) Record(msgType pb.ChaincodeMessage_Type, elapsed time.Duration) {
	if s == nil {
		return
	}
	idx := int(msgType)
	if idx < 0 || idx >= shimStatSlots {
		return
	}
	s.counts[idx].Add(1)
	s.durations[idx].Add(int64(elapsed))
}

// Attributes renders the accumulated totals for attachment to a span.
//
// Only types that actually occurred are included, so a transaction that reads
// state does not carry a dozen zeroed counters for the operations it never
// performed.
func (s *ShimStats) Attributes() []attribute.KeyValue {
	if s == nil {
		return nil
	}

	var totalCount, totalDuration int64
	attrs := make([]attribute.KeyValue, 0, 8)

	for i := range shimStatSlots {
		count := s.counts[i].Load()
		if count == 0 {
			continue
		}
		duration := s.durations[i].Load()
		totalCount += count
		totalDuration += duration

		if shimCountKeys[i] != "" {
			attrs = append(
				attrs,
				shimCountKeys[i].Int64(count),
				shimDurationKeys[i].Float64(millis(duration)),
			)
		}
	}

	if totalCount == 0 {
		return nil
	}
	return append(
		attrs,
		AttrShimTotalCount.Int64(totalCount),
		AttrShimTotalDuration.Float64(millis(totalDuration)),
	)
}

// millis converts a nanosecond total to milliseconds, keeping fractions so that
// a handful of fast writes does not round away to zero.
func millis(nanos int64) float64 {
	return float64(nanos) / float64(time.Millisecond)
}
