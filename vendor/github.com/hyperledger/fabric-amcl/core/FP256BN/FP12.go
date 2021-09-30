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

/* MiotCL Fp^12 functions */
/* FP12 elements are of the form a+i.b+i^2.c */

package FP256BN



type FP12 struct {
	a     *FP4
	b     *FP4
	c     *FP4
	stype int
}

/* Constructors */
func NewFP12fp4(d *FP4) *FP12 {
	F := new(FP12)
	F.a = NewFP4copy(d)
	F.b = NewFP4()
	F.c = NewFP4()
	F.stype = FP_SPARSEST
	return F
}

func NewFP12() *FP12 {
	F := new(FP12)
	F.a = NewFP4()
	F.b = NewFP4()
	F.c = NewFP4()
	F.stype = FP_ZERO

	return F
}

func NewFP12int(d int) *FP12 {
	F := new(FP12)
	F.a = NewFP4int(d)
	F.b = NewFP4()
	F.c = NewFP4()
	if d == 1 {
		F.stype = FP_ONE
	} else {
		F.stype = FP_SPARSEST
	}
	return F
}

func NewFP12fp4s(d *FP4, e *FP4, f *FP4) *FP12 {
	F := new(FP12)
	F.a = NewFP4copy(d)
	F.b = NewFP4copy(e)
	F.c = NewFP4copy(f)
	F.stype = FP_DENSE
	return F
}

func NewFP12copy(x *FP12) *FP12 {
	F := new(FP12)
	F.a = NewFP4copy(x.a)
	F.b = NewFP4copy(x.b)
	F.c = NewFP4copy(x.c)
	F.stype = x.stype
	return F
}

/* reduce all components of this mod Modulus */
func (F *FP12) reduce() {
	F.a.reduce()
	F.b.reduce()
	F.c.reduce()
}

/* normalise all components of this */
func (F *FP12) norm() {
	F.a.norm()
	F.b.norm()
	F.c.norm()
}

/* test x==0 ? */
func (F *FP12) iszilch() bool {
	return (F.a.iszilch() && F.b.iszilch() && F.c.iszilch())
}

/* Conditional move */
func (F *FP12) cmove(g *FP12, d int) {
	F.a.cmove(g.a, d)
	F.b.cmove(g.b, d)
	F.c.cmove(g.c, d)
	d = ^(d - 1)
	F.stype ^= (F.stype ^ g.stype) & d
}

/* Constant time select from pre-computed table */
func (F *FP12) selector(g []*FP12, b int32) {

	m := b >> 31
	babs := (b ^ m) - m

	babs = (babs - 1) / 2

	F.cmove(g[0], teq(babs, 0)) // conditional move
	F.cmove(g[1], teq(babs, 1))
	F.cmove(g[2], teq(babs, 2))
	F.cmove(g[3], teq(babs, 3))
	F.cmove(g[4], teq(babs, 4))
	F.cmove(g[5], teq(babs, 5))
	F.cmove(g[6], teq(babs, 6))
	F.cmove(g[7], teq(babs, 7))

	invF := NewFP12copy(F)
	invF.conj()
	F.cmove(invF, int(m&1))
}

/* test x==1 ? */
func (F *FP12) Isunity() bool {
	one := NewFP4int(1)
	return (F.a.Equals(one) && F.b.iszilch() && F.c.iszilch())
}

/* return 1 if x==y, else 0 */
func (F *FP12) Equals(x *FP12) bool {
	return (F.a.Equals(x.a) && F.b.Equals(x.b) && F.c.Equals(x.c))
}

/* extract a from this */
func (F *FP12) geta() *FP4 {
	return F.a
}

/* extract b */
func (F *FP12) getb() *FP4 {
	return F.b
}

/* extract c */
func (F *FP12) getc() *FP4 {
	return F.c
}

/* copy this=x */
func (F *FP12) Copy(x *FP12) {
	F.a.copy(x.a)
	F.b.copy(x.b)
	F.c.copy(x.c)
	F.stype = x.stype
}

/* set this=1 */
func (F *FP12) one() {
	F.a.one()
	F.b.zero()
	F.c.zero()
	F.stype = FP_ONE
}

/* set this=0 */
func (F *FP12) zero() {
	F.a.zero()
	F.b.zero()
	F.c.zero()
	F.stype = FP_ZERO
}

/* this=conj(this) */
func (F *FP12) conj() {
	F.a.conj()
	F.b.nconj()
	F.c.conj()
}

/* Granger-Scott Unitary Squaring */
func (F *FP12) usqr() {
	A := NewFP4copy(F.a)
	B := NewFP4copy(F.c)
	C := NewFP4copy(F.b)
	D := NewFP4()

	F.a.sqr()
	D.copy(F.a)
	D.add(F.a)
	F.a.add(D)

	F.a.norm()
	A.nconj()

	A.add(A)
	F.a.add(A)
	B.sqr()
	B.times_i()

	D.copy(B)
	D.add(B)
	B.add(D)
	B.norm()

	C.sqr()
	D.copy(C)
	D.add(C)
	C.add(D)
	C.norm()

	F.b.conj()
	F.b.add(F.b)
	F.c.nconj()

	F.c.add(F.c)
	F.b.add(B)
	F.c.add(C)
	F.reduce()
	F.stype = FP_DENSE

}

/* Chung-Hasan SQR2 method from http://cacr.uwaterloo.ca/techreports/2006/cacr2006-24.pdf */
func (F *FP12) sqr() {
	if F.stype == FP_ONE {
		return
	}

	A := NewFP4copy(F.a)
	B := NewFP4copy(F.b)
	C := NewFP4copy(F.c)
	D := NewFP4copy(F.a)

	A.sqr()
	B.mul(F.c)
	B.add(B)
	B.norm()
	C.sqr()
	D.mul(F.b)
	D.add(D)

	F.c.add(F.a)
	F.c.add(F.b)
	F.c.norm()
	F.c.sqr()

	F.a.copy(A)

	A.add(B)
	A.norm()
	A.add(C)
	A.add(D)
	A.norm()

	A.neg()
	B.times_i()
	C.times_i()

	F.a.add(B)

	F.b.copy(C)
	F.b.add(D)
	F.c.add(A)
	if F.stype == FP_SPARSER || F.stype == FP_SPARSEST {
		F.stype = FP_SPARSE
	} else {
		F.stype = FP_DENSE
	}
	F.norm()
}

/* FP12 full multiplication this=this*y */
func (F *FP12) Mul(y *FP12) {
	z0 := NewFP4copy(F.a)
	z1 := NewFP4()
	z2 := NewFP4copy(F.b)
	z3 := NewFP4()
	t0 := NewFP4copy(F.a)
	t1 := NewFP4copy(y.a)

	z0.mul(y.a)
	z2.mul(y.b)

	t0.add(F.b)
	t0.norm()
	t1.add(y.b)
	t1.norm()

	z1.copy(t0)
	z1.mul(t1)
	t0.copy(F.b)
	t0.add(F.c)
	t0.norm()

	t1.copy(y.b)
	t1.add(y.c)
	t1.norm()
	z3.copy(t0)
	z3.mul(t1)

	t0.copy(z0)
	t0.neg()
	t1.copy(z2)
	t1.neg()

	z1.add(t0)
	F.b.copy(z1)
	F.b.add(t1)

	z3.add(t1)
	z2.add(t0)

	t0.copy(F.a)
	t0.add(F.c)
	t0.norm()
	t1.copy(y.a)
	t1.add(y.c)
	t1.norm()
	t0.mul(t1)
	z2.add(t0)

	t0.copy(F.c)
	t0.mul(y.c)
	t1.copy(t0)
	t1.neg()

	F.c.copy(z2)
	F.c.add(t1)
	z3.add(t1)
	t0.times_i()
	F.b.add(t0)
	z3.norm()
	z3.times_i()
	F.a.copy(z0)
	F.a.add(z3)
	F.stype = FP_DENSE
	F.norm()
}

/* FP12 full multiplication w=w*y */
/* Supports sparse multiplicands */
/* Usually w is denser than y */
func (F *FP12) ssmul(y *FP12) {
	if F.stype == FP_ONE {
		F.Copy(y)
		return
	}
	if y.stype == FP_ONE {
		return
	}
	if y.stype >= FP_SPARSE {
		z0 := NewFP4copy(F.a)
		z1 := NewFP4()
		z2 := NewFP4()
		z3 := NewFP4()
		z0.mul(y.a)

		if SEXTIC_TWIST == M_TYPE {
			if y.stype == FP_SPARSE || F.stype == FP_SPARSE {
				z2.getb().copy(F.b.getb())
				z2.getb().mul(y.b.getb())
				z2.geta().zero()
				if y.stype != FP_SPARSE {
					z2.geta().copy(F.b.getb())
					z2.geta().mul(y.b.geta())
				}
				if F.stype != FP_SPARSE {
					z2.geta().copy(F.b.geta())
					z2.geta().mul(y.b.getb())
				}
				z2.times_i()
			} else {
				z2.copy(F.b)
				z2.mul(y.b)
			}
		} else {
			z2.copy(F.b)
			z2.mul(y.b)
		}
		t0 := NewFP4copy(F.a)
		t1 := NewFP4copy(y.a)
		t0.add(F.b)
		t0.norm()
		t1.add(y.b)
		t1.norm()

		z1.copy(t0)
		z1.mul(t1)
		t0.copy(F.b)
		t0.add(F.c)
		t0.norm()
		t1.copy(y.b)
		t1.add(y.c)
		t1.norm()

		z3.copy(t0)
		z3.mul(t1)

		t0.copy(z0)
		t0.neg()
		t1.copy(z2)
		t1.neg()

		z1.add(t0)
		F.b.copy(z1)
		F.b.add(t1)

		z3.add(t1)
		z2.add(t0)

		t0.copy(F.a)
		t0.add(F.c)
		t0.norm()
		t1.copy(y.a)
		t1.add(y.c)
		t1.norm()

		t0.mul(t1)
		z2.add(t0)

		if SEXTIC_TWIST == D_TYPE {
			if y.stype == FP_SPARSE || F.stype == FP_SPARSE {
				t0.geta().copy(F.c.geta())
				t0.geta().mul(y.c.geta())
				t0.getb().zero()
				if y.stype != FP_SPARSE {
					t0.getb().copy(F.c.geta())
					t0.getb().mul(y.c.getb())
				}
				if F.stype != FP_SPARSE {
					t0.getb().copy(F.c.getb())
					t0.getb().mul(y.c.geta())
				}
			} else {
				t0.copy(F.c)
				t0.mul(y.c)
			}
		} else {
			t0.copy(F.c)
			t0.mul(y.c)
		}
		t1.copy(t0)
		t1.neg()

		F.c.copy(z2)
		F.c.add(t1)
		z3.add(t1)
		t0.times_i()
		F.b.add(t0)
		z3.norm()
		z3.times_i()
		F.a.copy(z0)
		F.a.add(z3)
	} else {
		if F.stype == FP_SPARSER || F.stype == FP_SPARSEST {
			F.smul(y)
			return
		}
		if SEXTIC_TWIST == D_TYPE { // dense by sparser - 13m
			z0 := NewFP4copy(F.a)
			z2 := NewFP4copy(F.b)
			z3 := NewFP4copy(F.b)
			t0 := NewFP4()
			t1 := NewFP4copy(y.a)
			z0.mul(y.a)

			if y.stype == FP_SPARSEST {
				z2.qmul(y.b.a.a)
			} else {
				z2.pmul(y.b.a)
			}
			F.b.add(F.a)
			t1.geta().add(y.b.geta())

			t1.norm()
			F.b.norm()
			F.b.mul(t1)
			z3.add(F.c)
			z3.norm()

			if y.stype == FP_SPARSEST {
				z3.qmul(y.b.a.a)
			} else {
				z3.pmul(y.b.a)
			}

			t0.copy(z0)
			t0.neg()
			t1.copy(z2)
			t1.neg()

			F.b.add(t0)

			F.b.add(t1)
			z3.add(t1)
			z2.add(t0)

			t0.copy(F.a)
			t0.add(F.c)
			t0.norm()
			z3.norm()
			t0.mul(y.a)
			F.c.copy(z2)
			F.c.add(t0)

			z3.times_i()
			F.a.copy(z0)
			F.a.add(z3)
		}
		if SEXTIC_TWIST == M_TYPE {
			z0 := NewFP4copy(F.a)
			z1 := NewFP4()
			z2 := NewFP4()
			z3 := NewFP4()
			t0 := NewFP4copy(F.a)
			t1 := NewFP4()

			z0.mul(y.a)
			t0.add(F.b)
			t0.norm()

			z1.copy(t0)
			z1.mul(y.a)
			t0.copy(F.b)
			t0.add(F.c)
			t0.norm()

			z3.copy(t0)

			if y.stype == FP_SPARSEST {
				z3.qmul(y.c.b.a)
			} else {
				z3.pmul(y.c.b)
			}
			z3.times_i()

			t0.copy(z0)
			t0.neg()
			z1.add(t0)
			F.b.copy(z1)
			z2.copy(t0)

			t0.copy(F.a)
			t0.add(F.c)
			t0.norm()
			t1.copy(y.a)
			t1.add(y.c)
			t1.norm()

			t0.mul(t1)
			z2.add(t0)
			t0.copy(F.c)

			if y.stype == FP_SPARSEST {
				t0.qmul(y.c.b.a)
			} else {
				t0.pmul(y.c.b)
			}
			t0.times_i()
			t1.copy(t0)
			t1.neg()

			F.c.copy(z2)
			F.c.add(t1)
			z3.add(t1)
			t0.times_i()
			F.b.add(t0)
			z3.norm()
			z3.times_i()
			F.a.copy(z0)
			F.a.add(z3)
		}
	}
	F.stype = FP_DENSE
	F.norm()
}

/* Special case of multiplication arises from special form of ATE pairing line function */
/* F and y are both sparser or sparsest line functions - cost <= 6m */
func (F *FP12) smul(y *FP12) {
	if SEXTIC_TWIST == D_TYPE {
		w1 := NewFP2copy(F.a.geta())
		w2 := NewFP2copy(F.a.getb())
		var w3 *FP2

		w1.mul(y.a.geta())
		w2.mul(y.a.getb())

		if y.stype == FP_SPARSEST || F.stype == FP_SPARSEST {
			if y.stype == FP_SPARSEST && F.stype == FP_SPARSEST {
				t := NewFPcopy(F.b.a.a)
				t.mul(y.b.a.a)
				w3 = NewFP2fp(t)
			} else {
				if y.stype != FP_SPARSEST {
					w3 = NewFP2copy(y.b.geta())
					w3.pmul(F.b.a.a)
				} else {
					w3 = NewFP2copy(F.b.geta())
					w3.pmul(y.b.a.a)
				}
			}
		} else {
			w3 = NewFP2copy(F.b.geta())
			w3.mul(y.b.geta())
		}

		ta := NewFP2copy(F.a.geta())
		tb := NewFP2copy(y.a.geta())
		ta.add(F.a.getb())
		ta.norm()
		tb.add(y.a.getb())
		tb.norm()
		tc := NewFP2copy(ta)
		tc.mul(tb)
		t := NewFP2copy(w1)
		t.add(w2)
		t.neg()
		tc.add(t)

		ta.copy(F.a.geta())
		ta.add(F.b.geta())
		ta.norm()
		tb.copy(y.a.geta())
		tb.add(y.b.geta())
		tb.norm()
		td := NewFP2copy(ta)
		td.mul(tb)
		t.copy(w1)
		t.add(w3)
		t.neg()
		td.add(t)

		ta.copy(F.a.getb())
		ta.add(F.b.geta())
		ta.norm()
		tb.copy(y.a.getb())
		tb.add(y.b.geta())
		tb.norm()
		te := NewFP2copy(ta)
		te.mul(tb)
		t.copy(w2)
		t.add(w3)
		t.neg()
		te.add(t)

		w2.mul_ip()
		w1.add(w2)

		F.a.geta().copy(w1)
		F.a.getb().copy(tc)
		F.b.geta().copy(td)
		F.b.getb().copy(te)
		F.c.geta().copy(w3)
		F.c.getb().zero()

		F.a.norm()
		F.b.norm()
	} else {
		w1 := NewFP2copy(F.a.geta())
		w2 := NewFP2copy(F.a.getb())
		var w3 *FP2

		w1.mul(y.a.geta())
		w2.mul(y.a.getb())

		if y.stype == FP_SPARSEST || F.stype == FP_SPARSEST {
			if y.stype == FP_SPARSEST && F.stype == FP_SPARSEST {
				t := NewFPcopy(F.c.b.a)
				t.mul(y.c.b.a)
				w3 = NewFP2fp(t)
			} else {
				if y.stype != FP_SPARSEST {
					w3 = NewFP2copy(y.c.getb())
					w3.pmul(F.c.b.a)
				} else {
					w3 = NewFP2copy(F.c.getb())
					w3.pmul(y.c.b.a)
				}
			}
		} else {
			w3 = NewFP2copy(F.c.getb())
			w3.mul(y.c.getb())
		}

		ta := NewFP2copy(F.a.geta())
		tb := NewFP2copy(y.a.geta())
		ta.add(F.a.getb())
		ta.norm()
		tb.add(y.a.getb())
		tb.norm()
		tc := NewFP2copy(ta)
		tc.mul(tb)
		t := NewFP2copy(w1)
		t.add(w2)
		t.neg()
		tc.add(t)

		ta.copy(F.a.geta())
		ta.add(F.c.getb())
		ta.norm()
		tb.copy(y.a.geta())
		tb.add(y.c.getb())
		tb.norm()
		td := NewFP2copy(ta)
		td.mul(tb)
		t.copy(w1)
		t.add(w3)
		t.neg()
		td.add(t)

		ta.copy(F.a.getb())
		ta.add(F.c.getb())
		ta.norm()
		tb.copy(y.a.getb())
		tb.add(y.c.getb())
		tb.norm()
		te := NewFP2copy(ta)
		te.mul(tb)
		t.copy(w2)
		t.add(w3)
		t.neg()
		te.add(t)

		w2.mul_ip()
		w1.add(w2)
		F.a.geta().copy(w1)
		F.a.getb().copy(tc)

		w3.mul_ip()
		w3.norm()
		F.b.geta().zero()
		F.b.getb().copy(w3)

		te.norm()
		te.mul_ip()
		F.c.geta().copy(te)
		F.c.getb().copy(td)

		F.a.norm()
		F.c.norm()

	}
	F.stype = FP_SPARSE
}

/* this=1/this */
func (F *FP12) Inverse() {
	f0 := NewFP4copy(F.a)
	f1 := NewFP4copy(F.b)
	f2 := NewFP4copy(F.a)
	f3 := NewFP4()

	F.norm()
	f0.sqr()
	f1.mul(F.c)
	f1.times_i()
	f0.sub(f1)
	f0.norm()

	f1.copy(F.c)
	f1.sqr()
	f1.times_i()
	f2.mul(F.b)
	f1.sub(f2)
	f1.norm()

	f2.copy(F.b)
	f2.sqr()
	f3.copy(F.a)
	f3.mul(F.c)
	f2.sub(f3)
	f2.norm()

	f3.copy(F.b)
	f3.mul(f2)
	f3.times_i()
	F.a.mul(f0)
	f3.add(F.a)
	F.c.mul(f1)
	F.c.times_i()

	f3.add(F.c)
	f3.norm()
	f3.inverse(nil)
	F.a.copy(f0)
	F.a.mul(f3)
	F.b.copy(f1)
	F.b.mul(f3)
	F.c.copy(f2)
	F.c.mul(f3)
	F.stype = FP_DENSE
}

/* this=this^p using Frobenius */
func (F *FP12) frob(f *FP2) {
	f2 := NewFP2copy(f)
	f3 := NewFP2copy(f)

	f2.sqr()
	f3.mul(f2)

	F.a.frob(f3)
	F.b.frob(f3)
	F.c.frob(f3)

	F.b.pmul(f)
	F.c.pmul(f2)
	F.stype = FP_DENSE
}

/* trace function */
func (F *FP12) trace() *FP4 {
	t := NewFP4()
	t.copy(F.a)
	t.imul(3)
	t.reduce()
	return t
}

/* convert from byte array to FP12 */
func FP12_fromBytes(w []byte) *FP12 {
	var t [4*int(MODBYTES)]byte
	MB := 4*int(MODBYTES)

	for i:=0;i<MB;i++ {
		t[i]=w[i]
	}
    c:=FP4_fromBytes(t[:])
	for i:=0;i<MB;i++ {
		t[i]=w[i+MB]
	}
    b:=FP4_fromBytes(t[:])
	for i:=0;i<MB;i++ {
		t[i]=w[i+2*MB]
	}
    a:=FP4_fromBytes(t[:])
	return NewFP12fp4s(a,b,c)
}

/* convert this to byte array */
func (F *FP12) ToBytes(w []byte) {
	var t [4*int(MODBYTES)]byte
	MB := 4*int(MODBYTES)
    F.c.ToBytes(t[:])
	for i:=0;i<MB;i++ { 
		w[i]=t[i]
	}
    F.b.ToBytes(t[:])
	for i:=0;i<MB;i++ {
		w[i+MB]=t[i]
	}
    F.a.ToBytes(t[:])
	for i:=0;i<MB;i++ {
		w[i+2*MB]=t[i]
	}
}

/* convert to hex string */
func (F *FP12) ToString() string {
	return ("[" + F.a.toString() + "," + F.b.toString() + "," + F.c.toString() + "]")
}

/* this=this^e */
func (F *FP12) Pow(e *BIG) *FP12 {
	//F.norm()
	e1 := NewBIGcopy(e)
	e1.norm()
	e3 := NewBIGcopy(e1)
	e3.pmul(3)
	e3.norm()
	sf := NewFP12copy(F)
	sf.norm()
	w := NewFP12copy(sf)
    if e3.iszilch() {
        w.one()
        return w
    }
	nb := e3.nbits()
	for i := nb - 2; i >= 1; i-- {
		w.usqr()
		bt := e3.bit(i) - e1.bit(i)
		if bt == 1 {
			w.Mul(sf)
		}
		if bt == -1 {
			sf.conj()
			w.Mul(sf)
			sf.conj()
		}
	}
	w.reduce()
	return w
}

/* constant time powering by small integer of max length bts */
func (F *FP12) pinpow(e int, bts int) {
	var R []*FP12
	R = append(R, NewFP12int(1))
	R = append(R, NewFP12copy(F))

	for i := bts - 1; i >= 0; i-- {
		b := (e >> uint(i)) & 1
		R[1-b].Mul(R[b])
		R[b].usqr()
	}
	F.Copy(R[0])
}

/* Fast compressed FP4 power of unitary FP12 */
func (F *FP12) Compow(e *BIG, r *BIG) *FP4 {
	q := NewBIGints(Modulus)
	f := NewFP2bigs(NewBIGints(Fra), NewBIGints(Frb))

	m := NewBIGcopy(q)
	m.Mod(r)

	a := NewBIGcopy(e)
	a.Mod(m)

	b := NewBIGcopy(e)
	b.div(m)

	g1 := NewFP12copy(F)
	c := g1.trace()

	if b.iszilch() {
		c = c.xtr_pow(e)
		return c
	}

	g2 := NewFP12copy(F)
	g2.frob(f)
	cp := g2.trace()

	g1.conj()
	g2.Mul(g1)
	cpm1 := g2.trace()
	g2.Mul(g1)
	cpm2 := g2.trace()

	c = c.xtr_pow2(cp, cpm1, cpm2, a, b)
	return c
}

/* p=q0^u0.q1^u1.q2^u2.q3^u3 */
// Bos & Costello https://eprint.iacr.org/2013/458.pdf
// Faz-Hernandez & Longa & Sanchez  https://eprint.iacr.org/2013/158.pdf
// Side channel attack secure

func pow4(q []*FP12, u []*BIG) *FP12 {
	var g []*FP12
	var w [NLEN*int(BASEBITS) + 1]int8
	var s [NLEN*int(BASEBITS) + 1]int8
	var t []*BIG
	r := NewFP12()
	p := NewFP12()
	mt := NewBIGint(0)

	for i := 0; i < 4; i++ {
		t = append(t, NewBIGcopy(u[i]))
	}

	g = append(g, NewFP12copy(q[0])) // q[0]
	g = append(g, NewFP12copy(g[0]))
	g[1].Mul(q[1]) // q[0].q[1]
	g = append(g, NewFP12copy(g[0]))
	g[2].Mul(q[2]) // q[0].q[2]
	g = append(g, NewFP12copy(g[1]))
	g[3].Mul(q[2]) // q[0].q[1].q[2]
	g = append(g, NewFP12copy(g[0]))
	g[4].Mul(q[3]) // q[0].q[3]
	g = append(g, NewFP12copy(g[1]))
	g[5].Mul(q[3]) // q[0].q[1].q[3]
	g = append(g, NewFP12copy(g[2]))
	g[6].Mul(q[3]) // q[0].q[2].q[3]
	g = append(g, NewFP12copy(g[3]))
	g[7].Mul(q[3]) // q[0].q[1].q[2].q[3]

	// Make it odd
	pb := 1 - t[0].parity()
	t[0].inc(pb)
	//	t[0].norm();

	// Number of bits
	mt.zero()
	for i := 0; i < 4; i++ {
		t[i].norm()
		mt.or(t[i])
	}

	nb := 1 + mt.nbits()

	// Sign pivot
	s[nb-1] = 1
	for i := 0; i < nb-1; i++ {
		t[0].fshr(1)
		s[i] = 2*int8(t[0].parity()) - 1
	}

	// Recoded exponent
	for i := 0; i < nb; i++ {
		w[i] = 0
		k := 1
		for j := 1; j < 4; j++ {
			bt := s[i] * int8(t[j].parity())
			t[j].fshr(1)
			t[j].dec(int(bt) >> 1)
			t[j].norm()
			w[i] += bt * int8(k)
			k *= 2
		}
	}

	// Main loop
	p.selector(g, int32(2*w[nb-1]+1))
	for i := nb - 2; i >= 0; i-- {
		p.usqr()
		r.selector(g, int32(2*w[i]+s[i]))
		p.Mul(r)
	}

	// apply correction
	r.Copy(q[0])
	r.conj()
	r.Mul(p)
	p.cmove(r, pb)

	p.reduce()
	return p
}
