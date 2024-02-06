/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gurvy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

/*********************************************************************/

type bn254G1 struct {
	bn254.G1Affine
}

func (g *bn254G1) Clone(a driver.G1) {
	raw := a.(*bn254G1).G1Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bn254G1) Copy() driver.G1 {
	c := &bn254G1{}
	c.Set(&e.G1Affine)
	return c
}

func (g *bn254G1) Add(a driver.G1) {
	j := bn254.G1Jac{}
	j.FromAffine(&g.G1Affine)
	j.AddMixed((*bn254.G1Affine)(&a.(*bn254G1).G1Affine))
	g.G1Affine.FromJacobian(&j)
}

func (g *bn254G1) Mul(a driver.Zr) driver.G1 {
	res := &bn254G1{}
	res.G1Affine.ScalarMultiplication(&g.G1Affine, &a.(*common.BaseZr).Int)

	return res
}

func (g *bn254G1) Mul2(e driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	a := g.Mul(e)
	b := Q.Mul(f)
	a.Add(b)

	return a
}

func (g *bn254G1) Equals(a driver.G1) bool {
	return g.G1Affine.Equal(&a.(*bn254G1).G1Affine)
}

func (g *bn254G1) Bytes() []byte {
	raw := g.G1Affine.RawBytes()
	return raw[:]
}

func (g *bn254G1) Compressed() []byte {
	raw := g.G1Affine.Bytes()
	return raw[:]
}

func (g *bn254G1) Sub(a driver.G1) {
	j, k := bn254.G1Jac{}, bn254.G1Jac{}
	j.FromAffine(&g.G1Affine)
	k.FromAffine(&a.(*bn254G1).G1Affine)
	j.SubAssign(&k)
	g.G1Affine.FromJacobian(&j)
}

func (g *bn254G1) IsInfinity() bool {
	return g.G1Affine.IsInfinity()
}

var g1StrRegexp *regexp.Regexp = regexp.MustCompile(`^E\([[]([0-9]+),([0-9]+)[]]\)$`)

func (g *bn254G1) String() string {
	rawstr := g.G1Affine.String()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

func (g *bn254G1) Neg() {
	g.G1Affine.Neg(&g.G1Affine)
}

/*********************************************************************/

type bn254G2 struct {
	bn254.G2Affine
}

func (g *bn254G2) Clone(a driver.G2) {
	raw := a.(*bn254G2).G2Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bn254G2) Copy() driver.G2 {
	c := &bn254G2{}
	c.Set(&e.G2Affine)
	return c
}

func (g *bn254G2) Mul(a driver.Zr) driver.G2 {
	gc := &bn254G2{}
	gc.G2Affine.ScalarMultiplication(&g.G2Affine, &a.(*common.BaseZr).Int)

	return gc
}

func (g *bn254G2) Add(a driver.G2) {
	j := bn254.G2Jac{}
	j.FromAffine(&g.G2Affine)
	j.AddMixed((*bn254.G2Affine)(&a.(*bn254G2).G2Affine))
	g.G2Affine.FromJacobian(&j)
}

func (g *bn254G2) Sub(a driver.G2) {
	j := bn254.G2Jac{}
	j.FromAffine(&g.G2Affine)
	aJac := bn254.G2Jac{}
	aJac.FromAffine((*bn254.G2Affine)(&a.(*bn254G2).G2Affine))
	j.SubAssign(&aJac)
	g.G2Affine.FromJacobian(&j)
}

func (g *bn254G2) Affine() {
	// we're always affine
}

func (g *bn254G2) Bytes() []byte {
	raw := g.G2Affine.RawBytes()
	return raw[:]
}

func (g *bn254G2) Compressed() []byte {
	raw := g.G2Affine.Bytes()
	return raw[:]
}

func (g *bn254G2) String() string {
	return g.G2Affine.String()
}

func (g *bn254G2) Equals(a driver.G2) bool {
	return g.G2Affine.Equal(&a.(*bn254G2).G2Affine)
}

/*********************************************************************/

type bn254Gt struct {
	bn254.GT
}

func (g *bn254Gt) Exp(x driver.Zr) driver.Gt {
	copy := bn254.GT{}
	return &bn254Gt{*copy.Exp(g.GT, &x.(*common.BaseZr).Int)}
}

func (g *bn254Gt) Equals(a driver.Gt) bool {
	return g.GT.Equal(&a.(*bn254Gt).GT)
}

func (g *bn254Gt) Inverse() {
	g.GT.Inverse(&g.GT)
}

func (g *bn254Gt) Mul(a driver.Gt) {
	g.GT.Mul(&g.GT, &a.(*bn254Gt).GT)
}

func (g *bn254Gt) IsUnity() bool {
	unity := bn254.GT{}
	unity.SetOne()

	return unity.Equal(&g.GT)
}

func (g *bn254Gt) ToString() string {
	return g.GT.String()
}

func (g *bn254Gt) Bytes() []byte {
	raw := g.GT.Bytes()
	return raw[:]
}

/*********************************************************************/

func NewBn254() *Bn254 {
	return &Bn254{common.CurveBase{Modulus: *fr.Modulus()}}
}

type Bn254 struct {
	common.CurveBase
}

func (c *Bn254) Pairing(p2 driver.G2, p1 driver.G1) driver.Gt {
	t, err := bn254.MillerLoop([]bn254.G1Affine{p1.(*bn254G1).G1Affine}, []bn254.G2Affine{p2.(*bn254G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing failed [%s]", err.Error()))
	}

	return &bn254Gt{t}
}

func (c *Bn254) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	t, err := bn254.MillerLoop([]bn254.G1Affine{p1a.(*bn254G1).G1Affine, p1b.(*bn254G1).G1Affine}, []bn254.G2Affine{p2a.(*bn254G2).G2Affine, p2b.(*bn254G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing 2 failed [%s]", err.Error()))
	}

	return &bn254Gt{t}
}

func (c *Bn254) FExp(a driver.Gt) driver.Gt {
	return &bn254Gt{bn254.FinalExponentiation(&a.(*bn254Gt).GT)}
}

var g1Bytes254 [32]byte
var g2Bytes254 [64]byte

func init() {
	_, _, g1, g2 := bn254.Generators()
	g1Bytes254 = g1.Bytes()
	g2Bytes254 = g2.Bytes()
}

func (c *Bn254) GenG1() driver.G1 {
	r := &bn254G1{}
	_, err := r.SetBytes(g1Bytes254[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Bn254) GenG2() driver.G2 {
	r := &bn254G2{}
	_, err := r.SetBytes(g2Bytes254[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Bn254) GenGt() driver.Gt {
	g1 := c.GenG1()
	g2 := c.GenG2()
	gengt := c.Pairing(g2, g1)
	gengt = c.FExp(gengt)
	return gengt
}

func (c *Bn254) CoordinateByteSize() int {
	return bn254.SizeOfG1AffineCompressed
}

func (c *Bn254) G1ByteSize() int {
	return bn254.SizeOfG1AffineUncompressed
}

func (c *Bn254) CompressedG1ByteSize() int {
	return bn254.SizeOfG1AffineCompressed
}

func (c *Bn254) G2ByteSize() int {
	return bn254.SizeOfG2AffineUncompressed
}

func (c *Bn254) CompressedG2ByteSize() int {
	return bn254.SizeOfG2AffineCompressed
}

func (c *Bn254) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (c *Bn254) NewG1() driver.G1 {
	return &bn254G1{}
}

func (c *Bn254) NewG2() driver.G2 {
	return &bn254G2{}
}

func (c *Bn254) NewG1FromBytes(b []byte) driver.G1 {
	v := &bn254G1{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bn254) NewG2FromBytes(b []byte) driver.G2 {
	v := &bn254G2{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bn254) NewG1FromCompressed(b []byte) driver.G1 {
	v := &bn254G1{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bn254) NewG2FromCompressed(b []byte) driver.G2 {
	v := &bn254G2{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bn254) NewGtFromBytes(b []byte) driver.Gt {
	v := &bn254Gt{}
	err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bn254) HashToG1(data []byte) driver.G1 {
	g1, err := bn254.HashToG1(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bn254G1{g1}
}

func (p *Bn254) HashToG1WithDomain(data, domain []byte) driver.G1 {
	g1, err := bn254.HashToG1(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bn254G1{g1}
}
