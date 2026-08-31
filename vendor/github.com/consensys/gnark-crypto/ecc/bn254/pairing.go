// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

package bn254

import (
	"errors"

	"github.com/consensys/gnark-crypto/ecc/bn254/fp"
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
// ∏ᵢ e(Pᵢ, Qᵢ).
//
// This function doesn't check that the inputs are in the correct subgroup. See IsInSubGroup.
func Pair(P []G1Affine, Q []G2Affine) (GT, error) {
	f, err := MillerLoop(P, Q)
	if err != nil {
		return GT{}, err
	}
	return FinalExponentiation(&f), nil
}

// PairingCheck calculates the reduced pairing for a set of points and returns True if the result is One
// ∏ᵢ e(Pᵢ, Qᵢ) =? 1
//
// This function doesn't check that the inputs are in the correct subgroup. See IsInSubGroup.
func PairingCheck(P []G1Affine, Q []G2Affine) (bool, error) {
	f, err := Pair(P, Q)
	if err != nil {
		return false, err
	}
	var one GT
	one.SetOne()
	return f.Equal(&one), nil
}

// FinalExponentiation computes the exponentiation (∏ᵢ zᵢ)ᵈ
// where d = (p¹²-1)/r = (p¹²-1)/Φ₁₂(p) ⋅ Φ₁₂(p)/r = (p⁶-1)(p²+1)(p⁴ - p² +1)/r
// we use instead d=s ⋅ (p⁶-1)(p²+1)(p⁴ - p² +1)/r
// where s is the cofactor 2x₀(6x₀²+3x₀+1)
func FinalExponentiation(z *GT, _z ...*GT) GT {

	var result GT
	result.Set(z)

	for _, e := range _z {
		result.Mul(&result, e)
	}

	var t [5]GT

	// Easy part
	// (p⁶-1)(p²+1)
	t[0].Conjugate(&result)
	result.Inverse(&result)
	t[0].Mul(&t[0], &result)
	result.FrobeniusSquare(&t[0]).Mul(&result, &t[0])

	var one GT
	one.SetOne()
	if result.Equal(&one) {
		return result
	}

	// Hard part (up to permutation)
	// 2x₀(6x₀²+3x₀+1)(p⁴-p²+1)/r
	// Duquesne and Ghammam
	// https://eprint.iacr.org/2015/192.pdf
	// Fuentes et al. (alg. 6)
	t[0].Expt(&result).
		Conjugate(&t[0])
	t[0].CyclotomicSquare(&t[0])
	t[1].CyclotomicSquare(&t[0])
	t[1].Mul(&t[0], &t[1])
	t[2].Expt(&t[1])
	t[2].Conjugate(&t[2])
	t[3].Conjugate(&t[1])
	t[1].Mul(&t[2], &t[3])
	t[3].CyclotomicSquare(&t[2])
	t[4].Expt(&t[3])
	t[4].Mul(&t[1], &t[4])
	t[3].Mul(&t[0], &t[4])
	t[0].Mul(&t[2], &t[4])
	t[0].Mul(&result, &t[0])
	t[2].Frobenius(&t[3])
	t[0].Mul(&t[2], &t[0])
	t[2].FrobeniusSquare(&t[4])
	t[0].Mul(&t[2], &t[0])
	t[2].Conjugate(&result)
	t[2].Mul(&t[2], &t[3])
	t[2].FrobeniusCube(&t[2])
	t[0].Mul(&t[2], &t[0])

	return t[0]
}

// MillerLoop computes the multi-Miller loop
// ∏ᵢ MillerLoop(Pᵢ, Qᵢ) =
// ∏ᵢ { fᵢ_{6x₀+2,Qᵢ}(Pᵢ) · ℓᵢ_{[6x₀+2]Qᵢ,π(Qᵢ)}(Pᵢ) · ℓᵢ_{[6x₀+2]Qᵢ+π(Qᵢ),-π²(Qᵢ)}(Pᵢ) }
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
	var l2, l1 lineEvaluation
	var prodLines [5]E2

	// Compute ∏ᵢ { fᵢ_{6x₀+2,Q}(P) }
	if n >= 1 {
		// i = 64, separately to avoid an E12 Square
		// (Square(res) = 1² = 1)
		// LoopCounter[64] = 0
		// k = 0, separately to avoid MulBy034 (res × ℓ)
		// (assign line to res)

		// qProj[0] ← 2qProj[0] and l1 the tangent ℓ passing 2qProj[0]
		qProj[0].doubleStep(&l1)
		// line evaluation at P[0] (assign)
		result.C0.B0.MulByElement(&l1.r0, &p[0].Y)
		result.C1.B0.MulByElement(&l1.r1, &p[0].X)
		result.C1.B1.Set(&l1.r2)
	}

	if n >= 2 {
		// k = 1, separately to avoid MulBy034 (res × ℓ)
		// (res is also a line at this point, so we use Mul034By034 ℓ × ℓ)

		// qProj[1] ← 2qProj[1] and l1 the tangent ℓ passing 2qProj[1]
		qProj[1].doubleStep(&l1)
		// line evaluation at P[1]
		l1.r0.MulByElement(&l1.r0, &p[1].Y)
		l1.r1.MulByElement(&l1.r1, &p[1].X)
		// ℓ × res
		prodLines = fptower.Mul034By034(&l1.r0, &l1.r1, &l1.r2, &result.C0.B0, &result.C1.B0, &result.C1.B1)
		result.C0.B0 = prodLines[0]
		result.C0.B1 = prodLines[1]
		result.C0.B2 = prodLines[2]
		result.C1.B0 = prodLines[3]
		result.C1.B1 = prodLines[4]
	}

	// k >= 2
	for k := 2; k < n; k++ {
		// qProj[k] ← 2qProj[k] and l1 the tangent ℓ passing 2qProj[k]
		qProj[k].doubleStep(&l1)
		// line evaluation at P[k]
		l1.r0.MulByElement(&l1.r0, &p[k].Y)
		l1.r1.MulByElement(&l1.r1, &p[k].X)
		// ℓ × res
		result.MulBy034(&l1.r0, &l1.r1, &l1.r2)
	}

	// i = 63, separately to avoid a doubleStep (LoopCounter[63]=-1)
	// (at this point qProj = 2Q, so 2qProj-Q=3Q is equivalent to qProj+Q=3Q
	// this means doubleStep followed by an addMixedStep is equivalent to an
	// addMixedStep here)

	result.Square(&result)
	for k := 0; k < n; k++ {
		// l2 the line passing qProj[k] and -Q
		// (avoids a point addition: qProj[k]-Q)
		qProj[k].lineCompute(&l2, &qNeg[k])
		// line evaluation at P[k]
		l2.r0.MulByElement(&l2.r0, &p[k].Y)
		l2.r1.MulByElement(&l2.r1, &p[k].X)
		// qProj[k] ← qProj[k]+Q[k] and
		// l1 the line ℓ passing qProj[k] and Q[k]
		qProj[k].addMixedStep(&l1, &q[k])
		// line evaluation at P[k]
		l1.r0.MulByElement(&l1.r0, &p[k].Y)
		l1.r1.MulByElement(&l1.r1, &p[k].X)
		// ℓ × ℓ
		prodLines = fptower.Mul034By034(&l1.r0, &l1.r1, &l1.r2, &l2.r0, &l2.r1, &l2.r2)
		// (ℓ × ℓ) × res
		result.MulBy01234(&prodLines)
	}

	// i <= 62
	for i := len(LoopCounter) - 4; i >= 0; i-- {
		// mutualize the square among n Miller loops
		// (∏ᵢfᵢ)²
		result.Square(&result)

		for k := 0; k < n; k++ {
			// qProj[k] ← 2qProj[k] and l1 the tangent ℓ passing 2qProj[k]
			qProj[k].doubleStep(&l1)
			// line evaluation at P[k]
			l1.r0.MulByElement(&l1.r0, &p[k].Y)
			l1.r1.MulByElement(&l1.r1, &p[k].X)

			switch LoopCounter[i] {
			case 1:
				// qProj[k] ← qProj[k]+Q[k] and
				// l2 the line ℓ passing qProj[k] and Q[k]
				qProj[k].addMixedStep(&l2, &q[k])
				// line evaluation at P[k]
				l2.r0.MulByElement(&l2.r0, &p[k].Y)
				l2.r1.MulByElement(&l2.r1, &p[k].X)
				// ℓ × ℓ
				prodLines = fptower.Mul034By034(&l1.r0, &l1.r1, &l1.r2, &l2.r0, &l2.r1, &l2.r2)
				// (ℓ × ℓ) × res
				result.MulBy01234(&prodLines)
			case -1:
				// qProj[k] ← qProj[k]-Q[k] and
				// l2 the line ℓ passing qProj[k] and -Q[k]
				qProj[k].addMixedStep(&l2, &qNeg[k])
				// line evaluation at P[k]
				l2.r0.MulByElement(&l2.r0, &p[k].Y)
				l2.r1.MulByElement(&l2.r1, &p[k].X)
				// ℓ × ℓ
				prodLines = fptower.Mul034By034(&l1.r0, &l1.r1, &l1.r2, &l2.r0, &l2.r1, &l2.r2)
				// (ℓ × ℓ) × res
				result.MulBy01234(&prodLines)
			default:
				// ℓ × res
				result.MulBy034(&l1.r0, &l1.r1, &l1.r2)
			}
		}
	}

	// Compute  ∏ᵢ { ℓᵢ_{[6x₀+2]Q,π(Q)}(P) · ℓᵢ_{[6x₀+2]Q+π(Q),-π²(Q)}(P) }
	var Q1, Q2 G2Affine
	for k := 0; k < n; k++ {
		//Q1 = π(Q)
		Q1.X.Conjugate(&q[k].X).MulByNonResidue1Power2(&Q1.X)
		Q1.Y.Conjugate(&q[k].Y).MulByNonResidue1Power3(&Q1.Y)

		// Q2 = -π²(Q)
		Q2.X.MulByNonResidue2Power2(&q[k].X)
		Q2.Y.MulByNonResidue2Power3(&q[k].Y).Neg(&Q2.Y)

		// qProj[k] ← qProj[k]+π(Q) and
		// l1 the line passing qProj[k] and π(Q)
		qProj[k].addMixedStep(&l2, &Q1)
		// line evaluation at P[k]
		l2.r0.MulByElement(&l2.r0, &p[k].Y)
		l2.r1.MulByElement(&l2.r1, &p[k].X)

		// l2 the line passing qProj[k] and -π²(Q)
		// (avoids a point addition: qProj[k]-π²(Q))
		qProj[k].lineCompute(&l1, &Q2)
		// line evaluation at P[k]
		l1.r0.MulByElement(&l1.r0, &p[k].Y)
		l1.r1.MulByElement(&l1.r1, &p[k].X)

		// ℓ × ℓ
		prodLines = fptower.Mul034By034(&l1.r0, &l1.r1, &l1.r2, &l2.r0, &l2.r1, &l2.r2)
		// (ℓ × ℓ) × res
		result.MulBy01234(&prodLines)
	}

	return result, nil
}

// doubleStep doubles a point in Homogenous projective coordinates, and evaluates the line in Miller loop
// https://eprint.iacr.org/2013/722.pdf (Section 4.3)
func (p *g2Proj) doubleStep(evaluations *lineEvaluation) {

	// get some Element from our pool
	var t1, A, B, C, D, E, EE, F, G, H, I, J, K fptower.E2
	A.Mul(&p.x, &p.y)
	A.Halve()
	B.Square(&p.y)
	C.Square(&p.z)
	D.Double(&C).
		Add(&D, &C)
	E.MulBybTwistCurveCoeff(&D)
	F.Double(&E).
		Add(&F, &E)
	G.Add(&B, &F)
	G.Halve()
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

// addMixedStep point addition in Mixed Homogenous projective and Affine coordinates
// https://eprint.iacr.org/2013/722.pdf (Section 4.3)
func (p *g2Proj) addMixedStep(evaluations *lineEvaluation, a *G2Affine) {

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

// lineCompute computes the line through p in Homogenous projective coordinates
// and a in affine coordinates. It does not compute the resulting point p+a.
func (p *g2Proj) lineCompute(evaluations *lineEvaluation, a *G2Affine) {

	// get some Element from our pool
	var Y2Z1, X2Z1, O, L, t2, J fptower.E2
	Y2Z1.Mul(&a.Y, &p.z)
	O.Sub(&p.y, &Y2Z1)
	X2Z1.Mul(&a.X, &p.z)
	L.Sub(&p.x, &X2Z1)
	t2.Mul(&L, &a.Y)
	J.Mul(&a.X, &O).
		Sub(&J, &t2)

	// Line evaluation
	evaluations.r0.Set(&L)
	evaluations.r1.Neg(&O)
	evaluations.r2.Set(&J)
}

// ----------------------
// Fixed-argument pairing
// ----------------------

type LineEvaluationAff struct {
	R0 fptower.E2
	R1 fptower.E2
}

// PairFixedQ calculates the reduced pairing for a set of points
// ∏ᵢ e(Pᵢ, Qᵢ) where Q are fixed points in G2.
//
// This function doesn't check that the inputs are in the correct subgroup. See IsInSubGroup.
func PairFixedQ(P []G1Affine, lines [][2][len(LoopCounter)]LineEvaluationAff) (GT, error) {
	f, err := MillerLoopFixedQ(P, lines)
	if err != nil {
		return GT{}, err
	}
	return FinalExponentiation(&f), nil
}

// PairingCheckFixedQ calculates the reduced pairing for a set of points and returns True if the result is One
// ∏ᵢ e(Pᵢ, Qᵢ) =? 1 where Q are fixed points in G2.
//
// This function doesn't check that the inputs are in the correct subgroup. See IsInSubGroup.
func PairingCheckFixedQ(P []G1Affine, lines [][2][len(LoopCounter)]LineEvaluationAff) (bool, error) {
	f, err := PairFixedQ(P, lines)
	if err != nil {
		return false, err
	}
	var one GT
	one.SetOne()
	return f.Equal(&one), nil
}

// manyDoubleSteps performs k consecutive doublings on p and returns the line evaluations.
// It uses a recurrence to compute 2^k*P with a single batch inversion.
//
// The recurrence maintains A[i], B[i], C[i] where:
//   - A[i] relates to x-coordinate with cumulative scaling
//   - B[i] = 3*A[i]² (numerator of slope)
//   - C[i] = -y[i] with cumulative scaling
//   - D[i] = -2*C[i] = 2*y[i] (denominator of slope for step i)
//
// After k doublings with cumulative product T[k-1] = D[0]*D[1]*...*D[k-1]:
//   - x[k] = A[k] / T[k-1]²
//   - y[k] = -C[k] / T[k-1]³
func (p *G2Affine) manyDoubleSteps(k int, evaluations []LineEvaluationAff) {
	if k == 0 {
		return
	}

	// Step 1: Compute A[i], B[i], C[i] using the recurrence
	A := make([]fptower.E2, k+1)
	B := make([]fptower.E2, k+1)
	C := make([]fptower.E2, k+1)

	var tmp fptower.E2
	A[0].Set(&p.X)
	C[0].Neg(&p.Y)
	tmp.Square(&p.X)
	B[0].Double(&tmp).Add(&B[0], &tmp) // B[0] = 3x²

	for i := 1; i <= k; i++ {
		var Csq, ACs, eightACs fptower.E2
		Csq.Square(&C[i-1])
		ACs.Mul(&A[i-1], &Csq)
		eightACs.Double(&ACs).Double(&eightACs).Double(&eightACs)
		A[i].Square(&B[i-1]).Sub(&A[i], &eightACs)

		tmp.Square(&A[i])
		B[i].Double(&tmp).Add(&B[i], &tmp)

		var C4, fourACs, diff fptower.E2
		C4.Square(&Csq)
		fourACs.Double(&ACs).Double(&fourACs)
		diff.Sub(&A[i], &fourACs)
		C[i].Double(&C4).Double(&C[i]).Double(&C[i]) // 8*C[i-1]⁴
		tmp.Mul(&B[i-1], &diff)
		C[i].Add(&C[i], &tmp) // C[i] = 8*C[i-1]⁴ + B[i-1]*(A[i] - 4*A[i-1]*C[i-1]²)
	}

	// Step 2: Compute D[i] = -2*C[i] = 2*y[i] for i = 0..k-1
	D := make([]fptower.E2, k)
	for i := range k {
		D[i].Double(&C[i]).Neg(&D[i])
	}

	// Step 3: Compute T[i] = D[0]*D[1]*...*D[i] for i = 0..k-1
	T := make([]fptower.E2, k)
	T[0].Set(&D[0])
	for i := 1; i < k; i++ {
		T[i].Mul(&T[i-1], &D[i])
	}

	// Step 4: Batch invert T
	invT := fptower.BatchInvertE2(T)

	// Step 5: Compute line evaluations
	// For step i, doubling (x[i], y[i]) to (x[i+1], y[i+1]):
	//   λ[i] = B[i] / T[i]
	//   R0 = λ[i]
	//   R1 = λ[i] * x[i] - y[i]
	//
	// For i = 0: x[0] = A[0], y[0] = -C[0]
	// For i > 0: x[i] = A[i] / T[i-1]², y[i] = -C[i] / T[i-1]³

	// Step 0: special case since scaling is 1
	evaluations[0].R0.Mul(&B[0], &invT[0])
	evaluations[0].R1.Mul(&B[0], &A[0]).Mul(&evaluations[0].R1, &invT[0]).Add(&evaluations[0].R1, &C[0])

	// Steps 1 to k-1
	var invT2, invT3 fptower.E2
	for i := 1; i < k; i++ {
		// R0 = B[i] / T[i]
		evaluations[i].R0.Mul(&B[i], &invT[i])

		// R1 = λ[i] * x[i] - y[i]
		//    = B[i]/T[i] * A[i]/T[i-1]² + C[i]/T[i-1]³
		//    = B[i]*A[i]/(T[i]*T[i-1]²) + C[i]/T[i-1]³
		invT2.Square(&invT[i-1])
		invT3.Mul(&invT2, &invT[i-1])

		var term1, term2 fptower.E2
		term1.Mul(&B[i], &A[i]).Mul(&term1, &invT[i]).Mul(&term1, &invT2)
		term2.Mul(&C[i], &invT3)
		evaluations[i].R1.Add(&term1, &term2)
	}

	// Step 6: Final point coordinates
	// x[k] = A[k] / T[k-1]²
	// y[k] = -C[k] / T[k-1]³
	invT2.Square(&invT[k-1])
	invT3.Mul(&invT2, &invT[k-1])
	p.X.Mul(&A[k], &invT2)
	p.Y.Mul(&C[k], &invT3).Neg(&p.Y)
}

// manyDoublesAndAdd performs k consecutive doublings followed by a doubleAndAdd operation,
// using a single batch inversion for all k+2 line evaluations.
// This combines manyDoubleSteps(k) + doubleAndAddStep into one batch inversion (2 inv → 1 inv).
func (p *G2Affine) manyDoublesAndAdd(k int, doubEvals []LineEvaluationAff, addEval1, addEval2 *LineEvaluationAff, a *G2Affine) {
	if k == 0 {
		p.doubleAndAddStep(addEval1, addEval2, a)
		return
	}

	// Step 1: Compute A[i], B[i], C[i] using the recurrence (same as manyDoubleSteps)
	A := make([]fptower.E2, k+1)
	B := make([]fptower.E2, k+1)
	C := make([]fptower.E2, k+1)

	var tmp fptower.E2
	A[0].Set(&p.X)
	C[0].Neg(&p.Y)
	tmp.Square(&p.X)
	B[0].Double(&tmp).Add(&B[0], &tmp) // B[0] = 3x²

	for i := 1; i <= k; i++ {
		var Csq, ACs, eightACs fptower.E2
		Csq.Square(&C[i-1])
		ACs.Mul(&A[i-1], &Csq)
		eightACs.Double(&ACs).Double(&eightACs).Double(&eightACs)
		A[i].Square(&B[i-1]).Sub(&A[i], &eightACs)

		tmp.Square(&A[i])
		B[i].Double(&tmp).Add(&B[i], &tmp)

		var C4, fourACs, diff fptower.E2
		C4.Square(&Csq)
		fourACs.Double(&ACs).Double(&fourACs)
		diff.Sub(&A[i], &fourACs)
		C[i].Double(&C4).Double(&C[i]).Double(&C[i])
		tmp.Mul(&B[i-1], &diff)
		C[i].Add(&C[i], &tmp)
	}

	// Step 2: Compute D[i] = -2*C[i] for i = 0..k-1
	D := make([]fptower.E2, k)
	for i := range k {
		D[i].Double(&C[i]).Neg(&D[i])
	}

	// Step 3: Compute partial products T[i] = D[0]*D[1]*...*D[i]
	T := make([]fptower.E2, k+1)
	T[0].Set(&D[0])
	for i := 1; i < k; i++ {
		T[i].Mul(&T[i-1], &D[i])
	}

	// Step 4: Compute ELM formula numerators using scaled coordinates
	var S2, S3, A_num, B_num, U_num fptower.E2
	S := &T[k-1]
	S2.Square(S)
	S3.Mul(&S2, S)

	A_num.Mul(&a.X, &S2)
	A_num.Sub(&A[k], &A_num)

	B_num.Neg(&C[k])
	tmp.Mul(&a.Y, &S3)
	B_num.Sub(&B_num, &tmp)

	var A_num2, B_num2, coeff fptower.E2
	A_num2.Square(&A_num)
	B_num2.Square(&B_num)
	coeff.Double(&A[k])
	tmp.Mul(&a.X, &S2)
	coeff.Add(&coeff, &tmp)
	U_num.Mul(&coeff, &A_num2)
	U_num.Sub(&B_num2, &U_num)

	// Step 5: Extend T for ELM batch inversion
	T[k].Mul(S, &A_num)
	T[k].Mul(&T[k], &U_num)

	// Step 6: Batch invert T[0..k]
	invT := fptower.BatchInvertE2(T)

	// Step 7: Compute doubling line evaluations
	doubEvals[0].R0.Mul(&B[0], &invT[0])
	doubEvals[0].R1.Mul(&B[0], &A[0]).Mul(&doubEvals[0].R1, &invT[0]).Add(&doubEvals[0].R1, &C[0])

	var invT2, invT3 fptower.E2
	for i := 1; i < k; i++ {
		doubEvals[i].R0.Mul(&B[i], &invT[i])

		invT2.Square(&invT[i-1])
		invT3.Mul(&invT2, &invT[i-1])

		var term1, term2 fptower.E2
		term1.Mul(&B[i], &A[i]).Mul(&term1, &invT[i]).Mul(&term1, &invT2)
		term2.Mul(&C[i], &invT3)
		doubEvals[i].R1.Add(&term1, &term2)
	}

	// Step 8: Compute point coordinates after k doublings
	invT2.Square(&invT[k-1])
	invT3.Mul(&invT2, &invT[k-1])
	var x_P, y_P fptower.E2
	x_P.Mul(&A[k], &invT2)
	y_P.Mul(&C[k], &invT3).Neg(&y_P)

	// Step 9: Compute ELM slopes
	var invSA, invSU fptower.E2
	invSA.Mul(&U_num, &invT[k])
	invSU.Mul(&A_num, &invT[k])

	var l1 fptower.E2
	l1.Mul(&B_num, &invSA)

	var l2 fptower.E2
	l2.Double(&C[k])
	l2.Neg(&l2)
	l2.Mul(&l2, &A_num2)
	l2.Mul(&l2, &invSU)
	l2.Add(&l2, &l1)
	l2.Neg(&l2)

	// Step 10: Compute ELM line evaluations
	addEval1.R0.Set(&l1)
	addEval1.R1.Mul(&l1, &x_P)
	addEval1.R1.Sub(&addEval1.R1, &y_P)

	addEval2.R0.Set(&l2)
	addEval2.R1.Mul(&l2, &x_P)
	addEval2.R1.Sub(&addEval2.R1, &y_P)

	// Step 11: Compute final point 2P + Q
	var xPQ fptower.E2
	xPQ.Square(&l1)
	xPQ.Sub(&xPQ, &x_P)
	xPQ.Sub(&xPQ, &a.X)

	var x4, y4 fptower.E2
	x4.Square(&l2)
	x4.Sub(&x4, &x_P)
	x4.Sub(&x4, &xPQ)

	y4.Sub(&x_P, &x4)
	y4.Mul(&l2, &y4)
	y4.Sub(&y4, &y_P)

	p.X.Set(&x4)
	p.Y.Set(&y4)
}

// PrecomputeLines precomputes the lines for the fixed-argument Miller loop
func PrecomputeLines(Q G2Affine) (PrecomputedLines [2][len(LoopCounter)]LineEvaluationAff) {
	var accQ, negQ G2Affine
	accQ.Set(&Q)
	negQ.Neg(&Q)

	// LoopCounter non-zero values: 3(1), 5(1), 7(-1), 10(-1), 14(1), 17(-1), 19(-1), 23(1),
	// 25(-1), 30(-1), 33(1), 35(-1), 38(1), 44(-1), 47(-1), 49(1), 51(-1), 55(-1), 57(-1),
	// 61(1), 63(-1), 65(1)
	// Runs of ≥3 consecutive zeros: 60→58(3), 54→52(3), 43→39(5), 29→26(4), 22→20(3), 13→11(3), 2→0(3)

	// i=64: LoopCounter[64]=0
	accQ.doubleStep(&PrecomputedLines[0][64])

	// i=63: LoopCounter[63]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][63], &PrecomputedLines[1][63], &negQ)

	// i=62: LoopCounter[62]=0
	accQ.doubleStep(&PrecomputedLines[0][62])

	// i=61: LoopCounter[61]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][61], &PrecomputedLines[1][61], &Q)

	// i=60→57: 3 consecutive zeros followed by add at i=57 (-1)
	// Uses manyDoublesAndAdd to combine into 1 inversion
	{
		var doubEvals [3]LineEvaluationAff
		accQ.manyDoublesAndAdd(3, doubEvals[:], &PrecomputedLines[0][57], &PrecomputedLines[1][57], &negQ)
		PrecomputedLines[0][60] = doubEvals[0]
		PrecomputedLines[0][59] = doubEvals[1]
		PrecomputedLines[0][58] = doubEvals[2]
	}

	// i=56: LoopCounter[56]=0
	accQ.doubleStep(&PrecomputedLines[0][56])

	// i=55: LoopCounter[55]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][55], &PrecomputedLines[1][55], &negQ)

	// i=54→51: 3 consecutive zeros followed by add at i=51 (-1)
	{
		var doubEvals [3]LineEvaluationAff
		accQ.manyDoublesAndAdd(3, doubEvals[:], &PrecomputedLines[0][51], &PrecomputedLines[1][51], &negQ)
		PrecomputedLines[0][54] = doubEvals[0]
		PrecomputedLines[0][53] = doubEvals[1]
		PrecomputedLines[0][52] = doubEvals[2]
	}

	// i=50: LoopCounter[50]=0
	accQ.doubleStep(&PrecomputedLines[0][50])

	// i=49: LoopCounter[49]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][49], &PrecomputedLines[1][49], &Q)

	// i=48: LoopCounter[48]=0
	accQ.doubleStep(&PrecomputedLines[0][48])

	// i=47: LoopCounter[47]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][47], &PrecomputedLines[1][47], &negQ)

	// i=46: LoopCounter[46]=0
	accQ.doubleStep(&PrecomputedLines[0][46])

	// i=45: LoopCounter[45]=0
	accQ.doubleStep(&PrecomputedLines[0][45])

	// i=44: LoopCounter[44]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][44], &PrecomputedLines[1][44], &negQ)

	// i=43→38: 5 consecutive zeros followed by add at i=38 (1)
	{
		var doubEvals [5]LineEvaluationAff
		accQ.manyDoublesAndAdd(5, doubEvals[:], &PrecomputedLines[0][38], &PrecomputedLines[1][38], &Q)
		PrecomputedLines[0][43] = doubEvals[0]
		PrecomputedLines[0][42] = doubEvals[1]
		PrecomputedLines[0][41] = doubEvals[2]
		PrecomputedLines[0][40] = doubEvals[3]
		PrecomputedLines[0][39] = doubEvals[4]
	}

	// i=37: LoopCounter[37]=0
	accQ.doubleStep(&PrecomputedLines[0][37])

	// i=36: LoopCounter[36]=0
	accQ.doubleStep(&PrecomputedLines[0][36])

	// i=35: LoopCounter[35]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][35], &PrecomputedLines[1][35], &negQ)

	// i=34: LoopCounter[34]=0
	accQ.doubleStep(&PrecomputedLines[0][34])

	// i=33: LoopCounter[33]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][33], &PrecomputedLines[1][33], &Q)

	// i=32: LoopCounter[32]=0
	accQ.doubleStep(&PrecomputedLines[0][32])

	// i=31: LoopCounter[31]=0
	accQ.doubleStep(&PrecomputedLines[0][31])

	// i=30: LoopCounter[30]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][30], &PrecomputedLines[1][30], &negQ)

	// i=29→25: 4 consecutive zeros followed by add at i=25 (-1)
	{
		var doubEvals [4]LineEvaluationAff
		accQ.manyDoublesAndAdd(4, doubEvals[:], &PrecomputedLines[0][25], &PrecomputedLines[1][25], &negQ)
		PrecomputedLines[0][29] = doubEvals[0]
		PrecomputedLines[0][28] = doubEvals[1]
		PrecomputedLines[0][27] = doubEvals[2]
		PrecomputedLines[0][26] = doubEvals[3]
	}

	// i=24: LoopCounter[24]=0
	accQ.doubleStep(&PrecomputedLines[0][24])

	// i=23: LoopCounter[23]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][23], &PrecomputedLines[1][23], &Q)

	// i=22→19: 3 consecutive zeros followed by add at i=19 (-1)
	{
		var doubEvals [3]LineEvaluationAff
		accQ.manyDoublesAndAdd(3, doubEvals[:], &PrecomputedLines[0][19], &PrecomputedLines[1][19], &negQ)
		PrecomputedLines[0][22] = doubEvals[0]
		PrecomputedLines[0][21] = doubEvals[1]
		PrecomputedLines[0][20] = doubEvals[2]
	}

	// i=18: LoopCounter[18]=0
	accQ.doubleStep(&PrecomputedLines[0][18])

	// i=17: LoopCounter[17]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][17], &PrecomputedLines[1][17], &negQ)

	// i=16: LoopCounter[16]=0
	accQ.doubleStep(&PrecomputedLines[0][16])

	// i=15: LoopCounter[15]=0
	accQ.doubleStep(&PrecomputedLines[0][15])

	// i=14: LoopCounter[14]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][14], &PrecomputedLines[1][14], &Q)

	// i=13→10: 3 consecutive zeros followed by add at i=10 (-1)
	{
		var doubEvals [3]LineEvaluationAff
		accQ.manyDoublesAndAdd(3, doubEvals[:], &PrecomputedLines[0][10], &PrecomputedLines[1][10], &negQ)
		PrecomputedLines[0][13] = doubEvals[0]
		PrecomputedLines[0][12] = doubEvals[1]
		PrecomputedLines[0][11] = doubEvals[2]
	}

	// i=9: LoopCounter[9]=0
	accQ.doubleStep(&PrecomputedLines[0][9])

	// i=8: LoopCounter[8]=0
	accQ.doubleStep(&PrecomputedLines[0][8])

	// i=7: LoopCounter[7]=-1
	accQ.doubleAndAddStep(&PrecomputedLines[0][7], &PrecomputedLines[1][7], &negQ)

	// i=6: LoopCounter[6]=0
	accQ.doubleStep(&PrecomputedLines[0][6])

	// i=5: LoopCounter[5]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][5], &PrecomputedLines[1][5], &Q)

	// i=4: LoopCounter[4]=0
	accQ.doubleStep(&PrecomputedLines[0][4])

	// i=3: LoopCounter[3]=1
	accQ.doubleAndAddStep(&PrecomputedLines[0][3], &PrecomputedLines[1][3], &Q)

	// i=2→0: 3 consecutive zeros
	{
		var evals [3]LineEvaluationAff
		accQ.manyDoubleSteps(3, evals[:])
		PrecomputedLines[0][2] = evals[0]
		PrecomputedLines[0][1] = evals[1]
		PrecomputedLines[0][0] = evals[2]
	}

	// Frobenius computations for final lines
	var psiQ, phiQ G2Affine
	phiQ.X.Conjugate(&Q.X).MulByNonResidue1Power2(&phiQ.X)
	phiQ.Y.Conjugate(&Q.Y).MulByNonResidue1Power3(&phiQ.Y)
	psiQ.X.MulByNonResidue2Power2(&Q.X)
	psiQ.Y.MulByNonResidue2Power3(&Q.Y).Neg(&psiQ.Y)

	accQ.addStep(&PrecomputedLines[1][65], &phiQ)
	accQ.addStep(&PrecomputedLines[0][65], &psiQ)

	return PrecomputedLines
}

// MillerLoopFixedQ computes the multi-Miller loop as in MillerLoop
// but Qᵢ are fixed points in G2 known in advance.
func MillerLoopFixedQ(P []G1Affine, lines [][2][len(LoopCounter)]LineEvaluationAff) (GT, error) {
	n := len(P)
	if n == 0 || n != len(lines) {
		return GT{}, errors.New("invalid inputs sizes")
	}

	// no need to filter infinity points:
	// 		1. if Pᵢ=(0,0) then -x/y=1/y=0 by gnark-crypto convention and so
	// 		lines R0 and R1 are 0. At the end it happens that result will stay
	// 		1 through the Miller loop because MulBy34(1,0,0)==1
	// 		Mul34By34(1,0,0,1,0,0)==1 and MulBy01234(1,0,0,0,0)==1.
	//
	// 		2. if Qᵢ=(0,0) then PrecomputeLines(Qᵢ) will return lines R0 and R1
	// 		that are 0 because of gnark-convention (*/0==0) in doubleStep and
	// 		addStep. Similarly to Pᵢ=(0,0) it happens that result stays 1
	// 		throughout the MillerLoop.

	// precomputations
	yInv := make([]fp.Element, n)
	xNegOverY := make([]fp.Element, n)
	for k := range n {
		yInv[k].Set(&P[k].Y)
	}
	yInv = fp.BatchInvert(yInv)
	for k := range n {
		xNegOverY[k].Mul(&P[k].X, &yInv[k]).
			Neg(&xNegOverY[k])
	}

	var result GT
	result.SetOne()
	var prodLines [5]E2

	// Compute ∏ᵢ { fᵢ_{6x₀+2,Q}(P) }
	if n >= 1 {
		// i = 64, separately to avoid an E12 Square
		// (Square(res) = 1² = 1)
		// LoopCounter[64] = 0
		// k = 0, separately to avoid MulBy34 (res × ℓ)
		// (assign line to res)

		// line evaluation at P[0] (assign)
		result.C1.B0.MulByElement(&lines[0][0][64].R0, &xNegOverY[0])
		result.C1.B1.MulByElement(&lines[0][0][64].R1, &yInv[0])
		// the coefficient which MulBy34 sets to 1 happens to be already 1 (result = 1)
	}

	if n >= 2 {
		// k = 1, separately to avoid MulBy34 (res × ℓ)
		// (res is also a line at this point, so we use Mul34By34 ℓ × ℓ)
		// line evaluation at P[1]
		lines[1][0][64].R0.MulByElement(&lines[1][0][64].R0, &xNegOverY[1])
		lines[1][0][64].R1.MulByElement(&lines[1][0][64].R1, &yInv[1])
		// ℓ × res
		prodLines = fptower.Mul34By34(&lines[1][0][64].R0, &lines[1][0][64].R1, &result.C1.B0, &result.C1.B1)
		result.C0.B0 = prodLines[0]
		result.C0.B1 = prodLines[1]
		result.C0.B2 = prodLines[2]
		result.C1.B0 = prodLines[3]
		result.C1.B1 = prodLines[4]
	}

	// k >= 2
	for k := 2; k < n; k++ {
		// line evaluation at P[k]
		lines[k][0][64].R0.MulByElement(&lines[k][0][64].R0, &xNegOverY[k])
		lines[k][0][64].R1.MulByElement(&lines[k][0][64].R1, &yInv[k])
		// ℓ × res
		result.MulBy34(
			&lines[k][0][64].R0,
			&lines[k][0][64].R1,
		)
	}

	for i := len(LoopCounter) - 3; i >= 0; i-- {
		// mutualize the square among n Miller loops
		// (∏ᵢfᵢ)²
		result.Square(&result)

		for k := range n {
			// line evaluation at P[k]
			lines[k][0][i].R0.
				MulByElement(
					&lines[k][0][i].R0,
					&xNegOverY[k],
				)
			lines[k][0][i].R1.
				MulByElement(
					&lines[k][0][i].R1,
					&yInv[k],
				)

			if LoopCounter[i] == 0 {
				// ℓ × res
				result.MulBy34(
					&lines[k][0][i].R0,
					&lines[k][0][i].R1,
				)
			} else {
				// line evaluation at P[k]
				lines[k][1][i].R0.
					MulByElement(
						&lines[k][1][i].R0,
						&xNegOverY[k],
					)
				lines[k][1][i].R1.
					MulByElement(
						&lines[k][1][i].R1,
						&yInv[k],
					)
				// ℓ × ℓ
				prodLines = fptower.Mul34By34(
					&lines[k][0][i].R0, &lines[k][0][i].R1,
					&lines[k][1][i].R0, &lines[k][1][i].R1,
				)
				// (ℓ × ℓ) × res
				result.MulBy01234(&prodLines)
			}
		}
	}

	// Compute  ∏ᵢ { ℓᵢ_{[6x₀+2]Q,π(Q)}(P) · ℓᵢ_{[6x₀+2]Q+π(Q),-π²(Q)}(P) }
	for k := range n {
		// line evaluation at P[k]
		lines[k][1][65].R0.
			MulByElement(
				&lines[k][1][65].R0,
				&xNegOverY[k],
			)
		lines[k][1][65].R1.
			MulByElement(
				&lines[k][1][65].R1,
				&yInv[k],
			)
		// line evaluation at P[k]
		lines[k][0][65].R0.
			MulByElement(
				&lines[k][0][65].R0,
				&xNegOverY[k],
			)
		lines[k][0][65].R1.
			MulByElement(
				&lines[k][0][65].R1,
				&yInv[k],
			)
		// ℓ × ℓ
		prodLines = fptower.Mul34By34(
			&lines[k][1][65].R0, &lines[k][1][65].R1,
			&lines[k][0][65].R0, &lines[k][0][65].R1,
		)
		// (ℓ × ℓ) × res
		result.MulBy01234(&prodLines)
	}

	return result, nil
}

func (p *G2Affine) doubleStep(evaluations *LineEvaluationAff) {

	var n, d, λ, xr, yr fptower.E2
	// λ = 3x²/2y
	n.Square(&p.X)
	λ.Double(&n).
		Add(&λ, &n)
	d.Double(&p.Y)
	λ.Div(&λ, &d)

	// xr = λ²-2x
	xr.Square(&λ).
		Sub(&xr, &p.X).
		Sub(&xr, &p.X)

	// yr = λ(x-xr)-y
	yr.Sub(&p.X, &xr).
		Mul(&yr, &λ).
		Sub(&yr, &p.Y)

	evaluations.R0.Set(&λ)
	evaluations.R1.Mul(&λ, &p.X).
		Sub(&evaluations.R1, &p.Y)

	p.X.Set(&xr)
	p.Y.Set(&yr)
}

func (p *G2Affine) addStep(evaluations *LineEvaluationAff, a *G2Affine) {
	var n, d, λ, λλ, xr, yr fptower.E2

	// compute λ = (y2-y1)/(x2-x1)
	n.Sub(&a.Y, &p.Y)
	d.Sub(&a.X, &p.X)
	λ.Div(&n, &d)

	// xr = λ²-x1-x2
	λλ.Square(&λ)
	n.Add(&p.X, &a.X)
	xr.Sub(&λλ, &n)

	// yr = λ(x1-xr) - y1
	yr.Sub(&p.X, &xr).
		Mul(&yr, &λ).
		Sub(&yr, &p.Y)

	evaluations.R0.Set(&λ)
	evaluations.R1.Mul(&λ, &p.X).
		Sub(&evaluations.R1, &p.Y)

	p.X.Set(&xr)
	p.Y.Set(&yr)
}

func (p *G2Affine) doubleAndAddStep(evaluations1, evaluations2 *LineEvaluationAff, a *G2Affine) {
	var A, B, A2, B2, X2A2, t, U, AU, invAU, invA, invU, l1, x3, l2, x4, y4 fptower.E2

	// The Eisenträger-Lauter-Montgomery formula for 2P+Q (https://eprint.iacr.org/2003/257)
	// computes both slopes λ1 and λ2 using a single field inversion via batch inversion.
	//
	// Given P = (x1, y1) and Q = (x2, y2), let:
	//   A = x1 - x2
	//   B = y1 - y2
	//   U = B² - (2x1 + x2)·A²
	//
	// Then:
	//   λ1 = B/A                    (slope for P + Q)
	//   λ2 = -λ1 - 2y1·A²/U         (slope for P + (P+Q))
	//
	// We compute 1/A and 1/U using Montgomery's batch inversion:
	//   1/A = U/(A·U) and 1/U = A/(A·U) with a single inversion of A·U.

	// Compute A = x1 - x2 and B = y1 - y2
	A.Sub(&p.X, &a.X)
	B.Sub(&p.Y, &a.Y)

	// Compute A² and B²
	A2.Square(&A)
	B2.Square(&B)

	// Compute U = B² - (2x1 + x2)·A²
	t.Double(&p.X).Add(&t, &a.X)
	X2A2.Mul(&t, &A2)
	U.Sub(&B2, &X2A2)

	// Batch inversion: compute 1/A and 1/U with a single inversion
	AU.Mul(&A, &U)
	invAU.Inverse(&AU)
	invA.Mul(&U, &invAU)
	invU.Mul(&A, &invAU)

	// λ1 = B/A = B·(1/A)
	l1.Mul(&B, &invA)

	// x3 = λ1² - x1 - x2
	x3.Square(&l1)
	x3.Sub(&x3, &p.X)
	x3.Sub(&x3, &a.X)

	// line1 evaluation
	evaluations1.R0.Set(&l1)
	evaluations1.R1.Mul(&l1, &p.X)
	evaluations1.R1.Sub(&evaluations1.R1, &p.Y)

	// λ2 = -λ1 - 2y1·A²/U = -λ1 - 2y1·A²·(1/U)
	l2.Double(&p.Y)
	l2.Mul(&l2, &A2)
	l2.Mul(&l2, &invU)
	l2.Add(&l2, &l1)
	l2.Neg(&l2)

	// x4 = λ2² - x1 - x3
	x4.Square(&l2)
	x4.Sub(&x4, &p.X)
	x4.Sub(&x4, &x3)

	// y4 = λ2·(x1 - x4) - y1
	y4.Sub(&p.X, &x4)
	y4.Mul(&l2, &y4)
	y4.Sub(&y4, &p.Y)

	// line2 evaluation
	evaluations2.R0.Set(&l2)
	evaluations2.R1.Mul(&l2, &p.X)
	evaluations2.R1.Sub(&evaluations2.R1, &p.Y)

	p.X.Set(&x4)
	p.Y.Set(&y4)
}
