/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gurvy

import (
	"fmt"
	"hash"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"golang.org/x/crypto/blake2b"
)

/*********************************************************************/

type bls12381G1 struct {
	bls12381.G1Affine
}

func (g *bls12381G1) Clone(a driver.G1) {
	raw := a.(*bls12381G1).G1Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bls12381G1) Copy() driver.G1 {
	c := &bls12381G1{}
	c.Set(&e.G1Affine)
	return c
}

func (g *bls12381G1) Add(a driver.G1) {
	j := bls12381.G1Jac{}
	j.FromAffine(&g.G1Affine)
	j.AddMixed((*bls12381.G1Affine)(&a.(*bls12381G1).G1Affine))
	g.G1Affine.FromJacobian(&j)
}

func (g *bls12381G1) Mul(a driver.Zr) driver.G1 {
	gc := &bls12381G1{}
	gc.G1Affine.ScalarMultiplication(&g.G1Affine, &a.(*common.BaseZr).Int)

	return gc
}

func (g *bls12381G1) Mul2(e driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	a := g.Mul(e)
	b := Q.Mul(f)
	a.Add(b)

	return a
}

func (g *bls12381G1) Equals(a driver.G1) bool {
	return g.G1Affine.Equal(&a.(*bls12381G1).G1Affine)
}

func (g *bls12381G1) Bytes() []byte {
	raw := g.G1Affine.RawBytes()
	return raw[:]
}

func (g *bls12381G1) Compressed() []byte {
	raw := g.G1Affine.Bytes()
	return raw[:]
}

func (g *bls12381G1) Sub(a driver.G1) {
	j, k := bls12381.G1Jac{}, bls12381.G1Jac{}
	j.FromAffine(&g.G1Affine)
	k.FromAffine(&a.(*bls12381G1).G1Affine)
	j.SubAssign(&k)
	g.G1Affine.FromJacobian(&j)
}

func (g *bls12381G1) IsInfinity() bool {
	return g.G1Affine.IsInfinity()
}

func (g *bls12381G1) String() string {
	rawstr := g.G1Affine.String()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

func (g *bls12381G1) Neg() {
	g.G1Affine.Neg(&g.G1Affine)
}

/*********************************************************************/

type bls12381G2 struct {
	bls12381.G2Affine
}

func (g *bls12381G2) Clone(a driver.G2) {
	raw := a.(*bls12381G2).G2Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *bls12381G2) Copy() driver.G2 {
	c := &bls12381G2{}
	c.Set(&e.G2Affine)
	return c
}

func (g *bls12381G2) Mul(a driver.Zr) driver.G2 {
	gc := &bls12381G2{}
	gc.G2Affine.ScalarMultiplication(&g.G2Affine, &a.(*common.BaseZr).Int)

	return gc
}

func (g *bls12381G2) Add(a driver.G2) {
	j := bls12381.G2Jac{}
	j.FromAffine(&g.G2Affine)
	j.AddMixed((*bls12381.G2Affine)(&a.(*bls12381G2).G2Affine))
	g.G2Affine.FromJacobian(&j)
}

func (g *bls12381G2) Sub(a driver.G2) {
	j := bls12381.G2Jac{}
	j.FromAffine(&g.G2Affine)
	aJac := bls12381.G2Jac{}
	aJac.FromAffine((*bls12381.G2Affine)(&a.(*bls12381G2).G2Affine))
	j.SubAssign(&aJac)
	g.G2Affine.FromJacobian(&j)
}

func (g *bls12381G2) Affine() {
	// we're always affine
}

func (g *bls12381G2) Bytes() []byte {
	raw := g.G2Affine.RawBytes()
	return raw[:]
}

func (g *bls12381G2) Compressed() []byte {
	raw := g.G2Affine.Bytes()
	return raw[:]
}

func (g *bls12381G2) String() string {
	return g.G2Affine.String()
}

func (g *bls12381G2) Equals(a driver.G2) bool {
	return g.G2Affine.Equal(&a.(*bls12381G2).G2Affine)
}

/*********************************************************************/

type bls12381Gt struct {
	bls12381.GT
}

func (g *bls12381Gt) Exp(x driver.Zr) driver.Gt {
	copy := bls12381.GT{}
	return &bls12381Gt{*copy.Exp(g.GT, &x.(*common.BaseZr).Int)}
}

func (g *bls12381Gt) Equals(a driver.Gt) bool {
	return g.GT.Equal(&a.(*bls12381Gt).GT)
}

func (g *bls12381Gt) Inverse() {
	g.GT.Inverse(&g.GT)
}

func (g *bls12381Gt) Mul(a driver.Gt) {
	g.GT.Mul(&g.GT, &a.(*bls12381Gt).GT)
}

func (g *bls12381Gt) IsUnity() bool {
	unity := bls12381.GT{}
	unity.SetOne()

	return unity.Equal(&g.GT)
}

func (g *bls12381Gt) ToString() string {
	return g.GT.String()
}

func (g *bls12381Gt) Bytes() []byte {
	raw := g.GT.Bytes()
	return raw[:]
}

/*********************************************************************/

func NewBls12_381() *Bls12_381 {
	return &Bls12_381{common.CurveBase{Modulus: *fr.Modulus()}}
}

func NewBls12_381BBS() *Bls12_381BBS {
	return &Bls12_381BBS{*NewBls12_381()}
}

type Bls12_381 struct {
	common.CurveBase
}

type Bls12_381BBS struct {
	Bls12_381
}

func (c *Bls12_381) Pairing(p2 driver.G2, p1 driver.G1) driver.Gt {
	t, err := bls12381.MillerLoop([]bls12381.G1Affine{p1.(*bls12381G1).G1Affine}, []bls12381.G2Affine{p2.(*bls12381G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing failed [%s]", err.Error()))
	}

	return &bls12381Gt{t}
}

func (c *Bls12_381) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	t, err := bls12381.MillerLoop([]bls12381.G1Affine{p1a.(*bls12381G1).G1Affine, p1b.(*bls12381G1).G1Affine}, []bls12381.G2Affine{p2a.(*bls12381G2).G2Affine, p2b.(*bls12381G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing 2 failed [%s]", err.Error()))
	}

	return &bls12381Gt{t}
}

func (c *Bls12_381) FExp(a driver.Gt) driver.Gt {
	return &bls12381Gt{bls12381.FinalExponentiation(&a.(*bls12381Gt).GT)}
}

var g1Bytes12_381 [48]byte
var g2Bytes12_381 [96]byte

func init() {
	_, _, g1, g2 := bls12381.Generators()
	g1Bytes12_381 = g1.Bytes()
	g2Bytes12_381 = g2.Bytes()
}

func (c *Bls12_381) GenG1() driver.G1 {
	r := &bls12381G1{}
	_, err := r.SetBytes(g1Bytes12_381[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Bls12_381) GenG2() driver.G2 {
	r := &bls12381G2{}
	_, err := r.SetBytes(g2Bytes12_381[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Bls12_381) GenGt() driver.Gt {
	g1 := c.GenG1()
	g2 := c.GenG2()
	gengt := c.Pairing(g2, g1)
	gengt = c.FExp(gengt)
	return gengt
}

func (c *Bls12_381) CoordinateByteSize() int {
	return bls12381.SizeOfG1AffineCompressed
}

func (c *Bls12_381) G1ByteSize() int {
	return bls12381.SizeOfG1AffineUncompressed
}

func (c *Bls12_381) CompressedG1ByteSize() int {
	return bls12381.SizeOfG1AffineCompressed
}

func (c *Bls12_381) G2ByteSize() int {
	return bls12381.SizeOfG2AffineUncompressed
}

func (c *Bls12_381) CompressedG2ByteSize() int {
	return bls12381.SizeOfG2AffineCompressed
}

func (c *Bls12_381) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (c *Bls12_381) NewG1() driver.G1 {
	return &bls12381G1{}
}

func (c *Bls12_381) NewG2() driver.G2 {
	return &bls12381G2{}
}

func (c *Bls12_381) NewG1FromBytes(b []byte) driver.G1 {
	v := &bls12381G1{}
	_, err := v.G1Affine.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_381) NewG2FromBytes(b []byte) driver.G2 {
	v := &bls12381G2{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_381) NewG1FromCompressed(b []byte) driver.G1 {
	v := &bls12381G1{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_381) NewG2FromCompressed(b []byte) driver.G2 {
	v := &bls12381G2{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_381) NewGtFromBytes(b []byte) driver.Gt {
	v := &bls12381Gt{}
	err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Bls12_381) HashToG1(data []byte) driver.G1 {
	g1, err := bls12381.HashToG1(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bls12381G1{g1}
}

func (c *Bls12_381) HashToG2(data []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &bls12381G2{g2}
}

func (p *Bls12_381) HashToG1WithDomain(data, domain []byte) driver.G1 {
	g1, err := bls12381.HashToG1(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bls12381G1{g1}
}

func (p *Bls12_381) HashToG2WithDomain(data, domain []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &bls12381G2{g2}
}

func (c *Bls12_381BBS) HashToG1(data []byte) driver.G1 {
	hashFunc := func() hash.Hash {
		// We pass a null key so error is impossible here.
		h, _ := blake2b.New512(nil) //nolint:errcheck
		return h
	}

	g1, err := HashToG1GenericBESwu(data, []byte{}, hashFunc)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bls12381G1{g1}
}

func (c *Bls12_381BBS) HashToG2(data []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &bls12381G2{g2}
}

func (p *Bls12_381BBS) HashToG1WithDomain(data, domain []byte) driver.G1 {
	hashFunc := func() hash.Hash {
		// We pass a null key so error is impossible here.
		h, _ := blake2b.New512(nil) //nolint:errcheck
		return h
	}

	g1, err := HashToG1GenericBESwu(data, domain, hashFunc)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &bls12381G1{g1}
}

func (p *Bls12_381BBS) HashToG2WithDomain(data, domain []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &bls12381G2{g2}
}
