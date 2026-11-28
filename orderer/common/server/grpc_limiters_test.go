/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	broadcastmock "github.com/hyperledger/fabric/orderer/common/broadcast/mock"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestBroadcastLimiter(broadcastLimit, preAuthLimit int, idleTimeout time.Duration) *broadcastConcurrencyLimiter {
	conf := &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = broadcastLimit
	conf.General.Limits.Concurrency.BroadcastPreAuth = preAuthLimit
	limiter := initBroadcastConcurrencyLimiter(conf, &disabled.Provider{})
	limiter.idleTimeout = idleTimeout
	return limiter
}

func TestInitBroadcastConcurrencyLimiter(t *testing.T) {
	// both limits disabled: no limiter
	conf := &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = -1
	conf.General.Limits.Concurrency.BroadcastPreAuth = -1
	require.Nil(t, initBroadcastConcurrencyLimiter(conf, &disabled.Provider{}))

	// zero values select the defaults
	limiter := initBroadcastConcurrencyLimiter(&localconfig.TopLevel{}, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, localconfig.Defaults.General.Limits.Concurrency.BroadcastService, cap(limiter.sema))
	require.Equal(t, localconfig.Defaults.General.Limits.Concurrency.BroadcastPreAuth, cap(limiter.preAuthSema))

	// a disabled main budget still leaves the pre-auth allowance
	conf = &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = -1
	conf.General.Limits.Concurrency.BroadcastPreAuth = 100
	limiter = initBroadcastConcurrencyLimiter(conf, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Nil(t, limiter.sema)
	require.Equal(t, 100, cap(limiter.preAuthSema))

	// a disabled pre-auth allowance still leaves the main budget
	conf = &localconfig.TopLevel{}
	conf.General.Limits.Concurrency.BroadcastService = 100
	conf.General.Limits.Concurrency.BroadcastPreAuth = -1
	limiter = initBroadcastConcurrencyLimiter(conf, &disabled.Provider{})
	require.NotNil(t, limiter)
	require.Equal(t, 100, cap(limiter.sema))
	require.Nil(t, limiter.preAuthSema)
}

// The interceptor rejects at the pre-auth gate: once every pre-auth permit
// is held by streams that sent nothing, the next broadcast stream is
// rejected before its handler runs.
func TestBroadcastConcurrencyLimiterRejectsAtPreAuthGate(t *testing.T) {
	limiter := newTestBroadcastLimiter(10, 3, time.Minute)
	interceptor := limiter.interceptor()

	handler := func(any, grpc.ServerStream) error { return nil }
	broadcastInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Broadcast_FullMethodName}

	// occupy all pre-auth permits; the next broadcast stream must be rejected
	for range 3 {
		require.True(t, limiter.preAuthSema.TryAcquire())
	}
	err := interceptor(nil, nil, broadcastInfo, handler)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, "too many broadcast preauth streams, exceeding concurrency limit (3)", status.Convert(err).Message())

	// deliver shares the service but is not limited
	deliverInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Deliver_FullMethodName}
	require.NoError(t, interceptor(nil, nil, deliverInfo, handler))
}

// The main budget is charged only from the first received message: idle
// streams holding every pre-auth permit leave the main budget untouched,
// and a stream whose first message arrives when the main budget is full is
// rejected from RecvMsg.
func TestBroadcastBoundedStreamGraduatesFromPreAuthToMainBudget(t *testing.T) {
	limiter := newTestBroadcastLimiter(1, 1, time.Minute)
	interceptor := limiter.interceptor()
	broadcastInfo := &grpc.StreamServerInfo{FullMethod: ab.AtomicBroadcast_Broadcast_FullMethodName}

	// stream one: receives a first message and stays open, holding the only
	// main permit while its pre-auth permit is released
	first := &broadcastmock.ABServer{}
	first.RecvMsgReturns(nil)
	done := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		done <- interceptor(nil, first, broadcastInfo, func(_ any, ss grpc.ServerStream) error {
			if err := ss.RecvMsg(new(cb.Envelope)); err != nil {
				return err
			}
			<-release
			return nil
		})
	}()

	// stream two: admitted at the pre-auth gate, but its first message is
	// rejected because the main budget is exhausted
	require.Eventually(t, func() bool { return len(limiter.sema) == 1 },
		5*time.Second, time.Millisecond, "first stream must hold the main permit")
	require.Empty(t, limiter.preAuthSema, "first message must release the pre-auth permit")
	second := &broadcastmock.ABServer{}
	second.RecvMsgReturns(nil)
	err := interceptor(nil, second, broadcastInfo, func(_ any, ss grpc.ServerStream) error {
		return ss.RecvMsg(new(cb.Envelope))
	})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, "too many broadcast active streams, exceeding concurrency limit (1)", status.Convert(err).Message())
	require.Equal(t, 1, cap(limiter.preAuthSema)-len(limiter.preAuthSema), "a rejected stream must release its pre-auth permit")

	close(release)
	require.NoError(t, <-done)
	require.Empty(t, limiter.sema, "a closed stream must release its main permit")
}

// An admitted stream that sends no message loses its permit once the idle
// timeout expires.
func TestBroadcastBoundedStreamTimesOutIdleStream(t *testing.T) {
	fake := &broadcastmock.ABServer{}
	release := make(chan struct{})
	defer close(release)
	fake.RecvMsgStub = func(any) error { <-release; return io.EOF }

	limiter := newTestBroadcastLimiter(1, 1, 20*time.Millisecond)
	wrapped := &broadcastBoundedStream{ServerStream: fake, limiter: limiter, timeout: limiter.idleTimeout, holdsPreAuth: false}
	err := wrapped.RecvMsg(new(cb.Envelope))
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
}

// The idle timeout re-arms on every receive: a stream that delivered a
// message and then goes silent is expired exactly like a never-sending
// stream, so one successful message cannot hold a permit forever.
func TestBroadcastBoundedStreamReArmsTimeoutAfterFirstMessage(t *testing.T) {
	fake := &broadcastmock.ABServer{}
	secondRecv := make(chan struct{})
	defer close(secondRecv)
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

	limiter := newTestBroadcastLimiter(1, 1, 20*time.Millisecond)
	wrapped := &broadcastBoundedStream{ServerStream: fake, limiter: limiter, timeout: limiter.idleTimeout, holdsPreAuth: false}
	require.NoError(t, wrapped.RecvMsg(new(cb.Envelope)))
	require.True(t, wrapped.authenticated)

	err := wrapped.RecvMsg(new(cb.Envelope))
	require.Equal(t, codes.DeadlineExceeded, status.Code(err), "the idle timeout must re-arm after a received message")
}

// A stream that keeps delivering messages within the idle timeout is never
// expired, however long it lives.
func TestBroadcastBoundedStreamActiveStreamIsNotExpired(t *testing.T) {
	fake := &broadcastmock.ABServer{}
	fake.RecvMsgReturns(nil)
	fake.ContextReturns(context.Background())

	limiter := newTestBroadcastLimiter(1, 1, 30*time.Millisecond)
	wrapped := &broadcastBoundedStream{ServerStream: fake, limiter: limiter, timeout: limiter.idleTimeout, holdsPreAuth: false}
	for range 10 {
		require.NoError(t, wrapped.RecvMsg(new(cb.Envelope)))
		time.Sleep(15 * time.Millisecond) // active, but below the idle timeout
	}
}

func TestRateLimitedLogger(t *testing.T) {
	logged := 0
	r := &rateLimitedLogger{interval: time.Hour, logf: func(string, ...any) { logged++ }}
	r.errorf("one")
	r.errorf("two")
	require.Equal(t, 1, logged)
	r.last = r.last.Add(-2 * time.Hour)
	r.errorf("three")
	require.Equal(t, 2, logged)
}
