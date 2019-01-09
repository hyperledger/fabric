/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package semaphore

import "context"

type Semaphore chan struct{}

func New(count int) Semaphore {
	if count <= 0 {
		panic("count must be greater than 0")
	}
	return make(chan struct{}, count)
}

func (s Semaphore) Acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s <- struct{}{}:
		return nil
	}
}

func (s Semaphore) Release() {
	select {
	case <-s:
	default:
		panic("semaphore buffer is empty")
	}
}
