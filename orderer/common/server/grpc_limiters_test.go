/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/semaphore"
	broadcastmock "github.com/hyperledger/fabric/orderer/common/broadcast/mock"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInitBroadcastConcurrencyLimiter(t *testing.T) {
	conf := &localconfig.TopLevel{}
	require.Nil(t, initBroadcastConcurrencyLimiter(conf))

	conf.General.Limits.Concurrency.BroadcastService = -1
	require.Nil(t, initBroadcastConcurrencyLimiter(conf))

	conf.General.Limits.Concurrency.BroadcastService = 100
	limiter := initBroadcastConcurrencyLimiter(conf)
	require.NotNil(t, limiter)
	require.Equal(t, 100, cap(limiter.sema))
}

func TestBroadcastConcurrencyLimiterRejectsBeyondLimit(t *testing.T) {
	limiter := &broadcastConcurrencyLimiter{sema: semaphore.New(3), idleTimeout: time.Minute}
	interceptor := limiter.interceptor()

	handler := func(any, grpc.ServerStream) error { return nil }
	broadcastInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Broadcast_FullMethodName}

	// occupy all permits; the next broadcast stream must be rejected
	for range 3 {
		require.True(t, limiter.sema.TryAcquire())
	}
	err := interceptor(nil, nil, broadcastInfo, handler)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (3)", broadcastInfo.FullMethod), status.Convert(err).Message())

	// deliver shares the service but is not limited
	deliverInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Deliver_FullMethodName}
	require.NoError(t, interceptor(nil, nil, deliverInfo, handler))
}

func TestBroadcastConcurrencyLimiterAdmitsAndReleases(t *testing.T) {
	limiter := &broadcastConcurrencyLimiter{sema: semaphore.New(1), idleTimeout: time.Minute}
	interceptor := limiter.interceptor()
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

	close(release)
	require.NoError(t, <-errCh)

	// the released permit admits a new stream
	require.NoError(t, interceptor(nil, nil, broadcastInfo, func(any, grpc.ServerStream) error { return nil }))
}

// An admitted stream that sends no first message must lose its permit once
// the first-message timeout expires.
func TestFirstMessageBoundedStreamTimesOutIdleStream(t *testing.T) {
	fake := &broadcastmock.ABServer{}
	release := make(chan struct{})
	defer close(release)
	fake.RecvMsgStub = func(any) error { <-release; return io.EOF }

	wrapped := &firstMessageBoundedStream{ServerStream: fake, timeout: 20 * time.Millisecond}
	err := wrapped.RecvMsg(new(cb.Envelope))
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
}

// Once a first message has been received, later Recv calls are not
// time-bounded.
func TestFirstMessageBoundedStreamPassesThroughAfterFirstMessage(t *testing.T) {
	fake := &broadcastmock.ABServer{}
	secondRecv := make(chan struct{})
	calls := 0
	fake.RecvMsgStub = func(any) error {
		calls++
		if calls == 1 {
			return nil
		}
		<-secondRecv
		return io.EOF
	}
	fake.ContextReturns(context.Background())

	wrapped := &firstMessageBoundedStream{ServerStream: fake, timeout: 20 * time.Millisecond}
	require.NoError(t, wrapped.RecvMsg(new(cb.Envelope)))

	done := make(chan error, 1)
	go func() { done <- wrapped.RecvMsg(new(cb.Envelope)) }()
	select {
	case err := <-done:
		t.Fatalf("second Recv should not be time-bounded, got %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(secondRecv)
	require.Equal(t, io.EOF, <-done)
}

func TestInitializeServerConfigBroadcastLimiter(t *testing.T) {
	conf := &localconfig.TopLevel{}
	sc := initializeServerConfig(conf, nil)
	require.Len(t, sc.StreamInterceptors, 2)

	conf.General.Limits.Concurrency.BroadcastService = 100
	sc = initializeServerConfig(conf, nil)
	require.Len(t, sc.StreamInterceptors, 3)
}
