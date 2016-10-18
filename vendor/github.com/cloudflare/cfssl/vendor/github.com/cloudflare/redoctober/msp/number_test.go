package msp

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestFieldElemMultiplicationOne(t *testing.T) {
	x := FieldElem(make([]byte, ModulusSize))
	rand.Read(x)

	xy, yx := x.Mul(One), One.Mul(x)

	if !One.IsOne() {
		t.Fatalf("One is not one?")
	}

	if bytes.Compare(xy, x) != 0 || bytes.Compare(yx, x) != 0 {
		t.Fatalf("Multiplication by 1 failed!\nx = %x\n1*x = %x\nx*1 = %x", x, yx, xy)
	}
}

func TestFieldElemMultiplicationZero(t *testing.T) {
	x := FieldElem(make([]byte, ModulusSize))
	rand.Read(x)

	xy, yx := x.Mul(Zero), Zero.Mul(x)

	if !Zero.IsZero() {
		t.Fatalf("Zero is not zero?")
	}

	if !xy.IsZero() || !yx.IsZero() {
		t.Fatalf("Multiplication by 0 failed!\nx = %x\n0*x = %x\nx*0 = %x", x, yx, xy)
	}
}

func TestFieldElemInvert(t *testing.T) {
	x := FieldElem(make([]byte, ModulusSize))
	rand.Read(x)

	xInv := x.Invert()

	xy, yx := x.Mul(xInv), xInv.Mul(x)

	if !xy.IsOne() || !yx.IsOne() {
		t.Fatalf("Multiplication by inverse failed!\nx = %x\nxInv = %x\nxInv*x = %x\nx*xInv = %x", x, xInv, yx, xy)
	}
}
