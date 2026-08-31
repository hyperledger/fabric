package ecc

import (
	"math/big"
	"math/bits"
)

//-------------------------------------------------------
// Ate loop counter (not used for each curve)

// NafDecomposition gets the naf decomposition of a big number
func NafDecomposition(a *big.Int, result []int8) int {

	var zero, one, two, three big.Int

	one.SetUint64(1)
	two.SetUint64(2)
	three.SetUint64(3)

	length := 0

	// some buffers
	var buf, aCopy big.Int
	aCopy.Set(a)

	for aCopy.Cmp(&zero) != 0 {

		// if aCopy % 2 == 0
		buf.And(&aCopy, &one)

		// aCopy even
		if buf.Cmp(&zero) == 0 {
			result[length] = 0
		} else { // aCopy odd
			buf.And(&aCopy, &three)
			if buf.Cmp(&three) == 0 {
				result[length] = -1
				aCopy.Add(&aCopy, &one)
			} else {
				result[length] = 1
			}
		}
		aCopy.Rsh(&aCopy, 1)
		length++
	}
	return length
}

// WnafDecomposition gets the wNAF decomposition of a big number.
// 2 <= window <= 8 is the wNAF window size.
func WnafDecomposition(a *big.Int, window uint, result []int8) int {
	if window < 2 || window > 8 {
		return 0
	}
	if a.Sign() == 0 {
		return 0
	}

	var aCopy, mask, mod, tmp big.Int
	aCopy.Set(a)
	mask.SetUint64((1 << window) - 1)

	length := 0
	for aCopy.Sign() != 0 {
		if aCopy.Bit(0) == 1 {
			mod.And(&aCopy, &mask)
			u := mod.Int64()
			if u > (1 << (window - 1)) {
				u -= (1 << window)
			}
			result[length] = int8(u)
			if u > 0 {
				tmp.SetInt64(u)
				aCopy.Sub(&aCopy, &tmp)
			} else {
				tmp.SetInt64(-u)
				aCopy.Add(&aCopy, &tmp)
			}
		} else {
			result[length] = 0
		}
		aCopy.Rsh(&aCopy, 1)
		length++
	}
	return length
}

//-------------------------------------------------------
// GLV utils

// Lattice represents a Z module spanned by V1, V2.
// det is the associated determinant.
type Lattice struct {
	V1, V2      [2]big.Int
	Det, b1, b2 big.Int
}

// Lattice4 represents a Z module spanned by V[0], V[1], V[2], V[3].
// The vectors are stored as columns: V[col][row].
// det is the associated determinant.
type Lattice4 struct {
	V              [4][4]big.Int
	Det            big.Int
	b1, b2, b3, b4 big.Int
}

// PrecomputeLattice res such that res.V1, res.V2
// are short vectors satisfying v11+v12.λ=v21+v22.λ=0[r].
// cf https://www.iacr.org/archive/crypto2001/21390189.pdf
func PrecomputeLattice(r, lambda *big.Int, res *Lattice) {

	var rst [2][3]big.Int
	var tmp [3]big.Int
	var quotient, remainder, sqroot, _r, _t big.Int

	rst[0][0].Set(r)
	rst[0][1].SetUint64(1)
	rst[0][2].SetUint64(0)

	rst[1][0].Set(lambda)
	rst[1][1].SetUint64(0)
	rst[1][2].SetUint64(1)

	sqroot.Sqrt(r)

	var one big.Int
	one.SetUint64(1)

	// r_i+1 = r_i-1 - q_i.r_i
	// s_i+1 = s_i-1 - q_i.s_i
	// t_i+1 = t_i-1 - q_i.s_i
	for rst[1][0].Cmp(&sqroot) >= 1 {

		quotient.Div(&rst[0][0], &rst[1][0])
		remainder.Mod(&rst[0][0], &rst[1][0])

		tmp[0].Set(&rst[1][0])
		tmp[1].Set(&rst[1][1])
		tmp[2].Set(&rst[1][2])

		rst[1][0].Set(&remainder)
		rst[1][1].Mul(&rst[1][1], &quotient).Sub(&rst[0][1], &rst[1][1])
		rst[1][2].Mul(&rst[1][2], &quotient).Sub(&rst[0][2], &rst[1][2])

		rst[0][0].Set(&tmp[0])
		rst[0][1].Set(&tmp[1])
		rst[0][2].Set(&tmp[2])
	}

	quotient.Div(&rst[0][0], &rst[1][0])
	remainder.Mod(&rst[0][0], &rst[1][0])
	_r.Set(&remainder)
	_t.Mul(&rst[1][2], &quotient).Sub(&rst[0][2], &_t)

	res.V1[0].Set(&rst[1][0])
	res.V1[1].Neg(&rst[1][2])

	// take the shorter of [rst[0][0], rst[0][2]], [_r, _t]
	tmp[1].Mul(&rst[0][2], &rst[0][2])
	tmp[0].Mul(&rst[0][0], &rst[0][0]).Add(&tmp[1], &tmp[0])
	tmp[2].Mul(&_r, &_r)
	tmp[1].Mul(&_t, &_t).Add(&tmp[2], &tmp[1])
	if tmp[0].Cmp(&tmp[1]) == 1 {
		res.V2[0].Set(&_r)
		res.V2[1].Neg(&_t)
	} else {
		res.V2[0].Set(&rst[0][0])
		res.V2[1].Neg(&rst[0][2])
	}

	// sets determinant
	tmp[0].Mul(&res.V1[1], &res.V2[0])
	res.Det.Mul(&res.V1[0], &res.V2[1]).Sub(&res.Det, &tmp[0])

	// sets roundings of 2^n*v21/d and 2^n*v11/d (where 2ⁿ > d)
	n := 2 * uint(((res.Det.BitLen()+32)>>6)<<6)
	res.b1.Lsh(&res.V2[1], n)
	rounding(&res.b1, &res.Det, &res.b1)
	res.b2.Lsh(&res.V1[1], n)
	rounding(&res.b2, &res.Det, &res.b2)
}

// PrecomputeLattice4 sets res.Det and the rounding constants (b1..b4)
// for a 4-dimensional lattice basis stored in res.V.
// The constants correspond to the first column of adj(res.V)/det(res.V),
// used for the closest vector approximation to (s,0,0,0).
func PrecomputeLattice4(res *Lattice4) {
	var cof [4]big.Int
	// cofactors for row 0
	cof[0] = det3(
		&res.V[1][1], &res.V[2][1], &res.V[3][1],
		&res.V[1][2], &res.V[2][2], &res.V[3][2],
		&res.V[1][3], &res.V[2][3], &res.V[3][3],
	)
	cof[1] = det3(
		&res.V[0][1], &res.V[2][1], &res.V[3][1],
		&res.V[0][2], &res.V[2][2], &res.V[3][2],
		&res.V[0][3], &res.V[2][3], &res.V[3][3],
	)
	cof[1].Neg(&cof[1])
	cof[2] = det3(
		&res.V[0][1], &res.V[1][1], &res.V[3][1],
		&res.V[0][2], &res.V[1][2], &res.V[3][2],
		&res.V[0][3], &res.V[1][3], &res.V[3][3],
	)
	cof[3] = det3(
		&res.V[0][1], &res.V[1][1], &res.V[2][1],
		&res.V[0][2], &res.V[1][2], &res.V[2][2],
		&res.V[0][3], &res.V[1][3], &res.V[2][3],
	)
	cof[3].Neg(&cof[3])

	var tmp big.Int
	res.Det.SetUint64(0)
	for col := range 4 {
		tmp.Mul(&res.V[col][0], &cof[col])
		res.Det.Add(&res.Det, &tmp)
	}
	if res.Det.Sign() < 0 {
		res.Det.Neg(&res.Det)
		for i := range 4 {
			cof[i].Neg(&cof[i])
		}
	}

	n := 2 * uint(((res.Det.BitLen()+32)>>6)<<6)
	tmp.Lsh(&cof[0], n)
	roundingSigned(&tmp, &res.Det, &res.b1)
	tmp.Lsh(&cof[1], n)
	roundingSigned(&tmp, &res.Det, &res.b2)
	tmp.Lsh(&cof[2], n)
	roundingSigned(&tmp, &res.Det, &res.b3)
	tmp.Lsh(&cof[3], n)
	roundingSigned(&tmp, &res.Det, &res.b4)
}

// SplitScalar outputs u,v such that u+vlambda=s[r].
// The method is to view s as (s,0) in ZxZ, and find a close
// vector w of (s,0) in <l>, where l is a sub Z-module of
// ker((a,b) → a+b.λ[r]): then (u,v)=w-(s,0), and
// u+v.λ=s[r].
// cf https://www.iacr.org/archive/crypto2001/21390189.pdf
func SplitScalar(s *big.Int, l *Lattice) [2]big.Int {

	var k1, k2 big.Int
	k1.Mul(s, &l.b1)
	k2.Mul(s, &l.b2).Neg(&k2)
	// right-shift instead of division by lattice determinant
	// this increases the bounds on k1 and k2 by 1
	// but we check this ScalarMultiplication alg. (not constant-time)
	n := 2 * uint(((l.Det.BitLen()+32)>>6)<<6)
	k1.Rsh(&k1, n)
	k2.Rsh(&k2, n)
	v := getVector(l, &k1, &k2)
	v[0].Sub(s, &v[0])
	v[1].Neg(&v[1])
	return v
}

// SplitScalarFour computes k0,k1,k2,k3 such that
// k0+k1*lambda1+k2*lambda2+k3*lambda1*lambda2 = s [r].
// It uses a closest vector approximation in a 4-dimensional lattice.
func SplitScalarFour(s *big.Int, l *Lattice4) [4]big.Int {
	var k1, k2, k3, k4 big.Int
	k1.Mul(s, &l.b1)
	k2.Mul(s, &l.b2)
	k3.Mul(s, &l.b3)
	k4.Mul(s, &l.b4)

	n := 2 * uint(((l.Det.BitLen()+32)>>6)<<6)
	k1.Rsh(&k1, n)
	k2.Rsh(&k2, n)
	k3.Rsh(&k3, n)
	k4.Rsh(&k4, n)

	res := getVector4(l, &k1, &k2, &k3, &k4)
	res[0].Sub(s, &res[0])
	res[1].Neg(&res[1])
	res[2].Neg(&res[2])
	res[3].Neg(&res[3])
	return res
}

// sets res to the closest integer from n/d
func rounding(n, d, res *big.Int) {
	var dshift, r, one big.Int
	one.SetUint64(1)
	dshift.Rsh(d, 1)
	r.Mod(n, d)
	res.Div(n, d)
	if r.Cmp(&dshift) == 1 {
		res.Add(res, &one)
	}
}

// roundingSigned rounds n/d to the closest integer and preserves sign.
func roundingSigned(n, d, res *big.Int) {
	if n.Sign() >= 0 {
		rounding(n, d, res)
		return
	}
	var nn big.Int
	nn.Neg(n)
	rounding(&nn, d, res)
	res.Neg(res)
}

// getVector returns aV1 + bV2
func getVector(l *Lattice, a, b *big.Int) [2]big.Int {
	var res [2]big.Int
	var tmp big.Int
	tmp.Mul(b, &l.V2[0])
	res[0].Mul(a, &l.V1[0]).Add(&res[0], &tmp)
	tmp.Mul(b, &l.V2[1])
	res[1].Mul(a, &l.V1[1]).Add(&res[1], &tmp)
	return res
}

// getVector4 returns aV1 + bV2 + cV3 + dV4.
func getVector4(l *Lattice4, a, b, c, d *big.Int) [4]big.Int {
	var res [4]big.Int
	coeffs := [4]*big.Int{a, b, c, d}
	var tmp big.Int
	for row := range 4 {
		for col := range 4 {
			tmp.Mul(coeffs[col], &l.V[col][row])
			res[row].Add(&res[row], &tmp)
		}
	}
	return res
}

func det3(a00, a01, a02, a10, a11, a12, a20, a21, a22 *big.Int) big.Int {
	var t0, t1, t2, t3, t4 big.Int
	t0.Mul(a11, a22)
	t1.Mul(a12, a21)
	t0.Sub(&t0, &t1)

	t1.Mul(a10, a22)
	t2.Mul(a12, a20)
	t1.Sub(&t1, &t2)

	t2.Mul(a10, a21)
	t3.Mul(a11, a20)
	t2.Sub(&t2, &t3)

	t3.Mul(a00, &t0)
	t4.Mul(a01, &t1)
	t3.Sub(&t3, &t4)
	t4.Mul(a02, &t2)
	t3.Add(&t3, &t4)
	return t3
}

// NextPowerOfTwo(n) = 2^⌈log₂(n)⌉
// or 2ᵏ where 2ᵏ⁻¹ < n ≤ 2ᵏ
func NextPowerOfTwo(n uint64) uint64 {
	c := bits.OnesCount64(n)
	if c == 0 {
		return 1
	}
	if c == 1 {
		return n
	}
	t := bits.LeadingZeros64(n)
	if t == 0 {
		panic("next power of 2 overflows uint64")
	}
	return uint64(1) << (64 - t)
}
