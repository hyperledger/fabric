<<<<<<<< HEAD:vendor/go.uber.org/zap/internal/pool/pool.go
// Copyright (c) 2023 Uber Technologies, Inc.
========
// Copyright (c) 2017 Uber Technologies, Inc.
>>>>>>>> main:vendor/github.com/onsi/ginkgo/v2/ginkgo/automaxprocs/runtime.go
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

<<<<<<<< HEAD:vendor/go.uber.org/zap/internal/pool/pool.go
// Package pool provides internal pool utilities.
package pool

import (
	"sync"
)

// A Pool is a generic wrapper around [sync.Pool] to provide strongly-typed
// object pooling.
//
// Note that SA6002 (ref: https://staticcheck.io/docs/checks/#SA6002) will
// not be detected, so all internal pool use must take care to only store
// pointer types.
type Pool[T any] struct {
	pool sync.Pool
}

// New returns a new [Pool] for T, and will use fn to construct new Ts when
// the pool is empty.
func New[T any](fn func() T) *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				return fn()
			},
		},
	}
}

// Get gets a T from the pool, or creates a new one if the pool is empty.
func (p *Pool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put returns x into the pool.
func (p *Pool[T]) Put(x T) {
	p.pool.Put(x)
========
package automaxprocs

import "math"

// CPUQuotaStatus presents the status of how CPU quota is used
type CPUQuotaStatus int

const (
	// CPUQuotaUndefined is returned when CPU quota is undefined
	CPUQuotaUndefined CPUQuotaStatus = iota
	// CPUQuotaUsed is returned when a valid CPU quota can be used
	CPUQuotaUsed
	// CPUQuotaMinUsed is returned when CPU quota is smaller than the min value
	CPUQuotaMinUsed
)

// DefaultRoundFunc is the default function to convert CPU quota from float to int. It rounds the value down (floor).
func DefaultRoundFunc(v float64) int {
	return int(math.Floor(v))
>>>>>>>> main:vendor/github.com/onsi/ginkgo/v2/ginkgo/automaxprocs/runtime.go
}
