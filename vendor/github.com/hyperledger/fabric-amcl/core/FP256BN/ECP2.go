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

/* MiotCL Weierstrass elliptic curve functions over FP2 */

package FP256BN



type ECP2 struct {
	x *FP2
	y *FP2
	z *FP2
}

func NewECP2() *ECP2 {
	E := new(ECP2)
	E.x = NewFP2()
	E.y = NewFP2int(1)
	E.z = NewFP2()
	return E
}

/* Test this=O? */
func (E *ECP2) Is_infinity() bool {
	return E.x.iszilch() && E.z.iszilch()
}

/* copy this=P */
func (E *ECP2) Copy(P *ECP2) {
	E.x.copy(P.x)
	E.y.copy(P.y)
	E.z.copy(P.z)
}

/* set this=O */
func (E *ECP2) inf() {
	E.x.zero()
	E.y.one()
	E.z.zero()
}

/* set this=-this */
func (E *ECP2) neg() {
	E.y.norm()
	E.y.neg()
	E.y.norm()
}

/* Conditional move of Q to P dependant on d */
func (E *ECP2) cmove(Q *ECP2, d int) {
	E.x.cmove(Q.x, d)
	E.y.cmove(Q.y, d)
	E.z.cmove(Q.z, d)
}

/* Constant time select from pre-computed table */
func (E *ECP2) selector(W []*ECP2, b int32) {
	MP := NewECP2()
	m := b >> 31
	babs := (b ^ m) - m

	babs = (babs - 1) / 2

	E.cmove(W[0], teq(babs, 0)) // conditional move
	E.cmove(W[1], teq(babs, 1))
	E.cmove(W[2], teq(babs, 2))
	E.cmove(W[3], teq(babs, 3))
	E.cmove(W[4], teq(babs, 4))
	E.cmove(W[5], teq(babs, 5))
	E.cmove(W[6], teq(babs, 6))
	E.cmove(W[7], teq(babs, 7))

	MP.Copy(E)
	MP.neg()
	E.cmove(MP, int(m&1))
}

/* Test if P == Q */
func (E *ECP2) Equals(Q *ECP2) bool {

	a := NewFP2copy(E.x)
	b := NewFP2copy(Q.x)
	a.mul(Q.z)
	b.mul(E.z)

	if !a.Equals(b) {
		return false
	}
	a.copy(E.y)
	b.copy(Q.y)
	a.mul(Q.z)
	b.mul(E.z)
	if !a.Equals(b) {
		return false
	}

	return true
}

/* set to Affine - (x,y,z) to (x,y) */
func (E *ECP2) Affine() {
	if E.Is_infinity() {
		return
	}
	one := NewFP2int(1)
	if E.z.Equals(one) {
		E.x.reduce()
		E.y.reduce()
		return
	}
	E.z.inverse(nil)

	E.x.mul(E.z)
	E.x.reduce()
	E.y.mul(E.z)
	E.y.reduce()
	E.z.copy(one)
}

/* extract affine x as FP2 */
func (E *ECP2) GetX() *FP2 {
	W := NewECP2()
	W.Copy(E)
	W.Affine()
	return W.x
}

/* extract affine y as FP2 */
func (E *ECP2) GetY() *FP2 {
	W := NewECP2()
	W.Copy(E)
	W.Affine()
	return W.y
}

/* extract projective x */
func (E *ECP2) getx() *FP2 {
	return E.x
}

/* extract projective y */
func (E *ECP2) gety() *FP2 {
	return E.y
}

/* extract projective z */
func (E *ECP2) getz() *FP2 {
	return E.z
}

/* convert to byte array */
func (E *ECP2) ToBytes(b []byte,compress bool) {
	var t [2*int(MODBYTES)]byte
	MB := 2*int(MODBYTES)
	alt:=false
	W := NewECP2()
	W.Copy(E)
	W.Affine()
	W.x.ToBytes(t[:])

	if (MODBITS-1)%8 <= 4 && ALLOW_ALT_COMPRESS {
		alt=true
	}

    if alt {
		for i:=0;i<MB;i++ {
			b[i]=t[i]
		}
        if !compress {
            W.y.ToBytes(t[:]);
            for i:=0;i<MB;i++ {
				b[i+MB]=t[i]
			}
        } else {
            b[0]|=0x80
            if W.y.islarger()==1 {
				b[0]|=0x20
			}
        }
		
	} else {
		for i:=0;i<MB;i++ {
			b[i+1]=t[i]
		}
        if !compress {
            b[0]=0x04
            W.y.ToBytes(t[:])
	        for i:=0;i<MB;i++ {
			    b[i+MB+1]=t[i]
			}
        } else {
            b[0]=0x02
            if W.y.sign() == 1 {
                b[0]=0x03
			}
        }
	}
}

/* convert from byte array to point */
func ECP2_fromBytes(b []byte) *ECP2 {
	var t [2*int(MODBYTES)]byte
	MB := 2*int(MODBYTES)
	typ := int(b[0])
	alt:=false

	if (MODBITS-1)%8 <= 4 && ALLOW_ALT_COMPRESS {
		alt=true
	}

	if alt {
        for i:=0;i<MB;i++  {
			t[i]=b[i]
		}
        t[0]&=0x1f
        rx:=FP2_fromBytes(t[:]);
        if (b[0]&0x80)==0 {
            for i:=0;i<MB;i++ {
				t[i]=b[i+MB]
			}
            ry:=FP2_fromBytes(t[:])
            return NewECP2fp2s(rx,ry)
        } else {
            sgn:=(b[0]&0x20)>>5
            P:=NewECP2fp2(rx,0)
            cmp:=P.y.islarger()
            if (sgn==1 && cmp!=1) || (sgn==0 && cmp==1) {
				P.neg()
			}
            return P;
        }
    } else {
		for i:=0;i<MB;i++ {
			t[i]=b[i+1]
		}
        rx:=FP2_fromBytes(t[:])
        if typ == 0x04 {
		    for i:=0;i<MB;i++ {
				t[i]=b[i+MB+1]
			}
		    ry:=FP2_fromBytes(t[:])
		    return NewECP2fp2s(rx,ry)
        } else {
            return NewECP2fp2(rx,typ&1)
        }
    }
}

/* convert this to hex string */
func (E *ECP2) ToString() string {
	W := NewECP2()
	W.Copy(E)
	W.Affine()
	if W.Is_infinity() {
		return "infinity"
	}
	return "(" + W.x.toString() + "," + W.y.toString() + ")"
}

/* Calculate RHS of twisted curve equation x^3+B/i */
func RHS2(x *FP2) *FP2 {
	r := NewFP2copy(x)
	r.sqr()
	b := NewFP2big(NewBIGints(CURVE_B))

	if SEXTIC_TWIST == D_TYPE {
		b.div_ip()
	}
	if SEXTIC_TWIST == M_TYPE {
		b.norm()
		b.mul_ip()
		b.norm()
	}
	r.mul(x)
	r.add(b)

	r.reduce()
	return r
}

/* construct this from (x,y) - but set to O if not on curve */
func NewECP2fp2s(ix *FP2, iy *FP2) *ECP2 {
	E := new(ECP2)
	E.x = NewFP2copy(ix)
	E.y = NewFP2copy(iy)
	E.z = NewFP2int(1)
	E.x.norm()
	rhs := RHS2(E.x)
	y2 := NewFP2copy(E.y)
	y2.sqr()
	if !y2.Equals(rhs) {
		E.inf()
	}
	return E
}

/* construct this from x - but set to O if not on curve */
func NewECP2fp2(ix *FP2, s int) *ECP2 {
	E := new(ECP2)
	h:=NewFP()
	E.x = NewFP2copy(ix)
	E.y = NewFP2int(1)
	E.z = NewFP2int(1)
	E.x.norm()
	rhs := RHS2(E.x)
	if rhs.qr(h) == 1 {
		rhs.sqrt(h)
		if rhs.sign() != s {
			rhs.neg()
		}
		rhs.reduce()
		E.y.copy(rhs)

	} else {
		E.inf()
	}
	return E
}

/* this+=this */
func (E *ECP2) dbl() int {
	iy := NewFP2copy(E.y)
	if SEXTIC_TWIST == D_TYPE {
		iy.mul_ip()
		iy.norm()
	}

	t0 := NewFP2copy(E.y) //***** Change
	t0.sqr()
	if SEXTIC_TWIST == D_TYPE {
		t0.mul_ip()
	}
	t1 := NewFP2copy(iy)
	t1.mul(E.z)
	t2 := NewFP2copy(E.z)
	t2.sqr() // z^2

	E.z.copy(t0) // y^2
	E.z.add(t0)
	E.z.norm() // 2y^2
	E.z.add(E.z)
	E.z.add(E.z) // 8y^2
	E.z.norm()

	t2.imul(3 * CURVE_B_I) // 3bz^2
	if SEXTIC_TWIST == M_TYPE {
		t2.mul_ip()
		t2.norm()
	}
	x3 := NewFP2copy(t2)
	x3.mul(E.z)

	y3 := NewFP2copy(t0)

	y3.add(t2)
	y3.norm()
	E.z.mul(t1)
	t1.copy(t2)
	t1.add(t2)
	t2.add(t1)
	t2.norm()
	t0.sub(t2)
	t0.norm() //y^2-9bz^2
	y3.mul(t0)
	y3.add(x3) //(y^2+3z*2)(y^2-9z^2)+3b.z^2.8y^2
	t1.copy(E.x)
	t1.mul(iy) //
	E.x.copy(t0)
	E.x.norm()
	E.x.mul(t1)
	E.x.add(E.x) //(y^2-9bz^2)xy2

	E.x.norm()
	E.y.copy(y3)
	E.y.norm()

	return 1
}

/* this+=Q - return 0 for add, 1 for double, -1 for O */
func (E *ECP2) Add(Q *ECP2) int {
	b := 3 * CURVE_B_I
	t0 := NewFP2copy(E.x)
	t0.mul(Q.x) // x.Q.x
	t1 := NewFP2copy(E.y)
	t1.mul(Q.y) // y.Q.y

	t2 := NewFP2copy(E.z)
	t2.mul(Q.z)
	t3 := NewFP2copy(E.x)
	t3.add(E.y)
	t3.norm() //t3=X1+Y1
	t4 := NewFP2copy(Q.x)
	t4.add(Q.y)
	t4.norm()  //t4=X2+Y2
	t3.mul(t4) //t3=(X1+Y1)(X2+Y2)
	t4.copy(t0)
	t4.add(t1) //t4=X1.X2+Y1.Y2

	t3.sub(t4)
	t3.norm()
	if SEXTIC_TWIST == D_TYPE {
		t3.mul_ip()
		t3.norm() //t3=(X1+Y1)(X2+Y2)-(X1.X2+Y1.Y2) = X1.Y2+X2.Y1
	}
	t4.copy(E.y)
	t4.add(E.z)
	t4.norm() //t4=Y1+Z1
	x3 := NewFP2copy(Q.y)
	x3.add(Q.z)
	x3.norm() //x3=Y2+Z2

	t4.mul(x3)  //t4=(Y1+Z1)(Y2+Z2)
	x3.copy(t1) //
	x3.add(t2)  //X3=Y1.Y2+Z1.Z2

	t4.sub(x3)
	t4.norm()
	if SEXTIC_TWIST == D_TYPE {
		t4.mul_ip()
		t4.norm() //t4=(Y1+Z1)(Y2+Z2) - (Y1.Y2+Z1.Z2) = Y1.Z2+Y2.Z1
	}
	x3.copy(E.x)
	x3.add(E.z)
	x3.norm() // x3=X1+Z1
	y3 := NewFP2copy(Q.x)
	y3.add(Q.z)
	y3.norm()  // y3=X2+Z2
	x3.mul(y3) // x3=(X1+Z1)(X2+Z2)
	y3.copy(t0)
	y3.add(t2) // y3=X1.X2+Z1+Z2
	y3.rsub(x3)
	y3.norm() // y3=(X1+Z1)(X2+Z2) - (X1.X2+Z1.Z2) = X1.Z2+X2.Z1

	if SEXTIC_TWIST == D_TYPE {
		t0.mul_ip()
		t0.norm() // x.Q.x
		t1.mul_ip()
		t1.norm() // y.Q.y
	}
	x3.copy(t0)
	x3.add(t0)
	t0.add(x3)
	t0.norm()
	t2.imul(b)
	if SEXTIC_TWIST == M_TYPE {
		t2.mul_ip()
		t2.norm()
	}
	z3 := NewFP2copy(t1)
	z3.add(t2)
	z3.norm()
	t1.sub(t2)
	t1.norm()
	y3.imul(b)
	if SEXTIC_TWIST == M_TYPE {
		y3.mul_ip()
		y3.norm()
	}
	x3.copy(y3)
	x3.mul(t4)
	t2.copy(t3)
	t2.mul(t1)
	x3.rsub(t2)
	y3.mul(t0)
	t1.mul(z3)
	y3.add(t1)
	t0.mul(t3)
	z3.mul(t4)
	z3.add(t0)

	E.x.copy(x3)
	E.x.norm()
	E.y.copy(y3)
	E.y.norm()
	E.z.copy(z3)
	E.z.norm()

	return 0
}

/* set this-=Q */
func (E *ECP2) Sub(Q *ECP2) int {
	NQ := NewECP2()
	NQ.Copy(Q)
	NQ.neg()
	D := E.Add(NQ)
	return D
}

/* set this*=q, where q is Modulus, using Frobenius */
func (E *ECP2) frob(X *FP2) {
	X2 := NewFP2copy(X)
	X2.sqr()
	E.x.conj()
	E.y.conj()
	E.z.conj()
	E.z.reduce()
	E.x.mul(X2)
	E.y.mul(X2)
	E.y.mul(X)
}

/* P*=e */
func (E *ECP2) mul(e *BIG) *ECP2 {
	/* fixed size windows */
	mt := NewBIG()
	t := NewBIG()
	P := NewECP2()
	Q := NewECP2()
	C := NewECP2()

	if E.Is_infinity() {
		return NewECP2()
	}

	var W []*ECP2
	var w [1 + (NLEN*int(BASEBITS)+3)/4]int8

	/* precompute table */
	Q.Copy(E)
	Q.dbl()

	W = append(W, NewECP2())
	W[0].Copy(E)

	for i := 1; i < 8; i++ {
		W = append(W, NewECP2())
		W[i].Copy(W[i-1])
		W[i].Add(Q)
	}

	/* make exponent odd - add 2P if even, P if odd */
	t.copy(e)
	s := int(t.parity())
	t.inc(1)
	t.norm()
	ns := int(t.parity())
	mt.copy(t)
	mt.inc(1)
	mt.norm()
	t.cmove(mt, s)
	Q.cmove(E, ns)
	C.Copy(Q)

	nb := 1 + (t.nbits()+3)/4
	/* convert exponent to signed 4-bit window */
	for i := 0; i < nb; i++ {
		w[i] = int8(t.lastbits(5) - 16)
		t.dec(int(w[i]))
		t.norm()
		t.fshr(4)
	}
	w[nb] = int8(t.lastbits(5))

	P.Copy(W[(w[nb]-1)/2])
	for i := nb - 1; i >= 0; i-- {
		Q.selector(W, int32(w[i]))
		P.dbl()
		P.dbl()
		P.dbl()
		P.dbl()
		P.Add(Q)
	}
	P.Sub(C)
	return P
}

/* Public version */
func (E *ECP2) Mul(e *BIG) *ECP2 {
	return E.mul(e)
}


/* clear cofactor */
func (E *ECP2) Cfp() {
	var T, K, xQ, x2Q *ECP2
	/* Fast Hashing to G2 - Fuentes-Castaneda, Knapp and Rodriguez-Henriquez */
	Fra := NewBIGints(Fra)
	Frb := NewBIGints(Frb)
	X := NewFP2bigs(Fra, Frb)
	if SEXTIC_TWIST == M_TYPE {
		X.inverse(nil)
		X.norm()
	}

	x := NewBIGints(CURVE_Bnx)

	if CURVE_PAIRING_TYPE == BN {
		T = NewECP2()
		T.Copy(E)
		T = T.mul(x)
		if SIGN_OF_X == NEGATIVEX {
			T.neg()
		}

		K = NewECP2()
		K.Copy(T)
		K.dbl()
		K.Add(T)

		K.frob(X)
		E.frob(X)
		E.frob(X)
		E.frob(X)
		E.Add(T)
		E.Add(K)
		T.frob(X)
		T.frob(X)
		E.Add(T)
	}
	if CURVE_PAIRING_TYPE > BN {
		xQ = E.mul(x)
		x2Q = xQ.mul(x)

		if SIGN_OF_X == NEGATIVEX {
			xQ.neg()
		}

		x2Q.Sub(xQ)
		x2Q.Sub(E)

		xQ.Sub(E)
		xQ.frob(X)

		E.dbl()
		E.frob(X)
		E.frob(X)

		E.Add(x2Q)
		E.Add(xQ)
	}
}


/* P=u0.Q0+u1*Q1+u2*Q2+u3*Q3 */
// Bos & Costello https://eprint.iacr.org/2013/458.pdf
// Faz-Hernandez & Longa & Sanchez  https://eprint.iacr.org/2013/158.pdf
// Side channel attack secure
func mul4(Q []*ECP2, u []*BIG) *ECP2 {
	W := NewECP2()
	P := NewECP2()
	var T []*ECP2
	mt := NewBIG()
	var t []*BIG

	var w [NLEN*int(BASEBITS) + 1]int8
	var s [NLEN*int(BASEBITS) + 1]int8

	for i := 0; i < 4; i++ {
		t = append(t, NewBIGcopy(u[i]))
	}

	T = append(T, NewECP2())
	T[0].Copy(Q[0]) // Q[0]
	T = append(T, NewECP2())
	T[1].Copy(T[0])
	T[1].Add(Q[1]) // Q[0]+Q[1]
	T = append(T, NewECP2())
	T[2].Copy(T[0])
	T[2].Add(Q[2]) // Q[0]+Q[2]
	T = append(T, NewECP2())
	T[3].Copy(T[1])
	T[3].Add(Q[2]) // Q[0]+Q[1]+Q[2]
	T = append(T, NewECP2())
	T[4].Copy(T[0])
	T[4].Add(Q[3]) // Q[0]+Q[3]
	T = append(T, NewECP2())
	T[5].Copy(T[1])
	T[5].Add(Q[3]) // Q[0]+Q[1]+Q[3]
	T = append(T, NewECP2())
	T[6].Copy(T[2])
	T[6].Add(Q[3]) // Q[0]+Q[2]+Q[3]
	T = append(T, NewECP2())
	T[7].Copy(T[3])
	T[7].Add(Q[3]) // Q[0]+Q[1]+Q[2]+Q[3]

	// Make it odd
	pb := 1 - t[0].parity()
	t[0].inc(pb)

	// Number of bits
	mt.zero()
	for i := 0; i < 4; i++ {
		t[i].norm()
		mt.or(t[i])
	}

	nb := 1 + mt.nbits()

	// Sign pivot
	s[nb-1] = 1
	for i := 0; i < nb-1; i++ {
		t[0].fshr(1)
		s[i] = 2*int8(t[0].parity()) - 1
	}

	// Recoded exponent
	for i := 0; i < nb; i++ {
		w[i] = 0
		k := 1
		for j := 1; j < 4; j++ {
			bt := s[i] * int8(t[j].parity())
			t[j].fshr(1)
			t[j].dec(int(bt) >> 1)
			t[j].norm()
			w[i] += bt * int8(k)
			k *= 2
		}
	}

	// Main loop
	P.selector(T, int32(2*w[nb-1]+1))
	for i := nb - 2; i >= 0; i-- {
		P.dbl()
		W.selector(T, int32(2*w[i]+s[i]))
		P.Add(W)
	}

	// apply correction
	W.Copy(P)
	W.Sub(Q[0])
	P.cmove(W, pb)

	return P
}

/* Hunt and Peck a BIG to a curve point */
func ECP2_hap2point(h *BIG) *ECP2 {
	x := NewBIGcopy(h)
	one := NewBIGint(1)
	var X *FP2
	var Q *ECP2
	for true {
		X = NewFP2bigs(one, x)
		Q = NewECP2fp2(X,0)
		if !Q.Is_infinity() {
			break
		}
		x.inc(1)
		x.norm()
	}
	return Q
}

/* Constant time Map to Point */
 func ECP2_map2point(H *FP2) *ECP2 {
    // Shallue and van de Woestijne
	var Q *ECP2
    NY:=NewFP2int(1)
    T:=NewFP2copy(H) /**/
	sgn:=T.sign() /**/
	if HTC_ISO_G2 == 0 {
/* */
		Z:=NewFPint(RIADZG2A);
		X1:=NewFP2fp(Z)
		X3:=NewFP2copy(X1)
		A:=RHS2(X1)
		W:=NewFP2copy(A)
		if (RIADZG2A==-1 && RIADZG2B==0 && SEXTIC_TWIST==M_TYPE && CURVE_B_I==4) { // special case for BLS12381
			W.copy(NewFP2ints(2,1))
		} else {
			W.sqrt(nil)
		}
		s:=NewFPbig(NewBIGints(SQRTm3))
		Z.mul(s)

		T.sqr()
		Y:=NewFP2copy(A); Y.mul(T)
		T.copy(NY); T.add(Y); T.norm()
		Y.rsub(NY); Y.norm()
		NY.copy(T); NY.mul(Y); 
	
		NY.pmul(Z)
		NY.inverse(nil)

		W.pmul(Z)
		if (W.sign()==1) {
			W.neg()
			W.norm()
		}
		W.pmul(Z)
		W.mul(H); W.mul(Y); W.mul(NY)

		X1.neg(); X1.norm(); X1.div2()
		X2:=NewFP2copy(X1)
		X1.sub(W); X1.norm()
		X2.add(W); X2.norm()
		A.add(A); A.add(A); A.norm()
		T.sqr(); T.mul(NY); T.sqr()
		A.mul(T)
		X3.add(A); X3.norm()

		Y.copy(RHS2(X2))
		X3.cmove(X2,Y.qr(nil))
		Y.copy(RHS2(X1))
		X3.cmove(X1,Y.qr(nil))
		Y.copy(RHS2(X3))
		Y.sqrt(nil)

		ne:=Y.sign()^sgn
		W.copy(Y); W.neg(); W.norm()
		Y.cmove(W,ne)

		Q=NewECP2fp2s(X3,Y)
/* */
	} else {

/* CAHCZS
		Q=NewECP2()
		Ad:=NewFP2bigs(NewBIGints(CURVE_Adr), NewBIGints(CURVE_Adi))
		Bd:=NewFP2bigs(NewBIGints(CURVE_Bdr), NewBIGints(CURVE_Bdi))
		ZZ:=NewFP2ints(RIADZG2A,RIADZG2B)
		hint:=NewFP()

		T.sqr()
		T.mul(ZZ)
		W:=NewFP2copy(T)
		W.add(NY); W.norm()

		W.mul(T)
		D:=NewFP2copy(Ad)
		D.mul(W)

		W.add(NY); W.norm()
		W.mul(Bd); 
		W.neg(); W.norm()

		X2:=NewFP2copy(W)
		X3:=NewFP2copy(T)
		X3.mul(X2)

		GX1:=NewFP2copy(X2); GX1.sqr()
		D2:=NewFP2copy(D); D2.sqr()

        W.copy(Ad); W.mul(D2); GX1.add(W); GX1.norm(); GX1.mul(X2); D2.mul(D); W.copy(Bd); W.mul(D2); GX1.add(W); GX1.norm() // x^3+Ax+b

        W.copy(GX1); W.mul(D)
        qr:=W.qr(hint)
        D.copy(W); D.inverse(hint)
        D.mul(GX1)
        X2.mul(D)
        X3.mul(D)
        T.mul(H)
        D2.copy(D); D2.sqr()

        D.copy(D2); D.mul(T)
        T.copy(W); T.mul(ZZ)

		s:=NewFPbig(NewBIGints(CURVE_HTPC2))
        s.mul(hint);

        X2.cmove(X3,1-qr)
        W.cmove(T,1-qr)
        D2.cmove(D,1-qr)
        hint.cmove(s,1-qr)

        Y:=NewFP2copy(W); Y.sqrt(hint)
        Y.mul(D2)

		ne:=Y.sign()^sgn
		W.copy(Y); W.neg(); W.norm()
		Y.cmove(W,ne)

		k:=0
		isox:=HTC_ISO_G2
		isoy:=3*(isox-1)/2

//xnum
		xnum:=NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k])); k+=1
		for i:=0;i<isox;i++ {
			xnum.mul(X2)
			xnum.add(NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k]))); k+=1
			xnum.norm();
		}
//xden
		xden:=NewFP2copy(X2)
		xden.add(NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k]))); k+=1
		xden.norm();
		for i:=0;i<isox-2;i++ {
			xden.mul(X2)
			xden.add(NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k]))); k+=1
			xden.norm();
		}		
//ynum
		ynum:=NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k])); k+=1
		for i:=0;i<isoy;i++ {
			ynum.mul(X2)
			ynum.add(NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k]))); k+=1
			ynum.norm();
		}
//yden
		yden:=NewFP2copy(X2)
		yden.add(NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k]))); k+=1
		yden.norm();
		for i:=0;i<isoy-1;i++ {
			yden.mul(X2)
			yden.add(NewFP2bigs(NewBIGints(PCR[k]),NewBIGints(PCI[k]))); k+=1
			yden.norm();
		}	
		ynum.mul(Y)
		T.copy(xnum); T.mul(yden)
		Q.x.copy(T)
		T.copy(ynum); T.mul(xden)
		Q.y.copy(T)
		T.copy(xden); T.mul(yden)
		Q.z.copy(T)

CAHCZF */

	}
	return Q
}

/* Map octet string to curve point */
func ECP2_mapit(h []byte) *ECP2 {
	q := NewBIGints(Modulus)
	dx:=DBIG_fromBytes(h);
    x:=dx.Mod(q);

	Q:=ECP2_hap2point(x)
	Q.Cfp()
    return Q
}

func ECP2_generator() *ECP2 {
	var G *ECP2
	G = NewECP2fp2s(NewFP2bigs(NewBIGints(CURVE_Pxa), NewBIGints(CURVE_Pxb)), NewFP2bigs(NewBIGints(CURVE_Pya), NewBIGints(CURVE_Pyb)))
	return G
}
