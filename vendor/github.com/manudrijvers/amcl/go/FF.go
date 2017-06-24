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

package amcl

//import "fmt"
//import "os"

//var debug bool = false

type FF struct {
	length int
	v []*BIG
}

/* Constructors */
func NewFFint(n int) *FF {
	F:=new(FF)
	for i:=0;i<n;i++ {
		F.v=append(F.v,NewBIG())
	}
	F.length=n
	return F
}
/*
func NewFFints(x [][NLEN]int64,n int) *FF {
	F:=new(FF)
	for i:=0;i<n;i++ {
		F.v=append(F.v,NewBIGints(x[i]))
	}
	F.length=n
	return F
}
*/
/* set to zero */
func (F *FF) zero() {
	for i:=0;i<F.length;i++ {
		F.v[i].zero()
	}
}

func (F *FF) getlen() int {
		return F.length
	}

/* set to integer */
func (F *FF) set(m int) {
	F.zero()
	F.v[0].set(0,Chunk(m))
}

/* copy from FF b */
func (F *FF) copy(b *FF) {
	for i:=0;i<F.length;i++ {
		F.v[i].copy(b.v[i])
	}
}

/* x=y<<n */
func (F *FF) dsucopy(b *FF) {
	for i:=0;i<b.length;i++ {
		F.v[b.length+i].copy(b.v[i])
		F.v[i].zero()
	}
}

/* x=y */
func (F *FF) dscopy(b *FF) {
	for i:=0;i<b.length;i++ {
		F.v[i].copy(b.v[i])
		F.v[b.length+i].zero()
	}
}

/* x=y>>n */
func (F *FF) sducopy(b *FF) {
	for i:=0;i<F.length;i++ {
		F.v[i].copy(b.v[F.length+i])
	}
}

func (F *FF) one() {
	F.v[0].one();
	for i:=1;i<F.length;i++ {
		F.v[i].zero()
	}
}

/* test equals 0 */
func (F *FF) iszilch() bool {
	for i:=0;i<F.length;i++ {
		if !F.v[i].iszilch() {return false}
	}
	return true
}

/* shift right by BIGBITS-bit words */
func (F *FF) shrw(n int) {
	for i:=0;i<n;i++ {
		F.v[i].copy(F.v[i+n])
		F.v[i+n].zero()
	}
}

/* shift left by BIGBITS-bit words */
func (F *FF) shlw(n int) {
	for i:=0;i<n;i++ {
		F.v[n+i].copy(F.v[i])
		F.v[i].zero()
	}
}

/* extract last bit */
func (F *FF) parity() int {
	return F.v[0].parity()
}

func (F *FF) lastbits(m int) int {
	return F.v[0].lastbits(m)
}

/* compare x and y - must be normalised, and of same length */
func ff_comp(a *FF,b *FF) int {
	for i:=a.length-1;i>=0;i-- {
		j:=comp(a.v[i],b.v[i])
		if j!=0 {return j}
	}
	return 0
}

/* recursive add */
func (F *FF) radd(vp int,x *FF,xp int,y *FF,yp int,n int) {
	for i:=0;i<n;i++ {
		F.v[vp+i].copy(x.v[xp+i])
		F.v[vp+i].add(y.v[yp+i])
	}
}

/* recursive inc */
func (F *FF) rinc(vp int,y *FF,yp int,n int) {
	for i:=0;i<n;i++ {
		F.v[vp+i].add(y.v[yp+i])
	}
}

/* recursive sub */
func (F *FF) rsub(vp int,x *FF,xp int,y *FF,yp int,n int) {
	for i:=0;i<n;i++ {
		F.v[vp+i].copy(x.v[xp+i])
		F.v[vp+i].sub(y.v[yp+i])
	}
}

/* recursive dec */
func (F *FF) rdec(vp int,y *FF,yp int,n int) {
	for i:=0;i<n;i++ {
		F.v[vp+i].sub(y.v[yp+i])
	}
}

/* simple add */
func (F *FF) add(b *FF) {
	for i:=0;i<F.length;i++ {
		F.v[i].add(b.v[i])
	}
}

/* simple sub */
func (F *FF) sub(b *FF) {
	for i:=0;i<F.length;i++ {
		F.v[i].sub(b.v[i])
	}
}
	
/* reverse sub */
func (F *FF) revsub(b *FF) {
	for i:=0;i<F.length;i++ {
		F.v[i].rsub(b.v[i])
	}
}

/* normalise - but hold any overflow in top part unless n<0 */
func (F *FF) rnorm(vp int,n int) {
	trunc:=false
	var carry Chunk
	if n<0 { /* -v n signals to do truncation */
		n=-n
		trunc=true
	}
	for i:=0;i<n-1;i++ {
		carry=F.v[vp+i].norm()
		F.v[vp+i].xortop(carry<<P_TBITS)
		F.v[vp+i+1].w[0]+=carry; // inc(carry)
	}
	carry=F.v[vp+n-1].norm()
	if trunc {
		F.v[vp+n-1].xortop(carry<<P_TBITS)
	}
}

func (F *FF) norm() {
	F.rnorm(0,F.length)
}

/* increment/decrement by a small integer */
func (F *FF) inc(m int) {
	F.v[0].inc(m)
	F.norm()
}

func (F *FF) dec(m int) {
	F.v[0].dec(m)
	F.norm()
}

/* shift left by one bit */
func (F *FF) shl() {
	var delay_carry int=0
	for i:=0;i<F.length-1;i++ {
		carry:=F.v[i].fshl(1)
		F.v[i].inc(delay_carry)
		F.v[i].xortop(Chunk(carry)<<P_TBITS)
		delay_carry=int(carry)
	}
	F.v[F.length-1].fshl(1)
	F.v[F.length-1].inc(delay_carry)
}

/* shift right by one bit */

func (F *FF) shr() {
	for i:=F.length-1;i>0;i-- {
		carry:=F.v[i].fshr(1)
		F.v[i-1].xortop(Chunk(carry)<<P_TBITS)
	}
	F.v[0].fshr(1)
}

/* Convert to Hex String */
func (F *FF) toString() string {
	F.norm()
	s:=""
	for i:=F.length-1;i>=0;i-- {
		s+=F.v[i].toString()
	}
	return s
}

/* Convert FFs to/from byte arrays */
func (F *FF) toBytes(b []byte) {
	for i:=0;i<F.length;i++ {
		F.v[i].tobytearray(b,(F.length-i-1)*int(MODBYTES))
	}
}

func ff_fromBytes(x *FF,b []byte) {
	for i:=0;i<x.length;i++ {
		x.v[i]=frombytearray(b,(x.length-i-1)*int(MODBYTES))
	}
}

/* in-place swapping using xor - side channel resistant - lengths must be the same */
func ff_cswap(a *FF,b *FF,d int) {
	for i:=0;i<a.length;i++ {
		a.v[i].cswap(b.v[i],d)
	}
}

/* z=x*y, t is workspace */
func (F *FF) karmul(vp int,x *FF,xp int,y *FF,yp int,t *FF,tp int,n int) {
	if n==1 {
		d:=mul(x.v[xp],y.v[yp])
		F.v[vp+1]=d.split(8*MODBYTES)
		F.v[vp].dcopy(d)
		return
	}
	nd2:=n/2
	F.radd(vp,x,xp,x,xp+nd2,nd2)
		F.rnorm(vp,nd2)
	F.radd(vp+nd2,y,yp,y,yp+nd2,nd2)
		F.rnorm(vp+nd2,nd2)
	t.karmul(tp,F,vp,F,vp+nd2,t,tp+n,nd2)
	F.karmul(vp,x,xp,y,yp,t,tp+n,nd2)
	F.karmul(vp+n,x,xp+nd2,y,yp+nd2,t,tp+n,nd2)
	t.rdec(tp,F,vp,n)
	t.rdec(tp,F,vp+n,n)
	F.rinc(vp+nd2,t,tp,n)
	F.rnorm(vp,2*n)
}

func (F *FF) karsqr(vp int,x *FF,xp int,t *FF,tp int,n int) {
	if n==1 {
		d:=sqr(x.v[xp])
		F.v[vp+1].copy(d.split(8*MODBYTES))
		F.v[vp].dcopy(d)
		return
	}	

	nd2:=n/2
	F.karsqr(vp,x,xp,t,tp+n,nd2)
	F.karsqr(vp+n,x,xp+nd2,t,tp+n,nd2)
	t.karmul(tp,x,xp,x,xp+nd2,t,tp+n,nd2)
	F.rinc(vp+nd2,t,tp,n)
	F.rinc(vp+nd2,t,tp,n)
	F.rnorm(vp+nd2,n)
}

/* Calculates Least Significant bottom half of x*y */
func (F *FF) karmul_lower(vp int,x *FF,xp int,y *FF,yp int,t *FF,tp int,n int) { 
	if n==1 { /* only calculate bottom half of product */
		F.v[vp].copy(smul(x.v[xp],y.v[yp]))
		return
	}
	nd2:=n/2

	F.karmul(vp,x,xp,y,yp,t,tp+n,nd2)
	t.karmul_lower(tp,x,xp+nd2,y,yp,t,tp+n,nd2)
	F.rinc(vp+nd2,t,tp,nd2)
	t.karmul_lower(tp,x,xp,y,yp+nd2,t,tp+n,nd2)
	F.rinc(vp+nd2,t,tp,nd2)
	F.rnorm(vp+nd2,-nd2)  /* truncate it */
}

/* Calculates Most Significant upper half of x*y, given lower part */
func (F *FF) karmul_upper(x *FF,y *FF,t *FF,n int) { 
	nd2:=n/2
	F.radd(n,x,0,x,nd2,nd2)
	F.radd(n+nd2,y,0,y,nd2,nd2)
	F.rnorm(n,nd2)
	F.rnorm(n+nd2,nd2)

	t.karmul(0,F,n+nd2,F,n,t,n,nd2)  /* t = (a0+a1)(b0+b1) */
	F.karmul(n,x,nd2,y,nd2,t,n,nd2) /* z[n]= a1*b1 */

					/* z[0-nd2]=l(a0b0) z[nd2-n]= h(a0b0)+l(t)-l(a0b0)-l(a1b1) */
	t.rdec(0,F,n,n)              /* t=t-a1b1  */	
						
	F.rinc(nd2,F,0,nd2)  /* z[nd2-n]+=l(a0b0) = h(a0b0)+l(t)-l(a1b1)  */
	F.rdec(nd2,t,0,nd2)   /* z[nd2-n]=h(a0b0)+l(t)-l(a1b1)-l(t-a1b1)=h(a0b0) */

	F.rnorm(0,-n)		/* a0b0 now in z - truncate it */

	t.rdec(0,F,0,n)         /* (a0+a1)(b0+b1) - a0b0 */
	F.rinc(nd2,t,0,n)

	F.rnorm(nd2,n)
}

/* z=x*y. Assumes x and y are of same length. */
func ff_mul(x *FF,y *FF) *FF {
	n:=x.length
	z:=NewFFint(2*n)
	t:=NewFFint(2*n)
	z.karmul(0,x,0,y,0,t,0,n)
	return z
}

/* return low part of product this*y */
func (F *FF) lmul(y *FF) {
	n:=F.length
	t:=NewFFint(2*n)
	x:=NewFFint(n); x.copy(F)
	F.karmul_lower(0,x,0,y,0,t,0,n)
}

/* Set b=b mod c */
func (F *FF) mod(c *FF) {
	var k int=1  

	F.norm()
	if ff_comp(F,c)<0 {return}

	c.shl()
	for ff_comp(F,c)>=0 {
		c.shl()
		k++
	}

	for k>0 {
		c.shr()
		if ff_comp(F,c)>=0 {
			F.sub(c)
			F.norm()
		}
		k--
	}
}

/* z=x^2 */
func ff_sqr(x *FF) *FF {
	n:=x.length
	z:=NewFFint(2*n)
	t:=NewFFint(2*n)
	z.karsqr(0,x,0,t,0,n)
	return z
}

/* return This mod modulus, N is modulus, ND is Montgomery Constant */
func (F *FF) reduce(N *FF,ND *FF) *FF { /* fast karatsuba Montgomery reduction */
	n:=N.length
	t:=NewFFint(2*n)
	r:=NewFFint(n)
	m:=NewFFint(n)

	r.sducopy(F)
	m.karmul_lower(0,F,0,ND,0,t,0,n)

	F.karmul_upper(N,m,t,n)
	
	m.sducopy(F)
	r.add(N)
	r.sub(m)
	r.norm()

	return r

}

/* Set r=this mod b */
/* this is of length - 2*n */
/* r,b is of length - n */
func (F *FF) dmod(b *FF) *FF {
	n:=b.length
	m:=NewFFint(2*n)
	x:=NewFFint(2*n)
	r:=NewFFint(n)

	x.copy(F)
	x.norm()
	m.dsucopy(b); k:=BIGBITS*n

	for k>0 {	
		m.shr()

		if ff_comp(x,m)>=0 {
			x.sub(m)
			x.norm()
		}
		k--
	}

	r.copy(x)
	r.mod(b)
	return r
}

/* Set return=1/this mod p. Binary method - a<p on entry */

func (F *FF) invmodp(p *FF) {
	n:=p.length

	u:=NewFFint(n)
	v:=NewFFint(n)
	x1:=NewFFint(n)
	x2:=NewFFint(n)
	t:=NewFFint(n)
	one:=NewFFint(n)

	one.one()
	u.copy(F)
	v.copy(p)
	x1.copy(one)
	x2.zero()

	// reduce n in here as well! 
	for (ff_comp(u,one)!=0 && ff_comp(v,one)!=0) {
		for u.parity()==0 {
			u.shr()
			if x1.parity()!=0 {
				x1.add(p)
				x1.norm()
			}
			x1.shr()
		}
		for v.parity()==0 {
			v.shr() 
			if x2.parity()!=0 {
				x2.add(p)
				x2.norm()
			}
			x2.shr()
		}
		if ff_comp(u,v)>=0 {
			u.sub(v)
			u.norm()
			if ff_comp(x1,x2)>=0 {
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
			if ff_comp(x2,x1)>=0 { 
				x2.sub(x1)
			} else {
				t.copy(p)
				t.sub(x1)
				x2.add(t)
			}
			x2.norm()
		}
	}
	if ff_comp(u,one)==0 {
		F.copy(x1)
	} else {
		F.copy(x2)
	}
}

/* nresidue mod m */
func (F *FF) nres(m *FF) {
	n:=m.length
	d:=NewFFint(2*n)
	d.dsucopy(F)
	F.copy(d.dmod(m))
}

func (F *FF) redc(m *FF,ND *FF) {
	n:=m.length
	d:=NewFFint(2*n)
	F.mod(m)
	d.dscopy(F)
	F.copy(d.reduce(m,ND))
	F.mod(m)
}

func (F *FF) mod2m(m int) {
	for i:=m;i<F.length;i++ {
		F.v[i].zero()
	}
}

/* U=1/a mod 2^m - Arazi & Qi */
func (F *FF) invmod2m() *FF {
	n:=F.length

	b:=NewFFint(n)
	c:=NewFFint(n)
	U:=NewFFint(n)

	U.zero()
	U.v[0].copy(F.v[0])
	U.v[0].invmod2m()

	for i:=1;i<n;i<<=1 {
		b.copy(F); b.mod2m(i)
		t:=ff_mul(U,b); t.shrw(i); b.copy(t)
		c.copy(F); c.shrw(i); c.mod2m(i)
		c.lmul(U); c.mod2m(i)

		b.add(c); b.norm()
		b.lmul(U); b.mod2m(i)

		c.one(); c.shlw(i); b.revsub(c); b.norm()
		b.shlw(i)
		U.add(b)
	}
	U.norm()
	return U
}

func (F *FF) random(rng *RAND) {
	n:=F.length
	for i:=0;i<n;i++ {
		F.v[i].copy(random(rng))
	}
	/* make sure top bit is 1 */
	for (F.v[n-1].nbits()<int(MODBYTES*8)) {
		F.v[n-1].copy(random(rng))
	}
}

/* generate random x less than p */
func (F *FF) randomnum(p *FF,rng *RAND) {
	n:=F.length
	d:=NewFFint(2*n)

	for i:=0;i<2*n;i++ {
		d.v[i].copy(random(rng))
	}
	F.copy(d.dmod(p))
}

/* this*=y mod p */
func (F *FF) modmul(y *FF,p *FF,nd *FF) {
	if ff_pexceed(F.v[F.length-1],y.v[y.length-1]) {F.mod(p)}
	d:=ff_mul(F,y)
	F.copy(d.reduce(p,nd))
}

/* this*=y mod p */
func (F *FF) modsqr(p *FF,nd *FF) {
	if ff_sexceed(F.v[F.length-1]) {F.mod(p)}
	d:=ff_sqr(F)
	F.copy(d.reduce(p,nd))
}

/* this=this^e mod p using side-channel resistant Montgomery Ladder, for large e */
func (F *FF) skpow(e *FF,p *FF) {
	n:=p.length
	R0:=NewFFint(n)
	R1:=NewFFint(n)
	ND:=p.invmod2m()

	F.mod(p)
	R0.one()
	R1.copy(F)
	R0.nres(p)
	R1.nres(p)

	for i:=int(8*MODBYTES)*n-1;i>=0;i-- {
		b:=int(e.v[i/BIGBITS].bit(i%BIGBITS))
		F.copy(R0)
		F.modmul(R1,p,ND)

		ff_cswap(R0,R1,b)
		R0.modsqr(p,ND)

		R1.copy(F)
		ff_cswap(R0,R1,b)
	}
	F.copy(R0)
	F.redc(p,ND)
}

/* this =this^e mod p using side-channel resistant Montgomery Ladder, for short e */
func (F *FF) skpows(e *BIG,p *FF) {
	n:=p.length
	R0:=NewFFint(n)
	R1:=NewFFint(n)
	ND:=p.invmod2m()

	F.mod(p)
	R0.one()
	R1.copy(F)
	R0.nres(p)
	R1.nres(p)

	for i:=int(8*MODBYTES)-1;i>=0;i-- {
		b:=int(e.bit(i))
		F.copy(R0)
		F.modmul(R1,p,ND)

		ff_cswap(R0,R1,b)
		R0.modsqr(p,ND)

		R1.copy(F)
		ff_cswap(R0,R1,b)
	}
	F.copy(R0)
	F.redc(p,ND)
}

/* raise to an integer power - right-to-left method */
func (F *FF) power(e int,p *FF) {
	n:=p.length
	w:=NewFFint(n)
	ND:=p.invmod2m()
	f:=true

	w.copy(F)
	w.nres(p)
//i:=0;
	if e==2 {
		F.copy(w)
		F.modsqr(p,ND)
	} else {
		for (true) {
			if e%2==1 {
				if f {
					F.copy(w)
				} else {F.modmul(w,p,ND)}
				f=false

			}
			e>>=1
			if e==0 {break}
//fmt.Printf("wb= "+w.toString()+"\n");
//debug=true;
			w.modsqr(p,ND)
//debug=false;
//fmt.Printf("wa= "+w.toString()+"\n");
//i+=1;
//os.Exit(0);
		}
	}

	F.redc(p,ND)

}

/* this=this^e mod p, faster but not side channel resistant */
func (F *FF) pow(e *FF,p *FF) {
	n:=p.length
	w:=NewFFint(n)
	ND:=p.invmod2m()
//fmt.Printf("ND= "+ND.toString() +"\n");
	w.copy(F)
	F.one()
	F.nres(p)
	w.nres(p)
	for i:=int(8*MODBYTES)*n-1;i>=0;i-- {
		F.modsqr(p,ND)
		b:=e.v[i/BIGBITS].bit(i%BIGBITS)
		if b==1 {F.modmul(w,p,ND)}
	}
	F.redc(p,ND)
}

/* double exponentiation r=x^e.y^f mod p */
func (F *FF) pow2(e *BIG,y *FF,f *BIG,p *FF) {
	n:=p.length
	xn:=NewFFint(n)
	yn:=NewFFint(n)
	xy:=NewFFint(n)
	ND:=p.invmod2m()

	xn.copy(F)
	yn.copy(y)
	xn.nres(p)
	yn.nres(p)
	xy.copy(xn); xy.modmul(yn,p,ND)
	F.one()
	F.nres(p)

	for i:=int(8*MODBYTES)-1;i>=0;i-- {
		eb:=e.bit(i)
		fb:=f.bit(i)
		F.modsqr(p,ND)
		if eb==1 {
			if fb==1 {
				F.modmul(xy,p,ND)
			} else {F.modmul(xn,p,ND)}
		} else	{
			if fb==1 {F.modmul(yn,p,ND)}
		}
	}
	F.redc(p,ND)
}

func igcd(x int,y int) int { /* integer GCD, returns GCD of x and y */
	var r int
	if y==0 {return x}
	for true {
		r=x%y
		if r==0 {break}
		x=y;y=r
	}
	return y
}

/* quick and dirty check for common factor with n */
func (F *FF) cfactor(s int) bool {
	n:=F.length

	x:=NewFFint(n)
	y:=NewFFint(n)

	y.set(s)
	x.copy(F)
	x.norm()

	x.sub(y)
	x.norm()

	for (!x.iszilch() && x.parity()==0) {x.shr()}

	for (ff_comp(x,y)>0) {
		x.sub(y)
		x.norm()
		for (!x.iszilch() && x.parity()==0) {x.shr()}
	}

	g:=int(x.v[0].get(0))
	r:=igcd(s,g)
	if r>1 {return true}
	return false
}

/* Miller-Rabin test for primality. Slow. */
func prime(p *FF,rng *RAND) bool {
	s:=0
	n:=p.length
	d:=NewFFint(n)
	x:=NewFFint(n)
	unity:=NewFFint(n)
	nm1:=NewFFint(n)

	sf:=4849845 /* 3*5*.. *19 */
	p.norm()

	if p.cfactor(sf) {return false}
	unity.one()
	nm1.copy(p)
	nm1.sub(unity)
	nm1.norm()
	d.copy(nm1)

	for d.parity()==0 {
		d.shr()
		s++
	}
	if s==0 {return false}

	for i:=0;i<10;i++ {
		x.randomnum(p,rng)
		x.pow(d,p)

		if (ff_comp(x,unity)==0 || ff_comp(x,nm1)==0) {continue}
		loop:=false
		for j:=1;j<s;j++ {
			x.power(2,p)
			if ff_comp(x,unity)==0 {return false}
			if ff_comp(x,nm1)==0 {loop=true; break}
		}
		if loop {continue}
		return false
	}

	return true
}
/*
func main() {

	var P = [4][5]int64 {{0xAD19A781670957,0x76A79C00965796,0xDEFCC5FC9A9717,0xF02F2940E20E9,0xBF59E34F},{0x6894F31844C908,0x8DADA70E82C79F,0xFD29F3836046F6,0x8C1D874D314DD0,0x46D077B},{0x3C515217813331,0x56680FD1CE935B,0xE55C53EEA8838E,0x92C2F7E14A4A95,0xD945E5B1},{0xACF673E919F5EF,0x6723E7E7DAB446,0x6B6FA69B36EB1B,0xF7D13920ECA300,0xB5FC2165}}

	fmt.Printf("Testing FF\n")
	var raw [100]byte
	rng:=NewRAND()

	rng.Clean()
	for i:=0;i<100;i++ {
		raw[i]=byte(i)
	}

	rng.Seed(100,raw[:])

	n:=4

	x:=NewFFint(n)
	x.set(3)

	p:=NewFFints(P[:],n)

	if prime(p,rng) {fmt.Printf("p is a prime\n"); fmt.Printf("\n")}

	e:=NewFFint(n)
	e.copy(p)
	e.dec(1); e.norm()

	fmt.Printf("e= "+e.toString())
	fmt.Printf("\n")
	x.skpow(e,p)
	fmt.Printf("x= "+x.toString())
	fmt.Printf("\n")
}
*/