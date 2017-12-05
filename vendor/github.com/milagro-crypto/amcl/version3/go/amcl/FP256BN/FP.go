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

package FP256BN

//import "fmt"

const NOT_SPECIAL int=0
const PSEUDO_MERSENNE int=1
const MONTGOMERY_FRIENDLY int=2
const GENERALISED_MERSENNE int=3

const MODBITS uint=256 /* Number of bits in Modulus */
const MOD8 uint=3  /* Modulus mod 8 */
const MODTYPE int=NOT_SPECIAL //NOT_SPECIAL

const FEXCESS int32=(int32(1)<<24)
const OMASK Chunk= ((Chunk(-1))<<(MODBITS%BASEBITS))
const TBITS uint=MODBITS%BASEBITS // Number of active bits in top word 
const TMASK Chunk=(Chunk(1)<<TBITS)-1


type FP struct {
	x *BIG
	XES int32
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
	F.XES=a.XES
	return F
}

func (F *FP) toString() string {
	return F.redc().toString()
}

/* convert to Montgomery n-residue form */
func (F *FP) nres() {
	if MODTYPE!=PSEUDO_MERSENNE && MODTYPE!=GENERALISED_MERSENNE {
		r:=NewBIGints(R2modp)
		d:=mul(F.x,r)
		F.x.copy(mod(d))
		F.XES=2
	} else {
		F.XES=1
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

/* reduce a DBIG to a BIG using the appropriate form of the modulus */

func mod(d *DBIG) *BIG {
	if MODTYPE==PSEUDO_MERSENNE {
		t:=d.split(MODBITS)
		b:=NewBIGdcopy(d)

		v:=t.pmul(int(MConst))

		t.add(b)
		t.norm()

		tw:=t.w[NLEN-1]
		t.w[NLEN-1]&=TMASK
		t.w[0]+=(MConst*((tw>>TBITS)+(v<<(BASEBITS-TBITS))))

		t.norm()
		return t
	//	b.add(t)
	//	b.norm()
	//	return b		
	}
	if MODTYPE==MONTGOMERY_FRIENDLY {
		for i:=0;i<NLEN;i++ {
			top,bot:=muladd(d.w[i],MConst-1,d.w[i],d.w[NLEN+i-1])
			d.w[NLEN+i-1]=bot
			d.w[NLEN+i]+=top
		}
		b:=NewBIG()

		for i:=0;i<NLEN;i++ {
			b.w[i]=d.w[NLEN+i]
		}
		b.norm()
		return b		
	}

	if MODTYPE==GENERALISED_MERSENNE { // GoldiLocks only
		t:=d.split(MODBITS)
		b:=NewBIGdcopy(d)
		b.add(t);
		dd:=NewDBIGscopy(t)
		dd.shl(MODBITS/2)

		tt:=dd.split(MODBITS)
		lo:=NewBIGdcopy(dd)
		b.add(tt)
		b.add(lo)
		b.norm()
		tt.shl(MODBITS/2)
		b.add(tt)

		carry:=b.w[NLEN-1]>>TBITS
		b.w[NLEN-1]&=TMASK
		b.w[0]+=carry
			
		b.w[224/BASEBITS]+=carry<<(224%BASEBITS);
		b.norm()
		return b		
	}

	if MODTYPE==NOT_SPECIAL {
		md:=NewBIGints(Modulus)
		return monty(md,MConst,d) 
	}
	return NewBIG()
}


/* reduce this mod Modulus */
func (F *FP) reduce() {
	p:=NewBIGints(Modulus)
	F.x.Mod(p)
	F.XES=1
}

/* test this=0? */
func (F *FP) iszilch() bool {
	F.reduce()
	return F.x.iszilch()
}

/* copy from FP b */
func (F *FP) copy(b *FP ) {
	F.x.copy(b.x)
	F.XES=b.XES
}

/* set this=0 */
func (F *FP) zero() {
	F.x.zero()
	F.XES=1
}
	
/* set this=1 */
func (F *FP) one() {
	F.x.one(); F.nres()
}

/* normalise this */
func (F *FP) norm() {
	F.x.norm()
}

/* swap FPs depending on d */
func (F *FP) cswap(b *FP,d int) {
	c:=int32(d)
	c=^(c-1)
	t:=c&(F.XES^b.XES)
	F.XES^=t
	b.XES^=t
	F.x.cswap(b.x,d)
}

/* copy FPs depending on d */
func (F *FP) cmove(b *FP,d int) {
	F.x.cmove(b.x,d)
	c:=int32(-d)
	F.XES^=(F.XES^b.XES)&c
}

/* this*=b mod Modulus */
func (F *FP) mul(b *FP) {

	if int64(F.XES)*int64(b.XES)>int64(FEXCESS) {F.reduce()}

	d:=mul(F.x,b.x)
	F.x.copy(mod(d))
	F.XES=2
}

/* this = -this mod Modulus */
func (F *FP) neg() {
	m:=NewBIGints(Modulus)
	sb:=logb2(uint32(F.XES-1))

	m.fshl(sb)
	F.x.rsub(m)		

	F.XES=(1<<sb)
	if F.XES>FEXCESS {F.reduce()}
}


/* this*=c mod Modulus, where c is a small int */
func (F *FP) imul(c int) {
//	F.norm()
	s:=false
	if (c<0) {
		c=-c
		s=true
	}

	if MODTYPE==PSEUDO_MERSENNE || MODTYPE==GENERALISED_MERSENNE {
		d:=F.x.pxmul(c)
		F.x.copy(mod(d))
		F.XES=2
	} else {
		if F.XES*int32(c)<=FEXCESS {
			F.x.pmul(c)
			F.XES*=int32(c)
		} else {
			n:=NewFPint(c)
			F.mul(n)
		}
	}
	if s {F.neg(); F.norm()}
}

/* this*=this mod Modulus */
func (F *FP) sqr() {
	if int64(F.XES)*int64(F.XES)>int64(FEXCESS) {F.reduce()}	
	d:=sqr(F.x)	
	F.x.copy(mod(d))
	F.XES=2
}

/* this+=b */
func (F *FP) add(b *FP) {
	F.x.add(b.x)
	F.XES+=b.XES
	if (F.XES>FEXCESS) {F.reduce()}
}

/* this-=b */
func (F *FP) sub(b *FP) {
	n:=NewFPcopy(b)
	n.neg()
	F.add(n)
}

func (F *FP) rsub(b *FP) {
	F.neg()
	F.add(b)
}

/* this/=2 mod Modulus */
func (F *FP) div2() {
//	F.x.norm()
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
	r.Invmodp(p)
	F.x.copy(r)
	F.nres()
}

/* return TRUE if this==a */
func (F *FP) Equals(a *FP) bool {
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
	r.x.Mod(p);
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
	return w.Jacobi(p)
}
