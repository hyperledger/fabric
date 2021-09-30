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

/* Finite Field arithmetic  Fp^4 functions */

/* FP4 elements are of the form a+ib, where i is sqrt(-1+sqrt(-1)) */

package FP256BN
import "github.com/hyperledger/fabric-amcl/core"


type FP4 struct {
	a *FP2
	b *FP2
}

func NewFP4() *FP4 {
	F := new(FP4)
	F.a = NewFP2()
	F.b = NewFP2()
	return F
}

/* Constructors */
func NewFP4int(a int) *FP4 {
	F := new(FP4)
	F.a = NewFP2int(a)
	F.b = NewFP2()
	return F
}

/* Constructors */
func NewFP4ints(a int,b int) *FP4 {
	F := new(FP4)
	F.a = NewFP2int(a)
	F.b = NewFP2int(b)
	return F
}

func NewFP4copy(x *FP4) *FP4 {
	F := new(FP4)
	F.a = NewFP2copy(x.a)
	F.b = NewFP2copy(x.b)
	return F
}

func NewFP4fp2s(c *FP2, d *FP2) *FP4 {
	F := new(FP4)
	F.a = NewFP2copy(c)
	F.b = NewFP2copy(d)
	return F
}

func NewFP4fp2(c *FP2) *FP4 {
	F := new(FP4)
	F.a = NewFP2copy(c)
	F.b = NewFP2()
	return F
}

func NewFP4fp(c *FP) *FP4 {
	F := new(FP4)
	F.a = NewFP2fp(c)
	F.b = NewFP2()
	return F
}

func NewFP4rand(rng *core.RAND) *FP4 {
	F := NewFP4fp2s(NewFP2rand(rng),NewFP2rand(rng))
	return F
}

/* reduce all components of this mod Modulus */
func (F *FP4) reduce() {
	F.a.reduce()
	F.b.reduce()
}

/* normalise all components of this mod Modulus */
func (F *FP4) norm() {
	F.a.norm()
	F.b.norm()
}

/* test this==0 ? */
func (F *FP4) iszilch() bool {
	return F.a.iszilch() && F.b.iszilch()
}

func (F *FP4) islarger() int {
    if F.iszilch() {
		return 0;
	}
	cmp:=F.b.islarger()
	if cmp!=0 {
		return cmp
	}
	return F.a.islarger()
}

func (F *FP4) ToBytes(bf []byte) {
	var t [2*int(MODBYTES)]byte
	MB := 2*int(MODBYTES)
	F.b.ToBytes(t[:]);
	for i:=0;i<MB;i++ {
		bf[i]=t[i];
	}
	F.a.ToBytes(t[:]);
	for i:=0;i<MB;i++ {
		bf[i+MB]=t[i];
	}
}

func FP4_fromBytes(bf []byte) *FP4 {
	var t [2*int(MODBYTES)]byte
	MB := 2*int(MODBYTES)
	for i:=0;i<MB;i++ {
        t[i]=bf[i];
	}
    tb:=FP2_fromBytes(t[:])
	for i:=0;i<MB;i++ {
        t[i]=bf[i+MB]
	}
    ta:=FP2_fromBytes(t[:])
	return NewFP4fp2s(ta,tb)
}


/* Conditional move */
func (F *FP4) cmove(g *FP4, d int) {
	F.a.cmove(g.a, d)
	F.b.cmove(g.b, d)
}

/* test this==1 ? */
func (F *FP4) isunity() bool {
	one := NewFP2int(1)
	return F.a.Equals(one) && F.b.iszilch()
}

/* test is w real? That is in a+ib test b is zero */
func (F *FP4) isreal() bool {
	return F.b.iszilch()
}

/* extract real part a */
func (F *FP4) real() *FP2 {
	return F.a
}

func (F *FP4) geta() *FP2 {
	return F.a
}

/* extract imaginary part b */
func (F *FP4) getb() *FP2 {
	return F.b
}

/* test this=x? */
func (F *FP4) Equals(x *FP4) bool {
	return (F.a.Equals(x.a) && F.b.Equals(x.b))
}

/* copy this=x */
func (F *FP4) copy(x *FP4) {
	F.a.copy(x.a)
	F.b.copy(x.b)
}

/* set this=0 */
func (F *FP4) zero() {
	F.a.zero()
	F.b.zero()
}

/* set this=1 */
func (F *FP4) one() {
	F.a.one()
	F.b.zero()
}

/* Return sign */
func (F *FP4) sign() int {
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

/* set this=-this */
func (F *FP4) neg() {
	F.norm()
	m := NewFP2copy(F.a)
	t := NewFP2()
	m.add(F.b)
	m.neg()
	t.copy(m)
	t.add(F.b)
	F.b.copy(m)
	F.b.add(F.a)
	F.a.copy(t)
	F.norm()
}

/* this=conjugate(this) */
func (F *FP4) conj() {
	F.b.neg()
	F.norm()
}

/* this=-conjugate(this) */
func (F *FP4) nconj() {
	F.a.neg()
	F.norm()
}

/* this+=x */
func (F *FP4) add(x *FP4) {
	F.a.add(x.a)
	F.b.add(x.b)
}

/* this-=x */
func (F *FP4) sub(x *FP4) {
	m := NewFP4copy(x)
	m.neg()
	F.add(m)
}

/* this-=x */
func (F *FP4) rsub(x *FP4) {
	F.neg()
	F.add(x)
}

/* this*=s where s is FP2 */
func (F *FP4) pmul(s *FP2) {
	F.a.mul(s)
	F.b.mul(s)
}

/* this*=s where s is FP2 */
func (F *FP4) qmul(s *FP) {
	F.a.pmul(s)
	F.b.pmul(s)
}

/* this*=c where c is int */
func (F *FP4) imul(c int) {
	F.a.imul(c)
	F.b.imul(c)
}

/* this*=this */
func (F *FP4) sqr() {
	t1 := NewFP2copy(F.a)
	t2 := NewFP2copy(F.b)
	t3 := NewFP2copy(F.a)

	t3.mul(F.b)
	t1.add(F.b)
	t2.mul_ip()

	t2.add(F.a)

	t1.norm()
	t2.norm()

	F.a.copy(t1)

	F.a.mul(t2)

	t2.copy(t3)
	t2.mul_ip()
	t2.add(t3)
	t2.norm()
	t2.neg()
	F.a.add(t2)

	F.b.copy(t3)
	F.b.add(t3)

	F.norm()
}

/* this*=y */
func (F *FP4) mul(y *FP4) {
	t1 := NewFP2copy(F.a)
	t2 := NewFP2copy(F.b)
	t3 := NewFP2()
	t4 := NewFP2copy(F.b)

	t1.mul(y.a)
	t2.mul(y.b)
	t3.copy(y.b)
	t3.add(y.a)
	t4.add(F.a)

	t3.norm()
	t4.norm()

	t4.mul(t3)

	t3.copy(t1)
	t3.neg()
	t4.add(t3)
	t4.norm()

	t3.copy(t2)
	t3.neg()
	F.b.copy(t4)
	F.b.add(t3)

	t2.mul_ip()
	F.a.copy(t2)
	F.a.add(t1)

	F.norm()
}

/* convert this to hex string */
func (F *FP4) toString() string {
	return ("[" + F.a.toString() + "," + F.b.toString() + "]")
}

/* this=1/this */
func (F *FP4) inverse(h *FP) {
	t1 := NewFP2copy(F.a)
	t2 := NewFP2copy(F.b)

	t1.sqr()
	t2.sqr()
	t2.mul_ip()
	t2.norm()
	t1.sub(t2)

	t1.inverse(h)
	F.a.mul(t1)
	t1.neg()
	t1.norm()
	F.b.mul(t1)
}

/* this*=i where i = sqrt(2^i+sqrt(-1)) */
func (F *FP4) times_i() {
	t := NewFP2copy(F.b)
	F.b.copy(F.a)
	t.mul_ip()
	F.a.copy(t)
	F.norm()
	if TOWER == POSITOWER {
		F.neg()
		F.norm()
	}
}

/* this=this^p using Frobenius */
func (F *FP4) frob(f *FP2) {
	F.a.conj()
	F.b.conj()
	F.b.mul(f)
}

/* this=this^e 
func (F *FP4) pow(e *BIG) *FP4 {
	w := NewFP4copy(F)
	w.norm()
	z := NewBIGcopy(e)
	r := NewFP4int(1)
	z.norm()
	for true {
		bt := z.parity()
		z.fshr(1)
		if bt == 1 {
			r.mul(w)
		}
		if z.iszilch() {
			break
		}
		w.sqr()
	}
	r.reduce()
	return r
}
*/
/* XTR xtr_a function */
func (F *FP4) xtr_A(w *FP4, y *FP4, z *FP4) {
	r := NewFP4copy(w)
	t := NewFP4copy(w)
	r.sub(y)
	r.norm()
	r.pmul(F.a)
	t.add(y)
	t.norm()
	t.pmul(F.b)
	t.times_i()

	F.copy(r)
	F.add(t)
	F.add(z)

	F.norm()
}

/* XTR xtr_d function */
func (F *FP4) xtr_D() {
	w := NewFP4copy(F)
	F.sqr()
	w.conj()
	w.add(w)
	w.norm()
	F.sub(w)
	F.reduce()
}

/* r=x^n using XTR method on traces of FP12s */
func (F *FP4) xtr_pow(n *BIG) *FP4 {
	a := NewFP4int(3)
	b := NewFP4copy(F)
	c := NewFP4copy(b)
	c.xtr_D()
	t := NewFP4()
	r := NewFP4()
	sf := NewFP4copy(F)
	sf.norm()

	par := n.parity()
	v := NewBIGcopy(n)
	v.norm()
	v.fshr(1)
	if par == 0 {
		v.dec(1)
		v.norm()
	}

	nb := v.nbits()
	for i := nb - 1; i >= 0; i-- {
		if v.bit(i) != 1 {
			t.copy(b)
			sf.conj()
			c.conj()
			b.xtr_A(a, sf, c)
			sf.conj()
			c.copy(t)
			c.xtr_D()
			a.xtr_D()
		} else {
			t.copy(a)
			t.conj()
			a.copy(b)
			a.xtr_D()
			b.xtr_A(c, sf, t)
			c.xtr_D()
		}
	}
	if par == 0 {
		r.copy(c)
	} else {
		r.copy(b)
	}
	r.reduce()
	return r
}

/* r=ck^a.cl^n using XTR double exponentiation method on traces of FP12s. See Stam thesis. */
func (F *FP4) xtr_pow2(ck *FP4, ckml *FP4, ckm2l *FP4, a *BIG, b *BIG) *FP4 {

	e := NewBIGcopy(a)
	d := NewBIGcopy(b)
	w := NewBIGint(0)
	e.norm()
	d.norm()

	cu := NewFP4copy(ck) // can probably be passed in w/o copying
	cv := NewFP4copy(F)
	cumv := NewFP4copy(ckml)
	cum2v := NewFP4copy(ckm2l)
	r := NewFP4()
	t := NewFP4()

	f2 := 0
	for d.parity() == 0 && e.parity() == 0 {
		d.fshr(1)
		e.fshr(1)
		f2++
	}

	for Comp(d, e) != 0 {
		if Comp(d, e) > 0 {
			w.copy(e)
			w.imul(4)
			w.norm()
			if Comp(d, w) <= 0 {
				w.copy(d)
				d.copy(e)
				e.rsub(w)
				e.norm()

				t.copy(cv)
				t.xtr_A(cu, cumv, cum2v)
				cum2v.copy(cumv)
				cum2v.conj()
				cumv.copy(cv)
				cv.copy(cu)
				cu.copy(t)
			} else {
				if d.parity() == 0 {
					d.fshr(1)
					r.copy(cum2v)
					r.conj()
					t.copy(cumv)
					t.xtr_A(cu, cv, r)
					cum2v.copy(cumv)
					cum2v.xtr_D()
					cumv.copy(t)
					cu.xtr_D()
				} else {
					if e.parity() == 1 {
						d.sub(e)
						d.norm()
						d.fshr(1)
						t.copy(cv)
						t.xtr_A(cu, cumv, cum2v)
						cu.xtr_D()
						cum2v.copy(cv)
						cum2v.xtr_D()
						cum2v.conj()
						cv.copy(t)
					} else {
						w.copy(d)
						d.copy(e)
						d.fshr(1)
						e.copy(w)
						t.copy(cumv)
						t.xtr_D()
						cumv.copy(cum2v)
						cumv.conj()
						cum2v.copy(t)
						cum2v.conj()
						t.copy(cv)
						t.xtr_D()
						cv.copy(cu)
						cu.copy(t)
					}
				}
			}
		}
		if Comp(d, e) < 0 {
			w.copy(d)
			w.imul(4)
			w.norm()
			if Comp(e, w) <= 0 {
				e.sub(d)
				e.norm()
				t.copy(cv)
				t.xtr_A(cu, cumv, cum2v)
				cum2v.copy(cumv)
				cumv.copy(cu)
				cu.copy(t)
			} else {
				if e.parity() == 0 {
					w.copy(d)
					d.copy(e)
					d.fshr(1)
					e.copy(w)
					t.copy(cumv)
					t.xtr_D()
					cumv.copy(cum2v)
					cumv.conj()
					cum2v.copy(t)
					cum2v.conj()
					t.copy(cv)
					t.xtr_D()
					cv.copy(cu)
					cu.copy(t)
				} else {
					if d.parity() == 1 {
						w.copy(e)
						e.copy(d)
						w.sub(d)
						w.norm()
						d.copy(w)
						d.fshr(1)
						t.copy(cv)
						t.xtr_A(cu, cumv, cum2v)
						cumv.conj()
						cum2v.copy(cu)
						cum2v.xtr_D()
						cum2v.conj()
						cu.copy(cv)
						cu.xtr_D()
						cv.copy(t)
					} else {
						d.fshr(1)
						r.copy(cum2v)
						r.conj()
						t.copy(cumv)
						t.xtr_A(cu, cv, r)
						cum2v.copy(cumv)
						cum2v.xtr_D()
						cumv.copy(t)
						cu.xtr_D()
					}
				}
			}
		}
	}
	r.copy(cv)
	r.xtr_A(cu, cumv, cum2v)
	for i := 0; i < f2; i++ {
		r.xtr_D()
	}
	r = r.xtr_pow(d)
	return r
}

/* this/=2 */
func (F *FP4) div2() {
	F.a.div2()
	F.b.div2()
}

func (F *FP4) div_i() {
	u := NewFP2copy(F.a)
	v := NewFP2copy(F.b)
	u.div_ip()
	F.a.copy(v)
	F.b.copy(u)
	if TOWER == POSITOWER {
		F.neg()
		F.norm()
	}
}
/*
func (F *FP4) pow(b *BIG) {
	w := NewFP4copy(F);
	r := NewFP4int(1)
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
	r.reduce();
	F.copy(r);
}
*/

/* PFGE24S
// Test for Quadratic Residue 
func (F *FP4) qr(h *FP) int {
	c := NewFP4copy(F)
	c.conj()
	c.mul(F)
	return c.a.qr(h)
}

// sqrt(a+ib) = sqrt(a+sqrt(a*a-n*b*b)/2)+ib/(2*sqrt(a+sqrt(a*a-n*b*b)/2)) 
func (F *FP4) sqrt(h *FP)  {
	if F.iszilch() {
		return 
	}

	a := NewFP2copy(F.a)
	b := NewFP2()
	s := NewFP2copy(F.b)
	t := NewFP2copy(F.a)
	hint := NewFP()

	s.sqr()
	a.sqr()
	s.mul_ip()
	s.norm()
	a.sub(s)

	s.copy(a); s.norm()
	s.sqrt(h);

	a.copy(t)
	b.copy(t)

	a.add(s)
	a.norm()
	a.div2()


    b.copy(F.b); b.div2()
    qr:=a.qr(hint)

// tweak hint - multiply old hint by Norm(1/Beta)^e where Beta is irreducible polynomial
    s.copy(a)
	twk:=NewFPbig(NewBIGints(TWK))
    twk.mul(hint)
    s.div_ip(); s.norm()

    a.cmove(s,1-qr)
    hint.cmove(twk,1-qr)

    F.a.copy(a); F.a.sqrt(hint)
    s.copy(a); s.inverse(hint)
    s.mul(F.a)
    F.b.copy(s); F.b.mul(b)
    t.copy(F.a);

    F.a.cmove(F.b,1-qr);
    F.b.cmove(t,1-qr);

	sgn:=F.sign()
	nr:=NewFP4copy(F)
	nr.neg(); nr.norm()
	F.cmove(nr,sgn)
}
PFGE24F */