/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package amcl

//import "fmt"

/* Elliptic Curve Point Structure */

type ECP struct {
	x *FP
	y *FP
	z *FP
	INF bool
}

/* Constructors */
func NewECP() *ECP {
	E:=new(ECP)
	E.x=NewFPint(0)
	E.y=NewFPint(0)
	E.z=NewFPint(0)
	E.INF=true
	return E
}

/* set (x,y) from two BIGs */
func NewECPbigs(ix *BIG,iy *BIG) *ECP {
	E:=new(ECP)
	E.x=NewFPbig(ix)
	E.y=NewFPbig(iy)
	E.z=NewFPint(1)
	rhs:=RHS(E.x)

	if CURVETYPE==MONTGOMERY {
		if rhs.jacobi()==1 {
			E.INF=false
		} else {E.inf()}
	} else {
		y2:=NewFPcopy(E.y)
		y2.sqr()
		if y2.equals(rhs) {
			E.INF=false
		} else {E.inf()}
	}
	return E
}

/* set (x,y) from BIG and a bit */
func NewECPbigint(ix *BIG,s int) *ECP {
	E:=new(ECP)
	E.x=NewFPbig(ix)
	E.y=NewFPint(0)
	rhs:=RHS(E.x)
	E.z=NewFPint(1)
	if rhs.jacobi()==1 {
		ny:=rhs.sqrt()
		if ny.redc().parity()!=s {ny.neg()}
		E.y.copy(ny)
		E.INF=false
	} else {E.inf()}
	return E;
}

/* set from x - calculate y from curve equation */
func NewECPbig(ix *BIG) *ECP {
	E:=new(ECP)	
	E.x=NewFPbig(ix)
	E.y=NewFPint(0)
	rhs:=RHS(E.x)
	E.z=NewFPint(1)
	if rhs.jacobi()==1 {
		if CURVETYPE!=MONTGOMERY {E.y.copy(rhs.sqrt())}
		E.INF=false
	} else {E.INF=true}
	return E
}

/* test for O point-at-infinity */
func (E *ECP) is_infinity() bool {
	if CURVETYPE==EDWARDS {
		E.x.reduce(); E.y.reduce(); E.z.reduce()
		return (E.x.iszilch() && E.y.equals(E.z))
	} else {return E.INF}
}

/* Conditional swap of P and Q dependant on d */
func (E *ECP) cswap(Q *ECP,d int) {
	E.x.cswap(Q.x,d)
	if CURVETYPE!=MONTGOMERY {E.y.cswap(Q.y,d)}
	E.z.cswap(Q.z,d)
	if CURVETYPE!=EDWARDS {
		bd:=true
		if d==0 {bd=false}
		bd=bd&&(E.INF!=Q.INF)
		E.INF=(bd!=E.INF)
		Q.INF=(bd!=Q.INF)
	}
}

/* Conditional move of Q to P dependant on d */
func (E *ECP) cmove(Q *ECP,d int) {
	E.x.cmove(Q.x,d)
	if CURVETYPE!=MONTGOMERY {E.y.cmove(Q.y,d)}
	E.z.cmove(Q.z,d);
	if CURVETYPE!=EDWARDS {
		bd:=true
		if d==0 {bd=false}
		E.INF=(E.INF!=((E.INF!=Q.INF)&&bd))
	}
}

/* return 1 if b==c, no branching */
func teq(b int32,c int32) int {
	x:=b^c
	x-=1  // if x=0, x now -1
	return int((x>>31)&1)
}

/* this=P */
func (E *ECP) copy(P *ECP) {
	E.x.copy(P.x);
	if CURVETYPE!=MONTGOMERY {E.y.copy(P.y)}
	E.z.copy(P.z);
	E.INF=P.INF;
}

/* this=-this */
func (E *ECP) neg() {
	if E.is_infinity() {return}
	if CURVETYPE==WEIERSTRASS {
		E.y.neg(); E.y.norm()
	}
	if CURVETYPE==EDWARDS {
		E.x.neg(); E.x.norm()
	}
	return;
}

/* Constant time select from pre-computed table */
func (E *ECP) selector(W []*ECP,b int32) {
	MP:=NewECP()
	m:=b>>31;
	babs:=(b^m)-m;

	babs=(babs-1)/2

	E.cmove(W[0],teq(babs,0))  // conditional move
	E.cmove(W[1],teq(babs,1))
	E.cmove(W[2],teq(babs,2))
	E.cmove(W[3],teq(babs,3))
	E.cmove(W[4],teq(babs,4))
	E.cmove(W[5],teq(babs,5))
	E.cmove(W[6],teq(babs,6))
	E.cmove(W[7],teq(babs,7))
 
	MP.copy(E);
	MP.neg()
	E.cmove(MP,int(m&1));
}

/* set this=O */
func (E *ECP) inf() {
	E.INF=true;
	E.x.zero()
	E.y.one()
	E.z.one()
}

/* Test P == Q */
func( E *ECP) equals(Q *ECP) bool {
	if E.is_infinity() && Q.is_infinity() {return true}
	if E.is_infinity() || Q.is_infinity() {return false}
	if CURVETYPE==WEIERSTRASS {
		zs2:=NewFPcopy(E.z); zs2.sqr()
		zo2:=NewFPcopy(Q.z); zo2.sqr()
		zs3:=NewFPcopy(zs2); zs3.mul(E.z)
		zo3:=NewFPcopy(zo2); zo3.mul(Q.z)
		zs2.mul(Q.x)
		zo2.mul(E.x)
		if !zs2.equals(zo2) {return false}
		zs3.mul(Q.y)
		zo3.mul(E.y)
		if !zs3.equals(zo3) {return false}
	} else {
		a:=NewFPint(0)
		b:=NewFPint(0)
		a.copy(E.x); a.mul(Q.z); a.reduce()
		b.copy(Q.x); b.mul(E.z); b.reduce()
		if !a.equals(b) {return false}
		if CURVETYPE==EDWARDS {
			a.copy(E.y); a.mul(Q.z); a.reduce()
			b.copy(Q.y); b.mul(E.z); b.reduce()
			if !a.equals(b) {return false}
		}
	}
	return true
}

/* Calculate RHS of curve equation */
func RHS(x *FP) *FP {
	x.norm()
	r:=NewFPcopy(x)
	r.sqr();

	if CURVETYPE==WEIERSTRASS { // x^3+Ax+B
		b:=NewFPbig(NewBIGints(CURVE_B))
		r.mul(x);
		if CURVE_A==-3 {
			cx:=NewFPcopy(x)
			cx.imul(3)
			cx.neg(); cx.norm()
			r.add(cx)
		}
		r.add(b)
	}
	if CURVETYPE==EDWARDS { // (Ax^2-1)/(Bx^2-1) 
		b:=NewFPbig(NewBIGints(CURVE_B))

		one:=NewFPint(1)
		b.mul(r)
		b.sub(one)
		if CURVE_A==-1 {r.neg()}
		r.sub(one)
		b.inverse()
		r.mul(b)
	}
	if CURVETYPE==MONTGOMERY { // x^3+Ax^2+x
		x3:=NewFPint(0)
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
func (E *ECP) affine() {
	if E.is_infinity() {return}
	one:=NewFPint(1)
	if E.z.equals(one) {return}
	E.z.inverse()
	if CURVETYPE==WEIERSTRASS {
		z2:=NewFPcopy(E.z)
		z2.sqr()
		E.x.mul(z2); E.x.reduce()
		E.y.mul(z2)
		E.y.mul(E.z);  E.y.reduce()
	}
	if CURVETYPE==EDWARDS {
		E.x.mul(E.z); E.x.reduce()
		E.y.mul(E.z); E.y.reduce()
	}
	if CURVETYPE==MONTGOMERY {
		E.x.mul(E.z); E.x.reduce()
	}
	E.z.one()
}

/* extract x as a BIG */
func (E *ECP) getX() *BIG {
	E.affine()
	return E.x.redc()
}
/* extract y as a BIG */
func (E *ECP) getY() *BIG {
	E.affine()
	return E.y.redc()
}

/* get sign of Y */
func (E *ECP) getS() int {
	E.affine()
	y:=E.getY()
	return y.parity()
}
/* extract x as an FP */
func (E *ECP) getx() *FP {
	return E.x;
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
func (E *ECP) toBytes(b []byte) {
	var t [int(MODBYTES)]byte
	MB:=int(MODBYTES)
	if CURVETYPE!=MONTGOMERY {
		b[0]=0x04
	} else {b[0]=0x02}
	
	E.affine()
	E.x.redc().toBytes(t[:])
	for i:=0;i<MB;i++ {b[i+1]=t[i]}
	if CURVETYPE!=MONTGOMERY {
		E.y.redc().toBytes(t[:])
		for i:=0;i<MB;i++ {b[i+MB+1]=t[i]}
	}
}

/* convert from byte array to point */
func ECP_fromBytes(b []byte) *ECP {
	var t [int(MODBYTES)]byte
	MB:=int(MODBYTES)
	p:=NewBIGints(Modulus)

	for i:=0;i<MB;i++ {t[i]=b[i+1]}
	px:=fromBytes(t[:])
	if comp(px,p)>=0 {return NewECP()}

	if (b[0]==0x04) {
		for i:=0;i<MB;i++ {t[i]=b[i+MB+1]}
		py:=fromBytes(t[:])
		if comp(py,p)>=0 {return NewECP()}
		return NewECPbigs(px,py)
	} else {return NewECPbig(px)}
}

/* convert to hex string */
func (E *ECP) toString() string {
	if E.is_infinity() {return "infinity"}
	E.affine();
	if CURVETYPE==MONTGOMERY {
		return "("+E.x.redc().toString()+")"
	} else {return "("+E.x.redc().toString()+","+E.y.redc().toString()+")"}
}

/* this*=2 */
func (E *ECP) dbl() {
	if CURVETYPE==WEIERSTRASS {
		if E.INF {return}
		if E.y.iszilch() {
			E.inf()
			return
		}

		w1:=NewFPcopy(E.x);
		w6:=NewFPcopy(E.z);
		w2:=NewFPint(0);
		w3:=NewFPcopy(E.x)
		w8:=NewFPcopy(E.x)

		if CURVE_A==-3 {
			w6.sqr()
			w1.copy(w6)
			w1.neg()
			w3.add(w1)

			w8.add(w6)

			w3.mul(w8)
			w8.copy(w3)
			w8.imul(3)
		} else {
			w1.sqr()
			w8.copy(w1)
			w8.imul(3)
		}

		w2.copy(E.y); w2.sqr()
		w3.copy(E.x); w3.mul(w2)
		w3.imul(4)
		w1.copy(w3); w1.neg()
	//		w1.norm();


		E.x.copy(w8); E.x.sqr()
		E.x.add(w1)
		E.x.add(w1)
	//		x.reduce();
		E.x.norm()

		E.z.mul(E.y)
		E.z.add(E.z)

		w2.add(w2)
		w2.sqr()
		w2.add(w2)
		w3.sub(E.x)
		E.y.copy(w8); E.y.mul(w3);
	//		w2.norm();
		E.y.sub(w2)
	//		y.reduce();
	//		z.reduce();
		E.y.norm()
		E.z.norm()

	}
	if CURVETYPE==EDWARDS {
		C:=NewFPcopy(E.x)
		D:=NewFPcopy(E.y)
		H:=NewFPcopy(E.z)
		J:=NewFPint(0)
	
		E.x.mul(E.y); E.x.add(E.x)
		C.sqr()
		D.sqr()
		if CURVE_A==-1 {C.neg()}	
		E.y.copy(C); E.y.add(D)
	//		y.norm();
		H.sqr(); H.add(H)
		E.z.copy(E.y)
		J.copy(E.y); J.sub(H)
		E.x.mul(J)
		C.sub(D)
		E.y.mul(C)
		E.z.mul(J)

		E.x.norm()
		E.y.norm()
		E.z.norm()
	}
	if CURVETYPE==MONTGOMERY {
		A:=NewFPcopy(E.x)
		B:=NewFPcopy(E.x)	
		AA:=NewFPint(0)
		BB:=NewFPint(0)
		C:=NewFPint(0)
	
		if E.INF {return}

		A.add(E.z)
		AA.copy(A); AA.sqr()
		B.sub(E.z)
		BB.copy(B); BB.sqr()
		C.copy(AA); C.sub(BB)
	//		C.norm();

		E.x.copy(AA); E.x.mul(BB)

		A.copy(C); A.imul((CURVE_A+2)/4)

		BB.add(A)
		E.z.copy(BB); E.z.mul(C)
	//		x.reduce();
	//		z.reduce();
		E.x.norm()
		E.z.norm()
	}
	return;
}

/* this+=Q */
func (E *ECP) add(Q *ECP) {
	if CURVETYPE==WEIERSTRASS {
		if E.INF {
			E.copy(Q)
			return
		}
		if Q.INF {return}

		aff:=false

		one:=NewFPint(1)
		if Q.z.equals(one) {aff=true}

		var A,C *FP
		B:=NewFPcopy(E.z)
		D:=NewFPcopy(E.z)
		if !aff {
			A=NewFPcopy(Q.z)
			C=NewFPcopy(Q.z)

			A.sqr(); B.sqr()
			C.mul(A); D.mul(B)

			A.mul(E.x)
			C.mul(E.y)
		} else {
			A=NewFPcopy(E.x)
			C=NewFPcopy(E.y)
	
			B.sqr()
			D.mul(B)
		}

		B.mul(Q.x); B.sub(A)
		D.mul(Q.y); D.sub(C)

		if B.iszilch() {
			if D.iszilch() {
				E.dbl()
				return
			} else {
				E.INF=true
				return
			}
		}

		if !aff {E.z.mul(Q.z)}
		E.z.mul(B)

		e:=NewFPcopy(B); e.sqr()
		B.mul(e)
		A.mul(e)

		e.copy(A)
		e.add(A); e.add(B)
		E.x.copy(D); E.x.sqr(); E.x.sub(e);

		A.sub(E.x);
		E.y.copy(A); E.y.mul(D)
		C.mul(B); E.y.sub(C)

		//	x.reduce();
		//	y.reduce();
		//	z.reduce();
		E.x.norm()
		E.y.norm()
		E.z.norm()
	}
	if CURVETYPE==EDWARDS {
		b:=NewFPbig(NewBIGints(CURVE_B))
		A:=NewFPcopy(E.z)
		B:=NewFPint(0)
		C:=NewFPcopy(E.x)
		D:=NewFPcopy(E.y)
		EE:=NewFPint(0)
		F:=NewFPint(0)
		G:=NewFPint(0)
		//H:=NewFPint(0)
		//I:=NewFPint(0)
	
		A.mul(Q.z);
		B.copy(A); B.sqr()
		C.mul(Q.x)
		D.mul(Q.y)

		EE.copy(C); EE.mul(D); EE.mul(b)
		F.copy(B); F.sub(EE)
		G.copy(B); G.add(EE)

		if CURVE_A==1 {
			EE.copy(D); EE.sub(C)
		}
		C.add(D)

		B.copy(E.x); B.add(E.y)
		D.copy(Q.x); D.add(Q.y)
		B.mul(D)
		B.sub(C)
		B.mul(F)
		E.x.copy(A); E.x.mul(B)

		if CURVE_A==1 {
			C.copy(EE); C.mul(G)
		}
		if CURVE_A==-1 {
			C.mul(G)
		}
		E.y.copy(A); E.y.mul(C)
		E.z.copy(F); E.z.mul(G)
		//	x.reduce(); y.reduce(); z.reduce();
		E.x.norm(); E.y.norm(); E.z.norm()
	}
	return
}

/* Differential Add for Montgomery curves. this+=Q where W is this-Q and is affine. */
func (E *ECP) dadd(Q *ECP,W *ECP) {
	A:=NewFPcopy(E.x)
	B:=NewFPcopy(E.x)
	C:=NewFPcopy(Q.x)
	D:=NewFPcopy(Q.x)
	DA:=NewFPint(0)
	CB:=NewFPint(0)
			
	A.add(E.z)
	B.sub(E.z)

	C.add(Q.z)
	D.sub(Q.z)

	DA.copy(D); DA.mul(A)
	CB.copy(C); CB.mul(B)

	A.copy(DA); A.add(CB); A.sqr()
	B.copy(DA); B.sub(CB); B.sqr()

	E.x.copy(A)
	E.z.copy(W.x); E.z.mul(B)

	if E.z.iszilch() {
		E.inf()
	} else {E.INF=false;}

	//	x.reduce();
	E.x.norm();
}

/* this-=Q */
func (E *ECP) sub(Q *ECP) {
	Q.neg()
	E.add(Q)
	Q.neg()
}

func multiaffine(m int,P []*ECP) {
	t1:=NewFPint(0)
	t2:=NewFPint(0)

	var work []*FP

	for i:=0;i<m;i++ {
		work=append(work,NewFPint(0))
	}
	
	work[0].one()
	work[1].copy(P[0].z)

	for i:=2;i<m;i++ {
		work[i].copy(work[i-1])
		work[i].mul(P[i-1].z)
	}

	t1.copy(work[m-1])
	t1.mul(P[m-1].z)
	t1.inverse()
	t2.copy(P[m-1].z)
	work[m-1].mul(t1)

	for i:=m-2;;i-- {
		if i==0 {
			work[0].copy(t1)
			work[0].mul(t2)
			break
		}
		work[i].mul(t2)
		work[i].mul(t1)
		t2.mul(P[i].z)
	}
/* now work[] contains inverses of all Z coordinates */

	for i:=0;i<m;i++ {
		P[i].z.one()
		t1.copy(work[i])
		t1.sqr()
		P[i].x.mul(t1)
		t1.mul(work[i])
		P[i].y.mul(t1)
	}    
}

/* constant time multiply by small integer of length bts - use ladder */
func (E *ECP) pinmul(e int32,bts int32) *ECP {	
	if CURVETYPE==MONTGOMERY {
		return E.mul(NewBIGint(int(e)))
	} else {
		P:=NewECP()
		R0:=NewECP()
		R1:=NewECP(); R1.copy(E)

		for i:=bts-1;i>=0;i-- {
			b:=int((e>>uint32(i))&1)
			P.copy(R1)
			P.add(R0)
			R0.cswap(R1,b)
			R1.copy(P)
			R0.dbl()
			R0.cswap(R1,b)
		}
		P.copy(R0)
		P.affine()
		return P
	}
}

/* return e.this */

func (E *ECP) mul(e *BIG) *ECP {
	if (e.iszilch() || E.is_infinity()) {return NewECP()}
	P:=NewECP()
	if CURVETYPE==MONTGOMERY {
/* use Ladder */
		D:=NewECP();
		R0:=NewECP(); R0.copy(E)
		R1:=NewECP(); R1.copy(E)
		R1.dbl()
		D.copy(E); D.affine()
		nb:=e.nbits()
		for i:=nb-2;i>=0;i-- {
			b:=int(e.bit(i))
			P.copy(R1)
			P.dadd(R0,D)
			R0.cswap(R1,b)
			R1.copy(P)
			R0.dbl()
			R0.cswap(R1,b)
		}
		P.copy(R0)
	} else {
// fixed size windows 
		mt:=NewBIG()
		t:=NewBIG()
		Q:=NewECP()
		C:=NewECP()

		var W []*ECP
		var w [1+(NLEN*int(BASEBITS)+3)/4]int8

		E.affine();

		Q.copy(E);
		Q.dbl();

		W=append(W,NewECP());
		W[0].copy(E);

		for i:=1;i<8;i++ {
			W=append(W,NewECP())
			W[i].copy(W[i-1])
			W[i].add(Q)
		}


// convert the table to affine 
		if CURVETYPE==WEIERSTRASS {
			multiaffine(8,W[:])
		}


// make exponent odd - add 2P if even, P if odd 
		t.copy(e)
		s:=int(t.parity())
		t.inc(1); t.norm(); ns:=int(t.parity()); mt.copy(t); mt.inc(1); mt.norm()
		t.cmove(mt,s)
		Q.cmove(E,ns)
		C.copy(Q)

		nb:=1+(t.nbits()+3)/4

// convert exponent to signed 4-bit window 
		for i:=0;i<nb;i++ {
			w[i]=int8(t.lastbits(5)-16)
			t.dec(int(w[i])); t.norm()
			t.fshr(4)	
		}
		w[nb]=int8(t.lastbits(5))

		P.copy(W[(int(w[nb])-1)/2])  
		for i:=nb-1;i>=0;i-- {
			Q.selector(W,int32(w[i]))
			P.dbl()
			P.dbl()
			P.dbl()
			P.dbl()
			P.add(Q)
		}
		P.sub(C) /* apply correction */
	}
	P.affine()
	return P
}

/* Return e.this+f.Q */

func (E *ECP) mul2(e *BIG,Q *ECP,f *BIG) *ECP {
	te:=NewBIG()
	tf:=NewBIG()
	mt:=NewBIG()
	S:=NewECP()
	T:=NewECP()
	C:=NewECP()
	var W [] *ECP
	//ECP[] W=new ECP[8];
	var w [1+(NLEN*int(BASEBITS)+1)/2]int8		

	E.affine()
	Q.affine()

	te.copy(e)
	tf.copy(f)

// precompute table 
	for i:=0;i<8;i++ {
		W=append(W,NewECP())
	}
	W[1].copy(E); W[1].sub(Q)
	W[2].copy(E); W[2].add(Q);
	S.copy(Q); S.dbl();
	W[0].copy(W[1]); W[0].sub(S);
	W[3].copy(W[2]); W[3].add(S);
	T.copy(E); T.dbl();
	W[5].copy(W[1]); W[5].add(T);
	W[6].copy(W[2]); W[6].add(T);
	W[4].copy(W[5]); W[4].sub(S);
	W[7].copy(W[6]); W[7].add(S);

// convert the table to affine 
	if CURVETYPE==WEIERSTRASS { 
		multiaffine(8,W)
	}

// if multiplier is odd, add 2, else add 1 to multiplier, and add 2P or P to correction 

	s:=int(te.parity());
	te.inc(1); te.norm(); ns:=int(te.parity()); mt.copy(te); mt.inc(1); mt.norm()
	te.cmove(mt,s)
	T.cmove(E,ns)
	C.copy(T)

	s=int(tf.parity())
	tf.inc(1); tf.norm(); ns=int(tf.parity()); mt.copy(tf); mt.inc(1); mt.norm()
	tf.cmove(mt,s)
	S.cmove(Q,ns)
	C.add(S)

	mt.copy(te); mt.add(tf); mt.norm()
	nb:=1+(mt.nbits()+1)/2

// convert exponent to signed 2-bit window 
	for i:=0;i<nb;i++ {
		a:=(te.lastbits(3)-4)
		te.dec(int(a)); te.norm()
		te.fshr(2)
		b:=(tf.lastbits(3)-4)
		tf.dec(int(b)); tf.norm()
		tf.fshr(2)
		w[i]=int8(4*a+b)
	}
	w[nb]=int8(4*te.lastbits(3)+tf.lastbits(3))
	S.copy(W[(w[nb]-1)/2])  

	for i:=nb-1;i>=0;i-- {
		T.selector(W,int32(w[i]));
		S.dbl()
		S.dbl()
		S.add(T)
	}
	S.sub(C) /* apply correction */
	S.affine()
	return S
}

/*
func main() {
	Gx:=NewBIGints(CURVE_Gx);
	var Gy *BIG
	var P *ECP

	if CURVETYPE!=MONTGOMERY {Gy=NewBIGints(CURVE_Gy)}
	r:=NewBIGints(CURVE_Order)

	//r.dec(7);
	
	fmt.Printf("Gx= "+Gx.toString())
	fmt.Printf("\n")

	if CURVETYPE!=MONTGOMERY {
		fmt.Printf("Gy= "+Gy.toString())
		fmt.Printf("\n")
	}	

	if CURVETYPE!=MONTGOMERY {
		P=NewECPbigs(Gx,Gy)
	} else  {P=NewECPbig(Gx)}

	fmt.Printf("P= "+P.toString());		
	fmt.Printf("\n")

	R:=P.mul(r);
		//for (int i=0;i<10000;i++)
		//	R=P.mul(r);
	
	fmt.Printf("R= "+R.toString())
	fmt.Printf("\n")
}
*/