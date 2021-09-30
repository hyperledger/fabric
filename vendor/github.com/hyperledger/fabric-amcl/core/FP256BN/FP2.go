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

/* Finite Field arithmetic  Fp^2 functions */

/* FP2 elements are of the form a+ib, where i is sqrt(-1) */

package FP256BN
import "github.com/hyperledger/fabric-amcl/core"


type FP2 struct {
	a *FP
	b *FP
}

func NewFP2() *FP2 {
	F := new(FP2)
	F.a = NewFP()
	F.b = NewFP()
	return F
}

/* Constructors */
func NewFP2int(a int) *FP2 {
	F := new(FP2)
	F.a = NewFPint(a)
	F.b = NewFP()
	return F
}

func NewFP2ints(a int, b int) *FP2 {
	F := new(FP2)
	F.a = NewFPint(a)
	F.b = NewFPint(b)
	return F
}

func NewFP2copy(x *FP2) *FP2 {
	F := new(FP2)
	F.a = NewFPcopy(x.a)
	F.b = NewFPcopy(x.b)
	return F
}

func NewFP2fps(c *FP, d *FP) *FP2 {
	F := new(FP2)
	F.a = NewFPcopy(c)
	F.b = NewFPcopy(d)
	return F
}

func NewFP2bigs(c *BIG, d *BIG) *FP2 {
	F := new(FP2)
	F.a = NewFPbig(c)
	F.b = NewFPbig(d)
	return F
}

func NewFP2fp(c *FP) *FP2 {
	F := new(FP2)
	F.a = NewFPcopy(c)
	F.b = NewFP()
	return F
}

func NewFP2big(c *BIG) *FP2 {
	F := new(FP2)
	F.a = NewFPbig(c)
	F.b = NewFP()
	return F
}

func NewFP2rand(rng *core.RAND) *FP2 {
	F := NewFP2fps(NewFPrand(rng),NewFPrand(rng))
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
	return (F.a.iszilch() && F.b.iszilch())
}

func (F *FP2) islarger() int {
    if F.iszilch() {
		return 0;
	}
	cmp:=F.b.islarger()
	if cmp!=0 {
		return cmp
	}
	return F.a.islarger()
}

func (F *FP2) ToBytes(bf []byte) {
	var t [int(MODBYTES)]byte
	MB := int(MODBYTES)
	F.b.ToBytes(t[:]);
	for i:=0;i<MB;i++ {
		bf[i]=t[i];
	}
	F.a.ToBytes(t[:]);
	for i:=0;i<MB;i++ {
		bf[i+MB]=t[i];
	}
}

func FP2_fromBytes(bf []byte) *FP2 {
	var t [int(MODBYTES)]byte
	MB := int(MODBYTES)
	for i:=0;i<MB;i++ {
        t[i]=bf[i];
	}
    tb:=FP_fromBytes(t[:])
	for i:=0;i<MB;i++ {
        t[i]=bf[i+MB]
	}
    ta:=FP_fromBytes(t[:])
	return NewFP2fps(ta,tb)
}

func (F *FP2) cmove(g *FP2, d int) {
	F.a.cmove(g.a, d)
	F.b.cmove(g.b, d)
}

/* test this=1 ? */
func (F *FP2) isunity() bool {
	one := NewFPint(1)
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
/* Return sign */
func (F *FP2) sign() int {
	p1 := F.a.sign();
	p2 := F.b.sign();
	var u int
	if BIG_ENDIAN_SIGN {
		if F.b.iszilch() {
			u=1;
		} else {
			u=0;
		}
		p2^=(p1^p2)&u;
		return p2;
	} else {
		if F.a.iszilch() {
			u=1;
		} else {
			u=0;
		}
		p1^=(p1^p2)&u;
		return p1;
	}
}

/* negate this mod Modulus */
func (F *FP2) neg() {
	m := NewFPcopy(F.a)
	t := NewFP()

	m.add(F.b)
	m.neg()
	t.copy(m)
	t.add(F.b)
	F.b.copy(m)
	F.b.add(F.a)
	F.a.copy(t)
}

/* set to a-ib */
func (F *FP2) conj() {
	F.b.neg()
	F.b.norm()
}

/* this+=a */
func (F *FP2) add(x *FP2) {
	F.a.add(x.a)
	F.b.add(x.b)
}

/* this-=a */
func (F *FP2) sub(x *FP2) {
	m := NewFP2copy(x)
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
	w1 := NewFPcopy(F.a)
	w3 := NewFPcopy(F.a)
	mb := NewFPcopy(F.b)
	w1.add(F.b)

	w3.add(F.a)
	w3.norm()
	F.b.mul(w3)

	mb.neg()
	F.a.add(mb)

	w1.norm()
	F.a.norm()

	F.a.mul(w1)
}

/* this*=y */
/* Now using Lazy reduction */
func (F *FP2) mul(y *FP2) {

	if int64(F.a.XES+F.b.XES)*int64(y.a.XES+y.b.XES) > int64(FEXCESS) {
		if F.a.XES > 1 {
			F.a.reduce()
		}
		if F.b.XES > 1 {
			F.b.reduce()
		}
	}

	pR := NewDBIG()
	C := NewBIGcopy(F.a.x)
	D := NewBIGcopy(y.a.x)
	p := NewBIGints(Modulus)

	pR.ucopy(p)

	A := mul(F.a.x, y.a.x)
	B := mul(F.b.x, y.b.x)

	C.add(F.b.x)
	C.norm()
	D.add(y.b.x)
	D.norm()

	E := mul(C, D)
	FF := NewDBIGcopy(A)
	FF.add(B)
	B.rsub(pR)

	A.add(B)
	A.norm()
	E.sub(FF)
	E.norm()

	F.a.x.copy(mod(A))
	F.a.XES = 3
	F.b.x.copy(mod(E))
	F.b.XES = 2

}
/*
func (F *FP2) pow(b *BIG)  {
	w := NewFP2copy(F);
	r := NewFP2int(1)
	z := NewBIGcopy(b)
	for true {
		bt := z.parity()
		z.shr(1)
		if bt==1 {
			r.mul(w)
		}
		if z.iszilch() {break}
		w.sqr()
	}
	r.reduce()
	F.copy(r)
}
*/
func (F *FP2) qr(h *FP) int {
	c := NewFP2copy(F)
	c.conj()
	c.mul(F)
	return c.a.qr(h)
}

/* sqrt(a+ib) = sqrt(a+sqrt(a*a-n*b*b)/2)+ib/(2*sqrt(a+sqrt(a*a-n*b*b)/2)) */
func (F *FP2) sqrt(h *FP) {
	if F.iszilch() {
		return 
	}
	w1 := NewFPcopy(F.b)
	w2 := NewFPcopy(F.a)
	w3 := NewFP()
	w4 := NewFP()
	hint:=NewFP()
	w1.sqr()
	w2.sqr()
	w1.add(w2); w1.norm()

	w1 = w1.sqrt(h)
	w2.copy(F.a)
	w3.copy(F.a)

	w2.add(w1)
	w2.norm()
	w2.div2()

	w1.copy(F.b); w1.div2()
	qr:=w2.qr(hint)

// tweak hint
    w3.copy(hint); w3.neg(); w3.norm()
    w4.copy(w2); w4.neg(); w4.norm()

    w2.cmove(w4,1-qr)
    hint.cmove(w3,1-qr)

    F.a.copy(w2.sqrt(hint))
    w3.copy(w2); w3.inverse(hint)
    w3.mul(F.a)
    F.b.copy(w3); F.b.mul(w1)
    w4.copy(F.a)

    F.a.cmove(F.b,1-qr)
    F.b.cmove(w4,1-qr)

/*
	F.a.copy(w2.sqrt(hint))
	w3.copy(w2); w3.inverse(hint)
	w3.mul(F.a)
	F.b.copy(w3); F.b.mul(w1)

	hint.neg(); hint.norm()
	w2.neg(); w2.norm()

	w4.copy(w2.sqrt(hint))
	w3.copy(w2); w3.inverse(hint)
	w3.mul(w4)
	w3.mul(w1)

	F.a.cmove(w3,1-qr)
	F.b.cmove(w4,1-qr)
*/

	sgn:=F.sign()
	nr:=NewFP2copy(F)
	nr.neg(); nr.norm()
	F.cmove(nr,sgn)
}

/* output to hex string */
func (F *FP2) ToString() string {
	return ("[" + F.a.ToString() + "," + F.b.ToString() + "]")
}

/* output to hex string */
func (F *FP2) toString() string {
	return ("[" + F.a.ToString() + "," + F.b.ToString() + "]")
}

/* this=1/this */
func (F *FP2) inverse(h *FP) {
	F.norm()
	w1 := NewFPcopy(F.a)
	w2 := NewFPcopy(F.b)

	w1.sqr()
	w2.sqr()
	w1.add(w2)
	w1.inverse(h)
	F.a.mul(w1)
	w1.neg()
	w1.norm()
	F.b.mul(w1)
}

/* this/=2 */
func (F *FP2) div2() {
	F.a.div2()
	F.b.div2()
}

/* this*=sqrt(-1) */
func (F *FP2) times_i() {
	z := NewFPcopy(F.a)
	F.a.copy(F.b)
	F.a.neg()
	F.b.copy(z)
}

/* w*=(1+sqrt(-1)) */
/* where X*2-(2^i+sqrt(-1)) is irreducible for FP4 */
func (F *FP2) mul_ip() {
	t := NewFP2copy(F)
	i := QNRI
	F.times_i()
	for i > 0 {
		t.add(t)
		t.norm()
		i--
	}
	F.add(t)

	if TOWER == POSITOWER {
		F.norm()
		F.neg()
	}

}

/* w/=(2^i+sqrt(-1)) */
func (F *FP2) div_ip() {
	z := NewFP2ints(1<<uint(QNRI), 1)
	z.inverse(nil)
	F.norm()
	F.mul(z)
	if TOWER == POSITOWER {
		F.neg()
		F.norm()
	}
}
