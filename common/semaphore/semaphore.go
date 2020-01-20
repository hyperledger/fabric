/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package semaphore provides an implementation of a counting semaphore.
package semaphore

import "context"

// Semaphore is an interface to limit concurrency
type Semaphore interface {
	// Acquire acquires a permit. This call will block until a permit is available
	// or the provided context is completed.
	Acquire(ctx context.Context) error
	// Release releases a permit.
	Release()
}

// CountingSemaphore is a buffered channel based implementation of a counting semaphore
type CountingSemaphore struct {
	buf chan struct{}
}

// New creates a Semaphore with the specified number of permits.
func New(permits int) Semaphore {
	if permits <= 0 {
		panic("permits must be greater than 0")
	}
	return &CountingSemaphore{buf: make(chan struct{}, permits)}
}

// Acquire acquires a permit. This call will block until a permit is available
// or the provided context is completed.
//
// If the provided context is completed, the method will return the
// cancellation error.
func (s *CountingSemaphore) Acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.buf <- struct{}{}:
		return nil
	}
}

// Release releases a permit.
func (s *CountingSemaphore) Release() {
	select {
	case <-s.buf:
	default:
		panic("semaphore buffer is empty")
	}
}

// Disabled is a semaphore that does nothing
var Disabled = &disabled{}

type disabled struct {
}

// Acquire does nothing
func (d *disabled) Acquire(ctx context.Context) error {
	return nil
}

// Release does nothing
func (d *disabled) Release() {
}
