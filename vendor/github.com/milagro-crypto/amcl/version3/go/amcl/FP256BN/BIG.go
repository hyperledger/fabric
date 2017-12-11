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

/* AMCL BIG number class */ 

package FP256BN

import "strconv"
import "github.com/milagro-crypto/amcl/version3/go/amcl"

//import "fmt"

const MODBYTES uint=32
const BASEBITS uint=56

const NLEN int=int((1+((8*MODBYTES-1)/BASEBITS)))
const DNLEN int=2*NLEN
const BMASK Chunk= ((Chunk(1)<<BASEBITS)-1)
const HBITS uint=(BASEBITS/2)
const HMASK Chunk= ((Chunk(1)<<HBITS)-1)
//const NEXCESS int=4
const NEXCESS int=(1<<(uint(CHUNK)-BASEBITS-1));

const BIGBITS int=int(MODBYTES*8)


type BIG struct {
	w [NLEN]Chunk
}

type DBIG struct {
	w [2*NLEN]Chunk
}

/*
func (r *BIG) isok() bool {
	ok:=true
	for i:=0;i<NLEN;i++ {
		if (r.w[i]>>BASEBITS)!=0 {
			ok=false
		}
	}
	return ok;
}
*/
/***************** 64-bit specific code ****************/

/* First the 32/64-bit dependent BIG code */
/* Note that because of the lack of a 128-bit integer, 32 and 64-bit code needs to be done differently */

/* return a*b as DBIG */
func mul(a *BIG,b *BIG) *DBIG {
	c:=NewDBIG()
	carry:= Chunk(0)
//	a.norm()
//	b.norm()

//	if !a.isok() || !b.isok() {fmt.Printf("Problem in mul\n")}


	for i:=0;i<NLEN;i++ {
		carry=0
		for j:=0;j<NLEN;j++ {
			carry,c.w[i+j]=muladd(a.w[i],b.w[j],carry,c.w[i+j])
			//carry=c.muladd(a.w[i],b.w[j],carry,i+j)
		}
		c.w[NLEN+i]=carry
	}
	
	return c
}

/* return a^2 as DBIG */
func sqr(a *BIG) *DBIG {
	c:=NewDBIG()
	carry:= Chunk(0)
//	a.norm()

//if !a.isok() {fmt.Printf("Problem in sqr")}

	for i:=0;i<NLEN;i++ {
		carry=0;
		for j:=i+1;j<NLEN;j++ {
			carry,c.w[i+j]=muladd(2*a.w[i],a.w[j],carry,c.w[i+j])
			//carry=c.muladd(2*a.w[i],a.w[j],carry,i+j)
		}
		c.w[NLEN+i]=carry
	}

	for i:=0;i<NLEN;i++ {
		top,bot:=muladd(a.w[i],a.w[i],0,c.w[2*i])
		c.w[2*i]=bot
		c.w[2*i+1]+=top
		//c.w[2*i+1]+=c.muladd(a.w[i],a.w[i],0,2*i)

	}
	c.norm()
	return c
}

func monty(md* BIG, mc Chunk,d* DBIG) *BIG {
	carry:=Chunk(0)
	m:=Chunk(0)
	for i:=0;i<NLEN;i++ {
		if (mc==-1) { 
			m=(-d.w[i])&BMASK
		} else {
			if (mc==1) {
				m=d.w[i]
			} else {m=(mc*d.w[i])&BMASK}
		}

		carry=0
		for j:=0;j<NLEN;j++ {
			carry,d.w[i+j]=muladd(m,md.w[j],carry,d.w[i+j])
				//carry=d.muladd(m,md.w[j],carry,i+j)
		}
		d.w[NLEN+i]+=carry
	}

	b:=NewBIG()
	for i:=0;i<NLEN;i++ {
		b.w[i]=d.w[NLEN+i]
	}
	b.norm()
	return b		
}

/* set this[i]+=x*y+c, and return high part */
func muladd(a Chunk,b Chunk,c Chunk,r Chunk) (Chunk,Chunk) {
	x0:=a&HMASK
	x1:=(a>>HBITS)
	y0:=b&HMASK;
	y1:=(b>>HBITS)
	bot:=x0*y0
	top:=x1*y1
	mid:=x0*y1+x1*y0
	x0=mid&HMASK;
	x1=(mid>>HBITS)
	bot+=x0<<HBITS; bot+=c; bot+=r 
	top+=x1;
	carry:=bot>>BASEBITS
	bot&=BMASK
	top+=carry
	return top,bot
}

/************************************************************/

func (r *BIG) get(i int) Chunk {
	return r.w[i] 
}

func (r *BIG) set(i int,x Chunk) {
	r.w[i]=x	
}

func (r *BIG) xortop(x Chunk) {
	r.w[NLEN-1]^=x
}

/* normalise BIG - force all digits < 2^BASEBITS */
func (r *BIG) norm() Chunk {
	carry:=Chunk(0)
	for i:=0;i<NLEN-1;i++ {
		d:=r.w[i]+carry
		r.w[i]=d&BMASK
		carry=d>>BASEBITS
	}
	r.w[NLEN-1]=(r.w[NLEN-1]+carry)
	return (r.w[NLEN-1]>>((8*MODBYTES)%BASEBITS))  
}

/* Shift right by less than a word */
func (r *BIG) fshr(k uint) int {
	w:=r.w[0]&((Chunk(1)<<k)-1) /* shifted out part */
	for i:=0;i<NLEN-1;i++ {
		r.w[i]=(r.w[i]>>k)|((r.w[i+1]<<(BASEBITS-k))&BMASK)
	}
	r.w[NLEN-1]=r.w[NLEN-1]>>k
	return int(w)
}

/* Shift right by less than a word */
func (r *BIG) fshl(k uint) int {
	r.w[NLEN-1]=((r.w[NLEN-1]<<k))|(r.w[NLEN-2]>>(BASEBITS-k))
	for i:=NLEN-2;i>0;i-- {
		r.w[i]=((r.w[i]<<k)&BMASK)|(r.w[i-1]>>(BASEBITS-k))
	}
	r.w[0]=(r.w[0]<<k)&BMASK
	return int(r.w[NLEN-1]>>((8*MODBYTES)%BASEBITS)) /* return excess - only used in ff.c */
}

func NewBIG() *BIG {
	b:=new(BIG)
	for i:=0;i<NLEN;i++ {
		b.w[i]=0
	}
	return b
}

func NewBIGints(x [NLEN]Chunk) *BIG {
	b:=new(BIG)
	for i:=0;i<NLEN;i++ {
		b.w[i]=x[i]
	}
	return b	
}

func NewBIGint(x int) *BIG {
	b:=new(BIG)
	b.w[0]=Chunk(x)
	for i:=1;i<NLEN;i++ {
		b.w[i]=0
	}
	return b
}

func NewBIGcopy(x *BIG) *BIG {
	b:=new(BIG)
	for i:=0;i<NLEN;i++ {
		b.w[i]=x.w[i]
	}
	return b
}

func NewBIGdcopy(x *DBIG) *BIG {
	b:=new(BIG)
	for i:=0;i<NLEN;i++ {
		b.w[i]=x.w[i]
	}
	return b
}

/* test for zero */
func (r *BIG) iszilch() bool {
	for i:=0;i<NLEN;i++ {
		if r.w[i]!=0 {return false}
	}
	return true; 
}

/* set to zero */
func (r *BIG) zero() {
	for i:=0;i<NLEN;i++ {
		r.w[i]=0
	}
}

/* Test for equal to one */
func (r *BIG) isunity() bool {
	for i:=1;i<NLEN;i++ {
		if r.w[i]!=0 {return false}
	}
	if r.w[0]!=1 {return false}
	return true;
}


/* set to one */
func (r *BIG) one() {
	r.w[0]=1
	for i:=1;i<NLEN;i++ {
		r.w[i]=0
	}
}

/* Copy from another BIG */
func (r *BIG) copy(x *BIG) {
	for i:=0;i<NLEN;i++ {
		r.w[i]=x.w[i]
	}
}

/* Copy from another DBIG */
func (r *BIG) dcopy(x *DBIG) {
	for i:=0;i<NLEN;i++ {
		r.w[i]=x.w[i]
	}
}

/* Conditional swap of two bigs depending on d using XOR - no branches */
func (r *BIG) cswap(b *BIG,d int) {
	c:=Chunk(d)
	c=^(c-1)

	for i:=0;i<NLEN;i++ {
		t:=c&(r.w[i]^b.w[i])
		r.w[i]^=t
		b.w[i]^=t
	}
}

func (r *BIG) cmove(g *BIG,d int){
	b:=Chunk(-d)

	for i:=0;i<NLEN;i++ {
		r.w[i]^=(r.w[i]^g.w[i])&b
	}
}

/* general shift right */
func (r *BIG) shr(k uint) {
	n:=(k%BASEBITS)
	m:=int(k/BASEBITS)	
	for i:=0;i<NLEN-m-1;i++ {
		r.w[i]=(r.w[m+i]>>n)|((r.w[m+i+1]<<(BASEBITS-n))&BMASK)
	}
	r.w[NLEN-m-1]=r.w[NLEN-1]>>n;
	for i:=NLEN-m;i<NLEN;i++ {r.w[i]=0}
}


/* general shift left */
func (r *BIG) shl(k uint) {
	n:=k%BASEBITS
	m:=int(k/BASEBITS)

	r.w[NLEN-1]=((r.w[NLEN-1-m]<<n))
	if NLEN>=m+2 {r.w[NLEN-1]|=(r.w[NLEN-m-2]>>(BASEBITS-n))}
	for i:=NLEN-2;i>m;i-- {
		r.w[i]=((r.w[i-m]<<n)&BMASK)|(r.w[i-m-1]>>(BASEBITS-n))
	}
	r.w[m]=(r.w[0]<<n)&BMASK; 
	for i:=0;i<m;i++ {r.w[i]=0}
}

/* return number of bits */
func (r *BIG) nbits() int {
	k:=NLEN-1
	r.norm()
	for (k>=0 && r.w[k]==0) {k--}
	if k<0 {return 0}
	bts:=int(BASEBITS)*k;
	c:=r.w[k];
	for c!=0 {c/=2; bts++}
	return bts
}

/* Convert to Hex String */
func (r *BIG) toString() string {
	s:=""
	len:=r.nbits()

	if len%4==0 {
		len/=4
	} else {
		len/=4 
		len++

	}
	MB:=int(MODBYTES*2)
	if len<MB {len=MB}

	for i:=len-1;i>=0;i-- {
		b:=NewBIGcopy(r)
		
		b.shr(uint(i*4))
		s+=strconv.FormatInt(int64(b.w[0]&15),16)
	}
	return s
}

func (r *BIG) add(x *BIG) {
	for i:=0;i<NLEN;i++ {
		r.w[i]=r.w[i]+x.w[i] 
	}
}

/* return this+x */
func (r *BIG) Plus(x *BIG) *BIG {
	s:=new(BIG)
	for i:=0;i<NLEN;i++ {
		s.w[i]=r.w[i]+x.w[i];
	}
	return s;
}

/* this+=x, where x is int */
func (r *BIG) inc(x int) {
	r.norm();
	r.w[0]+=Chunk(x);
}

/* this*=c and catch overflow in DBIG */
func (r *BIG) pxmul(c int) *DBIG {
	m:=NewDBIG()	
	carry:=Chunk(0)
	for j:=0;j<NLEN;j++ {
		carry,m.w[j]=muladd(r.w[j],Chunk(c),carry,m.w[j])
	}
	m.w[NLEN]=carry;		
	return m;
}

/* return this-x */
func (r *BIG) Minus(x *BIG) *BIG {
	d:=new(BIG)
	for i:=0;i<NLEN;i++ {
		d.w[i]=r.w[i]-x.w[i] 
	}
	return d;
}

/* this-=x */
func (r *BIG) sub(x *BIG) {
	for i:=0;i<NLEN;i++ {
		r.w[i]=r.w[i]-x.w[i] 
	}
} 

/* reverse subtract this=x-this */ 
func (r *BIG) rsub(x *BIG) {
	for i:=0;i<NLEN;i++ {
		r.w[i]=x.w[i]-r.w[i] 
	}
} 

/* this-=x, where x is int */
func (r *BIG) dec(x int) {
	r.norm();
	r.w[0]-=Chunk(x)
} 

/* this*=x, where x is small int<NEXCESS */
func (r *BIG) imul(c int) {
	for i:=0;i<NLEN;i++{ 
		r.w[i]*=Chunk(c)
	}
}

/* this*=x, where x is >NEXCESS */
func (r *BIG) pmul(c int) Chunk {
	carry:=Chunk(0)
//	r.norm();
	for i:=0;i<NLEN;i++ {
		ak:=r.w[i]
		r.w[i]=0
		carry,r.w[i]=muladd(ak,Chunk(c),carry,r.w[i])
	}
	return carry
}

/* convert this BIG to byte array */
func (r *BIG) tobytearray(b []byte,n int) {
	r.norm();
	c:=NewBIGcopy(r)

	for i:=int(MODBYTES)-1;i>=0;i-- {
		b[i+n]=byte(c.w[0])
		c.fshr(8)
	}
}

/* convert from byte array to BIG */
func frombytearray(b []byte,n int) *BIG {
	m:=NewBIG();
	for i:=0;i<int(MODBYTES);i++ {
		m.fshl(8); m.w[0]+=Chunk(int(b[i+n]&0xff))
	}
	return m
}

func (r *BIG) ToBytes(b []byte) {
	r.tobytearray(b,0)
}

func FromBytes(b []byte) *BIG {
	return frombytearray(b,0)
}

/* divide by 3 */
func (r *BIG) div3() int {	
	carry:=Chunk(0)
	r.norm();
	base:=(Chunk(1)<<BASEBITS)
	for i:=NLEN-1;i>=0;i-- {
		ak:=(carry*base+r.w[i])
		r.w[i]=ak/3;
		carry=ak%3;
	}
	return int(carry)
}

/* return a*b where result fits in a BIG */
func smul(a *BIG,b *BIG) *BIG {
	carry:=Chunk(0)
	c:=NewBIG()
	for i:=0;i<NLEN;i++ {
		carry=0;
		for j:=0;j<NLEN;j++ {
			if i+j<NLEN {
				carry,c.w[i+j]=muladd(a.w[i],b.w[j],carry,c.w[i+j])
				//carry=c.muladd(a.w[i],b.w[j],carry,i+j)
			}
		}
	}
	return c;
}



/* Compare a and b, return 0 if a==b, -1 if a<b, +1 if a>b. Inputs must be normalised */
func comp(a *BIG,b *BIG) int {
	for i:=NLEN-1;i>=0;i-- {
		if a.w[i]==b.w[i] {continue}
		if a.w[i]>b.w[i] {
			return 1
		} else  {return -1}
	}
	return 0
}

/* return parity */
func (r *BIG) parity() int {
	return int(r.w[0]%2)
}

/* return n-th bit */
func (r *BIG) bit(n int) int {
	if (r.w[n/int(BASEBITS)]&(Chunk(1)<<(uint(n)%BASEBITS)))>0 {return 1}
	return 0;
}

/* return n last bits */
func (r *BIG) lastbits(n int) int {
	msk:=(1<<uint(n))-1;
	r.norm();
	return (int(r.w[0]))&msk
}


/* set x = x mod 2^m */
func (r *BIG) mod2m(m uint) {
	wd:=int(m/BASEBITS)
	bt:=m%BASEBITS
	msk:=(Chunk(1)<<bt)-1
	r.w[wd]&=msk
	for i:=wd+1;i<NLEN;i++ {r.w[i]=0}
}



/* a=1/a mod 2^256. This is very fast! */
func (r *BIG) invmod2m() {
	U:=NewBIG()
	b:=NewBIG()
	c:=NewBIG()

	U.inc(invmod256(r.lastbits(8)))

	for i:=8;i<BIGBITS;i<<=1 {
		U.norm()
		ui:=uint(i)
		b.copy(r); b.mod2m(ui)
		t1:=smul(U,b); t1.shr(ui)
		c.copy(r); c.shr(ui); c.mod2m(ui)

		t2:=smul(U,c); t2.mod2m(ui)
		t1.add(t2)
		t1.norm()
		b=smul(t1,U); t1.copy(b)
		t1.mod2m(ui)

		t2.one(); t2.shl(ui); t1.rsub(t2); t1.norm()
		t1.shl(ui)
		U.add(t1)
	}
	U.mod2m(8*MODBYTES)
	r.copy(U)
	r.norm()
}

/* reduce this mod m */
func (r *BIG) Mod(m *BIG) {
	sr:=NewBIG()
	r.norm()
	if comp(r,m)<0 {return}

	m.fshl(1); k:=1

	for comp(r,m)>=0 {
		m.fshl(1)
		k++;
	}

	for k>0 {
		m.fshr(1);

			sr.copy(r)
			sr.sub(m)
			sr.norm()
			r.cmove(sr,int(1-((sr.w[NLEN-1]>>uint(CHUNK-1))&1)));
/*
		if comp(r,m)>=0 {
			r.sub(m)
			r.norm()
		} */
		k--;
	}
}

/* divide this by m */
func (r *BIG) div(m *BIG) {
	var d int
	k:=0
	r.norm();
	sr:=NewBIG();
	e:=NewBIGint(1)
	b:=NewBIGcopy(r)
	r.zero();

	for (comp(b,m)>=0) {
		e.fshl(1)
		m.fshl(1)
		k++
	}

	for k>0 {
		m.fshr(1)
		e.fshr(1)

		sr.copy(b);
		sr.sub(m);
		sr.norm();
		d=int(1-((sr.w[NLEN-1]>>uint(CHUNK-1))&1));
		b.cmove(sr,d);
		sr.copy(r);
		sr.add(e);
		sr.norm();
		r.cmove(sr,d);
/*
		if comp(b,m)>=0 {
			r.add(e)
			r.norm()
			b.sub(m)
			b.norm()
		} */
		k--
	}
}

/* get 8*MODBYTES size random number */
func random(rng *amcl.RAND) *BIG {
	m:=NewBIG()
	var j int=0
	var r byte=0
/* generate random BIG */ 
	for i:=0;i<8*int(MODBYTES);i++   {
		if j==0 {
			r=rng.GetByte()
		} else {r>>=1}

		b:=Chunk(int(r&1))
		m.shl(1); m.w[0]+=b// m.inc(b)
		j++; j&=7; 
	}
	return m;
}

/* Create random BIG in portable way, one bit at a time */
func Randomnum(q *BIG,rng *amcl.RAND) *BIG {
	d:=NewDBIG();
	var j int=0
	var r byte=0
	for i:=0;i<2*q.nbits();i++ {
		if (j==0) {
			r=rng.GetByte();
		} else {r>>=1}

		b:=Chunk(int(r&1))
		d.shl(1); d.w[0]+=b// m.inc(b);
		j++; j&=7 
	}
	m:=d.mod(q)
	return m;
}


/* return NAF value as +/- 1, 3 or 5. x and x3 should be normed. 
nbs is number of bits processed, and nzs is number of trailing 0s detected */
/*
func nafbits(x *BIG,x3 *BIG ,i int) [3]int {
	var n [3]int
	var j int
	nb:=x3.bit(i)-x.bit(i)


	n[1]=1
	n[0]=0
	if nb==0 {n[0]=0; return n}
	if i==0 {n[0]=nb; return n}
	if nb>0 {
		n[0]=1;
	} else  {n[0]=(-1)}

	for j=i-1;j>0;j-- {
		n[1]++
		n[0]*=2
		nb=x3.bit(j)-x.bit(j)
		if nb>0 {n[0]+=1}
		if nb<0 {n[0]-=1}
		if (n[0]>5 || n[0] < -5) {break}
	}

	if n[0]%2!=0 && j!=0 { // backtrack 
		if nb>0 {n[0]=(n[0]-1)/2}
		if nb<0 {n[0]=(n[0]+1)/2}
		n[1]--
	}
	for n[0]%2==0 { // remove trailing zeros 
		n[0]/=2
		n[2]++
		n[1]--
	}
	return n;
}
*/

/* return a*b mod m */
func Modmul(a,b,m *BIG) *BIG {
	a.Mod(m)
	b.Mod(m)
	d:=mul(a,b);
	return d.mod(m)
}

/* return a^2 mod m */
func Modsqr(a,m *BIG) *BIG {
	a.Mod(m)
	d:=sqr(a)
	return d.mod(m)
}

/* return -a mod m */
func Modneg(a,m *BIG) *BIG {
	a.Mod(m)
	return m.Minus(a)
}

/* Jacobi Symbol (this/p). Returns 0, 1 or -1 */
func (r *BIG) Jacobi(p *BIG) int {
	m:=0;
	t:=NewBIGint(0)
	x:=NewBIGint(0)
	n:=NewBIGint(0)
	zilch:=NewBIGint(0)
	one:=NewBIGint(1)
	if (p.parity()==0 || comp(r,zilch)==0 || comp(p,one)<=0) {return 0}
	r.norm()
	x.copy(r)
	n.copy(p)
	x.Mod(p)

	for comp(n,one)>0 {
		if comp(x,zilch)==0 {return 0}
		n8:=n.lastbits(3)
		k:=0
		for x.parity()==0 {
			k++
			x.shr(1)
		}
		if k%2==1 {m+=(n8*n8-1)/8}
		m+=(n8-1)*(x.lastbits(2)-1)/4
		t.copy(n)
		t.Mod(x)
		n.copy(x)
		x.copy(t)
		m%=2

	}
	if m==0 {return 1}
	return -1
}

/* this=1/this mod p. Binary method */
func (r *BIG) Invmodp(p *BIG) {
	r.Mod(p)
	u:=NewBIGcopy(r)

	v:=NewBIGcopy(p)
	x1:=NewBIGint(1)
	x2:=NewBIGint(0)
	t:=NewBIGint(0)
	one:=NewBIGint(1)
	for (comp(u,one)!=0 && comp(v,one)!=0) {
		for u.parity()==0 {
			u.shr(1);
			if x1.parity()!=0 {
				x1.add(p)
				x1.norm()
			}
			x1.shr(1)
		}
		for v.parity()==0 {
			v.shr(1);
			if x2.parity()!=0 {
				x2.add(p)
				x2.norm()
			}
			x2.shr(1)
		}
		if comp(u,v)>=0 {
			u.sub(v)
			u.norm()
			if comp(x1,x2)>=0 {
				x1.sub(x2)
			} else {
				t.copy(p)
				t.sub(x2)
				x1.add(t)
			}
			x1.norm()
		} else {
			v.sub(u)
			v.norm()
			if comp(x2,x1)>=0 { 
				x2.sub(x1)
			} else {
				t.copy(p)
				t.sub(x1)
				x2.add(t)
			}
			x2.norm()
		}
	}
	if comp(u,one)==0 {
		r.copy(x1)
	} else {r.copy(x2)}
}

/* return this^e mod m */
func (r *BIG) powmod(e *BIG,m *BIG) *BIG {
	r.norm()
	e.norm()
	a:=NewBIGint(1)
	z:=NewBIGcopy(e)
	s:=NewBIGcopy(r)
	for true {
		bt:=z.parity()
		z.fshr(1)
		if bt==1 {a=Modmul(a,s,m)}
		if z.iszilch() {break}
		s=Modsqr(s,m)
	}
	return a;
}

/* Arazi and Qi inversion mod 256 */
func invmod256(a int) int {
	var t1 int=0
	c:=(a>>1)&1
	t1+=c
	t1&=1
	t1=2-t1
	t1<<=1
	U:=t1+1;

// i=2
	b:=a&3;
	t1=U*b; t1>>=2
	c=(a>>2)&3
	t2:=(U*c)&3
	t1+=t2;
	t1*=U; t1&=3
	t1=4-t1
	t1<<=2
	U+=t1

// i=4
	b=a&15
	t1=U*b; t1>>=4
	c=(a>>4)&15
	t2=(U*c)&15
	t1+=t2
	t1*=U; t1&=15
	t1=16-t1
	t1<<=4
	U+=t1

	return U;
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
	return (r)
}


/*
func main() {
	a := NewBIGint(3)
	m := NewBIGints(Modulus)

	fmt.Printf("Modulus= "+m.toString())
	fmt.Printf("\n")


	e := NewBIGcopy(m);
	e.dec(1); e.norm();
	fmt.Printf("Exponent= "+e.toString())
	fmt.Printf("\n")
	a=a.powmod(e,m);
	fmt.Printf("Result= "+a.toString())
}
*/
