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

package FP256BN
//import "fmt"

const WEIERSTRASS int=0
const EDWARDS int=1
const MONTGOMERY int=2
const NOT int=0
const BN int=1
const BLS int=2
const D_TYPE int=0
const M_TYPE int=1

const CURVETYPE int=WEIERSTRASS
const CURVE_PAIRING_TYPE int=BN
const SEXTIC_TWIST int=M_TYPE

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
	E.y=NewFPint(1)
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
		if y2.Equals(rhs) {
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
func (E *ECP) Is_infinity() bool {
	if E.INF {return true}
	E.x.reduce(); E.z.reduce()
	if CURVETYPE==EDWARDS {
		E.y.reduce();
		E.INF=(E.x.iszilch() && E.y.Equals(E.z))
	} 
	if CURVETYPE==WEIERSTRASS {
		E.y.reduce();
		E.INF=(E.x.iszilch() && E.z.iszilch())
	}
	if CURVETYPE==MONTGOMERY {
		E.INF=E.z.iszilch()
	}
	return E.INF
}

/* Conditional swap of P and Q dependant on d */
func (E *ECP) cswap(Q *ECP,d int) {
	E.x.cswap(Q.x,d)
	if CURVETYPE!=MONTGOMERY {E.y.cswap(Q.y,d)}
	E.z.cswap(Q.z,d)

	bd:=true
	if d==0 {bd=false}
	bd=bd&&(E.INF!=Q.INF)
	E.INF=(bd!=E.INF)
	Q.INF=(bd!=Q.INF)

}

/* Conditional move of Q to P dependant on d */
func (E *ECP) cmove(Q *ECP,d int) {
	E.x.cmove(Q.x,d)
	if CURVETYPE!=MONTGOMERY {E.y.cmove(Q.y,d)}
	E.z.cmove(Q.z,d);

	bd:=true
	if d==0 {bd=false}
	E.INF=(E.INF!=((E.INF!=Q.INF)&&bd))
}

/* return 1 if b==c, no branching */
func teq(b int32,c int32) int {
	x:=b^c
	x-=1  // if x=0, x now -1
	return int((x>>31)&1)
}

/* this=P */
func (E *ECP) Copy(P *ECP) {
	E.x.copy(P.x);
	if CURVETYPE!=MONTGOMERY {E.y.copy(P.y)}
	E.z.copy(P.z);
	E.INF=P.INF;
}

/* this=-this */
func (E *ECP) neg() {
//	if E.Is_infinity() {return}
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
 
	MP.Copy(E);
	MP.neg()
	E.cmove(MP,int(m&1));
}

/* set this=O */
func (E *ECP) inf() {
	E.INF=true;
	E.x.zero()
	if CURVETYPE!=MONTGOMERY {E.y.one()}
	if CURVETYPE!=EDWARDS {
		E.z.zero()
	} else {E.z.one()}
}

/* Test P == Q */
func( E *ECP) Equals(Q *ECP) bool {
	if E.Is_infinity() && Q.Is_infinity() {return true}
	if E.Is_infinity() || Q.Is_infinity() {return false}

	a:=NewFPint(0)
	b:=NewFPint(0)
	a.copy(E.x); a.mul(Q.z); a.reduce()
	b.copy(Q.x); b.mul(E.z); b.reduce()
	if !a.Equals(b) {return false}
	if CURVETYPE!=MONTGOMERY {
		a.copy(E.y); a.mul(Q.z); a.reduce()
		b.copy(Q.y); b.mul(E.z); b.reduce()
		if !a.Equals(b) {return false}
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
		r.sub(one); r.norm()
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
func (E *ECP) Affine() {
	if E.Is_infinity() {return}
	one:=NewFPint(1)
	if E.z.Equals(one) {return}
	E.z.inverse()
	E.x.mul(E.z); E.x.reduce()

	if CURVETYPE!=MONTGOMERY {
		E.y.mul(E.z); E.y.reduce()
	}
	E.z.copy(one)
}

/* extract x as a BIG */
func (E *ECP) GetX() *BIG {
	E.Affine()
	return E.x.redc()
}
/* extract y as a BIG */
func (E *ECP) GetY() *BIG {
	E.Affine()
	return E.y.redc()
}

/* get sign of Y */
func (E *ECP) GetS() int {
	E.Affine()
	y:=E.GetY()
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
func (E *ECP) ToBytes(b []byte) {
	var t [int(MODBYTES)]byte
	MB:=int(MODBYTES)
	if CURVETYPE!=MONTGOMERY {
		b[0]=0x04
	} else {b[0]=0x02}
	
	E.Affine()
	E.x.redc().ToBytes(t[:])
	for i:=0;i<MB;i++ {b[i+1]=t[i]}
	if CURVETYPE!=MONTGOMERY {
		E.y.redc().ToBytes(t[:])
		for i:=0;i<MB;i++ {b[i+MB+1]=t[i]}
	}
}

/* convert from byte array to point */
func ECP_fromBytes(b []byte) *ECP {
	var t [int(MODBYTES)]byte
	MB:=int(MODBYTES)
	p:=NewBIGints(Modulus)

	for i:=0;i<MB;i++ {t[i]=b[i+1]}
	px:=FromBytes(t[:])
	if comp(px,p)>=0 {return NewECP()}

	if (b[0]==0x04) {
		for i:=0;i<MB;i++ {t[i]=b[i+MB+1]}
		py:=FromBytes(t[:])
		if comp(py,p)>=0 {return NewECP()}
		return NewECPbigs(px,py)
	} else {return NewECPbig(px)}
}

/* convert to hex string */
func (E *ECP) toString() string {
	if E.Is_infinity() {return "infinity"}
	E.Affine();
	if CURVETYPE==MONTGOMERY {
		return "("+E.x.redc().toString()+")"
	} else {return "("+E.x.redc().toString()+","+E.y.redc().toString()+")"}
}

/* this*=2 */
func (E *ECP) dbl() {

	if E.INF {return}
	if CURVETYPE==WEIERSTRASS {
		if CURVE_A==0 {
			t0:=NewFPcopy(E.y)                      /*** Change ***/    // Edits made
			t0.sqr()
			t1:=NewFPcopy(E.y)
			t1.mul(E.z)
			t2:=NewFPcopy(E.z)
			t2.sqr()

			E.z.copy(t0)
			E.z.add(t0); E.z.norm(); 
			E.z.add(E.z); E.z.add(E.z); E.z.norm()
			t2.imul(3*CURVE_B_I)

			x3:=NewFPcopy(t2)
			x3.mul(E.z)

			y3:=NewFPcopy(t0)
			y3.add(t2); y3.norm()
			E.z.mul(t1)
			t1.copy(t2); t1.add(t2); t2.add(t1)
			t0.sub(t2); t0.norm(); y3.mul(t0); y3.add(x3)
			t1.copy(E.x); t1.mul(E.y) 
			E.x.copy(t0); E.x.norm(); E.x.mul(t1); E.x.add(E.x)
			E.x.norm(); 
			E.y.copy(y3); E.y.norm();
		} else {
			t0:=NewFPcopy(E.x)
			t1:=NewFPcopy(E.y)
			t2:=NewFPcopy(E.z)
			t3:=NewFPcopy(E.x)
			z3:=NewFPcopy(E.z)
			y3:=NewFPint(0)
			x3:=NewFPint(0)
			b:=NewFPint(0)

			if CURVE_B_I==0 {b.copy(NewFPbig(NewBIGints(CURVE_B)))}

			t0.sqr()  //1    x^2
			t1.sqr()  //2    y^2
			t2.sqr()  //3

			t3.mul(E.y) //4
			t3.add(t3); t3.norm() //5
			z3.mul(E.x);   //6
			z3.add(z3);  z3.norm()//7
			y3.copy(t2) 
				
			if CURVE_B_I==0 {
				y3.mul(b)
			} else {
				y3.imul(CURVE_B_I)
			}
				
			y3.sub(z3) //y3.norm(); //9  ***
			x3.copy(y3); x3.add(y3); x3.norm() //10

			y3.add(x3) //y3.norm();//11
			x3.copy(t1); x3.sub(y3); x3.norm() //12
			y3.add(t1); y3.norm() //13
			y3.mul(x3)  //14
			x3.mul(t3)  //15
			t3.copy(t2); t3.add(t2)  //t3.norm(); //16
			t2.add(t3)  //t2.norm(); //17

			if CURVE_B_I==0 {
				z3.mul(b)
			} else {
				z3.imul(CURVE_B_I)
			}

			z3.sub(t2) //z3.norm();//19
			z3.sub(t0); z3.norm()//20  ***
			t3.copy(z3); t3.add(z3) //t3.norm();//21

			z3.add(t3); z3.norm()  //22
			t3.copy(t0); t3.add(t0)  //t3.norm(); //23
			t0.add(t3)  //t0.norm();//24
			t0.sub(t2); t0.norm() //25

			t0.mul(z3) //26
			y3.add(t0) //y3.norm();//27
			t0.copy(E.y); t0.mul(E.z)//28
			t0.add(t0); t0.norm() //29
			z3.mul(t0)//30
			x3.sub(z3) //x3.norm();//31
			t0.add(t0); t0.norm() //32
			t1.add(t1); t1.norm() //33
			z3.copy(t0); z3.mul(t1) //34

			E.x.copy(x3); E.x.norm() 
			E.y.copy(y3); E.y.norm()
			E.z.copy(z3); E.z.norm()
		}
	}

	if CURVETYPE==EDWARDS {
		C:=NewFPcopy(E.x)
		D:=NewFPcopy(E.y)
		H:=NewFPcopy(E.z)
		J:=NewFPint(0)
	
		E.x.mul(E.y); E.x.add(E.x); E.x.norm()
		C.sqr()
		D.sqr()
		if CURVE_A==-1 {C.neg()}	
		E.y.copy(C); E.y.add(D); E.y.norm()

		H.sqr(); H.add(H)
		E.z.copy(E.y)
		J.copy(E.y); J.sub(H); J.norm()
		E.x.mul(J)
		C.sub(D); C.norm()
		E.y.mul(C)
		E.z.mul(J)


	}
	if CURVETYPE==MONTGOMERY {
		A:=NewFPcopy(E.x)
		B:=NewFPcopy(E.x)	
		AA:=NewFPint(0)
		BB:=NewFPint(0)
		C:=NewFPint(0)
	
		if E.INF {return}

		A.add(E.z); A.norm()
		AA.copy(A); AA.sqr()
		B.sub(E.z); B.norm()
		BB.copy(B); BB.sqr()
		C.copy(AA); C.sub(BB)
		C.norm()

		E.x.copy(AA); E.x.mul(BB)

		A.copy(C); A.imul((CURVE_A+2)/4)

		BB.add(A); BB.norm()
		E.z.copy(BB); E.z.mul(C)
	}
	return;
}

/* this+=Q */
func (E *ECP) Add(Q *ECP) {

	if E.INF {
		E.Copy(Q)
		return
	}
	if Q.INF {return}

	if CURVETYPE==WEIERSTRASS {
		if CURVE_A==0 {
			b:=3*CURVE_B_I
			t0:=NewFPcopy(E.x)
			t0.mul(Q.x)
			t1:=NewFPcopy(E.y)
			t1.mul(Q.y)
			t2:=NewFPcopy(E.z)
			t2.mul(Q.z)
			t3:=NewFPcopy(E.x)
			t3.add(E.y); t3.norm()
			t4:=NewFPcopy(Q.x)
			t4.add(Q.y); t4.norm()
			t3.mul(t4)
			t4.copy(t0); t4.add(t1)

			t3.sub(t4); t3.norm()
			t4.copy(E.y)
			t4.add(E.z); t4.norm()
			x3:=NewFPcopy(Q.y)
			x3.add(Q.z); x3.norm()

			t4.mul(x3)
			x3.copy(t1)
			x3.add(t2)
	
			t4.sub(x3); t4.norm()
			x3.copy(E.x); x3.add(E.z); x3.norm()
			y3:=NewFPcopy(Q.x)
			y3.add(Q.z); y3.norm()
			x3.mul(y3)
			y3.copy(t0)
			y3.add(t2)
			y3.rsub(x3); y3.norm()
			x3.copy(t0); x3.add(t0) 
			t0.add(x3); t0.norm()
			t2.imul(b)

			z3:=NewFPcopy(t1); z3.add(t2); z3.norm()
			t1.sub(t2); t1.norm() 
			y3.imul(b)
	
			x3.copy(y3); x3.mul(t4); t2.copy(t3); t2.mul(t1); x3.rsub(t2)
			y3.mul(t0); t1.mul(z3); y3.add(t1)
			t0.mul(t3); z3.mul(t4); z3.add(t0)

			E.x.copy(x3); E.x.norm()
			E.y.copy(y3); E.y.norm()
			E.z.copy(z3); E.z.norm()	
		} else {

			t0:=NewFPcopy(E.x)
			t1:=NewFPcopy(E.y)
			t2:=NewFPcopy(E.z)
			t3:=NewFPcopy(E.x)
			t4:=NewFPcopy(Q.x)
			z3:=NewFPint(0)
			y3:=NewFPcopy(Q.x)
			x3:=NewFPcopy(Q.y)
			b:=NewFPint(0)

			if CURVE_B_I==0 {b.copy(NewFPbig(NewBIGints(CURVE_B)))}

			t0.mul(Q.x) //1
			t1.mul(Q.y) //2
			t2.mul(Q.z) //3

			t3.add(E.y); t3.norm() //4
			t4.add(Q.y); t4.norm() //5
			t3.mul(t4) //6
			t4.copy(t0); t4.add(t1) //t4.norm(); //7
			t3.sub(t4); t3.norm() //8
			t4.copy(E.y); t4.add(E.z); t4.norm() //9
			x3.add(Q.z); x3.norm() //10
			t4.mul(x3) //11
			x3.copy(t1); x3.add(t2) //x3.norm();//12

			t4.sub(x3); t4.norm() //13
			x3.copy(E.x); x3.add(E.z); x3.norm() //14
			y3.add(Q.z); y3.norm() //15

			x3.mul(y3) //16
			y3.copy(t0); y3.add(t2) //y3.norm();//17

			y3.rsub(x3); y3.norm() //18
			z3.copy(t2) 
				
			if CURVE_B_I==0 {
				z3.mul(b)
			} else {
				z3.imul(CURVE_B_I)
			}
				
			x3.copy(y3); x3.sub(z3); x3.norm() //20
			z3.copy(x3); z3.add(x3) //z3.norm(); //21

			x3.add(z3) //x3.norm(); //22
			z3.copy(t1); z3.sub(x3); z3.norm() //23
			x3.add(t1); x3.norm() //24

			if CURVE_B_I==0 {
				y3.mul(b)
			} else {
				y3.imul(CURVE_B_I)
			}

			t1.copy(t2); t1.add(t2); //t1.norm();//26
			t2.add(t1) //t2.norm();//27

			y3.sub(t2) //y3.norm(); //28

			y3.sub(t0); y3.norm() //29
			t1.copy(y3); t1.add(y3) //t1.norm();//30
			y3.add(t1); y3.norm() //31

			t1.copy(t0); t1.add(t0) //t1.norm(); //32
			t0.add(t1) //t0.norm();//33
			t0.sub(t2); t0.norm() //34
			t1.copy(t4); t1.mul(y3) //35
			t2.copy(t0); t2.mul(y3) //36
			y3.copy(x3); y3.mul(z3) //37
			y3.add(t2) //y3.norm();//38
			x3.mul(t3) //39
			x3.sub(t1) //40
			z3.mul(t4) //41
			t1.copy(t3); t1.mul(t0) //42
			z3.add(t1) 
			E.x.copy(x3); E.x.norm() 
			E.y.copy(y3); E.y.norm()
			E.z.copy(z3); E.z.norm()

		}
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
		B.norm(); D.norm()
		B.mul(D)
		B.sub(C)
		B.norm(); F.norm()
		B.mul(F)
		E.x.copy(A); E.x.mul(B)
		G.norm()
		if CURVE_A==1 {
			EE.norm(); C.copy(EE); C.mul(G)
		}
		if CURVE_A==-1 {
			C.norm(); C.mul(G)
		}
		E.y.copy(A); E.y.mul(C)
		E.z.copy(F); E.z.mul(G)
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
	A.norm(); D.norm()

	DA.copy(D); DA.mul(A)
	C.norm(); B.norm()

	CB.copy(C); CB.mul(B)

	A.copy(DA); A.add(CB); A.norm(); A.sqr()
	B.copy(DA); B.sub(CB); B.norm(); B.sqr()

	E.x.copy(A)
	E.z.copy(W.x); E.z.mul(B)

//	if E.z.iszilch() {
//		E.inf()
//	} else {E.INF=false;}

}

/* this-=Q */
func (E *ECP) Sub(Q *ECP) {
	Q.neg()
	E.Add(Q)
	Q.neg()
}

/* constant time multiply by small integer of length bts - use ladder */
func (E *ECP) pinmul(e int32,bts int32) *ECP {	
	if CURVETYPE==MONTGOMERY {
		return E.mul(NewBIGint(int(e)))
	} else {
		P:=NewECP()
		R0:=NewECP()
		R1:=NewECP(); R1.Copy(E)

		for i:=bts-1;i>=0;i-- {
			b:=int((e>>uint32(i))&1)
			P.Copy(R1)
			P.Add(R0)
			R0.cswap(R1,b)
			R1.Copy(P)
			R0.dbl()
			R0.cswap(R1,b)
		}
		P.Copy(R0)
		P.Affine()
		return P
	}
}

/* return e.this */

func (E *ECP) mul(e *BIG) *ECP {
	if (e.iszilch() || E.Is_infinity()) {return NewECP()}
	P:=NewECP()
	if CURVETYPE==MONTGOMERY {
/* use Ladder */
		D:=NewECP();
		R0:=NewECP(); R0.Copy(E)
		R1:=NewECP(); R1.Copy(E)
		R1.dbl()
		D.Copy(E); D.Affine()
		nb:=e.nbits()
		for i:=nb-2;i>=0;i-- {
			b:=int(e.bit(i))
			P.Copy(R1)
			P.dadd(R0,D)
			R0.cswap(R1,b)
			R1.Copy(P)
			R0.dbl()
			R0.cswap(R1,b)
		}
		P.Copy(R0)
	} else {
// fixed size windows 
		mt:=NewBIG()
		t:=NewBIG()
		Q:=NewECP()
		C:=NewECP()

		var W []*ECP
		var w [1+(NLEN*int(BASEBITS)+3)/4]int8

		E.Affine();

		Q.Copy(E);
		Q.dbl();

		W=append(W,NewECP());
		W[0].Copy(E);

		for i:=1;i<8;i++ {
			W=append(W,NewECP())
			W[i].Copy(W[i-1])
			W[i].Add(Q)
		}

// make exponent odd - add 2P if even, P if odd 
		t.copy(e)
		s:=int(t.parity())
		t.inc(1); t.norm(); ns:=int(t.parity()); mt.copy(t); mt.inc(1); mt.norm()
		t.cmove(mt,s)
		Q.cmove(E,ns)
		C.Copy(Q)

		nb:=1+(t.nbits()+3)/4

// convert exponent to signed 4-bit window 
		for i:=0;i<nb;i++ {
			w[i]=int8(t.lastbits(5)-16)
			t.dec(int(w[i])); t.norm()
			t.fshr(4)	
		}
		w[nb]=int8(t.lastbits(5))

		P.Copy(W[(int(w[nb])-1)/2])  
		for i:=nb-1;i>=0;i-- {
			Q.selector(W,int32(w[i]))
			P.dbl()
			P.dbl()
			P.dbl()
			P.dbl()
			P.Add(Q)
		}
		P.Sub(C) /* apply correction */
	}
	P.Affine()
	return P
}

/* Public version */
func (E *ECP) Mul(e *BIG) *ECP {
	return E.mul(e)
}

/* Return e.this+f.Q */

func (E *ECP) Mul2(e *BIG,Q *ECP,f *BIG) *ECP {
	te:=NewBIG()
	tf:=NewBIG()
	mt:=NewBIG()
	S:=NewECP()
	T:=NewECP()
	C:=NewECP()
	var W [] *ECP
	//ECP[] W=new ECP[8];
	var w [1+(NLEN*int(BASEBITS)+1)/2]int8		

	E.Affine()
	Q.Affine()

	te.copy(e)
	tf.copy(f)

// precompute table 
	for i:=0;i<8;i++ {
		W=append(W,NewECP())
	}
	W[1].Copy(E); W[1].Sub(Q)
	W[2].Copy(E); W[2].Add(Q);
	S.Copy(Q); S.dbl();
	W[0].Copy(W[1]); W[0].Sub(S);
	W[3].Copy(W[2]); W[3].Add(S);
	T.Copy(E); T.dbl();
	W[5].Copy(W[1]); W[5].Add(T);
	W[6].Copy(W[2]); W[6].Add(T);
	W[4].Copy(W[5]); W[4].Sub(S);
	W[7].Copy(W[6]); W[7].Add(S);

// if multiplier is odd, add 2, else add 1 to multiplier, and add 2P or P to correction 

	s:=int(te.parity());
	te.inc(1); te.norm(); ns:=int(te.parity()); mt.copy(te); mt.inc(1); mt.norm()
	te.cmove(mt,s)
	T.cmove(E,ns)
	C.Copy(T)

	s=int(tf.parity())
	tf.inc(1); tf.norm(); ns=int(tf.parity()); mt.copy(tf); mt.inc(1); mt.norm()
	tf.cmove(mt,s)
	S.cmove(Q,ns)
	C.Add(S)

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
	S.Copy(W[(w[nb]-1)/2])  

	for i:=nb-1;i>=0;i-- {
		T.selector(W,int32(w[i]));
		S.dbl()
		S.dbl()
		S.Add(T)
	}
	S.Sub(C) /* apply correction */
	S.Affine()
	return S
}

func ECP_mapit(h []byte) *ECP {
	q:=NewBIGints(Modulus)
	x:=FromBytes(h[:])
	x.Mod(q)
	var P *ECP
	for true {
		P=NewECPbigint(x,0)
		if !P.Is_infinity() {break}
		x.inc(1); x.norm()
	}
	if CURVE_PAIRING_TYPE!=BN {
		c:=NewBIGints(CURVE_Cof)
		P=P.mul(c)
	}	
	return P
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