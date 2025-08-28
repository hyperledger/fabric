// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

// Package ecc provides bls12-381, bls12-377, bn254, bw6-761, bls24-315, bls24-317, bw6-633, secp256k1 and stark-curve elliptic curves implementation (+pairing).
//
// Also
//
//   - Multi exponentiation
//   - FFT
//   - Polynomial commitment schemes
//   - MiMC
//   - twisted edwards "companion curves"
//   - EdDSA (on the "companion" twisted edwards curves)
package ecc

// ID represent a unique ID for a curve
type ID uint16

// do not modify the order of this enum
const (
	UNKNOWN ID = iota
	BN254
	BLS12_377
	BLS12_381
	BLS24_315
	BLS24_317
	BW6_761
	BW6_633
	STARK_CURVE
	SECP256K1
	GRUMPKIN
)

// MultiExpConfig enables to set optional configuration attribute to a call to MultiExp
type MultiExpConfig struct {
	NbTasks int // go routines to be used in the multiexp. can be larger than num cpus.
}
