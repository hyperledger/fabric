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

/* Finite Field arithmetic  Fp^2 functions */

/* FP2 elements are of the form a+ib, where i is sqrt(-1) */

package FP256BN

//import "fmt"

type FP2 struct {
	a *FP
	b *FP
}

/* Constructors */
func NewFP2int(a int) *FP2 {
	F:=new(FP2)
	F.a=NewFPint(a)
	F.b=NewFPint(0)
	return F
}

func NewFP2copy(x *FP2) *FP2 {
	F:=new(FP2)
	F.a=NewFPcopy(x.a)
	F.b=NewFPcopy(x.b)
	return F
}

func NewFP2fps(c *FP,d *FP) *FP2 {
	F:=new(FP2)
	F.a=NewFPcopy(c)
	F.b=NewFPcopy(d)
	return F
}

func NewFP2bigs(c *BIG,d *BIG) *FP2 {
	F:=new(FP2)
	F.a=NewFPbig(c)
	F.b=NewFPbig(d)
	return F
}

func NewFP2fp(c *FP) *FP2 {
	F:=new(FP2)
	F.a=NewFPcopy(c)
	F.b=NewFPint(0)
	return F
}

func NewFP2big(c *BIG) *FP2 {
	F:=new(FP2)
	F.a=NewFPbig(c)
	F.b=NewFPint(0)
	return F
}

/* reduce components mod Modulus */
func (F *FP2) reduce() {
	F.a.reduce()
	F.b.reduce()
}

/* normalise components of w */
func (F *FP2) norm() {
	F.a.norm()
	F.b.norm()
}

/* test this=0 ? */
func (F *FP2) iszilch() bool {
	F.reduce()
	return (F.a.iszilch() && F.b.iszilch())
}

func (F *FP2) cmove(g *FP2,d int) {
	F.a.cmove(g.a,d)
	F.b.cmove(g.b,d)
}

/* test this=1 ? */
func (F *FP2)  isunity() bool {
	one:=NewFPint(1)
	return (F.a.Equals(one) && F.b.iszilch())
}

/* test this=x */
func (F *FP2) Equals(x *FP2) bool {
	return (F.a.Equals(x.a) && F.b.Equals(x.b))
}

/* extract a */
func (F *FP2) GetA() *BIG { 
	return F.a.redc()
}

/* extract b */
func (F *FP2) GetB() *BIG {
	return F.b.redc()
}

/* copy this=x */
func (F *FP2) copy(x *FP2) {
	F.a.copy(x.a)
	F.b.copy(x.b)
}

/* set this=0 */
func (F *FP2) zero() {
	F.a.zero()
	F.b.zero()
}

/* set this=1 */
func (F *FP2) one() {
	F.a.one()
	F.b.zero()
}

/* negate this mod Modulus */
func (F *FP2) neg() {
//	F.norm()
	m:=NewFPcopy(F.a)
	t:= NewFPint(0)

	m.add(F.b)
	m.neg()
//	m.norm()
	t.copy(m); t.add(F.b)
	F.b.copy(m)
	F.b.add(F.a)
	F.a.copy(t)
}

/* set to a-ib */
func (F *FP2) conj() {
	F.b.neg(); F.b.norm()
}

/* this+=a */
func (F *FP2) add(x *FP2) {
	F.a.add(x.a)
	F.b.add(x.b)
}

/* this-=a */
func (F *FP2) sub(x *FP2) {
	m:=NewFP2copy(x)
	m.neg()
	F.add(m)
}

/* this-=a */
func (F *FP2) rsub(x *FP2) {
	F.neg()
	F.add(x)
}

/* this*=s, where s is an FP */
func (F *FP2) pmul(s *FP) {
	F.a.mul(s)
	F.b.mul(s)
}

/* this*=i, where i is an int */
func (F *FP2) imul(c int) {
	F.a.imul(c)
	F.b.imul(c)
}

/* this*=this */
func (F *FP2) sqr() {
	w1:=NewFPcopy(F.a)
	w3:=NewFPcopy(F.a)
	mb:=NewFPcopy(F.b)

//	w3.mul(F.b)
	w1.add(F.b)

w3.add(F.a);
w3.norm();
F.b.mul(w3);

	mb.neg()
	F.a.add(mb)

	w1.norm()
	F.a.norm()

	F.a.mul(w1)
//	F.b.copy(w3); F.b.add(w3)

//	F.b.norm()
}

/* this*=y */
/* Now using Lazy reduction */
func (F *FP2) mul(y *FP2) {

	if int64(F.a.XES+F.b.XES)*int64(y.a.XES+y.b.XES)>int64(FEXCESS) {
		if F.a.XES>1 {F.a.reduce()}
		if F.b.XES>1 {F.b.reduce()}		
	}

	pR:=NewDBIG()
	C:=NewBIGcopy(F.a.x)
	D:=NewBIGcopy(y.a.x)
	p:=NewBIGints(Modulus)

	pR.ucopy(p)

	A:=mul(F.a.x,y.a.x)
	B:=mul(F.b.x,y.b.x)

	C.add(F.b.x); C.norm()
	D.add(y.b.x); D.norm()

	E:=mul(C,D)
	FF:=NewDBIGcopy(A); FF.add(B)
	B.rsub(pR)

	A.add(B); A.norm()
	E.sub(FF); E.norm()

	F.a.x.copy(mod(A)); F.a.XES=3
	F.b.x.copy(mod(E)); F.b.XES=2

}

/* sqrt(a+ib) = sqrt(a+sqrt(a*a-n*b*b)/2)+ib/(2*sqrt(a+sqrt(a*a-n*b*b)/2)) */
/* returns true if this is QR */
func (F *FP2) sqrt() bool {
	if F.iszilch() {return true}
	w1:=NewFPcopy(F.b)
	w2:=NewFPcopy(F.a)
	w1.sqr(); w2.sqr(); w1.add(w2)
	if w1.jacobi()!=1 { F.zero(); return false }
	w1=w1.sqrt()
	w2.copy(F.a); w2.add(w1); w2.norm(); w2.div2()
	if w2.jacobi()!=1 {
		w2.copy(F.a); w2.sub(w1); w2.norm(); w2.div2()
		if w2.jacobi()!=1 { F.zero(); return false }
	}
	w2=w2.sqrt()
	F.a.copy(w2)
	w2.add(w2)
	w2.inverse()
	F.b.mul(w2)
	return true
}

/* output to hex string */
func (F *FP2) toString() string {
	return ("["+F.a.toString()+","+F.b.toString()+"]")
}

/* this=1/this */
func (F *FP2) inverse() {
	F.norm()
	w1:=NewFPcopy(F.a)
	w2:=NewFPcopy(F.b)

	w1.sqr()
	w2.sqr()
	w1.add(w2)
	w1.inverse()
	F.a.mul(w1)
	w1.neg(); w1.norm();
	F.b.mul(w1)
}

/* this/=2 */
func (F *FP2) div2() {
	F.a.div2()
	F.b.div2()
}

/* this*=sqrt(-1) */
func (F *FP2) times_i() {
	//	a.norm();
	z:=NewFPcopy(F.a)
	F.a.copy(F.b); F.a.neg()
	F.b.copy(z)
}

/* w*=(1+sqrt(-1)) */
/* where X*2-(1+sqrt(-1)) is irreducible for FP4, assumes p=3 mod 8 */
func (F *FP2) mul_ip() {
//	F.norm()
	t:=NewFP2copy(F)
	z:=NewFPcopy(F.a)
	F.a.copy(F.b)
	F.a.neg()
	F.b.copy(z)
	F.add(t)
//	F.norm()
}

func (F *FP2) div_ip2() {
	t:=NewFP2int(0)
	t.a.copy(F.a); t.a.add(F.b)
	t.b.copy(F.b); t.b.sub(F.a);
	F.copy(t);
}

/* w/=(1+sqrt(-1)) */
func (F *FP2) div_ip() {
	t:=NewFP2int(0)
	F.norm()
	t.a.copy(F.a); t.a.add(F.b)
	t.b.copy(F.b); t.b.sub(F.a);
	F.copy(t); F.norm()
	F.div2()
}
