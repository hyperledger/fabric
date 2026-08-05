/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

func testSpanContext(t *testing.T) trace.SpanContext {
	t.Helper()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	require.NoError(t, err)

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
}

func TestTraceContextExtensionRoundTrip(t *testing.T) {
	sc := testSpanContext(t)

	extracted := TraceContextFromHeaderExtension(MarshalTraceContextExtension(sc))

	require.True(t, extracted.IsValid())
	require.Equal(t, sc.TraceID(), extracted.TraceID())
	require.Equal(t, sc.SpanID(), extracted.SpanID())
	require.True(t, extracted.IsSampled())
}

// The whole envelope-carried approach rests on the claim that trace context can
// ride along in a ChaincodeHeaderExtension without disturbing the message for
// anyone who does not know about it. This exercises that end to end: a stock
// peer must still read the chaincode id, and the unknown fields must survive
// being unmarshalled and re-marshalled by generated code that has never heard of
// them, because that is what happens to a transaction in transit.
func TestTraceContextSurvivesProtoRoundTrip(t *testing.T) {
	sc := testSpanContext(t)

	original := &peer.ChaincodeHeaderExtension{
		ChaincodeId: &peer.ChaincodeID{Name: "mycc", Version: "1.0"},
	}
	marshalled, err := proto.Marshal(original)
	require.NoError(t, err)

	withTrace := append(marshalled, MarshalTraceContextExtension(sc)...)

	// A peer with no knowledge of these fields parses the message normally.
	decoded := &peer.ChaincodeHeaderExtension{}
	require.NoError(t, proto.Unmarshal(withTrace, decoded))
	require.Equal(t, "mycc", decoded.ChaincodeId.Name)
	require.Equal(t, "1.0", decoded.ChaincodeId.Version)

	// And re-serializing it preserves what it did not understand.
	reMarshalled, err := proto.Marshal(decoded)
	require.NoError(t, err)

	extracted := TraceContextFromHeaderExtension(reMarshalled)
	require.True(t, extracted.IsValid())
	require.Equal(t, sc.TraceID(), extracted.TraceID())
	require.Equal(t, sc.SpanID(), extracted.SpanID())
}

func TestTraceContextFromHeaderExtensionWithoutTraceContext(t *testing.T) {
	marshalled, err := proto.Marshal(&peer.ChaincodeHeaderExtension{
		ChaincodeId: &peer.ChaincodeID{Name: "mycc"},
	})
	require.NoError(t, err)

	require.False(t, TraceContextFromHeaderExtension(marshalled).IsValid())
	require.False(t, TraceContextFromHeaderExtension(nil).IsValid())
}

// Malformed bytes reach this code from the network, so it has to give up rather
// than panic or spin.
func TestTraceContextFromHeaderExtensionMalformed(t *testing.T) {
	for name, input := range map[string][]byte{
		"truncated tag":    {0xff},
		"truncated length": {0xc2, 0x3e, 0xff},
		"garbage":          {0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	} {
		t.Run(name, func(t *testing.T) {
			require.False(t, TraceContextFromHeaderExtension(input).IsValid())
		})
	}
}

func TestMarshalTraceContextExtensionInvalidSpanContext(t *testing.T) {
	require.Nil(t, MarshalTraceContextExtension(trace.SpanContext{}))
}

func TestRegistryRemembersAndForgets(t *testing.T) {
	sc := testSpanContext(t)
	r := NewSpanContextRegistry(time.Minute, 100)

	_, ok := r.Lookup("tx1")
	require.False(t, ok)

	r.Remember("tx1", sc)
	got, ok := r.Lookup("tx1")
	require.True(t, ok)
	require.Equal(t, sc.TraceID(), got.TraceID())

	r.Forget("tx1")
	_, ok = r.Lookup("tx1")
	require.False(t, ok)
}

// An unsampled context would produce a link pointing at a span no backend ever
// received, so it is not worth the memory.
func TestRegistryIgnoresUnusableContexts(t *testing.T) {
	r := NewSpanContextRegistry(time.Minute, 100)

	unsampled := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: testSpanContext(t).TraceID(),
		SpanID:  testSpanContext(t).SpanID(),
	})
	r.Remember("unsampled", unsampled)
	_, ok := r.Lookup("unsampled")
	require.False(t, ok)

	r.Remember("invalid", trace.SpanContext{})
	_, ok = r.Lookup("invalid")
	require.False(t, ok)

	r.Remember("", testSpanContext(t))
	_, ok = r.Lookup("")
	require.False(t, ok)
}

func TestRegistryExpiresAfterTwoGenerations(t *testing.T) {
	sc := testSpanContext(t)
	r := NewSpanContextRegistry(time.Minute, 100)

	now := time.Now()
	r.now = func() time.Time { return now }
	r.rotateAt = now.Add(time.Minute)

	r.Remember("tx1", sc)

	// Still in the current generation.
	_, ok := r.Lookup("tx1")
	require.True(t, ok)

	// One rotation demotes it but keeps it reachable, which is what gives
	// entries a lifetime of at least the configured TTL.
	now = now.Add(90 * time.Second)
	_, ok = r.Lookup("tx1")
	require.True(t, ok)

	// A second rotation drops it.
	now = now.Add(90 * time.Second)
	_, ok = r.Lookup("tx1")
	require.False(t, ok)
}

// Without a size cap a stalled ordering service would grow this map until the
// peer ran out of memory.
func TestRegistryRotatesOnSizeCap(t *testing.T) {
	r := NewSpanContextRegistry(time.Hour, 2)
	sc := testSpanContext(t)

	r.Remember("tx1", sc)
	r.Remember("tx2", sc)
	// Hitting the cap rotates, so tx1 and tx2 move to the previous generation.
	r.Remember("tx3", sc)
	require.LessOrEqual(t, len(r.current), 2)

	_, ok := r.Lookup("tx3")
	require.True(t, ok)

	// A further rotation evicts the oldest generation entirely.
	r.Remember("tx4", sc)
	r.Remember("tx5", sc)
	_, ok = r.Lookup("tx1")
	require.False(t, ok)
}

func TestRegistryNilSafe(t *testing.T) {
	var r *SpanContextRegistry
	require.NotPanics(t, func() {
		r.Remember("tx1", testSpanContext(t))
		r.Forget("tx1")
		_, ok := r.Lookup("tx1")
		require.False(t, ok)
	})
}

func TestSamplerFromEnv(t *testing.T) {
	for _, tc := range []struct {
		sampler     string
		arg         string
		description string
	}{
		{"", "", sdktrace.ParentBased(sdktrace.AlwaysSample()).Description()},
		{"always_on", "", sdktrace.AlwaysSample().Description()},
		{"always_off", "", sdktrace.NeverSample().Description()},
		{"traceidratio", "0.25", sdktrace.TraceIDRatioBased(0.25).Description()},
		{"parentbased_always_off", "", sdktrace.ParentBased(sdktrace.NeverSample()).Description()},
		{"parentbased_traceidratio", "0.1", sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1)).Description()},
		{"nonsense", "", sdktrace.ParentBased(sdktrace.AlwaysSample()).Description()},
		// A malformed ratio must not silently become zero and drop every trace.
		{"traceidratio", "not-a-number", sdktrace.TraceIDRatioBased(1).Description()},
	} {
		t.Run(tc.sampler+"/"+tc.arg, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER", tc.sampler)
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tc.arg)
			require.Equal(t, tc.description, samplerFromEnv().Description())
		})
	}
}

// Stock Fabric, and every existing test and CI job, must behave as if none of
// this code exists until an endpoint is configured.
func TestInitializeIsInertWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	shutdown, err := Initialize(context.Background(), Config{ServiceName: "peer"})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.False(t, Enabled())
	require.NoError(t, shutdown(context.Background()))

	// The global tracer must still hand out usable no-op spans.
	_, span := Tracer("test").Start(context.Background(), "noop")
	require.False(t, span.SpanContext().IsSampled())
	span.End()
}

// The counterpart to the inert case: with an endpoint configured, real spans
// must be recorded. The endpoint is never contacted here because the OTLP
// exporter connects lazily, which is what makes this safe to assert offline.
func TestInitializeEnablesRecordingWhenEndpointIsSet(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")

	shutdown, err := Initialize(context.Background(), Config{ServiceName: "fabric-peer"})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctx)
		require.False(t, Enabled(), "shutdown must return the package to its inert state")
	})

	require.True(t, Enabled())

	_, span := Tracer("test").Start(context.Background(), "real")
	require.True(t, span.SpanContext().IsValid())
	require.True(t, span.IsRecording())
	span.End()

	// Endorsements are only remembered while tracing is live, so this is also
	// the only place the registry's real behaviour can be checked.
	ctx := trace.ContextWithSpanContext(context.Background(), testSpanContext(t))
	RememberEndorsement(ctx, "tx-enabled")
	got, ok := LookupEndorsement("tx-enabled")
	require.True(t, ok)
	require.Equal(t, testSpanContext(t).TraceID(), got.TraceID())
}
