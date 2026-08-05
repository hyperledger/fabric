/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"slices"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc/stats"
)

// untracedServices are gRPC services whose RPCs never get an automatic span.
//
// Two different reasons appear here, and it is worth keeping them apart.
//
// The first is volume. Gossip, the orderer's Raft cluster service and gRPC
// health checks run constantly, in proportion to cluster size rather than to
// transaction load, and would swamp a trace backend with spans that say nothing
// about why a transaction was slow. Consensus timing is better answered by the
// existing Prometheus metrics, which is what they are for.
//
// The second, and the more important one, is that an automatic span lasts
// exactly as long as its RPC, and these services are built on long-lived
// streams. A peer's deliver stream stays open for the life of the connection,
// so a span wrapping it would stay open for hours, be held in memory by the
// batch processor that whole time, and then arrive as a single useless span
// covering everything the stream ever carried. Ordering and block delivery are
// instead traced per message, at the point where an individual transaction or
// block is actually handled.
var untracedServices = []string{
	"grpc.health.v1.Health",
	"gossip.Gossip",
	"orderer.Cluster",
	"orderer.ClusterNodeService",
	// Long-lived streams, traced per message instead.
	"protos.Deliver",
	"orderer.AtomicBroadcast",
}

// ServerFilter reports whether an inbound RPC should get an automatic span.
func ServerFilter(info *stats.RPCTagInfo) bool {
	if info == nil {
		return false
	}
	return !isUntracedMethod(info.FullMethodName)
}

// isUntracedMethod matches a fully qualified method name of the form
// "/package.Service/Method" against the excluded services.
func isUntracedMethod(fullMethod string) bool {
	service := strings.TrimPrefix(fullMethod, "/")
	if idx := strings.LastIndex(service, "/"); idx >= 0 {
		service = service[:idx]
	}

	return slices.Contains(untracedServices, service)
}

// ServerHandler returns a gRPC stats handler that opens a server span for each
// inbound RPC and continues the caller's trace from the traceparent in the
// request metadata.
//
// This is what connects a client's trace to everything the peer or orderer does
// on its behalf: spans created further down the endorsement path find this span
// in the context and attach to it. Without it they would each start a
// disconnected trace.
//
// When tracing is disabled the handler is still installed but the global
// provider is a no-op, so the cost is a filter check and a no-op span per RPC.
func ServerHandler(opts ...otelgrpc.Option) stats.Handler {
	return otelgrpc.NewServerHandler(
		append([]otelgrpc.Option{otelgrpc.WithFilter(ServerFilter)}, opts...)...,
	)
}
