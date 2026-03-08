<<<<<<<< HEAD:vendor/github.com/consensys/gnark-crypto/ecc/bls12-377/fp/element_utils.go
// Copyright 2020 ConsenSys Software Inc.
//
========
// Copyright 2018 The Prometheus Authors
>>>>>>>> main:vendor/github.com/prometheus/client_golang/prometheus/num_threads.go
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

<<<<<<<< HEAD:vendor/github.com/consensys/gnark-crypto/ecc/bls12-377/fp/element_utils.go
package fp

// MulByNonResidueInv ...
func (z *Element) MulByNonResidueInv(x *Element) *Element {
	qnrInv := Element{
		9255502405446297221,
		10229180150694123945,
		9215585410771530959,
		13357015519562362907,
		5437107869987383107,
		16259554076827459,
	}
	z.Mul(x, &qnrInv)
	return z
========
//go:build !js || wasm
// +build !js wasm

package prometheus

import "runtime"

// getRuntimeNumThreads returns the number of open OS threads.
func getRuntimeNumThreads() float64 {
	n, _ := runtime.ThreadCreateProfile(nil)
	return float64(n)
>>>>>>>> main:vendor/github.com/prometheus/client_golang/prometheus/num_threads.go
}
