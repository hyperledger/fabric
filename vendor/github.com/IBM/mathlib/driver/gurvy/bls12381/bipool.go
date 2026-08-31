/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bls12381

import (
	"math/big"
	"sync"
)

// bigIntPool is a shared *big.Int memory pool for temporary conversions
var bigIntPool biPool

var _bigIntPool = sync.Pool{
	New: func() any {
		return new(big.Int)
	},
}

type biPool struct{}

func (biPool) Get() *big.Int {
	return _bigIntPool.Get().(*big.Int)
}

func (biPool) Put(v *big.Int) {
	if v == nil {
		panic("put called with nil value")
	}
	// reset v before putting it back
	v.SetInt64(0)
	_bigIntPool.Put(v)
}
