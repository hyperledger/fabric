//go:build !amd64
// +build !amd64

// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

package fptower

import "github.com/consensys/gnark-crypto/ecc/bn254/fp"

// MulByNonResidue multiplies a E2 by (9,1)
func (z *E2) MulByNonResidue(x *E2) *E2 {
	var a, b fp.Element
	a.Double(&x.A0).Double(&a).Double(&a).Add(&a, &x.A0).Sub(&a, &x.A1)
	b.Double(&x.A1).Double(&b).Double(&b).Add(&b, &x.A1).Add(&b, &x.A0)
	z.A0.Set(&a)
	z.A1.Set(&b)
	return z
}

// Mul sets z to the E2-product of x,y, returns z
func (z *E2) Mul(x, y *E2) *E2 {
	mulGenericE2(z, x, y)
	return z
}

// Square sets z to the E2-product of x,x returns z
func (z *E2) Square(x *E2) *E2 {
	squareGenericE2(z, x)
	return z
}
