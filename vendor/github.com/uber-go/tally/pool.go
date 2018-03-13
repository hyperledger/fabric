// Copyright (c) 2018 Uber Technologies, Inc.
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

package tally

// ObjectPool is an minimalistic object pool to avoid
// any circular dependencies on any other object pool.
type ObjectPool struct {
	values chan interface{}
	alloc  func() interface{}
}

// NewObjectPool creates a new pool.
func NewObjectPool(size int) *ObjectPool {
	return &ObjectPool{
		values: make(chan interface{}, size),
	}
}

// Init initializes the object pool.
func (p *ObjectPool) Init(alloc func() interface{}) {
	p.alloc = alloc

	for i := 0; i < cap(p.values); i++ {
		p.values <- p.alloc()
	}
}

// Get gets an object from the pool.
func (p *ObjectPool) Get() interface{} {
	var v interface{}
	select {
	case v = <-p.values:
	default:
		v = p.alloc()
	}
	return v
}

// Put puts an object back to the pool.
func (p *ObjectPool) Put(obj interface{}) {
	select {
	case p.values <- obj:
	default:
	}
}
