/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"math/big"

	"github.com/IBM/mathlib/driver"
)

var onebytes = []byte{
	255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255,
}
var onebig = new(big.Int).SetBytes(onebytes)

const ScalarByteSize = 32

func BigToBytes(bi *big.Int) []byte {
	b := bi.Bytes()

	if bi.Sign() >= 0 {
		return append(make([]byte, ScalarByteSize-len(b)), b...)
	}

	twoscomp := new(big.Int).Set(onebig)
	pos := new(big.Int).Neg(bi)
	twoscomp = twoscomp.Sub(twoscomp, pos)
	twoscomp = twoscomp.Add(twoscomp, big.NewInt(1))
	b = twoscomp.Bytes()

	return append(onebytes[:ScalarByteSize-len(b)], b...)
}

type BaseZr struct {
	big.Int
	Modulus big.Int
}

func (b *BaseZr) IsZero() bool {
	return b.BitLen() == 0
}

func (b *BaseZr) IsOne() bool {
	bits := b.Bits()

	return len(bits) == 1 && bits[0] == 1 && b.Sign() > 0
}

func (b *BaseZr) BigInt() *big.Int {
	return &b.Int
}

func (b *BaseZr) Plus(a driver.Zr) driver.Zr {
	rv := &BaseZr{Modulus: b.Modulus}
	rv.Add(&b.Int, &a.(*BaseZr).Int)

	return rv
}

func (b *BaseZr) Minus(a driver.Zr) driver.Zr {
	rv := &BaseZr{Modulus: b.Modulus}
	rv.Sub(&b.Int, &a.(*BaseZr).Int)

	return rv
}

func (b *BaseZr) Mul(a driver.Zr) driver.Zr {
	rv := &BaseZr{Modulus: b.Modulus}
	rv.Int.Mul(&b.Int, &a.(*BaseZr).Int)
	rv.Int.Mod(&rv.Int, &b.Modulus)

	return rv
}

func (b *BaseZr) PowMod(x driver.Zr) driver.Zr {
	rv := &BaseZr{Modulus: b.Modulus}
	rv.Exp(&b.Int, &x.(*BaseZr).Int, &b.Modulus)

	return rv
}

func (b *BaseZr) Mod(a driver.Zr) {
	b.Int.Mod(&b.Int, &a.(*BaseZr).Int)
}

func (b *BaseZr) InvModP(p driver.Zr) {
	b.ModInverse(&b.Int, &p.(*BaseZr).Int)
}

func (b *BaseZr) InvModOrder() {
	b.ModInverse(&b.Int, &b.Modulus)
}

func (b *BaseZr) Bytes() []byte {
	target := b.Int

	if b.Sign() < 0 || b.Cmp(&b.Modulus) > 0 {
		target = *new(big.Int).Set(&b.Int)
		target = *target.Mod(&target, &b.Modulus)
		if target.Sign() < 0 {
			target = *target.Add(&target, &b.Modulus)
		}
	}

	return BigToBytes(&target)
}

func (b *BaseZr) Equals(p driver.Zr) bool {
	return b.Cmp(&p.(*BaseZr).Int) == 0
}

func (b *BaseZr) Copy() driver.Zr {
	rv := &BaseZr{Modulus: b.Modulus}
	rv.Set(&b.Int)

	return rv
}

func (b *BaseZr) Clone(a driver.Zr) {
	raw := a.(*BaseZr).Int.Bytes()
	b.SetBytes(raw)
}

func (b *BaseZr) String() string {
	return b.Text(16)
}

func (b *BaseZr) Neg() {
	b.Int.Neg(&b.Int)
}
