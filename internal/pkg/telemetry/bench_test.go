/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"
	"testing"
	"time"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// shimAttributes mirrors what the chaincode callback span records, which is the
// most frequent span the peer produces.
func shimAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrShimRequest.String("GET_STATE"),
		AttrChannelID.String("mychannel"),
		AttrChaincodeName.String("basic"),
		AttrTxID.String("6f8f1b6e2c4d4f0a9b3e7c1d5a2f8e4b6d0c9a7e3f1b5d8c2a4e6f0b9d3c7a1e"),
	}
}

// benchmarkTracer returns a tracer whose sampler always makes the given
// decision, so the two sides of the sampling ratio can be measured separately.
func benchmarkTracer(b *testing.B, sampler sdktrace.Sampler) trace.Tracer {
	b.Helper()

	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler))
	b.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return provider.Tracer("bench")
}

// BenchmarkSpanUnsampledEager measures the pattern this code used to follow:
// attributes passed to Start, which builds them before the sampler is
// consulted. Under a low sampling ratio this is what the overwhelming majority
// of transactions pay.
func BenchmarkSpanUnsampledEager(b *testing.B) {
	tracer := benchmarkTracer(b, sdktrace.NeverSample())
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_, span := tracer.Start(ctx, "Chaincode.GET_STATE", trace.WithAttributes(shimAttributes()...))
		span.End()
	}
}

// BenchmarkSpanUnsampledDeferred measures the pattern the code follows now:
// attributes attached only once the span is known to be recording. This is the
// same work as above from the caller's point of view, minus everything that
// would have been discarded.
func BenchmarkSpanUnsampledDeferred(b *testing.B) {
	tracer := benchmarkTracer(b, sdktrace.NeverSample())
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_, span := tracer.Start(ctx, "Chaincode.GET_STATE")
		if span.IsRecording() {
			span.SetAttributes(shimAttributes()...)
		}
		span.End()
	}
}

// BenchmarkSpanSampled is the cost when a transaction is actually being traced,
// which deferring does not avoid and is not meant to. It is here so the two
// sides of a sampling ratio can be weighed against each other.
func BenchmarkSpanSampled(b *testing.B) {
	tracer := benchmarkTracer(b, sdktrace.AlwaysSample())
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_, span := tracer.Start(ctx, "Chaincode.GET_STATE")
		if span.IsRecording() {
			span.SetAttributes(shimAttributes()...)
		}
		span.End()
	}
}

// BenchmarkNoTelemetry is the floor: what a callback costs on a node with no
// OTLP endpoint configured, where the global provider is the no-op one.
func BenchmarkNoTelemetry(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_, span := Tracer(TracerChaincode).Start(ctx, "Chaincode.GET_STATE")
		if span.IsRecording() {
			span.SetAttributes(shimAttributes()...)
		}
		span.End()
	}
}

// BenchmarkShimAggregate is what a callback costs under the default setting:
// two atomic adds folded into per-type totals, with no span at all.
func BenchmarkShimAggregate(b *testing.B) {
	stats := NewShimStats()

	b.ReportAllocs()
	for b.Loop() {
		stats.Record(pb.ChaincodeMessage_GET_STATE, 400*time.Microsecond)
	}
}

// BenchmarkShimAggregateNil is the cost on a peer that is not tracing, or on a
// transaction that was not sampled: the accumulator is nil and the callback
// path does nothing.
func BenchmarkShimAggregateNil(b *testing.B) {
	var stats *ShimStats

	b.ReportAllocs()
	for b.Loop() {
		stats.Record(pb.ChaincodeMessage_GET_STATE, 400*time.Microsecond)
	}
}

// BenchmarkShimStatsAttributes renders the totals, which happens once per
// invocation rather than once per callback.
func BenchmarkShimStatsAttributes(b *testing.B) {
	stats := NewShimStats()
	stats.Record(pb.ChaincodeMessage_GET_STATE, time.Millisecond)
	stats.Record(pb.ChaincodeMessage_PUT_STATE, time.Millisecond)

	b.ReportAllocs()
	for b.Loop() {
		_ = stats.Attributes()
	}
}

// BenchmarkChaincodeFunctionName covers the validation applied to the first
// argument of an invocation, which runs once per proposal on the endorsement
// path whenever a span is recording.
func BenchmarkChaincodeFunctionName(b *testing.B) {
	args := [][]byte{[]byte("CreateAsset"), []byte("asset1"), []byte("blue"), []byte("20")}

	b.ReportAllocs()
	for b.Loop() {
		if ChaincodeFunctionName(args) == "" {
			b.Fatal("expected a function name")
		}
	}
}
