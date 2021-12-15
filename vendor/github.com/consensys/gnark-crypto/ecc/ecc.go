/*
Copyright © 2020 ConsenSys

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package ecc provides bls12-381, bls12-377, bn254, bw6-761, bls24-315 and bw6-633 elliptic curves implementation (+pairing).
//
// Also
//
//	* Multi exponentiation
//	* FFT
//	* Polynomial commitment schemes
//	* MiMC
//	* twisted edwards "companion curves"
//	* EdDSA (on the "companion" twisted edwards curves)
package ecc

import (
	"math/big"

	"github.com/consensys/gnark-crypto/internal/generator/config"
)

// ID represent a unique ID for a curve
type ID uint16

// do not modify the order of this enum
const (
	UNKNOWN ID = iota
	BN254
	BLS12_377
	BLS12_381
	BLS24_315
	BW6_761
	BW6_633
)

// Implemented return the list of curves fully implemented in gnark-crypto
func Implemented() []ID {
	return []ID{BN254, BLS12_377, BLS12_381, BW6_761, BLS24_315}
}

func (id ID) String() string {
	// TODO link with config.XXX.Name ?
	switch id {
	case BLS12_377:
		return "bls12_377"
	case BLS12_381:
		return "bls12_381"
	case BN254:
		return "bn254"
	case BW6_761:
		return "bw6_761"
	case BW6_633:
		return "bw6_633"
	case BLS24_315:
		return "bls24_315"
	default:
		panic("unimplemented ecc ID")
	}
}

// Info returns constants related to a curve
func (id ID) Info() Info {
	// note to avoid circular dependency these are hard coded
	// values are checked for non regression in code generation
	switch id {
	case BLS12_377:
		return newInfo(&config.BLS12_377)
	case BLS12_381:
		return newInfo(&config.BLS12_381)
	case BN254:
		return newInfo(&config.BN254)
	case BW6_761:
		return newInfo(&config.BW6_761)
	case BW6_633:
		return newInfo(&config.BW6_633)
	case BLS24_315:
		return newInfo(&config.BLS24_315)
	default:
		panic("unimplemented ecc ID")
	}
}

func newInfo(c *config.Curve) Info {
	return Info{
		Fp: config.Field{
			Bits:    c.FpInfo.Bits,
			Bytes:   c.FpInfo.Bytes,
			Modulus: func() *big.Int { return new(big.Int).Set(c.FpInfo.Modulus()) },
		},
		Fr: config.Field{
			Bits:    c.FrInfo.Bits,
			Bytes:   c.FrInfo.Bytes,
			Modulus: func() *big.Int { return new(big.Int).Set(c.FrInfo.Modulus()) },
		},
	}
}

// Info contains constants related to a curve
type Info struct {
	Fp, Fr config.Field
}

// MultiExpConfig enables to set optional configuration attribute to a call to MultiExp
type MultiExpConfig struct {
	NbTasks     int  // go routines to be used in the multiexp. can be larger than num cpus.
	ScalarsMont bool // indicates if the scalars are in montgommery form. Default to false.
}
