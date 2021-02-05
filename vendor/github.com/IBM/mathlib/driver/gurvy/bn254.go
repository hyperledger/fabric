/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gurvy

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

/*********************************************************************/

type bn254Zr struct {
	*big.Int
}

func (z *bn254Zr) Plus(a driver.Zr) driver.Zr {
	return &bn254Zr{new(big.Int).Add(z.Int, a.(*bn254Zr).Int)}
}

func (z *bn254Zr) Mod(a driver.Zr) {
	z.Int.Mod(z.Int, a.(*bn254Zr).Int)
}

func (z *bn254Zr) PowMod(x driver.Zr) driver.Zr {
	return &bn254Zr{new(big.Int).Exp(z.Int, x.(*bn254Zr).Int, fr.Modulus())}
}

func (z *bn254Zr) InvModP(a driver.Zr) {
	z.Int.ModInverse(z.Int, a.(*bn254Zr).Int)
}

func (z *bn254Zr) Bytes() []byte {
	return common.BigToBytes(z.Int)
}

func (z *bn254Zr) Equals(a driver.Zr) bool {
	return z.Int.Cmp(a.(*bn254Zr).Int) == 0
}

func (z *bn254Zr) Copy() driver.Zr {
	return &bn254Zr{new(big.Int).Set(z.Int)}
}

func (z *bn254Zr) Clone(a driver.Zr) {
	raw := a.(*bn254Zr).Int.Bytes()
	z.Int.SetBytes(raw)
}

func (z *bn254Zr) String() string {
	return z.Int.Text(16)
}

/*********************************************************************/

type bn254G1 struct {
	*bn254.G1Affine
}

func (g *bn254G1) Clone(a driver.G1) {
	raw := a.(*bn254G1).G1Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bn254G1) Copy() driver.G1 {
	c := &bn254.G1Affine{}
	c.Set(e.G1Affine)
	return &bn254G1{c}
}

func (g *bn254G1) Add(a driver.G1) {
	j := &bn254.G1Jac{}
	j.FromAffine(g.G1Affine)
	j.AddMixed((*bn254.G1Affine)(a.(*bn254G1).G1Affine))
	g.G1Affine.FromJacobian(j)
}

func (g *bn254G1) Mul(a driver.Zr) driver.G1 {
	gc := &bn254G1{&bn254.G1Affine{}}
	gc.Clone(g)
	gc.G1Affine.ScalarMultiplication(g.G1Affine, a.(*bn254Zr).Int)

	return gc
}

func (g *bn254G1) Mul2(e driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	a := g.Mul(e)
	b := Q.Mul(f)
	a.Add(b)

	return a
}

func (g *bn254G1) Equals(a driver.G1) bool {
	return g.G1Affine.Equal(a.(*bn254G1).G1Affine)
}

func (g *bn254G1) Bytes() []byte {
	raw := g.G1Affine.RawBytes()
	return raw[:]
}

func (g *bn254G1) Sub(a driver.G1) {
	j, k := &bn254.G1Jac{}, &bn254.G1Jac{}
	j.FromAffine(g.G1Affine)
	k.FromAffine(a.(*bn254G1).G1Affine)
	j.SubAssign(k)
	g.G1Affine.FromJacobian(j)
}

func (g *bn254G1) IsInfinity() bool {
	return g.G1Affine.IsInfinity()
}

var g1StrRegexp *regexp.Regexp = regexp.MustCompile(`^E\([[]([0-9]+),([0-9]+)[]]\),$`)

func (g *bn254G1) String() string {
	rawstr := g.G1Affine.String()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

/*********************************************************************/

type bn254G2 struct {
	*bn254.G2Affine
}

func (g *bn254G2) Clone(a driver.G2) {
	raw := a.(*bn254G2).G2Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bn254G2) Copy() driver.G2 {
	c := &bn254.G2Affine{}
	c.Set(e.G2Affine)
	return &bn254G2{c}
}

func (g *bn254G2) Mul(a driver.Zr) driver.G2 {
	gc := &bn254G2{&bn254.G2Affine{}}
	gc.Clone(g)
	gc.G2Affine.ScalarMultiplication(g.G2Affine, a.(*bn254Zr).Int)

	return gc
}

func (g *bn254G2) Add(a driver.G2) {
	j := &bn254.G2Jac{}
	j.FromAffine(g.G2Affine)
	j.AddMixed((*bn254.G2Affine)(a.(*bn254G2).G2Affine))
	g.G2Affine.FromJacobian(j)
}

func (g *bn254G2) Sub(a driver.G2) {
	j := &bn254.G2Jac{}
	j.FromAffine(g.G2Affine)
	aJac := &bn254.G2Jac{}
	aJac.FromAffine((*bn254.G2Affine)(a.(*bn254G2).G2Affine))
	j.SubAssign(aJac)
	g.G2Affine.FromJacobian(j)
}

func (g *bn254G2) Affine() {
	// we're always affine
}

func (g *bn254G2) Bytes() []byte {
	raw := g.G2Affine.RawBytes()
	return raw[:]
}

func (g *bn254G2) String() string {
	return g.G2Affine.String()
}

func (g *bn254G2) Equals(a driver.G2) bool {
	return g.G2Affine.Equal(a.(*bn254G2).G2Affine)
}

/*********************************************************************/

type bn254Gt struct {
	*bn254.GT
}

func (g *bn254Gt) Equals(a driver.Gt) bool {
	return g.GT.Equal(a.(*bn254Gt).GT)
}

func (g *bn254Gt) Inverse() {
	g.GT.Inverse(g.GT)
}

func (g *bn254Gt) Mul(a driver.Gt) {
	g.GT.Mul(g.GT, a.(*bn254Gt).GT)
}

func (g *bn254Gt) IsUnity() bool {
	unity := &bn254.GT{}
	unity.SetOne()

	return unity.Equal(g.GT)
}

func (g *bn254Gt) ToString() string {
	return g.GT.String()
}

func (g *bn254Gt) Bytes() []byte {
	raw := g.GT.Bytes()
	return raw[:]
}

/*********************************************************************/

type Bn254 struct {
}

func (c *Bn254) Pairing(p2 driver.G2, p1 driver.G1) driver.Gt {
	t, err := bn254.MillerLoop([]bn254.G1Affine{*p1.(*bn254G1).G1Affine}, []bn254.G2Affine{*p2.(*bn254G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing failed [%s]", err.Error()))
	}

	return &bn254Gt{&t}
}

func (c *Bn254) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	t, err := bn254.MillerLoop([]bn254.G1Affine{*p1a.(*bn254G1).G1Affine, *p1b.(*bn254G1).G1Affine}, []bn254.G2Affine{*p2a.(*bn254G2).G2Affine, *p2b.(*bn254G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing 2 failed [%s]", err.Error()))
	}

	return &bn254Gt{&t}
}

func (c *Bn254) FExp(a driver.Gt) driver.Gt {
	gt := bn254.FinalExponentiation(a.(*bn254Gt).GT)
	return &bn254Gt{&gt}
}

func (*Bn254) ModAdd(a, b, m driver.Zr) driver.Zr {
	c := a.Plus(b)
	c.Mod(m)
	return c
}

func (c *Bn254) ModSub(a, b, m driver.Zr) driver.Zr {
	return c.ModAdd(a, c.ModNeg(b, m), m)
}

func (c *Bn254) ModNeg(a1, m driver.Zr) driver.Zr {
	a := a1.Copy()
	a.Mod(m)
	return &bn254Zr{a.(*bn254Zr).Int.Sub(m.(*bn254Zr).Int, a.(*bn254Zr).Int)}
}

func (c *Bn254) ModMul(a1, b1, m driver.Zr) driver.Zr {
	a := a1.Copy()
	b := b1.Copy()
	a.Mod(m)
	b.Mod(m)
	return &bn254Zr{a.(*bn254Zr).Int.Mul(a.(*bn254Zr).Int, b.(*bn254Zr).Int)}
}

func (c *Bn254) GenG1() driver.G1 {
	_, _, g1, _ := bn254.Generators()
	raw := g1.Bytes()

	r := &bn254.G1Affine{}
	_, err := r.SetBytes(raw[:])
	if err != nil {
		panic("could not generate point")
	}

	return &bn254G1{r}
}

func (c *Bn254) GenG2() driver.G2 {
	_, _, _, g2 := bn254.Generators()
	raw := g2.Bytes()

	r := &bn254.G2Affine{}
	_, err := r.SetBytes(raw[:])
	if err != nil {
		panic("could not generate point")
	}

	return &bn254G2{r}
}

func (c *Bn254) GenGt() driver.Gt {
	g1 := c.GenG1()
	g2 := c.GenG2()
	gengt := c.Pairing(g2, g1)
	gengt = c.FExp(gengt)
	return gengt
}

func (c *Bn254) GroupOrder() driver.Zr {
	return &bn254Zr{fr.Modulus()}
}

func (c *Bn254) FieldBytes() int {
	return 32
}

func (c *Bn254) NewG1() driver.G1 {
	return &bn254G1{&bn254.G1Affine{}}
}

func (c *Bn254) NewG2() driver.G2 {
	return &bn254G2{&bn254.G2Affine{}}
}

func (c *Bn254) NewG1FromCoords(ix, iy driver.Zr) driver.G1 {
	return nil
}

func (c *Bn254) NewZrFromBytes(b []byte) driver.Zr {
	return &bn254Zr{new(big.Int).SetBytes(b)}
}

func (c *Bn254) NewZrFromInt(i int64) driver.Zr {
	return &bn254Zr{big.NewInt(i)}
}

func (c *Bn254) NewG1FromBytes(b []byte) driver.G1 {
	v := &bn254.G1Affine{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bn254G1{v}
}

func (c *Bn254) NewG2FromBytes(b []byte) driver.G2 {
	v := &bn254.G2Affine{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bn254G2{v}
}

func (c *Bn254) NewGtFromBytes(b []byte) driver.Gt {
	v := &bn254.GT{}
	err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bn254Gt{v}
}

func (c *Bn254) HashToZr(data []byte) driver.Zr {
	digest := sha256.Sum256(data)
	digestBig := c.NewZrFromBytes(digest[:])
	digestBig.Mod(c.GroupOrder())
	return digestBig
}

func (c *Bn254) HashToG1(data []byte) driver.G1 {
	g1, err := bn254.HashToCurveG1Svdw(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bn254G1{&g1}
}

func (c *Bn254) NewRandomZr(rng io.Reader) driver.Zr {
	res := new(big.Int)
	v := &fr.Element{}
	_, err := v.SetRandom()
	if err != nil {
		panic(err)
	}

	return &bn254Zr{v.ToBigIntRegular(res)}
}

func (c *Bn254) Rand() (io.Reader, error) {
	return rand.Reader, nil
}
