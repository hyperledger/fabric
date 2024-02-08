package bls12381

import (
	"errors"
	"math/big"
)

type fp2Temp struct {
	t [4]*fe
}

type fp2 struct {
	fp2Temp
}

func newFp2Temp() fp2Temp {
	t := [4]*fe{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe{}
	}
	return fp2Temp{t}
}

func newFp2() *fp2 {
	t := newFp2Temp()
	return &fp2{t}
}

func (e *fp2) fromBytes(in []byte) (*fe2, error) {
	if len(in) != 2*fpByteSize {
		return nil, errors.New("input string must be equal to 96 bytes")
	}
	c1, err := fromBytes(in[:fpByteSize])
	if err != nil {
		return nil, err
	}
	c0, err := fromBytes(in[fpByteSize:])
	if err != nil {
		return nil, err
	}
	return &fe2{*c0, *c1}, nil
}

func (e *fp2) toBytes(a *fe2) []byte {
	out := make([]byte, 2*fpByteSize)
	copy(out[:fpByteSize], toBytes(&a[1]))
	copy(out[fpByteSize:], toBytes(&a[0]))
	return out
}

func (e *fp2) new() *fe2 {
	return new(fe2).zero()
}

func (e *fp2) zero() *fe2 {
	return new(fe2).zero()
}

func (e *fp2) one() *fe2 {
	return new(fe2).one()
}

func (e *fp2) fromMont(c, a *fe2) {
	// c0 = a0 / r
	// c1 = a1 / r
	fromMont(&c[0], &a[0])
	fromMont(&c[1], &a[1])
}

func (e *fp2) add(c, a, b *fe2) {
	// c0 = a0 + b0
	// c1 = a1 + b1
	add(&c[0], &a[0], &b[0])
	add(&c[1], &a[1], &b[1])
}

func (e *fp2) addAssign(a, b *fe2) {
	// a0 = a0 + b0
	// a1 = a1 + b1
	addAssign(&a[0], &b[0])
	addAssign(&a[1], &b[1])
}

func (e *fp2) ladd(c, a, b *fe2) {
	// c0 = a0 + b0
	// c1 = a1 + b1
	ladd(&c[0], &a[0], &b[0])
	ladd(&c[1], &a[1], &b[1])
}

func (e *fp2) double(c, a *fe2) {
	// c0 = 2a0
	// c1 = 2a1
	double(&c[0], &a[0])
	double(&c[1], &a[1])
}

func (e *fp2) doubleAssign(a *fe2) {
	// a0 = 2a0
	// a1 = 2a1
	doubleAssign(&a[0])
	doubleAssign(&a[1])
}

func (e *fp2) ldouble(c, a *fe2) {
	// c0 = 2a0
	// c1 = 2a1
	ldouble(&c[0], &a[0])
	ldouble(&c[1], &a[1])
}

func (e *fp2) sub(c, a, b *fe2) {
	// c0 = a0 - b0
	// c1 = a1 - b1
	sub(&c[0], &a[0], &b[0])
	sub(&c[1], &a[1], &b[1])
}

func (e *fp2) subAssign(c, a *fe2) {
	// a0 = a0 - b0
	// a1 = a1 - b1
	subAssign(&c[0], &a[0])
	subAssign(&c[1], &a[1])
}

func (e *fp2) neg(c, a *fe2) {
	// c0 = -a0
	// c1 = -a1
	neg(&c[0], &a[0])
	neg(&c[1], &a[1])
}

func (e *fp2) conjugate(c, a *fe2) {
	// c0 = a0
	// c1 = -a1
	c[0].set(&a[0])
	neg(&c[1], &a[1])
}

func (e *fp2) mul(c, a, b *fe2) {
	t := e.t
	// Guide to Pairing Based Cryptography
	// Algorithm 5.16

	mul(t[1], &a[0], &b[0])  // a0b0
	mul(t[2], &a[1], &b[1])  // a1b1
	ladd(t[0], &a[0], &a[1]) // a0 + a1
	ladd(t[3], &b[0], &b[1]) // b0 + b1
	sub(&c[0], t[1], t[2])   // c0 = a0b0 - a1b1
	addAssign(t[1], t[2])    // a0b0 + a1b1
	mul(t[0], t[0], t[3])    // (a0 + a1)(b0 + b1)
	sub(&c[1], t[0], t[1])   // c1 = (a0 + a1)(b0 + b1) - (a0b0 + a1b1)
}

func (e *fp2) mulAssign(a, b *fe2) {
	t := e.t
	mul(t[1], &a[0], &b[0])
	mul(t[2], &a[1], &b[1])
	ladd(t[0], &a[0], &a[1])
	ladd(t[3], &b[0], &b[1])
	sub(&a[0], t[1], t[2])
	addAssign(t[1], t[2])
	mul(t[0], t[0], t[3])
	sub(&a[1], t[0], t[1])
}

func (e *fp2) square(c, a *fe2) {
	t := e.t
	// Guide to Pairing Based Cryptography
	// Algorithm 5.16

	ladd(t[0], &a[0], &a[1]) // (a0 + a1)
	sub(t[1], &a[0], &a[1])  // (a0 - a1)
	ldouble(t[2], &a[0])     // 2a0
	mul(&c[0], t[0], t[1])   // c0 = (a0 + a1)(a0 - a1)
	mul(&c[1], t[2], &a[1])  // c1 = 2a0a1
}

func (e *fp2) squareAssign(a *fe2) {
	t := e.t
	ladd(t[0], &a[0], &a[1])
	sub(t[1], &a[0], &a[1])
	ldouble(t[2], &a[0])
	mul(&a[0], t[0], t[1])
	mul(&a[1], t[2], &a[1])
}

func (e *fp2) mul0(c, a *fe2, b *fe) {
	mul(&c[0], &a[0], b)
	mul(&c[1], &a[1], b)
}

func (e *fp2) mulByNonResidue(c, a *fe2) {
	t := e.t
	// c0 = (a0 - a1)
	// c1 = (a0 + a1)
	sub(t[0], &a[0], &a[1])
	add(&c[1], &a[0], &a[1])
	c[0].set(t[0])
}

func (e *fp2) mulByB(c, a *fe2) {
	t := e.t
	// c0 = 4a0 - 4a1
	// c1 = 4a0 + 4a1
	double(t[0], &a[0])
	doubleAssign(t[0])
	double(t[1], &a[1])
	doubleAssign(t[1])
	sub(&c[0], t[0], t[1])
	add(&c[1], t[0], t[1])
}

func (e *fp2) inverse(c, a *fe2) {
	t := e.t
	// Guide to Pairing Based Cryptography
	// Algorithm 5.16

	square(t[0], &a[0])     // a0^2
	square(t[1], &a[1])     // a1^2
	addAssign(t[0], t[1])   // a0^2 + a1^2
	inverse(t[0], t[0])     // (a0^2 + a1^2)^-1
	mul(&c[0], &a[0], t[0]) // c0 = a0(a0^2 + a1^2)^-1
	mul(t[0], t[0], &a[1])  // a1(a0^2 + a1^2)^-1
	neg(&c[1], t[0])        // c1 = a1(a0^2 + a1^2)^-1
}

func (e *fp2) inverseBatch(in []fe2) {

	n, N, setFirst := 0, len(in), false

	for i := 0; i < len(in); i++ {
		if !in[i].isZero() {
			n++
		}
	}
	if n == 0 {
		return
	}

	tA := make([]fe2, n)
	tB := make([]fe2, n)

	// a, ab, abc, abcd, ...
	for i, j := 0, 0; i < N; i++ {
		if !in[i].isZero() {
			if !setFirst {
				setFirst = true
				tA[j].set(&in[i])
			} else {
				e.mul(&tA[j], &in[i], &tA[j-1])
			}
			j = j + 1
		}
	}

	// (abcd...)^-1
	e.inverse(&tB[n-1], &tA[n-1])

	// a^-1, ab^-1, abc^-1, abcd^-1, ...
	for i, j := N-1, n-1; j != 0; i-- {
		if !in[i].isZero() {
			e.mul(&tB[j-1], &tB[j], &in[i])
			j = j - 1
		}
	}

	// a^-1, b^-1, c^-1, d^-1
	for i, j := 0, 0; i < N; i++ {
		if !in[i].isZero() {
			if setFirst {
				setFirst = false
				in[i].set(&tB[j])
			} else {
				e.mul(&in[i], &tA[j-1], &tB[j])
			}
			j = j + 1
		}
	}
}

func (e *fp2) exp(c, a *fe2, s *big.Int) {
	z := e.one()
	for i := s.BitLen() - 1; i >= 0; i-- {
		e.square(z, z)
		if s.Bit(i) == 1 {
			e.mul(z, z, a)
		}
	}
	c.set(z)
}

func (e *fp2) frobeniusMap1(a *fe2) {
	e.conjugate(a, a)
}

func (e *fp2) frobeniusMap(a *fe2, power int) {
	if power&1 == 1 {
		e.conjugate(a, a)
	}
}

func (e *fp2) sqrt(c, a *fe2) bool {
	u, x0, a1, alpha := &fe2{}, &fe2{}, &fe2{}, &fe2{}
	u.set(a)
	e.exp(a1, a, pMinus3Over4)
	e.square(alpha, a1)
	e.mul(alpha, alpha, a)
	e.mul(x0, a1, a)
	if alpha.equal(negativeOne2) {
		neg(&c[0], &x0[1])
		c[1].set(&x0[0])
		return true
	}
	e.add(alpha, alpha, e.one())
	e.exp(alpha, alpha, pMinus1Over2)
	e.mul(c, alpha, x0)
	e.square(alpha, c)
	return alpha.equal(u)
}

func (e *fp2) isQuadraticNonResidue(a *fe2) bool {
	c0, c1 := new(fe), new(fe)
	square(c0, &a[0])
	square(c1, &a[1])
	add(c1, c1, c0)
	return isQuadraticNonResidue(c1)
}

// faster square root algorith is adapted from blst library
// https://github.com/supranational/blst/blob/master/src/sqrt.c

func (e *fp2) sqrtBLST(out, inp *fe2) bool {
	aa, bb := new(fe), new(fe)
	ret := new(fe2)
	square(aa, &inp[0])
	square(bb, &inp[1])
	add(aa, aa, bb)
	sqrt(aa, aa)
	sub(bb, &inp[0], aa)
	add(aa, &inp[0], aa)
	if aa.isZero() {
		aa.set(bb)
	}
	mul(aa, aa, twoInv)
	rsqrt(&ret[0], aa)
	ret[1].set(&inp[1])
	mul(&ret[1], &ret[1], twoInv)
	mul(&ret[1], &ret[1], &ret[0])
	mul(&ret[0], &ret[0], aa)
	return e.sqrtAlignBLST(out, ret, ret, inp)
}

func (e *fp2) sqrtAlignBLST(out, ret, sqrt, inp *fe2) bool {

	t0, t1 := new(fe2), new(fe2)
	coeff := e.one()
	e.square(t0, sqrt)

	//
	e.sub(t1, t0, inp)
	isSqrt := t1.isZero()

	//
	e.add(t1, t0, inp)
	flag := t1.isZero()
	if flag {
		coeff.set(sqrtMinus1)
	}
	isSqrt = flag || isSqrt

	//
	sub(&t1[0], &t0[0], &inp[1])
	add(&t1[1], &t0[1], &inp[0])
	flag = t1.isZero()
	if flag {
		coeff.set(sqrtSqrtMinus1)
	}
	isSqrt = flag || isSqrt

	//
	add(&t1[0], &t0[0], &inp[1])
	sub(&t1[1], &t0[1], &inp[0])
	flag = t1.isZero()
	if flag {

		coeff.set(sqrtMinusSqrtMinus1)
	}
	isSqrt = flag || isSqrt

	e.mul(out, coeff, ret)
	return isSqrt
}
