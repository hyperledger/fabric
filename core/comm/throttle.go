/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"

	"github.com/hyperledger/fabric/common/semaphore"
	"google.golang.org/grpc"
)

// Semaphore defines the interface for a counting semaphore.
type Semaphore interface {
	Acquire(ctx context.Context) error
	Release()
}

// NewSempahoreFunc defines the contract for creating a new Semaphore.
type NewSemaphoreFunc func(size int) Semaphore

// A throttle is used to control concurrency of gRPC requests.
type Throttle struct {
	newSemaphore NewSemaphoreFunc
	semaphore    Semaphore
}

// A ThrottleOption is a configuration callback.
type ThrottleOption func(t *Throttle)

// WithNewSemaphore is used to provide a custom semaphore constructor.
func WithNewSemaphore(newSemaphore NewSemaphoreFunc) ThrottleOption {
	return func(t *Throttle) {
		t.newSemaphore = newSemaphore
	}
}

// NewThrottle creates a new Throttle with the provided maximum concurrency.
func NewThrottle(maxConcurrency int, options ...ThrottleOption) *Throttle {
	t := &Throttle{
		newSemaphore: func(count int) Semaphore { return semaphore.New(count) },
	}

	for _, optionFunc := range options {
		optionFunc(t)
	}

	t.semaphore = t.newSemaphore(maxConcurrency)
	return t
}

// UnaryServerIntceptor can be used to throttle unary gRPC requests.
func (t *Throttle) UnaryServerIntercptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := t.semaphore.Acquire(ctx); err != nil {
		return nil, err
	}
	defer t.semaphore.Release()

	return handler(ctx, req)
}

// StreamServerInterceptor can be used to throttle streaming gRPC requests.
// This only limits the number of active streams, not the messages exchanged
// over the streams.
func (t *Throttle) StreamServerInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := ss.Context()
	if err := t.semaphore.Acquire(ctx); err != nil {
		return err
	}
	defer t.semaphore.Release()

	return handler(srv, ss)
}
