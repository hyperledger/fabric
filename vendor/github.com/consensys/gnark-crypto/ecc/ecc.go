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

// Package ecc is an elliptic curve (+pairing) library.
// Provides implementation for bls12-381, bls12-377, bn254, bw6-761 and their twisted edwards "companion curves"
package ecc

// do not modify the order of this enum
const (
	UNKNOWN ID = iota
	BN254
	BLS12_377
	BLS12_381
	BW6_761
)

// ID represent a unique ID for a curve
type ID uint16

func (id ID) String() string {
	switch id {
	case BLS12_377:
		return "bls12_377"
	case BLS12_381:
		return "bls12_381"
	case BN254:
		return "bn254"
	case BW6_761:
		return "bw6_761"
	default:
		panic("unimplemented ecc ID")
	}
}
