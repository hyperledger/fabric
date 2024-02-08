/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gurvy

import (
	"fmt"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	bls12377 "github.com/consensys/gnark-crypto/ecc/bls12-377"
	"github.com/consensys/gnark-crypto/ecc/bls12-377/fr"
)

/*********************************************************************/

type bls12377G1 struct {
	bls12377.G1Affine
}

func (g *bls12377G1) Clone(a driver.G1) {
	raw := a.(*bls12377G1).G1Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bls12377G1) Copy() driver.G1 {
	c := &bls12377G1{}
	c.Set(&e.G1Affine)
	return c
}

func (g *bls12377G1) Add(a driver.G1) {
	j := bls12377.G1Jac{}
	j.FromAffine(&g.G1Affine)
	j.AddMixed((*bls12377.G1Affine)(&a.(*bls12377G1).G1Affine))
	g.G1Affine.FromJacobian(&j)
}

func (g *bls12377G1) Mul(a driver.Zr) driver.G1 {
	ret := &bls12377G1{}
	ret.G1Affine.ScalarMultiplication(&g.G1Affine, &a.(*common.BaseZr).Int)

	return ret
}

func (g *bls12377G1) Mul2(e driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	a := g.Mul(e)
	b := Q.Mul(f)
	a.Add(b)

	return a
}

func (g *bls12377G1) Equals(a driver.G1) bool {
	return g.G1Affine.Equal(&a.(*bls12377G1).G1Affine)
}

func (g *bls12377G1) Bytes() []byte {
	raw := g.G1Affine.RawBytes()
	return raw[:]
}

func (g *bls12377G1) Compressed() []byte {
	raw := g.G1Affine.Bytes()
	return raw[:]
}

func (g *bls12377G1) Sub(a driver.G1) {
	j, k := bls12377.G1Jac{}, bls12377.G1Jac{}
	j.FromAffine(&g.G1Affine)
	k.FromAffine(&a.(*bls12377G1).G1Affine)
	j.SubAssign(&k)
	g.G1Affine.FromJacobian(&j)
}

func (g *bls12377G1) IsInfinity() bool {
	return g.G1Affine.IsInfinity()
}

func (g *bls12377G1) String() string {
	rawstr := g.G1Affine.String()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

func (g *bls12377G1) Neg() {
	g.G1Affine.Neg(&g.G1Affine)
}

/*********************************************************************/

type bls12377G2 struct {
	bls12377.G2Affine
}

func (g *bls12377G2) Clone(a driver.G2) {
	raw := a.(*bls12377G2).G2Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bls12377G2) Copy() driver.G2 {
	c := &bls12377G2{}
	c.Set(&e.G2Affine)
	return c
}

func (g *bls12377G2) Mul(a driver.Zr) driver.G2 {
	gc := &bls12377G2{}
	gc.G2Affine.ScalarMultiplication(&g.G2Affine, &a.(*common.BaseZr).Int)

	return gc
}

func (g *bls12377G2) Add(a driver.G2) {
	j := bls12377.G2Jac{}
	j.FromAffine(&g.G2Affine)
	j.AddMixed((*bls12377.G2Affine)(&a.(*bls12377G2).G2Affine))
	g.G2Affine.FromJacobian(&j)
}

func (g *bls12377G2) Sub(a driver.G2) {
	j := bls12377.G2Jac{}
	j.FromAffine(&g.G2Affine)
	aJac := bls12377.G2Jac{}
	aJac.FromAffine((*bls12377.G2Affine)(&a.(*bls12377G2).G2Affine))
	j.SubAssign(&aJac)
	g.G2Affine.FromJacobian(&j)
}

func (g *bls12377G2) Affine() {
	// we're always affine
}

func (g *bls12377G2) Bytes() []byte {
	raw := g.G2Affine.RawBytes()
	return raw[:]
}

func (g *bls12377G2) Compressed() []byte {
	raw := g.G2Affine.Bytes()
	return raw[:]
}

func (g *bls12377G2) String() string {
	return g.G2Affine.String()
}

func (g *bls12377G2) Equals(a driver.G2) bool {
	return g.G2Affine.Equal(&a.(*bls12377G2).G2Affine)
}

/*********************************************************************/

type bls12377Gt struct {
	bls12377.GT
}

func (g *bls12377Gt) Exp(x driver.Zr) driver.Gt {
	copy := bls12377.GT{}
	return &bls12377Gt{*copy.Exp(g.GT, &x.(*common.BaseZr).Int)}
}

func (g *bls12377Gt) Equals(a driver.Gt) bool {
	return g.GT.Equal(&a.(*bls12377Gt).GT)
}

func (g *bls12377Gt) Inverse() {
	g.GT.Inverse(&g.GT)
}

func (g *bls12377Gt) Mul(a driver.Gt) {
	g.GT.Mul(&g.GT, &a.(*bls12377Gt).GT)
}

func (g *bls12377Gt) IsUnity() bool {
	unity := bls12377.GT{}
	unity.SetOne()

	return unity.Equal(&g.GT)
}

func (g *bls12377Gt) ToString() string {
	return g.GT.String()
}

func (g *bls12377Gt) Bytes() []byte {
	raw := g.GT.Bytes()
	return raw[:]
}

/*********************************************************************/

func NewBls12_377() *Bls12_377 {
	return &Bls12_377{common.CurveBase{Modulus: *fr.Modulus()}}
}

type Bls12_377 struct {
	common.CurveBase
}

func (c *Bls12_377) Pairing(p2 driver.G2, p1 driver.G1) driver.Gt {
	t, err := bls12377.MillerLoop([]bls12377.G1Affine{p1.(*bls12377G1).G1Affine}, []bls12377.G2Affine{p2.(*bls12377G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing failed [%s]", err.Error()))
	}

	return &bls12377Gt{t}
}

func (c *Bls12_377) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	t, err := bls12377.MillerLoop([]bls12377.G1Affine{p1a.(*bls12377G1).G1Affine, p1b.(*bls12377G1).G1Affine}, []bls12377.G2Affine{p2a.(*bls12377G2).G2Affine, p2b.(*bls12377G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing 2 failed [%s]", err.Error()))
	}

	return &bls12377Gt{t}
}

func (c *Bls12_377) FExp(a driver.Gt) driver.Gt {
	return &bls12377Gt{bls12377.FinalExponentiation(&a.(*bls12377Gt).GT)}
}

var g1Bytes12_377 [48]byte
var g2Bytes12_377 [96]byte

func init() {
	_, _, g1, g2 := bls12377.Generators()
	g1Bytes12_377 = g1.Bytes()
	g2Bytes12_377 = g2.Bytes()
}

func (c *Bls12_377) GenG1() driver.G1 {
	r := &bls12377G1{}
	_, err := r.SetBytes(g1Bytes12_377[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Bls12_377) GenG2() driver.G2 {
	r := &bls12377G2{}
	_, err := r.SetBytes(g2Bytes12_377[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Bls12_377) GenGt() driver.Gt {
	g1 := c.GenG1()
	g2 := c.GenG2()
	gengt := c.Pairing(g2, g1)
	gengt = c.FExp(gengt)
	return gengt
}

func (c *Bls12_377) CoordinateByteSize() int {
	return bls12377.SizeOfG1AffineCompressed
}

func (c *Bls12_377) G1ByteSize() int {
	return bls12377.SizeOfG1AffineUncompressed
}

func (c *Bls12_377) CompressedG1ByteSize() int {
	return bls12377.SizeOfG1AffineCompressed
}

func (c *Bls12_377) G2ByteSize() int {
	return bls12377.SizeOfG2AffineUncompressed
}

func (c *Bls12_377) CompressedG2ByteSize() int {
	return bls12377.SizeOfG2AffineCompressed
}

func (c *Bls12_377) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (c *Bls12_377) NewG1() driver.G1 {
	return &bls12377G1{}
}

func (c *Bls12_377) NewG2() driver.G2 {
	return &bls12377G2{}
}

func (c *Bls12_377) NewG1FromBytes(b []byte) driver.G1 {
	v := &bls12377G1{}
	_, err := v.G1Affine.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_377) NewG2FromBytes(b []byte) driver.G2 {
	v := &bls12377G2{}
	_, err := v.G2Affine.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_377) NewG1FromCompressed(b []byte) driver.G1 {
	v := &bls12377G1{}
	_, err := v.G1Affine.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_377) NewG2FromCompressed(b []byte) driver.G2 {
	v := &bls12377G2{}
	_, err := v.G2Affine.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_377) NewGtFromBytes(b []byte) driver.Gt {
	v := &bls12377Gt{}
	err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_377) HashToG1(data []byte) driver.G1 {
	g1, err := bls12377.HashToG1(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bls12377G1{g1}
}

func (p *Bls12_377) HashToG1WithDomain(data, domain []byte) driver.G1 {
	g1, err := bls12377.HashToG1(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bls12377G1{g1}
}
