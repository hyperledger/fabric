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

/* MiotCL BN Curve Pairing functions */

package amcl

//import "fmt"

/* Line function */
func line(A *ECP2,B *ECP2,Qx *FP,Qy *FP) *FP12 {
	P:=NewECP2()

	P.copy(A);
	ZZ:=NewFP2copy(P.getz())
	ZZ.sqr()
	var D int
	if A==B {
		D=A.dbl() 
	} else {D=A.add(B)}

	if D<0 {return NewFP12int(1)}

	Z3:=NewFP2copy(A.getz())

	var a *FP4
	var b *FP4
	c:=NewFP4int(0)

	if (D==0) { /* Addition */
		X:=NewFP2copy(B.getx())
		Y:=NewFP2copy(B.gety())
		T:=NewFP2copy(P.getz()) 
		T.mul(Y)
		ZZ.mul(T)

		NY:=NewFP2copy(P.gety()); NY.neg()
		ZZ.add(NY)
		Z3.pmul(Qy)
		T.mul(P.getx());
		X.mul(NY);
		T.add(X);
		a=NewFP4fp2s(Z3,T)
		ZZ.neg();
		ZZ.pmul(Qx)
		b=NewFP4fp2(ZZ)
	} else { /* Doubling */
		X:=NewFP2copy(P.getx())
		Y:=NewFP2copy(P.gety())
		T:=NewFP2copy(P.getx())
		T.sqr()
		T.imul(3)

		Y.sqr()
		Y.add(Y)
		Z3.mul(ZZ)
		Z3.pmul(Qy)

		X.mul(T)
		X.sub(Y)
		a=NewFP4fp2s(Z3,X)
		T.neg()
		ZZ.mul(T)
		ZZ.pmul(Qx)
		b=NewFP4fp2(ZZ)
	}
	return NewFP12fp4s(a,b,c)
}

/* Optimal R-ate pairing */
func ate(P *ECP2,Q *ECP) *FP12 {
	f:=NewFP2bigs(NewBIGints(CURVE_Fra),NewBIGints(CURVE_Frb))
	x:=NewBIGints(CURVE_Bnx)
	n:=NewBIGcopy(x)
	K:=NewECP2()
	var lv *FP12
	
	if CURVE_PAIRING_TYPE == BN_CURVE {
		n.pmul(6); n.dec(2)
	} else {n.copy(x)}
	
	n.norm()
	P.affine()
	Q.affine()
	Qx:=NewFPcopy(Q.getx())
	Qy:=NewFPcopy(Q.gety())

	A:=NewECP2()
	r:=NewFP12int(1)

	A.copy(P)
	nb:=n.nbits()

	for i:=nb-2;i>=1;i-- {
		lv=line(A,A,Qx,Qy)
		r.smul(lv)
		if n.bit(i)==1 {
	
			lv=line(A,P,Qx,Qy)
			
			r.smul(lv)
		}		
		r.sqr()
	}

	lv=line(A,A,Qx,Qy)
	r.smul(lv)

	if n.parity()==1 {
		lv=line(A,P,Qx,Qy)
		r.smul(lv)
	}

/* R-ate fixup required for BN curves */

	if CURVE_PAIRING_TYPE == BN_CURVE {
		r.conj()
		K.copy(P)
		K.frob(f)
		A.neg()
		lv=line(A,K,Qx,Qy)
		r.smul(lv)
		K.frob(f)
		K.neg()
		lv=line(A,K,Qx,Qy)
		r.smul(lv)
	}

	return r
}

/* Optimal R-ate double pairing e(P,Q).e(R,S) */
func ate2(P *ECP2,Q *ECP,R *ECP2,S *ECP) *FP12 {
	f:=NewFP2bigs(NewBIGints(CURVE_Fra),NewBIGints(CURVE_Frb))
	x:=NewBIGints(CURVE_Bnx)
	n:=NewBIGcopy(x)
	K:=NewECP2()
	var lv *FP12

	if CURVE_PAIRING_TYPE == BN_CURVE {
		n.pmul(6); n.dec(2)
	} else {n.copy(x)}
	
	n.norm()
	P.affine()
	Q.affine()
	R.affine()
	S.affine()

	Qx:=NewFPcopy(Q.getx())
	Qy:=NewFPcopy(Q.gety())
	Sx:=NewFPcopy(S.getx())
	Sy:=NewFPcopy(S.gety())

	A:=NewECP2()
	B:=NewECP2()
	r:=NewFP12int(1)

	A.copy(P)
	B.copy(R)
	nb:=n.nbits()

	for i:=nb-2;i>=1;i-- {
		lv=line(A,A,Qx,Qy)
		r.smul(lv)
		lv=line(B,B,Sx,Sy)
		r.smul(lv)

		if n.bit(i)==1 {
			lv=line(A,P,Qx,Qy)
			r.smul(lv)
			lv=line(B,R,Sx,Sy)
			r.smul(lv)
		}
		r.sqr()
	}

	lv=line(A,A,Qx,Qy)
	r.smul(lv)
	lv=line(B,B,Sx,Sy)
	r.smul(lv)
	if n.parity()==1 {
		lv=line(A,P,Qx,Qy)
		r.smul(lv)
		lv=line(B,R,Sx,Sy)
		r.smul(lv)
	}

/* R-ate fixup */
	if CURVE_PAIRING_TYPE == BN_CURVE {
		r.conj()
		K.copy(P)
		K.frob(f)
		A.neg()
		lv=line(A,K,Qx,Qy)
		r.smul(lv)
		K.frob(f)
		K.neg()
		lv=line(A,K,Qx,Qy)
		r.smul(lv)

		K.copy(R)
		K.frob(f)
		B.neg()
		lv=line(B,K,Sx,Sy)
		r.smul(lv)
		K.frob(f)
		K.neg()
		lv=line(B,K,Sx,Sy)
		r.smul(lv)
	}

	return r
}

/* final exponentiation - keep separate for multi-pairings and to avoid thrashing stack */
func fexp(m *FP12) *FP12 {
	f:=NewFP2bigs(NewBIGints(CURVE_Fra),NewBIGints(CURVE_Frb))
	x:=NewBIGints(CURVE_Bnx)
	r:=NewFP12copy(m)
		
/* Easy part of final exp */
	lv:=NewFP12copy(r)
	lv.inverse()
	r.conj()

	r.mul(lv)
	lv.copy(r)
	r.frob(f)
	r.frob(f)
	r.mul(lv)
/* Hard part of final exp */
	if CURVE_PAIRING_TYPE == BN_CURVE {
		lv.copy(r)
		lv.frob(f)
		x0:=NewFP12copy(lv)
		x0.frob(f)
		lv.mul(r)
		x0.mul(lv)
		x0.frob(f)
		x1:=NewFP12copy(r)
		x1.conj()
		x4:=r.pow(x)

		x3:=NewFP12copy(x4)
		x3.frob(f)

		x2:=x4.pow(x)

		x5:=NewFP12copy(x2); x5.conj()
		lv=x2.pow(x)

		x2.frob(f)
		r.copy(x2); r.conj()

		x4.mul(r)
		x2.frob(f)

		r.copy(lv)
		r.frob(f)
		lv.mul(r)

		lv.usqr()
		lv.mul(x4)
		lv.mul(x5)
		r.copy(x3)
		r.mul(x5)
		r.mul(lv)
		lv.mul(x2)
		r.usqr()
		r.mul(lv)
		r.usqr()
		lv.copy(r)
		lv.mul(x1)
		r.mul(x0)
		lv.usqr()
		r.mul(lv)
		r.reduce()
	} else {
		
// Ghamman & Fouotsa Method
		y0:=NewFP12copy(r); y0.usqr()
		y1:=y0.pow(x)
		x.fshr(1); y2:=y1.pow(x); x.fshl(1)
		y3:=NewFP12copy(r); y3.conj()
		y1.mul(y3)

		y1.conj()
		y1.mul(y2)

		y2=y1.pow(x)

		y3=y2.pow(x)
		y1.conj()
		y3.mul(y1)

		y1.conj();
		y1.frob(f); y1.frob(f); y1.frob(f)
		y2.frob(f); y2.frob(f)
		y1.mul(y2)

		y2=y3.pow(x)
		y2.mul(y0)
		y2.mul(r)

		y1.mul(y2)
		y2.copy(y3); y2.frob(f)
		y1.mul(y2)
		r.copy(y1)
		r.reduce()


/*
		x0:=NewFP12copy(r)
		x1:=NewFP12copy(r)
		lv.copy(r); lv.frob(f)
		x3:=NewFP12copy(lv); x3.conj(); x1.mul(x3)
		lv.frob(f); lv.frob(f)
		x1.mul(lv)

		r.copy(r.pow(x))  //r=r.pow(x);
		x3.copy(r); x3.conj(); x1.mul(x3)
		lv.copy(r); lv.frob(f)
		x0.mul(lv)
		lv.frob(f)
		x1.mul(lv)
		lv.frob(f)
		x3.copy(lv); x3.conj(); x0.mul(x3)

		r.copy(r.pow(x))
		x0.mul(r)
		lv.copy(r); lv.frob(f); lv.frob(f)
		x3.copy(lv); x3.conj(); x0.mul(x3)
		lv.frob(f)
		x1.mul(lv)

		r.copy(r.pow(x))
		lv.copy(r); lv.frob(f)
		x3.copy(lv); x3.conj(); x0.mul(x3)
		lv.frob(f)
		x1.mul(lv)

		r.copy(r.pow(x))
		x3.copy(r); x3.conj(); x0.mul(x3)
		lv.copy(r); lv.frob(f)
		x1.mul(lv)

		r.copy(r.pow(x))
		x1.mul(r)

		x0.usqr()
		x0.mul(x1)
		r.copy(x0)
		r.reduce() */
	}
	return r
}

/* GLV method */
func glv(e *BIG) []*BIG {
	var u []*BIG
	if CURVE_PAIRING_TYPE == BN_CURVE {
		t:=NewBIGint(0)
		q:=NewBIGints(CURVE_Order)
		var v []*BIG

		for i:=0;i<2;i++ {
			t.copy(NewBIGints(CURVE_W[i]))  // why not just t=new BIG(ROM.CURVE_W[i]); 
			d:=mul(t,e)
			v=append(v,NewBIGcopy(d.div(q)))
			u=append(u,NewBIGint(0))
		}
		u[0].copy(e)
		for i:=0;i<2;i++ {
			for j:=0;j<2;j++ {
				t.copy(NewBIGints(CURVE_SB[j][i]))
				t.copy(modmul(v[j],t,q))
				u[i].add(q)
				u[i].sub(t)
				u[i].mod(q)
			}
		}
	} else {
		q:=NewBIGints(CURVE_Order)
		x:=NewBIGints(CURVE_Bnx)
		x2:=smul(x,x)
		u=append(u,NewBIGcopy(e))
		u[0].mod(x2)
		u=append(u,NewBIGcopy(e))
		u[1].div(x2)
		u[1].rsub(q)
	}
	return u
}

/* Galbraith & Scott Method */
func gs(e *BIG) []*BIG {
	var u []*BIG
	if CURVE_PAIRING_TYPE == BN_CURVE {
		t:=NewBIGint(0)
		q:=NewBIGints(CURVE_Order)

		var v []*BIG
		for i:=0;i<4;i++ {
			t.copy(NewBIGints(CURVE_WB[i]))
			d:=mul(t,e)
			v=append(v,NewBIGcopy(d.div(q)))
			u=append(u,NewBIGint(0))
		}
		u[0].copy(e)
		for i:=0;i<4;i++ {
			for j:=0;j<4;j++ {
				t.copy(NewBIGints(CURVE_BB[j][i]))
				t.copy(modmul(v[j],t,q))
				u[i].add(q)
				u[i].sub(t)
				u[i].mod(q)
			}
		}
	} else {
		x:=NewBIGints(CURVE_Bnx)
		w:=NewBIGcopy(e)
		for i:=0;i<4;i++ {
			u=append(u,NewBIGcopy(w))
			u[i].mod(x)
			w.div(x)
		}
	}
	return u
}	

/* Multiply P by e in group G1 */
func G1mul(P *ECP,e *BIG) *ECP {
	var R *ECP
	if (USE_GLV) {
		P.affine()
		R=NewECP()
		R.copy(P)
		Q:=NewECP()
		Q.copy(P)
		q:=NewBIGints(CURVE_Order);
		cru:=NewFPbig(NewBIGints(CURVE_Cru))
		t:=NewBIGint(0)
		u:=glv(e)
		Q.getx().mul(cru)

		np:=u[0].nbits()
		t.copy(modneg(u[0],q))
		nn:=t.nbits()
		if nn<np {
			u[0].copy(t)
			R.neg()
		}

		np=u[1].nbits()
		t.copy(modneg(u[1],q))
		nn=t.nbits()
		if nn<np {
			u[1].copy(t)
			Q.neg()
		}

		R=R.mul2(u[0],Q,u[1])
			
	} else {
		R=P.mul(e)
	}
	return R
}

/* Multiply P by e in group G2 */
func G2mul(P *ECP2,e *BIG) *ECP2 {
	var R *ECP2
	if (USE_GS_G2) {
		var Q []*ECP2
		f:=NewFP2bigs(NewBIGints(CURVE_Fra),NewBIGints(CURVE_Frb))
		q:=NewBIGints(CURVE_Order)
		u:=gs(e)

		t:=NewBIGint(0)
		P.affine()
		Q=append(Q,NewECP2());  Q[0].copy(P);
		for i:=1;i<4;i++ {
			Q=append(Q,NewECP2()); Q[i].copy(Q[i-1])
			Q[i].frob(f)
		}
		for i:=0;i<4;i++ {
			np:=u[i].nbits()
			t.copy(modneg(u[i],q))
			nn:=t.nbits()
			if nn<np {
				u[i].copy(t)
				Q[i].neg()
			}
		}

		R=mul4(Q,u)

	} else {
		R=P.mul(e)
	}
	return R
}

/* f=f^e */
/* Note that this method requires a lot of RAM! Better to use compressed XTR method, see FP4.java */
func GTpow(d *FP12,e *BIG) *FP12 {
	var r *FP12
	if USE_GS_GT {
		var g []*FP12
		f:=NewFP2bigs(NewBIGints(CURVE_Fra),NewBIGints(CURVE_Frb))
		q:=NewBIGints(CURVE_Order)
		t:=NewBIGint(0)
	
		u:=gs(e)

		g=append(g,NewFP12copy(d))
		for i:=1;i<4;i++ {
			g=append(g,NewFP12int(0))
			g[i].copy(g[i-1])
			g[i].frob(f)
		}
		for i:=0;i<4;i++ {
			np:=u[i].nbits()
			t.copy(modneg(u[i],q))
			nn:=t.nbits()
			if nn<np {
				u[i].copy(t)
				g[i].conj()
			}
		}
		r=pow4(g,u)
	} else {
		r=d.pow(e)
	}
	return r
}

/* test group membership - no longer needed*/
/* with GT-Strong curve, now only check that m!=1, conj(m)*m==1, and m.m^{p^4}=m^{p^2} */
/*
func GTmember(m *FP12) bool {
	if m.isunity() {return false}
	r:=NewFP12copy(m)
	r.conj()
	r.mul(m)
	if !r.isunity() {return false}

	f:=NewFP2bigs(NewBIGints(CURVE_Fra),NewBIGints(CURVE_Frb))

	r.copy(m); r.frob(f); r.frob(f)
	w:=NewFP12copy(r); w.frob(f); w.frob(f)
	w.mul(m)
	if !GT_STRONG {
		if !w.equals(r) {return false}
		x:=NewBIGints(CURVE_Bnx);
		r.copy(m); w=r.pow(x); w=w.pow(x)
		r.copy(w); r.sqr(); r.mul(w); r.sqr()
		w.copy(m); w.frob(f)
	}
	return w.equals(r)
}
*/
/*
func main() {

	Q:=NewECPbigs(NewBIGints(CURVE_Gx),NewBIGints(CURVE_Gy))
	P:=NewECP2fp2s(NewFP2bigs(NewBIGints(CURVE_Pxa),NewBIGints(CURVE_Pxb)),NewFP2bigs(NewBIGints(CURVE_Pya),NewBIGints(CURVE_Pyb)))

	//r:=NewBIGints(CURVE_Order)
	//xa:=NewBIGints(CURVE_Pxa)

	fmt.Printf("P= "+P.toString())
	fmt.Printf("\n");
	fmt.Printf("Q= "+Q.toString());
	fmt.Printf("\n");

	//m:=NewBIGint(17)

	e:=ate(P,Q)
	e=fexp(e)
	for i:=1;i<1000;i++ {
		e=ate(P,Q)
//	fmt.Printf("\ne= "+e.toString())
//	fmt.Printf("\n")

		e=fexp(e)
	}
	//	e=GTpow(e,m);

	fmt.Printf("\ne= "+e.toString())
	fmt.Printf("\n");
	GLV:=glv(r)

	fmt.Printf("GLV[0]= "+GLV[0].toString())
	fmt.Printf("\n")

	fmt.Printf("GLV[0]= "+GLV[1].toString())
	fmt.Printf("\n")

	G:=NewECP(); G.copy(Q)
	R:=NewECP2(); R.copy(P)


	e=ate(R,Q)
	e=fexp(e)

	e=GTpow(e,xa)
	fmt.Printf("\ne= "+e.toString());
	fmt.Printf("\n")

	R=G2mul(R,xa)
	e=ate(R,G)
	e=fexp(e)

	fmt.Printf("\ne= "+e.toString())
	fmt.Printf("\n")

	G=G1mul(G,xa)
	e=ate(P,G)
	e=fexp(e)
	fmt.Printf("\ne= "+e.toString())
	fmt.Printf("\n") 
}
*/
