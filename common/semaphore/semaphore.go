/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package semaphore provides an implementation of a counting semaphore.
package semaphore

import (
	"context"
)

// Semaphore is a buffered channel based implementation of a counting
// semaphore.
type Semaphore chan struct{}

// New creates a Semaphore with the specified number of permits.
func New(permits int) Semaphore {
	if permits <= 0 {
		panic("permits must be greater than 0")
	}
	return make(chan struct{}, permits)
}

// Acquire acquires a permit. This call will block until a permit is available
// or the provided context is completed.
//
// If the provided context is completed, the method will return the
// cancellation error.
func (s Semaphore) Acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s <- struct{}{}:
		return nil
	}
}

// TryAcquire acquires the semaphore without blocking.
// Returns true if the semaphore is acquired.
// Otherwise, returns false and leaves the semaphore unchanged.
func (s Semaphore) TryAcquire() bool {
	select {
	case s <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release releases a permit.
func (s Semaphore) Release() {
	select {
	case <-s:
	default:
		panic("semaphore buffer is empty")
	}
}
