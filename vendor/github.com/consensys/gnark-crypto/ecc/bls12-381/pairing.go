// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

package bls12381

import (
	"errors"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fp"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/internal/fptower"
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
// where s is the cofactor 3 (Hayashida et al.)
func FinalExponentiation(z *GT, _z ...*GT) GT {
	var result GT
	result.Set(z)

	for _, e := range _z {
		result.Mul(&result, e)
	}

	var t [3]GT

	// Easy part
	// (p⁶-1)(p²+1)
	t[0].Conjugate(&result)
	result.Inverse(&result)
	t[0].Mul(&t[0], &result)
	result.FrobeniusSquare(&t[0]).
		Mul(&result, &t[0])

	var one GT
	one.SetOne()
	if result.Equal(&one) {
		return result
	}

	// Hard part (up to permutation)
	// Daiki Hayashida, Kenichiro Hayasaka and Tadanori Teruya
	// https://eprint.iacr.org/2020/875.pdf
	t[0].CyclotomicSquare(&result)
	t[1].ExptHalf(&t[0])
	t[2].InverseUnitary(&result)
	t[1].Mul(&t[1], &t[2])
	t[2].Expt(&t[1])
	t[1].InverseUnitary(&t[1])
	t[1].Mul(&t[1], &t[2])
	t[2].Expt(&t[1])
	t[1].Frobenius(&t[1])
	t[1].Mul(&t[1], &t[2])
	result.Mul(&result, &t[0])
	t[0].Expt(&t[1])
	t[2].Expt(&t[0])
	t[0].FrobeniusSquare(&t[1])
	t[1].InverseUnitary(&t[1])
	t[1].Mul(&t[1], &t[2])
	t[1].Mul(&t[1], &t[0])
	result.Mul(&result, &t[1])

	return result
}

// MillerLoop computes the multi-Miller loop
// ∏ᵢ MillerLoop(Pᵢ, Qᵢ) = ∏ᵢ { fᵢ_{x,Qᵢ}(Pᵢ) }
func MillerLoop(P []G1Affine, Q []G2Affine) (GT, error) {
	// check input size match
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
	for k := 0; k < n; k++ {
		qProj[k].FromAffine(&q[k])
	}

	var result GT
	result.SetOne()
	var l1, l2 lineEvaluation
	var prodLines [5]E2

	// Compute ∏ᵢ { fᵢ_{x₀,Q}(P) }
	if n >= 1 {
		// i = 62, separately to avoid an E12 Square
		// (Square(res) = 1² = 1)
		// LoopCounter[62] = 1
		// k = 0, separately to avoid MulBy014 (res × ℓ)
		// (assign line to res)

		// qProj[0] ← 2qProj[0] and l1 the tangent ℓ passing 2qProj[0]
		qProj[0].doubleStep(&l1)
		// line evaluation at P[0] (assign)
		result.C0.B0.Set(&l1.r0)
		result.C0.B1.MulByElement(&l1.r1, &p[0].X)
		result.C1.B1.MulByElement(&l1.r2, &p[0].Y)

		// qProj[0] ← qProj[0]+Q[0] and
		// l2 the line ℓ passing qProj[0] and Q[0]
		qProj[0].addMixedStep(&l2, &q[0])
		// line evaluation at P[0] (assign)
		l2.r1.MulByElement(&l2.r1, &p[0].X)
		l2.r2.MulByElement(&l2.r2, &p[0].Y)
		// ℓ × res
		prodLines = fptower.Mul014By014(&l2.r0, &l2.r1, &l2.r2, &result.C0.B0, &result.C0.B1, &result.C1.B1)
		result.C0.B0 = prodLines[0]
		result.C0.B1 = prodLines[1]
		result.C0.B2 = prodLines[2]
		result.C1.B1 = prodLines[3]
		result.C1.B2 = prodLines[4]
	}

	// k >= 1
	for k := 1; k < n; k++ {
		// qProj[k] ← 2qProj[k] and l1 the tangent ℓ passing 2qProj[k]
		qProj[k].doubleStep(&l1)
		// line evaluation at P[k]
		l1.r1.MulByElement(&l1.r1, &p[k].X)
		l1.r2.MulByElement(&l1.r2, &p[k].Y)

		// qProj[k] ← qProj[k]+Q[k] and
		// l2 the line ℓ passing qProj[k] and Q[k]
		qProj[k].addMixedStep(&l2, &q[k])
		// line evaluation at P[k]
		l2.r1.MulByElement(&l2.r1, &p[k].X)
		l2.r2.MulByElement(&l2.r2, &p[k].Y)
		// ℓ × ℓ
		prodLines = fptower.Mul014By014(&l2.r0, &l2.r1, &l2.r2, &l1.r0, &l1.r1, &l1.r2)
		// (ℓ × ℓ) × result
		result.MulBy01245(&prodLines)
	}

	// i <= 61
	for i := len(LoopCounter) - 3; i >= 1; i-- {
		// mutualize the square among n Miller loops
		// (∏ᵢfᵢ)²
		result.Square(&result)

		for k := 0; k < n; k++ {
			// qProj[k] ← 2qProj[k] and l1 the tangent ℓ passing 2qProj[k]
			qProj[k].doubleStep(&l1)
			// line evaluation at P[k]
			l1.r1.MulByElement(&l1.r1, &p[k].X)
			l1.r2.MulByElement(&l1.r2, &p[k].Y)

			if LoopCounter[i] == 0 {
				// ℓ × res
				result.MulBy014(&l1.r0, &l1.r1, &l1.r2)
			} else {
				// qProj[k] ← qProj[k]+Q[k] and
				// l2 the line ℓ passing qProj[k] and Q[k]
				qProj[k].addMixedStep(&l2, &q[k])
				// line evaluation at P[k]
				l2.r1.MulByElement(&l2.r1, &p[k].X)
				l2.r2.MulByElement(&l2.r2, &p[k].Y)
				// ℓ × ℓ
				prodLines = fptower.Mul014By014(&l2.r0, &l2.r1, &l2.r2, &l1.r0, &l1.r1, &l1.r2)
				// (ℓ × ℓ) × result
				result.MulBy01245(&prodLines)
			}
		}
	}

	// i = 0, separately to avoid a point doubling
	// LoopCounter[0] = 0
	result.Square(&result)
	for k := 0; k < n; k++ {
		// l1 the tangent ℓ passing 2qProj[k]
		qProj[k].tangentLine(&l1)
		// line evaluation at P[k]
		l1.r1.MulByElement(&l1.r1, &p[k].X)
		l1.r2.MulByElement(&l1.r2, &p[k].Y)
		// ℓ × result
		result.MulBy014(&l1.r0, &l1.r1, &l1.r2)
	}

	// negative x₀
	result.Conjugate(&result)

	return result, nil
}

// doubleStep doubles a point in Homogenous projective coordinates, and evaluates the line in Miller loop
// https://eprint.iacr.org/2013/722.pdf (Section 4.3)
func (p *g2Proj) doubleStep(l *lineEvaluation) {

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
	l.r0.Set(&I)
	l.r1.Double(&J).
		Add(&l.r1, &J)
	l.r2.Neg(&H)

}

// addMixedStep point addition in Mixed Homogenous projective and Affine coordinates
// https://eprint.iacr.org/2013/722.pdf (Section 4.3)
func (p *g2Proj) addMixedStep(l *lineEvaluation, a *G2Affine) {

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
	l.r0.Set(&J)
	l.r1.Neg(&O)
	l.r2.Set(&L)
}

// tangentCompute computes the tangent through [2]p in Homogenous projective coordinates.
// It does not compute the resulting point [2]p.
func (p *g2Proj) tangentLine(l *lineEvaluation) {

	// get some Element from our pool
	var t1, B, C, D, E, H, I, J fptower.E2
	B.Square(&p.y)
	C.Square(&p.z)
	D.Double(&C).
		Add(&D, &C)
	E.MulBybTwistCurveCoeff(&D)
	H.Add(&p.y, &p.z).
		Square(&H)
	t1.Add(&B, &C)
	H.Sub(&H, &t1)
	I.Sub(&E, &B)
	J.Square(&p.x)

	// Line evaluation
	l.r0.Set(&I)
	l.r1.Double(&J).
		Add(&l.r1, &J)
	l.r2.Neg(&H)
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
func PairFixedQ(P []G1Affine, lines [][2][len(LoopCounter) - 1]LineEvaluationAff) (GT, error) {
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
func PairingCheckFixedQ(P []G1Affine, lines [][2][len(LoopCounter) - 1]LineEvaluationAff) (bool, error) {
	f, err := PairFixedQ(P, lines)
	if err != nil {
		return false, err
	}
	var one GT
	one.SetOne()
	return f.Equal(&one), nil
}

// manyDoublesAndAdd performs k consecutive doublings followed by a doubleAndAdd operation,
// using a single batch inversion for all k+2 line evaluations.
// This combines manyDoubleSteps(k) + doubleAndAddStep into one batch inversion (2 inv → 1 inv).
//
// Output:
//   - doubEvals[0..k-1]: line evaluations for the k doublings
//   - addEval1: first line evaluation from doubleAndAdd (chord through P and Q)
//   - addEval2: second line evaluation from doubleAndAdd (for the final doubling)
func (p *G2Affine) manyDoublesAndAdd(k int, doubEvals []LineEvaluationAff, addEval1, addEval2 *LineEvaluationAff, a *G2Affine) {
	if k == 0 {
		// Just do doubleAndAddStep
		p.doubleAndAddStep(addEval1, addEval2, a)
		return
	}

	// Step 1: Compute A[i], B[i], C[i] using the recurrence
	// A[i] represents scaled x-coordinate: x[i] = A[i] / T[i-1]²
	// C[i] represents scaled -y-coordinate: y[i] = -C[i] / T[i-1]³
	// B[i] = 3*A[i]² (numerator for λ = 3x²/2y)
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
		C[i].Add(&C[i], &tmp)
	}

	// Step 2: Compute D[i] = -2*C[i] = 2*y[i] for i = 0..k-1
	D := make([]fptower.E2, k)
	for i := range k {
		D[i].Double(&C[i]).Neg(&D[i])
	}

	// Step 3: Compute T[i] = D[0]*D[1]*...*D[i] for i = 0..k-1
	// We extend T to k+1 elements to include the ELM denominator
	T := make([]fptower.E2, k+1)
	T[0].Set(&D[0])
	for i := 1; i < k; i++ {
		T[i].Mul(&T[i-1], &D[i])
	}
	// S = T[k-1] is the accumulated scaling factor
	// Point after k doublings: x_k = A[k]/S², y_k = -C[k]/S³

	// Step 4: Compute ELM numerators for the doubleAndAdd
	// The point P = (A[k]/S², -C[k]/S³) and we add Q = (a.X, a.Y)
	//
	// In ELM formula:
	//   A_elm = x_P - x_Q = A[k]/S² - a.X = (A[k] - a.X*S²) / S²
	//   B_elm = y_P - y_Q = -C[k]/S³ - a.Y = (-C[k] - a.Y*S³) / S³
	//
	// λ1 = B_elm/A_elm = B_num/(S*A_num) where:
	//   A_num = A[k] - a.X*S²
	//   B_num = -C[k] - a.Y*S³
	//
	// U = B² - (2x_P + x_Q)*A²
	// U_num = B_num² - (2*A[k] + a.X*S²)*A_num²

	var S2, S3, A_num, B_num, U_num fptower.E2
	S := &T[k-1]
	S2.Square(S)
	S3.Mul(&S2, S)

	// A_num = A[k] - a.X * S²
	A_num.Mul(&a.X, &S2)
	A_num.Sub(&A[k], &A_num)

	// B_num = -C[k] - a.Y * S³
	B_num.Neg(&C[k]) // B_num = -C[k]
	tmp.Mul(&a.Y, &S3)
	B_num.Sub(&B_num, &tmp) // B_num = -C[k] - a.Y*S³

	// U_num = B_num² - (2*A[k] + a.X*S²)*A_num²
	var A_num2, B_num2, coeff fptower.E2
	A_num2.Square(&A_num)
	B_num2.Square(&B_num)
	coeff.Double(&A[k])
	tmp.Mul(&a.X, &S2)
	coeff.Add(&coeff, &tmp) // coeff = 2*A[k] + a.X*S²
	U_num.Mul(&coeff, &A_num2)
	U_num.Sub(&B_num2, &U_num)

	// Step 5: Extend T for ELM batch inversion
	// T[k] = S * A_num * U_num
	// This allows us to compute 1/(S*A_num) and 1/(S*U_num) from 1/T[k]
	T[k].Mul(S, &A_num)
	T[k].Mul(&T[k], &U_num)

	// Step 6: Batch invert T[0..k]
	invT := fptower.BatchInvertE2(T)

	// Step 7: Compute doubling line evaluations (same as manyDoubleSteps)
	// For i = 0: R0 = B[0]/T[0], R1 = B[0]*A[0]/T[0] + C[0]
	doubEvals[0].R0.Mul(&B[0], &invT[0])
	doubEvals[0].R1.Mul(&B[0], &A[0]).Mul(&doubEvals[0].R1, &invT[0]).Add(&doubEvals[0].R1, &C[0])

	// For i = 1..k-1
	var invT2, invT3 fptower.E2
	for i := 1; i < k; i++ {
		// R0 = B[i] / T[i]
		doubEvals[i].R0.Mul(&B[i], &invT[i])

		// R1 = B[i]*A[i]/(T[i]*T[i-1]²) + C[i]/T[i-1]³
		invT2.Square(&invT[i-1])
		invT3.Mul(&invT2, &invT[i-1])

		var term1, term2 fptower.E2
		term1.Mul(&B[i], &A[i]).Mul(&term1, &invT[i]).Mul(&term1, &invT2)
		term2.Mul(&C[i], &invT3)
		doubEvals[i].R1.Add(&term1, &term2)
	}

	// Step 8: Compute point coordinates after k doublings
	// x_P = A[k] / S², y_P = -C[k] / S³
	invT2.Square(&invT[k-1])
	invT3.Mul(&invT2, &invT[k-1])
	var x_P, y_P fptower.E2
	x_P.Mul(&A[k], &invT2)
	y_P.Mul(&C[k], &invT3).Neg(&y_P)

	// Step 9: Compute ELM slopes using batch-inverted values
	// From T[k] = S * A_num * U_num and invT[k] = 1/T[k]:
	//   1/(S*A_num) = U_num * invT[k]
	//   1/(S*U_num) = A_num * invT[k]
	var invSA, invSU fptower.E2
	invSA.Mul(&U_num, &invT[k])
	invSU.Mul(&A_num, &invT[k])

	// λ1 = B_num / (S * A_num) = B_num * invSA
	var l1 fptower.E2
	l1.Mul(&B_num, &invSA)

	// λ2 = -λ1 - 2*y_P*A²/U
	// In scaled coordinates:
	//   y_P = -C[k]/S³, A² = A_num²/S⁴, U = U_num/S⁶
	//   2*y_P*A²/U = 2*(-C[k]/S³)*(A_num²/S⁴)*(S⁶/U_num)
	//             = -2*C[k]*A_num²/(S*U_num)
	//             = -2*C[k]*A_num²*invSU
	var l2 fptower.E2
	l2.Double(&C[k])
	l2.Neg(&l2)          // -2*C[k]
	l2.Mul(&l2, &A_num2) // -2*C[k]*A_num²
	l2.Mul(&l2, &invSU)  // -2*C[k]*A_num²/(S*U_num) = 2*y_P*A²/U
	l2.Add(&l2, &l1)
	l2.Neg(&l2)

	// Step 10: Compute ELM line evaluations
	// addEval1: R0 = λ1, R1 = λ1*x_P - y_P
	addEval1.R0.Set(&l1)
	addEval1.R1.Mul(&l1, &x_P)
	addEval1.R1.Sub(&addEval1.R1, &y_P)

	// addEval2: R0 = λ2, R1 = λ2*x_P - y_P
	addEval2.R0.Set(&l2)
	addEval2.R1.Mul(&l2, &x_P)
	addEval2.R1.Sub(&addEval2.R1, &y_P)

	// Step 11: Compute final point 2P + Q
	// x_{P+Q} = λ1² - x_P - a.X
	var xPQ fptower.E2
	xPQ.Square(&l1)
	xPQ.Sub(&xPQ, &x_P)
	xPQ.Sub(&xPQ, &a.X)

	// x_{2P+Q} = λ2² - x_P - x_{P+Q}
	var x4, y4 fptower.E2
	x4.Square(&l2)
	x4.Sub(&x4, &x_P)
	x4.Sub(&x4, &xPQ)

	// y_{2P+Q} = λ2*(x_P - x_{2P+Q}) - y_P
	y4.Sub(&x_P, &x4)
	y4.Mul(&l2, &y4)
	y4.Sub(&y4, &y_P)

	p.X.Set(&x4)
	p.Y.Set(&y4)
}

// manyDoubleSteps performs k consecutive doublings on p and returns the line evaluations.
// It uses a recurrence to compute 2^k*P with a single batch inversion.
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

		// R1 = B[i]*A[i]/(T[i]*T[i-1]²) + C[i]/T[i-1]³
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

// PrecomputeLines precomputes the lines for the fixed-argument Miller loop
func PrecomputeLines(Q G2Affine) (PrecomputedLines [2][len(LoopCounter) - 1]LineEvaluationAff) {
	var accQ G2Affine
	accQ.Set(&Q)

	// LoopCounter non-zero values: 16(1), 48(1), 57(1), 60(1), 62(1), 63(1)
	// Structure optimized using manyDoublesAndAdd to minimize inversions:
	//   - i=62: doubleStep + addStep (2 inv) - can't use ELM when P=Q
	//   - i=61,60: manyDoublesAndAdd(1) (1 inv)
	//   - i=59,58,57: manyDoublesAndAdd(2) (1 inv)
	//   - i=56-49,48: manyDoublesAndAdd(8) (1 inv)
	//   - i=47-17,16: manyDoublesAndAdd(31) (1 inv)
	//   - i=15-0: manyDoubleSteps(16) (1 inv)
	// Total: 7 inversions (down from 12)

	// i=62: LoopCounter[62]=1, double+add
	// Cannot use doubleAndAddStep here because accQ = Q, causing division by zero
	// in ELM formula (A = x_P - x_Q = 0)
	accQ.doubleStep(&PrecomputedLines[0][62])
	accQ.addStep(&PrecomputedLines[1][62], &Q)

	// i=61,60: 1 double followed by doubleAndAdd
	{
		var doubEvals [1]LineEvaluationAff
		accQ.manyDoublesAndAdd(1, doubEvals[:],
			&PrecomputedLines[0][60], &PrecomputedLines[1][60], &Q)
		PrecomputedLines[0][61] = doubEvals[0]
	}

	// i=59,58,57: 2 doubles followed by doubleAndAdd
	{
		var doubEvals [2]LineEvaluationAff
		accQ.manyDoublesAndAdd(2, doubEvals[:],
			&PrecomputedLines[0][57], &PrecomputedLines[1][57], &Q)
		PrecomputedLines[0][59] = doubEvals[0]
		PrecomputedLines[0][58] = doubEvals[1]
	}

	// i=56-49,48: 8 doubles followed by doubleAndAdd
	{
		var doubEvals [8]LineEvaluationAff
		accQ.manyDoublesAndAdd(8, doubEvals[:],
			&PrecomputedLines[0][48], &PrecomputedLines[1][48], &Q)
		PrecomputedLines[0][56] = doubEvals[0]
		PrecomputedLines[0][55] = doubEvals[1]
		PrecomputedLines[0][54] = doubEvals[2]
		PrecomputedLines[0][53] = doubEvals[3]
		PrecomputedLines[0][52] = doubEvals[4]
		PrecomputedLines[0][51] = doubEvals[5]
		PrecomputedLines[0][50] = doubEvals[6]
		PrecomputedLines[0][49] = doubEvals[7]
	}

	// i=47-17,16: 31 doubles followed by doubleAndAdd
	{
		var doubEvals [31]LineEvaluationAff
		accQ.manyDoublesAndAdd(31, doubEvals[:],
			&PrecomputedLines[0][16], &PrecomputedLines[1][16], &Q)
		for j := range 31 {
			PrecomputedLines[0][47-j] = doubEvals[j]
		}
	}

	// i=15-0: 16 consecutive zeros, no add at the end
	{
		var evals [16]LineEvaluationAff
		accQ.manyDoubleSteps(16, evals[:])
		for j := range 16 {
			PrecomputedLines[0][15-j] = evals[j]
		}
	}

	return PrecomputedLines
}

// MillerLoopFixedQ computes the multi-Miller loop as in MillerLoop
// but Qᵢ are fixed points in G2 known in advance.
func MillerLoopFixedQ(P []G1Affine, lines [][2][len(LoopCounter) - 1]LineEvaluationAff) (GT, error) {
	n := len(P)
	if n == 0 || n != len(lines) {
		return GT{}, errors.New("invalid inputs sizes")
	}

	// no need to filter infinity points:
	// 		1. if Pᵢ=(0,0) then -x/y=1/y=0 by gnark-crypto convention and so
	// 		lines R0 and R1 are 0. It happens that result will stay, through
	// 		the Miller loop, in 𝔽p⁶ because MulBy01(0,0,1),
	// 		Mul01By01(0,0,1,0,0,1) and MulBy01245 set result.C0 to 0. At the
	// 		end result will be in a proper subgroup of Fp¹² so it be reduced to
	// 		1 in FinalExponentiation.
	//
	//      and/or
	//
	// 		2. if Qᵢ=(0,0) then PrecomputeLines(Qᵢ) will return lines R0 and R1
	// 		that are 0 because of gnark-convention (*/0==0) in doubleStep and
	// 		addStep. Similarly to Pᵢ=(0,0) it happens that result be 1
	// 		after the FinalExponentiation.

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

	// Compute ∏ᵢ { fᵢ_{x₀,Q}(P) }
	for i := len(LoopCounter) - 2; i >= 0; i-- {
		// mutualize the square among n Miller loops
		// (∏ᵢfᵢ)²
		result.Square(&result)

		for k := range n {
			// line evaluation at P[k]
			lines[k][0][i].R1.
				MulByElement(
					&lines[k][0][i].R1,
					&yInv[k],
				)
			lines[k][0][i].R0.
				MulByElement(&lines[k][0][i].R0,
					&xNegOverY[k],
				)
			if LoopCounter[i] == 0 {
				// ℓ × res
				result.MulBy01(
					&lines[k][0][i].R1,
					&lines[k][0][i].R0,
				)

			} else {
				lines[k][1][i].R1.
					MulByElement(
						&lines[k][1][i].R1,
						&yInv[k],
					)
				lines[k][1][i].R0.
					MulByElement(
						&lines[k][1][i].R0,
						&xNegOverY[k],
					)
				prodLines = fptower.Mul01By01(
					&lines[k][0][i].R1, &lines[k][0][i].R0,
					&lines[k][1][i].R1, &lines[k][1][i].R0,
				)
				result.MulBy01245(&prodLines)
			}
		}
	}

	// negative x₀
	result.Conjugate(&result)

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
