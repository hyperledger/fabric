/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"io"

	math "github.com/IBM/mathlib"
	"github.com/pkg/errors"
)

// wbbKeyGen creates a fresh weak-Boneh-Boyen signature key pair (http://ia.cr/2004/171)
func wbbKeyGen(curve *math.Curve, rng io.Reader) (*math.Zr, *math.G2) {
	// sample sk uniform from Zq
	sk := curve.NewRandomZr(rng)
	// set pk = g2^sk
	pk := curve.GenG2.Mul(sk)
	return sk, pk
}

// wbbSign places a weak Boneh-Boyen signature on message m using secret key sk
func wbbSign(curve *math.Curve, sk *math.Zr, m *math.Zr) *math.G1 {
	// compute exp = 1/(m + sk) mod q
	exp := curve.ModAdd(sk, m, curve.GroupOrder)
	exp.InvModP(curve.GroupOrder)

	// return signature sig = g1^(1/(m + sk))
	return curve.GenG1.Mul(exp)
}

// wbbVerify verifies a weak Boneh-Boyen signature sig on message m with public key pk
func wbbVerify(curve *math.Curve, pk *math.G2, sig *math.G1, m *math.Zr) error {
	if pk == nil || sig == nil || m == nil {
		return errors.Errorf("Weak-BB signature invalid: received nil input")
	}
	// Set P = pk * g2^m
	P := curve.NewG2()
	P.Clone(pk)
	P.Add(curve.GenG2.Mul(m))
	P.Affine()
	// check that e(sig, pk * g2^m) = e(g1, g2)
	if !curve.FExp(curve.Pairing(P, sig)).Equals(curve.GenGt) {
		return errors.Errorf("Weak-BB signature is invalid")
	}
	return nil
}
