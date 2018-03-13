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

/*
 * Implementation of the Secure Hashing Algorithm (SHA-384)
 *
 * Generates a 384 bit message digest. It should be impossible to come
 * come up with two messages that hash to the same value ("collision free").
 *
 * For use with byte-oriented messages only. 
 */


package amcl



const SHA512 int=64

const hash512_H0 uint64=0x6a09e667f3bcc908
const hash512_H1 uint64=0xbb67ae8584caa73b
const hash512_H2 uint64=0x3c6ef372fe94f82b
const hash512_H3 uint64=0xa54ff53a5f1d36f1
const hash512_H4 uint64=0x510e527fade682d1
const hash512_H5 uint64=0x9b05688c2b3e6c1f
const hash512_H6 uint64=0x1f83d9abfb41bd6b
const hash512_H7 uint64=0x5be0cd19137e2179

var hash512_K = [...]uint64 {
	0x428a2f98d728ae22,0x7137449123ef65cd,0xb5c0fbcfec4d3b2f,0xe9b5dba58189dbbc,
	0x3956c25bf348b538,0x59f111f1b605d019,0x923f82a4af194f9b,0xab1c5ed5da6d8118,
	0xd807aa98a3030242,0x12835b0145706fbe,0x243185be4ee4b28c,0x550c7dc3d5ffb4e2,
	0x72be5d74f27b896f,0x80deb1fe3b1696b1,0x9bdc06a725c71235,0xc19bf174cf692694,
	0xe49b69c19ef14ad2,0xefbe4786384f25e3,0x0fc19dc68b8cd5b5,0x240ca1cc77ac9c65,
	0x2de92c6f592b0275,0x4a7484aa6ea6e483,0x5cb0a9dcbd41fbd4,0x76f988da831153b5,
	0x983e5152ee66dfab,0xa831c66d2db43210,0xb00327c898fb213f,0xbf597fc7beef0ee4,
	0xc6e00bf33da88fc2,0xd5a79147930aa725,0x06ca6351e003826f,0x142929670a0e6e70,
	0x27b70a8546d22ffc,0x2e1b21385c26c926,0x4d2c6dfc5ac42aed,0x53380d139d95b3df,
	0x650a73548baf63de,0x766a0abb3c77b2a8,0x81c2c92e47edaee6,0x92722c851482353b,
	0xa2bfe8a14cf10364,0xa81a664bbc423001,0xc24b8b70d0f89791,0xc76c51a30654be30,
	0xd192e819d6ef5218,0xd69906245565a910,0xf40e35855771202a,0x106aa07032bbd1b8,
	0x19a4c116b8d2d0c8,0x1e376c085141ab53,0x2748774cdf8eeb99,0x34b0bcb5e19b48a8,
	0x391c0cb3c5c95a63,0x4ed8aa4ae3418acb,0x5b9cca4f7763e373,0x682e6ff3d6b2b8a3,
	0x748f82ee5defb2fc,0x78a5636f43172f60,0x84c87814a1f0ab72,0x8cc702081a6439ec,
	0x90befffa23631e28,0xa4506cebde82bde9,0xbef9a3f7b2c67915,0xc67178f2e372532b,
	0xca273eceea26619c,0xd186b8c721c0c207,0xeada7dd6cde0eb1e,0xf57d4f7fee6ed178,
	0x06f067aa72176fba,0x0a637dc5a2c898a6,0x113f9804bef90dae,0x1b710b35131c471b,
	0x28db77f523047d84,0x32caab7b40c72493,0x3c9ebe0a15c9bebc,0x431d67c49c100d4c,
	0x4cc5d4becb3e42b6,0x597f299cfc657e2a,0x5fcb6fab3ad6faec,0x6c44198c4a475817}


type HASH512 struct {
	length [2]uint64
	h [8]uint64
	w [80]uint64

}

/* functions */
func hash512_S(n uint64,x uint64) uint64 {
	return (((x)>>n) | ((x)<<(64-n)))
}

func hash512_R(n uint64,x uint64) uint64 {
	return ((x)>>n)
}

func hash512_Ch(x,y,z uint64) uint64 {
	return ((x&y)^(^(x)&z))
}

func hash512_Maj(x,y,z uint64) uint64 {
	return ((x&y)^(x&z)^(y&z))
}

func hash512_Sig0(x uint64) uint64 {
	return (hash512_S(28,x)^hash512_S(34,x)^hash512_S(39,x))
}

func hash512_Sig1(x uint64) uint64 {
	return (hash512_S(14,x)^hash512_S(18,x)^hash512_S(41,x))
}

func hash512_theta0(x uint64) uint64 {
	return (hash512_S(1,x)^hash512_S(8,x)^hash512_R(7,x));
}

func hash512_theta1(x uint64) uint64 {
		return (hash512_S(19,x)^hash512_S(61,x)^hash512_R(6,x))
}

func (H *HASH512) transform() { /* basic transformation step */
	for j:=16;j<80;j++ {
		H.w[j]=hash512_theta1(H.w[j-2])+H.w[j-7]+hash512_theta0(H.w[j-15])+H.w[j-16]
	}
	a:=H.h[0]; b:=H.h[1]; c:=H.h[2]; d:=H.h[3] 
	e:=H.h[4]; f:=H.h[5]; g:=H.h[6]; hh:=H.h[7]
	for j:=0;j<80;j++ { /* 80 times - mush it up */
		t1:=hh+hash512_Sig1(e)+hash512_Ch(e,f,g)+hash512_K[j]+H.w[j]
		t2:=hash512_Sig0(a)+hash512_Maj(a,b,c)
		hh=g; g=f; f=e
		e=d+t1
		d=c
		c=b
		b=a
		a=t1+t2  
	}
	H.h[0]+=a; H.h[1]+=b; H.h[2]+=c; H.h[3]+=d 
	H.h[4]+=e; H.h[5]+=f; H.h[6]+=g; H.h[7]+=hh 
} 

/* Initialise Hash function */
func (H *HASH512) Init() { /* initialise */
	for i:=0;i<80;i++ {H.w[i]=0}
	H.length[0]=0; H.length[1]=0
	H.h[0]=hash512_H0
	H.h[1]=hash512_H1
	H.h[2]=hash512_H2
	H.h[3]=hash512_H3
	H.h[4]=hash512_H4
	H.h[5]=hash512_H5
	H.h[6]=hash512_H6
	H.h[7]=hash512_H7
}

func NewHASH512() *HASH512 {
	H:= new(HASH512)
	H.Init()
	return H
}

/* process a single byte */
func (H *HASH512) Process(byt byte) { /* process the next message byte */
	cnt:=(H.length[0]/64)%16;
    
	H.w[cnt]<<=8;
	H.w[cnt]|=uint64(byt&0xFF);
	H.length[0]+=8;
	if H.length[0]==0 {H.length[1]++; H.length[0]=0}
	if (H.length[0]%1024)==0 {H.transform()}
}

/* process an array of bytes */	
func (H *HASH512) Process_array(b []byte) {
	for i:=0;i<len(b);i++ {H.Process((b[i]))}
}

/* process a 32-bit integer */
func (H *HASH512) Process_num(n int32) {
	H.Process(byte((n>>24)&0xff));
	H.Process(byte((n>>16)&0xff));
	H.Process(byte((n>>8)&0xff));
	H.Process(byte(n&0xff));
}

/* Generate 64-byte Hash */
func (H *HASH512) Hash() []byte { /* pad message and finish - supply digest */
	var digest [64]byte
	len0:=H.length[0]
	len1:=H.length[1]
	H.Process(0x80);
	for (H.length[0]%1024)!=896 {H.Process(0)}
	H.w[14]=len1;
	H.w[15]=len0;    
	H.transform();
	for i:=0;i<64;i++ { /* convert to bytes */
		digest[i]=byte((H.h[i/8]>>uint(8*(7-i%8))) & 0xff);
	}
	H.Init()
	return digest[0:64]
}

/* test program: should produce digest */

//8e959b75dae313da 8cf4f72814fc143f 8f7779c6eb9f7fa1 7299aeadb6889018 501d289e4900f7e4 331b99dec4b5433a c7d329eeb6dd2654 5e96e55b874be909
/*
func main() {

	test := []byte("abcdefghbcdefghicdefghijdefghijkefghijklfghijklmghijklmnhijklmnoijklmnopjklmnopqklmnopqrlmnopqrsmnopqrstnopqrstu")
	sh:=NewHASH512()

	for i:=0;i<len(test);i++ {
		sh.Process(test[i])
	}
		
	digest:=sh.Hash()    
	for i:=0;i<64;i++ {fmt.Printf("%02x",digest[i])}

}
*/
