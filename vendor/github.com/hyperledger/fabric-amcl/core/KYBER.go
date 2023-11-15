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

/* Kyber API */

package core



const KY_LGN uint = 8
const KY_DEGREE int = (1 << KY_LGN);
const KY_PRIME int32 = 0xD01

const KY_ONE int32 = 0x549		// R mod Q
const KY_QINV int32 = 62209   // q^(-1) mod 2^16

const KYBER_SECRET_CPA_SIZE_512 int = (2*(KY_DEGREE*3)/2)
const KYBER_PUBLIC_SIZE_512 int = (32+2*(KY_DEGREE*3)/2)
const KYBER_CIPHERTEXT_SIZE_512 int = ((10*2+4)*KY_DEGREE/8)
const KYBER_SECRET_CCA_SIZE_512 int = (KYBER_SECRET_CPA_SIZE_512+KYBER_PUBLIC_SIZE_512+64)
const KYBER_SHARED_SECRET_512 int = 32

const KYBER_SECRET_CPA_SIZE_768 int = (3*(KY_DEGREE*3)/2)
const KYBER_PUBLIC_SIZE_768 int = (32+3*(KY_DEGREE*3)/2)
const KYBER_CIPHERTEXT_SIZE_768 int = ((10*3+4)*KY_DEGREE/8)
const KYBER_SECRET_CCA_SIZE_768 int = (KYBER_SECRET_CPA_SIZE_768+KYBER_PUBLIC_SIZE_768+64)
const KYBER_SHARED_SECRET_768 int = 32

const KYBER_SECRET_CPA_SIZE_1024 int = (4*(KY_DEGREE*3)/2)
const KYBER_PUBLIC_SIZE_1024 int = (32+4*(KY_DEGREE*3)/2)
const KYBER_CIPHERTEXT_SIZE_1024 int = ((11*4+5)*KY_DEGREE/8)
const KYBER_SECRET_CCA_SIZE_1024 int = (KYBER_SECRET_CPA_SIZE_1024+KYBER_PUBLIC_SIZE_1024+64)
const KYBER_SHARED_SECRET_1024 int = 32

const KY_MAXK = 4;

// parameters for each security level
// K,eta1,eta2,du,dv,shared secret
var PARAMS_512 = [6]int{2,3,2,10,4,32}
var PARAMS_768 = [6]int{3,2,2,10,4,32}
var PARAMS_1024 = [6]int{4,2,2,11,5,32}

/* Translated from public domain reference implementation code - taken from https://github.com/pq-crystals/kyber */
var ZETAS = [256]int16{
  -1044,  -758,  -359, -1517,  1493,  1422,   287,   202,
   -171,   622,  1577,   182,   962, -1202, -1474,  1468,
    573, -1325,   264,   383,  -829,  1458, -1602,  -130,
   -681,  1017,   732,   608, -1542,   411,  -205, -1571,
   1223,   652,  -552,  1015, -1293,  1491,  -282, -1544,
    516,    -8,  -320,  -666, -1618, -1162,   126,  1469,
   -853,   -90,  -271,   830,   107, -1421,  -247,  -951,
   -398,   961, -1508,  -725,   448, -1065,   677, -1275,
  -1103,   430,   555,   843, -1251,   871,  1550,   105,
    422,   587,   177,  -235,  -291,  -460,  1574,  1653,
   -246,   778,  1159,  -147,  -777,  1483,  -602,  1119,
  -1590,   644,  -872,   349,   418,   329,  -156,   -75,
    817,  1097,   603,   610,  1322, -1285, -1465,   384,
  -1215,  -136,  1218, -1335,  -874,   220, -1187, -1659,
  -1185, -1530, -1278,   794, -1510,  -854,  -870,   478,
   -108,  -308,   996,   991,   958, -1460,  1522,  1628}

func montgomery_reduce(a int32) int16 {
	t := int16(a*KY_QINV)
	t = int16((a - int32(t)*KY_PRIME) >> 16)
	return t
}

func barrett_reduce(a int16) int16 {
	v := int16(((int32(1)<<26) + KY_PRIME/2)/KY_PRIME)
	vv := int32(v)
	aa := int32(a)
	t := int16((vv*aa + 0x2000000) >> 26);
	t *= int16(KY_PRIME)
	return int16(a - t);
}

func fqmul(a int16, b int16) int16 {
	return montgomery_reduce(int32(a)*int32(b));
}

func ntt(r []int16) {
	var j int
	k := 1
	for len := 128; len >= 2; len >>= 1 {
		for start := 0; start < 256; start = j + len {
			zeta := ZETAS[k]; k+=1
			for j = start; j < start + len; j++ {
				t := fqmul(zeta, r[j + len])
				r[j + len] = r[j] - t
				r[j] = r[j] + t
			}
		}
	}
}

func invntt(r []int16) {
	var j int
	f := int16(1441) // mont^2/128
	k := 127
	for len := 2; len <= 128; len <<= 1 {
		for start := 0; start < 256; start = j + len {
			zeta := ZETAS[k]; k-=1
			for j = start; j < start + len; j++ {
				t := r[j]
				r[j] = barrett_reduce(t + r[j + len])
				r[j + len] = (r[j + len] - t)
				r[j + len] = fqmul(zeta, r[j + len])
			}
		}
	}

	for j := 0; j < 256; j++ {
		r[j] = fqmul(r[j], f)
	}
}

func basemul(index int,r []int16, a []int16, b []int16,zeta int16) {
	i:=index
	j:=index+1
	r[i]  = fqmul(a[j], b[j])
	r[i]  = fqmul(r[i], zeta)
	r[i] += fqmul(a[i], b[i])
	r[j]  = fqmul(a[i], b[j])
	r[j] += fqmul(a[j], b[i])
}

func poly_reduce(r []int16) {
	for i:=0;i<KY_DEGREE;i++ {
		r[i] = barrett_reduce(r[i])
	}
}

func poly_ntt(r []int16) {
	ntt(r) 
	poly_reduce(r)
}

func poly_invntt(r []int16) {
	invntt(r)
}

// Note r must be distinct from a and b
func poly_mul(r []int16, a []int16, b []int16) {
	for i := 0; i < KY_DEGREE/4; i++ {
		basemul(4*i,r,a,b,ZETAS[64+i])
		basemul(4*i+2,r,a,b,-ZETAS[64+i])
	}
}

func poly_tomont(r []int16) {
	f := int32(KY_ONE);
	for i:=0;i<KY_DEGREE;i++ {
		r[i] = montgomery_reduce(int32(r[i])*f)
	}
}

/* End of public domain reference code use */

// copy polynomial
func poly_copy(p1 []int16, p2 []int16) {
	for i := 0; i < KY_DEGREE; i++ {
		p1[i] = p2[i]
	}
}	

// zero polynomial
func poly_zero(p1 []int16) {
	for i := 0; i < KY_DEGREE; i++ {
		p1[i] = 0
	}
}

// add polynomials
func poly_add(p1 []int16, p2 []int16, p3 []int16) {
	for i := 0; i < KY_DEGREE; i++ {
		p1[i] = (p2[i] + p3[i])
	}
}

// subtract polynomials
func poly_sub(p1 []int16, p2 []int16, p3 []int16) {
	for i := 0; i < KY_DEGREE; i++ {
		p1[i] = (p2[i] - p3[i])
	}
}

// Generate A[i][j] from rho
func expandAij(rho []byte,Aij []int16,i int,j int) {
	sh := NewSHA3(SHA3_SHAKE128)
	var buff [640]byte   // should be plenty (?)
	for m:=0;m<32;m++ {
		sh.Process(rho[m])
	}

	sh.Process(byte(j&0xff))
	sh.Process(byte(i&0xff))
	sh.Shake(buff[:],640)
	i = 0 
	j = 0
	for j<KY_DEGREE {
		d1 := int16(buff[i])+256*int16(buff[i+1]&0x0F);
		d2 := int16(buff[i+1])/16+16*int16(buff[i+2]);

		if (d1<int16(KY_PRIME)) {
			Aij[j]=d1; j+=1
		}
		if (d2<int16(KY_PRIME) && j<KY_DEGREE) {
			Aij[j]=d2; j+=1
		}
		i+=3
	}
}

// get n-th bit from byte array
func getbit(b []byte,n int) int {
	wd:=n/8;
	bt:=n%8;
	return int((b[wd]>>bt)&1)
}

// centered binomial distribution
func cbd(bts []byte,eta int,f []int16) {
	for i:=0;i<KY_DEGREE;i++ {
		a:=0; b:=0
		for j:=0;j<eta;j++ {
			a+=getbit(bts,2*i*eta+j)
			b+=getbit(bts,2*i*eta+eta+j)
		}
		f[i]=int16(a-b) 
	}
}

// extract ab bits into word from dense byte stream
func nextword(ab int,t []byte,position []int) int16 {
	ptr:=position[0] // index in array
	bts:=position[1] // bit index in byte
	r:=int16(t[ptr]>>bts)
	mask:=int16((1<<ab)-1)
	i:=0
	gotbits:=8-bts // bits left in current byte
	for gotbits<ab {
		i++
		w:=int16(t[ptr+i])
		r|=w<<gotbits
		gotbits+=8
	}
	bts+=ab
	for bts>=8{
		bts-=8
		ptr++
	}
	w:=int16(r&mask)
	position[0]=ptr
	position[1]=bts 
	return w  
}

// array t has ab active bits per word
// extract bytes from array of words
// if max!=0 then -max<=t[i]<=+max
func nextbyte16(ab int,t []int16,position []int) byte {
	ptr:=position[0] // index in array
	bts:=position[1] // bit index in byte

	left:=ab-bts // number of bits left in this word
	i:=0
	k:=ptr%256

	w:=t[k]; w+=(w>>15)&int16(KY_PRIME)
	r:=int16(w>>bts);
	for left<8 {
		i++
		w=t[k+i]; w+=(w>>15)&int16(KY_PRIME)
		r|=w<<left
		left+=ab
	}

	bts+=8
	for bts>=ab {
		bts-=ab;
		ptr++;
	}
	position[0]=ptr
	position[1]=bts
	return byte(r&0xff);        
}

// encode polynomial vector of length len with coefficients of length L, into packed bytes
func encode(t []int16,pos []int,L int,pack []byte,pptr int) {
	k:=(KY_DEGREE*L)/8  // compressed length
	for n:=0;n<k;n++ {
		pack[n+pptr*k]=nextbyte16(L,t,pos)
	}
}

func chk_encode(t []int16,pos []int,L int,pack []byte,pptr int) byte {
	k:=(KY_DEGREE*L)/8
	diff:=byte(0)
	for n:=0;n<k;n++  {
		m:=nextbyte16(L,t,pos)
		diff|=(m^pack[n+pptr*k])
	}   
	return diff;
}

// decode packed bytes into polynomial vector, with coefficients of length L
// pos indicates current position in byte array pack
func decode(pack []byte,L int,t []int16,pos []int) {
	for i:=0;i<KY_DEGREE;i++ {
		t[i]=nextword(L,pack,pos)
	}
}

// compress polynomial coefficents in place, for polynomial vector of length len
func compress(t []int16,d int) {
	twod:=int32(1<<d)
	for i:=0;i<KY_DEGREE;i++ {
		t[i]+=(t[i]>>15)&int16(KY_PRIME)
		t[i]= int16(((twod*int32(t[i])+KY_PRIME/2)/KY_PRIME)&(twod-1))
	}
}

// decompress polynomial coefficents in place, for polynomial vector of length len
func decompress(t []int16,d int) {
	twod1:=int32(1<<(d-1))
	for i:=0;i<KY_DEGREE;i++ {
		t[i]=int16((KY_PRIME*int32(t[i])+twod1)>>d)
	}
}

// input entropy, output key pair
func cpa_keypair(params [6]int,tau []byte,sk []byte,pk []byte) {
	sh := NewSHA3(SHA3_HASH512)
	var rho [32]byte
	var sigma [33]byte 
	var buff [256]byte 

	ck:=params[0]
	var r [KY_DEGREE]int16
	var w [KY_DEGREE]int16
	var Aij [KY_DEGREE]int16

	var s= make([][KY_DEGREE]int16, ck)
	var e= make([][KY_DEGREE]int16, ck)
	var p= make([][KY_DEGREE]int16, ck)

	eta1:=params[1]
	public_key_size:=32+ck*(KY_DEGREE*3)/2
//	secret_cpa_key_size:=ck*(KY_DEGREE*3)/2
   
	for i:=0;i<32;i++ {
		sh.Process(tau[i])
	}
	bf := sh.Hash();
	for i:=0;i<32;i++ {
		rho[i]=bf[i]
		sigma[i]=bf[i+32]
	}
	sigma[32]=0   // N

// create s
	for i:=0;i<ck;i++ {
		sh= NewSHA3(SHA3_SHAKE256)
		for j:=0;j<33;j++{
			sh.Process(sigma[j])
		}
		sh.Shake(buff[:],64*eta1);
		cbd(buff[:],eta1,s[i][:])
		sigma[32]+=1
	}

// create e
	for i:=0;i<ck;i++ {
		sh= NewSHA3(SHA3_SHAKE256)
		for j:=0;j<33;j++ {
			sh.Process(sigma[j])
		}
		sh.Shake(buff[:],64*eta1)
		cbd(buff[:],eta1,e[i][:])
		sigma[32]+=1
	}

	for k:=0;k<ck;k++ {
		poly_ntt(s[k][:])
		poly_ntt(e[k][:])
	}

	for i:=0;i<ck;i++ {
		expandAij(rho[:],Aij[:],i,0)
		poly_mul(r[:],Aij[:],s[0][:])

		for j:=1;j<ck;j++ {
			expandAij(rho[:],Aij[:],i,j)
			poly_mul(w[:],s[j][:],Aij[:])
			poly_add(r[:],r[:],w[:])
		}
		poly_reduce(r[:])
		poly_tomont(r[:])
		poly_add(p[i][:],r[:],e[i][:])
		poly_reduce(p[i][:])
	}

	var pos [2]int
	pos[0]=0; pos[1]=0
	for i:=0;i<ck;i++ {
		encode(s[i][:],pos[:],12,sk,i)
	}
	pos[0]=0; pos[1]=0
	for i:=0;i<ck;i++ {
		encode(p[i][:],pos[:],12,pk,i)
	}
	for i:=0;i<32;i++ {
		pk[public_key_size-32+i]=rho[i]
	}
}

// input 64 random bytes, output secret and public keys
func cca_keypair(params [6]int,randbytes64 []byte,sk []byte,pk []byte) {
	sh:= NewSHA3(SHA3_HASH256)
	sks:=(params[0]*(KY_DEGREE*3)/2)
	pks:=(32+params[0]*(KY_DEGREE*3)/2)

	cpa_keypair(params,randbytes64[0:32],sk,pk)
	for i:=0;i<pks;i++ {
		sk[sks+i]=pk[i]
	}
	for i:=0;i<pks;i++ {
		sh.Process(pk[i])
	}
	h:=sh.Hash();
	for i:=0;i<32;i++ {
		sk[sks+pks+i]=h[i]
	}
	for i:=0;i<32;i++ {
		sk[sks+pks+32+i]=randbytes64[32+i]
	}
}

func cpa_base_encrypt(params [6]int,coins []byte,pk []byte,ss []byte,u [][256]int16, v []int16) {
	var rho [32]byte
	var sigma [33]byte 
	var buff [256]byte 

	ck:=params[0]
	var r [KY_DEGREE]int16
	var w [KY_DEGREE]int16
	var Aij [KY_DEGREE]int16

	var q= make([][KY_DEGREE]int16, ck)
	var p= make([][KY_DEGREE]int16, ck)

	eta1:=params[1]
	eta2:=params[2]
	du:=params[3]
	dv:=params[4]
	public_key_size:=32+ck*(KY_DEGREE*3)/2

	for i:=0;i<32;i++ {
		sigma[i]=coins[i] //i+6 //RAND_byte(RNG);
	}
	sigma[32]=0

	for i:=0;i<32;i++ {
		rho[i]=pk[public_key_size-32+i]
	}

// create q
	for i:=0;i<ck;i++ {
		sh := NewSHA3(SHA3_SHAKE256)
		for j:=0;j<33;j++ {
			sh.Process(sigma[j])
		}
		sh.Shake(buff[:],64*eta1)
		cbd(buff[:],eta1,q[i][:])
		sigma[32]+=1
	}
// create e1
	for i:=0;i<ck;i++ {
		sh := NewSHA3(SHA3_SHAKE256)
		for j:=0;j<33;j++ {
			sh.Process(sigma[j])
		}
		sh.Shake(buff[:],64*eta2);
		cbd(buff[:],eta1,u[i][:])          // e1
		sigma[32]+=1
	}
	for i:=0;i<ck;i++ {
		poly_ntt(q[i][:])
	}
	
	for i:=0;i<ck;i++ {
		expandAij(rho[:],Aij[:],0,i)
		poly_mul(r[:],Aij[:],q[0][:])
		for j:=1;j<ck;j++ {
			expandAij(rho[:],Aij[:],j,i)
			poly_mul(w[:],q[j][:],Aij[:])
			poly_add(r[:],r[:],w[:])
		}
		poly_reduce(r[:]);
		poly_invntt(r[:]);
		poly_add(u[i][:],u[i][:],r[:]);
		poly_reduce(u[i][:]);
	}

	var pos [2]int
	pos[0]=0; pos[1]=0
	for i:=0;i<ck;i++ {
		decode(pk,12,p[i][:],pos[:])
	}
	
	poly_mul(v[:],p[0][:],q[0][:])

	for i:=1;i<ck;i++ {
		poly_mul(r[:],p[i][:],q[i][:])
		poly_add(v[:],v[:],r[:])
	}    
	poly_invntt(v[:])        

// create e2
	sh := NewSHA3(SHA3_SHAKE256)
	for j:=0;j<33;j++ {
		sh.Process(sigma[j])
	}
	sh.Shake(buff[:],64*eta2)
	cbd(buff[:],eta1,w[:])  // e2

	poly_add(v[:],v[:],w[:])
	pos[0]=0; pos[1]=0
	decode(ss,1,r[:],pos[:])
	decompress(r[:],1)

	poly_add(v[:],v[:],r[:])
	poly_reduce(v[:])

	for i:=0;i<ck;i++ {
		compress(u[i][:],du)
	}
	compress(v[:],dv)		
}

// Given input of entropy, public key and shared secret is an input, outputs ciphertext
func cpa_encrypt(params [6]int,coins []byte,pk []byte,ss []byte,ct []byte) {
	ck:=params[0]
	var v [KY_DEGREE]int16
	var u= make([][KY_DEGREE]int16, ck)

	du:=params[3]
	dv:=params[4]
	ciphertext_size:=(du*ck+dv)*KY_DEGREE/8

	cpa_base_encrypt(params,coins,pk,ss,u,v[:])
	var pos [2]int
	pos[0]=0; pos[1]=0
	for i:=0;i<ck;i++ {
		encode(u[i][:],pos[:],du,ct,i)
	}
	encode(v[:],pos[:],dv,ct[ciphertext_size-(dv*KY_DEGREE/8):ciphertext_size],0)
}

// Re-encrypt and check that ct is OK (if so return is zero)
func cpa_check_encrypt(params [6]int,coins []byte,pk []byte,ss []byte,ct []byte) byte {
	ck:=params[0]
	var v [KY_DEGREE]int16
	var u= make([][KY_DEGREE]int16, ck)

	du:=params[3]
	dv:=params[4]
	ciphertext_size:=(du*ck+dv)*KY_DEGREE/8

	d1:=byte(0)
		
	cpa_base_encrypt(params,coins,pk,ss,u,v[:]);
	var pos [2]int
	pos[0]=0; pos[1]=0

	for i:=0;i<ck;i++  {
		d1|=chk_encode(u[i][:],pos[:],du,ct,i)
	}

	d2:=chk_encode(v[:],pos[:],dv,ct[ciphertext_size-(dv*KY_DEGREE/8):ciphertext_size],0);
	if (d1|d2)==0 {
		return 0
	} else {
		return byte(0xff)
	}
}

func cca_encrypt(params [6]int,randbytes32 []byte,pk []byte,ss []byte,ct []byte) {
	var coins [32]byte

	ck:=params[0]
	du:=params[3]
	dv:=params[4]
	shared_secret_size:=params[5]
	public_key_size:=32+ck*(KY_DEGREE*3)/2
	ciphertext_size:=(du*ck+dv)*KY_DEGREE/8

	sh := NewSHA3(SHA3_HASH256)
	for i:=0;i<32;i++{
		sh.Process(randbytes32[i])
	}
	hm := sh.Hash();

	sh = NewSHA3(SHA3_HASH256)
	for i:=0;i<public_key_size;i++ {
		sh.Process(pk[i])
	}
	h := sh.Hash()

	sh = NewSHA3(SHA3_HASH512);
	for i:=0;i<32;i++ {
		sh.Process(hm[i])
	}
	for i:=0;i<32;i++ {
		sh.Process(h[i])
	}
	g:= sh.Hash()

	for i:=0;i<32;i++ {
		coins[i]=g[i+32]
	}

	cpa_encrypt(params,coins[:],pk,hm,ct)

	sh = NewSHA3(SHA3_HASH256)
	for i:=0;i<ciphertext_size;i++ {
		sh.Process(ct[i])
	}
	h= sh.Hash();

	sh = NewSHA3(SHA3_SHAKE256)
	for i:=0;i<32;i++ {
		sh.Process(g[i])
	}
	for i:=0;i<32;i++ {
		sh.Process(h[i])
	}

	sh.Shake(ss[:],shared_secret_size)
}

func cpa_decrypt(params [6]int,SK []byte,CT []byte,SS []byte) {
	ck:=params[0]
	var w [KY_DEGREE]int16
	var v [KY_DEGREE]int16
	var r [KY_DEGREE]int16

	var u= make([][KY_DEGREE]int16, ck)
	var s= make([][KY_DEGREE]int16, ck)

	du:=params[3]
	dv:=params[4]
	//shared_secret_size:=params[5] 

	var pos [2]int
	pos[0]=0; pos[1]=0

	for i:=0;i<ck;i++ {
		decode(CT,du,u[i][:],pos[:])
	}
	decode(CT,dv,v[:],pos[:]);
	for i:=0;i<ck;i++ {
		decompress(u[i][:],du)
	}
	decompress(v[:],dv)
	pos[0]=0; pos[1]=0
	for i:=0;i<ck;i++ {
		decode(SK,12,s[i][:],pos[:])
	}

	poly_ntt(u[0][:]);
	poly_mul(w[:],u[0][:],s[0][:]);
	for i:=1;i<ck;i++ {
		poly_ntt(u[i][:])
		poly_mul(r[:],u[i][:],s[i][:])
		poly_add(w[:],w[:],r[:])
	}
	poly_reduce(w[:]);
	poly_invntt(w[:]);
	poly_sub(v[:],v[:],w[:]);
	compress(v[:],1);
	pos[0]=0; pos[1]=0;
	encode(v[:],pos[:],1,SS,0); 
}

func cca_decrypt(params [6]int,SK []byte,CT []byte,SS []byte) {
	ck:=params[0]
	du:=params[3]
	dv:=params[4]
	secret_cpa_key_size:=ck*(KY_DEGREE*3)/2
	public_key_size:=32+ck*(KY_DEGREE*3)/2
	shared_secret_size:=params[5]
	ciphertext_size:=(du*ck+dv)*KY_DEGREE/8

	var h [32]byte
	var z [32]byte
	var m [32]byte
	var coins [32]byte

	PK:=SK[secret_cpa_key_size:secret_cpa_key_size+public_key_size]

	for i:=0;i<32;i++ {
		h[i]=SK[secret_cpa_key_size+public_key_size+i]
	}
	for i:=0;i<32;i++ {
		z[i]=SK[secret_cpa_key_size+public_key_size+32+i]
	}
	cpa_decrypt(params,SK,CT,m[:])

	sh := NewSHA3(SHA3_HASH512)
	for i:=0;i<32;i++ {
		sh.Process(m[i])
	}
	for i:=0;i<32;i++ {
		sh.Process(h[i])
	}
	g := sh.Hash()

	for i:=0;i<32;i++ {
		coins[i]=g[i+32]
	}

	mask:=cpa_check_encrypt(params,coins[:],PK,m[:],CT)
	for i:=0;i<32;i++ {
		g[i]^=(g[i]^z[i])&mask              // substitute z for Kb on failure
	}

	sh = NewSHA3(SHA3_HASH256)
	for i:=0;i<ciphertext_size;i++ {
		sh.Process(CT[i])
	}
	hh:=sh.Hash()

	sh = NewSHA3(SHA3_SHAKE256);
	for i:=0;i<32;i++ {
		sh.Process(g[i])
	}
	for i:=0;i<32;i++ {
		sh.Process(hh[i])
	}
	sh.Shake(SS,shared_secret_size)
}

func KYBER_keypair512(r64 []byte,SK []byte,PK []byte) {
	cca_keypair(PARAMS_512,r64,SK,PK)
}

func KYBER_encrypt512(r32 []byte,PK []byte,SS []byte,CT []byte) {
	cca_encrypt(PARAMS_512,r32,PK,SS,CT)
}

func KYBER_decrypt512(SK []byte,CT []byte,SS []byte) {
	cca_decrypt(PARAMS_512,SK,CT,SS)
}


func KYBER_keypair768(r64 []byte,SK []byte,PK []byte) {
	cca_keypair(PARAMS_768,r64,SK,PK)
}

func KYBER_encrypt768(r32 []byte,PK []byte,SS []byte,CT []byte) {
	cca_encrypt(PARAMS_768,r32,PK,SS,CT)
}

func KYBER_decrypt768(SK []byte,CT []byte,SS []byte) {
	cca_decrypt(PARAMS_768,SK,CT,SS)
}


func KYBER_keypair1024(r64 []byte,SK []byte,PK []byte) {
	cca_keypair(PARAMS_1024,r64,SK,PK)
}

func KYBER_encrypt1024(r32 []byte,PK []byte,SS []byte,CT []byte) {
	cca_encrypt(PARAMS_1024,r32,PK,SS,CT)
}

func KYBER_decrypt1024(SK []byte,CT []byte,SS []byte) {
	cca_decrypt(PARAMS_1024,SK,CT,SS)
}
