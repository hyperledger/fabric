package bls12381

import (
	"errors"
	"math/big"
)

type fp6Temp struct {
	t [6]*fe2
}

type fp6 struct {
	fp2 *fp2
	fp6Temp
}

func newFp6Temp() fp6Temp {
	t := [6]*fe2{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe2{}
	}
	return fp6Temp{t}
}

func newFp6(f *fp2) *fp6 {
	t := newFp6Temp()
	if f == nil {
		return &fp6{newFp2(), t}
	}
	return &fp6{f, t}
}

func (e *fp6) fromBytes(b []byte) (*fe6, error) {
	if len(b) != 288 {
		return nil, errors.New("input string length must be equal to 288 bytes")
	}
	fp2 := e.fp2
	u2, err := fp2.fromBytes(b[:2*fpByteSize])
	if err != nil {
		return nil, err
	}
	u1, err := fp2.fromBytes(b[2*fpByteSize : 4*fpByteSize])
	if err != nil {
		return nil, err
	}
	u0, err := fp2.fromBytes(b[4*fpByteSize:])
	if err != nil {
		return nil, err
	}
	return &fe6{*u0, *u1, *u2}, nil
}

func (e *fp6) toBytes(a *fe6) []byte {
	fp2 := e.fp2
	out := make([]byte, 6*fpByteSize)
	copy(out[:2*fpByteSize], fp2.toBytes(&a[2]))
	copy(out[2*fpByteSize:4*fpByteSize], fp2.toBytes(&a[1]))
	copy(out[4*fpByteSize:], fp2.toBytes(&a[0]))
	return out
}

func (e *fp6) new() *fe6 {
	return new(fe6)
}

func (e *fp6) zero() *fe6 {
	return new(fe6)
}

func (e *fp6) one() *fe6 {
	return new(fe6).one()
}

func (e *fp6) add(c, a, b *fe6) {
	fp2 := e.fp2
	// c0 = a0 + b0
	// c1 = a1 + b1
	// c2 = a2 + b2
	fp2.add(&c[0], &a[0], &b[0])
	fp2.add(&c[1], &a[1], &b[1])
	fp2.add(&c[2], &a[2], &b[2])
}

func (e *fp6) addAssign(a, b *fe6) {
	fp2 := e.fp2
	// a0 = a0 + b0
	// a1 = a1 + b1
	// a2 = a2 + b2
	fp2.addAssign(&a[0], &b[0])
	fp2.addAssign(&a[1], &b[1])
	fp2.addAssign(&a[2], &b[2])
}

func (e *fp6) double(c, a *fe6) {
	fp2 := e.fp2
	// c0 = 2a0
	// c1 = 2a1
	// c2 = 2a2
	fp2.double(&c[0], &a[0])
	fp2.double(&c[1], &a[1])
	fp2.double(&c[2], &a[2])
}

func (e *fp6) doubleAssign(a *fe6) {
	fp2 := e.fp2
	// c0 = 2c0
	// c1 = 2c1
	// c2 = 2c2
	fp2.doubleAssign(&a[0])
	fp2.doubleAssign(&a[1])
	fp2.doubleAssign(&a[2])
}

func (e *fp6) sub(c, a, b *fe6) {
	fp2 := e.fp2
	// c0 = a0 - b0
	// c1 = a1 - b1
	// c2 = a2 - b2
	fp2.sub(&c[0], &a[0], &b[0])
	fp2.sub(&c[1], &a[1], &b[1])
	fp2.sub(&c[2], &a[2], &b[2])
}

func (e *fp6) subAssign(a, b *fe6) {
	fp2 := e.fp2
	// a0 = a0 - b0
	// a1 = a1 - b1
	// a2 = a2 - b2
	fp2.subAssign(&a[0], &b[0])
	fp2.subAssign(&a[1], &b[1])
	fp2.subAssign(&a[2], &b[2])
}

func (e *fp6) neg(c, a *fe6) {
	fp2 := e.fp2
	// c0 = -a0
	// c1 = -a1
	// c2 = -a2
	fp2.neg(&c[0], &a[0])
	fp2.neg(&c[1], &a[1])
	fp2.neg(&c[2], &a[2])
}

func (e *fp6) conjugate(c, a *fe6) {
	fp2 := e.fp2
	// c0 = a0
	// c1 = -a1
	// c2 = a2
	c[0].set(&a[0])
	fp2.neg(&c[1], &a[1])
	c[0].set(&a[2])
}

func (e *fp6) mul(c, a, b *fe6) {
	fp2, t := e.fp2, e.t
	// Guide to Pairing Based Cryptography
	// Algorithm 5.21

	fp2.mul(t[0], &a[0], &b[0])     // v0 = a0b0
	fp2.mul(t[1], &a[1], &b[1])     // v1 = a1b1
	fp2.mul(t[2], &a[2], &b[2])     // v2 = a2b2
	fp2.add(t[3], &a[1], &a[2])     // a1 + a2
	fp2.add(t[4], &b[1], &b[2])     // b1 + b2
	fp2.mulAssign(t[3], t[4])       // (a1 + a2)(b1 + b2)
	fp2.add(t[4], t[1], t[2])       // v1 + v2
	fp2.subAssign(t[3], t[4])       // (a1 + a2)(b1 + b2) - v1 - v2
	fp2.mulByNonResidue(t[3], t[3]) // ((a1 + a2)(b1 + b2) - v1 - v2)β
	fp2.addAssign(t[3], t[0])       // c0 = ((a1 + a2)(b1 + b2) - v1 - v2)β + v0
	fp2.add(t[5], &a[0], &a[1])     // a0 + a1
	fp2.add(t[4], &b[0], &b[1])     // b0 + b1
	fp2.mulAssign(t[5], t[4])       // (a0 + a1)(b0 + b1)
	fp2.add(t[4], t[0], t[1])       // v0 + v1
	fp2.subAssign(t[5], t[4])       // (a0 + a1)(b0 + b1) - v0 - v1
	fp2.mulByNonResidue(t[4], t[2]) // βv2
	fp2.add(&c[1], t[5], t[4])      // c1 = (a0 + a1)(b0 + b1) - v0 - v1 + βv2
	fp2.add(t[5], &a[0], &a[2])     // a0 + a2
	fp2.add(t[4], &b[0], &b[2])     // b0 + b2
	fp2.mulAssign(t[5], t[4])       // (a0 + a2)(b0 + b2)
	fp2.add(t[4], t[0], t[2])       // v0 + v2
	fp2.subAssign(t[5], t[4])       // (a0 + a2)(b0 + b2) - v0 - v2
	fp2.add(&c[2], t[1], t[5])      // c2 = (a0 + a2)(b0 + b2) - v0 - v2 + v1
	c[0].set(t[3])
}

func (e *fp6) mulAssign(a, b *fe6) {
	fp2, t := e.fp2, e.t
	fp2.mul(t[0], &a[0], &b[0])
	fp2.mul(t[1], &a[1], &b[1])
	fp2.mul(t[2], &a[2], &b[2])
	fp2.add(t[3], &a[1], &a[2])
	fp2.add(t[4], &b[1], &b[2])
	fp2.mulAssign(t[3], t[4])
	fp2.add(t[4], t[1], t[2])
	fp2.subAssign(t[3], t[4])
	fp2.mulByNonResidue(t[3], t[3])
	fp2.addAssign(t[3], t[0])
	fp2.add(t[5], &a[0], &a[1])
	fp2.add(t[4], &b[0], &b[1])
	fp2.mulAssign(t[5], t[4])
	fp2.add(t[4], t[0], t[1])
	fp2.subAssign(t[5], t[4])
	fp2.mulByNonResidue(t[4], t[2])
	fp2.add(&a[1], t[5], t[4])
	fp2.add(t[5], &a[0], &a[2])
	fp2.add(t[4], &b[0], &b[2])
	fp2.mulAssign(t[5], t[4])
	fp2.add(t[4], t[0], t[2])
	fp2.subAssign(t[5], t[4])
	fp2.add(&a[2], t[1], t[5])
	a[0].set(t[3])
}

func (e *fp6) square(c, a *fe6) {
	fp2, t := e.fp2, e.t
	// Multiplication and Squaring on Pairing-Friendly Fields
	// Algorithm CH-SQR2
	// https://eprint.iacr.org/2006/471

	fp2.square(t[0], &a[0])         // s0 = a0^2
	fp2.mul(t[1], &a[0], &a[1])     // a0a1
	fp2.doubleAssign(t[1])          // s1 = 2a0a1
	fp2.sub(t[2], &a[0], &a[1])     // a0 - a1
	fp2.addAssign(t[2], &a[2])      // a0 - a1 + a2
	fp2.squareAssign(t[2])          // s2 = (a0 - a1 + a2)^2
	fp2.mul(t[3], &a[1], &a[2])     // a1a2
	fp2.doubleAssign(t[3])          // s3 = 2a1a2
	fp2.square(t[4], &a[2])         // s4 = a2^2
	fp2.mulByNonResidue(t[5], t[3]) // βs3
	fp2.add(&c[0], t[0], t[5])      // c0 = s0 + βs3
	fp2.mulByNonResidue(t[5], t[4]) // βs4
	fp2.add(&c[1], t[1], t[5])      // c1 = s1 + βs4
	fp2.addAssign(t[1], t[2])
	fp2.addAssign(t[1], t[3])
	fp2.addAssign(t[0], t[4])
	fp2.sub(&c[2], t[1], t[0]) // c2 = s1 + s2 - s0 - s4
}

func (e *fp6) mul01(c, a *fe6, b0, b1 *fe2) {
	fp2, t := e.fp2, e.t
	// v0 = a0b0
	// v1 = a1b1
	// c0 = (b1(a1 + a2) - v1)β + v0
	// c1 = (a0 + a1)(b0 + b1) - v0 - v1
	// c2 = b0(a0 + a2) - v0 + v1

	fp2.mul(t[0], &a[0], b0)        // v0 = b0a0
	fp2.mul(t[1], &a[1], b1)        // v1 = a1b1
	fp2.add(t[2], &a[1], &a[2])     // a1 + a2
	fp2.mulAssign(t[2], b1)         // b1(a1 + a2)
	fp2.subAssign(t[2], t[1])       // b1(a1 + a2) - v1
	fp2.mulByNonResidue(t[2], t[2]) // (b1(a1 + a2) - v1)β
	fp2.add(t[3], &a[0], &a[2])     // a0 + a2
	fp2.mulAssign(t[3], b0)         // b0(a0 + a2)
	fp2.subAssign(t[3], t[0])       // b0(a0 + a2) - v0
	fp2.add(&c[2], t[3], t[1])      // b0(a0 + a2) - v0 + v1
	fp2.add(t[4], b0, b1)           // (b0 + b1)
	fp2.add(t[3], &a[0], &a[1])     // (a0 + a1)
	fp2.mulAssign(t[4], t[3])       // (a0 + a1)(b0 + b1)
	fp2.subAssign(t[4], t[0])       // (a0 + a1)(b0 + b1) - v0
	fp2.sub(&c[1], t[4], t[1])      // (a0 + a1)(b0 + b1) - v0 - v1
	fp2.add(&c[0], t[2], t[0])      //  (b1(a1 + a2) - v1)β + v0
}

func (e *fp6) mul1(c, a *fe6, b1 *fe2) {
	fp2, t := e.fp2, e.t
	// c0 = βa2b1
	// c1 = a0b1
	// c2 = a1b1
	fp2.mul(t[0], &a[2], b1)
	fp2.mul(&c[2], &a[1], b1)
	fp2.mul(&c[1], &a[0], b1)
	fp2.mulByNonResidue(&c[0], t[0])
}

func (e *fp6) mulByNonResidue(c, a *fe6) {
	fp2, t := e.fp2, e.t
	t[0].set(&a[0])
	fp2.mulByNonResidue(&c[0], &a[2])
	c[2].set(&a[1])
	c[1].set(t[0])
}

func (e *fp6) mulByBaseField(c, a *fe6, b *fe2) {
	fp2 := e.fp2
	// c0 = a0b0
	// c1 = a1b1
	// c2 = a2b2
	fp2.mul(&c[0], &a[0], b)
	fp2.mul(&c[1], &a[1], b)
	fp2.mul(&c[2], &a[2], b)
}

func (e *fp6) exp(c, a *fe6, s *big.Int) {
	z := e.one()
	for i := s.BitLen() - 1; i >= 0; i-- {
		e.square(z, z)
		if s.Bit(i) == 1 {
			e.mul(z, z, a)
		}
	}
	c.set(z)
}

func (e *fp6) inverse(c, a *fe6) {
	// Guide to Pairing Based Cryptography
	// Algorithm 5.23

	fp2, t := e.fp2, e.t
	fp2.square(t[0], &a[0])
	fp2.mul(t[1], &a[1], &a[2])
	fp2.mulByNonResidue(t[1], t[1])
	fp2.subAssign(t[0], t[1])       // A = v0 - βv5
	fp2.square(t[1], &a[1])         // v1 = a1^2
	fp2.mul(t[2], &a[0], &a[2])     // v4 = a0a2
	fp2.subAssign(t[1], t[2])       // C = v1 - v4
	fp2.square(t[2], &a[2])         // v2 = a2^2
	fp2.mulByNonResidue(t[2], t[2]) // βv2
	fp2.mul(t[3], &a[0], &a[1])     // v3 = a0a1
	fp2.subAssign(t[2], t[3])       // B = βv2 - v3
	fp2.mul(t[3], &a[2], t[2])      // B * a2
	fp2.mul(t[4], &a[1], t[1])      // C * a1
	fp2.addAssign(t[3], t[4])       // Ca1 + Ba2
	fp2.mulByNonResidue(t[3], t[3]) // β(Ca1 + Ba2)
	fp2.mul(t[4], &a[0], t[0])      // Aa0
	fp2.addAssign(t[3], t[4])       // v6 = Aa0 + β(Ca1 + Ba2)
	fp2.inverse(t[3], t[3])         // F = v6^-1
	fp2.mul(&c[0], t[0], t[3])      // c0 = AF
	fp2.mul(&c[1], t[2], t[3])      // c1 = BF
	fp2.mul(&c[2], t[1], t[3])      // c2 = CF
}

func (e *fp6) frobeniusMap(a *fe6, power int) {
	fp2 := e.fp2
	fp2.frobeniusMap(&a[0], power)
	fp2.frobeniusMap(&a[1], power)
	fp2.frobeniusMap(&a[2], power)
	fp2.mulAssign(&a[1], &frobeniusCoeffs61[power%6])
	fp2.mulAssign(&a[2], &frobeniusCoeffs62[power%6])
}

func (e *fp6) frobeniusMap1(a *fe6) {
	fp2 := e.fp2
	fp2.frobeniusMap1(&a[0])
	fp2.frobeniusMap1(&a[1])
	fp2.frobeniusMap1(&a[2])
	fp2.mulAssign(&a[1], &frobeniusCoeffs61[1])
	fp2.mulAssign(&a[2], &frobeniusCoeffs62[1])
}

func (e *fp6) frobeniusMap2(a *fe6) {
	e.fp2.mulAssign(&a[1], &frobeniusCoeffs61[2])
	e.fp2.mulAssign(&a[2], &frobeniusCoeffs62[2])
}

func (e *fp6) frobeniusMap3(a *fe6) {
	fp2, t := e.fp2, e.t
	fp2.frobeniusMap1(&a[0])
	fp2.frobeniusMap1(&a[1])
	fp2.frobeniusMap1(&a[2])
	neg(&t[0][0], &a[1][1])
	a[1][1].set(&a[1][0])
	a[1][0].set(&t[0][0])
	fp2.neg(&a[2], &a[2])
}
