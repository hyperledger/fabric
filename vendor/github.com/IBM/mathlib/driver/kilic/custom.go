/*
Copyright IBM Corp. All Rights Reserved.
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kilic

import (
	"errors"
	"hash"
	"unsafe"
	_ "unsafe"

	bls12381 "github.com/kilic/bls12-381"
	"golang.org/x/crypto/blake2b"
)

const fpByteSize = 48

const fpNumberOfLimbs = 6

type Fe [fpNumberOfLimbs]uint64

var modulus = Fe{0xb9feffffffffaaab, 0x1eabfffeb153ffff, 0x6730d2a0f6b0f624, 0x64774b84f38512bf, 0x4b1ba7b6434bacd7, 0x1a0111ea397fe69a}

// r1 = r mod p
var r1 = &Fe{0x760900000002fffd, 0xebf4000bc40c0002, 0x5f48985753c758ba, 0x77ce585370525745, 0x5c071a97a256ec6d, 0x15f65ec3fa80e493}

var swuParamsForG1 = struct {
	z           *Fe
	zInv        *Fe
	a           *Fe
	b           *Fe
	minusBOverA *Fe
}{
	a:           &Fe{0x2f65aa0e9af5aa51, 0x86464c2d1e8416c3, 0xb85ce591b7bd31e2, 0x27e11c91b5f24e7c, 0x28376eda6bfc1835, 0x155455c3e5071d85},
	b:           &Fe{0xfb996971fe22a1e0, 0x9aa93eb35b742d6f, 0x8c476013de99c5c4, 0x873e27c3a221e571, 0xca72b5e45a52d888, 0x06824061418a386b},
	z:           &Fe{0x886c00000023ffdc, 0x0f70008d3090001d, 0x77672417ed5828c3, 0x9dac23e943dc1740, 0x50553f1b9c131521, 0x078c712fbe0ab6e8},
	zInv:        &Fe{0x0e8a2e8ba2e83e10, 0x5b28ba2ca4d745d1, 0x678cd5473847377a, 0x4c506dd8a8076116, 0x9bcb227d79284139, 0x0e8d3154b0ba099a},
	minusBOverA: &Fe{0x052583c93555a7fe, 0x3b40d72430f93c82, 0x1b75faa0105ec983, 0x2527e7dc63851767, 0x99fffd1f34fc181d, 0x097cab54770ca0d3},
}

func (fe *Fe) setBytes(in []byte) *Fe {
	l := len(in)
	if l >= fpByteSize {
		l = fpByteSize
	}
	padded := make([]byte, fpByteSize)
	copy(padded[fpByteSize-l:], in[:])
	var a int
	for i := 0; i < fpNumberOfLimbs; i++ {
		a = fpByteSize - i*8
		fe[i] = uint64(padded[a-1]) | uint64(padded[a-2])<<8 |
			uint64(padded[a-3])<<16 | uint64(padded[a-4])<<24 |
			uint64(padded[a-5])<<32 | uint64(padded[a-6])<<40 |
			uint64(padded[a-7])<<48 | uint64(padded[a-8])<<56
	}
	return fe
}

func (fe *Fe) isValid() bool {
	return fe.cmp(&modulus) == -1
}

func (fe *Fe) cmp(fe2 *Fe) int {
	for i := fpNumberOfLimbs - 1; i >= 0; i-- {
		if fe[i] > fe2[i] {
			return 1
		} else if fe[i] < fe2[i] {
			return -1
		}
	}
	return 0
}

func (fe *Fe) isZero() bool {
	return (fe[5] | fe[4] | fe[3] | fe[2] | fe[1] | fe[0]) == 0
}

func (fe *Fe) one() *Fe {
	return fe.set(r1)
}

func (fe *Fe) set(fe2 *Fe) *Fe {
	fe[0] = fe2[0]
	fe[1] = fe2[1]
	fe[2] = fe2[2]
	fe[3] = fe2[3]
	fe[4] = fe2[4]
	fe[5] = fe2[5]
	return fe
}

func (e *Fe) signBE() bool {
	negZ, z := new(Fe), new(Fe)
	fromMont(z, e)
	neg(negZ, z)
	return negZ.cmp(z) > -1
}

//go:linkname add github.com/kilic/bls12-381.add
func add(c, a, b *Fe)

//go:linkname toMont github.com/kilic/bls12-381.toMont
func toMont(a, b *Fe)

//go:linkname inverse github.com/kilic/bls12-381.inverse
func inverse(inv, e *Fe)

//go:linkname isQuadraticNonResidue github.com/kilic/bls12-381.isQuadraticNonResidue
func isQuadraticNonResidue(a *Fe) bool

//go:linkname sqrt github.com/kilic/bls12-381.sqrt
func sqrt(c, a *Fe) bool

//go:linkname square github.com/kilic/bls12-381.square
func square(c, a *Fe)

//go:linkname fromMont github.com/kilic/bls12-381.fromMont
func fromMont(c, a *Fe)

//go:linkname neg github.com/kilic/bls12-381.neg
func neg(c, a *Fe)

//go:linkname isogenyMapG1 github.com/kilic/bls12-381.isogenyMapG1
func isogenyMapG1(x, y *Fe)

func swuMapG1Pre(u *Fe) (*Fe, *Fe, *Fe) {
	var params = swuParamsForG1
	var tv [4]*Fe
	for i := 0; i < 4; i++ {
		tv[i] = new(Fe)
	}
	square(tv[0], u)
	mul(tv[0], tv[0], params.z)
	square(tv[1], tv[0])
	x1 := new(Fe)
	add(x1, tv[0], tv[1])
	inverse(x1, x1)
	e1 := x1.isZero()
	one := new(Fe).one()
	add(x1, x1, one)
	if e1 {
		x1.set(params.zInv)
	}
	mul(x1, x1, params.minusBOverA)
	gx1 := new(Fe)
	square(gx1, x1)
	add(gx1, gx1, params.a)
	mul(gx1, gx1, x1)
	add(gx1, gx1, params.b)
	x2 := new(Fe)
	mul(x2, tv[0], x1)
	mul(tv[1], tv[0], tv[1])
	gx2 := new(Fe)
	mul(gx2, gx1, tv[1])
	e2 := !isQuadraticNonResidue(gx1)
	x, y2 := new(Fe), new(Fe)
	if e2 {
		x.set(x1)
		y2.set(gx1)
	} else {
		x.set(x2)
		y2.set(gx2)
	}
	y := new(Fe)
	sqrt(y, y2)

	// This function is modified to perform the sign correction outside.
	return x, y, u
}

// SwuMapG1BE is implementation of Simplified Shallue-van de Woestijne-Ulas Method
// follows the implementation at draft-irtf-cfrg-hash-to-curve-06.
// uses big-endian variant: https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-06#section-4.1.1
func SwuMapG1BE(u *Fe) (*Fe, *Fe) {
	x, y, u := swuMapG1Pre(u)

	if y.signBE() != u.signBE() {
		neg(y, y)
	}
	return x, y
}

type PointG1 [3]Fe

func pointG1tobls12381PointG1(p *PointG1) *bls12381.PointG1 {
	return (*bls12381.PointG1)(unsafe.Pointer(p))
}

func feAtPos(pos int, p *bls12381.PointG1) *Fe {
	return (*Fe)(unsafe.Pointer(&(p[pos])))
}

func HashToG1GenericBESwu(data, domain []byte) (*bls12381.PointG1, error) {
	hashFunc := func() hash.Hash {
		// We pass a null key so error is impossible here.
		h, _ := blake2b.New512(nil) //nolint:errcheck
		return h
	}

	p, err := HashToCurveGenericBESwu(data, domain, hashFunc)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func HashToCurveGenericBESwu(msg, domain []byte, hashFunc func() hash.Hash) (*bls12381.PointG1, error) {
	g := bls12381.NewG1()
	hashRes, err := hashToFpXMD(hashFunc, msg, domain, 2)
	if err != nil {
		return nil, err
	}
	u0, u1 := hashRes[0], hashRes[1]

	x0, y0 := SwuMapG1BE(u0)
	x1, y1 := SwuMapG1BE(u1)
	one := new(Fe).one()
	p0, p1 := pointG1tobls12381PointG1(&PointG1{*x0, *y0, *one}), pointG1tobls12381PointG1(&PointG1{*x1, *y1, *one})

	g.Add(p0, p0, p1)
	g.Affine(p0)
	isogenyMapG1(feAtPos(0, p0), feAtPos(1, p0))
	g.ClearCofactor(p0)
	return g.Affine(p0), nil
}

func hashToFpXMD(f func() hash.Hash, msg []byte, domain []byte, count int) ([]*Fe, error) {
	randBytes, err := expandMsgXMD(f, msg, domain, count*64)
	if err != nil {
		return nil, err
	}

	els := make([]*Fe, count)
	for i := 0; i < count; i++ {
		var err error

		els[i], err = from64Bytes(randBytes[i*64 : (i+1)*64])
		if err != nil {
			return nil, err
		}
	}
	return els, nil
}

func expandMsgXMD(f func() hash.Hash, msg []byte, domain []byte, outLen int) ([]byte, error) {
	h := f()
	domainLen := uint8(len(domain))
	if domainLen > 255 {
		return nil, errors.New("invalid domain length")
	}

	// DST_prime = DST || I2OSP(len(DST), 1)
	// b_0 = H(Z_pad || msg || l_i_b_str || I2OSP(0, 1) || DST_prime)
	_, _ = h.Write(make([]byte, h.BlockSize()))
	_, _ = h.Write(msg)
	_, _ = h.Write([]byte{uint8(outLen >> 8), uint8(outLen)})
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(domain)
	_, _ = h.Write([]byte{domainLen})
	b0 := h.Sum(nil)

	// b_1 = H(b_0 || I2OSP(1, 1) || DST_prime)
	h.Reset()
	_, _ = h.Write(b0)
	_, _ = h.Write([]byte{1})
	_, _ = h.Write(domain)
	_, _ = h.Write([]byte{domainLen})
	b1 := h.Sum(nil)

	// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
	ell := (outLen + h.Size() - 1) / h.Size()
	bi := b1
	out := make([]byte, outLen)
	for i := 1; i < ell; i++ {
		h.Reset()
		// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
		tmp := make([]byte, h.Size())
		for j := 0; j < h.Size(); j++ {
			tmp[j] = b0[j] ^ bi[j]
		}
		_, _ = h.Write(tmp)
		_, _ = h.Write([]byte{1 + uint8(i)})
		_, _ = h.Write(domain)
		_, _ = h.Write([]byte{domainLen})

		// b_1 || ... || b_(ell - 1)
		copy(out[(i-1)*h.Size():i*h.Size()], bi[:])
		bi = h.Sum(nil)
	}
	// b_ell
	copy(out[(ell-1)*h.Size():], bi[:])

	return out[:outLen], nil
}

func from64Bytes(in []byte) (*Fe, error) {
	if len(in) != 32*2 {
		return nil, errors.New("input string must be equal 64 bytes")
	}
	a0 := make([]byte, fpByteSize)
	copy(a0[fpByteSize-32:fpByteSize], in[:32])
	a1 := make([]byte, fpByteSize)
	copy(a1[fpByteSize-32:fpByteSize], in[32:])
	e0, err := fromBytes(a0)
	if err != nil {
		return nil, err
	}
	e1, err := fromBytes(a1)
	if err != nil {
		return nil, err
	}
	// F = 2 ^ 256 * R
	F := Fe{
		0x75b3cd7c5ce820f,
		0x3ec6ba621c3edb0b,
		0x168a13d82bff6bce,
		0x87663c4bf8c449d2,
		0x15f34c83ddc8d830,
		0xf9628b49caa2e85,
	}

	mul(e0, e0, &F)
	add(e1, e1, e0)
	return e1, nil
}

func fromBytes(in []byte) (*Fe, error) {
	fe := &Fe{}
	if len(in) != fpByteSize {
		return nil, errors.New("input string must be equal 48 bytes")
	}
	fe.setBytes(in)
	if !fe.isValid() {
		return nil, errors.New("must be less than modulus")
	}
	toMont(fe, fe)
	return fe, nil
}
