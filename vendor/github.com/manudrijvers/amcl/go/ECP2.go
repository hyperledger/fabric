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

/* MiotCL Weierstrass elliptic curve functions over FP2 */

package amcl

//import "fmt"

type ECP2 struct {
	x *FP2
	y *FP2
	z *FP2
	INF bool
}

func NewECP2() *ECP2 {
	E:=new(ECP2)
	E.x=NewFP2int(0)
	E.y=NewFP2int(1)
	E.z=NewFP2int(1)
	E.INF=true
	return E
}

/* Test this=O? */
func (E *ECP2) is_infinity() bool {
		return E.INF
}
/* copy this=P */
func (E *ECP2) copy(P *ECP2) {
	E.x.copy(P.x)
	E.y.copy(P.y)
	E.z.copy(P.z)
	E.INF=P.INF
}
/* set this=O */
func (E *ECP2) inf() {
	E.INF=true
	E.x.zero()
	E.y.zero()
	E.z.zero()
}

/* set this=-this */
func (E *ECP2) neg() {
	if E.is_infinity() {return}
	E.y.neg(); E.y.reduce()
}

/* Conditional move of Q to P dependant on d */
func (E *ECP2) cmove(Q *ECP2,d int) {
	E.x.cmove(Q.x,d)
	E.y.cmove(Q.y,d)
	E.z.cmove(Q.z,d)

	var bd bool
	if (d==0) {
		bd=false
	} else {bd=true}
	E.INF=(E.INF!=(E.INF!=Q.INF)&&bd)
}

/* Constant time select from pre-computed table */
func (E *ECP2) selector(W []*ECP2,b int32) {
	MP:=NewECP2() 
	m:=b>>31
	babs:=(b^m)-m

	babs=(babs-1)/2

	E.cmove(W[0],teq(babs,0))  // conditional move
	E.cmove(W[1],teq(babs,1))
	E.cmove(W[2],teq(babs,2))
	E.cmove(W[3],teq(babs,3))
	E.cmove(W[4],teq(babs,4))
	E.cmove(W[5],teq(babs,5))
	E.cmove(W[6],teq(babs,6))
	E.cmove(W[7],teq(babs,7))
 
	MP.copy(E)
	MP.neg()
	E.cmove(MP,int(m&1))
}

/* Test if P == Q */
func (E *ECP2) equals(Q *ECP2) bool {
	if E.is_infinity() && Q.is_infinity() {return true}
	if E.is_infinity() || Q.is_infinity() {return false}

	zs2:=NewFP2copy(E.z); zs2.sqr()
	zo2:=NewFP2copy(Q.z); zo2.sqr()
	zs3:=NewFP2copy(zs2); zs3.mul(E.z)
	zo3:=NewFP2copy(zo2); zo3.mul(Q.z)
	zs2.mul(Q.x)
	zo2.mul(E.x)
	if !zs2.equals(zo2) {return false}
	zs3.mul(Q.y)
	zo3.mul(E.y)
	if !zs3.equals(zo3) {return false}

	return true
}

/* set to Affine - (x,y,z) to (x,y) */
func (E *ECP2) affine() {
	if E.is_infinity() {return}
	one:=NewFP2int(1)
	if E.z.equals(one) {return}
	E.z.inverse()

	z2:=NewFP2copy(E.z);
	z2.sqr()
	E.x.mul(z2); E.x.reduce()
	E.y.mul(z2) 
	E.y.mul(E.z);  E.y.reduce()
	E.z.copy(one)
}

/* extract affine x as FP2 */
func (E *ECP2) getX() *FP2 {
	E.affine()
	return E.x
}
/* extract affine y as FP2 */
func (E *ECP2) getY() *FP2 {
	E.affine();
	return E.y;
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
func (E *ECP2) toBytes(b []byte) {
	var t [int(MODBYTES)]byte
	MB:=int(MODBYTES)

	E.affine()
	E.x.getA().toBytes(t[:])
	for i:=0;i<MB;i++ { b[i]=t[i]}
	E.x.getB().toBytes(t[:])
	for i:=0;i<MB;i++ { b[i+MB]=t[i]}

	E.y.getA().toBytes(t[:])
	for i:=0;i<MB;i++ {b[i+2*MB]=t[i]}
	E.y.getB().toBytes(t[:])
	for i:=0;i<MB;i++ {b[i+3*MB]=t[i]}
}

/* convert from byte array to point */
func ECP2_fromBytes(b []byte) *ECP2 {
	var t [int(MODBYTES)]byte
	MB:=int(MODBYTES)

	for i:=0;i<MB;i++ {t[i]=b[i]}
	ra:=fromBytes(t[:])
	for i:=0;i<MB;i++ {t[i]=b[i+MB]}
	rb:=fromBytes(t[:])
	rx:=NewFP2bigs(ra,rb)

	for i:=0;i<MB;i++ {t[i]=b[i+2*MB]}
	ra=fromBytes(t[:])
	for i:=0;i<MB;i++ {t[i]=b[i+3*MB]}
	rb=fromBytes(t[:])
	ry:=NewFP2bigs(ra,rb)

	return NewECP2fp2s(rx,ry)
}

/* convert this to hex string */
func (E *ECP2) toString() string {
	if E.is_infinity() {return "infinity"}
	E.affine()
	return "("+E.x.toString()+","+E.y.toString()+")"
}

/* Calculate RHS of twisted curve equation x^3+B/i */
func RHS2(x *FP2) *FP2 {
	x.norm()
	r:=NewFP2copy(x)
	r.sqr()
	b:=NewFP2big(NewBIGints(CURVE_B))
	b.div_ip()
	r.mul(x)
	r.add(b)

	r.reduce()
	return r
}

/* construct this from (x,y) - but set to O if not on curve */
func NewECP2fp2s(ix *FP2,iy *FP2) *ECP2 {
	E:=new(ECP2)
	E.x=NewFP2copy(ix)
	E.y=NewFP2copy(iy)
	E.z=NewFP2int(1)
	rhs:=RHS2(E.x)
	y2:=NewFP2copy(E.y)
	y2.sqr()
	if y2.equals(rhs) {
		E.INF=false
	} else {E.x.zero();E.INF=true}
	return E
}

/* construct this from x - but set to O if not on curve */
func NewECP2fp2(ix *FP2) *ECP2 {	
	E:=new(ECP2)
	E.x=NewFP2copy(ix)
	E.y=NewFP2int(1)
	E.z=NewFP2int(1)
	rhs:=RHS2(E.x)
	if rhs.sqrt() {
			E.y.copy(rhs)
			E.INF=false;
	} else {E.x.zero();E.INF=true}
	return E
}

/* this+=this */
func (E *ECP2) dbl() int {
	if E.INF {return -1}
	if E.y.iszilch() {
		E.inf()
		return -1
	}

	w1:=NewFP2copy(E.x)
	w2:=NewFP2int(0)
	w3:=NewFP2copy(E.x)
	w8:=NewFP2copy(E.x)

	w1.sqr()
	w8.copy(w1)
	w8.imul(3)

	w2.copy(E.y); w2.sqr()
	w3.copy(E.x); w3.mul(w2)
	w3.imul(4)
	w1.copy(w3); w1.neg()
	w1.norm();

	E.x.copy(w8); E.x.sqr()
	E.x.add(w1)
	E.x.add(w1)
	E.x.norm()

	E.z.mul(E.y)
	E.z.add(E.z)

	w2.add(w2)
	w2.sqr()
	w2.add(w2)
	w3.sub(E.x);
	E.y.copy(w8); E.y.mul(w3)
	//	w2.norm();
	E.y.sub(w2)

	E.y.norm()
	E.z.norm()

	return 1
}

/* this+=Q - return 0 for add, 1 for double, -1 for O */
func (E *ECP2) add(Q *ECP2) int {
	if E.INF {
		E.copy(Q)
		return -1
	}
	if Q.INF {return -1}

	aff:=false

	if Q.z.isunity() {aff=true}

	var A,C *FP2
	B:=NewFP2copy(E.z)
	D:=NewFP2copy(E.z)
	if !aff{
		A=NewFP2copy(Q.z)
		C=NewFP2copy(Q.z)

		A.sqr(); B.sqr()
		C.mul(A); D.mul(B)

		A.mul(E.x)
		C.mul(E.y)
	} else {
		A=NewFP2copy(E.x)
		C=NewFP2copy(E.y)
	
		B.sqr()
		D.mul(B)
	}

	B.mul(Q.x); B.sub(A)
	D.mul(Q.y); D.sub(C)

	if B.iszilch() {
		if D.iszilch() {
			E.dbl()
			return 1
		} else	{
			E.INF=true
			return -1
		}
	}

	if !aff {E.z.mul(Q.z)}
	E.z.mul(B)

	e:=NewFP2copy(B); e.sqr()
	B.mul(e)
	A.mul(e)

	e.copy(A)
	e.add(A); e.add(B)
	E.x.copy(D); E.x.sqr(); E.x.sub(e)

	A.sub(E.x);
	E.y.copy(A); E.y.mul(D)
	C.mul(B); E.y.sub(C)

	E.x.norm()
	E.y.norm()
	E.z.norm()

	return 0
}

/* set this-=Q */
func (E *ECP2) sub(Q *ECP2) int {
	Q.neg()
	D:=E.add(Q)
	Q.neg()
	return D
}
/* set this*=q, where q is Modulus, using Frobenius */
func (E *ECP2) frob(X *FP2) {
	if E.INF {return}
	X2:=NewFP2copy(X)
	X2.sqr()
	E.x.conj()
	E.y.conj()
	E.z.conj()
	E.z.reduce();
	E.x.mul(X2)
	E.y.mul(X2)
	E.y.mul(X)
}

/* normalises m-array of ECP2 points. Requires work vector of m FP2s */

func multiaffine2(m int,P []*ECP2) {
	t1:=NewFP2int(0)
	t2:=NewFP2int(0)

	var work []*FP2

	for i:=0;i<m;i++ {
		work=append(work,NewFP2int(0))
	}

	work[0].one()
	work[1].copy(P[0].z)

	for i:=2;i<m;i++ {
		work[i].copy(work[i-1])
		work[i].mul(P[i-1].z)
	}

	t1.copy(work[m-1]); t1.mul(P[m-1].z)

	t1.inverse()

	t2.copy(P[m-1].z)
	work[m-1].mul(t1)

	for i:=m-2;;i-- {
		if i==0 {
			work[0].copy(t1)
			work[0].mul(t2)
			break
		}
		work[i].mul(t2);
		work[i].mul(t1);
		t2.mul(P[i].z);
	}
/* now work[] contains inverses of all Z coordinates */

	for i:=0;i<m;i++ {
		P[i].z.one();
		t1.copy(work[i]); t1.sqr()
		P[i].x.mul(t1)
		t1.mul(work[i])
		P[i].y.mul(t1)
	}    
}

/* P*=e */
func (E *ECP2) mul(e *BIG) *ECP2 {
/* fixed size windows */
	mt:=NewBIG()
	t:=NewBIG()
	P:=NewECP2()
	Q:=NewECP2()
	C:=NewECP2()

	if E.is_infinity() {return NewECP2()}

	var W []*ECP2
	var w [1+(NLEN*int(BASEBITS)+3)/4]int8

	E.affine()

/* precompute table */
	Q.copy(E)
	Q.dbl()
		
	W=append(W,NewECP2())
	W[0].copy(E);

	for i:=1;i<8;i++ {
		W=append(W,NewECP2())
		W[i].copy(W[i-1])
		W[i].add(Q)
	}

/* convert the table to affine */

	multiaffine2(8,W[:])

/* make exponent odd - add 2P if even, P if odd */
	t.copy(e)
	s:=int(t.parity())
	t.inc(1); t.norm(); ns:=int(t.parity()); mt.copy(t); mt.inc(1); mt.norm()
	t.cmove(mt,s)
	Q.cmove(E,ns)
	C.copy(Q)

	nb:=1+(t.nbits()+3)/4
/* convert exponent to signed 4-bit window */
	for i:=0;i<nb;i++ {
		w[i]=int8(t.lastbits(5)-16)
		t.dec(int(w[i])); t.norm()
		t.fshr(4)	
	}
	w[nb]=int8(t.lastbits(5))
		
	P.copy(W[(w[nb]-1)/2])
	for i:=nb-1;i>=0;i-- {
		Q.selector(W,int32(w[i]))
		P.dbl()
		P.dbl()
		P.dbl()
		P.dbl()
		P.add(Q)
	}
	P.sub(C)
	P.affine()
	return P
}

/* P=u0.Q0+u1*Q1+u2*Q2+u3*Q3 */
func mul4(Q []*ECP2,u []*BIG) *ECP2 {
	var a [4]int8
	T:=NewECP2()
	C:=NewECP2()
	P:=NewECP2()

	var W [] *ECP2

	mt:=NewBIG()
	var t []*BIG

	var w [NLEN*int(BASEBITS)+1]int8	

	for i:=0;i<4;i++ {
		t=append(t,NewBIGcopy(u[i]));
		Q[i].affine();
	}

/* precompute table */

	W=append(W,NewECP2()); W[0].copy(Q[0]); W[0].sub(Q[1])
	W=append(W,NewECP2()); W[1].copy(W[0])
	W=append(W,NewECP2()); W[2].copy(W[0])
	W=append(W,NewECP2()); W[3].copy(W[0])
	W=append(W,NewECP2()); W[4].copy(Q[0]); W[4].add(Q[1])
	W=append(W,NewECP2()); W[5].copy(W[4])
	W=append(W,NewECP2()); W[6].copy(W[4])
	W=append(W,NewECP2()); W[7].copy(W[4])

	T.copy(Q[2]); T.sub(Q[3])
	W[1].sub(T)
	W[2].add(T)
	W[5].sub(T)
	W[6].add(T)
	T.copy(Q[2]); T.add(Q[3])
	W[0].sub(T)
	W[3].add(T)
	W[4].sub(T)
	W[7].add(T)

	multiaffine2(8,W[:])

/* if multiplier is even add 1 to multiplier, and add P to correction */
	mt.zero(); C.inf()
	for i:=0;i<4;i++ {
		if t[i].parity()==0 {
			t[i].inc(1); t[i].norm()
			C.add(Q[i])
		}
		mt.add(t[i]); mt.norm()
	}

	nb:=1+mt.nbits();

/* convert exponent to signed 1-bit window */
	for j:=0;j<nb;j++ {
		for i:=0;i<4;i++ {
			a[i]=int8(t[i].lastbits(2)-2)
			t[i].dec(int(a[i])); t[i].norm()
			t[i].fshr(1)
		}
		w[j]=(8*a[0]+4*a[1]+2*a[2]+a[3])
	}
	w[nb]=int8(8*t[0].lastbits(2)+4*t[1].lastbits(2)+2*t[2].lastbits(2)+t[3].lastbits(2))

	P.copy(W[(w[nb]-1)/2])  
	for i:=nb-1;i>=0;i-- {
		T.selector(W,int32(w[i]))
		P.dbl()
		P.add(T)
	}
	P.sub(C) /* apply correction */

	P.affine()
	return P
}

