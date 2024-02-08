/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kilic

import (
	"fmt"
	"math/big"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	bls12381 "github.com/kilic/bls12-381"
)

/*********************************************************************/

type bls12_381G1 struct {
	bls12381.PointG1
	bls12381.G1
}

func (g *bls12_381G1) Clone(a driver.G1) {
	g.Set(&a.(*bls12_381G1).PointG1)
}

func (e *bls12_381G1) Copy() driver.G1 {
	c := &bls12_381G1{G1: *bls12381.NewG1()}
	c.Set(&e.PointG1)
	return c
}

func (g *bls12_381G1) Add(a driver.G1) {
	g.G1.Add(&g.PointG1, &g.PointG1, &a.(*bls12_381G1).PointG1)
}

func (g *bls12_381G1) Mul(a driver.Zr) driver.G1 {
	g1 := bls12381.NewG1()
	res := g1.New()

	g1.MulScalarBig(res, &g.PointG1, &a.(*common.BaseZr).Int)

	return &bls12_381G1{
		G1:      *g1,
		PointG1: *res,
	}
}

func (g *bls12_381G1) Mul2(e driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	a := g.Mul(e)
	b := Q.Mul(f)
	a.Add(b)

	return a
}

func (g *bls12_381G1) Equals(a driver.G1) bool {
	g1 := bls12381.NewG1()
	return g1.Equal(&a.(*bls12_381G1).PointG1, &g.PointG1)
}

func (g *bls12_381G1) Bytes() []byte {
	g1 := bls12381.NewG1()
	raw := g1.ToUncompressed(&g.PointG1)
	return raw[:]
}

func (g *bls12_381G1) Compressed() []byte {
	raw := g.G1.ToCompressed(&g.PointG1)
	return raw[:]
}

func (g *bls12_381G1) Sub(a driver.G1) {
	g.G1.Sub(&g.PointG1, &g.PointG1, &a.(*bls12_381G1).PointG1)
}

func (g *bls12_381G1) IsInfinity() bool {
	return g.G1.IsZero(&g.PointG1)
}

func (g *bls12_381G1) String() string {
	gb := g.Bytes()
	x := new(big.Int).SetBytes(gb[:len(gb)/2])
	y := new(big.Int).SetBytes(gb[len(gb)/2:])

	return "(" + x.String() + "," + y.String() + ")"
}

func (g *bls12_381G1) Neg() {
	g.G1.Neg(&g.PointG1, &g.PointG1)
}

/*********************************************************************/

type bls12_381G2 struct {
	bls12381.PointG2
	bls12381.G2
}

func (g *bls12_381G2) Clone(a driver.G2) {
	g.Set(&a.(*bls12_381G2).PointG2)
}

func (e *bls12_381G2) Copy() driver.G2 {
	c := &bls12_381G2{
		G2: *bls12381.NewG2(),
	}
	c.Set(&e.PointG2)
	return c
}

func (g *bls12_381G2) Mul(a driver.Zr) driver.G2 {
	g2 := bls12381.NewG2()
	res := g2.New()

	g2.MulScalarBig(res, &g.PointG2, &a.(*common.BaseZr).Int)

	return &bls12_381G2{
		G2:      *g2,
		PointG2: *res,
	}
}

func (g *bls12_381G2) Add(a driver.G2) {
	g.G2.Add(&g.PointG2, &g.PointG2, &a.(*bls12_381G2).PointG2)
}

func (g *bls12_381G2) Sub(a driver.G2) {
	g.G2.Sub(&g.PointG2, &g.PointG2, &a.(*bls12_381G2).PointG2)
}

func (g *bls12_381G2) Affine() {
	g2 := bls12381.NewG2()
	g.PointG2 = *g2.Affine(&g.PointG2)
}

func (g *bls12_381G2) Bytes() []byte {
	g2 := bls12381.NewG2()
	raw := g2.ToUncompressed(&g.PointG2)
	return raw[:]
}

func (g *bls12_381G2) Compressed() []byte {
	g2 := bls12381.NewG2()
	raw := g2.ToCompressed(&g.PointG2)
	return raw[:]
}

func (g *bls12_381G2) String() string {
	// FIXME
	return ""
}

func (g *bls12_381G2) Equals(a driver.G2) bool {
	g2 := bls12381.NewG2()
	return g2.Equal(&a.(*bls12_381G2).PointG2, &g.PointG2)
}

/*********************************************************************/

type bls12_381Gt struct {
	bls12381.E
	bls12381.GT
	GTInitialised bool
}

func (g *bls12_381Gt) Exp(x driver.Zr) driver.Gt {
	gt := bls12381.NewGT()
	res := gt.New()
	gt.Exp(res, &g.E, &x.(*common.BaseZr).Int)

	return &bls12_381Gt{
		E:             *res,
		GT:            *gt,
		GTInitialised: true,
	}
}

func (g *bls12_381Gt) Equals(a driver.Gt) bool {
	return a.(*bls12_381Gt).E.Equal(&g.E)
}

func (g *bls12_381Gt) Inverse() {
	if !g.GTInitialised {
		g.GT = *bls12381.NewGT()
	}
	g.GT.Inverse(&g.E, &g.E)
}

func (g *bls12_381Gt) Mul(a driver.Gt) {
	if !g.GTInitialised {
		g.GT = *bls12381.NewGT()
	}
	g.GT.Mul(&g.E, &g.E, &a.(*bls12_381Gt).E)
}

func (g *bls12_381Gt) IsUnity() bool {
	return g.E.IsOne()
}

func (g *bls12_381Gt) ToString() string {
	// FIXME
	return ""
}

func (g *bls12_381Gt) Bytes() []byte {
	if !g.GTInitialised {
		g.GT = *bls12381.NewGT()
	}
	raw := g.GT.ToBytes(&g.E)
	return raw[:]
}

/*********************************************************************/

func NewBls12_381() *Bls12_381 {
	return &Bls12_381{common.CurveBase{Modulus: *bls12381.NewG1().Q()}}
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
	bls := bls12381.NewEngine()
	bls.AddPair(&p1.(*bls12_381G1).PointG1, &p2.(*bls12_381G2).PointG2)

	return &bls12_381Gt{
		E: *bls.Result(),
	}
}

func (c *Bls12_381) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	bls := bls12381.NewEngine()
	bls.AddPair(&p1a.(*bls12_381G1).PointG1, &p2a.(*bls12_381G2).PointG2)
	bls.AddPair(&p1b.(*bls12_381G1).PointG1, &p2b.(*bls12_381G2).PointG2)

	return &bls12_381Gt{
		E: *bls.Result(),
	}
}

func (c *Bls12_381) FExp(a driver.Gt) driver.Gt {
	return a
}

func (c *Bls12_381) GenG1() driver.G1 {
	g := bls12381.NewG1()
	g1 := g.One()
	return &bls12_381G1{
		G1:      *g,
		PointG1: *g1,
	}
}

func (c *Bls12_381) GenG2() driver.G2 {
	g := bls12381.NewG2()
	g2 := g.One()
	return &bls12_381G2{
		G2:      *g,
		PointG2: *g2,
	}
}

func (c *Bls12_381) GenGt() driver.Gt {
	g1 := c.GenG1()
	g2 := c.GenG2()
	gengt := c.Pairing(g2, g1)
	gengt = c.FExp(gengt)
	return gengt
}

func (c *Bls12_381) CoordinateByteSize() int {
	return fpByteSize
}

func (c *Bls12_381) G1ByteSize() int {
	return 2 * fpByteSize
}

func (c *Bls12_381) CompressedG1ByteSize() int {
	return fpByteSize
}

func (c *Bls12_381) G2ByteSize() int {
	return 4 * fpByteSize
}

func (c *Bls12_381) CompressedG2ByteSize() int {
	return 2 * fpByteSize
}

func (c *Bls12_381) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (c *Bls12_381) NewG1() driver.G1 {
	return &bls12_381G1{G1: *bls12381.NewG1()}
}

func (c *Bls12_381) NewG2() driver.G2 {
	return &bls12_381G2{G2: *bls12381.NewG2()}
}

func (c *Bls12_381) NewG1FromBytes(b []byte) driver.G1 {
	g1 := bls12381.NewG1()
	p, err := g1.FromUncompressed(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bls12_381G1{
		PointG1: *p,
		G1:      *g1,
	}
}

func (c *Bls12_381) NewG2FromBytes(b []byte) driver.G2 {
	g2 := bls12381.NewG2()
	p, err := g2.FromUncompressed(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bls12_381G2{
		G2:      *g2,
		PointG2: *p,
	}
}

func (c *Bls12_381) NewG1FromCompressed(b []byte) driver.G1 {
	g1 := bls12381.NewG1()
	p, err := g1.FromCompressed(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bls12_381G1{
		PointG1: *p,
		G1:      *g1,
	}
}

func (c *Bls12_381) NewG2FromCompressed(b []byte) driver.G2 {
	g2 := bls12381.NewG2()
	p, err := g2.FromCompressed(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bls12_381G2{
		G2:      *g2,
		PointG2: *p,
	}
}

func (c *Bls12_381) NewGtFromBytes(b []byte) driver.Gt {
	gt := bls12381.NewGT()
	p, err := gt.FromBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return &bls12_381Gt{
		E:             *p,
		GT:            *gt,
		GTInitialised: true,
	}
}

func (c *Bls12_381) HashToG1(data []byte) driver.G1 {
	g1 := bls12381.NewG1()
	p, err := g1.HashToCurve(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToCurve failed [%s]", err.Error()))
	}

	return &bls12_381G1{
		PointG1: *p,
		G1:      *g1,
	}
}

func (c *Bls12_381) HashToG1WithDomain(data, domain []byte) driver.G1 {
	g1 := bls12381.NewG1()
	p, err := g1.HashToCurve(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToCurve failed [%s]", err.Error()))
	}

	return &bls12_381G1{
		PointG1: *p,
		G1:      *g1,
	}
}

func (c *Bls12_381BBS) HashToG1(data []byte) driver.G1 {
	p, err := HashToG1GenericBESwu(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToCurve failed [%s]", err.Error()))
	}

	return &bls12_381G1{
		PointG1: *p,
		G1:      *bls12381.NewG1(),
	}
}

func (c *Bls12_381BBS) HashToG1WithDomain(data, domain []byte) driver.G1 {
	p, err := HashToG1GenericBESwu(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToCurve failed [%s]", err.Error()))
	}

	return &bls12_381G1{
		PointG1: *p,
		G1:      *bls12381.NewG1(),
	}
}
