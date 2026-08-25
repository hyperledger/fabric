/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// initializeMetrics installs a global MeterProvider exporting over OTLP.
//
// Nothing in Fabric records to this provider by default. It only becomes a
// source of data once the operations subsystem is configured with
// metrics.provider: otel, which swaps Fabric's metrics.Provider for the adapter
// in provider.go and sends every existing Fabric metric here. Installing the
// provider unconditionally alongside the tracer keeps that a one-line
// configuration change rather than a second bootstrap path.
func initializeMetrics(ctx context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	exporter, err := newMetricExporter(ctx)
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(
		metric.WithResource(res),
		// The reader's interval, and everything else about export, comes from
		// the standard OTEL_METRIC_EXPORT_* environment variables.
		metric.WithReader(metric.NewPeriodicReader(exporter)),
	)
	otel.SetMeterProvider(provider)

	return provider, nil
}

// newMetricExporter builds an OTLP metric exporter for the configured protocol,
// mirroring the choice made for traces so that both signals reach the same
// collector the same way.
func newMetricExporter(ctx context.Context) (metric.Exporter, error) {
	switch p := otlpProtocol(); p {
	case "grpc":
		return otlpmetricgrpc.New(ctx)
	case "http/protobuf", "http/json", "":
		return otlpmetrichttp.New(ctx)
	default:
		logger.Warnw("Unrecognized OTLP protocol for metrics, defaulting to http/protobuf", "protocol", p)
		return otlpmetrichttp.New(ctx)
	}
}
