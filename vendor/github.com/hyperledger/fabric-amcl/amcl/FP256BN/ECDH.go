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

/* Elliptic Curve API high-level functions  */

package FP256BN


import "github.com/hyperledger/fabric-amcl/amcl"

const INVALID_PUBLIC_KEY int=-2
const ERROR int=-3
const INVALID int=-4
const EFS int=int(MODBYTES)
const EGS int=int(MODBYTES)
//const EAS int=16
//const EBS int=16


//const ECDH_HASH_TYPE int=amcl.SHA512

/* Convert Integer to n-byte array */
func inttoBytes(n int,len int) []byte {
	var b []byte
	var i int
	for i=0;i<len;i++ {b=append(b,0)}
	i=len
	for (n>0 && i>0) {
		i--;
		b[i]=byte(n&0xff)
		n/=256
	}	
	return b
}

func ehashit(sha int,A []byte,n int,B []byte,pad int) []byte {
	var R []byte
	if sha==amcl.SHA256 {
		H:=amcl.NewHASH256()
		H.Process_array(A)
		if n>0 {H.Process_num(int32(n))}
		if B!=nil {H.Process_array(B)}
		R=H.Hash()
	}
	if sha==amcl.SHA384 {
		H:=amcl.NewHASH384()
		H.Process_array(A)
		if n>0 {H.Process_num(int32(n))}
		if B!=nil {H.Process_array(B)}
		R=H.Hash()
	}
	if sha==amcl.SHA512 {
		H:=amcl.NewHASH512()
		H.Process_array(A)
		if n>0 {H.Process_num(int32(n))}
		if B!=nil {H.Process_array(B)}
		R=H.Hash()
	}
	if R==nil {return nil}

	if pad==0 {return R}
	var W []byte
	for i:=0;i<pad;i++ {W=append(W,0)}
	if pad<=sha {
		for i:=0;i<pad;i++ {W[i]=R[i]}
	} else {
		for i:=0;i<sha;i++ {W[i+pad-sha]=R[i]}
		for i:=0;i<pad-sha;i++ {W[i]=0}
 

		//for i:=0;i<sha;i++ {W[i]=R[i]}
		//for i:=sha;i<pad;i++ {W[i]=0}
	}
	return W
}

/* Key Derivation Functions */
/* Input octet Z */
/* Output key of length olen */
func ECDH_KDF1(sha int,Z []byte,olen int) []byte {
/* NOTE: the parameter olen is the length of the output K in bytes */
	hlen:=sha
	var K []byte
	k:=0
    
	for i:=0;i<olen;i++ {K=append(K,0)}

	cthreshold:=olen/hlen; if olen%hlen!=0 {cthreshold++}

	for counter:=0;counter<cthreshold;counter++ {
		B:=ehashit(sha,Z,counter,nil,0)
		if k+hlen>olen {
			for i:=0;i<olen%hlen;i++ {K[k]=B[i]; k++}
		} else {
			for i:=0;i<hlen;i++ {K[k]=B[i]; k++}
		}
	}
	return K;
}

func ECDH_KDF2(sha int,Z []byte,P []byte,olen int) []byte {
/* NOTE: the parameter olen is the length of the output k in bytes */
	hlen:=sha
	var K []byte
	k:=0
    
	for i:=0;i<olen;i++ {K=append(K,0)}

	cthreshold:=olen/hlen; if olen%hlen!=0 {cthreshold++}

	for counter:=1;counter<=cthreshold;counter++ {
		B:=ehashit(sha,Z,counter,P,0)
		if k+hlen>olen {
			for i:=0;i<olen%hlen;i++ {K[k]=B[i]; k++}
		} else {
			for i:=0;i<hlen;i++ {K[k]=B[i]; k++}
		}
	}
	return K
}

/* Password based Key Derivation Function */
/* Input password p, salt s, and repeat count */
/* Output key of length olen */
func ECDH_PBKDF2(sha int,Pass []byte,Salt []byte,rep int,olen int) []byte {
	d:=olen/sha; if olen%sha!=0 {d++}

	var F []byte
	var U []byte
	var S []byte
	var K []byte
	
	for i:=0;i<sha;i++{F=append(F,0); U=append(U,0)}

	for i:=1;i<=d;i++ {
		for j:=0;j<len(Salt);j++ {S=append(S,Salt[j])} 
		N:=inttoBytes(i,4)
		for j:=0;j<4;j++ {S=append(S,N[j])}   

		HMAC(sha,S,Pass,F[:])

		for j:=0;j<sha;j++ {U[j]=F[j]}
		for j:=2;j<=rep;j++ {
			HMAC(sha,U[:],Pass,U[:]);
			for k:=0;k<sha;k++ {F[k]^=U[k]}
		}
		for j:=0;j<sha;j++ {K=append(K,F[j])} 
	}
	var key []byte
	for i:=0;i<olen;i++ {key=append(key,K[i])}
	return key
}

/* Calculate HMAC of m using key k. HMAC is tag of length olen (which is length of tag) */
func HMAC(sha int,M []byte,K []byte,tag []byte) int {
	/* Input is from an octet m        *
	* olen is requested output length in bytes. k is the key  *
	* The output is the calculated tag */
	var B []byte
	b:=64
	if sha>32 {b=128}

	var K0 [128]byte
	olen:=len(tag)

	if (olen<4 /*|| olen>sha */) {return 0}

	for i:=0;i<b;i++ {K0[i]=0}

	if len(K) > b {
		B=ehashit(sha,K,0,nil,0) 
		for i:=0;i<sha;i++ {K0[i]=B[i]}
	} else {
		for i:=0;i<len(K);i++  {K0[i]=K[i]}
	}
		
	for i:=0;i<b;i++ {K0[i]^=0x36}
	B=ehashit(sha,K0[0:b],0,M,0);

	for i:=0;i<b;i++ {K0[i]^=0x6a}
	B=ehashit(sha,K0[0:b],0,B,olen)

	for i:=0;i<olen;i++ {tag[i]=B[i]}

	return 1
}

/* AES encryption/decryption. Encrypt byte array M using key K and returns ciphertext */
func AES_CBC_IV0_ENCRYPT(K []byte,M []byte) []byte { /* AES CBC encryption, with Null IV and key K */
	/* Input is from an octet string M, output is to an octet string C */
	/* Input is padded as necessary to make up a full final block */
	a:=amcl.NewAES()
	fin:=false

	var buff [16]byte
	var C []byte

	a.Init(amcl.AES_CBC,len(K),K,nil)

	ipt:=0; //opt:=0
	var i int
	for true {
		for i=0;i<16;i++ {
			if ipt<len(M) {
				buff[i]=M[ipt]; ipt++;
			} else {fin=true; break;}
		}
		if fin {break}
		a.Encrypt(buff[:])
		for i=0;i<16;i++ {
			C=append(C,buff[i])
		}
	}    

/* last block, filled up to i-th index */

	padlen:=16-i
	for j:=i;j<16;j++ {buff[j]=byte(padlen)}

	a.Encrypt(buff[:])

	for i=0;i<16;i++ {
		C=append(C,buff[i])
	}
	a.End()   
	return C
}

/* returns plaintext if all consistent, else returns null string */
func AES_CBC_IV0_DECRYPT(K []byte,C []byte) []byte { /* padding is removed */
	a:=amcl.NewAES()
	var buff [16]byte
	var MM []byte
	var M []byte

	var i int
	ipt:=0; opt:=0

	a.Init(amcl.AES_CBC,len(K),K,nil);

	if len(C)==0 {return nil}
	ch:=C[ipt]; ipt++
  
	fin:=false

	for true {
		for i=0;i<16;i++ {
			buff[i]=ch    
			if ipt>=len(C) {
				fin=true; break
			}  else {ch=C[ipt]; ipt++ }
		}
		a.Decrypt(buff[:])
		if fin {break}
		for i=0;i<16;i++ {
			MM=append(MM,buff[i]); opt++
		}
	}    

	a.End();
	bad:=false
	padlen:=int(buff[15])
	if (i!=15 || padlen<1 || padlen>16) {bad=true}
	if (padlen>=2 && padlen<=16) {
		for i=16-padlen;i<16;i++ {
			if buff[i]!=byte(padlen) {bad=true}
		}
	}
    
	if !bad { 
		for i=0;i<16-padlen;i++ {
			MM=append(MM,buff[i]); opt++
		}
	}

	if bad {return nil}

	for i=0;i<opt;i++ {M=append(M,MM[i])}

	return M;
}

/* Calculate a public/private EC GF(p) key pair W,S where W=S.G mod EC(p),
 * where S is the secret key and W is the public key
 * and G is fixed generator.
 * If RNG is NULL then the private key is provided externally in S
 * otherwise it is generated randomly internally */
func ECDH_KEY_PAIR_GENERATE(RNG *amcl.RAND,S []byte,W []byte) int {
	res:=0
//	var T [EFS]byte
	var s *BIG
	var G *ECP

	G=ECP_generator();

	r:=NewBIGints(CURVE_Order)

	if RNG==nil {
		s=FromBytes(S)
		s.Mod(r)
	} else {
		s=Randomnum(r,RNG)
		
	//	s.ToBytes(T[:])
	//	for i:=0;i<EGS;i++ {S[i]=T[i]}
	}

	//if AES_S>0 {
	//	s.mod2m(2*AES_S)
	//}
	s.ToBytes(S)

	WP:=G.mul(s)

	WP.ToBytes(W,false)    // To use point compression on public keys, change to true 

	return res
}

/* validate public key */
func ECDH_PUBLIC_KEY_VALIDATE(W []byte) int {
	WP:=ECP_fromBytes(W)
	res:=0

	r:=NewBIGints(CURVE_Order)

	if WP.Is_infinity() {res=INVALID_PUBLIC_KEY}
	if res==0 {

		q:=NewBIGints(Modulus)
		nb:=q.nbits()
		k:=NewBIGint(1); k.shl(uint((nb+4)/2))
		k.add(q)
		k.div(r)

		for (k.parity()==0) {
			k.shr(1)
			WP.dbl()
		}

		if !k.isunity() {
			WP=WP.mul(k)
		}
		if WP.Is_infinity() {res=INVALID_PUBLIC_KEY}

	}
	return res
}

/* IEEE-1363 Diffie-Hellman online calculation Z=S.WD */
func ECDH_ECPSVDP_DH(S []byte,WD []byte,Z []byte) int {
	res:=0;
	var T [EFS]byte

	s:=FromBytes(S)

	W:=ECP_fromBytes(WD)
	if W.Is_infinity() {res=ERROR}

	if res==0 {
		r:=NewBIGints(CURVE_Order)
		s.Mod(r)
		W=W.mul(s)
		if W.Is_infinity() { 
			res=ERROR
		} else {
			W.GetX().ToBytes(T[:])
			for i:=0;i<EFS;i++ {Z[i]=T[i]}
		}
	}
	return res
}

/* IEEE ECDSA Signature, C and D are signature on F using private key S */
func ECDH_ECPSP_DSA(sha int,RNG *amcl.RAND,S []byte,F []byte,C []byte,D []byte) int {
	var T [EFS]byte

	B:=ehashit(sha,F,0,nil,int(MODBYTES));
	G:=ECP_generator();

	r:=NewBIGints(CURVE_Order)

	s:=FromBytes(S)
	f:=FromBytes(B[:])

	c:=NewBIGint(0)
	d:=NewBIGint(0)
	V:=NewECP()

	for d.iszilch() {
		u:=Randomnum(r,RNG);
		w:=Randomnum(r,RNG);  /* side channel masking */
		//if AES_S>0 {
		//	u.mod2m(2*AES_S)
		//}			
		V.Copy(G)
		V=V.mul(u)   		
		vx:=V.GetX()
		c.copy(vx)
		c.Mod(r);
		if c.iszilch() {continue}
		u.copy(Modmul(u,w,r))
		u.Invmodp(r)
		d.copy(Modmul(s,c,r))
		d.add(f)
		d.copy(Modmul(d,w,r))
		d.copy(Modmul(u,d,r))
	} 
       
	c.ToBytes(T[:])
	for i:=0;i<EFS;i++ {C[i]=T[i]}
	d.ToBytes(T[:])
	for i:=0;i<EFS;i++ {D[i]=T[i]}
	return 0
}

/* IEEE1363 ECDSA Signature Verification. Signature C and D on F is verified using public key W */
func ECDH_ECPVP_DSA(sha int,W []byte,F []byte,C []byte,D []byte) int {
	res:=0

	B:=ehashit(sha,F,0,nil,int(MODBYTES));

	G:=ECP_generator(); 
	r:=NewBIGints(CURVE_Order)

	c:=FromBytes(C)
	d:=FromBytes(D)
	f:=FromBytes(B[:])
     
	if (c.iszilch() || comp(c,r)>=0 || d.iszilch() || comp(d,r)>=0) {
            res=INVALID;
	}

	if res==0 {
		d.Invmodp(r)
		f.copy(Modmul(f,d,r))
		h2:=Modmul(c,d,r)

		WP:=ECP_fromBytes(W)
		if WP.Is_infinity() {
			res=ERROR
		} else {
			P:=NewECP()
			P.Copy(WP)

			P=P.Mul2(h2,G,f)

			if P.Is_infinity() {
				res=INVALID;
			} else {
				d=P.GetX()
				d.Mod(r)

				if comp(d,c)!=0 {res=INVALID}
			}
		}
	}

	return res
}

/* IEEE1363 ECIES encryption. Encryption of plaintext M uses public key W and produces ciphertext V,C,T */
func ECDH_ECIES_ENCRYPT(sha int,P1 []byte,P2 []byte,RNG *amcl.RAND,W []byte,M []byte,V []byte,T []byte) []byte { 
	var Z [EFS]byte
	var VZ [3*EFS+1]byte
	var K1 [AESKEY]byte
	var K2 [AESKEY]byte
	var U [EGS]byte

	if ECDH_KEY_PAIR_GENERATE(RNG,U[:],V)!=0 {return nil}
	if ECDH_ECPSVDP_DH(U[:],W,Z[:])!=0 {return nil}     

	for i:=0;i<2*EFS+1;i++ {VZ[i]=V[i]}
	for i:=0;i<EFS;i++ {VZ[2*EFS+1+i]=Z[i]}


	K:=ECDH_KDF2(sha,VZ[:],P1,2*AESKEY)

	for i:=0;i<AESKEY;i++ {K1[i]=K[i]; K2[i]=K[AESKEY+i]} 

	C:=AES_CBC_IV0_ENCRYPT(K1[:],M)

	L2:=inttoBytes(len(P2),8)	
	
	var AC []byte

	for i:=0;i<len(C);i++ {AC=append(AC,C[i])}   
	for i:=0;i<len(P2);i++ {AC=append(AC,P2[i])}
	for i:=0;i<8;i++ {AC=append(AC,L2[i])}
	
	HMAC(sha,AC,K2[:],T)

	return C
}

/* IEEE1363 ECIES decryption. Decryption of ciphertext V,C,T using private key U outputs plaintext M */
func ECDH_ECIES_DECRYPT(sha int,P1 []byte,P2 []byte,V []byte,C []byte,T []byte,U []byte) []byte { 
	var Z [EFS]byte
	var VZ [3*EFS+1]byte
	var K1 [AESKEY]byte
	var K2 [AESKEY]byte

	var TAG []byte =T[:]  

	if ECDH_ECPSVDP_DH(U,V,Z[:])!=0 {return nil}

	for i:=0;i<2*EFS+1;i++ {VZ[i]=V[i]}
	for i:=0;i<EFS;i++ {VZ[2*EFS+1+i]=Z[i]}

	K:=ECDH_KDF2(sha,VZ[:],P1,2*AESKEY)

	for i:=0;i<AESKEY;i++ {K1[i]=K[i]; K2[i]=K[AESKEY+i]} 

	M:=AES_CBC_IV0_DECRYPT(K1[:],C)

	if M==nil {return nil}

	L2:=inttoBytes(len(P2),8)	
	
	var AC []byte
	
	for i:=0;i<len(C);i++ {AC=append(AC,C[i])}   
	for i:=0;i<len(P2);i++ {AC=append(AC,P2[i])}
	for i:=0;i<8;i++ {AC=append(AC,L2[i])}
	
	HMAC(sha,AC,K2[:],TAG)

	same:=true
	for i:=0;i<len(T);i++ {
		if T[i]!=TAG[i] {same=false}
	}
	if !same {return nil}
	
	return M
}


