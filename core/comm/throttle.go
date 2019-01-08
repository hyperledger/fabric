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

type Semaphore interface {
	Acquire(ctx context.Context) error
	Release()
}

type Throttle struct {
	newSemaphore NewSemaphoreFunc
	semaphore    Semaphore
}

type ThrottleOption func(t *Throttle)
type NewSemaphoreFunc func(size int) Semaphore

func WithNewSemaphore(newSemaphore NewSemaphoreFunc) ThrottleOption {
	return func(t *Throttle) {
		t.newSemaphore = newSemaphore
	}
}

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

func (t *Throttle) UnaryServerIntercptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := t.semaphore.Acquire(ctx); err != nil {
		return nil, err
	}
	defer t.semaphore.Release()

	return handler(ctx, req)
}

func (t *Throttle) StreamServerInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := ss.Context()
	if err := t.semaphore.Acquire(ctx); err != nil {
		return err
	}
	defer t.semaphore.Release()

	return handler(srv, ss)
}
