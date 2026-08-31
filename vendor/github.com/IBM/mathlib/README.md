# mathlib

[![License](https://img.shields.io/badge/license-Apache%202-blue)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/IBM/mathlib)](https://goreportcard.com/badge/github.com/IBM/mathlib)
[![Go](https://github.com/IBM/mathlib/actions/workflows/go.yml/badge.svg)](https://github.com/IBM/mathlib/actions/workflows/go.yml/badge.svg)
[![GoDoc](https://godoc.org/github.com/IBM/mathlib?status.svg)](https://godoc.org/github.com/IBM/mathlib)

A high-performance Go library for pairing-based cryptography operations over elliptic curve groups.

## Overview

`mathlib` provides a unified interface for performing cryptographic operations on pairing-friendly elliptic curves. It supports multiple curve implementations and backends, making it suitable for various cryptographic protocols including:

- Zero-knowledge proofs
- Anonymous credentials

The library abstracts the complexity of pairing operations while providing flexibility to choose different curve types and backend implementations based on performance and security requirements.

## Features

- **Multiple Curve Support**: FP256BN, BN254, BLS12-381, BLS12-377, and BBS+ variants
- **Pluggable Backends**: Support for AMCL and Gurvy implementations
- **Type-Safe API**: Strongly-typed group elements (G1, G2, Gt, Zr)
- **Efficient Operations**: Optimized pairing computations and multi-scalar multiplications
- **Serialization**: Support for both compressed and uncompressed point representations
- **Hash-to-Curve**: Secure hashing to curve points with optional domain separation
- **Modular Arithmetic**: Comprehensive scalar field operations

## Installation

```bash
go get github.com/IBM/mathlib
```

**Requirements:**
- Go 1.25 or higher

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/IBM/mathlib"
)

func main() {
    // Select a curve (BLS12-381 in this example)
    curve := math.Curves[math.BLS12_381]
    
    // Generate random scalars
    rng, _ := curve.Rand()
    a := curve.NewRandomZr(rng)
    b := curve.NewRandomZr(rng)
    
    // Perform scalar multiplication on G1
    P := curve.GenG1.Mul(a)
    Q := curve.GenG1.Mul(b)
    
    // Compute pairing
    e1 := curve.Pairing(curve.GenG2, P)
    e2 := curve.Pairing(curve.GenG2, Q)
    
    // Multiply in target group
    e1.Mul(e2)
    
    fmt.Printf("Pairing result: %s\n", e1.String())
}
```

## Supported Curves

| Curve ID | Description | Backend | Use Case |
|----------|-------------|---------|----------|
| `FP256BN_AMCL` | 256-bit Barreto-Naehrig curve | AMCL | General-purpose pairing operations |
| `FP256BN_AMCL_MIRACL` | 256-bit BN curve (MIRACL variant) | AMCL | Legacy compatibility |
| `BN254` | 254-bit Barreto-Naehrig curve | Gurvy | High-performance applications |
| `BLS12_381` | BLS12-381 curve | Gurvy | Modern protocols, BLS signatures |
| `BLS12_381_GURVY` | BLS12-381 curve | Gurvy | Performance-optimized BLS12-381 |
| `BLS12_377_GURVY` | BLS12-377 curve | Gurvy | Recursive proof systems |
| `BLS12_381_BBS` | BLS12-381 for BBS+ signatures | Gurvy | Anonymous credentials |
| `BLS12_381_BBS_GURVY` | BLS12-381 for BBS+ signatures | Gurvy | High-performance BBS+ |

> **Note:** `BLS12_381` and `BLS12_381_BBS` were previously backed by the Kilic
> implementation. They are now backed by Gurvy (gnark-crypto) and remain
> byte-compatible with both their prior output and their explicit `_GURVY`
> siblings.

### Choosing a Curve

- **BLS12-381**: Recommended for new projects, widely standardized, excellent security margins
- **BN254**: Good performance, but security margins are tighter than BLS12-381
- **BLS12-377**: Specialized for recursive proof composition (e.g., zk-SNARKs)
- **BBS+ variants**: Specifically optimized for BBS+ signature schemes

## API Overview

### Core Types

- **`Curve`**: Main interface for curve operations, provides factory methods for group elements
- **`Zr`**: Elements in the scalar field (integers modulo curve order)
- **`G1`**: Points on the first elliptic curve group
- **`G2`**: Points on the second elliptic curve group (twisted curve)
- **`Gt`**: Elements in the target group (result of pairing operations)

### Key Operations

```go
// Curve selection
curve := math.Curves[math.BLS12_381]

// Scalar operations
a := curve.NewZrFromInt(42)
b := curve.HashToZr([]byte("some data"))
c := a.Plus(b)

// G1 operations
P := curve.GenG1.Mul(a)
Q := curve.HashToG1([]byte("hash to point"))
P.Add(Q)

// G2 operations
R := curve.GenG2.Mul(b)

// Pairing
e := curve.Pairing(R, P)

// Target group operations
e2 := e.Exp(c)
```

## Usage Examples

### Example 1: Basic Pairing Operation

```go
curve := math.Curves[math.BLS12_381]

// Create scalars
a := curve.NewZrFromInt(5)
b := curve.NewZrFromInt(7)

// Compute [a]G1 and [b]G2
P := curve.GenG1.Mul(a)
Q := curve.GenG2.Mul(b)

// Compute pairing e([b]G2, [a]G1)
result := curve.Pairing(Q, P)

// Verify bilinearity: e(G2, [ab]G1) == e([b]G2, [a]G1)
ab := a.Mul(b)
expected := curve.Pairing(curve.GenG2, curve.GenG1.Mul(ab))

if result.Equals(expected) {
    fmt.Println("Pairing bilinearity verified!")
}
```

### Example 2: Serialization and Deserialization

```go
curve := math.Curves[math.BLS12_381]

// Create a point
rng, _ := curve.Rand()
scalar := curve.NewRandomZr(rng)
point := curve.GenG1.Mul(scalar)

// Serialize (uncompressed)
bytes := point.Bytes()

// Deserialize
recovered, err := curve.NewG1FromBytes(bytes)
if err != nil {
    panic(err)
}

// Serialize (compressed)
compressed := point.Compressed()
recoveredCompressed, err := curve.NewG1FromCompressed(compressed)
if err != nil {
    panic(err)
}

fmt.Printf("Original and recovered points match: %v\n", 
    point.Equals(recovered) && point.Equals(recoveredCompressed))
```

### Example 3: Hash-to-Curve with Domain Separation

```go
curve := math.Curves[math.BLS12_381]

// Hash to G1 with domain separation
message := []byte("sign this message")
domain := []byte("my-application-v1")

point := curve.HashToG1WithDomain(message, domain)

// Use in signature scheme
rng, _ := curve.Rand()
secretKey := curve.NewRandomZr(rng)
signature := point.Mul(secretKey)

fmt.Printf("Signature: %x\n", signature.Compressed())
```

### Example 4: Multi-Scalar Multiplication

```go
curve := math.Curves[math.BLS12_381]

// Create multiple points and scalars
points := []*math.G1{
    curve.GenG1,
    curve.HashToG1([]byte("point2")),
    curve.HashToG1([]byte("point3")),
}

scalars := []*math.Zr{
    curve.NewZrFromInt(2),
    curve.NewZrFromInt(3),
    curve.NewZrFromInt(5),
}

// Efficient multi-scalar multiplication: [2]P1 + [3]P2 + [5]P3
result := curve.MultiScalarMul(points, scalars)

fmt.Printf("Multi-scalar multiplication result: %s\n", result.String())
```

## Architecture

### Driver Pattern

`mathlib` uses a driver pattern to support multiple backend implementations:

```
┌─────────────────────────────────────┐
│         mathlib (Public API)        │
│  Curve, G1, G2, Gt, Zr types        │
└─────────────────┬───────────────────┘
                  │
                  ▼
┌─────────────────────────────────────┐
│      driver (Interface Layer)       │
│  Curve, G1, G2, Gt, Zr interfaces   │
└─────────────────┬───────────────────┘
                  │
        ┌─────────┴─────────┐
        ▼                   ▼
    ┌──────┐            ┌──────┐
    │ AMCL │            │Gurvy │
    └──────┘            └──────┘
```

This design allows:
- **Flexibility**: Easy addition of new curve implementations
- **Performance**: Choose the fastest backend for your use case
- **Compatibility**: Support for different cryptographic libraries

### Backend Implementations

- **AMCL (Apache Milagro Crypto Library)**: Mature, well-tested implementation
- **Gurvy** (gnark-crypto): High-performance Go-native implementation with assembly optimizations; backs all BLS12-381, BLS12-377, and BN254 curves

## Performance Considerations

- Use compressed point serialization when bandwidth is limited
- Prefer `Pairing2` for double pairings (more efficient than two separate pairings)
- Use `MultiScalarMul` for multiple scalar multiplications (faster than individual operations)
- Consider `Mul2` and `Mul2InPlace` for combined operations on G1
- BLS12-381 with Gurvy backend offers excellent performance for most applications

## Testing

Run the test suite:

```bash
make unit-tests
```

Run benchmarks:

```bash
make perf
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Follow Go coding conventions and style guidelines
- Add tests for new functionality
- Update documentation for API changes
- Run `make checks` and `make lint` before committing
- Ensure all tests pass and linters are satisfied

## Security Considerations

- Always use cryptographically secure random number generators
- Validate all deserialized points (the library does this automatically)
- Use appropriate curve parameters for your security requirements
- Consider timing attack mitigations for sensitive operations
- Keep dependencies up to date

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## References

- [Pairing-Based Cryptography](https://en.wikipedia.org/wiki/Pairing-based_cryptography)
- [BLS12-381 Specification](https://github.com/zkcrypto/bls12_381)
- [BLS Signatures](https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-bls-signature)
- [Apache Milagro](https://milagro.apache.org/)

## Acknowledgments

This library builds upon the excellent work of:
- Apache Milagro Crypto Library (AMCL)
- ConsenSys gnark-crypto (Gurvy) library
- Kilic BLS12-381 implementation (basis for the original BLS12-381 backend and the big-endian-sign hash-to-curve now reimplemented natively)