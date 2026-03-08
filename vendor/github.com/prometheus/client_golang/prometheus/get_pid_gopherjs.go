<<<<<<<< HEAD:vendor/github.com/consensys/gnark-crypto/ecc/bls12-377/fp/asm.go
//go:build !noadx
// +build !noadx

// Copyright 2020 ConsenSys Software Inc.
//
========
// Copyright 2015 The Prometheus Authors
>>>>>>>> main:vendor/github.com/prometheus/client_golang/prometheus/get_pid_gopherjs.go
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

//go:build js && !wasm
// +build js,!wasm

<<<<<<<< HEAD:vendor/github.com/consensys/gnark-crypto/ecc/bls12-377/fp/asm.go
package fp

import "golang.org/x/sys/cpu"

var (
	supportAdx = cpu.X86.HasADX && cpu.X86.HasBMI2
	_          = supportAdx
)
========
package prometheus

func getPIDFn() func() (int, error) {
	return func() (int, error) {
		return 1, nil
	}
}
>>>>>>>> main:vendor/github.com/prometheus/client_golang/prometheus/get_pid_gopherjs.go
