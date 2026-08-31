/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package driver defines the interface layer for pairing-based cryptography implementations.
// This package uses a driver pattern to support multiple backend implementations (AMCL, Gurvy, Kilic)
// while providing a consistent API to the higher-level math package.
//
// # Architecture
//
// The driver package defines interfaces for:
//   - Curve: Factory and operations for a specific elliptic curve
//   - Zr: Scalar field elements (integers modulo curve order)
//   - G1: Points on the first curve group
//   - G2: Points on the second curve group (twisted curve)
//   - Gt: Elements in the target group (pairing results)
//
// # Implementing a New Backend
//
// To add a new backend implementation:
//  1. Implement all five interfaces (Curve, Zr, G1, G2, Gt)
//  2. Ensure thread-safety if required by your use case
//  3. Handle edge cases (point at infinity, zero scalar, etc.)
//  4. Validate all inputs in deserialization methods
//  5. Register your implementation in the math package
//
// # Thread Safety
//
// Implementations are not required to be thread-safe. Users should implement
// their own synchronization when sharing instances across goroutines.
package driver

import (
	"io"
	"math/big"
)

// Curve defines the interface for a pairing-friendly elliptic curve implementation.
// It provides factory methods for creating group elements, pairing operations,
// and various cryptographic primitives.
//
// Implementations must handle:
//   - Point validation during deserialization
//   - Proper error handling (may panic on invalid input)
//   - Consistent serialization formats
//   - Efficient pairing computations
type Curve interface {
	// Pairing computes the bilinear pairing e(G2, G1) → Gt.
	Pairing(G2, G1) Gt

	// Pairing2 efficiently computes e(p2a, p1a) * e(p2b, p1b).
	Pairing2(p2a, p2b G2, p1a, p1b G1) Gt

	// FExp performs the final exponentiation in pairing computation.
	FExp(Gt) Gt

	// ModMul computes (a1 * b1) mod m.
	ModMul(a1, b1, m Zr) Zr

	// ModNeg computes (-a1) mod m.
	ModNeg(a1, m Zr) Zr

	// GenG1 returns the generator point for the G1 group.
	GenG1() G1

	// GenG2 returns the generator point for the G2 group.
	GenG2() G2

	// GenGt returns the generator (identity) element for the Gt group.
	GenGt() Gt

	// GroupOrder returns the order of the curve groups as a Zr element.
	GroupOrder() Zr

	// CoordinateByteSize returns the size of a single coordinate in bytes.
	CoordinateByteSize() int

	// G1ByteSize returns the size of an uncompressed G1 point in bytes.
	G1ByteSize() int

	// CompressedG1ByteSize returns the size of a compressed G1 point in bytes.
	CompressedG1ByteSize() int

	// G2ByteSize returns the size of an uncompressed G2 point in bytes.
	G2ByteSize() int

	// CompressedG2ByteSize returns the size of a compressed G2 point in bytes.
	CompressedG2ByteSize() int

	// ScalarByteSize returns the size of a scalar (Zr) in bytes.
	ScalarByteSize() int

	// NewG1 creates a new G1 point at the identity (point at infinity).
	NewG1() G1

	// NewG2 creates a new G2 point at the identity (point at infinity).
	NewG2() G2

	// NewZrFromBytes deserializes a Zr scalar from bytes.
	NewZrFromBytes(b []byte) Zr

	// NewZrFromInt64 creates a Zr scalar from an int64 value.
	NewZrFromInt64(i int64) Zr

	// NewZrFromUint64 creates a Zr scalar from a uint64 value.
	NewZrFromUint64(i uint64) Zr

	// NewZrFromBigInt creates a Zr scalar from a *big.Int, reducing modulo the curve order.
	NewZrFromBigInt(i *big.Int) Zr

	// NewG1FromBytes deserializes a G1 point from uncompressed bytes.
	// May panic if bytes are invalid.
	NewG1FromBytes(b []byte) G1

	// NewG1FromCompressed deserializes a G1 point from compressed bytes.
	// May panic if bytes are invalid.
	NewG1FromCompressed(b []byte) G1

	// NewG2FromBytes deserializes a G2 point from uncompressed bytes.
	// May panic if bytes are invalid.
	NewG2FromBytes(b []byte) G2

	// NewG2FromCompressed deserializes a G2 point from compressed bytes.
	// May panic if bytes are invalid.
	NewG2FromCompressed(b []byte) G2

	// NewGtFromBytes deserializes a Gt element from bytes.
	// May panic if bytes are invalid.
	NewGtFromBytes(b []byte) Gt

	// ModAdd computes (a + b) mod m.
	ModAdd(a, b, m Zr) Zr

	// ModSub computes (a - b) mod m.
	ModSub(a, b, m Zr) Zr

	// HashToZr hashes data to a scalar using a cryptographic hash function.
	HashToZr(data []byte) Zr

	// HashToG1 hashes data to a G1 point using a hash-to-curve algorithm.
	HashToG1(data []byte) G1

	// HashToG1WithDomain hashes data to G1 with domain separation.
	HashToG1WithDomain(data, domain []byte) G1

	// HashToG2 hashes data to a G2 point using a hash-to-curve algorithm.
	HashToG2(data []byte) G2

	// HashToG2WithDomain hashes data to G2 with domain separation.
	HashToG2WithDomain(data, domain []byte) G2

	// NewRandomZr generates a random scalar using the provided RNG.
	NewRandomZr(rng io.Reader) Zr

	// Rand returns a cryptographically secure random number generator.
	Rand() (io.Reader, error)

	// ModAddMul computes sum of products: (driver[0]*driver2[0] + ... + driver[n]*driver2[n]) mod zr.
	ModAddMul(driver []Zr, driver2 []Zr, zr Zr) Zr

	// ModAddMul2 computes (a1*c1 + b1*c2) mod m.
	ModAddMul2(a1 Zr, c1 Zr, b1 Zr, c2 Zr, m Zr) Zr

	// ModAddMul3 computes (a1*a2 + b1*b2 + c1*c2) mod m.
	ModAddMul3(a1 Zr, a2 Zr, b1 Zr, b2 Zr, c1 Zr, c2 Zr, m Zr) Zr

	// MultiScalarMul computes multi-scalar multiplication: [b[0]]a[0] + [b[1]]a[1] + ... + [b[n]]a[n].
	MultiScalarMul(a []G1, b []Zr) G1

	// ModMulInPlace computes (a * b) mod m and stores the result in result.
	ModMulInPlace(result, a, b, m Zr)

	// ModAddMul2InPlace computes (a1*c1 + b1*c2) mod m and stores the result in result.
	ModAddMul2InPlace(result Zr, a1, c1, b1, c2, m Zr)

	// ModAddMul3InPlace computes (a1*a2 + b1*b2 + c1*c2) mod m and stores the result in result.
	ModAddMul3InPlace(result Zr, a1, a2, b1, b2, c1, c2, m Zr)
}

// Zr represents an element in the scalar field of an elliptic curve.
// Scalars are integers modulo the curve's group order and are used for
// scalar multiplication and other arithmetic operations.
//
// Implementations must:
//   - Handle modular arithmetic correctly
//   - Support conversion to/from various numeric types
//   - Provide efficient arithmetic operations
//   - Handle edge cases (zero, one, etc.)
type Zr interface {
	// IsZero returns true if this scalar is zero.
	IsZero() bool

	// IsOne returns true if this scalar is one.
	IsOne() bool

	// BigInt returns the scalar as a *big.Int.
	BigInt() *big.Int

	// Plus returns a new Zr representing (this + Zr) mod order.
	Plus(Zr) Zr

	// Minus returns a new Zr representing (this - Zr) mod order.
	Minus(Zr) Zr

	// Mul returns a new Zr representing (this * Zr) mod order.
	Mul(Zr) Zr

	// Mod sets this scalar to (this mod Zr) in place.
	Mod(Zr)

	// PowMod returns a new Zr representing this^Zr mod order.
	PowMod(Zr) Zr

	// InvModP sets this scalar to its modular inverse modulo Zr in place.
	InvModP(Zr)

	// Bytes returns the byte representation of this scalar.
	Bytes() []byte

	// Equals returns true if this scalar equals the given scalar.
	Equals(Zr) bool

	// Copy returns a deep copy of this scalar.
	Copy() Zr

	// Clone copies the value of a into this scalar.
	Clone(a Zr)

	// String returns a string representation of this scalar.
	String() string

	// Neg negates this scalar in place (this = -this mod order).
	Neg()

	// InvModOrder sets this scalar to its modular inverse modulo the curve order in place.
	InvModOrder()
}

// G1 represents a point on the first elliptic curve group.
// G1 is typically used for signatures and commitments in pairing-based protocols.
//
// Implementations must:
//   - Handle the point at infinity (identity element) correctly
//   - Validate points during deserialization
//   - Support both compressed and uncompressed serialization
//   - Provide efficient group operations
type G1 interface {
	// Clone copies the value of the given G1 point into this point.
	Clone(G1)

	// Copy returns a deep copy of this G1 point.
	Copy() G1

	// Add adds the given G1 point to this point in place (this = this + G1).
	Add(G1)

	// Mul returns a new G1 point representing scalar multiplication [Zr]this.
	Mul(Zr) G1

	// Mul2 computes [e]this + [f]Q and returns the result as a new G1 point.
	Mul2(e Zr, Q G1, f Zr) G1

	// Mul2InPlace computes [e]this + [f]Q and stores the result in this point.
	Mul2InPlace(e Zr, Q G1, f Zr)

	// Equals returns true if this point equals the given point.
	Equals(G1) bool

	// Bytes returns the uncompressed byte representation of this point.
	Bytes() []byte

	// Compressed returns the compressed byte representation of this point.
	Compressed() []byte

	// Sub subtracts the given G1 point from this point in place (this = this - G1).
	Sub(G1)

	// IsInfinity returns true if this point is the point at infinity (identity element).
	IsInfinity() bool

	// String returns a string representation of this point.
	String() string

	// Neg negates this point in place (this = -this).
	Neg()
}

// G2 represents a point on the second elliptic curve group (twisted curve).
// G2 is typically used for public keys in pairing-based protocols.
//
// Implementations must:
//   - Handle the point at infinity (identity element) correctly
//   - Validate points during deserialization
//   - Support both compressed and uncompressed serialization
//   - Provide efficient group operations
//   - Handle affine coordinate conversion when needed
type G2 interface {
	// Clone copies the value of the given G2 point into this point.
	Clone(G2)

	// Copy returns a deep copy of this G2 point.
	Copy() G2

	// Mul returns a new G2 point representing scalar multiplication [Zr]this.
	Mul(Zr) G2

	// Add adds the given G2 point to this point in place (this = this + G2).
	Add(G2)

	// Sub subtracts the given G2 point from this point in place (this = this - G2).
	Sub(G2)

	// Affine converts this point to affine coordinates in place.
	Affine()

	// Bytes returns the uncompressed byte representation of this point.
	Bytes() []byte

	// Compressed returns the compressed byte representation of this point.
	Compressed() []byte

	// String returns a string representation of this point.
	String() string

	// Equals returns true if this point equals the given point.
	Equals(G2) bool
}

// Gt represents an element in the target group of a pairing operation.
// Gt is a multiplicative group that results from pairing G1 and G2 elements.
//
// Implementations must:
//   - Handle the identity element (unity) correctly
//   - Provide efficient multiplication and exponentiation
//   - Support inversion operations
//   - Ensure consistent serialization
type Gt interface {
	// Equals returns true if this element equals the given element.
	Equals(Gt) bool

	// Inverse computes the multiplicative inverse of this element in place (this = this^-1).
	Inverse()

	// Mul multiplies this element by the given element in place (this = this * Gt).
	Mul(Gt)

	// IsUnity returns true if this element is the identity element (unity).
	IsUnity() bool

	// ToString returns a string representation of this element.
	ToString() string

	// Bytes returns the byte representation of this element.
	Bytes() []byte

	// Exp returns a new Gt element representing this^Zr (exponentiation).
	Exp(Zr) Gt
}
