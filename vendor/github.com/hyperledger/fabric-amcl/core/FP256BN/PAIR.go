/*
 * Copyright (c) 2012-2020 MIRACL UK Ltd.
 *
 * This file is part of MIRACL Core
 * (see https://github.com/miracl/core).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/* BN/BLS Curve Pairing functions */

package FP256BN



// Point doubling for pairings
func dbl(A *ECP2, AA *FP2, BB *FP2, CC *FP2) {
	CC.copy(A.getx())          //X
	YY := NewFP2copy(A.gety()) //Y
	BB.copy(A.getz())          //Z
	AA.copy(YY)                //Y
	AA.mul(BB)                 //YZ
	CC.sqr()                   //X^2
	YY.sqr()                   //Y^2
	BB.sqr()                   //Z^2

	AA.add(AA)
	AA.neg()
	AA.norm() //-2AA
	AA.mul_ip()
	AA.norm()

	sb := 3 * CURVE_B_I
	BB.imul(sb)
	CC.imul(3)
	if SEXTIC_TWIST == D_TYPE {
		YY.mul_ip()
		YY.norm()
		CC.mul_ip()
		CC.norm()
	}
	if SEXTIC_TWIST == M_TYPE {
		BB.mul_ip()
		BB.norm()
	}
	BB.sub(YY)
	BB.norm()

	A.dbl()

	/*
		AA.imul(4)
		AA.neg()
		AA.norm()   //-4YZ

		CC.imul(6)  //6X^2

		sb := 3 * CURVE_B_I
		BB.imul(sb) // 3bZ^2
		if SEXTIC_TWIST == D_TYPE {
			BB.div_ip2()
		}
		if SEXTIC_TWIST == M_TYPE {
			BB.mul_ip()
			BB.add(BB)
			AA.mul_ip()
			AA.norm()
		}
		BB.norm() // 3b.Z^2

		YY.add(YY)
		BB.sub(YY)
		BB.norm() // 3b.Z^2-2Y^2

		A.dbl() */
}

// Point addition for pairings
func add(A *ECP2, B *ECP2, AA *FP2, BB *FP2, CC *FP2) {
	AA.copy(A.getx())          // X1
	CC.copy(A.gety())          // Y1
	T1 := NewFP2copy(A.getz()) // Z1
	BB.copy(A.getz())          // Z1

	T1.mul(B.gety()) // T1=Z1.Y2
	BB.mul(B.getx()) // T2=Z1.X2

	AA.sub(BB)
	AA.norm() // X1=X1-Z1.X2
	CC.sub(T1)
	CC.norm() // Y1=Y1-Z1.Y2

	T1.copy(AA) // T1=X1-Z1.X2

	if SEXTIC_TWIST == M_TYPE {
		AA.mul_ip()
		AA.norm()
	}

	T1.mul(B.gety()) // T1=(X1-Z1.X2).Y2

	BB.copy(CC)      // T2=Y1-Z1.Y2
	BB.mul(B.getx()) // T2=(Y1-Z1.Y2).X2
	BB.sub(T1)
	BB.norm() // T2=(Y1-Z1.Y2).X2 - (X1-Z1.X2).Y2
	CC.neg()
	CC.norm() // Y1=-(Y1-Z1.Y2).Xs

	A.Add(B)
}

/* Line function */
func line(A *ECP2, B *ECP2, Qx *FP, Qy *FP) *FP12 {
	AA := NewFP2()
	BB := NewFP2()
	CC := NewFP2()

	var a *FP4
	var b *FP4
	var c *FP4

	if A == B {
		dbl(A, AA, BB, CC)
	} else {
		add(A, B, AA, BB, CC)
	}

	CC.pmul(Qx)
	AA.pmul(Qy)

	a = NewFP4fp2s(AA, BB)

	if SEXTIC_TWIST == D_TYPE {
		b = NewFP4fp2(CC) // L(0,1) | L(0,0) | L(1,0)
		c = NewFP4()
	}
	if SEXTIC_TWIST == M_TYPE {
		b = NewFP4()
		c = NewFP4fp2(CC)
		c.times_i()
	}

	r := NewFP12fp4s(a, b, c)
	r.stype = FP_SPARSER
	return r
}

/* prepare ate parameter, n=6u+2 (BN) or n=u (BLS), n3=3*n */
func lbits(n3 *BIG, n *BIG) int {
	n.copy(NewBIGints(CURVE_Bnx))
	if CURVE_PAIRING_TYPE == BN {
		n.pmul(6)
		if SIGN_OF_X == POSITIVEX {
			n.inc(2)
		} else {
			n.dec(2)
		}
	}

	n.norm()
	n3.copy(n)
	n3.pmul(3)
	n3.norm()
	return n3.nbits()
}

/* prepare for multi-pairing */
func Initmp() []*FP12 {
	var r []*FP12
	for i := ATE_BITS - 1; i >= 0; i-- {
		r = append(r, NewFP12int(1))
	}
	return r
}

/* basic Miller loop */
func Miller(r []*FP12) *FP12 {
	res := NewFP12int(1)
	for i := ATE_BITS - 1; i >= 1; i-- {
		res.sqr()
		res.ssmul(r[i])
		r[i].zero()
	}

	if SIGN_OF_X == NEGATIVEX {
		res.conj()
	}
	res.ssmul(r[0])
	r[0].zero()
	return res
}

// Store precomputed line details in an FP4
func pack(AA *FP2, BB *FP2, CC *FP2) *FP4 {
	i := NewFP2copy(CC)
	i.inverse(nil)
	a := NewFP2copy(AA)
	a.mul(i)
	b := NewFP2copy(BB)
	b.mul(i)
	return NewFP4fp2s(a, b)
}

// Unpack G2 line function details and include G1
func unpack(T *FP4, Qx *FP, Qy *FP) *FP12 {
	var a *FP4
	var b *FP4
	var c *FP4

	a = NewFP4copy(T)
	a.geta().pmul(Qy)
	t := NewFP2fp(Qx)
	if SEXTIC_TWIST == D_TYPE {
		b = NewFP4fp2(t)
		c = NewFP4()
	}
	if SEXTIC_TWIST == M_TYPE {
		b = NewFP4()
		c = NewFP4fp2(t)
		c.times_i()
	}
	v := NewFP12fp4s(a, b, c)
	v.stype = FP_SPARSEST
	return v
}

func precomp(GV *ECP2) []*FP4 {
	var f *FP2
	n := NewBIG()
	n3 := NewBIG()
	K := NewECP2()
	AA := NewFP2()
	BB := NewFP2()
	CC := NewFP2()
	var bt int
	P := NewECP2()
	P.Copy(GV)

	if CURVE_PAIRING_TYPE == BN {
		f = NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))
		if SEXTIC_TWIST == M_TYPE {
			f.inverse(nil)
			f.norm()
		}
	}
	A := NewECP2()
	A.Copy(P)
	MP := NewECP2()
	MP.Copy(P)
	MP.neg()

	nb := lbits(n3, n)
	var T []*FP4

	for i := nb - 2; i >= 1; i-- {
		dbl(A, AA, BB, CC)
		T = append(T, pack(AA, BB, CC))
		bt = n3.bit(i) - n.bit(i)
		if bt == 1 {
			add(A, P, AA, BB, CC)
			T = append(T, pack(AA, BB, CC))
		}
		if bt == -1 {
			add(A, MP, AA, BB, CC)
			T = append(T, pack(AA, BB, CC))
		}
	}
	if CURVE_PAIRING_TYPE == BN {
		if SIGN_OF_X == NEGATIVEX {
			A.neg()
		}
		K.Copy(P)
		K.frob(f)
		add(A, K, AA, BB, CC)
		T = append(T, pack(AA, BB, CC))
		K.frob(f)
		K.neg()
		add(A, K, AA, BB, CC)
		T = append(T, pack(AA, BB, CC))
	}
	return T
}

/* Accumulate another set of line functions for n-pairing, assuming precomputation on G2 */
func Another_pc(r []*FP12, T []*FP4, QV *ECP) {
	n := NewBIG()
	n3 := NewBIG()
	var lv, lv2 *FP12
	var bt, j int

	if QV.Is_infinity() {
		return
	}

	Q := NewECP()
	Q.Copy(QV)
	Q.Affine()
	Qx := NewFPcopy(Q.getx())
	Qy := NewFPcopy(Q.gety())

	nb := lbits(n3, n)
	j = 0
	for i := nb - 2; i >= 1; i-- {
		lv = unpack(T[j], Qx, Qy)
		j += 1
		bt = n3.bit(i) - n.bit(i)
		if bt == 1 {
			lv2 = unpack(T[j], Qx, Qy)
			j += 1
			lv.smul(lv2)
		}
		if bt == -1 {
			lv2 = unpack(T[j], Qx, Qy)
			j += 1
			lv.smul(lv2)
		}
		r[i].ssmul(lv)
	}
	if CURVE_PAIRING_TYPE == BN {
		lv = unpack(T[j], Qx, Qy)
		j += 1
		lv2 = unpack(T[j], Qx, Qy)
		j += 1
		lv.smul(lv2)
		r[0].ssmul(lv)
	}
}

/* Accumulate another set of line functions for n-pairing */
func Another(r []*FP12, P1 *ECP2, Q1 *ECP) {
	f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))
	n := NewBIG()
	n3 := NewBIG()
	K := NewECP2()
	var lv, lv2 *FP12

	if Q1.Is_infinity() {
		return
	}

	// P is needed in affine form for line function, Q for (Qx,Qy) extraction
	P := NewECP2()
	P.Copy(P1)
	Q := NewECP()
	Q.Copy(Q1)

	P.Affine()
	Q.Affine()

	if CURVE_PAIRING_TYPE == BN {
		if SEXTIC_TWIST == M_TYPE {
			f.inverse(nil)
			f.norm()
		}
	}

	Qx := NewFPcopy(Q.getx())
	Qy := NewFPcopy(Q.gety())

	A := NewECP2()
	A.Copy(P)

	MP := NewECP2()
	MP.Copy(P)
	MP.neg()

	nb := lbits(n3, n)

	for i := nb - 2; i >= 1; i-- {
		lv = line(A, A, Qx, Qy)

		bt := n3.bit(i) - n.bit(i)
		if bt == 1 {
			lv2 = line(A, P, Qx, Qy)
			lv.smul(lv2)
		}
		if bt == -1 {
			lv2 = line(A, MP, Qx, Qy)
			lv.smul(lv2)
		}
		r[i].ssmul(lv)
	}

	/* R-ate fixup required for BN curves */
	if CURVE_PAIRING_TYPE == BN {
		if SIGN_OF_X == NEGATIVEX {
			A.neg()
		}
		K.Copy(P)
		K.frob(f)
		lv = line(A, K, Qx, Qy)
		K.frob(f)
		K.neg()
		lv2 = line(A, K, Qx, Qy)
		lv.smul(lv2)
		r[0].ssmul(lv)
	}
}

/* Optimal R-ate pairing */
func Ate(P1 *ECP2, Q1 *ECP) *FP12 {
	f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))
	n := NewBIG()
	n3 := NewBIG()
	K := NewECP2()
	var lv, lv2 *FP12

	if Q1.Is_infinity() {
		return NewFP12int(1)
	}

	if CURVE_PAIRING_TYPE == BN {
		if SEXTIC_TWIST == M_TYPE {
			f.inverse(nil)
			f.norm()
		}
	}

	P := NewECP2()
	P.Copy(P1)
	P.Affine()
	Q := NewECP()
	Q.Copy(Q1)
	Q.Affine()

	Qx := NewFPcopy(Q.getx())
	Qy := NewFPcopy(Q.gety())

	A := NewECP2()
	r := NewFP12int(1)

	A.Copy(P)

	NP := NewECP2()
	NP.Copy(P)
	NP.neg()

	nb := lbits(n3, n)

	for i := nb - 2; i >= 1; i-- {
		r.sqr()
		lv = line(A, A, Qx, Qy)
		bt := n3.bit(i) - n.bit(i)
		if bt == 1 {
			lv2 = line(A, P, Qx, Qy)
			lv.smul(lv2)
		}
		if bt == -1 {
			lv2 = line(A, NP, Qx, Qy)
			lv.smul(lv2)
		}
		r.ssmul(lv)
	}

	if SIGN_OF_X == NEGATIVEX {
		r.conj()
	}

	/* R-ate fixup required for BN curves */

	if CURVE_PAIRING_TYPE == BN {
		if SIGN_OF_X == NEGATIVEX {
			A.neg()
		}

		K.Copy(P)
		K.frob(f)
		lv = line(A, K, Qx, Qy)
		K.frob(f)
		K.neg()
		lv2 = line(A, K, Qx, Qy)
		lv.smul(lv2)
		r.ssmul(lv)
	}

	return r
}

/* Optimal R-ate double pairing e(P,Q).e(R,S) */
func Ate2(P1 *ECP2, Q1 *ECP, R1 *ECP2, S1 *ECP) *FP12 {
	f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))
	n := NewBIG()
	n3 := NewBIG()
	K := NewECP2()
	var lv, lv2 *FP12

	if Q1.Is_infinity() {
		return Ate(R1, S1)
	}
	if S1.Is_infinity() {
		return Ate(P1, Q1)
	}
	if CURVE_PAIRING_TYPE == BN {
		if SEXTIC_TWIST == M_TYPE {
			f.inverse(nil)
			f.norm()
		}
	}

	P := NewECP2()
	P.Copy(P1)
	P.Affine()
	Q := NewECP()
	Q.Copy(Q1)
	Q.Affine()
	R := NewECP2()
	R.Copy(R1)
	R.Affine()
	S := NewECP()
	S.Copy(S1)
	S.Affine()

	Qx := NewFPcopy(Q.getx())
	Qy := NewFPcopy(Q.gety())
	Sx := NewFPcopy(S.getx())
	Sy := NewFPcopy(S.gety())

	A := NewECP2()
	B := NewECP2()
	r := NewFP12int(1)

	A.Copy(P)
	B.Copy(R)
	NP := NewECP2()
	NP.Copy(P)
	NP.neg()
	NR := NewECP2()
	NR.Copy(R)
	NR.neg()

	nb := lbits(n3, n)

	for i := nb - 2; i >= 1; i-- {
		r.sqr()
		lv = line(A, A, Qx, Qy)
		lv2 = line(B, B, Sx, Sy)
		lv.smul(lv2)
		r.ssmul(lv)
		bt := n3.bit(i) - n.bit(i)
		if bt == 1 {
			lv = line(A, P, Qx, Qy)
			lv2 = line(B, R, Sx, Sy)
			lv.smul(lv2)
			r.ssmul(lv)
		}
		if bt == -1 {
			lv = line(A, NP, Qx, Qy)
			lv2 = line(B, NR, Sx, Sy)
			lv.smul(lv2)
			r.ssmul(lv)
		}
	}

	if SIGN_OF_X == NEGATIVEX {
		r.conj()
	}

	/* R-ate fixup */
	if CURVE_PAIRING_TYPE == BN {
		if SIGN_OF_X == NEGATIVEX {
			A.neg()
			B.neg()
		}
		K.Copy(P)
		K.frob(f)

		lv = line(A, K, Qx, Qy)
		K.frob(f)
		K.neg()
		lv2 = line(A, K, Qx, Qy)
		lv.smul(lv2)
		r.ssmul(lv)
		K.Copy(R)
		K.frob(f)
		lv = line(B, K, Sx, Sy)
		K.frob(f)
		K.neg()
		lv2 = line(B, K, Sx, Sy)
		lv.smul(lv2)
		r.ssmul(lv)
	}

	return r
}

/* final exponentiation - keep separate for multi-pairings and to avoid thrashing stack */
func Fexp(m *FP12) *FP12 {
	f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))
	x := NewBIGints(CURVE_Bnx)
	r := NewFP12copy(m)

	/* Easy part of final exp */
	lv := NewFP12copy(r)
	lv.Inverse()
	r.conj()

	r.Mul(lv)
	lv.Copy(r)
	r.frob(f)
	r.frob(f)
	r.Mul(lv)

	/* Hard part of final exp */
	if CURVE_PAIRING_TYPE == BN {
		lv.Copy(r)
		lv.frob(f)
		x0 := NewFP12copy(lv)
		x0.frob(f)
		lv.Mul(r)
		x0.Mul(lv)
		x0.frob(f)
		x1 := NewFP12copy(r)
		x1.conj()
		x4 := r.Pow(x)
		if SIGN_OF_X == POSITIVEX {
			x4.conj()
		}

		x3 := NewFP12copy(x4)
		x3.frob(f)

		x2 := x4.Pow(x)
		if SIGN_OF_X == POSITIVEX {
			x2.conj()
		}

		x5 := NewFP12copy(x2)
		x5.conj()
		lv = x2.Pow(x)
		if SIGN_OF_X == POSITIVEX {
			lv.conj()
		}

		x2.frob(f)
		r.Copy(x2)
		r.conj()

		x4.Mul(r)
		x2.frob(f)

		r.Copy(lv)
		r.frob(f)
		lv.Mul(r)

		lv.usqr()
		lv.Mul(x4)
		lv.Mul(x5)
		r.Copy(x3)
		r.Mul(x5)
		r.Mul(lv)
		lv.Mul(x2)
		r.usqr()
		r.Mul(lv)
		r.usqr()
		lv.Copy(r)
		lv.Mul(x1)
		r.Mul(x0)
		lv.usqr()
		r.Mul(lv)
		r.reduce()
	} else {

// See https://eprint.iacr.org/2020/875.pdf
		y1:=NewFP12copy(r)
		y1.usqr()
		y1.Mul(r) // y1=r^3

		y0:=NewFP12copy(r.Pow(x))
		if SIGN_OF_X == NEGATIVEX {
			y0.conj()
		}
		t0:=NewFP12copy(r); t0.conj()
		r.Copy(y0)
		r.Mul(t0)

		y0.Copy(r.Pow(x))
		if SIGN_OF_X == NEGATIVEX {
			y0.conj()
		}
		t0.Copy(r); t0.conj()
		r.Copy(y0)
		r.Mul(t0)

// ^(x+p)
		y0.Copy(r.Pow(x));
		if SIGN_OF_X == NEGATIVEX {
			y0.conj()
		}
		t0.Copy(r)
		t0.frob(f)
		r.Copy(y0)
		r.Mul(t0);

// ^(x^2+p^2-1)
		y0.Copy(r.Pow(x))
		y0.Copy(y0.Pow(x))
		t0.Copy(r)
		t0.frob(f); t0.frob(f)
		y0.Mul(t0)
		t0.Copy(r); t0.conj()
		r.Copy(y0)
		r.Mul(t0)

		r.Mul(y1)
		r.reduce();

/*
		// Ghamman & Fouotsa Method
		y0 := NewFP12copy(r)
		y0.usqr()
		y1 := y0.Pow(x)
		if SIGN_OF_X == NEGATIVEX {
			y1.conj()
		}

		x.fshr(1)
		y2 := y1.Pow(x)
		if SIGN_OF_X == NEGATIVEX {
			y2.conj()
		}

		x.fshl(1)
		y3 := NewFP12copy(r)
		y3.conj()
		y1.Mul(y3)

		y1.conj()
		y1.Mul(y2)

		y2 = y1.Pow(x)
		if SIGN_OF_X == NEGATIVEX {
			y2.conj()
		}

		y3 = y2.Pow(x)
		if SIGN_OF_X == NEGATIVEX {
			y3.conj()
		}

		y1.conj()
		y3.Mul(y1)

		y1.conj()
		y1.frob(f)
		y1.frob(f)
		y1.frob(f)
		y2.frob(f)
		y2.frob(f)
		y1.Mul(y2)

		y2 = y3.Pow(x)
		if SIGN_OF_X == NEGATIVEX {
			y2.conj()
		}

		y2.Mul(y0)
		y2.Mul(r)

		y1.Mul(y2)
		y2.Copy(y3)
		y2.frob(f)
		y1.Mul(y2)
		r.Copy(y1)
		r.reduce()
*/
	}
	return r
}

/* GLV method */
func glv(e *BIG) []*BIG {
	var u []*BIG
	if CURVE_PAIRING_TYPE == BN {
/* */
		t := NewBIGint(0)
		q := NewBIGints(CURVE_Order)
		var v []*BIG

		for i := 0; i < 2; i++ {
			t.copy(NewBIGints(CURVE_W[i])) // why not just t=new BIG(ROM.CURVE_W[i]);
			d := mul(t, e)
			v = append(v, NewBIGcopy(d.div(q)))
			u = append(u, NewBIGint(0))
		}
		u[0].copy(e)
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				t.copy(NewBIGints(CURVE_SB[j][i]))
				t.copy(Modmul(v[j], t, q))
				u[i].add(q)
				u[i].sub(t)
				u[i].Mod(q)
			}
		}
/* */
	} else {
		q := NewBIGints(CURVE_Order)
		x := NewBIGints(CURVE_Bnx)
		x2 := smul(x, x)
		u = append(u, NewBIGcopy(e))
		u[0].Mod(x2)
		u = append(u, NewBIGcopy(e))
		u[1].div(x2)
		u[1].rsub(q)
	}
	return u
}

/* Galbraith & Scott Method */
func gs(e *BIG) []*BIG {
	var u []*BIG
	if CURVE_PAIRING_TYPE == BN {
/* */
		t := NewBIGint(0)
		q := NewBIGints(CURVE_Order)

		var v []*BIG
		for i := 0; i < 4; i++ {
			t.copy(NewBIGints(CURVE_WB[i]))
			d := mul(t, e)
			v = append(v, NewBIGcopy(d.div(q)))
			u = append(u, NewBIGint(0))
		}
		u[0].copy(e)
		for i := 0; i < 4; i++ {
			for j := 0; j < 4; j++ {
				t.copy(NewBIGints(CURVE_BB[j][i]))
				t.copy(Modmul(v[j], t, q))
				u[i].add(q)
				u[i].sub(t)
				u[i].Mod(q)
			}
		}
/* */
	} else {
		q := NewBIGints(CURVE_Order)
		x := NewBIGints(CURVE_Bnx)
		w := NewBIGcopy(e)
		for i := 0; i < 3; i++ {
			u = append(u, NewBIGcopy(w))
			u[i].Mod(x)
			w.div(x)
		}
		u = append(u, NewBIGcopy(w))
		if SIGN_OF_X == NEGATIVEX {
			u[1].copy(Modneg(u[1], q))
			u[3].copy(Modneg(u[3], q))
		}
	}
	return u
}

/* Multiply P by e in group G1 */
func G1mul(P *ECP, e *BIG) *ECP {
	var R *ECP
	if USE_GLV {
		R = NewECP()
		R.Copy(P)
		Q := NewECP()
		Q.Copy(P)
		Q.Affine()
		q := NewBIGints(CURVE_Order)
		cru := NewFPbig(NewBIGints(CRu))
		t := NewBIGint(0)
		u := glv(e)
		Q.getx().mul(cru)

		np := u[0].nbits()
		t.copy(Modneg(u[0], q))
		nn := t.nbits()
		if nn < np {
			u[0].copy(t)
			R.Neg()
		}

		np = u[1].nbits()
		t.copy(Modneg(u[1], q))
		nn = t.nbits()
		if nn < np {
			u[1].copy(t)
			Q.Neg()
		}
		u[0].norm()
		u[1].norm()
		R = R.Mul2(u[0], Q, u[1])

	} else {
		R = P.mul(e)
	}
	return R
}

/* Multiply P by e in group G2 */
func G2mul(P *ECP2, e *BIG) *ECP2 {
	var R *ECP2
	if USE_GS_G2 {
		var Q []*ECP2
		f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))

		if SEXTIC_TWIST == M_TYPE {
			f.inverse(nil)
			f.norm()
		}

		q := NewBIGints(CURVE_Order)
		u := gs(e)

		t := NewBIGint(0)
		Q = append(Q, NewECP2())
		Q[0].Copy(P)
		for i := 1; i < 4; i++ {
			Q = append(Q, NewECP2())
			Q[i].Copy(Q[i-1])
			Q[i].frob(f)
		}
		for i := 0; i < 4; i++ {
			np := u[i].nbits()
			t.copy(Modneg(u[i], q))
			nn := t.nbits()
			if nn < np {
				u[i].copy(t)
				Q[i].neg()
			}
			u[i].norm()
		}

		R = mul4(Q, u)

	} else {
		R = P.mul(e)
	}
	return R
}

/* f=f^e */
/* Note that this method requires a lot of RAM! Better to use compressed XTR method, see FP4.java */
func GTpow(d *FP12, e *BIG) *FP12 {
	var r *FP12
	if USE_GS_GT {
		var g []*FP12
		f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))
		q := NewBIGints(CURVE_Order)
		t := NewBIGint(0)

		u := gs(e)

		g = append(g, NewFP12copy(d))
		for i := 1; i < 4; i++ {
			g = append(g, NewFP12())
			g[i].Copy(g[i-1])
			g[i].frob(f)
		}
		for i := 0; i < 4; i++ {
			np := u[i].nbits()
			t.copy(Modneg(u[i], q))
			nn := t.nbits()
			if nn < np {
				u[i].copy(t)
				g[i].conj()
			}
			u[i].norm()
		}
		r = pow4(g, u)
	} else {
		r = d.Pow(e)
	}
	return r
}

/* test G1 group membership */
	func G1member(P *ECP) bool {
		q := NewBIGints(CURVE_Order)
		if P.Is_infinity() {return false}
		W:=G1mul(P,q)
		if !W.Is_infinity() {return false}
		return true
	}

/* test G2 group membership */
	func G2member(P *ECP2) bool {
		q := NewBIGints(CURVE_Order)
		if P.Is_infinity() {return false}
		W:=G2mul(P,q)
		if !W.Is_infinity() {return false}
		return true
	}

/* test group membership - no longer needed*/
/* Check that m!=1, conj(m)*m==1, and m.m^{p^4}=m^{p^2} */

func GTmember(m *FP12) bool {
	if m.Isunity() {return false}
	r:=NewFP12copy(m)
	r.conj()
	r.Mul(m)
	if !r.Isunity() {return false}

	f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))

	r.Copy(m); r.frob(f); r.frob(f)
	w:=NewFP12copy(r); w.frob(f); w.frob(f)
	w.Mul(m)
	if !w.Equals(r) {return false}

	q := NewBIGints(CURVE_Order)
	w.Copy(m)
	r.Copy(GTpow(w,q))
	if !r.Isunity() {return false}
	return true
}

