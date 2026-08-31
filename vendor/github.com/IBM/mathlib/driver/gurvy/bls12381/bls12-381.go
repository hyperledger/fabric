/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bls12381

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"math/big"
	"regexp"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	"github.com/IBM/mathlib/driver/gurvy"
	"github.com/consensys/gnark-crypto/ecc"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"golang.org/x/crypto/blake2b"
)

var g1StrRegexp = regexp.MustCompile(`^E\([[]([0-9]+),([0-9]+)[]]\)$`)
var g1Bytes12_381 [48]byte
var g2Bytes12_381 [96]byte

// point at infinity
var g1Infinity bls12381.G1Jac

var bigIntOne = big.NewInt(1)

func init() {
	_, _, g1, g2 := bls12381.Generators()
	g1Bytes12_381 = g1.Bytes()
	g2Bytes12_381 = g2.Bytes()
	g1Infinity.X.SetOne()
	g1Infinity.Y.SetOne()
}

// Zr represents a scalar field element backed by fr.Element ([4]uint64).
// The rawBigInt field is non-nil only for special values like GroupOrder
// (which equals p and is 0 in the field but needs its actual big.Int value
// for operations like Mod and InvModP).
//
// Only methods that explicitly branch on rawBigInt (IsZero, IsOne, Bytes, Equals,
// Copy, Clone, String, Mod, InvModP, InvModOrder, PowMod, toBigInt) preserve the
// true big-int value. Plus/Minus/Mul operate on val directly (val == 0 whenever
// rawBigInt != nil, since p mod p == 0), so arithmetic combining a rawBigInt-backed
// Zr with another Zr via Plus/Minus/Mul silently ignores the raw value. This is safe
// today because GroupOrder is only ever used as a PowMod exponent (where the result
// is invariant to the field reduction by Fermat's little theorem) or passed to the
// rawBigInt-aware methods above — do not add a new arithmetic use of GroupOrder via
// Plus/Minus/Mul without accounting for this.
type Zr struct {
	val       fr.Element
	rawBigInt *big.Int // non-nil only for GroupOrder
}

// toBigInt writes the Zr value into the provided big.Int.
func (b *Zr) toBigInt(bi *big.Int) {
	if b.rawBigInt != nil {
		bi.Set(b.rawBigInt)
	} else {
		b.val.BigInt(bi)
	}
}

func (b *Zr) Plus(a driver.Zr) driver.Zr {
	rv := &Zr{}
	rv.val.Add(&b.val, &a.(*Zr).val)

	return rv
}

func (b *Zr) IsZero() bool {
	if b.rawBigInt != nil {
		return b.rawBigInt.BitLen() == 0
	}

	return b.val.IsZero()
}

func (b *Zr) BigInt() *big.Int {
	bi := new(big.Int)
	b.toBigInt(bi)

	return bi
}

func (b *Zr) IsOne() bool {
	if b.rawBigInt != nil {
		return b.rawBigInt.Cmp(bigIntOne) == 0
	}

	return b.val.IsOne()
}

func (b *Zr) Minus(a driver.Zr) driver.Zr {
	rv := &Zr{}
	rv.val.Sub(&b.val, &a.(*Zr).val)

	return rv
}

func (b *Zr) Mul(x driver.Zr) driver.Zr {
	rv := &Zr{}
	rv.val.Mul(&b.val, &x.(*Zr).val)

	return rv
}

func (b *Zr) PowMod(x driver.Zr) driver.Zr {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	x.(*Zr).toBigInt(bi)

	rv := &Zr{}
	rv.val.Exp(b.val, bi)

	return rv
}

func (b *Zr) Mod(a driver.Zr) {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	b.toBigInt(bi)

	ai := bigIntPool.Get()
	defer bigIntPool.Put(ai)
	a.(*Zr).toBigInt(ai)

	bi.Mod(bi, ai)
	b.val.SetBigInt(bi)
	b.rawBigInt = nil
}

func (b *Zr) InvModP(p driver.Zr) {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	b.toBigInt(bi)

	pi := bigIntPool.Get()
	defer bigIntPool.Put(pi)
	p.(*Zr).toBigInt(pi)

	bi.ModInverse(bi, pi)
	b.val.SetBigInt(bi)
	b.rawBigInt = nil
}

func (b *Zr) InvModOrder() {
	b.val.Inverse(&b.val)
	b.rawBigInt = nil
}

func (b *Zr) Bytes() []byte {
	if b.rawBigInt != nil {
		return common.BigToBytes(b.rawBigInt)
	}
	raw := b.val.Bytes()

	return raw[:]
}

func (b *Zr) Equals(p driver.Zr) bool {
	other := p.(*Zr)
	if b.rawBigInt != nil || other.rawBigInt != nil {
		bi1 := bigIntPool.Get()
		defer bigIntPool.Put(bi1)
		bi2 := bigIntPool.Get()
		defer bigIntPool.Put(bi2)
		b.toBigInt(bi1)
		other.toBigInt(bi2)

		return bi1.Cmp(bi2) == 0
	}

	return b.val.Equal(&other.val)
}

func (b *Zr) Copy() driver.Zr {
	rv := &Zr{}
	rv.val.Set(&b.val)
	if b.rawBigInt != nil {
		rv.rawBigInt = new(big.Int).Set(b.rawBigInt)
	}

	return rv
}

func (b *Zr) Clone(a driver.Zr) {
	src := a.(*Zr)
	b.val.Set(&src.val)
	if src.rawBigInt != nil {
		b.rawBigInt = new(big.Int).Set(src.rawBigInt)
	} else {
		b.rawBigInt = nil
	}
}

func (b *Zr) String() string {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	b.toBigInt(bi)

	return bi.Text(16)
}

func (b *Zr) Neg() {
	b.val.Neg(&b.val)
	b.rawBigInt = nil
}

/*********************************************************************/

type G1 struct {
	bls12381.G1Affine
}

func (g *G1) Clone(a driver.G1) {
	raw := a.(*G1).G1Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *G1) Copy() driver.G1 {
	c := &G1{}
	c.Set(&e.G1Affine)

	return c
}

func (g *G1) Add(a driver.G1) {
	j := G1Jacs.Get()
	defer G1Jacs.Put(j)
	j.FromAffine(&g.G1Affine)
	j.AddMixed(&a.(*G1).G1Affine)
	g.FromJacobian(j)
}

func (g *G1) Mul(a driver.Zr) driver.G1 {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	a.(*Zr).toBigInt(bi)

	gc := &G1{}
	gc.ScalarMultiplication(&g.G1Affine, bi)

	return gc
}

// Mul2 computes [e]g + [f]Q via a joint Strauss-Shamir scalar multiplication.
// Benchmarked against two independent Mul calls plus an Add: allocates far less
// (1 vs ~26 allocs) but is not faster in wall-clock time, because — unlike Mul —
// it does not use the GLV endomorphism speedup, so it forgoes the ~2x speedup that
// GLV gives each individual scalar multiplication.
func (g *G1) Mul2(e driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	bi1 := bigIntPool.Get()
	defer bigIntPool.Put(bi1)
	e.(*Zr).toBigInt(bi1)

	bi2 := bigIntPool.Get()
	defer bigIntPool.Put(bi2)
	f.(*Zr).toBigInt(bi2)

	first := G1Jacs.Get()
	defer G1Jacs.Put(first)
	first = JointScalarMultiplication(first, &g.G1Affine, &Q.(*G1).G1Affine, bi1, bi2)
	gc := &G1{}
	gc.FromJacobian(first)

	return gc
}

func (g *G1) Mul2InPlace(e driver.Zr, Q driver.G1, f driver.Zr) {
	bi1 := bigIntPool.Get()
	defer bigIntPool.Put(bi1)
	e.(*Zr).toBigInt(bi1)

	bi2 := bigIntPool.Get()
	defer bigIntPool.Put(bi2)
	f.(*Zr).toBigInt(bi2)

	first := G1Jacs.Get()
	defer G1Jacs.Put(first)
	first = JointScalarMultiplication(first, &g.G1Affine, &Q.(*G1).G1Affine, bi1, bi2)
	g.FromJacobian(first)
}

func (g *G1) Equals(a driver.G1) bool {
	return g.Equal(&a.(*G1).G1Affine)
}

func (g *G1) Bytes() []byte {
	raw := g.RawBytes()

	return raw[:]
}

func (g *G1) Compressed() []byte {
	raw := g.G1Affine.Bytes()

	return raw[:]
}

func (g *G1) Sub(a driver.G1) {
	j, k := bls12381.G1Jac{}, bls12381.G1Jac{}
	j.FromAffine(&g.G1Affine)
	k.FromAffine(&a.(*G1).G1Affine)
	j.SubAssign(&k)
	g.FromJacobian(&j)
}

func (g *G1) IsInfinity() bool {
	return g.G1Affine.IsInfinity()
}

func (g *G1) String() string {
	rawstr := g.G1Affine.String()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)

	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

func (g *G1) Neg() {
	g.G1Affine.Neg(&g.G1Affine)
}

/*********************************************************************/

type G2 struct {
	bls12381.G2Affine
}

func (g *G2) Clone(a driver.G2) {
	raw := a.(*G2).G2Affine.Bytes()
	_, err := g.SetBytes(raw[:])
	if err != nil {
		panic("could not copy point")
	}
}

func (e *G2) Copy() driver.G2 {
	c := &G2{}
	c.Set(&e.G2Affine)

	return c
}

func (g *G2) Mul(a driver.Zr) driver.G2 {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	a.(*Zr).toBigInt(bi)

	gc := &G2{}
	gc.ScalarMultiplication(&g.G2Affine, bi)

	return gc
}

func (g *G2) Add(a driver.G2) {
	j := bls12381.G2Jac{}
	j.FromAffine(&g.G2Affine)
	j.AddMixed(&a.(*G2).G2Affine)
	g.FromJacobian(&j)
}

func (g *G2) Sub(a driver.G2) {
	j := bls12381.G2Jac{}
	j.FromAffine(&g.G2Affine)
	aJac := bls12381.G2Jac{}
	aJac.FromAffine(&a.(*G2).G2Affine)
	j.SubAssign(&aJac)
	g.FromJacobian(&j)
}

func (g *G2) Affine() {
	// we're always affine
}

func (g *G2) Bytes() []byte {
	raw := g.RawBytes()

	return raw[:]
}

func (g *G2) Compressed() []byte {
	raw := g.G2Affine.Bytes()

	return raw[:]
}

func (g *G2) String() string {
	return g.G2Affine.String()
}

func (g *G2) Equals(a driver.G2) bool {
	return g.Equal(&a.(*G2).G2Affine)
}

/*********************************************************************/

type Gt struct {
	bls12381.GT
}

func (g *Gt) Exp(x driver.Zr) driver.Gt {
	bi := bigIntPool.Get()
	defer bigIntPool.Put(bi)
	x.(*Zr).toBigInt(bi)

	c := bls12381.GT{}

	return &Gt{*c.Exp(g.GT, bi)}
}

func (g *Gt) Equals(a driver.Gt) bool {
	return g.Equal(&a.(*Gt).GT)
}

func (g *Gt) Inverse() {
	g.GT.Inverse(&g.GT)
}

func (g *Gt) Mul(a driver.Gt) {
	g.GT.Mul(&g.GT, &a.(*Gt).GT)
}

func (g *Gt) IsUnity() bool {
	unity := bls12381.GT{}
	unity.SetOne()

	return unity.Equal(&g.GT)
}

func (g *Gt) ToString() string {
	return g.String()
}

func (g *Gt) Bytes() []byte {
	raw := g.GT.Bytes()

	return raw[:]
}

/*********************************************************************/

type Curve struct {
	common.CurveBase
}

func NewCurve() *Curve {
	return &Curve{common.CurveBase{Modulus: *fr.Modulus()}}
}

func (c *Curve) Pairing(p2 driver.G2, p1 driver.G1) driver.Gt {
	t, err := bls12381.MillerLoop([]bls12381.G1Affine{p1.(*G1).G1Affine}, []bls12381.G2Affine{p2.(*G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing failed [%s]", err.Error()))
	}

	return &Gt{t}
}

func (c *Curve) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	t, err := bls12381.MillerLoop([]bls12381.G1Affine{p1a.(*G1).G1Affine, p1b.(*G1).G1Affine}, []bls12381.G2Affine{p2a.(*G2).G2Affine, p2b.(*G2).G2Affine})
	if err != nil {
		panic(fmt.Sprintf("pairing 2 failed [%s]", err.Error()))
	}

	return &Gt{t}
}

func (c *Curve) FExp(a driver.Gt) driver.Gt {
	return &Gt{bls12381.FinalExponentiation(&a.(*Gt).GT)}
}

func (c *Curve) GenG1() driver.G1 {
	r := &G1{}
	_, err := r.SetBytes(g1Bytes12_381[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Curve) GenG2() driver.G2 {
	r := &G2{}
	_, err := r.SetBytes(g2Bytes12_381[:])
	if err != nil {
		panic("could not generate point")
	}

	return r
}

func (c *Curve) GenGt() driver.Gt {
	g1 := c.GenG1()
	g2 := c.GenG2()
	gengt := c.Pairing(g2, g1)
	gengt = c.FExp(gengt)

	return gengt
}

func (c *Curve) CoordinateByteSize() int {
	return bls12381.SizeOfG1AffineCompressed
}

func (c *Curve) G1ByteSize() int {
	return bls12381.SizeOfG1AffineUncompressed
}

func (c *Curve) CompressedG1ByteSize() int {
	return bls12381.SizeOfG1AffineCompressed
}

func (c *Curve) G2ByteSize() int {
	return bls12381.SizeOfG2AffineUncompressed
}

func (c *Curve) CompressedG2ByteSize() int {
	return bls12381.SizeOfG2AffineCompressed
}

func (c *Curve) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (c *Curve) NewG1() driver.G1 {
	return &G1{}
}

func (c *Curve) NewG2() driver.G2 {
	return &G2{}
}

func (c *Curve) NewG1FromBytes(b []byte) driver.G1 {
	v := &G1{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Curve) NewG2FromBytes(b []byte) driver.G2 {
	v := &G2{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Curve) NewG1FromCompressed(b []byte) driver.G1 {
	v := &G1{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Curve) NewG2FromCompressed(b []byte) driver.G2 {
	v := &G2{}
	_, err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Curve) NewGtFromBytes(b []byte) driver.Gt {
	v := &Gt{}
	err := v.SetBytes(b)
	if err != nil {
		panic(fmt.Sprintf("set bytes failed [%s]", err.Error()))
	}

	return v
}

func (c *Curve) ModNeg(a1, m driver.Zr) driver.Zr {
	res := &Zr{}
	res.val.Neg(&a1.(*Zr).val)

	return res
}

func (c *Curve) ModSub(a1, b1, m driver.Zr) driver.Zr {
	res := &Zr{}
	res.val.Sub(&a1.(*Zr).val, &b1.(*Zr).val)

	return res
}

func (c *Curve) GroupOrder() driver.Zr {
	return &Zr{rawBigInt: new(big.Int).Set(&c.Modulus)}
}

func (c *Curve) NewZrFromBytes(b []byte) driver.Zr {
	res := &Zr{}
	res.val.SetBytes(b)

	return res
}

func (c *Curve) NewZrFromInt64(i int64) driver.Zr {
	res := &Zr{}
	res.val.SetInt64(i)

	return res
}

func (c *Curve) NewZrFromUint64(i uint64) driver.Zr {
	res := &Zr{}
	res.val.SetUint64(i)

	return res
}

func (c *Curve) NewZrFromBigInt(i *big.Int) driver.Zr {
	res := &Zr{}
	res.val.SetBigInt(i)

	return res
}

func (c *Curve) NewRandomZr(rng io.Reader) driver.Zr {
	bi, err := rand.Int(rng, &c.Modulus)
	if err != nil {
		panic(err)
	}

	return &Zr{val: *new(fr.Element).SetBigInt(bi)}
}

func (c *Curve) HashToZr(data []byte) driver.Zr {
	digest := sha256.Sum256(data)
	digestBig := new(big.Int).SetBytes(digest[:])
	digestBig.Mod(digestBig, &c.Modulus)

	res := &Zr{}
	res.val.SetBigInt(digestBig)

	return res
}

func (p *Curve) Rand() (io.Reader, error) {
	return rand.Reader, nil
}

func (c *Curve) HashToG1(data []byte) driver.G1 {
	g1, err := bls12381.HashToG1(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &G1{g1}
}

func (c *Curve) HashToG2(data []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &G2{g2}
}

func (c *Curve) HashToG1WithDomain(data, domain []byte) driver.G1 {
	g1, err := bls12381.HashToG1(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &G1{g1}
}

func (c *Curve) HashToG2WithDomain(data, domain []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &G2{g2}
}

func (c *Curve) ModMul(a1, b1, m driver.Zr) driver.Zr {
	res := &Zr{}
	res.val.Mul(&a1.(*Zr).val, &b1.(*Zr).val)

	return res
}

func (c *Curve) ModAddMul(a1, b1 []driver.Zr, m driver.Zr) driver.Zr {
	var sum, tmp fr.Element
	sum.SetZero()
	for i := range a1 {
		tmp.Mul(&a1[i].(*Zr).val, &b1[i].(*Zr).val)
		sum.Add(&sum, &tmp)
	}

	res := &Zr{}
	res.val.Set(&sum)

	return res
}

func (c *Curve) ModAddMul2(a1 driver.Zr, c1 driver.Zr, b1 driver.Zr, c2 driver.Zr, m driver.Zr) driver.Zr {
	var sum, tmp fr.Element
	sum.SetZero()

	tmp.Mul(&a1.(*Zr).val, &c1.(*Zr).val)
	sum.Add(&sum, &tmp)

	tmp.Mul(&b1.(*Zr).val, &c2.(*Zr).val)
	sum.Add(&sum, &tmp)

	res := &Zr{}
	res.val.Set(&sum)

	return res
}

func (c *Curve) ModAddMul3(
	a1 driver.Zr,
	a2 driver.Zr,
	b1 driver.Zr,
	b2 driver.Zr,
	d1 driver.Zr,
	d2 driver.Zr,
	m driver.Zr,
) driver.Zr {
	var sum, tmp fr.Element
	sum.SetZero()

	tmp.Mul(&a1.(*Zr).val, &a2.(*Zr).val)
	sum.Add(&sum, &tmp)

	tmp.Mul(&b1.(*Zr).val, &b2.(*Zr).val)
	sum.Add(&sum, &tmp)

	tmp.Mul(&d1.(*Zr).val, &d2.(*Zr).val)
	sum.Add(&sum, &tmp)

	res := &Zr{}
	res.val.Set(&sum)

	return res
}

func (c *Curve) ModAdd(a1, b1, m driver.Zr) driver.Zr {
	res := &Zr{}
	res.val.Add(&a1.(*Zr).val, &b1.(*Zr).val)

	return res
}

func (c *Curve) ModAdd2(a1, b1, c1, m driver.Zr) {
	a := a1.(*Zr)
	a.val.Add(&a.val, &b1.(*Zr).val)
	a.val.Add(&a.val, &c1.(*Zr).val)
	a.rawBigInt = nil
}

func (c *Curve) MultiScalarMul(a []driver.G1, b []driver.Zr) driver.G1 {
	affinePoints := make([]bls12381.G1Affine, len(a))
	scalars := make([]fr.Element, len(b))

	for i := range a {
		affinePoints[i] = a[i].(*G1).G1Affine
		scalars[i] = b[i].(*Zr).val // Direct fr.Element copy — no SetBigInt!
	}

	first := G1Jacs.Get()
	defer G1Jacs.Put(first)
	_, _ = first.MultiExp(affinePoints, scalars, ecc.MultiExpConfig{})

	gc := &G1{}
	gc.FromJacobian(first)

	return gc
}

type BBSCurve struct {
	Curve
}

func NewBBSCurve() *BBSCurve {
	return &BBSCurve{*NewCurve()}
}

func (c *Curve) ModMulInPlace(result, a, b, m driver.Zr) {
	r := result.(*Zr)
	r.val.Mul(&a.(*Zr).val, &b.(*Zr).val)
	r.rawBigInt = nil
}

func (c *Curve) ModAddMul2InPlace(result driver.Zr, a1, c1, b1, c2, m driver.Zr) {
	r := result.(*Zr)
	var tmp fr.Element
	r.val.Mul(&a1.(*Zr).val, &c1.(*Zr).val)
	tmp.Mul(&b1.(*Zr).val, &c2.(*Zr).val)
	r.val.Add(&r.val, &tmp)
	r.rawBigInt = nil
}

func (c *Curve) ModAddMul3InPlace(result driver.Zr, a1, a2, b1, b2, d1, d2, m driver.Zr) {
	r := result.(*Zr)
	var tmp fr.Element
	r.val.Mul(&a1.(*Zr).val, &a2.(*Zr).val)
	tmp.Mul(&b1.(*Zr).val, &b2.(*Zr).val)
	r.val.Add(&r.val, &tmp)
	tmp.Mul(&d1.(*Zr).val, &d2.(*Zr).val)
	r.val.Add(&r.val, &tmp)
	r.rawBigInt = nil
}

func (c *BBSCurve) HashToG1(data []byte) driver.G1 {
	hashFunc := func() hash.Hash {
		// We pass a null key so error is impossible here.
		h, _ := blake2b.New512(nil)

		return h
	}

	g1, err := gurvy.HashToG1GenericBESwu(data, []byte{}, hashFunc)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &G1{g1}
}

func (c *BBSCurve) HashToG2(data []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, []byte{})
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &G2{g2}
}

func (c *BBSCurve) HashToG1WithDomain(data, domain []byte) driver.G1 {
	hashFunc := func() hash.Hash {
		// We pass a null key so error is impossible here.
		h, _ := blake2b.New512(nil)

		return h
	}

	g1, err := gurvy.HashToG1GenericBESwu(data, domain, hashFunc)
	if err != nil {
		panic(fmt.Sprintf("HashToG1 failed [%s]", err.Error()))
	}

	return &G1{g1}
}

func (c *BBSCurve) HashToG2WithDomain(data, domain []byte) driver.G2 {
	g2, err := bls12381.HashToG2(data, domain)
	if err != nil {
		panic(fmt.Sprintf("HashToG2 failed [%s]", err.Error()))
	}

	return &G2{g2}
}

// JointScalarMultiplication computes [s1]a1+[s2]a2 using Strauss-Shamir technique
// where a1 and a2 are affine points. This does not use the GLV endomorphism (unlike
// G1Jac.ScalarMultiplication), so it is an allocation optimization over two independent
// scalar multiplications plus an addition, not a wall-clock optimization.
func JointScalarMultiplication(p *bls12381.G1Jac, a1, a2 *bls12381.G1Affine, s1, s2 *big.Int) *bls12381.G1Jac {
	var res, p1, p2 bls12381.G1Jac
	res.Set(&g1Infinity)
	p1.FromAffine(a1)
	p2.FromAffine(a2)

	var table [15]bls12381.G1Jac

	var k1, k2 big.Int
	if s1.Sign() == -1 {
		k1.Neg(s1)
		table[0].Neg(&p1)
	} else {
		k1 = *s1
		table[0].Set(&p1)
	}
	if s2.Sign() == -1 {
		k2.Neg(s2)
		table[3].Neg(&p2)
	} else {
		k2 = *s2
		table[3].Set(&p2)
	}

	// precompute table (2 bits sliding window)
	table[1].Double(&table[0])
	table[2].Set(&table[1]).AddAssign(&table[0])
	table[4].Set(&table[3]).AddAssign(&table[0])
	table[5].Set(&table[3]).AddAssign(&table[1])
	table[6].Set(&table[3]).AddAssign(&table[2])
	table[7].Double(&table[3])
	table[8].Set(&table[7]).AddAssign(&table[0])
	table[9].Set(&table[7]).AddAssign(&table[1])
	table[10].Set(&table[7]).AddAssign(&table[2])
	table[11].Set(&table[7]).AddAssign(&table[3])
	table[12].Set(&table[11]).AddAssign(&table[0])
	table[13].Set(&table[11]).AddAssign(&table[1])
	table[14].Set(&table[11]).AddAssign(&table[2])

	var s [2]fr.Element
	s[0] = s[0].SetBigInt(&k1).Bits()
	s[1] = s[1].SetBigInt(&k2).Bits()

	maxBit := max(k2.BitLen(), k1.BitLen())
	hiWordIndex := (maxBit - 1) / 64

	for i := hiWordIndex; i >= 0; i-- {
		mask := uint64(3) << 62
		for j := range 32 {
			res.Double(&res).Double(&res)
			b1 := (s[0][i] & mask) >> (62 - 2*j)
			b2 := (s[1][i] & mask) >> (62 - 2*j)
			if b1|b2 != 0 {
				s := (b2<<2 | b1)
				res.AddAssign(&table[s-1])
			}
			mask = mask >> 2
		}
	}

	p.Set(&res)

	return p
}
