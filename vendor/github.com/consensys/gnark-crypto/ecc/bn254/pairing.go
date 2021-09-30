// Copyright 2020 ConsenSys AG
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bn254

import (
	"errors"

	"github.com/consensys/gnark-crypto/ecc/bn254/internal/fptower"
)

// GT target group of the pairing
type GT = fptower.E12

type lineEvaluation struct {
	r0 fptower.E2
	r1 fptower.E2
	r2 fptower.E2
}

// Pair calculates the reduced pairing for a set of points
func Pair(P []G1Affine, Q []G2Affine) (GT, error) {
	f, err := MillerLoop(P, Q)
	if err != nil {
		return GT{}, err
	}
	return FinalExponentiation(&f), nil
}

// PairingCheck calculates the reduced pairing for a set of points and returns True if the result is One
func PairingCheck(P []G1Affine, Q []G2Affine) (bool, error) {
	f, err := Pair(P, Q)
	if err != nil {
		return false, err
	}
	var one GT
	one.SetOne()
	return f.Equal(&one), nil
}

// FinalExponentiation computes the final expo x**(p**6-1)(p**2+1)(p**4 - p**2 +1)/r
func FinalExponentiation(z *GT, _z ...*GT) GT {

	var result GT
	result.Set(z)

	for _, e := range _z {
		result.Mul(&result, e)
	}

	// https://eprint.iacr.org/2008/490.pdf
	var mt [4]GT // mt[i] is m^(t^i)

	// easy part
	mt[0].Set(&result)
	var temp GT
	temp.Conjugate(&mt[0])
	mt[0].Inverse(&mt[0])
	temp.Mul(&temp, &mt[0])
	mt[0].FrobeniusSquare(&temp).
		Mul(&mt[0], &temp)

	// hard part
	mt[1].Expt(&mt[0])
	mt[2].Expt(&mt[1])
	mt[3].Expt(&mt[2])

	var y [7]GT

	y[1].InverseUnitary(&mt[0])
	y[4].Set(&mt[1])
	y[5].InverseUnitary(&mt[2])
	y[6].Set(&mt[3])

	mt[0].Frobenius(&mt[0])
	mt[1].Frobenius(&mt[1])
	mt[2].Frobenius(&mt[2])
	mt[3].Frobenius(&mt[3])

	y[0].Set(&mt[0])
	y[3].InverseUnitary(&mt[1])
	y[4].Mul(&y[4], &mt[2]).InverseUnitary(&y[4])
	y[6].Mul(&y[6], &mt[3]).InverseUnitary(&y[6])

	mt[0].Frobenius(&mt[0])
	mt[2].Frobenius(&mt[2])

	y[0].Mul(&y[0], &mt[0])
	y[2].Set(&mt[2])

	mt[0].Frobenius(&mt[0])

	y[0].Mul(&y[0], &mt[0])

	// compute addition chain
	mt[0].CyclotomicSquare(&y[6])
	mt[0].Mul(&mt[0], &y[4])
	mt[0].Mul(&mt[0], &y[5])
	mt[1].Mul(&y[3], &y[5])
	mt[1].Mul(&mt[1], &mt[0])
	mt[0].Mul(&mt[0], &y[2])
	mt[1].CyclotomicSquare(&mt[1])
	mt[1].Mul(&mt[1], &mt[0])
	mt[1].CyclotomicSquare(&mt[1])
	mt[0].Mul(&mt[1], &y[1])
	mt[1].Mul(&mt[1], &y[0])
	mt[0].CyclotomicSquare(&mt[0])
	result.Mul(&mt[0], &mt[1])

	return result
}

// MillerLoop Miller loop
func MillerLoop(P []G1Affine, Q []G2Affine) (GT, error) {
	n := len(P)
	if n == 0 || n != len(Q) {
		return GT{}, errors.New("invalid inputs sizes")
	}

	// filter infinity points
	p := make([]G1Affine, 0, n)
	q := make([]G2Affine, 0, n)

	for k := 0; k < n; k++ {
		if P[k].IsInfinity() || Q[k].IsInfinity() {
			continue
		}
		p = append(p, P[k])
		q = append(q, Q[k])
	}
	n = len(p)

	// projective points for Q
	qProj := make([]g2Proj, n)
	qNeg := make([]G2Affine, n)
	for k := 0; k < n; k++ {
		qProj[k].FromAffine(&q[k])
		qNeg[k].Neg(&q[k])
	}

	var result GT
	result.SetOne()

	var l lineEvaluation

	for i := len(loopCounter) - 2; i >= 0; i-- {
		result.Square(&result)

		for k := 0; k < n; k++ {
			qProj[k].DoubleStep(&l)
			// line evaluation
			l.r0.MulByElement(&l.r0, &p[k].Y)
			l.r1.MulByElement(&l.r1, &p[k].X)
			result.MulBy034(&l.r0, &l.r1, &l.r2)

			if loopCounter[i] == 1 {
				qProj[k].AddMixedStep(&l, &q[k])
				// line evaluation
				l.r0.MulByElement(&l.r0, &p[k].Y)
				l.r1.MulByElement(&l.r1, &p[k].X)
				result.MulBy034(&l.r0, &l.r1, &l.r2)

			} else if loopCounter[i] == -1 {
				qProj[k].AddMixedStep(&l, &qNeg[k])
				// line evaluation
				l.r0.MulByElement(&l.r0, &p[k].Y)
				l.r1.MulByElement(&l.r1, &p[k].X)
				result.MulBy034(&l.r0, &l.r1, &l.r2)
			}
		}
	}

	var Q1, Q2 G2Affine
	// cf https://eprint.iacr.org/2010/354.pdf for instance for optimal Ate Pairing
	for k := 0; k < n; k++ {
		//Q1 = Frob(Q)
		Q1.X.Conjugate(&q[k].X).MulByNonResidue1Power2(&Q1.X)
		Q1.Y.Conjugate(&q[k].Y).MulByNonResidue1Power3(&Q1.Y)

		// Q2 = -Frob2(Q)
		Q2.X.MulByNonResidue2Power2(&q[k].X)
		Q2.Y.MulByNonResidue2Power3(&q[k].Y).Neg(&Q2.Y)

		qProj[k].AddMixedStep(&l, &Q1)
		// line evaluation
		l.r0.MulByElement(&l.r0, &p[k].Y)
		l.r1.MulByElement(&l.r1, &p[k].X)
		result.MulBy034(&l.r0, &l.r1, &l.r2)

		qProj[k].AddMixedStep(&l, &Q2)
		// line evaluation
		l.r0.MulByElement(&l.r0, &p[k].Y)
		l.r1.MulByElement(&l.r1, &p[k].X)
		result.MulBy034(&l.r0, &l.r1, &l.r2)
	}

	return result, nil
}

// DoubleStep doubles a point in Homogenous projective coordinates, and evaluates the line in Miller loop
// https://eprint.iacr.org/2013/722.pdf (Section 4.3)
func (p *g2Proj) DoubleStep(evaluations *lineEvaluation) {

	// get some Element from our pool
	var t0, t1, A, B, C, D, E, EE, F, G, H, I, J, K fptower.E2
	t0.Mul(&p.x, &p.y)
	A.MulByElement(&t0, &twoInv)
	B.Square(&p.y)
	C.Square(&p.z)
	D.Double(&C).
		Add(&D, &C)
	E.Mul(&D, &bTwistCurveCoeff)
	F.Double(&E).
		Add(&F, &E)
	G.Add(&B, &F)
	G.MulByElement(&G, &twoInv)
	H.Add(&p.y, &p.z).
		Square(&H)
	t1.Add(&B, &C)
	H.Sub(&H, &t1)
	I.Sub(&E, &B)
	J.Square(&p.x)
	EE.Square(&E)
	K.Double(&EE).
		Add(&K, &EE)

	// X, Y, Z
	p.x.Sub(&B, &F).
		Mul(&p.x, &A)
	p.y.Square(&G).
		Sub(&p.y, &K)
	p.z.Mul(&B, &H)

	// Line evaluation
	evaluations.r0.Neg(&H)
	evaluations.r1.Double(&J).
		Add(&evaluations.r1, &J)
	evaluations.r2.Set(&I)
}

// AddMixedStep point addition in Mixed Homogenous projective and Affine coordinates
// https://eprint.iacr.org/2013/722.pdf (Section 4.3)
func (p *g2Proj) AddMixedStep(evaluations *lineEvaluation, a *G2Affine) {

	// get some Element from our pool
	var Y2Z1, X2Z1, O, L, C, D, E, F, G, H, t0, t1, t2, J fptower.E2
	Y2Z1.Mul(&a.Y, &p.z)
	O.Sub(&p.y, &Y2Z1)
	X2Z1.Mul(&a.X, &p.z)
	L.Sub(&p.x, &X2Z1)
	C.Square(&O)
	D.Square(&L)
	E.Mul(&L, &D)
	F.Mul(&p.z, &C)
	G.Mul(&p.x, &D)
	t0.Double(&G)
	H.Add(&E, &F).
		Sub(&H, &t0)
	t1.Mul(&p.y, &E)

	// X, Y, Z
	p.x.Mul(&L, &H)
	p.y.Sub(&G, &H).
		Mul(&p.y, &O).
		Sub(&p.y, &t1)
	p.z.Mul(&E, &p.z)

	t2.Mul(&L, &a.Y)
	J.Mul(&a.X, &O).
		Sub(&J, &t2)

	// Line evaluation
	evaluations.r0.Set(&L)
	evaluations.r1.Neg(&O)
	evaluations.r2.Set(&J)
}
