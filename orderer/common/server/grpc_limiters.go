/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"fmt"
	"time"

	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/semaphore"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultBroadcastFirstMessageTimeout bounds how long a broadcast stream may
// hold a permit without delivering its first message. All known clients send
// immediately after opening the stream, and a stream cannot survive a failed
// first message (the handler closes it), so unauthenticated idle squatting is
// bounded by this timeout while authenticated sessions are unaffected.
const defaultBroadcastFirstMessageTimeout = 5 * time.Minute

// broadcastConcurrencyLimiter bounds concurrent broadcast streams server-wide.
// The permit is acquired before the first message is decoded (bounding
// pre-auth decode memory) and held for the stream lifetime; the wrapped
// stream fails the first Recv after idleTimeout so idle streams cannot hold
// permits indefinitely.
type broadcastConcurrencyLimiter struct {
	sema        semaphore.Semaphore
	idleTimeout time.Duration
}

// initBroadcastConcurrencyLimiter returns nil when no positive limit is
// configured (uninitialized config disables the limiter, like the deliver
// limiter; completeInitialization replaces an explicit 0 with the default).
func initBroadcastConcurrencyLimiter(conf *localconfig.TopLevel) *broadcastConcurrencyLimiter {
	limit := conf.General.Limits.Concurrency.BroadcastService
	if limit <= 0 {
		return nil
	}
	return &broadcastConcurrencyLimiter{
		sema:        semaphore.New(limit),
		idleTimeout: defaultBroadcastFirstMessageTimeout,
	}
}

func (l *broadcastConcurrencyLimiter) interceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod != ab.AtomicBroadcast_Broadcast_FullMethodName {
			return handler(srv, ss)
		}
		if !l.sema.TryAcquire() {
			logger.Errorf("Too many requests for %s, exceeding concurrency limit (%d)", info.FullMethod, cap(l.sema))
			return status.Error(codes.ResourceExhausted, fmt.Sprintf("too many requests for %s, exceeding concurrency limit (%d)", info.FullMethod, cap(l.sema)))
		}
		defer l.sema.Release()
		return handler(srv, &firstMessageBoundedStream{ServerStream: ss, timeout: l.idleTimeout})
	}
}

// firstMessageBoundedStream fails the first RecvMsg when no complete message
// arrives within timeout; once a message has been received, RecvMsg is passed
// through unmodified.
type firstMessageBoundedStream struct {
	grpc.ServerStream
	timeout  time.Duration
	received bool
}

func (s *firstMessageBoundedStream) RecvMsg(m any) error {
	if s.received {
		return s.ServerStream.RecvMsg(m)
	}
	type result struct{ err error }
	resCh := make(chan result, 1)
	go func() { resCh <- result{s.ServerStream.RecvMsg(m)} }()
	select {
	case res := <-resCh:
		s.received = true
		return res.err
	case <-time.After(s.timeout):
		return status.Errorf(codes.DeadlineExceeded, "no broadcast message received within %s", s.timeout)
	}
}
