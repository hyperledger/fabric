/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package math

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/amcl"
	"github.com/IBM/mathlib/driver/gurvy"
	"github.com/pkg/errors"
)

type CurveID int

const (
	FP256BN_AMCL CurveID = iota
	BN254
	FP256BN_AMCL_MIRACL
)

var Curves []*Curve = []*Curve{
	{
		c:          &amcl.Fp256bn{},
		GenG1:      &G1{g1: (&amcl.Fp256bn{}).GenG1(), curveID: FP256BN_AMCL},
		GenG2:      &G2{g2: (&amcl.Fp256bn{}).GenG2(), curveID: FP256BN_AMCL},
		GenGt:      &Gt{gt: (&amcl.Fp256bn{}).GenGt(), curveID: FP256BN_AMCL},
		GroupOrder: &Zr{zr: (&amcl.Fp256bn{}).GroupOrder(), curveID: FP256BN_AMCL},
		FieldBytes: (&amcl.Fp256bn{}).FieldBytes(),
		curveID:    FP256BN_AMCL,
	},
	{
		c:          &gurvy.Bn254{},
		GenG1:      &G1{g1: (&gurvy.Bn254{}).GenG1(), curveID: BN254},
		GenG2:      &G2{g2: (&gurvy.Bn254{}).GenG2(), curveID: BN254},
		GenGt:      &Gt{gt: (&gurvy.Bn254{}).GenGt(), curveID: BN254},
		GroupOrder: &Zr{zr: (&gurvy.Bn254{}).GroupOrder(), curveID: BN254},
		FieldBytes: (&gurvy.Bn254{}).FieldBytes(),
		curveID:    BN254,
	},
	{
		c:          &amcl.Fp256Miraclbn{},
		GenG1:      &G1{g1: (&amcl.Fp256Miraclbn{}).GenG1(), curveID: FP256BN_AMCL_MIRACL},
		GenG2:      &G2{g2: (&amcl.Fp256Miraclbn{}).GenG2(), curveID: FP256BN_AMCL_MIRACL},
		GenGt:      &Gt{gt: (&amcl.Fp256Miraclbn{}).GenGt(), curveID: FP256BN_AMCL_MIRACL},
		GroupOrder: &Zr{zr: (&amcl.Fp256Miraclbn{}).GroupOrder(), curveID: FP256BN_AMCL_MIRACL},
		FieldBytes: (&amcl.Fp256Miraclbn{}).FieldBytes(),
		curveID:    FP256BN_AMCL_MIRACL,
	},
}

/*********************************************************************/

type Zr struct {
	zr      driver.Zr
	curveID CurveID
}

func (z *Zr) Plus(a *Zr) *Zr {
	return &Zr{zr: z.zr.Plus(a.zr), curveID: z.curveID}
}

func (z *Zr) Mod(a *Zr) {
	z.zr.Mod(a.zr)
}

func (z *Zr) PowMod(a *Zr) *Zr {
	return &Zr{zr: z.zr.PowMod(a.zr), curveID: z.curveID}
}

func (z *Zr) InvModP(a *Zr) {
	z.zr.InvModP(a.zr)
}

func (z *Zr) Bytes() []byte {
	return z.zr.Bytes()
}

func (z *Zr) Equals(a *Zr) bool {
	return z.zr.Equals(a.zr)
}

func (z *Zr) Copy() *Zr {
	return &Zr{zr: z.zr.Copy(), curveID: z.curveID}
}

func (z *Zr) Clone(a *Zr) {
	z.zr.Clone(a.zr)
}

func (z *Zr) String() string {
	return z.zr.String()
}

var zerobytes = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
var onebytes = []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

func (z *Zr) Int() (int64, error) {
	b := z.Bytes()
	if !bytes.Equal(zerobytes, b[:32-8]) && !bytes.Equal(onebytes, b[:32-8]) {
		return 0, fmt.Errorf("out of range")
	}

	return int64(binary.BigEndian.Uint64(b[32-8:])), nil
}

/*********************************************************************/

type G1 struct {
	g1      driver.G1
	curveID CurveID
}

func (g *G1) Clone(a *G1) {
	g.g1.Clone(a.g1)
}

func (g *G1) Copy() *G1 {
	return &G1{g1: g.g1.Copy(), curveID: g.curveID}
}

func (g *G1) Add(a *G1) {
	g.g1.Add(a.g1)
}

func (g *G1) Mul(a *Zr) *G1 {
	return &G1{g1: g.g1.Mul(a.zr), curveID: g.curveID}
}

func (g *G1) Mul2(e *Zr, Q *G1, f *Zr) *G1 {
	return &G1{g1: g.g1.Mul2(e.zr, Q.g1, f.zr), curveID: g.curveID}
}

func (g *G1) Equals(a *G1) bool {
	return g.g1.Equals(a.g1)
}

func (g *G1) Bytes() []byte {
	return g.g1.Bytes()
}

func (g *G1) Sub(a *G1) {
	g.g1.Sub(a.g1)
}

func (g *G1) IsInfinity() bool {
	return g.g1.IsInfinity()
}

func (g *G1) String() string {
	return g.g1.String()
}

/*********************************************************************/

type G2 struct {
	g2      driver.G2
	curveID CurveID
}

func (g *G2) Clone(a *G2) {
	g.g2.Clone(a.g2)
}

func (g *G2) Copy() *G2 {
	return &G2{g2: g.g2.Copy(), curveID: g.curveID}
}

func (g *G2) Mul(a *Zr) *G2 {
	return &G2{g2: g.g2.Mul(a.zr), curveID: g.curveID}
}

func (g *G2) Add(a *G2) {
	g.g2.Add(a.g2)
}

func (g *G2) Sub(a *G2) {
	g.g2.Sub(a.g2)
}

func (g *G2) Affine() {
	g.g2.Affine()
}

func (g *G2) Bytes() []byte {
	return g.g2.Bytes()
}

func (g *G2) String() string {
	return g.g2.String()
}

func (g *G2) Equals(a *G2) bool {
	return g.g2.Equals(a.g2)
}

/*********************************************************************/

type Gt struct {
	gt      driver.Gt
	curveID CurveID
}

func (g *Gt) Equals(a *Gt) bool {
	return g.gt.Equals(a.gt)
}

func (g *Gt) Inverse() {
	g.gt.Inverse()
}

func (g *Gt) Mul(a *Gt) {
	g.gt.Mul(a.gt)
}

func (g *Gt) IsUnity() bool {
	return g.gt.IsUnity()
}

func (g *Gt) String() string {
	return g.gt.ToString()
}

func (g *Gt) Bytes() []byte {
	return g.gt.Bytes()
}

/*********************************************************************/

type Curve struct {
	c          driver.Curve
	GenG1      *G1
	GenG2      *G2
	GenGt      *Gt
	GroupOrder *Zr
	FieldBytes int
	curveID    CurveID
}

func (c *Curve) Rand() (io.Reader, error) {
	return c.c.Rand()
}

func (c *Curve) NewRandomZr(rng io.Reader) *Zr {
	return &Zr{zr: c.c.NewRandomZr(rng), curveID: c.curveID}
}

func (c *Curve) NewZrFromBytes(b []byte) *Zr {
	return &Zr{zr: c.c.NewZrFromBytes(b), curveID: c.curveID}
}

func (c *Curve) NewG1FromBytes(b []byte) (p *G1, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &G1{g1: c.c.NewG1FromBytes(b), curveID: c.curveID}
	return
}

func (c *Curve) NewG2FromBytes(b []byte) (p *G2, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &G2{g2: c.c.NewG2FromBytes(b), curveID: c.curveID}
	return
}

func (c *Curve) NewGtFromBytes(b []byte) (p *Gt, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &Gt{gt: c.c.NewGtFromBytes(b), curveID: c.curveID}
	return
}

func (c *Curve) NewZrFromInt(i int64) *Zr {
	return &Zr{zr: c.c.NewZrFromInt(i), curveID: c.curveID}
}

// func (c *Curve) NewG1FromCoords(ix, iy *Zr) *G1 {
// 	return &G1{c.c.NewG1FromCoords(ix.zr, iy.zr)}
// }

func (c *Curve) NewG2() *G2 {
	return &G2{g2: c.c.NewG2(), curveID: c.curveID}
}

func (c *Curve) NewG1() *G1 {
	return &G1{g1: c.c.NewG1(), curveID: c.curveID}
}

func (c *Curve) Pairing(a *G2, b *G1) *Gt {
	return &Gt{gt: c.c.Pairing(a.g2, b.g1), curveID: c.curveID}
}

func (c *Curve) Pairing2(p *G2, q *G1, r *G2, s *G1) *Gt {
	return &Gt{gt: c.c.Pairing2(p.g2, r.g2, q.g1, s.g1), curveID: c.curveID}
}

func (c *Curve) FExp(a *Gt) *Gt {
	return &Gt{gt: c.c.FExp(a.gt), curveID: c.curveID}
}

func (c *Curve) HashToZr(data []byte) *Zr {
	return &Zr{zr: c.c.HashToZr(data), curveID: c.curveID}
}

func (c *Curve) HashToG1(data []byte) *G1 {
	return &G1{g1: c.c.HashToG1(data), curveID: c.curveID}
}

func (c *Curve) ModSub(a, b, m *Zr) *Zr {
	return &Zr{zr: c.c.ModSub(a.zr, b.zr, m.zr), curveID: c.curveID}
}

func (c *Curve) ModAdd(a, b, m *Zr) *Zr {
	return &Zr{zr: c.c.ModAdd(a.zr, b.zr, m.zr), curveID: c.curveID}
}

func (c *Curve) ModMul(a1, b1, m *Zr) *Zr {
	return &Zr{zr: c.c.ModMul(a1.zr, b1.zr, m.zr), curveID: c.curveID}
}

func (c *Curve) ModNeg(a1, m *Zr) *Zr {
	return &Zr{zr: c.c.ModNeg(a1.zr, m.zr), curveID: c.curveID}
}

/*********************************************************************/
