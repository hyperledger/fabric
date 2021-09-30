package fptower

import (
	"math/bits"
)

// MulByVW set z to x*(y*v*w) and return z
// here y*v*w means the E12 element with C1.B1=y and all other components 0
func (z *E12) MulByVW(x *E12, y *E2) *E12 {

	var result E12
	var yNR E2

	yNR.MulByNonResidue(y)
	result.C0.B0.Mul(&x.C1.B1, &yNR)
	result.C0.B1.Mul(&x.C1.B2, &yNR)
	result.C0.B2.Mul(&x.C1.B0, y)
	result.C1.B0.Mul(&x.C0.B2, &yNR)
	result.C1.B1.Mul(&x.C0.B0, y)
	result.C1.B2.Mul(&x.C0.B1, y)
	z.Set(&result)
	return z
}

// MulByV set z to x*(y*v) and return z
// here y*v means the E12 element with C0.B1=y and all other components 0
func (z *E12) MulByV(x *E12, y *E2) *E12 {

	var result E12
	var yNR E2

	yNR.MulByNonResidue(y)
	result.C0.B0.Mul(&x.C0.B2, &yNR)
	result.C0.B1.Mul(&x.C0.B0, y)
	result.C0.B2.Mul(&x.C0.B1, y)
	result.C1.B0.Mul(&x.C1.B2, &yNR)
	result.C1.B1.Mul(&x.C1.B0, y)
	result.C1.B2.Mul(&x.C1.B1, y)
	z.Set(&result)
	return z
}

// MulByV2W set z to x*(y*v^2*w) and return z
// here y*v^2*w means the E12 element with C1.B2=y and all other components 0
func (z *E12) MulByV2W(x *E12, y *E2) *E12 {

	var result E12
	var yNR E2

	yNR.MulByNonResidue(y)
	result.C0.B0.Mul(&x.C1.B0, &yNR)
	result.C0.B1.Mul(&x.C1.B1, &yNR)
	result.C0.B2.Mul(&x.C1.B2, &yNR)
	result.C1.B0.Mul(&x.C0.B1, &yNR)
	result.C1.B1.Mul(&x.C0.B2, &yNR)
	result.C1.B2.Mul(&x.C0.B0, y)
	z.Set(&result)
	return z
}

// Expt set z to x^t in E12 and return z (t is the generator of the BN curve)
func (z *E12) Expt(x *E12) *E12 {

	const tAbsVal uint64 = 4965661367192848881

	var result E12
	result.Set(x)

	l := bits.Len64(tAbsVal) - 2
	for i := l; i >= 0; i-- {
		result.CyclotomicSquare(&result)
		if tAbsVal&(1<<uint(i)) != 0 {
			result.Mul(&result, x)
		}
	}

	z.Set(&result)
	return z
}

// MulBy034 multiplication by sparse element
func (z *E12) MulBy034(c0, c3, c4 *E2) *E12 {

	var z0, z1, z2, z3, z4, z5, tmp1, tmp2 E2
	var t [12]E2

	z0 = z.C0.B0
	z1 = z.C0.B1
	z2 = z.C0.B2
	z3 = z.C1.B0
	z4 = z.C1.B1
	z5 = z.C1.B2

	tmp1.MulByNonResidue(c3)
	tmp2.MulByNonResidue(c4)

	t[0].Mul(&tmp1, &z5)
	t[1].Mul(&tmp2, &z4)
	t[2].Mul(c3, &z3)
	t[3].Mul(&tmp2, &z5)
	t[4].Mul(c3, &z4)
	t[5].Mul(c4, &z3)
	t[6].Mul(c3, &z0)
	t[7].Mul(&tmp2, &z2)
	t[8].Mul(c3, &z1)
	t[9].Mul(c4, &z0)
	t[10].Mul(c3, &z2)
	t[11].Mul(c4, &z1)

	z.C0.B0.Mul(c0, &z0).
		Add(&z.C0.B0, &t[0]).
		Add(&z.C0.B0, &t[1])
	z.C0.B1.Mul(c0, &z1).
		Add(&z.C0.B1, &t[2]).
		Add(&z.C0.B1, &t[3])
	z.C0.B2.Mul(c0, &z2).
		Add(&z.C0.B2, &t[4]).
		Add(&z.C0.B2, &t[5])
	z.C1.B0.Mul(c0, &z3).
		Add(&z.C1.B0, &t[6]).
		Add(&z.C1.B0, &t[7])
	z.C1.B1.Mul(c0, &z4).
		Add(&z.C1.B1, &t[8]).
		Add(&z.C1.B1, &t[9])
	z.C1.B2.Mul(c0, &z5).
		Add(&z.C1.B2, &t[10]).
		Add(&z.C1.B2, &t[11])

	return z
}
