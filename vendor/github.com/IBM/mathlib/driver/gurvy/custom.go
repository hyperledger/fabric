/*
Copyright IBM Corp. All Rights Reserved.
Copyright 2020 ConsenSys Software Inc.

SPDX-License-Identifier: Apache-2.0
*/

package gurvy

import (
	"errors"
	"hash"
	"math/bits"

	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fp"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/hash_to_curve"
	"github.com/consensys/gnark-crypto/field/pool"
)

const Bits = 381 // number of bits needed to represent a Element

// modulusBits is the base field modulus p in normal (non-Montgomery) form,
// little-endian limbs. Used by signBE.
var modulusBits = [6]uint64{
	0xb9feffffffffaaab, 0x1eabfffeb153ffff, 0x6730d2a0f6b0f624,
	0x64774b84f38512bf, 0x4b1ba7b6434bacd7, 0x1a0111ea397fe69a,
}

// SWU map parameters for G1, in Montgomery form. These limbs are identical to
// the ones used by the original github.com/kilic/bls12-381 implementation and
// are valid gnark-crypto fp.Element values because both libraries use the same
// modulus and Montgomery radix (verified: fp.One() == kilic r1).
var swuParamsForG1 = struct {
	z, zInv, a, b, minusBOverA fp.Element
}{
	a:           fp.Element{0x2f65aa0e9af5aa51, 0x86464c2d1e8416c3, 0xb85ce591b7bd31e2, 0x27e11c91b5f24e7c, 0x28376eda6bfc1835, 0x155455c3e5071d85},
	b:           fp.Element{0xfb996971fe22a1e0, 0x9aa93eb35b742d6f, 0x8c476013de99c5c4, 0x873e27c3a221e571, 0xca72b5e45a52d888, 0x06824061418a386b},
	z:           fp.Element{0x886c00000023ffdc, 0x0f70008d3090001d, 0x77672417ed5828c3, 0x9dac23e943dc1740, 0x50553f1b9c131521, 0x078c712fbe0ab6e8},
	zInv:        fp.Element{0x0e8a2e8ba2e83e10, 0x5b28ba2ca4d745d1, 0x678cd5473847377a, 0x4c506dd8a8076116, 0x9bcb227d79284139, 0x0e8d3154b0ba099a},
	minusBOverA: fp.Element{0x052583c93555a7fe, 0x3b40d72430f93c82, 0x1b75faa0105ec983, 0x2527e7dc63851767, 0x99fffd1f34fc181d, 0x097cab54770ca0d3},
}

// signBE reports the "big-endian" sign of e, matching the semantics of the
// original kilic implementation: it compares the field negation of e against e
// in normal (non-Montgomery) form and returns true iff (p - e) >= e. This is
// the non-standard sign convention used by the BBS+ hash-to-curve variant.
func signBE(e *fp.Element) bool {
	z := e.Bits() // normal form, little-endian limbs
	negZ := feNeg(z)

	return feCmp(&negZ, &z) > -1
}

// feNeg returns p - z (mod p) for the normal-form element z, with 0 mapped to 0,
// matching kilic's neg on field elements.
func feNeg(z [6]uint64) [6]uint64 {
	if z == ([6]uint64{}) {
		return [6]uint64{}
	}
	var out [6]uint64
	var borrow uint64
	out[0], borrow = bits.Sub64(modulusBits[0], z[0], 0)
	out[1], borrow = bits.Sub64(modulusBits[1], z[1], borrow)
	out[2], borrow = bits.Sub64(modulusBits[2], z[2], borrow)
	out[3], borrow = bits.Sub64(modulusBits[3], z[3], borrow)
	out[4], borrow = bits.Sub64(modulusBits[4], z[4], borrow)
	out[5], _ = bits.Sub64(modulusBits[5], z[5], borrow)

	return out
}

// feCmp compares two normal-form little-endian field elements: 1 if a > b,
// -1 if a < b, 0 if equal (big-endian magnitude comparison).
func feCmp(a, b *[6]uint64) int {
	for i := 5; i >= 0; i-- {
		if a[i] > b[i] {
			return 1
		} else if a[i] < b[i] {
			return -1
		}
	}

	return 0
}

// swuMapG1Pre is the Simplified Shallue-van de Woestijne-Ulas map (pre sign
// correction), ported from the kilic implementation onto gnark fp.Element.
func swuMapG1Pre(u *fp.Element) (x, y fp.Element) {
	params := swuParamsForG1

	var tv0, tv1 fp.Element
	tv0.Square(u)
	tv0.Mul(&tv0, &params.z)
	tv1.Square(&tv0)

	var x1 fp.Element
	x1.Add(&tv0, &tv1)
	e1 := x1.IsZero()
	x1.Inverse(&x1) // Inverse(0) == 0 in gnark, matching kilic
	var one fp.Element
	one.SetOne()
	x1.Add(&x1, &one)
	if e1 {
		x1.Set(&params.zInv)
	}
	x1.Mul(&x1, &params.minusBOverA)

	var gx1 fp.Element
	gx1.Square(&x1)
	gx1.Add(&gx1, &params.a)
	gx1.Mul(&gx1, &x1)
	gx1.Add(&gx1, &params.b)

	var x2 fp.Element
	x2.Mul(&tv0, &x1)
	tv1.Mul(&tv0, &tv1)
	var gx2 fp.Element
	gx2.Mul(&gx1, &tv1)

	// e2 is true iff gx1 is a (nonzero) quadratic residue.
	e2 := gx1.Legendre() == 1

	var y2 fp.Element
	if e2 {
		x.Set(&x1)
		y2.Set(&gx1)
	} else {
		x.Set(&x2)
		y2.Set(&gx2)
	}
	// y2 is guaranteed to be a square here; Sqrt returns one of its roots.
	y.Sqrt(&y2)

	return x, y
}

// SwuMapG1BE is the big-endian-sign variant of the Simplified SWU map, following
// draft-irtf-cfrg-hash-to-curve-06 section 4.1.1. It reproduces, bit-for-bit,
// the output of the former kilic implementation.
func SwuMapG1BE(u *fp.Element) (fp.Element, fp.Element) {
	x, y := swuMapG1Pre(u)

	if signBE(&y) != signBE(u) {
		y.Neg(&y)
	}

	return x, y
}

// ExpandMsgXmd expands msg to a slice of lenInBytes bytes.
// https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-06#section-5
// https://tools.ietf.org/html/rfc8017#section-4.1 (I2OSP/O2ISP)
func ExpandMsgXmd(msg, dst []byte, lenInBytes int, hashFunc func() hash.Hash) ([]byte, error) {
	h := hashFunc()

	ell := (lenInBytes + h.Size() - 1) / h.Size() // ceil(len_in_bytes / b_in_bytes)
	if ell > 255 {
		return nil, errors.New("invalid lenInBytes")
	}
	dstLen := len(dst)
	if dstLen > 255 {
		return nil, errors.New("invalid domain size (>255 bytes)")
	}
	sizeDomain := uint8(dstLen)

	// Z_pad = I2OSP(0, r_in_bytes)
	// l_i_b_str = I2OSP(len_in_bytes, 2)
	// DST_prime = I2OSP(len(DST), 1) ∥ DST
	// b₀ = H(Z_pad ∥ msg ∥ l_i_b_str ∥ I2OSP(0, 1) ∥ DST_prime)
	h.Reset()
	if _, err := h.Write(make([]byte, h.BlockSize())); err != nil {
		return nil, err
	}
	if _, err := h.Write(msg); err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte{uint8(lenInBytes >> 8), uint8(lenInBytes), uint8(0)}); err != nil { // #nosec G115
		return nil, err
	}
	if _, err := h.Write(dst); err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte{sizeDomain}); err != nil {
		return nil, err
	}
	b0 := h.Sum(nil)

	// b₁ = H(b₀ ∥ I2OSP(1, 1) ∥ DST_prime)
	h.Reset()
	if _, err := h.Write(b0); err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte{uint8(1)}); err != nil {
		return nil, err
	}
	if _, err := h.Write(dst); err != nil {
		return nil, err
	}
	if _, err := h.Write([]byte{sizeDomain}); err != nil {
		return nil, err
	}
	b1 := h.Sum(nil)

	res := make([]byte, lenInBytes)
	copy(res[:h.Size()], b1)

	for i := 2; i <= ell; i++ {
		// b_i = H(strxor(b₀, b_(i - 1)) ∥ I2OSP(i, 1) ∥ DST_prime)
		h.Reset()
		strxor := make([]byte, h.Size())
		for j := range h.Size() {
			strxor[j] = b0[j] ^ b1[j]
		}
		if _, err := h.Write(strxor); err != nil {
			return nil, err
		}
		if _, err := h.Write([]byte{uint8(i)}); err != nil {
			return nil, err
		}
		if _, err := h.Write(dst); err != nil {
			return nil, err
		}
		if _, err := h.Write([]byte{sizeDomain}); err != nil {
			return nil, err
		}
		b1 = h.Sum(nil)
		copy(res[h.Size()*(i-1):min(h.Size()*i, len(res))], b1)
	}

	return res, nil
}

// Hash msg to count prime field elements.
// https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-06#section-5.2
func Hash(msg, dst []byte, count int, hashFunc func() hash.Hash) ([]fp.Element, error) {
	// 128 bits of security
	// L = ceil((ceil(log2(p)) + k) / 8), where k is the security parameter = 128
	const Bytes = 1 + (Bits-1)/8
	const L = 16 + Bytes

	lenInBytes := count * L
	pseudoRandomBytes, err := ExpandMsgXmd(msg, dst, lenInBytes, hashFunc)
	if err != nil {
		return nil, err
	}

	// get temporary big int from the pool
	vv := pool.BigInt.Get()

	res := make([]fp.Element, count)
	for i := range count {
		vv.SetBytes(pseudoRandomBytes[i*L : (i+1)*L])
		res[i].SetBigInt(vv)
	}

	// release object into pool
	pool.BigInt.Put(vv)

	return res, nil
}

func HashToG1GenericBESwu(msg, dst []byte, hashFunc func() hash.Hash) (bls12381.G1Affine, error) {
	u, err := Hash(msg, dst, 2*1, hashFunc)
	if err != nil {
		return bls12381.G1Affine{}, err
	}

	xQ0, yQ0 := SwuMapG1BE(&u[0])
	xQ1, yQ1 := SwuMapG1BE(&u[1])

	Q0 := bls12381.G1Affine{X: xQ0, Y: yQ0}
	Q1 := bls12381.G1Affine{X: xQ1, Y: yQ1}

	// TODO (perf): Add in E' first, then apply isogeny
	hash_to_curve.G1Isogeny(&Q0.X, &Q0.Y)
	hash_to_curve.G1Isogeny(&Q1.X, &Q1.Y)

	var _Q0, _Q1 bls12381.G1Jac
	_Q0.FromAffine(&Q0)
	_Q1.FromAffine(&Q1).AddAssign(&_Q0)

	_Q1.ClearCofactor(&_Q1)

	Q1.FromJacobian(&_Q1)

	return Q1, nil
}
