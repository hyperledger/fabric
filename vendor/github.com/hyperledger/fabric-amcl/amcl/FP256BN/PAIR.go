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

package FP256BN



/* Line function */
func line(A *ECP2,B *ECP2,Qx *FP,Qy *FP) *FP12 {
	var a *FP4
	var b *FP4
	var c *FP4

	if (A==B) { /* Doubling */
		XX:=NewFP2copy(A.getx())  //X
		YY:=NewFP2copy(A.gety())  //Y
		ZZ:=NewFP2copy(A.getz())  //Z
		YZ:=NewFP2copy(YY)        //Y 
		YZ.mul(ZZ)                //YZ
		XX.sqr()			//X^2
		YY.sqr()			//Y^2
		ZZ.sqr()			//Z^2
			
		YZ.imul(4)
		YZ.neg(); YZ.norm()       //-4YZ
		YZ.pmul(Qy);               //-4YZ.Ys

		XX.imul(6)                //6X^2
		XX.pmul(Qx);              //6X^2.Xs

		sb:=3*CURVE_B_I
		ZZ.imul(sb); 			// 3bZ^2
		if SEXTIC_TWIST == D_TYPE {	
			ZZ.div_ip2();
		}
		if SEXTIC_TWIST == M_TYPE {
			ZZ.mul_ip();
			ZZ.add(ZZ);
			YZ.mul_ip();
			YZ.norm();
		}
		ZZ.norm() // 3b.Z^2 

		YY.add(YY)
		ZZ.sub(YY); ZZ.norm()     // 3b.Z^2-2Y^2

		a=NewFP4fp2s(YZ,ZZ);          // -4YZ.Ys | 3b.Z^2-2Y^2 | 6X^2.Xs 
		if SEXTIC_TWIST == D_TYPE {	

			b=NewFP4fp2(XX)             // L(0,1) | L(0,0) | L(1,0)
			c=NewFP4int(0)
		}
		if SEXTIC_TWIST == M_TYPE {
			b=NewFP4int(0)
			c=NewFP4fp2(XX); c.times_i()
		}
		A.dbl();

	} else { /* Addition */

		X1:=NewFP2copy(A.getx())    // X1
		Y1:=NewFP2copy(A.gety())    // Y1
		T1:=NewFP2copy(A.getz())    // Z1
		T2:=NewFP2copy(A.getz())    // Z1
			
		T1.mul(B.gety())    // T1=Z1.Y2 
		T2.mul(B.getx())    // T2=Z1.X2

		X1.sub(T2); X1.norm()  // X1=X1-Z1.X2
		Y1.sub(T1); Y1.norm()  // Y1=Y1-Z1.Y2

		T1.copy(X1)            // T1=X1-Z1.X2
		X1.pmul(Qy)            // X1=(X1-Z1.X2).Ys

		if SEXTIC_TWIST == M_TYPE {
			X1.mul_ip()
			X1.norm()
		}

		T1.mul(B.gety())       // T1=(X1-Z1.X2).Y2

		T2.copy(Y1)           // T2=Y1-Z1.Y2
		T2.mul(B.getx())       // T2=(Y1-Z1.Y2).X2
		T2.sub(T1); T2.norm()          // T2=(Y1-Z1.Y2).X2 - (X1-Z1.X2).Y2
		Y1.pmul(Qx);  Y1.neg(); Y1.norm() // Y1=-(Y1-Z1.Y2).Xs

		a=NewFP4fp2s(X1,T2)       // (X1-Z1.X2).Ys  |  (Y1-Z1.Y2).X2 - (X1-Z1.X2).Y2  | - (Y1-Z1.Y2).Xs
		if SEXTIC_TWIST == D_TYPE {
			b=NewFP4fp2(Y1)
			c=NewFP4int(0)
		}
		if SEXTIC_TWIST == M_TYPE {
			b=NewFP4int(0)
			c=NewFP4fp2(Y1); c.times_i()
		}
		A.Add(B);
	}


	return NewFP12fp4s(a,b,c)
}

/* Optimal R-ate pairing */
func Ate(P1 *ECP2,Q1 *ECP) *FP12 {
	f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))
	x:=NewBIGints(CURVE_Bnx)
	n:=NewBIGcopy(x)
	K:=NewECP2()
	var lv *FP12
	
	if CURVE_PAIRING_TYPE == BN {
		if SEXTIC_TWIST==M_TYPE {
			f.inverse();
			f.norm();
		}
		n.pmul(6)
		if SIGN_OF_X==POSITIVEX {
			n.inc(2)
		} else {
			n.dec(2)
		}
	} else {n.copy(x)}
	
	n.norm()

	n3:=NewBIGcopy(n);
	n3.pmul(3);
	n3.norm();

	P:=NewECP2(); P.Copy(P1); P.Affine()
	Q:=NewECP(); Q.Copy(Q1); Q.Affine()


	Qx:=NewFPcopy(Q.getx())
	Qy:=NewFPcopy(Q.gety())

	A:=NewECP2()
	r:=NewFP12int(1)

	A.Copy(P)

	NP:=NewECP2()
	NP.Copy(P)
	NP.neg()

	nb:=n3.nbits()

	for i:=nb-2;i>=1;i-- {
		r.sqr()
		lv=line(A,A,Qx,Qy)
		r.smul(lv,SEXTIC_TWIST)
		bt:=n3.bit(i)-n.bit(i);
		if bt==1 {
			lv=line(A,P,Qx,Qy)
			r.smul(lv,SEXTIC_TWIST)
		}	
		if bt==-1 {
			//P.neg()
			lv=line(A,NP,Qx,Qy)
			r.smul(lv,SEXTIC_TWIST)
			//P.neg()
		}		
	}

	if SIGN_OF_X==NEGATIVEX {
		r.conj()
	}


/* R-ate fixup required for BN curves */

	if CURVE_PAIRING_TYPE == BN {
		if SIGN_OF_X==NEGATIVEX {
			//r.conj()
			A.neg()
		}

		K.Copy(P)
		K.frob(f)
		lv=line(A,K,Qx,Qy)
		r.smul(lv,SEXTIC_TWIST)
		K.frob(f)
		K.neg()
		lv=line(A,K,Qx,Qy)
		r.smul(lv,SEXTIC_TWIST)
	} 

	return r
}

/* Optimal R-ate double pairing e(P,Q).e(R,S) */
func Ate2(P1 *ECP2,Q1 *ECP,R1 *ECP2,S1 *ECP) *FP12 {
	f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))
	x:=NewBIGints(CURVE_Bnx)
	n:=NewBIGcopy(x)
	K:=NewECP2()
	var lv *FP12

	if CURVE_PAIRING_TYPE == BN {
		if SEXTIC_TWIST==M_TYPE {
			f.inverse();
			f.norm();
		}
		n.pmul(6); 
		if SIGN_OF_X==POSITIVEX {
			n.inc(2)
		} else {
			n.dec(2)
		}
	} else {n.copy(x)}
	
	n.norm()

	n3:=NewBIGcopy(n);
	n3.pmul(3);
	n3.norm();

	P:=NewECP2(); P.Copy(P1); P.Affine()
	Q:=NewECP(); Q.Copy(Q1); Q.Affine()
	R:=NewECP2(); R.Copy(R1); R.Affine()
	S:=NewECP(); S.Copy(S1); S.Affine()


	Qx:=NewFPcopy(Q.getx())
	Qy:=NewFPcopy(Q.gety())
	Sx:=NewFPcopy(S.getx())
	Sy:=NewFPcopy(S.gety())

	A:=NewECP2()
	B:=NewECP2()
	r:=NewFP12int(1)

	A.Copy(P)
	B.Copy(R)
	NP:=NewECP2()
	NP.Copy(P)
	NP.neg()
	NR:=NewECP2()
	NR.Copy(R)
	NR.neg()


	nb:=n3.nbits()

	for i:=nb-2;i>=1;i-- {
		r.sqr()
		lv=line(A,A,Qx,Qy)
		r.smul(lv,SEXTIC_TWIST)
		lv=line(B,B,Sx,Sy)
		r.smul(lv,SEXTIC_TWIST)
		bt:=n3.bit(i)-n.bit(i);
		if bt==1 {
			lv=line(A,P,Qx,Qy)
			r.smul(lv,SEXTIC_TWIST)
			lv=line(B,R,Sx,Sy)
			r.smul(lv,SEXTIC_TWIST)
		}
		if bt==-1 {
			//P.neg(); 
			lv=line(A,NP,Qx,Qy)
			r.smul(lv,SEXTIC_TWIST)
			//P.neg(); 
			//R.neg()
			lv=line(B,NR,Sx,Sy)
			r.smul(lv,SEXTIC_TWIST)
			//R.neg()
		}
	}

	if SIGN_OF_X==NEGATIVEX {
		r.conj()
	}

/* R-ate fixup */
	if CURVE_PAIRING_TYPE == BN {
		if SIGN_OF_X==NEGATIVEX {
		//	r.conj()
			A.neg()
			B.neg()
		}
		K.Copy(P)
		K.frob(f)

		lv=line(A,K,Qx,Qy)
		r.smul(lv,SEXTIC_TWIST)
		K.frob(f)
		K.neg()
		lv=line(A,K,Qx,Qy)
		r.smul(lv,SEXTIC_TWIST)

		K.Copy(R)
		K.frob(f)

		lv=line(B,K,Sx,Sy)
		r.smul(lv,SEXTIC_TWIST)
		K.frob(f)
		K.neg()
		lv=line(B,K,Sx,Sy)
		r.smul(lv,SEXTIC_TWIST)
	}

	return r
}

/* final exponentiation - keep separate for multi-pairings and to avoid thrashing stack */
func Fexp(m *FP12) *FP12 {
	f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))
	x:=NewBIGints(CURVE_Bnx)
	r:=NewFP12copy(m)
		
/* Easy part of final exp */
	lv:=NewFP12copy(r)
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
		x0:=NewFP12copy(lv)
		x0.frob(f)
		lv.Mul(r)
		x0.Mul(lv)
		x0.frob(f)
		x1:=NewFP12copy(r)
		x1.conj()
		x4:=r.Pow(x)
		if SIGN_OF_X==POSITIVEX {
			x4.conj();
		}

		x3:=NewFP12copy(x4)
		x3.frob(f)

		x2:=x4.Pow(x)
		if SIGN_OF_X==POSITIVEX {
			x2.conj();
		}

		x5:=NewFP12copy(x2); x5.conj()
		lv=x2.Pow(x)
		if SIGN_OF_X==POSITIVEX {
			lv.conj();
		}

		x2.frob(f)
		r.Copy(x2); r.conj()

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
		
// Ghamman & Fouotsa Method
		y0:=NewFP12copy(r); y0.usqr()
		y1:=y0.Pow(x)
		if SIGN_OF_X==NEGATIVEX {
			y1.conj();
		}

		x.fshr(1); y2:=y1.Pow(x); 
		if SIGN_OF_X==NEGATIVEX {
			y2.conj();
		}
		
		x.fshl(1)
		y3:=NewFP12copy(r); y3.conj()
		y1.Mul(y3)

		y1.conj()
		y1.Mul(y2)

		y2=y1.Pow(x)
		if SIGN_OF_X==NEGATIVEX {
			y2.conj();
		}


		y3=y2.Pow(x)
		if SIGN_OF_X==NEGATIVEX {
			y3.conj();
		}

		y1.conj()
		y3.Mul(y1)

		y1.conj();
		y1.frob(f); y1.frob(f); y1.frob(f)
		y2.frob(f); y2.frob(f)
		y1.Mul(y2)

		y2=y3.Pow(x)
		if SIGN_OF_X==NEGATIVEX {
			y2.conj();
		}

		y2.Mul(y0)
		y2.Mul(r)

		y1.Mul(y2)
		y2.Copy(y3); y2.frob(f)
		y1.Mul(y2)
		r.Copy(y1)
		r.reduce()


/*
		x0:=NewFP12copy(r)
		x1:=NewFP12copy(r)
		lv.Copy(r); lv.frob(f)
		x3:=NewFP12copy(lv); x3.conj(); x1.Mul(x3)
		lv.frob(f); lv.frob(f)
		x1.Mul(lv)

		r.Copy(r.Pow(x))  //r=r.Pow(x);
		x3.Copy(r); x3.conj(); x1.Mul(x3)
		lv.Copy(r); lv.frob(f)
		x0.Mul(lv)
		lv.frob(f)
		x1.Mul(lv)
		lv.frob(f)
		x3.Copy(lv); x3.conj(); x0.Mul(x3)

		r.Copy(r.Pow(x))
		x0.Mul(r)
		lv.Copy(r); lv.frob(f); lv.frob(f)
		x3.Copy(lv); x3.conj(); x0.Mul(x3)
		lv.frob(f)
		x1.Mul(lv)

		r.Copy(r.Pow(x))
		lv.Copy(r); lv.frob(f)
		x3.Copy(lv); x3.conj(); x0.Mul(x3)
		lv.frob(f)
		x1.Mul(lv)

		r.Copy(r.Pow(x))
		x3.Copy(r); x3.conj(); x0.Mul(x3)
		lv.Copy(r); lv.frob(f)
		x1.Mul(lv)

		r.Copy(r.Pow(x))
		x1.Mul(r)

		x0.usqr()
		x0.Mul(x1)
		r.Copy(x0)
		r.reduce() */
	}
	return r
}

/* GLV method */
func glv(e *BIG) []*BIG {
	var u []*BIG
	if CURVE_PAIRING_TYPE == BN {
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
				t.copy(Modmul(v[j],t,q))
				u[i].add(q)
				u[i].sub(t)
				u[i].Mod(q)
			}
		}
	} else {
		q:=NewBIGints(CURVE_Order)
		x:=NewBIGints(CURVE_Bnx)
		x2:=smul(x,x)
		u=append(u,NewBIGcopy(e))
		u[0].Mod(x2)
		u=append(u,NewBIGcopy(e))
		u[1].div(x2)
		u[1].rsub(q)
	}
	return u
}

/* Galbraith & Scott Method */
func gs(e *BIG) []*BIG {
	var u []*BIG
	if CURVE_PAIRING_TYPE == BN {
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
				t.copy(Modmul(v[j],t,q))
				u[i].add(q)
				u[i].sub(t)
				u[i].Mod(q)
			}
		}
	} else {
		q:=NewBIGints(CURVE_Order)
		x:=NewBIGints(CURVE_Bnx)
		w:=NewBIGcopy(e)
		for i:=0;i<3;i++ {
			u=append(u,NewBIGcopy(w))
			u[i].Mod(x)
			w.div(x)
		}
		u=append(u,NewBIGcopy(w))
		if SIGN_OF_X==NEGATIVEX {
			u[1].copy(Modneg(u[1],q));
			u[3].copy(Modneg(u[3],q));
		}
	}
	return u
}	

/* Multiply P by e in group G1 */
func G1mul(P *ECP,e *BIG) *ECP {
	var R *ECP
	if (USE_GLV) {
		//P.Affine()
		R=NewECP()
		R.Copy(P)
		Q:=NewECP()
		Q.Copy(P); Q.Affine()
		q:=NewBIGints(CURVE_Order)
		cru:=NewFPbig(NewBIGints(CURVE_Cru))
		t:=NewBIGint(0)
		u:=glv(e)
		Q.getx().mul(cru)

		np:=u[0].nbits()
		t.copy(Modneg(u[0],q))
		nn:=t.nbits()
		if nn<np {
			u[0].copy(t)
			R.neg()
		}

		np=u[1].nbits()
		t.copy(Modneg(u[1],q))
		nn=t.nbits()
		if nn<np {
			u[1].copy(t)
			Q.neg()
		}
		u[0].norm()
		u[1].norm()
		R=R.Mul2(u[0],Q,u[1])
			
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
		f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))

		if SEXTIC_TWIST==M_TYPE {
			f.inverse();
			f.norm();
		}

		q:=NewBIGints(CURVE_Order)
		u:=gs(e)

		t:=NewBIGint(0)
		//P.Affine()
		Q=append(Q,NewECP2());  Q[0].Copy(P);
		for i:=1;i<4;i++ {
			Q=append(Q,NewECP2()); Q[i].Copy(Q[i-1])
			Q[i].frob(f)
		}
		for i:=0;i<4;i++ {
			np:=u[i].nbits()
			t.copy(Modneg(u[i],q))
			nn:=t.nbits()
			if nn<np {
				u[i].copy(t)
				Q[i].neg()
			}
			u[i].norm()
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
		f:=NewFP2bigs(NewBIGints(Fra),NewBIGints(Frb))
		q:=NewBIGints(CURVE_Order)
		t:=NewBIGint(0)
	
		u:=gs(e)

		g=append(g,NewFP12copy(d))
		for i:=1;i<4;i++ {
			g=append(g,NewFP12int(0))
			g[i].Copy(g[i-1])
			g[i].frob(f)
		}
		for i:=0;i<4;i++ {
			np:=u[i].nbits()
			t.copy(Modneg(u[i],q))
			nn:=t.nbits()
			if nn<np {
				u[i].copy(t)
				g[i].conj()
			}
			u[i].norm()			
		}
		r=pow4(g,u)
	} else {
		r=d.Pow(e)
	}
	return r
}

/* test group membership - no longer needed*/
/* with GT-Strong curve, now only check that m!=1, conj(m)*m==1, and m.m^{p^4}=m^{p^2} */
/*
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
	if !GT_STRONG {
		if !w.Equals(r) {return false}
		x:=NewBIGints(CURVE_Bnx);
		r.Copy(m); w=r.Pow(x); w=w.Pow(x)
		r.Copy(w); r.sqr(); r.Mul(w); r.sqr()
		w.Copy(m); w.frob(f)
	}
	return w.Equals(r)
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

	e:=Ate(P,Q)
	e=Fexp(e)
	for i:=1;i<1000;i++ {
		e=Ate(P,Q)
//	fmt.Printf("\ne= "+e.toString())
//	fmt.Printf("\n")

		e=Fexp(e)
	}
	//	e=GTpow(e,m);

	fmt.Printf("\ne= "+e.toString())
	fmt.Printf("\n");
	GLV:=glv(r)

	fmt.Printf("GLV[0]= "+GLV[0].toString())
	fmt.Printf("\n")

	fmt.Printf("GLV[0]= "+GLV[1].toString())
	fmt.Printf("\n")

	G:=NewECP(); G.Copy(Q)
	R:=NewECP2(); R.Copy(P)


	e=Ate(R,Q)
	e=Fexp(e)

	e=GTpow(e,xa)
	fmt.Printf("\ne= "+e.toString());
	fmt.Printf("\n")

	R=G2mul(R,xa)
	e=Ate(R,G)
	e=Fexp(e)

	fmt.Printf("\ne= "+e.toString())
	fmt.Printf("\n")

	G=G1mul(G,xa)
	e=Ate(P,G)
	e=Fexp(e)
	fmt.Printf("\ne= "+e.toString())
	fmt.Printf("\n") 
}
*/
