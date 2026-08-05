/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/protowire"
)

// Field numbers carrying W3C trace context inside a transaction's
// ChaincodeHeaderExtension.
//
// A transaction that is endorsed on one peer is committed on every peer, minutes
// later, inside a block. gRPC metadata cannot carry trace context that far: it
// dies at the endorsement hop. The only thing that reliably travels the whole
// path is the transaction itself, so a client that wants end-to-end traces
// writes its traceparent into the header extension before signing.
//
// These are unknown fields as far as stock Fabric is concerned. protobuf
// preserves unknown fields, and signatures are computed over the serialized
// header bytes, so a transaction carrying them verifies normally on an
// unmodified peer, which simply ignores them. The numbers are high enough to
// leave room for upstream additions to ChaincodeHeaderExtension.
const (
	fieldTraceparent = 1000
	fieldTracestate  = 1001
)

// TraceContextFromHeaderExtension recovers the trace context a client attached
// to a transaction, given the raw bytes of ChannelHeader.Extension.
//
// The bytes are scanned on the wire rather than unmarshalled into a generated
// struct, because these fields exist in no .proto file; keeping the extraction
// here avoids forking fabric-protos to add them.
//
// It returns an invalid SpanContext when the transaction carries no trace
// context, which is the case for every client that has not opted in. Callers
// should treat that as "no link available", not as an error.
func TraceContextFromHeaderExtension(extension []byte) trace.SpanContext {
	traceparent, tracestate := scanTraceFields(extension)
	if traceparent == "" {
		return trace.SpanContext{}
	}
	return spanContextFromW3C(traceparent, tracestate)
}

// scanTraceFields walks the top-level fields of a protobuf message looking for
// the trace context fields, skipping everything else.
func scanTraceFields(b []byte) (traceparent, tracestate string) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return traceparent, tracestate
		}
		b = b[n:]

		if typ == protowire.BytesType && (num == fieldTraceparent || num == fieldTracestate) {
			v, vn := protowire.ConsumeBytes(b)
			if vn < 0 {
				return traceparent, tracestate
			}
			if num == fieldTraceparent {
				traceparent = string(v)
			} else {
				tracestate = string(v)
			}
			b = b[vn:]
			continue
		}

		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return traceparent, tracestate
		}
		b = b[n:]
	}
	return traceparent, tracestate
}

// spanContextFromW3C parses header values using the standard propagator, so
// that validation of the traceparent format lives in one place rather than
// being reimplemented here.
func spanContextFromW3C(traceparent, tracestate string) trace.SpanContext {
	carrier := propagation.MapCarrier{"traceparent": traceparent}
	if tracestate != "" {
		carrier["tracestate"] = tracestate
	}
	ctx := propagation.TraceContext{}.Extract(context.Background(), carrier)
	return trace.SpanContextFromContext(ctx)
}

// MarshalTraceContextExtension returns the wire-format bytes a client appends to
// a marshalled ChaincodeHeaderExtension to carry trace context.
//
// Fabric itself never calls this: transactions are built and signed by clients.
// It is provided so that the SDK-side change has a reference implementation to
// match, and so the round trip can be tested here rather than only in whichever
// repository ends up owning the client.
func MarshalTraceContextExtension(sc trace.SpanContext) []byte {
	if !sc.IsValid() {
		return nil
	}

	carrier := propagation.MapCarrier{}
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	propagation.TraceContext{}.Inject(ctx, carrier)

	var out []byte
	if tp := carrier.Get("traceparent"); tp != "" {
		out = protowire.AppendTag(out, fieldTraceparent, protowire.BytesType)
		out = protowire.AppendString(out, tp)
	}
	if ts := carrier.Get("tracestate"); ts != "" {
		out = protowire.AppendTag(out, fieldTracestate, protowire.BytesType)
		out = protowire.AppendString(out, ts)
	}
	return out
}
