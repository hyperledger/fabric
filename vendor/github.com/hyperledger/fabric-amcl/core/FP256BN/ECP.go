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

package FP256BN

/* Elliptic Curve Point Structure */

type ECP struct {
	x *FP
	y *FP
	z *FP
}

/* Constructors */
func NewECP() *ECP {
	E := new(ECP)
	E.x = NewFP()
	E.y = NewFPint(1)
	if CURVETYPE == EDWARDS {
		E.z = NewFPint(1)
	} else {
		E.z = NewFP()
	}
	return E
}

/* set (x,y) from two BIGs */
func NewECPbigs(ix *BIG, iy *BIG) *ECP {
	E := new(ECP)
	E.x = NewFPbig(ix)
	E.y = NewFPbig(iy)
	E.z = NewFPint(1)
	E.x.norm()
	rhs := RHS(E.x)

	if CURVETYPE == MONTGOMERY {
		if rhs.qr(nil) != 1 {
			E.inf()
		}
	} else {
		y2 := NewFPcopy(E.y)
		y2.sqr()
		if !y2.Equals(rhs) {
			E.inf()
		}
	}
	return E
}

/* set (x,y) from BIG and a bit */
func NewECPbigint(ix *BIG, s int) *ECP {
	E := new(ECP)
	E.x = NewFPbig(ix)
	E.y = NewFP()
	E.x.norm()
	rhs := RHS(E.x)
	E.z = NewFPint(1)
	hint := NewFP()
	if rhs.qr(hint) == 1 {
		ny := rhs.sqrt(hint)
		if ny.sign() != s {
			ny.neg()
			ny.norm()
		}
		E.y.copy(ny)
	} else {
		E.inf()
	}
	return E
}

/* set from x - calculate y from curve equation */
func NewECPbig(ix *BIG) *ECP {
	E := new(ECP)
	E.x = NewFPbig(ix)
	E.y = NewFP()
	E.x.norm()
	rhs := RHS(E.x)
	E.z = NewFPint(1)
	hint := NewFP()
	if rhs.qr(hint) == 1 {
		if CURVETYPE != MONTGOMERY {
			E.y.copy(rhs.sqrt(hint))
		}
	} else {
		E.inf()
	}
	return E
}

/* test for O point-at-infinity */
func (E *ECP) Is_infinity() bool {
	//	if E.INF {return true}

	if CURVETYPE == EDWARDS {
		return (E.x.iszilch() && E.y.Equals(E.z))
	}
	if CURVETYPE == WEIERSTRASS {
		return (E.x.iszilch() && E.z.iszilch())
	}
	if CURVETYPE == MONTGOMERY {
		return E.z.iszilch()
	}
	return true
}

/* Conditional swap of P and Q dependant on d */
func (E *ECP) cswap(Q *ECP, d int) {
	E.x.cswap(Q.x, d)
	if CURVETYPE != MONTGOMERY {
		E.y.cswap(Q.y, d)
	}
	E.z.cswap(Q.z, d)
}

/* Conditional move of Q to P dependant on d */
func (E *ECP) cmove(Q *ECP, d int) {
	E.x.cmove(Q.x, d)
	if CURVETYPE != MONTGOMERY {
		E.y.cmove(Q.y, d)
	}
	E.z.cmove(Q.z, d)
}

/* return 1 if b==c, no branching */
func teq(b int32, c int32) int {
	x := b ^ c
	x -= 1 // if x=0, x now -1
	return int((x >> 31) & 1)
}

/* this=P */
func (E *ECP) Copy(P *ECP) {
	E.x.copy(P.x)
	if CURVETYPE != MONTGOMERY {
		E.y.copy(P.y)
	}
	E.z.copy(P.z)
}

/* this=-this */
func (E *ECP) Neg() {
	if CURVETYPE == WEIERSTRASS {
		E.y.neg()
		E.y.norm()
	}
	if CURVETYPE == EDWARDS {
		E.x.neg()
		E.x.norm()
	}
	return
}

/* Constant time select from pre-computed table */
func (E *ECP) selector(W []*ECP, b int32) {
	MP := NewECP()
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
	MP.Neg()
	E.cmove(MP, int(m&1))
}

/* set this=O */
func (E *ECP) inf() {
	E.x.zero()
	if CURVETYPE != MONTGOMERY {
		E.y.one()
	}
	if CURVETYPE != EDWARDS {
		E.z.zero()
	} else {
		E.z.one()
	}
}

/* Test P == Q */
func (E *ECP) Equals(Q *ECP) bool {
	a := NewFP()
	b := NewFP()
	a.copy(E.x)
	a.mul(Q.z)
	a.reduce()
	b.copy(Q.x)
	b.mul(E.z)
	b.reduce()
	if !a.Equals(b) {
		return false
	}
	if CURVETYPE != MONTGOMERY {
		a.copy(E.y)
		a.mul(Q.z)
		a.reduce()
		b.copy(Q.y)
		b.mul(E.z)
		b.reduce()
		if !a.Equals(b) {
			return false
		}
	}

	return true
}

/* Calculate RHS of curve equation */
func RHS(x *FP) *FP {
	r := NewFPcopy(x)
	r.sqr()

	if CURVETYPE == WEIERSTRASS { // x^3+Ax+B
		b := NewFPbig(NewBIGints(CURVE_B))
		r.mul(x)
		if CURVE_A == -3 {
			cx := NewFPcopy(x)
			cx.imul(3)
			cx.neg()
			cx.norm()
			r.add(cx)
		}
		r.add(b)
	}
	if CURVETYPE == EDWARDS { // (Ax^2-1)/(Bx^2-1)
		b := NewFPbig(NewBIGints(CURVE_B))

		one := NewFPint(1)
		b.mul(r)
		b.sub(one)
		b.norm()
		if CURVE_A == -1 {
			r.neg()
		}
		r.sub(one)
		r.norm()
		b.inverse(nil)
		r.mul(b)
	}
	if CURVETYPE == MONTGOMERY { // x^3+Ax^2+x
		x3 := NewFP()
		x3.copy(r)
		x3.mul(x)
		r.imul(CURVE_A)
		r.add(x3)
		r.add(x)
	}
	r.reduce()
	return r
}

/* set to affine - from (x,y,z) to (x,y) */
func (E *ECP) Affine() {
	if E.Is_infinity() {
		return
	}
	one := NewFPint(1)
	if E.z.Equals(one) {
		return
	}
	E.z.inverse(nil)
	E.x.mul(E.z)
	E.x.reduce()

	if CURVETYPE != MONTGOMERY {
		E.y.mul(E.z)
		E.y.reduce()
	}
	E.z.copy(one)
}

/* extract x as a BIG */
func (E *ECP) GetX() *BIG {
	W := NewECP()
	W.Copy(E)
	W.Affine()
	return W.x.redc()
}

/* extract y as a BIG */
func (E *ECP) GetY() *BIG {
	W := NewECP()
	W.Copy(E)
	W.Affine()
	return W.y.redc()
}

/* get sign of Y */
func (E *ECP) GetS() int {
	W := NewECP()
	W.Copy(E)
	W.Affine()
	return W.y.sign()
}

/* extract x as an FP */
func (E *ECP) getx() *FP {
	return E.x
}

/* extract y as an FP */
func (E *ECP) gety() *FP {
	return E.y
}

/* extract z as an FP */
func (E *ECP) getz() *FP {
	return E.z
}

/* convert to byte array */
func (E *ECP) ToBytes(b []byte, compress bool) {
	var t [int(MODBYTES)]byte
	MB := int(MODBYTES)
	alt:=false
	W := NewECP()
	W.Copy(E)
	W.Affine()
	W.x.redc().ToBytes(t[:])

	if CURVETYPE == MONTGOMERY {
		for i := 0; i < MB; i++ {
			b[i] = t[i]
		}
		//b[0] = 0x06
		return
	}

	if (MODBITS-1)%8 <= 4 && ALLOW_ALT_COMPRESS {
		alt=true
	}

	if alt {
        for i:=0;i<MB;i++ {
			b[i]=t[i]
		}
        if compress {
            b[0]|=0x80
            if W.y.islarger()==1 {
				b[0]|=0x20
			}
        } else {
            W.y.redc().ToBytes(t[:]);
            for i:=0;i<MB;i++ {
				b[i+MB]=t[i]
			}
		}		
	} else {
		for i := 0; i < MB; i++ {
			b[i+1] = t[i]
		}
		if compress {
			b[0] = 0x02
			if W.y.sign() == 1 {
				b[0] = 0x03
			}
			return
		}
		b[0] = 0x04
		W.y.redc().ToBytes(t[:])
		for i := 0; i < MB; i++ {
			b[i+MB+1] = t[i]
		}
	}
}

/* convert from byte array to point */
func ECP_fromBytes(b []byte) *ECP {
	var t [int(MODBYTES)]byte
	MB := int(MODBYTES)
	p := NewBIGints(Modulus)
	alt:=false

	if CURVETYPE == MONTGOMERY {
		for i := 0; i < MB; i++ {
			t[i] = b[i]
		}
		px := FromBytes(t[:])
		if Comp(px, p) >= 0 {
			return NewECP()
		}
		return NewECPbig(px)
	}

	if (MODBITS-1)%8 <= 4 && ALLOW_ALT_COMPRESS {
		alt=true
	}

	if alt {
        for i:=0;i<MB;i++ {
			t[i]=b[i]
		}
        t[0]&=0x1f
        px:=FromBytes(t[:])
        if (b[0]&0x80)==0 {
            for i:=0;i<MB;i++ {
				t[i]=b[i+MB]
			}
            py:=FromBytes(t[:])
            return NewECPbigs(px,py)
        } else {
            sgn:=(b[0]&0x20)>>5
            P:=NewECPbigint(px,0)
            cmp:=P.y.islarger()
            if (sgn==1 && cmp!=1) || (sgn==0 && cmp==1) {
				P.Neg()
			}
            return P
        }
	} else {
		for i := 0; i < MB; i++ {
			t[i] = b[i+1]
		}
		px := FromBytes(t[:])
		if Comp(px, p) >= 0 {
			return NewECP()
		}

		if b[0] == 0x04 {
			for i := 0; i < MB; i++ {
				t[i] = b[i+MB+1]
			}
			py := FromBytes(t[:])
			if Comp(py, p) >= 0 {
				return NewECP()
			}
			return NewECPbigs(px, py)
		}

		if b[0] == 0x02 || b[0] == 0x03 {
			return NewECPbigint(px, int(b[0]&1))
		}
	}
	return NewECP()
}

/* convert to hex string */
func (E *ECP) ToString() string {
	W := NewECP()
	W.Copy(E)
	W.Affine()
	if W.Is_infinity() {
		return "infinity"
	}
	if CURVETYPE == MONTGOMERY {
		return "(" + W.x.redc().ToString() + ")"
	} else {
		return "(" + W.x.redc().ToString() + "," + W.y.redc().ToString() + ")"
	}
}

/* this*=2 */
func (E *ECP) dbl() {

	if CURVETYPE == WEIERSTRASS {
		if CURVE_A == 0 {
			t0 := NewFPcopy(E.y)
			t0.sqr()
			t1 := NewFPcopy(E.y)
			t1.mul(E.z)
			t2 := NewFPcopy(E.z)
			t2.sqr()

			E.z.copy(t0)
			E.z.add(t0)
			E.z.norm()
			E.z.add(E.z)
			E.z.add(E.z)
			E.z.norm()
			t2.imul(3 * CURVE_B_I)

			x3 := NewFPcopy(t2)
			x3.mul(E.z)

			y3 := NewFPcopy(t0)
			y3.add(t2)
			y3.norm()
			E.z.mul(t1)
			t1.copy(t2)
			t1.add(t2)
			t2.add(t1)
			t0.sub(t2)
			t0.norm()
			y3.mul(t0)
			y3.add(x3)
			t1.copy(E.x)
			t1.mul(E.y)
			E.x.copy(t0)
			E.x.norm()
			E.x.mul(t1)
			E.x.add(E.x)
			E.x.norm()
			E.y.copy(y3)
			E.y.norm()
		} else {
			t0 := NewFPcopy(E.x)
			t1 := NewFPcopy(E.y)
			t2 := NewFPcopy(E.z)
			t3 := NewFPcopy(E.x)
			z3 := NewFPcopy(E.z)
			y3 := NewFP()
			x3 := NewFP()
			b := NewFP()

			if CURVE_B_I == 0 {
				b.copy(NewFPbig(NewBIGints(CURVE_B)))
			}

			t0.sqr() //1    x^2
			t1.sqr() //2    y^2
			t2.sqr() //3

			t3.mul(E.y) //4
			t3.add(t3)
			t3.norm()   //5
			z3.mul(E.x) //6
			z3.add(z3)
			z3.norm() //7
			y3.copy(t2)

			if CURVE_B_I == 0 {
				y3.mul(b)
			} else {
				y3.imul(CURVE_B_I)
			}

			y3.sub(z3) //9  ***
			x3.copy(y3)
			x3.add(y3)
			x3.norm() //10

			y3.add(x3) //11
			x3.copy(t1)
			x3.sub(y3)
			x3.norm() //12
			y3.add(t1)
			y3.norm()  //13
			y3.mul(x3) //14
			x3.mul(t3) //15
			t3.copy(t2)
			t3.add(t2) //16
			t2.add(t3) //17

			if CURVE_B_I == 0 {
				z3.mul(b)
			} else {
				z3.imul(CURVE_B_I)
			}

			z3.sub(t2) //19
			z3.sub(t0)
			z3.norm() //20  ***
			t3.copy(z3)
			t3.add(z3) //21

			z3.add(t3)
			z3.norm() //22
			t3.copy(t0)
			t3.add(t0) //23
			t0.add(t3) //24
			t0.sub(t2)
			t0.norm() //25

			t0.mul(z3) //26
			y3.add(t0) //27
			t0.copy(E.y)
			t0.mul(E.z) //28
			t0.add(t0)
			t0.norm()  //29
			z3.mul(t0) //30
			x3.sub(z3) //x3.norm();//31
			t0.add(t0)
			t0.norm() //32
			t1.add(t1)
			t1.norm() //33
			z3.copy(t0)
			z3.mul(t1) //34

			E.x.copy(x3)
			E.x.norm()
			E.y.copy(y3)
			E.y.norm()
			E.z.copy(z3)
			E.z.norm()
		}
	}

	if CURVETYPE == EDWARDS {
		C := NewFPcopy(E.x)
		D := NewFPcopy(E.y)
		H := NewFPcopy(E.z)
		J := NewFP()

		E.x.mul(E.y)
		E.x.add(E.x)
		E.x.norm()
		C.sqr()
		D.sqr()
		if CURVE_A == -1 {
			C.neg()
		}
		E.y.copy(C)
		E.y.add(D)
		E.y.norm()

		H.sqr()
		H.add(H)
		E.z.copy(E.y)
		J.copy(E.y)
		J.sub(H)
		J.norm()
		E.x.mul(J)
		C.sub(D)
		C.norm()
		E.y.mul(C)
		E.z.mul(J)

	}
	if CURVETYPE == MONTGOMERY {
		A := NewFPcopy(E.x)
		B := NewFPcopy(E.x)
		AA := NewFP()
		BB := NewFP()
		C := NewFP()

		A.add(E.z)
		A.norm()
		AA.copy(A)
		AA.sqr()
		B.sub(E.z)
		B.norm()
		BB.copy(B)
		BB.sqr()
		C.copy(AA)
		C.sub(BB)
		C.norm()

		E.x.copy(AA)
		E.x.mul(BB)

		A.copy(C)
		A.imul((CURVE_A + 2) / 4)

		BB.add(A)
		BB.norm()
		E.z.copy(BB)
		E.z.mul(C)
	}
	return
}

/* this+=Q */
func (E *ECP) Add(Q *ECP) {

	if CURVETYPE == WEIERSTRASS {
		if CURVE_A == 0 {
			b := 3 * CURVE_B_I
			t0 := NewFPcopy(E.x)
			t0.mul(Q.x)
			t1 := NewFPcopy(E.y)
			t1.mul(Q.y)
			t2 := NewFPcopy(E.z)
			t2.mul(Q.z)
			t3 := NewFPcopy(E.x)
			t3.add(E.y)
			t3.norm()
			t4 := NewFPcopy(Q.x)
			t4.add(Q.y)
			t4.norm()
			t3.mul(t4)
			t4.copy(t0)
			t4.add(t1)

			t3.sub(t4)
			t3.norm()
			t4.copy(E.y)
			t4.add(E.z)
			t4.norm()
			x3 := NewFPcopy(Q.y)
			x3.add(Q.z)
			x3.norm()

			t4.mul(x3)
			x3.copy(t1)
			x3.add(t2)

			t4.sub(x3)
			t4.norm()
			x3.copy(E.x)
			x3.add(E.z)
			x3.norm()
			y3 := NewFPcopy(Q.x)
			y3.add(Q.z)
			y3.norm()
			x3.mul(y3)
			y3.copy(t0)
			y3.add(t2)
			y3.rsub(x3)
			y3.norm()
			x3.copy(t0)
			x3.add(t0)
			t0.add(x3)
			t0.norm()
			t2.imul(b)

			z3 := NewFPcopy(t1)
			z3.add(t2)
			z3.norm()
			t1.sub(t2)
			t1.norm()
			y3.imul(b)

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
		} else {

			t0 := NewFPcopy(E.x)
			t1 := NewFPcopy(E.y)
			t2 := NewFPcopy(E.z)
			t3 := NewFPcopy(E.x)
			t4 := NewFPcopy(Q.x)
			z3 := NewFP()
			y3 := NewFPcopy(Q.x)
			x3 := NewFPcopy(Q.y)
			b := NewFP()

			if CURVE_B_I == 0 {
				b.copy(NewFPbig(NewBIGints(CURVE_B)))
			}

			t0.mul(Q.x) //1
			t1.mul(Q.y) //2
			t2.mul(Q.z) //3

			t3.add(E.y)
			t3.norm() //4
			t4.add(Q.y)
			t4.norm()  //5
			t3.mul(t4) //6
			t4.copy(t0)
			t4.add(t1) //7
			t3.sub(t4)
			t3.norm() //8
			t4.copy(E.y)
			t4.add(E.z)
			t4.norm() //9
			x3.add(Q.z)
			x3.norm()  //10
			t4.mul(x3) //11
			x3.copy(t1)
			x3.add(t2) //12

			t4.sub(x3)
			t4.norm() //13
			x3.copy(E.x)
			x3.add(E.z)
			x3.norm() //14
			y3.add(Q.z)
			y3.norm() //15

			x3.mul(y3) //16
			y3.copy(t0)
			y3.add(t2) //17

			y3.rsub(x3)
			y3.norm() //18
			z3.copy(t2)

			if CURVE_B_I == 0 {
				z3.mul(b)
			} else {
				z3.imul(CURVE_B_I)
			}

			x3.copy(y3)
			x3.sub(z3)
			x3.norm() //20
			z3.copy(x3)
			z3.add(x3) //21

			x3.add(z3) //22
			z3.copy(t1)
			z3.sub(x3)
			z3.norm() //23
			x3.add(t1)
			x3.norm() //24

			if CURVE_B_I == 0 {
				y3.mul(b)
			} else {
				y3.imul(CURVE_B_I)
			}

			t1.copy(t2)
			t1.add(t2) //26
			t2.add(t1) //27

			y3.sub(t2) //28

			y3.sub(t0)
			y3.norm() //29
			t1.copy(y3)
			t1.add(y3) //30
			y3.add(t1)
			y3.norm() //31

			t1.copy(t0)
			t1.add(t0) //32
			t0.add(t1) //33
			t0.sub(t2)
			t0.norm() //34
			t1.copy(t4)
			t1.mul(y3) //35
			t2.copy(t0)
			t2.mul(y3) //36
			y3.copy(x3)
			y3.mul(z3) //37
			y3.add(t2) //38
			x3.mul(t3) //39
			x3.sub(t1) //40
			z3.mul(t4) //41
			t1.copy(t3)
			t1.mul(t0) //42
			z3.add(t1)
			E.x.copy(x3)
			E.x.norm()
			E.y.copy(y3)
			E.y.norm()
			E.z.copy(z3)
			E.z.norm()

		}
	}
	if CURVETYPE == EDWARDS {
		b := NewFPbig(NewBIGints(CURVE_B))
		A := NewFPcopy(E.z)
		B := NewFP()
		C := NewFPcopy(E.x)
		D := NewFPcopy(E.y)
		EE := NewFP()
		F := NewFP()
		G := NewFP()

		A.mul(Q.z)
		B.copy(A)
		B.sqr()
		C.mul(Q.x)
		D.mul(Q.y)

		EE.copy(C)
		EE.mul(D)
		EE.mul(b)
		F.copy(B)
		F.sub(EE)
		G.copy(B)
		G.add(EE)

		if CURVE_A == 1 {
			EE.copy(D)
			EE.sub(C)
		}
		C.add(D)

		B.copy(E.x)
		B.add(E.y)
		D.copy(Q.x)
		D.add(Q.y)
		B.norm()
		D.norm()
		B.mul(D)
		B.sub(C)
		B.norm()
		F.norm()
		B.mul(F)
		E.x.copy(A)
		E.x.mul(B)
		G.norm()
		if CURVE_A == 1 {
			EE.norm()
			C.copy(EE)
			C.mul(G)
		}
		if CURVE_A == -1 {
			C.norm()
			C.mul(G)
		}
		E.y.copy(A)
		E.y.mul(C)
		E.z.copy(F)
		E.z.mul(G)
	}
	return
}

/* Differential Add for Montgomery curves. this+=Q where W is this-Q and is affine. */
func (E *ECP) dadd(Q *ECP, W *ECP) {
	A := NewFPcopy(E.x)
	B := NewFPcopy(E.x)
	C := NewFPcopy(Q.x)
	D := NewFPcopy(Q.x)
	DA := NewFP()
	CB := NewFP()

	A.add(E.z)
	B.sub(E.z)

	C.add(Q.z)
	D.sub(Q.z)
	A.norm()
	D.norm()

	DA.copy(D)
	DA.mul(A)
	C.norm()
	B.norm()

	CB.copy(C)
	CB.mul(B)

	A.copy(DA)
	A.add(CB)
	A.norm()
	A.sqr()
	B.copy(DA)
	B.sub(CB)
	B.norm()
	B.sqr()

	E.x.copy(A)
	E.z.copy(W.x)
	E.z.mul(B)

}

/* this-=Q */
func (E *ECP) Sub(Q *ECP) {
	NQ := NewECP()
	NQ.Copy(Q)
	NQ.Neg()
	E.Add(NQ)
}

/* constant time multiply by small integer of length bts - use ladder */
func (E *ECP) pinmul(e int32, bts int32) *ECP {
	if CURVETYPE == MONTGOMERY {
		return E.mul(NewBIGint(int(e)))
	} else {
		P := NewECP()
		R0 := NewECP()
		R1 := NewECP()
		R1.Copy(E)

		for i := bts - 1; i >= 0; i-- {
			b := int((e >> uint32(i)) & 1)
			P.Copy(R1)
			P.Add(R0)
			R0.cswap(R1, b)
			R1.Copy(P)
			R0.dbl()
			R0.cswap(R1, b)
		}
		P.Copy(R0)
		return P
	}
}

/* return e.this */

func (E *ECP) mul(e *BIG) *ECP {
	if e.iszilch() || E.Is_infinity() {
		return NewECP()
	}
	P := NewECP()
	if CURVETYPE == MONTGOMERY {
		/* use Ladder */
		D := NewECP()
		R0 := NewECP()
		R0.Copy(E)
		R1 := NewECP()
		R1.Copy(E)
		R1.dbl()
		D.Copy(E); D.Affine()
		nb := e.nbits()
		for i := nb - 2; i >= 0; i-- {
			b := int(e.bit(i))
			P.Copy(R1)
			P.dadd(R0, D)
			R0.cswap(R1, b)
			R1.Copy(P)
			R0.dbl()
			R0.cswap(R1, b)
		}
		P.Copy(R0)
	} else {
		// fixed size windows
		mt := NewBIG()
		t := NewBIG()
		Q := NewECP()
		C := NewECP()

		var W []*ECP
		var w [1 + (NLEN*int(BASEBITS)+3)/4]int8

		Q.Copy(E)
		Q.dbl()

		W = append(W, NewECP())
		W[0].Copy(E)

		for i := 1; i < 8; i++ {
			W = append(W, NewECP())
			W[i].Copy(W[i-1])
			W[i].Add(Q)
		}

		// make exponent odd - add 2P if even, P if odd
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

		// convert exponent to signed 4-bit window
		for i := 0; i < nb; i++ {
			w[i] = int8(t.lastbits(5) - 16)
			t.dec(int(w[i]))
			t.norm()
			t.fshr(4)
		}
		w[nb] = int8(t.lastbits(5))

		P.Copy(W[(int(w[nb])-1)/2])
		for i := nb - 1; i >= 0; i-- {
			Q.selector(W, int32(w[i]))
			P.dbl()
			P.dbl()
			P.dbl()
			P.dbl()
			P.Add(Q)
		}
		P.Sub(C) /* apply correction */
	}
	return P
}

/* Public version */
func (E *ECP) Mul(e *BIG) *ECP {
	return E.mul(e)
}

// Generic multi-multiplication, fixed 4-bit window, P=Sigma e_i*X_i
func ECP_muln(n int, X []*ECP, e []*BIG )  *ECP {
    P:=NewECP()
    R:=NewECP()
    S:=NewECP()
	var B []*ECP
	t := NewBIG()
    for i:=0;i<16;i++ {
		B = append(B,NewECP())
    }
    mt:=NewBIGcopy(e[0]); mt.norm();
    for i:=1;i<n;i++ { // find biggest
        t.copy(e[i]); t.norm()
        k:=Comp(t,mt)
        mt.cmove(t,(k+1)/2)
    }
    nb:=(mt.nbits()+3)/4
    for i:=nb-1;i>=0;i-- {
        for j:=0;j<16;j++ {
            B[j].inf()
        }
        for j:=0;j<n;j++ { 
            mt.copy(e[j]); mt.norm()
            mt.shr(uint(i * 4))
            k:=mt.lastbits(4)
            B[k].Add(X[j])
        }
        R.inf(); S.inf()
        for j:=15;j>=1;j-- {
            R.Add(B[j])
            S.Add(R)
        }
        for j:=0;j<4;j++ {
            P.dbl()
        }
        P.Add(S)
    }
    return P
}

/* Return e.this+f.Q */

func (E *ECP) Mul2(e *BIG, Q *ECP, f *BIG) *ECP {
	te := NewBIG()
	tf := NewBIG()
	mt := NewBIG()
	S := NewECP()
	T := NewECP()
	C := NewECP()
	var W []*ECP
	var w [1 + (NLEN*int(BASEBITS)+1)/2]int8

	te.copy(e)
	tf.copy(f)

	// precompute table
	for i := 0; i < 8; i++ {
		W = append(W, NewECP())
	}
	W[1].Copy(E)
	W[1].Sub(Q)
	W[2].Copy(E)
	W[2].Add(Q)
	S.Copy(Q)
	S.dbl()
	W[0].Copy(W[1])
	W[0].Sub(S)
	W[3].Copy(W[2])
	W[3].Add(S)
	T.Copy(E)
	T.dbl()
	W[5].Copy(W[1])
	W[5].Add(T)
	W[6].Copy(W[2])
	W[6].Add(T)
	W[4].Copy(W[5])
	W[4].Sub(S)
	W[7].Copy(W[6])
	W[7].Add(S)

	// if multiplier is odd, add 2, else add 1 to multiplier, and add 2P or P to correction

	s := int(te.parity())
	te.inc(1)
	te.norm()
	ns := int(te.parity())
	mt.copy(te)
	mt.inc(1)
	mt.norm()
	te.cmove(mt, s)
	T.cmove(E, ns)
	C.Copy(T)

	s = int(tf.parity())
	tf.inc(1)
	tf.norm()
	ns = int(tf.parity())
	mt.copy(tf)
	mt.inc(1)
	mt.norm()
	tf.cmove(mt, s)
	S.cmove(Q, ns)
	C.Add(S)

	mt.copy(te)
	mt.add(tf)
	mt.norm()
	nb := 1 + (mt.nbits()+1)/2

	// convert exponent to signed 2-bit window
	for i := 0; i < nb; i++ {
		a := (te.lastbits(3) - 4)
		te.dec(int(a))
		te.norm()
		te.fshr(2)
		b := (tf.lastbits(3) - 4)
		tf.dec(int(b))
		tf.norm()
		tf.fshr(2)
		w[i] = int8(4*a + b)
	}
	w[nb] = int8(4*te.lastbits(3) + tf.lastbits(3))
	S.Copy(W[(w[nb]-1)/2])

	for i := nb - 1; i >= 0; i-- {
		T.selector(W, int32(w[i]))
		S.dbl()
		S.dbl()
		S.Add(T)
	}
	S.Sub(C) /* apply correction */
	return S
}

func (E *ECP) Cfp() {
	cf := CURVE_Cof_I
	if cf == 1 {
		return
	}
	if cf == 4 {
		E.dbl()
		E.dbl()
		return
	}
	if cf == 8 {
		E.dbl()
		E.dbl()
		E.dbl()
		return
	}
	c := NewBIGints(CURVE_Cof)
	E.Copy(E.mul(c))
}

/* Hunt and Peck a BIG to a curve point */
func ECP_hap2point(h *BIG) *ECP {
	var P *ECP
	x := NewBIGcopy(h)


	for true {
		if CURVETYPE != MONTGOMERY {
			P = NewECPbigint(x, 0)
		} else {
			P = NewECPbig(x)
		}
		x.inc(1)
		x.norm()
		if !P.Is_infinity() {
			break
		}
	}
	return P
}

/* Constant time Map to Point */
func ECP_map2point(h *FP) *ECP {
	P := NewECP()

	if CURVETYPE == MONTGOMERY {
// Elligator 2
			X1:=NewFP()
			X2:=NewFP()
			w:=NewFP()
			one:=NewFPint(1)
			A:=NewFPint(CURVE_A)
			t:=NewFPcopy(h)
			N:=NewFP()
			D:=NewFP()
			hint:=NewFP()

            t.sqr();

            if PM1D2 == 2 {
                t.add(t)
            }
            if PM1D2 == 1 {
                t.neg();
            }
            if PM1D2 > 2 {
                t.imul(QNRI);
            }

            t.norm()
            D.copy(t); D.add(one); D.norm()

            X1.copy(A)
            X1.neg(); X1.norm()
            X2.copy(X1)
            X2.mul(t)

            w.copy(X1); w.sqr(); N.copy(w); N.mul(X1)
            w.mul(A); w.mul(D); N.add(w)
            t.copy(D); t.sqr()
            t.mul(X1)
            N.add(t); N.norm()

            t.copy(N); t.mul(D)
            qres:=t.qr(hint)
            w.copy(t); w.inverse(hint)
            D.copy(w); D.mul(N)
            X1.mul(D)
            X2.mul(D)
            X1.cmove(X2,1-qres)

            a:=X1.redc()
            P.Copy(NewECPbig(a))
	}
	if CURVETYPE == EDWARDS {
// Elligator 2 - map to Montgomery, place point, map back
			X1:=NewFP()
			X2:=NewFP()
			t:=NewFPcopy(h)
			w:=NewFP()
            one:=NewFPint(1)
			A:=NewFP()
			w1:=NewFP()
			w2:=NewFP()
            B:=NewFPbig(NewBIGints(CURVE_B))
			Y:=NewFP()
            K:=NewFP()
			D:=NewFP()
			hint:=NewFP()
			//Y3:=NewFP()
			rfc:=0

			if MODTYPE !=  GENERALISED_MERSENNE {
				A.copy(B)

				if (CURVE_A==1) {
					A.add(one)
					B.sub(one)
				} else {
					A.sub(one)
					B.add(one)
				}
				A.norm(); B.norm()

				A.div2()
				B.div2()
				B.div2()

				K.copy(B)
				K.neg(); K.norm()
				//K.inverse(nil)
				K.invsqrt(K,w1);

				rfc=RIADZ
				if rfc==1 { // RFC7748
					A.mul(K)
					K.mul(w1);
					//K=K.sqrt(nil)
				} else {
					B.sqr()
				}
			} else {
				rfc=1
				A.copy(NewFPint(156326))
			}

            t.sqr()
			qnr:=0
            if PM1D2 == 2 {
                t.add(t)
				qnr=2
            }
            if PM1D2 == 1 {
                t.neg();
				qnr=-1
            }
            if PM1D2 > 2 {
                t.imul(QNRI);
				qnr=QNRI
            }
			t.norm()

            D.copy(t); D.add(one); D.norm()
            X1.copy(A)
            X1.neg(); X1.norm()
            X2.copy(X1); X2.mul(t)

// Figure out RHS of Montgomery curve in rational form gx1/d^3

            w.copy(X1); w.sqr(); w1.copy(w); w1.mul(X1)
            w.mul(A); w.mul(D); w1.add(w)
            w2.copy(D); w2.sqr()

            if rfc==0 {
                w.copy(X1); w.mul(B)
                w2.mul(w)
                w1.add(w2)
            } else {
                w2.mul(X1)
                w1.add(w2)
            }
            w1.norm()

            B.copy(w1); B.mul(D)
            qres:=B.qr(hint)
            w.copy(B); w.inverse(hint)
            D.copy(w); D.mul(w1)
            X1.mul(D)
            X2.mul(D)
            D.sqr()

            w1.copy(B); w1.imul(qnr)
            w.copy(NewFPbig(NewBIGints(CURVE_HTPC)))
            w.mul(hint)
            w2.copy(D); w2.mul(h)

            X1.cmove(X2,1-qres)
            B.cmove(w1,1-qres)
            hint.cmove(w,1-qres)
            D.cmove(w2,1-qres)

            Y.copy(B.sqrt(hint))
            Y.mul(D)

/*
            Y.copy(B.sqrt(hint))
            Y.mul(D)

			B.imul(qnr)
			w.copy(NewFPbig(NewBIGints(CURVE_HTPC)))
			hint.mul(w)

            Y3.copy(B.sqrt(hint))
            D.mul(h)
            Y3.mul(D)

            X1.cmove(X2,1-qres)
            Y.cmove(Y3,1-qres)
*/
            w.copy(Y); w.neg(); w.norm()
            Y.cmove(w,qres^Y.sign())

            if rfc==0 {
                X1.mul(K)
                Y.mul(K)
            }

			if MODTYPE ==  GENERALISED_MERSENNE {
				t.copy(X1); t.sqr()
				w.copy(t); w.add(one); w.norm()
				t.sub(one); t.norm()
				w1.copy(t); w1.mul(Y)
				w1.add(w1); X2.copy(w1); X2.add(w1); X2.norm()
				t.sqr()
				Y.sqr(); Y.add(Y); Y.add(Y); Y.norm()
				B.copy(t); B.add(Y); B.norm()

				w2.copy(Y); w2.sub(t); w2.norm()
				w2.mul(X1)
				t.mul(X1)
				Y.div2()
				w1.copy(Y); w1.mul(w)
				w1.rsub(t); w1.norm()
 
				t.copy(X2); t.mul(w1)
				P.x.copy(t)
				t.copy(w2); t.mul(B)
				P.y.copy(t)
				t.copy(w1); t.mul(B)
				P.z.copy(t)

				return P;
			} else {
				w1.copy(X1); w1.add(one); w1.norm()
				w2.copy(X1); w2.sub(one); w2.norm()
				t.copy(w1); t.mul(Y)
				X1.mul(w1)
	
				if rfc==1 {
					X1.mul(K)
				}
				Y.mul(w2)
                P.x.copy(X1)
                P.y.copy(Y)
                P.z.copy(t)

                return P				
			}
	}
	if CURVETYPE == WEIERSTRASS {
	// swu method
			A:=NewFP()
			B:=NewFP()
			X1:=NewFP()
			X2:=NewFP()
			X3:=NewFP()
            one:=NewFPint(1)
			Y:=NewFP()
			D:=NewFP()
            t:=NewFPcopy(h)
			w:=NewFP()
			D2:=NewFP()
			hint:=NewFP()
			GX1:=NewFP()
			//Y3:=NewFP()
            sgn:=t.sign()

            if CURVE_A != 0 || HTC_ISO != 0{
				if HTC_ISO!=0 {
/* CAHCZS
					A.copy(NewFPbig(NewBIGints(CURVE_Ad)))
					B.copy(NewFPbig(NewBIGints(CURVE_Bd)))
CAHCZF */
				} else {
					A.copy(NewFPint(CURVE_A))
					B.copy(NewFPbig(NewBIGints(CURVE_B)))
				}
				// SSWU method
				t.sqr();
				t.imul(RIADZ)
                w.copy(t); w.add(one); w.norm()

                w.mul(t); D.copy(A)
                D.mul(w)
       
                w.add(one); w.norm()
                w.mul(B)
                w.neg(); w.norm()

                X2.copy(w); 
                X3.copy(t); X3.mul(X2)

// x^3+Ad^2x+Bd^3
                GX1.copy(X2); GX1.sqr(); D2.copy(D)
                D2.sqr(); w.copy(A); w.mul(D2); GX1.add(w); GX1.norm(); GX1.mul(X2); D2.mul(D);w.copy(B); w.mul(D2); GX1.add(w); GX1.norm()

                w.copy(GX1); w.mul(D)
                qr:=w.qr(hint)
                D.copy(w); D.inverse(hint)
                D.mul(GX1)
                X2.mul(D)
                X3.mul(D)
                t.mul(h)
                D2.copy(D); D2.sqr()


                D.copy(D2); D.mul(t)
                t.copy(w); t.imul(RIADZ)
                X1.copy(NewFPbig(NewBIGints(CURVE_HTPC)))
                X1.mul(hint)

                X2.cmove(X3,1-qr)
                D2.cmove(D,1-qr)
                w.cmove(t,1-qr)
                hint.cmove(X1,1-qr)

                Y.copy(w.sqrt(hint))
                Y.mul(D2)
/*
                Y.copy(w.sqrt(hint))
                Y.mul(D2)

                D2.mul(t)
                w.imul(RIADZ)

                X1.copy(NewFPbig(NewBIGints(CURVE_HTPC)))
                hint.mul(X1)
                
                Y3.copy(w.sqrt(hint))
                Y3.mul(D2)

                X2.cmove(X3,1-qr)
                Y.cmove(Y3,1-qr)
*/
				ne:=Y.sign()^sgn
				w.copy(Y); w.neg(); w.norm()
				Y.cmove(w,ne)

				if HTC_ISO!=0 {
/* CAHCZS
					k:=0
					isox:=HTC_ISO
					isoy:=3*(isox-1)/2

				//xnum
					xnum:=NewFPbig(NewBIGints(PC[k])); k+=1
					for i:=0;i<isox;i++ {
						xnum.mul(X2)
						w.copy(NewFPbig(NewBIGints(PC[k]))); k+=1
						xnum.add(w); xnum.norm()
					}
				//xden
					xden:=NewFPcopy(X2)
					w.copy(NewFPbig(NewBIGints(PC[k]))); k+=1
					xden.add(w);xden.norm();
					for i:=0;i<isox-2;i++ {
						xden.mul(X2)
						w.copy(NewFPbig(NewBIGints(PC[k]))); k+=1
						xden.add(w); xden.norm()
					}
				//ynum
					ynum:=NewFPbig(NewBIGints(PC[k])); k+=1
					for i:=0;i<isoy;i++ {
						ynum.mul(X2)
						w.copy(NewFPbig(NewBIGints(PC[k]))); k+=1
						ynum.add(w); ynum.norm()
					}
					yden:=NewFPcopy(X2)
					w.copy(NewFPbig(NewBIGints(PC[k]))); k+=1
					yden.add(w);yden.norm();
					for i:=0;i<isoy-1;i++ {
						yden.mul(X2)
						w.copy(NewFPbig(NewBIGints(PC[k]))); k+=1
						yden.add(w); yden.norm()
					}
					ynum.mul(Y)
					w.copy(xnum); w.mul(yden)
					P.x.copy(w)
					w.copy(ynum); w.mul(xden)
					P.y.copy(w)
					w.copy(xden); w.mul(yden)
					P.z.copy(w)
					return P
CAHCZF */
				} else {
					x:=X2.redc()
					y:=Y.redc()
					P.Copy(NewECPbigs(x,y))
					return P
				}
            } else {
// Shallue and van de Woestijne
// SQRTm3 not available, so preprocess this out
/* */
                Z:=RIADZ
                X1.copy(NewFPint(Z))
                X3.copy(X1)
                A.copy(RHS(X1))
				B.copy(NewFPbig(NewBIGints(SQRTm3)))
				B.imul(Z)

                t.sqr()
                Y.copy(A); Y.mul(t)
                t.copy(one); t.add(Y); t.norm()
                Y.rsub(one); Y.norm()
                D.copy(t); D.mul(Y); 
				D.mul(B)
				
                w.copy(A);
				FP_tpo(D,w)

                w.mul(B)
                if (w.sign()==1) {
                    w.neg()
                    w.norm()
                }

                w.mul(B)				
                w.mul(h); w.mul(Y); w.mul(D)

                X1.neg(); X1.norm(); X1.div2()
                X2.copy(X1)
                X1.sub(w); X1.norm()
                X2.add(w); X2.norm()
                A.add(A); A.add(A); A.norm()
                t.sqr(); t.mul(D); t.sqr()
                A.mul(t)
                X3.add(A); X3.norm()

                rhs:=RHS(X2)
                X3.cmove(X2,rhs.qr(nil))
                rhs.copy(RHS(X1))
                X3.cmove(X1,rhs.qr(nil))
                rhs.copy(RHS(X3))
                Y.copy(rhs.sqrt(nil))

				ne:=Y.sign()^sgn
				w.copy(Y); w.neg(); w.norm()
				Y.cmove(w,ne)

				x:=X3.redc();
				y:=Y.redc();
				P.Copy(NewECPbigs(x,y))
				return P
/* */
            }
	}
	return P
}

func ECP_mapit(h []byte) *ECP {
	q := NewBIGints(Modulus)
	dx:= DBIG_fromBytes(h[:])
	x:= dx.Mod(q)

	P:= ECP_hap2point(x)
	P.Cfp()
	return P
}

func ECP_generator() *ECP {
	var G *ECP

	gx := NewBIGints(CURVE_Gx)
	if CURVETYPE != MONTGOMERY {
		gy := NewBIGints(CURVE_Gy)
		G = NewECPbigs(gx, gy)
	} else {
		G = NewECPbig(gx)
	}
	return G
}
