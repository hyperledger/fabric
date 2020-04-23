/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

// WBBKeyGen creates a fresh weak-Boneh-Boyen signature key pair (http://ia.cr/2004/171)
func WBBKeyGen(rng *amcl.RAND) (*FP256BN.BIG, *FP256BN.ECP2) {
	// sample sk uniform from Zq
	sk := RandModOrder(rng)
	// set pk = g2^sk
	pk := GenG2.Mul(sk)
	return sk, pk
}

// WBBSign places a weak Boneh-Boyen signature on message m using secret key sk
func WBBSign(sk *FP256BN.BIG, m *FP256BN.BIG) *FP256BN.ECP {
	// compute exp = 1/(m + sk) mod q
	exp := Modadd(sk, m, GroupOrder)
	exp.Invmodp(GroupOrder)

	// return signature sig = g1^(1/(m + sk))
	return GenG1.Mul(exp)
}

// WBBVerify verifies a weak Boneh-Boyen signature sig on message m with public key pk
func WBBVerify(pk *FP256BN.ECP2, sig *FP256BN.ECP, m *FP256BN.BIG) error {
	if pk == nil || sig == nil || m == nil {
		return errors.Errorf("Weak-BB signature invalid: received nil input")
	}
	// Set P = pk * g2^m
	P := FP256BN.NewECP2()
	P.Copy(pk)
	P.Add(GenG2.Mul(m))
	P.Affine()
	// check that e(sig, pk * g2^m) = e(g1, g2)
	if !FP256BN.Fexp(FP256BN.Ate(P, sig)).Equals(GenGT) {
		return errors.Errorf("Weak-BB signature is invalid")
	}
	return nil
}
