/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package math provides a high-level interface for pairing-based cryptography operations
// on elliptic curves. It supports multiple pairing-friendly curves including BN254, BLS12-381,
// BLS12-377, and FP256BN variants.
//
// # Overview
//
// This package implements operations on three main groups used in pairing-based cryptography:
//   - G1: Points on the first elliptic curve group
//   - G2: Points on the second elliptic curve group (twisted curve)
//   - Gt: Elements in the target group (result of pairing operations)
//   - Zr: Scalars in the field (integers modulo the curve order)
//
// The library uses a driver pattern to support multiple backend implementations (AMCL, Gurvy, Kilic),
// allowing users to choose the best performance/compatibility trade-off for their use case.
//
// # Basic Usage
//
// Select a curve and perform operations:
//
//	curve := math.Curves[math.BLS12_381]
//	rng, _ := curve.Rand()
//	scalar := curve.NewRandomZr(rng)
//	point := curve.GenG1.Mul(scalar)
//	result := curve.Pairing(curve.GenG2, point)
//
// # Supported Curves
//
// The package provides pre-configured curves accessible via the Curves slice:
//   - FP256BN_AMCL: 256-bit Barreto-Naehrig curve (AMCL backend)
//   - BN254: 254-bit Barreto-Naehrig curve (Gurvy backend)
//   - FP256BN_AMCL_MIRACL: 256-bit BN curve MIRACL variant (AMCL backend)
//   - BLS12_381: BLS12-381 curve (Gurvy backend)
//   - BLS12_377_GURVY: BLS12-377 curve (Gurvy backend)
//   - BLS12_381_GURVY: BLS12-381 curve (Gurvy backend)
//   - BLS12_381_BBS: BLS12-381 optimized for BBS+ signatures (Gurvy backend)
//   - BLS12_381_BBS_GURVY: BLS12-381 for BBS+ (Gurvy backend)
//
// # Thread Safety
//
// The types in this package are not thread-safe. Users should implement their own
// synchronization when sharing instances across goroutines.
package math

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/amcl"
	"github.com/IBM/mathlib/driver/gurvy"
	"github.com/IBM/mathlib/driver/gurvy/bls12381"
)

// CurveID identifies a specific elliptic curve configuration and its backend implementation.
// Each curve ID represents a unique combination of curve parameters and the underlying
// cryptographic library used for operations.
type CurveID int

const (
	// FP256BN_AMCL represents a 256-bit Barreto-Naehrig curve using the AMCL backend.
	// Suitable for general-purpose pairing operations with good performance.
	FP256BN_AMCL CurveID = iota

	// BN254 represents a 254-bit Barreto-Naehrig curve using the Gurvy backend.
	// Offers high performance but has tighter security margins than BLS12-381.
	BN254

	// FP256BN_AMCL_MIRACL represents a 256-bit BN curve MIRACL variant using AMCL.
	// Provided for legacy compatibility with MIRACL-based systems.
	FP256BN_AMCL_MIRACL

	// BLS12_381 represents the BLS12-381 curve using the Gurvy (gnark-crypto) backend.
	// Recommended for new projects due to excellent security margins and wide adoption.
	// Suitable for BLS signatures and modern cryptographic protocols.
	// Byte-compatible with the former Kilic backend and with BLS12_381_GURVY.
	BLS12_381

	// BLS12_377_GURVY represents the BLS12-377 curve using the Gurvy backend.
	// Optimized for recursive proof composition in zk-SNARK systems.
	BLS12_377_GURVY

	// BLS12_381_GURVY represents the BLS12-381 curve using the Gurvy backend.
	// Performance-optimized implementation of BLS12-381 with assembly optimizations.
	BLS12_381_GURVY

	// BLS12_381_BBS is equivalent to BLS12_381 up to HashToG1 and HashToG2.
	// Those functions follow the rules of the standard draft.
	BLS12_381_BBS

	// BLS12_381_BBS_GURVY is equivalent to BLS12_381_GURVY up to HashToG1 and HashToG2.
	// Those functions follow the rules of the standard draft.
	BLS12_381_BBS_GURVY
)

// CurveIDToString converts a CurveID to its string representation.
// Returns a human-readable name for the curve, useful for logging and debugging.
// Panics if the curve ID is unknown.
func CurveIDToString(id CurveID) string {
	switch id {
	case FP256BN_AMCL:
		return "FP256BN_AMCL"
	case BN254:
		return "BN254"
	case FP256BN_AMCL_MIRACL:
		return "FP256BN_AMCL_MIRACL"
	case BLS12_381:
		return "BLS12_381"
	case BLS12_377_GURVY:
		return "BLS12_377_GURVY"
	case BLS12_381_GURVY:
		return "BLS12_381_GURVY"
	case BLS12_381_BBS:
		return "BLS12_381_BBS"
	case BLS12_381_BBS_GURVY:
		return "BLS12_381_BBS_GURVY"
	default:
		panic(fmt.Sprintf("unknown curve %d", id))
	}
}

// Curves provides pre-configured instances of all supported elliptic curves.
// Each curve is fully initialized and ready to use. Access curves by their index
// using the CurveID constants (e.g., Curves[BLS12_381]).
//
// Example:
//
//	curve := math.Curves[math.BLS12_381]
//	point := curve.GenG1.Mul(curve.NewZrFromInt(42))
//
// The curves are instantiated at package initialization and can be used directly
// or via the NewCurve function for custom configurations.
var Curves []*Curve = []*Curve{
	NewCurve(
		amcl.NewFp256bn(),
		NewG1((&amcl.Fp256bn{}).GenG1(), FP256BN_AMCL),
		NewG2((&amcl.Fp256bn{}).GenG2(), FP256BN_AMCL),
		NewGt((&amcl.Fp256bn{}).GenGt(), FP256BN_AMCL),
		NewZr(amcl.NewFp256bn().GroupOrder(), FP256BN_AMCL),
		(&amcl.Fp256bn{}).CoordinateByteSize(),
		(&amcl.Fp256bn{}).G1ByteSize(),
		(&amcl.Fp256bn{}).CompressedG1ByteSize(),
		(&amcl.Fp256bn{}).G2ByteSize(),
		(&amcl.Fp256bn{}).CompressedG2ByteSize(),
		(&amcl.Fp256bn{}).ScalarByteSize(),
		FP256BN_AMCL,
	),
	{
		c:                    gurvy.NewBn254(),
		GenG1:                NewG1((&gurvy.Bn254{}).GenG1(), BN254),
		GenG2:                NewG2((&gurvy.Bn254{}).GenG2(), BN254),
		GenGt:                NewGt((&gurvy.Bn254{}).GenGt(), BN254),
		GroupOrder:           NewZr(gurvy.NewBn254().GroupOrder(), BN254),
		CoordByteSize:        (&gurvy.Bn254{}).CoordinateByteSize(),
		G1ByteSize:           (&gurvy.Bn254{}).G1ByteSize(),
		CompressedG1ByteSize: (&gurvy.Bn254{}).CompressedG1ByteSize(),
		G2ByteSize:           (&gurvy.Bn254{}).G2ByteSize(),
		CompressedG2ByteSize: (&gurvy.Bn254{}).CompressedG2ByteSize(),
		ScalarByteSize:       (&gurvy.Bn254{}).ScalarByteSize(),
		curveID:              BN254,
	},
	{
		c:                    amcl.NewFp256Miraclbn(),
		GenG1:                NewG1((&amcl.Fp256Miraclbn{}).GenG1(), FP256BN_AMCL_MIRACL),
		GenG2:                NewG2((&amcl.Fp256Miraclbn{}).GenG2(), FP256BN_AMCL_MIRACL),
		GenGt:                NewGt((&amcl.Fp256Miraclbn{}).GenGt(), FP256BN_AMCL_MIRACL),
		GroupOrder:           NewZr(amcl.NewFp256Miraclbn().GroupOrder(), FP256BN_AMCL_MIRACL),
		CoordByteSize:        (&amcl.Fp256Miraclbn{}).CoordinateByteSize(),
		G1ByteSize:           (&amcl.Fp256Miraclbn{}).G1ByteSize(),
		CompressedG1ByteSize: (&amcl.Fp256Miraclbn{}).CompressedG1ByteSize(),
		G2ByteSize:           (&amcl.Fp256Miraclbn{}).G2ByteSize(),
		CompressedG2ByteSize: (&amcl.Fp256Miraclbn{}).CompressedG2ByteSize(),
		ScalarByteSize:       (&amcl.Fp256Miraclbn{}).ScalarByteSize(),
		curveID:              FP256BN_AMCL_MIRACL,
	},
	{
		c:                    bls12381.NewCurve(),
		GenG1:                NewG1((&bls12381.Curve{}).GenG1(), BLS12_381),
		GenG2:                NewG2((&bls12381.Curve{}).GenG2(), BLS12_381),
		GenGt:                NewGt((&bls12381.Curve{}).GenGt(), BLS12_381),
		GroupOrder:           NewZr(bls12381.NewCurve().GroupOrder(), BLS12_381),
		CoordByteSize:        (&bls12381.Curve{}).CoordinateByteSize(),
		G1ByteSize:           (&bls12381.Curve{}).G1ByteSize(),
		CompressedG1ByteSize: (&bls12381.Curve{}).CompressedG1ByteSize(),
		G2ByteSize:           (&bls12381.Curve{}).G2ByteSize(),
		CompressedG2ByteSize: (&bls12381.Curve{}).CompressedG2ByteSize(),
		ScalarByteSize:       (&bls12381.Curve{}).ScalarByteSize(),
		curveID:              BLS12_381,
	},
	{
		c:                    gurvy.NewBls12_377(),
		GenG1:                NewG1((&gurvy.Bls12_377{}).GenG1(), BLS12_377_GURVY),
		GenG2:                NewG2((&gurvy.Bls12_377{}).GenG2(), BLS12_377_GURVY),
		GenGt:                NewGt((&gurvy.Bls12_377{}).GenGt(), BLS12_377_GURVY),
		GroupOrder:           NewZr(gurvy.NewBls12_377().GroupOrder(), BLS12_377_GURVY),
		CoordByteSize:        (&gurvy.Bls12_377{}).CoordinateByteSize(),
		G1ByteSize:           (&gurvy.Bls12_377{}).G1ByteSize(),
		CompressedG1ByteSize: (&gurvy.Bls12_377{}).CompressedG1ByteSize(),
		G2ByteSize:           (&gurvy.Bls12_377{}).G2ByteSize(),
		CompressedG2ByteSize: (&gurvy.Bls12_377{}).CompressedG2ByteSize(),
		ScalarByteSize:       (&gurvy.Bls12_377{}).ScalarByteSize(),
		curveID:              BLS12_377_GURVY,
	},
	{
		c:                    bls12381.NewCurve(),
		GenG1:                NewG1((&bls12381.Curve{}).GenG1(), BLS12_381_GURVY),
		GenG2:                NewG2((&bls12381.Curve{}).GenG2(), BLS12_381_GURVY),
		GenGt:                NewGt((&bls12381.Curve{}).GenGt(), BLS12_381_GURVY),
		GroupOrder:           NewZr(bls12381.NewCurve().GroupOrder(), BLS12_381_GURVY),
		CoordByteSize:        (&bls12381.Curve{}).CoordinateByteSize(),
		G1ByteSize:           (&bls12381.Curve{}).G1ByteSize(),
		CompressedG1ByteSize: (&bls12381.Curve{}).CompressedG1ByteSize(),
		G2ByteSize:           (&bls12381.Curve{}).G2ByteSize(),
		CompressedG2ByteSize: (&bls12381.Curve{}).CompressedG2ByteSize(),
		ScalarByteSize:       (&bls12381.Curve{}).ScalarByteSize(),
		curveID:              BLS12_381_GURVY,
	},
	{
		c:                    bls12381.NewBBSCurve(),
		GenG1:                NewG1(bls12381.NewBBSCurve().GenG1(), BLS12_381_BBS),
		GenG2:                NewG2(bls12381.NewBBSCurve().GenG2(), BLS12_381_BBS),
		GenGt:                NewGt(bls12381.NewBBSCurve().GenGt(), BLS12_381_BBS),
		GroupOrder:           NewZr(bls12381.NewCurve().GroupOrder(), BLS12_381_BBS),
		CoordByteSize:        bls12381.NewBBSCurve().CoordinateByteSize(),
		G1ByteSize:           bls12381.NewBBSCurve().G1ByteSize(),
		CompressedG1ByteSize: bls12381.NewBBSCurve().CompressedG1ByteSize(),
		G2ByteSize:           bls12381.NewBBSCurve().G2ByteSize(),
		CompressedG2ByteSize: bls12381.NewBBSCurve().CompressedG2ByteSize(),
		ScalarByteSize:       bls12381.NewBBSCurve().ScalarByteSize(),
		curveID:              BLS12_381_BBS,
	},
	{
		c:                    bls12381.NewBBSCurve(),
		GenG1:                NewG1(bls12381.NewBBSCurve().GenG1(), BLS12_381_BBS_GURVY),
		GenG2:                NewG2(bls12381.NewBBSCurve().GenG2(), BLS12_381_BBS_GURVY),
		GenGt:                NewGt(bls12381.NewBBSCurve().GenGt(), BLS12_381_BBS_GURVY),
		GroupOrder:           NewZr(bls12381.NewCurve().GroupOrder(), BLS12_381_BBS_GURVY),
		CoordByteSize:        bls12381.NewBBSCurve().CoordinateByteSize(),
		G1ByteSize:           bls12381.NewBBSCurve().G1ByteSize(),
		CompressedG1ByteSize: bls12381.NewBBSCurve().CompressedG1ByteSize(),
		G2ByteSize:           bls12381.NewBBSCurve().G2ByteSize(),
		CompressedG2ByteSize: bls12381.NewBBSCurve().CompressedG2ByteSize(),
		ScalarByteSize:       bls12381.NewBBSCurve().ScalarByteSize(),
		curveID:              BLS12_381_BBS_GURVY,
	},
}

/*********************************************************************/

// Zr represents an element in the scalar field of an elliptic curve.
// These are integers modulo the curve's group order, used for scalar multiplication
// and other arithmetic operations in pairing-based cryptography.
//
// Zr elements support standard arithmetic operations (addition, subtraction,
// multiplication) as well as modular operations and conversions to/from various
// numeric types.
//
// Example:
//
//	curve := math.Curves[math.BLS12_381]
//	a := curve.NewZrFromInt(5)
//	b := curve.NewZrFromInt(7)
//	c := a.Plus(b)  // c = 12 (mod curve order)
type Zr struct {
	zr      driver.Zr
	curveID CurveID
}

// NewZr creates a new Zr element from a driver.Zr implementation and curve ID.
// This is typically used internally; users should use Curve methods like
// NewZrFromInt, NewZrFromBytes, or NewRandomZr instead.
func NewZr(zr driver.Zr, curveID CurveID) *Zr {
	return &Zr{zr: zr, curveID: curveID}
}

// IsZero returns true if this scalar is zero.
func (z *Zr) IsZero() bool {
	return z.zr.IsZero()
}

// IsOne returns true if this scalar is one.
func (z *Zr) IsOne() bool {
	return z.zr.IsOne()
}

// BigInt returns the scalar as a *big.Int.
// BigInt assumes that its output will not be altered by the caller.
// It responsibility of the caller to clone the output of BigInt if needed.
func (z *Zr) BigInt() *big.Int {
	return z.zr.BigInt()
}

// CurveID returns the curve identifier for this scalar.
func (z *Zr) CurveID() CurveID {
	return z.curveID
}

// Plus returns a new Zr representing (z + a) mod order.
func (z *Zr) Plus(a *Zr) *Zr {
	return &Zr{zr: z.zr.Plus(a.zr), curveID: z.curveID}
}

// Minus returns a new Zr representing (z - a) mod order.
func (z *Zr) Minus(a *Zr) *Zr {
	return &Zr{zr: z.zr.Minus(a.zr), curveID: z.curveID}
}

// Mul returns a new Zr representing (z * a) mod order.
func (z *Zr) Mul(a *Zr) *Zr {
	return &Zr{zr: z.zr.Mul(a.zr), curveID: z.curveID}
}

// Mod sets z to z mod a in place.
func (z *Zr) Mod(a *Zr) {
	z.zr.Mod(a.zr)
}

// PowMod returns a new Zr representing z^a mod order.
func (z *Zr) PowMod(a *Zr) *Zr {
	return &Zr{zr: z.zr.PowMod(a.zr), curveID: z.curveID}
}

// InvModP sets z to its modular inverse modulo a in place.
func (z *Zr) InvModP(a *Zr) {
	z.zr.InvModP(a.zr)
}

// InvModOrder sets z to its modular inverse modulo the curve order in place.
func (z *Zr) InvModOrder() {
	z.zr.InvModOrder()
}

// Bytes returns the byte representation of this scalar.
// The format is backend-specific but typically big-endian.
func (z *Zr) Bytes() []byte {
	return z.zr.Bytes()
}

// Equals returns true if z and a represent the same scalar value.
func (z *Zr) Equals(a *Zr) bool {
	return z.zr.Equals(a.zr)
}

// Copy returns a deep copy of this scalar.
func (z *Zr) Copy() *Zr {
	return &Zr{zr: z.zr.Copy(), curveID: z.curveID}
}

// Clone copies the value of a into z.
func (z *Zr) Clone(a *Zr) {
	z.zr.Clone(a.zr)
}

// String returns a string representation of this scalar.
func (z *Zr) String() string {
	return z.zr.String()
}

// Neg negates z in place (z = -z mod order).
func (z *Zr) Neg() {
	z.zr.Neg()
}

var zerobytes = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
var onebytes = []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

// Uint converts the scalar to a uint64.
// Returns an error if the value is out of the uint64 range.
func (z *Zr) Uint() (uint64, error) {
	b := z.Bytes()
	if !bytes.Equal(zerobytes, b[:32-8]) && !bytes.Equal(onebytes, b[:32-8]) {
		return 0, errors.New("out of range")
	}

	return binary.BigEndian.Uint64(b[32-8:]), nil
}

// Int converts the scalar to an int64.
// Returns an error if the value is out of the int64 range.
func (z *Zr) Int() (int64, error) {
	b := z.Bytes()
	if !bytes.Equal(zerobytes, b[:32-8]) && !bytes.Equal(onebytes, b[:32-8]) {
		return 0, errors.New("out of range")
	}

	u := binary.BigEndian.Uint64(b[32-8:])

	return int64(u), nil // #nosec G115
}

/*********************************************************************/

// G1 represents a point on the first elliptic curve group in pairing-based cryptography.
// G1 is typically the "smaller" group in terms of representation size and is used for
// signatures, commitments, and as the first argument in pairing operations.
//
// G1 points support group operations (addition, scalar multiplication) and can be
// serialized in both compressed and uncompressed formats.
//
// Example:
//
//	curve := math.Curves[math.BLS12_381]
//	scalar := curve.NewZrFromInt(42)
//	point := curve.GenG1.Mul(scalar)  // [42]G1
//	point.Add(curve.GenG1)            // [43]G1
type G1 struct {
	g1      driver.G1
	curveID CurveID
}

// NewG1 creates a new G1 point from a driver.G1 implementation and curve ID.
// This is typically used internally; users should use Curve methods like
// NewG1, NewG1FromBytes, or HashToG1 instead.
func NewG1(g1 driver.G1, curveID CurveID) *G1 {
	return &G1{g1: g1, curveID: curveID}
}

// CurveID returns the curve identifier for this G1 point.
func (g *G1) CurveID() CurveID {
	return g.curveID
}

// Clone copies the value of a into g.
func (g *G1) Clone(a *G1) {
	g.g1.Clone(a.g1)
}

// Copy returns a deep copy of this G1 point.
func (g *G1) Copy() *G1 {
	return &G1{g1: g.g1.Copy(), curveID: g.curveID}
}

// Add adds point a to g in place (g = g + a).
func (g *G1) Add(a *G1) {
	g.g1.Add(a.g1)
}

// Mul returns a new G1 point representing scalar multiplication [a]g.
func (g *G1) Mul(a *Zr) *G1 {
	return &G1{g1: g.g1.Mul(a.zr), curveID: g.curveID}
}

// Mul2 computes [e]g + [f]Q and returns the result as a new G1 point.
// On the gnark-backed curves this allocates far less than two separate Mul calls
// plus an Add, but it is not necessarily faster: the joint Strauss-Shamir technique
// used here forgoes the GLV endomorphism speedup that Mul benefits from, so wall-clock
// time is comparable to (not better than) the naive two-Mul-plus-Add approach.
func (g *G1) Mul2(e *Zr, Q *G1, f *Zr) *G1 {
	return &G1{g1: g.g1.Mul2(e.zr, Q.g1, f.zr), curveID: g.curveID}
}

// Mul2InPlace computes [e]g + [f]Q and stores the result in g.
// This is more efficient than Mul2 when the result can overwrite g.
func (g *G1) Mul2InPlace(e *Zr, Q *G1, f *Zr) {
	g.g1.Mul2InPlace(e.zr, Q.g1, f.zr)
}

// Equals returns true if g and a represent the same point.
func (g *G1) Equals(a *G1) bool {
	return g.g1.Equals(a.g1)
}

// Bytes returns the uncompressed byte representation of this G1 point.
// The format is backend-specific but typically includes both coordinates.
func (g *G1) Bytes() []byte {
	return g.g1.Bytes()
}

// Compressed returns the compressed byte representation of this G1 point.
// Compressed format uses roughly half the space of uncompressed format.
func (g *G1) Compressed() []byte {
	return g.g1.Compressed()
}

// Sub subtracts point a from g in place (g = g - a).
func (g *G1) Sub(a *G1) {
	g.g1.Sub(a.g1)
}

// IsInfinity returns true if this point is the point at infinity (identity element).
func (g *G1) IsInfinity() bool {
	return g.g1.IsInfinity()
}

// String returns a string representation of this G1 point.
func (g *G1) String() string {
	return g.g1.String()
}

// Neg negates g in place (g = -g).
func (g *G1) Neg() {
	g.g1.Neg()
}

/*********************************************************************/

// G2 represents a point on the second elliptic curve group in pairing-based cryptography.
// G2 is typically the "larger" group (on a twisted curve) and is used as the second
// argument in pairing operations. In many protocols, public keys reside in G2 while
// signatures are in G1.
//
// G2 points support group operations (addition, scalar multiplication) and can be
// serialized in both compressed and uncompressed formats.
//
// Example:
//
//	curve := math.Curves[math.BLS12_381]
//	scalar := curve.NewZrFromInt(42)
//	point := curve.GenG2.Mul(scalar)  // [42]G2
type G2 struct {
	g2      driver.G2
	curveID CurveID
}

// NewG2 creates a new G2 point from a driver.G2 implementation and curve ID.
// This is typically used internally; users should use Curve methods like
// NewG2, NewG2FromBytes, or HashToG2 instead.
func NewG2(g2 driver.G2, curveID CurveID) *G2 {
	return &G2{g2: g2, curveID: curveID}
}

// CurveID returns the curve identifier for this G2 point.
func (g *G2) CurveID() CurveID {
	return g.curveID
}

// Clone copies the value of a into g.
func (g *G2) Clone(a *G2) {
	g.g2.Clone(a.g2)
}

// Copy returns a deep copy of this G2 point.
func (g *G2) Copy() *G2 {
	return &G2{g2: g.g2.Copy(), curveID: g.curveID}
}

// Mul returns a new G2 point representing scalar multiplication [a]g.
func (g *G2) Mul(a *Zr) *G2 {
	return &G2{g2: g.g2.Mul(a.zr), curveID: g.curveID}
}

// Add adds point a to g in place (g = g + a).
func (g *G2) Add(a *G2) {
	g.g2.Add(a.g2)
}

// Sub subtracts point a from g in place (g = g - a).
func (g *G2) Sub(a *G2) {
	g.g2.Sub(a.g2)
}

// Affine converts g to affine coordinates in place.
// This may be required by some operations or for serialization.
func (g *G2) Affine() {
	g.g2.Affine()
}

// Bytes returns the uncompressed byte representation of this G2 point.
// The format is backend-specific but typically includes all coordinates.
func (g *G2) Bytes() []byte {
	return g.g2.Bytes()
}

// Compressed returns the compressed byte representation of this G2 point.
// Compressed format uses roughly half the space of uncompressed format.
func (g *G2) Compressed() []byte {
	return g.g2.Compressed()
}

// String returns a string representation of this G2 point.
func (g *G2) String() string {
	return g.g2.String()
}

// Equals returns true if g and a represent the same point.
func (g *G2) Equals(a *G2) bool {
	return g.g2.Equals(a.g2)
}

/*********************************************************************/

// Gt represents an element in the target group of a pairing operation.
// Gt is the multiplicative group that results from pairing G1 and G2 elements.
// In pairing-based cryptography, the pairing function e: G2 × G1 → Gt has the
// bilinearity property: e([a]G2, [b]G1) = e(G2, G1)^(ab).
//
// Gt elements support multiplication, exponentiation, and inversion operations.
//
// Example:
//
//	curve := math.Curves[math.BLS12_381]
//	gt1 := curve.Pairing(curve.GenG2, curve.GenG1)
//	scalar := curve.NewZrFromInt(5)
//	gt2 := gt1.Exp(scalar)  // gt1^5
type Gt struct {
	gt      driver.Gt
	curveID CurveID
}

// NewGt creates a new Gt element from a driver.Gt implementation and curve ID.
// This is typically used internally; users should use Curve.Pairing or
// Curve.NewGtFromBytes instead.
func NewGt(gt driver.Gt, curveID CurveID) *Gt {
	return &Gt{gt: gt, curveID: curveID}
}

// CurveID returns the curve identifier for this Gt element.
func (g *Gt) CurveID() CurveID {
	return g.curveID
}

// Equals returns true if g and a represent the same target group element.
func (g *Gt) Equals(a *Gt) bool {
	return g.gt.Equals(a.gt)
}

// Inverse computes the multiplicative inverse of g in place (g = g^-1).
func (g *Gt) Inverse() {
	g.gt.Inverse()
}

// Mul multiplies g by a in place (g = g * a).
func (g *Gt) Mul(a *Gt) {
	g.gt.Mul(a.gt)
}

// Exp returns a new Gt element representing g^z (exponentiation).
func (g *Gt) Exp(z *Zr) *Gt {
	return &Gt{gt: g.gt.Exp(z.zr), curveID: g.curveID}
}

// IsUnity returns true if g is the identity element (unity) in Gt.
func (g *Gt) IsUnity() bool {
	return g.gt.IsUnity()
}

// String returns a string representation of this Gt element.
func (g *Gt) String() string {
	return g.gt.ToString()
}

// Bytes returns the byte representation of this Gt element.
// The format is backend-specific.
func (g *Gt) Bytes() []byte {
	return g.gt.Bytes()
}

/*********************************************************************/

// Curve represents a pairing-friendly elliptic curve and provides the main interface
// for cryptographic operations. It encapsulates the curve parameters, generator points,
// and factory methods for creating group elements.
//
// A Curve instance provides:
//   - Generator points (GenG1, GenG2, GenGt) for each group
//   - The group order (GroupOrder) as a Zr element
//   - Size information for serialization
//   - Factory methods for creating and deserializing group elements
//   - Pairing operations
//   - Hash-to-curve operations
//   - Modular arithmetic operations
//
// Example:
//
//	curve := math.Curves[math.BLS12_381]
//	rng, _ := curve.Rand()
//	secretKey := curve.NewRandomZr(rng)
//	publicKey := curve.GenG1.Mul(secretKey)
//	message := []byte("sign this")
//	signature := curve.HashToG1(message).Mul(secretKey)
type Curve struct {
	c                    driver.Curve
	GenG1                *G1 // Generator point for the G1 group
	GenG2                *G2 // Generator point for the G2 group
	GenGt                *Gt // Generator (identity) element for the Gt group
	GroupOrder           *Zr // Order of the curve groups
	CoordByteSize        int // Size of a single coordinate in bytes
	G1ByteSize           int // Size of uncompressed G1 point in bytes
	CompressedG1ByteSize int // Size of compressed G1 point in bytes
	G2ByteSize           int // Size of uncompressed G2 point in bytes
	CompressedG2ByteSize int // Size of compressed G2 point in bytes
	ScalarByteSize       int // Size of scalar (Zr) in bytes
	curveID              CurveID
}

// NewCurve creates a new Curve instance with the specified parameters.
// This is typically used internally during package initialization to create
// the pre-configured curves in the Curves slice. Most users should use
// the pre-configured curves rather than creating custom instances.
//
// Parameters:
//   - c: The underlying driver implementation
//   - genG1, genG2, genGt: Generator points for each group
//   - groupOrder: The order of the curve groups
//   - coordByteSize: Size of a coordinate in bytes
//   - g1ByteSize, compressedG1ByteSize: Sizes for G1 serialization
//   - g2ByteSize, compressedG2ByteSize: Sizes for G2 serialization
//   - scalarByteSize: Size of scalars in bytes
//   - curveID: Identifier for this curve configuration
func NewCurve(
	c driver.Curve,
	genG1 *G1,
	genG2 *G2,
	genGt *Gt,
	groupOrder *Zr,
	coordByteSize int,
	g1ByteSize int,
	compressedG1ByteSize int,
	g2ByteSize int,
	compressedG2ByteSize int,
	scalarByteSize int,
	curveID CurveID,
) *Curve {
	return &Curve{
		c:                    c,
		GenG1:                genG1,
		GenG2:                genG2,
		GenGt:                genGt,
		GroupOrder:           groupOrder,
		CoordByteSize:        coordByteSize,
		G1ByteSize:           g1ByteSize,
		CompressedG1ByteSize: compressedG1ByteSize,
		G2ByteSize:           g2ByteSize,
		CompressedG2ByteSize: compressedG2ByteSize,
		ScalarByteSize:       scalarByteSize,
		curveID:              curveID,
	}
}

// ID returns the curve identifier for this curve.
func (c *Curve) ID() CurveID {
	return c.curveID
}

// Rand returns a cryptographically secure random number generator.
// Returns an error if the RNG cannot be initialized.
func (c *Curve) Rand() (io.Reader, error) {
	return c.c.Rand()
}

// NewRandomZr generates a random scalar using the provided random number generator.
// The scalar is uniformly distributed in the range [0, group order).
func (c *Curve) NewRandomZr(rng io.Reader) *Zr {
	return &Zr{zr: c.c.NewRandomZr(rng), curveID: c.curveID}
}

// NewZrFromBytes creates a Zr scalar from its byte representation.
// The byte slice should be in the format produced by Zr.Bytes().
func (c *Curve) NewZrFromBytes(b []byte) *Zr {
	return &Zr{zr: c.c.NewZrFromBytes(b), curveID: c.curveID}
}

// NewG1FromBytes deserializes a G1 point from its uncompressed byte representation.
// Returns an error if the bytes are invalid or don't represent a valid point.
func (c *Curve) NewG1FromBytes(b []byte) (p *G1, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &G1{g1: c.c.NewG1FromBytes(b), curveID: c.curveID}

	return
}

// NewG2FromBytes deserializes a G2 point from its uncompressed byte representation.
// Returns an error if the bytes are invalid or don't represent a valid point.
func (c *Curve) NewG2FromBytes(b []byte) (p *G2, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &G2{g2: c.c.NewG2FromBytes(b), curveID: c.curveID}

	return
}

// NewG1FromCompressed deserializes a G1 point from its compressed byte representation.
// Returns an error if the bytes are invalid or don't represent a valid point.
func (c *Curve) NewG1FromCompressed(b []byte) (p *G1, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &G1{g1: c.c.NewG1FromCompressed(b), curveID: c.curveID}

	return
}

// NewG2FromCompressed deserializes a G2 point from its compressed byte representation.
// Returns an error if the bytes are invalid or don't represent a valid point.
func (c *Curve) NewG2FromCompressed(b []byte) (p *G2, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &G2{g2: c.c.NewG2FromCompressed(b), curveID: c.curveID}

	return
}

// NewGtFromBytes deserializes a Gt element from its byte representation.
// Returns an error if the bytes are invalid or don't represent a valid element.
func (c *Curve) NewGtFromBytes(b []byte) (p *Gt, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("failure [%s]", r)
			p = nil
		}
	}()

	p = &Gt{gt: c.c.NewGtFromBytes(b), curveID: c.curveID}

	return
}

// NewZrFromInt creates a Zr scalar from an int64 value.
func (c *Curve) NewZrFromInt(i int64) *Zr {
	return &Zr{zr: c.c.NewZrFromInt64(i), curveID: c.curveID}
}

// NewZrFromUint64 creates a Zr scalar from a uint64 value.
func (c *Curve) NewZrFromUint64(i uint64) *Zr {
	return &Zr{zr: c.c.NewZrFromUint64(i), curveID: c.curveID}
}

// NewZrFromBigInt creates a Zr scalar from a *big.Int value.
// The value is reduced modulo the curve order.
func (c *Curve) NewZrFromBigInt(i *big.Int) *Zr {
	return &Zr{zr: c.c.NewZrFromBigInt(i), curveID: c.curveID}
}

// NewG2 creates a new G2 point initialized to the identity element (point at infinity).
func (c *Curve) NewG2() *G2 {
	return &G2{g2: c.c.NewG2(), curveID: c.curveID}
}

// NewG1 creates a new G1 point initialized to the identity element (point at infinity).
func (c *Curve) NewG1() *G1 {
	return &G1{g1: c.c.NewG1(), curveID: c.curveID}
}

// Pairing computes the bilinear pairing e(a, b) where a ∈ G2 and b ∈ G1.
// The result is an element in the target group Gt.
// The pairing satisfies the bilinearity property: e([x]a, [y]b) = e(a, b)^(xy).
func (c *Curve) Pairing(a *G2, b *G1) *Gt {
	return &Gt{gt: c.c.Pairing(a.g2, b.g1), curveID: c.curveID}
}

// Pairing2 efficiently computes e(p, q) * e(r, s) where p, r ∈ G2 and q, s ∈ G1.
// This is more efficient than computing two separate pairings and multiplying them.
func (c *Curve) Pairing2(p *G2, q *G1, r *G2, s *G1) *Gt {
	return &Gt{gt: c.c.Pairing2(p.g2, r.g2, q.g1, s.g1), curveID: c.curveID}
}

// FExp performs the final exponentiation in the pairing computation.
// This is typically used internally but exposed for advanced use cases.
func (c *Curve) FExp(a *Gt) *Gt {
	return &Gt{gt: c.c.FExp(a.gt), curveID: c.curveID}
}

// HashToZr hashes arbitrary data to a scalar in Zr using a cryptographic hash function.
// The output is uniformly distributed in the scalar field.
func (c *Curve) HashToZr(data []byte) *Zr {
	return &Zr{zr: c.c.HashToZr(data), curveID: c.curveID}
}

// HashToG1 hashes arbitrary data to a point in G1 using a hash-to-curve algorithm.
// This is useful for creating deterministic points from messages.
func (c *Curve) HashToG1(data []byte) *G1 {
	return &G1{g1: c.c.HashToG1(data), curveID: c.curveID}
}

// HashToG1WithDomain hashes data to a G1 point with domain separation.
// The domain parameter prevents hash collisions across different protocols or contexts.
// Returns nil if the underlying hash-to-curve algorithm rejects the input (e.g. a domain
// longer than 255 bytes), rather than panicking.
func (c *Curve) HashToG1WithDomain(data, domain []byte) (p *G1) {
	defer func() {
		if r := recover(); r != nil {
			p = nil
		}
	}()

	p = &G1{g1: c.c.HashToG1WithDomain(data, domain), curveID: c.curveID}

	return
}

// HashToG2 hashes arbitrary data to a point in G2 using a hash-to-curve algorithm.
func (c *Curve) HashToG2(data []byte) *G2 {
	return &G2{g2: c.c.HashToG2(data), curveID: c.curveID}
}

// HashToG2WithDomain hashes data to a G2 point with domain separation.
// The domain parameter prevents hash collisions across different protocols or contexts.
// Returns nil if the underlying hash-to-curve algorithm rejects the input (e.g. a domain
// longer than 255 bytes), rather than panicking.
func (c *Curve) HashToG2WithDomain(data, domain []byte) (p *G2) {
	defer func() {
		if r := recover(); r != nil {
			p = nil
		}
	}()

	p = &G2{g2: c.c.HashToG2WithDomain(data, domain), curveID: c.curveID}

	return
}

// ModSub computes (a - b) mod m.
func (c *Curve) ModSub(a, b, m *Zr) *Zr {
	return &Zr{zr: c.c.ModSub(a.zr, b.zr, m.zr), curveID: c.curveID}
}

// ModAdd computes (a + b) mod m.
func (c *Curve) ModAdd(a, b, m *Zr) *Zr {
	return &Zr{zr: c.c.ModAdd(a.zr, b.zr, m.zr), curveID: c.curveID}
}

// ModMul computes (a1 * b1) mod m.
func (c *Curve) ModMul(a1, b1, m *Zr) *Zr {
	return &Zr{zr: c.c.ModMul(a1.zr, b1.zr, m.zr), curveID: c.curveID}
}

// ModNeg computes (-a1) mod m.
func (c *Curve) ModNeg(a1, m *Zr) *Zr {
	return &Zr{zr: c.c.ModNeg(a1.zr, m.zr), curveID: c.curveID}
}

// ModAddMul computes the sum of products: (a1[0]*b1[0] + a1[1]*b1[1] + ... + a1[n]*b1[n]) mod m.
// This is more efficient than computing each product separately.
// The slices a1 and b1 must have the same length.
func (c *Curve) ModAddMul(a1, b1 []*Zr, m *Zr) *Zr {
	a1Driver := make([]driver.Zr, len(a1))
	b1Driver := make([]driver.Zr, len(b1))
	for i := range a1 {
		a1Driver[i] = a1[i].zr
		b1Driver[i] = b1[i].zr
	}

	return &Zr{zr: c.c.ModAddMul(a1Driver, b1Driver, m.zr), curveID: c.curveID}
}

// ModAddMul2 computes (a1*a2 + b1*b2) mod m.
// This is more efficient than computing the products separately.
func (c *Curve) ModAddMul2(a1, a2, b1, b2 *Zr, m *Zr) *Zr {
	return &Zr{zr: c.c.ModAddMul2(a1.zr, a2.zr, b1.zr, b2.zr, m.zr), curveID: c.curveID}
}

// ModAddMul3 computes (a1*a2 + b1*b2 + c1*c2) mod m.
// This is more efficient than computing the products separately.
func (c *Curve) ModAddMul3(a1, a2, b1, b2, c1, c2 *Zr, m *Zr) *Zr {
	return &Zr{
		zr:      c.c.ModAddMul3(a1.zr, a2.zr, b1.zr, b2.zr, c1.zr, c2.zr, m.zr),
		curveID: c.curveID,
	}
}

// MultiScalarMul computes a multi-scalar multiplication: [b[0]]a[0] + [b[1]]a[1] + ... + [b[n]]a[n].
// This is significantly more efficient than computing each scalar multiplication separately.
// The slices a and b must have the same length.
func (c *Curve) MultiScalarMul(a []*G1, b []*Zr) *G1 {
	aDriver := make([]driver.G1, len(a))
	bDriver := make([]driver.Zr, len(b))
	for i := range a {
		aDriver[i] = a[i].g1
		bDriver[i] = b[i].zr
	}

	return &G1{g1: c.c.MultiScalarMul(aDriver, bDriver), curveID: c.curveID}
}

// ModMulInPlace computes (a * b) mod m and stores the result in result.
// This avoids allocating a new Zr for the result.
func (c *Curve) ModMulInPlace(result, a, b, m *Zr) {
	c.c.ModMulInPlace(result.zr, a.zr, b.zr, m.zr)
}

// ModAddMul2InPlace computes (a1*c1 + b1*c2) mod m and stores the result in result.
// This avoids allocating a new Zr for the result.
func (c *Curve) ModAddMul2InPlace(result, a1, c1, b1, c2, m *Zr) {
	c.c.ModAddMul2InPlace(result.zr, a1.zr, c1.zr, b1.zr, c2.zr, m.zr)
}

// ModAddMul3InPlace computes (a1*a2 + b1*b2 + c1*c2) mod m and stores the result in result.
// This avoids allocating a new Zr for the result.
func (c *Curve) ModAddMul3InPlace(result, a1, a2, b1, b2, c1, c2, m *Zr) {
	c.c.ModAddMul3InPlace(result.zr, a1.zr, a2.zr, b1.zr, b2.zr, c1.zr, c2.zr, m.zr)
}
