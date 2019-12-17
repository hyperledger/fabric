/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"

	"github.com/hyperledger/fabric/common/metrics"
	"google.golang.org/grpc/stats"
)

type ServerStatsHandler struct {
	OpenConnCounter   metrics.Counter
	ClosedConnCounter metrics.Counter
}

func (h *ServerStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *ServerStatsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {}

func (h *ServerStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *ServerStatsHandler) HandleConn(ctx context.Context, s stats.ConnStats) {
	switch s.(type) {
	case *stats.ConnBegin:
		h.OpenConnCounter.Add(1)
	case *stats.ConnEnd:
		h.ClosedConnCounter.Add(1)
	}
}
