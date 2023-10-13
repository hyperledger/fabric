/*
Copyright IBM Corp. All Rights Reserved.
Copyright 2020 ConsenSys Software Inc.

SPDX-License-Identifier: Apache-2.0
*/

package gurvy

import (
	"errors"
	"hash"
	"unsafe"

	"github.com/IBM/mathlib/driver/kilic"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fp"
	"github.com/consensys/gnark-crypto/field/pool"
)

const Bits = 381 // number of bits needed to represent a Element

type Element [6]uint64

type G1Affine struct {
	X, Y fp.Element
}

//go:linkname g1Isogeny github.com/consensys/gnark-crypto/ecc/bls12-381.g1Isogeny
func g1Isogeny(p *G1Affine)

func toKilicElement(p *fp.Element) *kilic.Fe {
	return (*kilic.Fe)(unsafe.Pointer(p))
}

func toGurvyElement(p *kilic.Fe) *fp.Element {
	return (*fp.Element)(unsafe.Pointer(p))
}

func toGurvyAffine(p *G1Affine) *bls12381.G1Affine {
	return (*bls12381.G1Affine)(unsafe.Pointer(p))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	if len(dst) > 255 {
		return nil, errors.New("invalid domain size (>255 bytes)")
	}
	sizeDomain := uint8(len(dst))

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
	if _, err := h.Write([]byte{uint8(lenInBytes >> 8), uint8(lenInBytes), uint8(0)}); err != nil {
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
		for j := 0; j < h.Size(); j++ {
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
	for i := 0; i < count; i++ {
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

	xQ0, yQ0 := kilic.SwuMapG1BE(toKilicElement(&u[0]))
	xQ1, yQ1 := kilic.SwuMapG1BE(toKilicElement(&u[1]))

	_xq0 := toGurvyElement(xQ0)
	_yq0 := toGurvyElement(yQ0)
	_xq1 := toGurvyElement(xQ1)
	_yq1 := toGurvyElement(yQ1)

	Q0 := G1Affine{*_xq0, *_yq0}
	Q1 := G1Affine{*_xq1, *_yq1}

	//TODO (perf): Add in E' first, then apply isogeny
	g1Isogeny(&Q0)
	g1Isogeny(&Q1)

	var _Q0, _Q1 bls12381.G1Jac
	_Q0.FromAffine(toGurvyAffine(&Q0))
	_Q1.FromAffine(toGurvyAffine(&Q1)).AddAssign(&_Q0)

	_Q1.ClearCofactor(&_Q1)

	toGurvyAffine(&Q1).FromJacobian(&_Q1)
	res := toGurvyAffine(&Q1)
	return *res, nil
}
