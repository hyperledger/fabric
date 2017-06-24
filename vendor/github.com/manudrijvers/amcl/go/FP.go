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

/* Finite Field arithmetic */
/* CLINT mod p functions */

package amcl

//import "fmt"

type FP struct {
	x *BIG
}

/* Constructors */
func NewFPint(a int) *FP {
	F:=new(FP)
	F.x=NewBIGint(a)
	F.nres()
	return F
}

func NewFPbig(a *BIG) *FP {
	F:=new(FP)
	F.x=NewBIGcopy(a)
	F.nres()
	return F
}

func NewFPcopy(a *FP) *FP {
	F:=new(FP)
	F.x=NewBIGcopy(a.x)
	return F
}

func (F *FP) toString() string {
	return F.redc().toString()
}

/* convert to Montgomery n-residue form */
func (F *FP) nres() {
	if MODTYPE!=PSEUDO_MERSENNE && MODTYPE!=GENERALISED_MERSENNE {
		p:=NewBIGints(Modulus);
		d:=NewDBIGscopy(F.x)
		d.shl(uint(NLEN)*BASEBITS)
		F.x.copy(d.mod(p))
	}
}

/* convert back to regular form */
func (F *FP) redc() *BIG {
	if MODTYPE!=PSEUDO_MERSENNE && MODTYPE!=GENERALISED_MERSENNE {
		d:=NewDBIGscopy(F.x)
		return mod(d)
	} else {
		r:=NewBIGcopy(F.x)
		return r
	}
}

/* reduce this mod Modulus */
func (F *FP) reduce() {
	p:=NewBIGints(Modulus)
	F.x.mod(p)
}

/* test this=0? */
func (F *FP) iszilch() bool {
	F.reduce()
	return F.x.iszilch()
}

/* copy from FP b */
func (F *FP) copy(b *FP ) {
	F.x.copy(b.x)
}

/* set this=0 */
func (F *FP) zero() {
	F.x.zero()
}
	
/* set this=1 */
func (F *FP) one() {
	F.x.one(); F.nres()
}

/* normalise this */
func (F *FP) norm() {
	F.x.norm();
}

/* swap FPs depending on d */
func (F *FP) cswap(b *FP,d int) {
	F.x.cswap(b.x,d);
}

/* copy FPs depending on d */
func (F *FP) cmove(b *FP,d int) {
	F.x.cmove(b.x,d)
}

/* this*=b mod Modulus */
func (F *FP) mul(b *FP) {

	F.norm()
	b.norm()
	if pexceed(F.x,b.x) {F.reduce()}
	d:=mul(F.x,b.x)
	F.x.copy(mod(d))
}

func logb2(w uint32) uint {
	v:=w
	v |= (v >> 1)
	v |= (v >> 2)
	v |= (v >> 4)
	v |= (v >> 8)
	v |= (v >> 16)

	v = v - ((v >> 1) & 0x55555555)                 
	v = (v & 0x33333333) + ((v >> 2) & 0x33333333)  
	r:= uint((   ((v + (v >> 4)) & 0xF0F0F0F)   * 0x1010101) >> 24)
	return (r+1)
}

/* this = -this mod Modulus */
func (F *FP) neg() {
	p:=NewBIGints(Modulus)
	m:=NewBIGcopy(p)
	F.norm()
	sb:=logb2(uint32(EXCESS(F.x)))

//	ov:=EXCESS(F.x); 
//	sb:=uint(1); for ov!=0 {sb++;ov>>=1} 

	m.fshl(sb)
	F.x.rsub(m)		

	if EXCESS(F.x)>=FEXCESS {F.reduce()}
}


/* this*=c mod Modulus, where c is a small int */
func (F *FP) imul(c int) {
	F.norm()
	s:=false
	if (c<0) {
		c=-c
		s=true
	}
	afx:=(EXCESS(F.x)+1)*(Chunk(c)+1)+1;
	if (c<NEXCESS && afx<FEXCESS) {
		F.x.imul(c);
	} else {
		if (afx<FEXCESS) {
			F.x.pmul(c)
		} else {
			p:=NewBIGints(Modulus);
			d:=F.x.pxmul(c)
			F.x.copy(d.mod(p))
		}
	}
	if s {F.neg()}
	F.norm()
}

/* this*=this mod Modulus */
func (F *FP) sqr() {
	F.norm();
	if sexceed(F.x) {F.reduce()}
	d:=sqr(F.x)	
	F.x.copy(mod(d))
}

/* this+=b */
func (F *FP) add(b *FP) {
	F.x.add(b.x)
	if (EXCESS(F.x)+2>=FEXCESS) {F.reduce()}
}

/* this-=b */
func (F *FP) sub(b *FP) {
	n:=NewFPcopy(b)
	n.neg()
	F.add(n)
}

/* this/=2 mod Modulus */
func (F *FP) div2() {
	F.x.norm()
	if (F.x.parity()==0) {
		F.x.fshr(1)
	} else {
		p:=NewBIGints(Modulus);
		F.x.add(p)
		F.x.norm()
		F.x.fshr(1)
	}
}

/* this=1/this mod Modulus */
func (F *FP) inverse() {
	p:=NewBIGints(Modulus);
	r:=F.redc()
	r.invmodp(p)
	F.x.copy(r)
	F.nres()
}

/* return TRUE if this==a */
func (F *FP) equals(a *FP) bool {
	a.reduce()
	F.reduce()
	if (comp(a.x,F.x)==0) {return true}
	return false
}

/* return this^e mod Modulus */
func (F *FP) pow(e *BIG) *FP {
	r:=NewFPint(1)
	e.norm()
	F.x.norm()
	m:=NewFPcopy(F)
	for true {
		bt:=e.parity();
		e.fshr(1);
		if bt==1 {r.mul(m)}
		if e.iszilch() {break}
		m.sqr();
	}
	p:=NewBIGints(Modulus);
	r.x.mod(p);
	return r;
}

/* return sqrt(this) mod Modulus */
func (F *FP) sqrt() *FP {
	F.reduce();
	p:=NewBIGints(Modulus);
	b:=NewBIGcopy(p)
	if MOD8==5 {
		b.dec(5); b.norm(); b.shr(3)
		i:=NewFPcopy(F); i.x.shl(1)
		v:=i.pow(b)
		i.mul(v); i.mul(v)
		i.x.dec(1)
		r:=NewFPcopy(F)
		r.mul(v); r.mul(i) 
		r.reduce()
		return r
	} else {
		b.inc(1); b.norm(); b.shr(2)
		return F.pow(b);
	}
}

/* return jacobi symbol (this/Modulus) */
func (F *FP) jacobi() int {
	w:=F.redc();
	p:=NewBIGints(Modulus);
	return w.jacobi(p)
}
