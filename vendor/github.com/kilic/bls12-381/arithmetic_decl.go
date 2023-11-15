// +build amd64,!generic

package bls12381

import (
	"golang.org/x/sys/cpu"
)

func init() {
	if !cpu.X86.HasADX || !cpu.X86.HasBMI2 {
		mul = mulNoADX
		mulFR = mulNoADXFR
		lmulFR = lmulNoADXFR
	}
}

var mul func(c, a, b *fe) = mulADX

func square(c, a *fe) {
	mul(c, a, a)
}

func neg(c, a *fe) {
	if a.isZero() {
		c.set(a)
	} else {
		_neg(c, a)
	}
}

//go:noescape
func add(c, a, b *fe)

//go:noescape
func addAssign(a, b *fe)

//go:noescape
func ladd(c, a, b *fe)

//go:noescape
func laddAssign(a, b *fe)

//go:noescape
func double(c, a *fe)

//go:noescape
func doubleAssign(a *fe)

//go:noescape
func ldouble(c, a *fe)

//go:noescape
func sub(c, a, b *fe)

//go:noescape
func subAssign(a, b *fe)

//go:noescape
func lsubAssign(a, b *fe)

//go:noescape
func _neg(c, a *fe)

//go:noescape
func mulNoADX(c, a, b *fe)

//go:noescape
func mulADX(c, a, b *fe)

var mulFR func(c, a, b *Fr) = mulADXFR
var lmulFR func(c *wideFr, a, b *Fr) = lmulADXFR

func squareFR(c, a *Fr) {
	mulFR(c, a, a)
}

func negFR(c, a *Fr) {
	if a.IsZero() {
		c.Set(a)
	} else {
		_negFR(c, a)
	}
}

//go:noescape
func addFR(c, a, b *Fr)

//go:noescape
func laddAssignFR(a, b *Fr)

//go:noescape
func doubleFR(c, a *Fr)

//go:noescape
func subFR(c, a, b *Fr)

//go:noescape
func lsubAssignFR(a, b *Fr)

//go:noescape
func _negFR(c, a *Fr)

//go:noescape
func mulNoADXFR(c, a, b *Fr)

//go:noescape
func mulADXFR(c, a, b *Fr)

//go:noescape
func lmulADXFR(c *wideFr, a, b *Fr)

//go:noescape
func lmulNoADXFR(c *wideFr, a, b *Fr)

//go:noescape
func addwFR(a, b *wideFr)
