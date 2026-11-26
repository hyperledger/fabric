/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/metrics"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultBroadcastIdleTimeout bounds how long a broadcast stream may wait
// between messages. All known clients send the first message immediately
// after opening the stream, so unauthenticated idle squatting is bounded by
// this timeout, while active streams re-arm it with every message.
const defaultBroadcastIdleTimeout = 5 * time.Minute

var broadcastSessionsRejected = metrics.CounterOpts{
	Namespace:    "broadcast",
	Name:         "sessions_rejected",
	Help:         "The number of broadcast streams rejected because a broadcast concurrency limit was reached.",
	LabelNames:   []string{"pool"},
	StatsdFormat: "%{#fqname}.%{pool}",
}

var broadcastSessionsHeld = metrics.GaugeOpts{
	Namespace:    "broadcast",
	Name:         "sessions_held",
	Help:         "The number of broadcast stream permits currently held.",
	LabelNames:   []string{"pool"},
	StatsdFormat: "%{#fqname}.%{pool}",
}

// broadcastConcurrencyLimiter bounds concurrent broadcast streams server-wide
// with two independent budgets: a small pre-authentication allowance for
// streams that have not delivered their first message yet (bounding pre-auth
// decode memory, so idle streams cannot exhaust the main budget), and the
// main budget charged from the first received message until the stream ends.
// The wrapped stream bounds every Recv wait with an idle timeout, so a
// stream idling after a successful message is expired like a never-sending
// one.
type broadcastConcurrencyLimiter struct {
	sema        semaphore.Semaphore // nil = unlimited
	preAuthSema semaphore.Semaphore // nil = unlimited
	idleTimeout time.Duration
	rejected    metrics.Counter
	held        metrics.Gauge
	throttles   map[string]*rateLimitedLogger
}

// initBroadcastConcurrencyLimiter returns nil when both limits are disabled.
// A zero value selects the configuration default, mirroring localconfig
// normalization (like the deliver limiter); -1 selects unlimited.
func initBroadcastConcurrencyLimiter(conf *localconfig.TopLevel, metricsProvider metrics.Provider) *broadcastConcurrencyLimiter {
	limits := conf.General.Limits.Concurrency
	broadcastLimit := limits.BroadcastService
	if broadcastLimit == 0 {
		broadcastLimit = localconfig.Defaults.General.Limits.Concurrency.BroadcastService
	}
	preAuthLimit := limits.BroadcastPreAuth
	if preAuthLimit == 0 {
		preAuthLimit = localconfig.Defaults.General.Limits.Concurrency.BroadcastPreAuth
	}
	if broadcastLimit <= 0 && preAuthLimit <= 0 {
		return nil
	}
	limiter := &broadcastConcurrencyLimiter{
		idleTimeout: defaultBroadcastIdleTimeout,
		rejected:    metricsProvider.NewCounter(broadcastSessionsRejected),
		held:        metricsProvider.NewGauge(broadcastSessionsHeld),
		throttles: map[string]*rateLimitedLogger{
			"preauth": {interval: time.Minute, logf: logger.Errorf},
			"active":  {interval: time.Minute, logf: logger.Errorf},
		},
	}
	if broadcastLimit > 0 {
		limiter.sema = semaphore.New(broadcastLimit)
	}
	if preAuthLimit > 0 {
		limiter.preAuthSema = semaphore.New(preAuthLimit)
	}
	return limiter
}

// admit takes one permit from sema, or counts and logs a rejection before
// returning an error. A nil semaphore is an unlimited budget.
func (l *broadcastConcurrencyLimiter) admit(sema semaphore.Semaphore, poolName string) error {
	if sema == nil {
		return nil
	}
	if sema.TryAcquire() {
		l.held.With("pool", poolName).Add(1)
		return nil
	}
	l.rejected.With("pool", poolName).Add(1)
	l.throttles[poolName].errorf("Too many broadcast %s streams, exceeding concurrency limit (%d)", poolName, cap(sema))
	return status.Error(codes.ResourceExhausted, fmt.Sprintf("too many broadcast %s streams, exceeding concurrency limit (%d)", poolName, cap(sema)))
}

func (l *broadcastConcurrencyLimiter) release(sema semaphore.Semaphore, poolName string) {
	l.held.With("pool", poolName).Add(-1)
	sema.Release()
}

func (l *broadcastConcurrencyLimiter) interceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod != ab.AtomicBroadcast_Broadcast_FullMethodName {
			return handler(srv, ss)
		}
		if err := l.admit(l.preAuthSema, "preauth"); err != nil {
			return err
		}
		stream := &broadcastBoundedStream{ServerStream: ss, limiter: l, timeout: l.idleTimeout, holdsPreAuth: l.preAuthSema != nil}
		defer stream.releasePermits()
		return handler(srv, stream)
	}
}

// broadcastBoundedStream fails RecvMsg when no message arrives within the
// idle timeout; the timeout re-arms on every receive so long-lived active
// streams are unaffected while idle ones cannot hold permits forever. The
// first successfully received message moves the stream from the pre-auth
// allowance into the authenticated broadcast budget.
type broadcastBoundedStream struct {
	grpc.ServerStream
	limiter       *broadcastConcurrencyLimiter
	timeout       time.Duration
	authenticated bool
	holdsPreAuth  bool
	holdsMain     bool
}

func (s *broadcastBoundedStream) RecvMsg(m any) error {
	err := s.recvBounded(m)
	if err != nil || s.authenticated {
		return err
	}
	// the first message graduates the stream into the main budget; only now
	// may it consume an authenticated permit, so idle streams never do
	if admitErr := s.limiter.admit(s.limiter.sema, "active"); admitErr != nil {
		return admitErr
	}
	s.holdsMain = s.limiter.sema != nil
	if s.holdsPreAuth {
		s.limiter.release(s.limiter.preAuthSema, "preauth")
		s.holdsPreAuth = false
	}
	s.authenticated = true
	return nil
}

// recvBounded bounds a single receive wait with the idle timeout. The
// receive runs in a goroutine because a ServerStream has no read deadline;
// when the timeout fires the handler returns and gRPC cancels the stream
// context, which unblocks the goroutine.
func (s *broadcastBoundedStream) recvBounded(m any) error {
	if s.timeout <= 0 {
		return s.ServerStream.RecvMsg(m)
	}
	type result struct{ err error }
	resCh := make(chan result, 1)
	go func() { resCh <- result{s.ServerStream.RecvMsg(m)} }()
	timer := time.NewTimer(s.timeout)
	defer timer.Stop()
	select {
	case res := <-resCh:
		return res.err
	case <-timer.C:
		return status.Errorf(codes.DeadlineExceeded, "no broadcast message received within %s", s.timeout)
	}
}

func (s *broadcastBoundedStream) releasePermits() {
	if s.holdsPreAuth {
		s.limiter.release(s.limiter.preAuthSema, "preauth")
		s.holdsPreAuth = false
	}
	if s.holdsMain {
		s.limiter.release(s.limiter.sema, "active")
		s.holdsMain = false
	}
}
