/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package tracing provides OpenTelemetry distributed tracing for the peer and
// the orderer.
//
// Tracing is opt-in and inert by default: unless an OTLP endpoint is configured
// via the environment, Initialize installs nothing and every instrumentation
// point in the codebase falls through to the OpenTelemetry no-op implementation.
// This keeps stock Fabric behaviour, and CI, completely unchanged.
//
// Configuration is entirely through the standard OTEL_* environment variables
// rather than core.yaml/orderer.yaml, so that the same deployment tooling used
// for the surrounding services applies unchanged here:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT         enables tracing when set
//	OTEL_EXPORTER_OTLP_TRACES_ENDPOINT  signal-specific override of the above
//	OTEL_EXPORTER_OTLP_PROTOCOL         "http/protobuf" (default) or "grpc"
//	OTEL_EXPORTER_OTLP_HEADERS          e.g. authorization tokens
//	OTEL_SERVICE_NAME                   defaults to the component name
//	OTEL_RESOURCE_ATTRIBUTES            extra resource attributes
//	OTEL_TRACES_SAMPLER                 see samplerFromEnv; default parentbased_always_on
//	OTEL_TRACES_SAMPLER_ARG             ratio for the traceidratio samplers
//
// A peer or orderer carrying real traffic emits a span per gRPC call plus
// several per transaction. Sampling everything is rarely what you want in
// production: set OTEL_TRACES_SAMPLER=parentbased_traceidratio with a small
// OTEL_TRACES_SAMPLER_ARG and let the client at the edge decide which
// transactions are interesting.
package telemetry

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var logger = flogging.MustGetLogger("tracing")

// enabled reports whether a TracerProvider that actually exports has been
// installed. Hot paths consult this to skip building attribute slices for spans
// that would be dropped anyway.
var enabled atomic.Bool

// Enabled reports whether tracing was successfully initialized and is exporting.
//
// Instrumentation does not have to check this: creating a span against the
// global no-op provider is already cheap. It is worth checking only before doing
// work that exists purely to populate a span, such as unmarshalling a payload
// solely to read an identifier off it.
func Enabled() bool { return enabled.Load() }

// Config describes a component being traced.
type Config struct {
	// ServiceName is the default value for the service.name resource attribute.
	// OTEL_SERVICE_NAME, if set, wins over this.
	ServiceName string

	// Attributes are extra resource attributes identifying this specific
	// process, such as the peer or orderer identity and its MSP. These are
	// attached to every span the process emits, so they must be values fixed at
	// startup, never per-transaction data.
	Attributes []attribute.KeyValue
}

// ShutdownFunc flushes buffered spans and releases exporter resources. It is
// safe to call when tracing was never enabled.
type ShutdownFunc func(context.Context) error

// noopShutdown is returned whenever tracing is disabled, so that callers can
// unconditionally defer the result of Initialize.
func noopShutdown(context.Context) error { return nil }

// Initialize configures the global TracerProvider and propagator from the
// environment.
//
// When no OTLP endpoint is configured it returns a no-op shutdown function and
// leaves the global provider alone, which means all instrumentation in the
// process silently becomes a no-op. An error is returned only when tracing was
// requested but could not be set up, so that a misconfigured endpoint surfaces
// at boot instead of being discovered later as missing telemetry.
func Initialize(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	endpoint := otlpEndpoint()
	if endpoint == "" {
		logger.Debug("OTLP endpoint not configured, distributed tracing is disabled")
		return noopShutdown, nil
	}

	// The W3C propagator is what carries traceparent across the gRPC hops that
	// connect client, peer and orderer. Baggage is included so that callers can
	// tag a whole transaction (for example with a load-test run id) at the edge.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	exporter, err := newExporter(ctx)
	if err != nil {
		return noopShutdown, err
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		// A partial resource is still usable and is much better than refusing to
		// start the node over a missing host attribute.
		if res == nil {
			return noopShutdown, err
		}
		logger.Warnw("Continuing with a partial telemetry resource", "error", err.Error())
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(samplerFromEnv()),
	)
	otel.SetTracerProvider(provider)
	enabled.Store(true)

	meterProvider, err := initializeMetrics(ctx, res)
	if err != nil {
		// Traces are already working at this point, so tearing everything down
		// over the metrics pipeline would lose more than it saves.
		logger.Warnw("Tracing is enabled but OTLP metrics could not be initialized", "error", err.Error())
	}

	logger.Infow(
		"Telemetry enabled",
		"endpoint", endpoint,
		"protocol", otlpProtocol(),
		"service", serviceName(cfg),
		"sampler", samplerDescription(),
		"metrics", meterProvider != nil,
	)

	return func(ctx context.Context) error {
		enabled.Store(false)

		// Shut both down even if the first fails, so that a stuck trace
		// exporter cannot strand buffered metrics.
		traceErr := provider.Shutdown(ctx)
		var metricErr error
		if meterProvider != nil {
			metricErr = meterProvider.Shutdown(ctx)
		}
		if traceErr != nil {
			return traceErr
		}
		return metricErr
	}, nil
}

// Tracer returns a named tracer from the global provider. Before Initialize
// installs a provider, and permanently when tracing is disabled, this is the
// no-op tracer.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// newExporter builds an OTLP exporter for the configured protocol. Endpoint,
// headers, TLS and timeouts are all read from the environment by the exporter
// itself, so that the full OTLP specification is supported without Fabric
// having to mirror each knob.
func newExporter(ctx context.Context) (*otlptrace.Exporter, error) {
	switch p := otlpProtocol(); p {
	case "grpc":
		return otlptracegrpc.New(ctx)
	case "http/protobuf", "http/json", "":
		// http/json is not implemented by the Go exporter; protobuf over HTTP is
		// the closest supported encoding and the OTLP default.
		return otlptracehttp.New(ctx)
	default:
		logger.Warnw("Unrecognized OTLP protocol, defaulting to http/protobuf", "protocol", p)
		return otlptracehttp.New(ctx)
	}
}

// newResource describes this process to the collector.
func newResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := make([]attribute.KeyValue, 0, len(cfg.Attributes)+1)
	// resource.WithFromEnv applies OTEL_SERVICE_NAME after this, so an explicit
	// environment setting still wins over the component default.
	attrs = append(attrs, attribute.String("service.name", cfg.ServiceName))
	attrs = append(attrs, cfg.Attributes...)

	return resource.New(
		ctx,
		resource.WithAttributes(attrs...),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessPID(),
	)
}

func serviceName(cfg Config) string {
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		return name
	}
	return cfg.ServiceName
}

// otlpEndpoint reports the configured OTLP endpoint, preferring the
// traces-specific variable. Its emptiness is what gates tracing on and off.
func otlpEndpoint() string {
	if e := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); e != "" {
		return e
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
}

func otlpProtocol() string {
	if p := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"); p != "" {
		return strings.ToLower(strings.TrimSpace(p))
	}
	return strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))
}

// samplerFromEnv implements the OTEL_TRACES_SAMPLER contract explicitly rather
// than relying on SDK defaults, because which spans a peer records is a
// throughput decision that should be obvious from reading this file.
func samplerFromEnv() sdktrace.Sampler {
	arg := func(fallback float64) float64 {
		raw := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
		if raw == "" {
			return fallback
		}
		ratio, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			logger.Warnw("Invalid OTEL_TRACES_SAMPLER_ARG, using fallback",
				"value", raw, "fallback", fallback, "error", err.Error())
			return fallback
		}
		return ratio
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))) {
	case "always_off":
		return sdktrace.NeverSample()
	case "always_on":
		return sdktrace.AlwaysSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(arg(1))
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(arg(1)))
	case "parentbased_always_on", "":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	default:
		logger.Warnw("Unrecognized OTEL_TRACES_SAMPLER, defaulting to parentbased_always_on",
			"value", os.Getenv("OTEL_TRACES_SAMPLER"))
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func samplerDescription() string {
	if s := os.Getenv("OTEL_TRACES_SAMPLER"); s != "" {
		if arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); arg != "" {
			return s + "{" + arg + "}"
		}
		return s
	}
	return "parentbased_always_on"
}
