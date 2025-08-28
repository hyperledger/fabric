/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"io"
)

type Curve interface {
	Pairing(G2, G1) Gt
	Pairing2(p2a, p2b G2, p1a, p1b G1) Gt
	FExp(Gt) Gt
	ModMul(a1, b1, m Zr) Zr
	ModNeg(a1, m Zr) Zr
	GenG1() G1
	GenG2() G2
	GenGt() Gt
	GroupOrder() Zr
	CoordinateByteSize() int
	G1ByteSize() int
	CompressedG1ByteSize() int
	G2ByteSize() int
	CompressedG2ByteSize() int
	ScalarByteSize() int
	NewG1() G1
	NewG2() G2
	NewZrFromBytes(b []byte) Zr
	NewZrFromInt64(i int64) Zr
	NewZrFromUint64(i uint64) Zr
	NewG1FromBytes(b []byte) G1
	NewG1FromCompressed(b []byte) G1
	NewG2FromBytes(b []byte) G2
	NewG2FromCompressed(b []byte) G2
	NewGtFromBytes(b []byte) Gt
	ModAdd(a, b, m Zr) Zr
	ModSub(a, b, m Zr) Zr
	HashToZr(data []byte) Zr
	HashToG1(data []byte) G1
	HashToG1WithDomain(data, domain []byte) G1
	HashToG2(data []byte) G2
	HashToG2WithDomain(data, domain []byte) G2
	NewRandomZr(rng io.Reader) Zr
	Rand() (io.Reader, error)
}

type Zr interface {
	Plus(Zr) Zr
	Minus(Zr) Zr
	Mul(Zr) Zr
	Mod(Zr)
	PowMod(Zr) Zr
	InvModP(Zr)
	Bytes() []byte
	Equals(Zr) bool
	Copy() Zr
	Clone(a Zr)
	String() string
	Neg()
}

type G1 interface {
	Clone(G1)
	Copy() G1
	Add(G1)
	Mul(Zr) G1
	Mul2(e Zr, Q G1, f Zr) G1
	Equals(G1) bool
	Bytes() []byte
	Compressed() []byte
	Sub(G1)
	IsInfinity() bool
	String() string
	Neg()
}

type G2 interface {
	Clone(G2)
	Copy() G2
	Mul(Zr) G2
	Add(G2)
	Sub(G2)
	Affine()
	Bytes() []byte
	Compressed() []byte
	String() string
	Equals(G2) bool
}

type Gt interface {
	Equals(Gt) bool
	Inverse()
	Mul(Gt)
	IsUnity() bool
	ToString() string
	Bytes() []byte
	Exp(Zr) Gt
}
