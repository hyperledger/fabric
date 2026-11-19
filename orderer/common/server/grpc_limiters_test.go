/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"
	"testing"

	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInitGrpcSemaphoresBroadcast(t *testing.T) {
	conf := &localconfig.TopLevel{}
	require.Empty(t, initGrpcSemaphores(conf))

	conf.General.Limits.Concurrency.BroadcastService = 100
	semaphores := initGrpcSemaphores(conf)
	require.Len(t, semaphores, 1)
	require.Equal(t, 100, cap(semaphores[ab.AtomicBroadcast_Broadcast_FullMethodName]))
}

func TestInitGrpcSemaphoresPanicsOnNegativeBroadcastLimit(t *testing.T) {
	conf := &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = -1
	require.PanicsWithValue(t, "permits must be greater than 0", func() { initGrpcSemaphores(conf) })
}

func TestStreamGrpcLimiterRejectsBroadcastBeyondLimit(t *testing.T) {
	limit := 3
	conf := &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = limit
	semaphores := initGrpcSemaphores(conf)
	interceptor := streamGrpcLimiter(semaphores)

	handler := func(any, grpc.ServerStream) error { return nil }
	broadcastInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Broadcast_FullMethodName}

	// occupy all permits; the next broadcast stream must be rejected
	for range limit {
		require.True(t, semaphores[ab.AtomicBroadcast_Broadcast_FullMethodName].TryAcquire())
	}
	err := interceptor(nil, nil, broadcastInfo, handler)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (%d)", broadcastInfo.FullMethod, limit), status.Convert(err).Message())

	// deliver shares the service but is not limited
	deliverInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Deliver_FullMethodName}
	require.NoError(t, interceptor(nil, nil, deliverInfo, handler))
}

func TestStreamGrpcLimiterBroadcastAdmitsAndReleases(t *testing.T) {
	conf := &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = 1
	interceptor := streamGrpcLimiter(initGrpcSemaphores(conf))
	broadcastInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Broadcast_FullMethodName}

	started := make(chan struct{})
	release := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- interceptor(nil, nil, broadcastInfo, func(any, grpc.ServerStream) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	// the permit is held for the lifetime of the stream
	err := interceptor(nil, nil, broadcastInfo, func(any, grpc.ServerStream) error { return nil })
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (1)", broadcastInfo.FullMethod), status.Convert(err).Message())

	close(release)
	require.NoError(t, <-errCh)

	// the released permit admits a new stream
	require.NoError(t, interceptor(nil, nil, broadcastInfo, func(any, grpc.ServerStream) error { return nil }))
}

func TestInitializeServerConfigBroadcastLimiter(t *testing.T) {
	conf := &localconfig.TopLevel{}
	sc := initializeServerConfig(conf, nil)
	require.Len(t, sc.StreamInterceptors, 2)

	conf.General.Limits.Concurrency.BroadcastService = 100
	sc = initializeServerConfig(conf, nil)
	require.Len(t, sc.StreamInterceptors, 3)
}
