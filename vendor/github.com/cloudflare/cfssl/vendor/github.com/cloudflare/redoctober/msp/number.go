// Polynomial fields with coefficients in GF(2)
package msp

import (
	"bytes"
)

type FieldElem []byte

var (
	Modulus        FieldElem = []byte{135, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // x^128 + x^7 + x^2 + x + 1
	ModulusSize    int       = 16
	ModulusBitSize int       = 128

	Zero FieldElem = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	One  FieldElem = []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
)

// NewFieldElem returns a new zero element.
func NewFieldElem() FieldElem {
	return FieldElem(make([]byte, ModulusSize))
}

// AddM mutates e into e+f.
func (e FieldElem) AddM(f FieldElem) {
	for i := 0; i < ModulusSize; i++ {
		e[i] ^= f[i]
	}
}

// Add returns e+f.
func (e FieldElem) Add(f FieldElem) FieldElem {
	out := e.Dup()
	out.AddM(f)

	return out
}

// Mul returns e*f.
func (e FieldElem) Mul(f FieldElem) FieldElem {
	out := NewFieldElem()

	for i := 0; i < ModulusBitSize; i++ { // Foreach bit e_i in e:
		if e.getCoeff(i) == 1 { // where e_i equals 1:
			temp := f.Dup() // Multiply f * x^i mod M(x):

			for j := 0; j < i; j++ { // Multiply f by x mod M(x), i times.
				carry := temp.shift()

				if carry {
					temp[0] ^= Modulus[0]
				}
			}

			out.AddM(temp) // Add f * x^i to the output
		}
	}

	return out
}

// Exp returns e^i.
func (e FieldElem) Exp(i int) FieldElem {
	out := One.Dup()

	for j := 0; j < i; j++ {
		out = out.Mul(e)
	}

	return out
}

// Invert returns the multiplicative inverse of e.
func (e FieldElem) Invert() FieldElem {
	out, temp := e.Dup(), e.Dup()

	for i := 0; i < 126; i++ {
		temp = temp.Mul(temp)
		out = out.Mul(temp)
	}

	return out.Mul(out)
}

// getCoeff returns the ith coefficient of the field element: either 0 or 1.
func (e FieldElem) getCoeff(i int) byte {
	return (e[i/8] >> (uint(i) % 8)) & 1
}

// shift multiplies e by 2 and returns true if there was overflow and false if there wasn't.
func (e FieldElem) shift() bool {
	carry := false

	for i := 0; i < ModulusSize; i++ {
		nextCarry := e[i] >= 128

		e[i] = e[i] << 1
		if carry {
			e[i]++
		}
		carry = nextCarry
	}

	return carry
}

func (e FieldElem) IsZero() bool { return bytes.Compare(e, Zero) == 0 }
func (e FieldElem) IsOne() bool  { return bytes.Compare(e, One) == 0 }

// Dup returns a duplicate of e.
func (e FieldElem) Dup() FieldElem {
	out := FieldElem(make([]byte, ModulusSize))
	copy(out, e)

	return out
}
