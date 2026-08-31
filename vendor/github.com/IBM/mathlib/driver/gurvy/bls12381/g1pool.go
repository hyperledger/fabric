/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bls12381

import (
	"sync"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

// G1Jacs is a shared *bls12381.G1Jac{} memory pool
var G1Jacs g1JacPool

var _g1JacPool = sync.Pool{
	New: func() any {
		return new(bls12381.G1Jac)
	},
}

type g1JacPool struct{}

func (g1JacPool) Get() *bls12381.G1Jac {
	return _g1JacPool.Get().(*bls12381.G1Jac)
}

func (g1JacPool) Put(v *bls12381.G1Jac) {
	if v == nil {
		panic("put called with nil value")
	}
	// reset v before putting it back
	v.X.SetZero()
	v.Y.SetZero()
	v.Z.SetZero()
	_g1JacPool.Put(v)
}
