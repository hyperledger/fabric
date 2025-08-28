// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

package fp

// MulByNonResidueInv ...
func (z *Element) MulByNonResidueInv(x *Element) *Element {
	qnrInv := Element{
		9255502405446297221,
		10229180150694123945,
		9215585410771530959,
		13357015519562362907,
		5437107869987383107,
		16259554076827459,
	}
	z.Mul(x, &qnrInv)
	return z
}
