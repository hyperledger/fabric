/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"crypto/rand"
	"crypto/sha256"
	"io"
	"math/big"

	"github.com/IBM/mathlib/driver"
)

type CurveBase struct {
	Modulus big.Int
}

func (c *CurveBase) ModNeg(a1, m driver.Zr) driver.Zr {
	res := &BaseZr{Modulus: c.Modulus}
	res.Sub(&m.(*BaseZr).Int, &a1.(*BaseZr).Int)
	res.Int.Mod(&res.Int, &m.(*BaseZr).Int)

	return res
}

func (c *CurveBase) ModMul(a1, b1, m driver.Zr) driver.Zr {
	res := &BaseZr{Modulus: c.Modulus}
	res.Int.Mul(&a1.(*BaseZr).Int, &b1.(*BaseZr).Int)
	res.Int.Mod(&res.Int, &m.(*BaseZr).Int)

	return res
}

func (c *CurveBase) ModSub(a1, b1, m driver.Zr) driver.Zr {
	res := &BaseZr{Modulus: c.Modulus}
	res.Sub(&a1.(*BaseZr).Int, &b1.(*BaseZr).Int)
	res.Int.Mod(&res.Int, &m.(*BaseZr).Int)

	return res
}

func (c *CurveBase) ModAdd(a1, b1, m driver.Zr) driver.Zr {
	res := &BaseZr{Modulus: c.Modulus}
	res.Add(&a1.(*BaseZr).Int, &b1.(*BaseZr).Int)
	res.Int.Mod(&res.Int, &m.(*BaseZr).Int)

	return res
}

func (c *CurveBase) GroupOrder() driver.Zr {
	return &BaseZr{Int: c.Modulus, Modulus: c.Modulus}
}

func (c *CurveBase) NewZrFromBytes(b []byte) driver.Zr {
	res := &BaseZr{Modulus: c.Modulus}
	res.SetBytes(b)

	return res
}

func (c *CurveBase) NewZrFromInt64(i int64) driver.Zr {
	return &BaseZr{Int: *big.NewInt(i), Modulus: c.Modulus}
}

func (c *CurveBase) NewZrFromUint64(i uint64) driver.Zr {
	return &BaseZr{Int: *new(big.Int).SetUint64(i), Modulus: c.Modulus}
}

func (c *CurveBase) NewZrFromBigInt(i *big.Int) driver.Zr {
	return &BaseZr{Int: *i, Modulus: c.Modulus}
}

func (c *CurveBase) NewRandomZr(rng io.Reader) driver.Zr {
	bi, err := rand.Int(rng, &c.Modulus)
	if err != nil {
		panic(err)
	}

	return &BaseZr{Int: *bi, Modulus: c.Modulus}
}

func (c *CurveBase) HashToZr(data []byte) driver.Zr {
	digest := sha256.Sum256(data)
	digestBig := new(big.Int).SetBytes(digest[:])
	digestBig.Mod(digestBig, &c.Modulus)

	return &BaseZr{Int: *digestBig, Modulus: c.Modulus}
}

func (p *CurveBase) Rand() (io.Reader, error) {
	return rand.Reader, nil
}

func (p *CurveBase) ModAddMul(a1 []driver.Zr, b1 []driver.Zr, modulo driver.Zr) driver.Zr {
	sum := p.NewZrFromInt64(0)
	for i := range a1 {
		sum = p.ModAdd(sum, p.ModMul(a1[i], b1[i], modulo), modulo)
	}

	return sum
}

func (p *CurveBase) ModAddMul2(a1 driver.Zr, c1 driver.Zr, b1 driver.Zr, c2 driver.Zr, m driver.Zr) driver.Zr {
	return p.ModAdd(p.ModMul(a1, c1, m), p.ModMul(b1, c2, m), m)
}

func (p *CurveBase) ModAddMul3(a1 driver.Zr, a2 driver.Zr, b1 driver.Zr, b2 driver.Zr, c1 driver.Zr, c2 driver.Zr, m driver.Zr) driver.Zr {
	return p.ModAdd(
		p.ModMul(c1, c2, m),
		p.ModAdd(
			p.ModMul(a1, a2, m),
			p.ModMul(b1, b2, m),
			m,
		),
		m,
	)
}

func (p *CurveBase) ModMulInPlace(result, a, b, m driver.Zr) {
	result.Clone(p.ModMul(a, b, m))
}

func (p *CurveBase) ModAddMul2InPlace(result driver.Zr, a1, c1, b1, c2, m driver.Zr) {
	result.Clone(p.ModAddMul2(a1, c1, b1, c2, m))
}

func (p *CurveBase) ModAddMul3InPlace(result driver.Zr, a1, a2, b1, b2, c1, c2, m driver.Zr) {
	result.Clone(p.ModAddMul3(a1, a2, b1, b2, c1, c2, m))
}
