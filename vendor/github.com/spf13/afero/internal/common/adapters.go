<<<<<<<< HEAD:vendor/github.com/consensys/gnark-crypto/ecc/bls12-377/internal/fptower/parameters.go
// Copyright 2020 ConsenSys AG
========
// Copyright © 2022 Steve Francia <spf@spf13.com>.
>>>>>>>> main:vendor/github.com/spf13/afero/internal/common/adapters.go
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

<<<<<<<< HEAD:vendor/github.com/consensys/gnark-crypto/ecc/bls12-377/internal/fptower/parameters.go
package fptower

import (
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bls12-377/fr"
)

// generator of the curve
var xGen big.Int

var glvBasis ecc.Lattice

func init() {
	xGen.SetString("9586122913090633729", 10)
	_r := fr.Modulus()
	ecc.PrecomputeLattice(_r, &xGen, &glvBasis)
========
package common

import "io/fs"

// FileInfoDirEntry provides an adapter from os.FileInfo to fs.DirEntry
type FileInfoDirEntry struct {
	fs.FileInfo
>>>>>>>> main:vendor/github.com/spf13/afero/internal/common/adapters.go
}

var _ fs.DirEntry = FileInfoDirEntry{}

func (d FileInfoDirEntry) Type() fs.FileMode { return d.FileInfo.Mode().Type() }

func (d FileInfoDirEntry) Info() (fs.FileInfo, error) { return d.FileInfo, nil }
