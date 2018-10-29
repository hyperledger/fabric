/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm_test

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric/common/metrics/metricsfakes"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/stats"
)

func TestConnectionCounters(t *testing.T) {
	openConn := &metricsfakes.Counter{}
	closedConn := &metricsfakes.Counter{}
	sh := &comm.ServerStatsHandler{
		OpenConnCounter:   openConn,
		ClosedConnCounter: closedConn,
	}

	for i := 1; i <= 10; i++ {
		sh.HandleRPC(context.Background(), &stats.Begin{})
	}
	assert.Equal(t, 10, openConn.AddCallCount())

	for i := 1; i <= 5; i++ {
		sh.HandleRPC(context.Background(), &stats.End{})
	}
	assert.Equal(t, 5, closedConn.AddCallCount())

}
