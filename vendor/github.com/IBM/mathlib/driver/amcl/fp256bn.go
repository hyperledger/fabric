/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package amcl

import (
	r "crypto/rand"
	"crypto/sha256"
	"io"
	"math/big"
	"regexp"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

/*********************************************************************/

type fp256bnZr struct {
	*FP256BN.BIG
}

func (b *fp256bnZr) Plus(a driver.Zr) driver.Zr {
	return &fp256bnZr{b.BIG.Plus(a.(*fp256bnZr).BIG)}
}

func (b *fp256bnZr) PowMod(x driver.Zr) driver.Zr {
	q := FP256BN.NewBIGints(FP256BN.CURVE_Order)

	return &fp256bnZr{b.BIG.Powmod(x.(*fp256bnZr).BIG, q)}
}

func (b *fp256bnZr) Mod(a driver.Zr) {
	b.BIG.Mod(a.(*fp256bnZr).BIG)
}

func (b *fp256bnZr) InvModP(p driver.Zr) {
	b.BIG.Invmodp(p.(*fp256bnZr).BIG)
}

func (b *fp256bnZr) Bytes() []byte {
	by := make([]byte, int(FP256BN.MODBYTES))
	b.BIG.ToBytes(by)
	return by
}

func (b *fp256bnZr) Equals(p driver.Zr) bool {
	return *b.BIG == *(p.(*fp256bnZr).BIG)
}

func (b *fp256bnZr) Copy() driver.Zr {
	return &fp256bnZr{FP256BN.NewBIGcopy(b.BIG)}
}

func (b *fp256bnZr) Clone(a driver.Zr) {
	c := a.Copy()
	b.BIG = c.(*fp256bnZr).BIG
}

func (b *fp256bnZr) String() string {
	return strings.TrimLeft(b.BIG.ToString(), "0")
}

/*********************************************************************/

type fp256bnGt struct {
	*FP256BN.FP12
}

func (a *fp256bnGt) Equals(b driver.Gt) bool {
	return a.FP12.Equals(b.(*fp256bnGt).FP12)
}

func (a *fp256bnGt) IsUnity() bool {
	return a.FP12.Isunity()
}

func (a *fp256bnGt) Inverse() {
	a.FP12.Inverse()
}

func (a *fp256bnGt) Mul(b driver.Gt) {
	a.FP12.Mul(b.(*fp256bnGt).FP12)
}

func (b *fp256bnGt) ToString() string {
	return b.FP12.ToString()
}

func (b *fp256bnGt) Bytes() []byte {
	bytes := make([]byte, 12*int(FP256BN.MODBYTES))
	b.FP12.ToBytes(bytes)
	return bytes
}

/*********************************************************************/

type Fp256bn struct {
}

func (*Fp256bn) Pairing(a driver.G2, b driver.G1) driver.Gt {
	return &fp256bnGt{FP256BN.Ate(a.(*fp256bnG2).ECP2, b.(*fp256bnG1).ECP)}
}

func (*Fp256bn) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	return &fp256bnGt{FP256BN.Ate2(p2a.(*fp256bnG2).ECP2, p1a.(*fp256bnG1).ECP, p2b.(*fp256bnG2).ECP2, p1b.(*fp256bnG1).ECP)}
}

func (*Fp256bn) FExp(e driver.Gt) driver.Gt {
	return &fp256bnGt{FP256BN.Fexp(e.(*fp256bnGt).FP12)}
}

func (*Fp256bn) ModMul(a1, b1, m driver.Zr) driver.Zr {
	return &fp256bnZr{FP256BN.Modmul(a1.(*fp256bnZr).BIG, b1.(*fp256bnZr).BIG, m.(*fp256bnZr).BIG)}
}

func (*Fp256bn) ModNeg(a1, m driver.Zr) driver.Zr {
	return &fp256bnZr{FP256BN.Modneg(a1.(*fp256bnZr).BIG, m.(*fp256bnZr).BIG)}
}

func (*Fp256bn) GenG1() driver.G1 {
	return &fp256bnG1{FP256BN.NewECPbigs(FP256BN.NewBIGints(FP256BN.CURVE_Gx), FP256BN.NewBIGints(FP256BN.CURVE_Gy))}
}

func (*Fp256bn) GenG2() driver.G2 {
	return &fp256bnG2{FP256BN.NewECP2fp2s(
		FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pxa), FP256BN.NewBIGints(FP256BN.CURVE_Pxb)),
		FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pya), FP256BN.NewBIGints(FP256BN.CURVE_Pyb)))}
}

func (p *Fp256bn) GenGt() driver.Gt {
	return &fp256bnGt{FP256BN.Fexp(FP256BN.Ate(p.GenG2().(*fp256bnG2).ECP2, p.GenG1().(*fp256bnG1).ECP))}
}

func (p *Fp256bn) GroupOrder() driver.Zr {
	return &fp256bnZr{FP256BN.NewBIGints(FP256BN.CURVE_Order)}
}

func (p *Fp256bn) FieldBytes() int {
	return int(FP256BN.MODBYTES)
}

func (p *Fp256bn) NewG1() driver.G1 {
	return &fp256bnG1{FP256BN.NewECP()}
}

func (p *Fp256bn) NewG2() driver.G2 {
	return &fp256bnG2{FP256BN.NewECP2()}
}

func (p *Fp256bn) NewG1FromCoords(ix, iy driver.Zr) driver.G1 {
	return &fp256bnG1{FP256BN.NewECPbigs(ix.(*fp256bnZr).BIG, iy.(*fp256bnZr).BIG)}
}

func (p *Fp256bn) NewZrFromBytes(b []byte) driver.Zr {
	return &fp256bnZr{FP256BN.FromBytes(b)}
}

func (p *Fp256bn) NewZrFromInt(i int64) driver.Zr {
	var i0, i1, i2, i3, i4 int64

	sign := int64(1)
	if i < 0 {
		sign = -1
	}

	b := common.BigToBytes(big.NewInt(i * sign))

	pos := 32
	i0 = new(big.Int).SetBytes(b[pos-7 : pos]).Int64()
	pos -= 7
	i1 = new(big.Int).SetBytes(b[pos-7 : pos]).Int64()
	pos -= 7
	i2 = new(big.Int).SetBytes(b[pos-7 : pos]).Int64()
	pos -= 7
	i3 = new(big.Int).SetBytes(b[pos-7 : pos]).Int64()
	pos -= 7
	i4 = new(big.Int).SetBytes(b[0:pos]).Int64()

	zr := FP256BN.NewBIGints([FP256BN.NLEN]FP256BN.Chunk{FP256BN.Chunk(i0), FP256BN.Chunk(i1), FP256BN.Chunk(i2), FP256BN.Chunk(i3), FP256BN.Chunk(i4)})
	if sign < 0 {
		zr = FP256BN.NewBIGint(0).Minus(zr)
	}

	return &fp256bnZr{zr}
}

func (p *Fp256bn) NewG1FromBytes(b []byte) driver.G1 {
	return &fp256bnG1{FP256BN.ECP_fromBytes(b)}
}

func (p *Fp256bn) NewG2FromBytes(b []byte) driver.G2 {
	return &fp256bnG2{FP256BN.ECP2_fromBytes(b)}
}

func (p *Fp256bn) NewGtFromBytes(b []byte) driver.Gt {
	return &fp256bnGt{FP256BN.FP12_fromBytes(b)}
}

func (p *Fp256bn) ModAdd(a, b, m driver.Zr) driver.Zr {
	c := a.Plus(b)
	c.Mod(m)
	return c
}

func (p *Fp256bn) ModSub(a, b, m driver.Zr) driver.Zr {
	return p.ModAdd(a, p.ModNeg(b, m), m)
}

func (p *Fp256bn) HashToZr(data []byte) driver.Zr {
	digest := sha256.Sum256(data)
	digestBig := FP256BN.FromBytes(digest[:])
	digestBig.Mod(FP256BN.NewBIGints(FP256BN.CURVE_Order))
	return &fp256bnZr{digestBig}
}

func (p *Fp256bn) HashToG1(data []byte) driver.G1 {
	return &fp256bnG1{FP256BN.Bls_hash(string(data))}
}

func (p *Fp256bn) Rand() (io.Reader, error) {
	seedLength := 32
	b := make([]byte, seedLength)
	_, err := r.Read(b)
	if err != nil {
		return nil, errors.Wrap(err, "error getting randomness for seed")
	}
	rng := amcl.NewRAND()
	rng.Clean()
	rng.Seed(seedLength, b)
	return &rand{rng}, nil
}

func (p *Fp256bn) NewRandomZr(rng io.Reader) driver.Zr {
	// curve order q
	q := FP256BN.NewBIGints(FP256BN.CURVE_Order)

	// Take random element in Zq
	return &fp256bnZr{FP256BN.Randomnum(q, rng.(*rand).R)}
}

/*********************************************************************/

type fp256bnG1 struct {
	*FP256BN.ECP
}

func (e *fp256bnG1) Clone(a driver.G1) {
	e.ECP.Copy(a.(*fp256bnG1).ECP)
}

func (e *fp256bnG1) Copy() driver.G1 {
	c := FP256BN.NewECP()
	c.Copy(e.ECP)
	return &fp256bnG1{c}
}

func (e *fp256bnG1) Add(a driver.G1) {
	e.ECP.Add(a.(*fp256bnG1).ECP)
}

func (e *fp256bnG1) Mul(a driver.Zr) driver.G1 {
	return &fp256bnG1{FP256BN.G1mul(e.ECP, a.(*fp256bnZr).BIG)}
}

func (e *fp256bnG1) Mul2(ee driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	return &fp256bnG1{e.ECP.Mul2(ee.(*fp256bnZr).BIG, Q.(*fp256bnG1).ECP, f.(*fp256bnZr).BIG)}
}

func (e *fp256bnG1) Equals(a driver.G1) bool {
	return e.ECP.Equals(a.(*fp256bnG1).ECP)
}

func (e *fp256bnG1) IsInfinity() bool {
	return e.ECP.Is_infinity()
}

func (e *fp256bnG1) Bytes() []byte {
	b := make([]byte, 2*int(FP256BN.MODBYTES)+1)
	e.ECP.ToBytes(b, false)
	return b
}

func (e *fp256bnG1) Sub(a driver.G1) {
	e.ECP.Sub(a.(*fp256bnG1).ECP)
}

var g1StrRegexp *regexp.Regexp = regexp.MustCompile(`^\(([0-9a-f]+),([0-9a-f]+)\)$`)

func (b *fp256bnG1) String() string {
	rawstr := b.ECP.ToString()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

/*********************************************************************/

type fp256bnG2 struct {
	*FP256BN.ECP2
}

func (e *fp256bnG2) Equals(a driver.G2) bool {
	return e.ECP2.Equals(a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Clone(a driver.G2) {
	e.ECP2.Copy(a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Copy() driver.G2 {
	c := FP256BN.NewECP2()
	c.Copy(e.ECP2)
	return &fp256bnG2{c}
}

func (e *fp256bnG2) Add(a driver.G2) {
	e.ECP2.Add(a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Sub(a driver.G2) {
	e.ECP2.Sub(a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Mul(a driver.Zr) driver.G2 {
	return &fp256bnG2{e.ECP2.Mul(a.(*fp256bnZr).BIG)}
}

func (e *fp256bnG2) Affine() {
	e.ECP2.Affine()
}

func (e *fp256bnG2) Bytes() []byte {
	b := make([]byte, 4*int(FP256BN.MODBYTES))
	e.ECP2.ToBytes(b)
	return b
}

func (b *fp256bnG2) String() string {
	return b.ECP2.ToString()
}

/*********************************************************************/

type rand struct {
	R *amcl.RAND
}

func (*rand) Read(p []byte) (n int, err error) {
	panic("not used")
}

/*********************************************************************/

func bigToBytes(big *FP256BN.BIG) []byte {
	ret := make([]byte, int(FP256BN.MODBYTES))
	big.ToBytes(ret)
	return ret
}
