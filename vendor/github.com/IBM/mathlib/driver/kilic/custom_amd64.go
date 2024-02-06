//go:build amd64 && !generic

/*
Copyright IBM Corp. All Rights Reserved.
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kilic

import (
	_ "unsafe"

	"golang.org/x/sys/cpu"
)

func init() {
	if !cpu.X86.HasADX || !cpu.X86.HasBMI2 {
		mul = mulNoADX
	}
}

var mul func(c, a, b *Fe) = mulADX

//go:linkname mulADX github.com/kilic/bls12-381.mulADX
func mulADX(c, a, b *Fe)

//go:linkname mulNoADX github.com/kilic/bls12-381.mulNoADX
func mulNoADX(c, a, b *Fe)
