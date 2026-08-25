/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"sync"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// attrMap flattens rendered attributes so assertions can name what they expect.
func attrMap(t *testing.T, attrs []attribute.KeyValue) map[string]attribute.Value {
	t.Helper()

	out := make(map[string]attribute.Value, len(attrs))
	for _, a := range attrs {
		out[string(a.Key)] = a.Value
	}
	return out
}

func TestShimStatsAccumulatesByType(t *testing.T) {
	stats := NewShimStats()

	for range 500 {
		stats.Record(pb.ChaincodeMessage_GET_STATE, 400*time.Microsecond)
	}
	stats.Record(pb.ChaincodeMessage_PUT_STATE, 2*time.Millisecond)
	stats.Record(pb.ChaincodeMessage_PUT_STATE, 3*time.Millisecond)

	attrs := attrMap(t, stats.Attributes())

	require.Equal(t, int64(500), attrs["fabric.shim.get_state.count"].AsInt64())
	require.InDelta(t, 200.0, attrs["fabric.shim.get_state.duration_ms"].AsFloat64(), 0.001)

	require.Equal(t, int64(2), attrs["fabric.shim.put_state.count"].AsInt64())
	require.InDelta(t, 5.0, attrs["fabric.shim.put_state.duration_ms"].AsFloat64(), 0.001)

	require.Equal(t, int64(502), attrs["fabric.shim.total_count"].AsInt64())
	require.InDelta(t, 205.0, attrs["fabric.shim.total_duration_ms"].AsFloat64(), 0.001)
}

// A transaction that only reads should not carry zeroed counters for every
// operation it never performed.
func TestShimStatsOmitsUnusedTypes(t *testing.T) {
	stats := NewShimStats()
	stats.Record(pb.ChaincodeMessage_GET_STATE, time.Millisecond)

	attrs := attrMap(t, stats.Attributes())

	require.Contains(t, attrs, "fabric.shim.get_state.count")
	require.NotContains(t, attrs, "fabric.shim.put_state.count")
	require.NotContains(t, attrs, "fabric.shim.del_state.count")
}

// Nothing recorded means nothing to say, rather than a span cluttered with a
// zero total.
func TestShimStatsEmptyRendersNothing(t *testing.T) {
	require.Nil(t, NewShimStats().Attributes())
}

// The peer leaves this nil whenever the enclosing span is not recording, and
// every call site has to tolerate that without a guard of its own.
func TestShimStatsNilSafe(t *testing.T) {
	var stats *ShimStats

	require.NotPanics(t, func() {
		stats.Record(pb.ChaincodeMessage_GET_STATE, time.Millisecond)
		require.Nil(t, stats.Attributes())
	})
}

// Sub-millisecond work must not round away to zero, or a contract doing many
// fast writes would look free.
func TestShimStatsKeepsSubMillisecondDurations(t *testing.T) {
	stats := NewShimStats()
	stats.Record(pb.ChaincodeMessage_PUT_STATE, 250*time.Microsecond)

	attrs := attrMap(t, stats.Attributes())
	require.InDelta(t, 0.25, attrs["fabric.shim.put_state.duration_ms"].AsFloat64(), 0.001)
}

// Out-of-range types must be dropped rather than corrupting adjacent counters
// or panicking, since the message type arrives from the chaincode.
func TestShimStatsIgnoresOutOfRangeTypes(t *testing.T) {
	stats := NewShimStats()

	require.NotPanics(t, func() {
		stats.Record(pb.ChaincodeMessage_Type(9999), time.Millisecond)
		stats.Record(pb.ChaincodeMessage_Type(-1), time.Millisecond)
	})
	require.Nil(t, stats.Attributes())
}

// Callbacks are handled on separate goroutines. They serialise in practice, but
// the counters must not lose updates if that ever stops being true.
func TestShimStatsConcurrentRecording(t *testing.T) {
	stats := NewShimStats()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				stats.Record(pb.ChaincodeMessage_GET_STATE, time.Microsecond)
			}
		}()
	}
	wg.Wait()

	attrs := attrMap(t, stats.Attributes())
	require.Equal(t, int64(5000), attrs["fabric.shim.get_state.count"].AsInt64())
	require.InDelta(t, 5.0, attrs["fabric.shim.total_duration_ms"].AsFloat64(), 0.001)
}
