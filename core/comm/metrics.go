/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "github.com/hyperledger/fabric/common/metrics"

var (
	openConnCounterOpts = metrics.CounterOpts{
		Namespace: "grpc",
		Subsystem: "comm",
		Name:      "conn_opened",
		Help:      "gRPC connections opened. Open minus closed is the active number of connections.",
	}

	closedConnCounterOpts = metrics.CounterOpts{
		Namespace: "grpc",
		Subsystem: "comm",
		Name:      "conn_closed",
		Help:      "gRPC connections closed. Open minus closed is the active number of connections.",
	}
)

func NewServerStatsHandler(p metrics.Provider) *ServerStatsHandler {
	return &ServerStatsHandler{
		OpenConnCounter:   p.NewCounter(openConnCounterOpts),
		ClosedConnCounter: p.NewCounter(closedConnCounterOpts),
	}
}
