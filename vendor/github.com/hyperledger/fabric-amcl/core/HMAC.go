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

/*
 * Implementation of the Secure Hashing Algorithm (SHA-256)
 *
 * Generates a 256 bit message digest. It should be impossible to come
 * come up with two messages that hash to the same value ("collision free").
 *
 * For use with byte-oriented messages only.
 */

package core

const MC_SHA2 int=2
const MC_SHA3 int=3

/* Convert Integer to n-byte array */
func InttoBytes(n int, len int) []byte {
	var b []byte
	var i int
	for i = 0; i < len; i++ {
		b = append(b, 0)
	}
	i = len
	for n > 0 && i > 0 {
		i--
		b[i] = byte(n & 0xff)
		n /= 256
	}
	return b
}
/* general purpose hashing of Byte array|integer|Byte array. Output of length olen, padded with leading zeros if required */

func GPhashit(hash int,hlen int, olen int, zpad int,A []byte, n int32, B []byte) []byte {
	var R []byte
	if hash == MC_SHA2 {
		if hlen == SHA256 {
			H := NewHASH256()
			for i := 0; i < zpad; i++ {
				H.Process(0)
			}
			if A != nil {
				H.Process_array(A)
			}
			if n >= 0 {
				H.Process_num(int32(n))
			}
			if B != nil {
				H.Process_array(B)
			}
			R = H.Hash()
		}
		if hlen == SHA384 {
			H := NewHASH384()
			for i := 0; i < zpad; i++ {
				H.Process(0)
			}
			if A != nil {
				H.Process_array(A)
			}
			if n >= 0 {
				H.Process_num(int32(n))
			}
			if B != nil {
				H.Process_array(B)
			}
			R = H.Hash()
		}
		if hlen == SHA512 {
			H := NewHASH512()
			for i := 0; i < zpad; i++ {
				H.Process(0)
			}
			if A != nil {
				H.Process_array(A)
			}
			if n >= 0 {
				H.Process_num(int32(n))
			}
			if B != nil {
				H.Process_array(B)
			}
			R = H.Hash()
		}
	}
	if hash == MC_SHA3 {
		H := NewSHA3(hlen)
		for i := 0; i < zpad; i++ {
			H.Process(0)
		}
		if A != nil {
			H.Process_array(A)
		}
		if n >= 0 {
			H.Process_num(int32(n))
		}
		if B != nil {
			H.Process_array(B)
		}
		R = H.Hash()
	}

	if R == nil {
		return nil
	}

	if olen == 0 {
		return R
	}
	var W []byte
	for i := 0; i < olen; i++ {
		W = append(W, 0)
	}
	if olen <= hlen {
		for i := 0; i < olen; i++ {
			W[i] = R[i]
		}
	} else {
		for i := 0; i < hlen; i++ {
			W[i+olen-hlen] = R[i]
		}
		for i := 0; i < olen-hlen; i++ {
			W[i] = 0
		}
	}
	return W
}

/* Simple hashing of byte array */
func SPhashit(hash int,hlen int, A []byte) []byte {
	return GPhashit(hash,hlen,0,0,A,-1,nil)
}

/* Key Derivation Function */
/* Input octet Z */
/* Output key of length olen */

func KDF2(hash int, sha int, Z []byte, P []byte, olen int) []byte {
	/* NOTE: the parameter olen is the length of the output k in bytes */
	hlen := sha
	var K []byte
	k := 0

	for i := 0; i < olen; i++ {
		K = append(K, 0)
	}

	cthreshold := olen / hlen
	if olen%hlen != 0 {
		cthreshold++
	}

	for counter := 1; counter <= cthreshold; counter++ {
		B := GPhashit(hash,sha, 0, 0, Z, int32(counter), P)
		if k+hlen > olen {
			for i := 0; i < olen%hlen; i++ {
				K[k] = B[i]
				k++
			}
		} else {
			for i := 0; i < hlen; i++ {
				K[k] = B[i]
				k++
			}
		}
	}
	return K
}

/* Password based Key Derivation Function */
/* Input password p, salt s, and repeat count */
/* Output key of length olen */
func PBKDF2(hash int, sha int, Pass []byte, Salt []byte, rep int, olen int) []byte {
	d := olen / sha
	if olen%sha != 0 {
		d++
	}

	var F []byte
	var U []byte
	var S []byte
	var K []byte

	for i := 0; i < sha; i++ {
		F = append(F, 0)
		U = append(U, 0)
	}

	for i := 1; i <= d; i++ {
		for j := 0; j < len(Salt); j++ {
			S = append(S, Salt[j])
		}
		N := InttoBytes(i, 4)
		for j := 0; j < 4; j++ {
			S = append(S, N[j])
		}

		HMAC(MC_SHA2, sha, F[:], sha, S, Pass)

		for j := 0; j < sha; j++ {
			U[j] = F[j]
		}
		for j := 2; j <= rep; j++ {
			HMAC(MC_SHA2, sha, U[:], sha, U[:], Pass)
			for k := 0; k < sha; k++ {
				F[k] ^= U[k]
			}
		}
		for j := 0; j < sha; j++ {
			K = append(K, F[j])
		}
	}
	var key []byte
	for i := 0; i < olen; i++ {
		key = append(key, K[i])
	}
	return key
}

func blksize(hash int,sha int) int {
	b := 0
	if hash == MC_SHA2 {
		b = 64
		if sha > 32 {
			b = 128
		}
	}
	if hash == MC_SHA3 {
		b=200-2*sha
	}
	return b
}

/* Calculate HMAC of m using key k. HMAC is tag of length olen (which is length of tag) */
func HMAC(hash int, sha int, tag []byte, olen int, K []byte, M []byte) int {
	/* Input is from an octet m        *
	* olen is requested output length in bytes. k is the key  *
	* The output is the calculated tag */
	var B []byte

	b := blksize(hash,sha)
	if b == 0 {return 0}

	var K0 [200]byte
	//olen := len(tag)

	for i := 0; i < b; i++ {
		K0[i] = 0
	}

	if len(K) > b {
		B = SPhashit(hash, sha, K)
		for i := 0; i < sha; i++ {
			K0[i] = B[i]
		}
	} else {
		for i := 0; i < len(K); i++ {
			K0[i] = K[i]
		}
	}

	for i := 0; i < b; i++ {
		K0[i] ^= 0x36
	}
	B = GPhashit(hash, sha, 0, 0, K0[0:b], -1, M)

	for i := 0; i < b; i++ {
		K0[i] ^= 0x6a
	}
	B = GPhashit(hash, sha, olen, 0, K0[0:b], -1, B)

	for i := 0; i < olen; i++ {
		tag[i] = B[i]
	}

	return 1
}

func HKDF_Extract(hash int, hlen int, SALT []byte, IKM []byte)  []byte { 
	var PRK []byte
	for i:=0;i<hlen;i++ {
		PRK = append(PRK,0)
	}
	if SALT == nil {
		var H []byte
		for i := 0; i < hlen; i++ {
			H = append(H, 0)
		}
		HMAC(hash,hlen,PRK,hlen,H,IKM)
	} else {
		HMAC(hash,hlen,PRK,hlen,SALT,IKM)
	}
	return PRK
}

func HKDF_Expand(hash int, hlen int, olen int, PRK []byte, INFO []byte) []byte { 
	n := olen/hlen;
	flen := olen%hlen;

	var OKM []byte
	var T []byte
	var K [64]byte

	for i:=1;i<=n;i++ {
		for j := 0; j < len(INFO); j++ {
			T = append(T, INFO[j])
		}
		T = append(T, byte(i))
		HMAC(hash,hlen,K[:],hlen,PRK,T);
		T = nil
		for j := 0; j < hlen; j++ {
			OKM = append(OKM, K[j])
			T= append(T,K[j])
		}
	}
	if flen > 0 {
		for j := 0; j < len(INFO); j++ {
			T = append(T, INFO[j])
		}
		T = append(T, byte(n+1))
		HMAC(hash,hlen,K[:],flen,PRK,T);
		for j := 0; j < flen; j++ {
			OKM = append(OKM, K[j])
		}
	}
	return OKM
}

func ceil(a int,b int) int {
    return (((a)-1)/(b)+1)
}

func XOF_Expand(hlen int,olen int,DST []byte,MSG []byte) []byte {
	var OKM =make([]byte,olen)
	H := NewSHA3(hlen)
	for i:=0;i<len(MSG);i++ {
		H.Process(MSG[i])
	}
	H.Process(byte((olen >> 8) & 0xff));
	H.Process(byte(olen & 0xff));

	for i:=0;i<len(DST);i++ {
		H.Process(DST[i])
	}
	H.Process(byte(len(DST) & 0xff));

	H.Shake(OKM[:],olen)
	return OKM
}

func XMD_Expand(hash int,hlen int,olen int,DST []byte,MSG []byte) []byte {
	var OKM =make([]byte,olen)
	var TMP =make([]byte,len(DST)+4)

	ell:=ceil(olen,hlen)
	blk:=blksize(hash,hlen)
	TMP[0]=byte((olen >> 8) & 0xff)
	TMP[1]=byte(olen & 0xff)
	TMP[2]=byte(0)

	for j:=0;j<len(DST);j++ {
		TMP[3+j]=DST[j]
	}
	TMP[3+len(DST)]=byte(len(DST) & 0xff)
	var H0=GPhashit(hash, hlen, 0, blk, MSG, -1, TMP)

	var H1=make([]byte,hlen)
	var TMP2=make([]byte,len(DST)+2)

    k:=0
	for i:=1;i<=ell;i++ {
		for j:=0;j<hlen;j++ {
			H1[j]^=H0[j]
		}          
		TMP2[0]=byte(i)

		for j:=0;j<len(DST);j++ {
			TMP2[1+j]=DST[j]
		}
		TMP2[1+len(DST)]=byte(len(DST) & 0xff)

		H1=GPhashit(hash, hlen, 0, 0, H1, -1, TMP2);
		for j:=0;j<hlen && k<olen;j++ {
                OKM[k]=H1[j]
				k++
        }
	}        
    return OKM;
}


/* Mask Generation Function */

func MGF1(sha int, Z []byte, olen int, K []byte) {
	hlen := sha

	var k int = 0
	for i := 0; i < len(K); i++ {
		K[i] = 0
	}

	cthreshold := olen / hlen
	if olen%hlen != 0 {
		cthreshold++
	}
	for counter := 0; counter < cthreshold; counter++ {
		B := GPhashit(MC_SHA2,sha,0,0,Z,int32(counter),nil)
		//B := hashit(sha, Z, counter)

		if k+hlen > olen {
			for i := 0; i < olen%hlen; i++ {
				K[k] = B[i]
				k++
			}
		} else {
			for i := 0; i < hlen; i++ {
				K[k] = B[i]
				k++
			}
		}
	}
}


func MGF1XOR(sha int, Z []byte, olen int, K []byte) {
	hlen := sha

	var k int = 0

	cthreshold := olen / hlen
	if olen%hlen != 0 {
		cthreshold++
	}
	for counter := 0; counter < cthreshold; counter++ {
		B := GPhashit(MC_SHA2,sha,0,0,Z,int32(counter),nil)
		//B := hashit(sha, Z, counter)

		if k+hlen > olen {
			for i := 0; i < olen%hlen; i++ {
				K[k] ^= B[i]
				k++
			}
		} else {
			for i := 0; i < hlen; i++ {
				K[k] ^= B[i]
				k++
			}
		}
	}
}

/* SHAXXX identifier strings */
var SHA256ID = [...]byte{0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20}
var SHA384ID = [...]byte{0x30, 0x41, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x05, 0x00, 0x04, 0x30}
var SHA512ID = [...]byte{0x30, 0x51, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x05, 0x00, 0x04, 0x40}

func RSA_PKCS15(sha int, m []byte, w []byte, RFS int) bool {
	olen := RFS
	hlen := sha
	idlen := 19

	if olen < idlen+hlen+10 {
		return false
	}
	H := SPhashit(MC_SHA2,sha,m)
	//H := hashit(sha, m, -1)

	for i := 0; i < len(w); i++ {
		w[i] = 0
	}
	i := 0
	w[i] = 0
	i++
	w[i] = 1
	i++
	for j := 0; j < olen-idlen-hlen-3; j++ {
		w[i] = 0xff
		i++
	}
	w[i] = 0
	i++

	if hlen == SHA256 {
		for j := 0; j < idlen; j++ {
			w[i] = SHA256ID[j]
			i++
		}
	}
	if hlen == SHA384 {
		for j := 0; j < idlen; j++ {
			w[i] = SHA384ID[j]
			i++
		}
	}
	if hlen == SHA512 {
		for j := 0; j < idlen; j++ {
			w[i] = SHA512ID[j]
			i++
		}
	}
	for j := 0; j < hlen; j++ {
		w[i] = H[j]
		i++
	}

	return true
}

/* SHAXXX identifier strings */
var SHA256IDb = [...]byte{0x30, 0x2f, 0x30, 0x0b, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x04, 0x20}
var SHA384IDb = [...]byte{0x30, 0x3f, 0x30, 0x0b, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x02, 0x04, 0x30}
var SHA512IDb = [...]byte{0x30, 0x4f, 0x30, 0x0b, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x03, 0x04, 0x40}

func RSA_PKCS15b(sha int, m []byte, w []byte, RFS int) bool {
	olen := RFS
	hlen := sha
	idlen := 17

	if olen < idlen+hlen+10 {
		return false
	}
	H := SPhashit(MC_SHA2,sha,m)
	//H := hashit(sha, m, -1)

	for i := 0; i < len(w); i++ {
		w[i] = 0
	}
	i := 0
	w[i] = 0
	i++
	w[i] = 1
	i++
	for j := 0; j < olen-idlen-hlen-3; j++ {
		w[i] = 0xff
		i++
	}
	w[i] = 0
	i++

	if hlen == SHA256 {
		for j := 0; j < idlen; j++ {
			w[i] = SHA256IDb[j]
			i++
		}
	}
	if hlen == SHA384 {
		for j := 0; j < idlen; j++ {
			w[i] = SHA384IDb[j]
			i++
		}
	}
	if hlen == SHA512 {
		for j := 0; j < idlen; j++ {
			w[i] = SHA512IDb[j]
			i++
		}
	}
	for j := 0; j < hlen; j++ {
		w[i] = H[j]
		i++
	}
	return true
}


func RSA_PSS_ENCODE(sha int, m []byte, rng *RAND, RFS int) []byte {
	emlen:=RFS
	embits:=8*emlen-1

	hlen:=sha
	SALT := make([]byte,hlen)
	for i := 0; i < hlen; i++ {
		SALT[i] = rng.GetByte()
	}
	mask:=byte(0xff>>(8*emlen-embits))

	H := SPhashit(MC_SHA2,sha,m)
	if emlen < hlen+hlen+2 {
		return nil
	}

	MD := make([]byte,8+hlen+hlen)
	for i:=0;i<8;i++ {
		MD[i]=0;
	}
	for i:=0;i<hlen;i++ {
		MD[8+i]=H[i]
	}
	for i:=0;i<hlen;i++ {
		MD[8+hlen+i]=SALT[i]
	}
	H=SPhashit(MC_SHA2,sha,MD)
	f:= make([]byte,RFS)
	for i:=0;i<emlen-hlen-hlen-2;i++ {
		f[i]=0;
	}
	f[emlen-hlen-hlen-2]=0x1
	for i:=0;i<hlen;i++ {
		f[emlen+i-hlen-hlen-1]=SALT[i]
	}

	MGF1XOR(sha,H,emlen-hlen-1,f)
	f[0]&=mask;
	for i:=0;i<hlen;i++ {
		f[emlen+i-hlen-1]=H[i]
	}
	f[emlen-1]=byte(0xbc)
	return f
}

func RSA_PSS_VERIFY(sha int, m []byte, f []byte) bool {
	emlen:=len(f)
	embits:=8*emlen-1
	hlen:=sha
	SALT := make([]byte,hlen)
	mask:=byte(0xff>>(8*emlen-embits))

	HMASK := SPhashit(MC_SHA2,sha,m)
    if emlen < hlen + hlen +  2 {
		return false
	}
    if (f[emlen-1]!=byte(0xbc)) {
		return false
	}
    if (f[0]&(^mask))!=0 {
		return false
	}
	DB:=make([]byte,emlen-hlen-1)
	for i:=0;i<emlen-hlen-1;i++ {
		DB[i]=f[i]
	}
	H:=make([]byte,hlen)
	for i:=0;i<hlen;i++ {
		H[i]=f[emlen+i-hlen-1]
	}
	MGF1XOR(sha,H,emlen-hlen-1,DB)
	DB[0]&=mask;
	k:=byte(0);
	for i:=0;i<emlen-hlen-hlen-2;i++ {
		k|=DB[i]
	}
	if k!=0 {
		return false
	}
    if DB[emlen-hlen-hlen-2]!=0x01 {
		return false
	}

	for i:=0;i<hlen;i++ {
		SALT[i]=DB[emlen+i-hlen-hlen-1]
	}
	MD:=make([]byte,8+hlen+hlen)
    for i:=0;i<8;i++ {
        MD[i]=0
	}
    for i:=0;i<hlen;i++ {
        MD[8+i]=HMASK[i]
	}
    for i:=0;i<hlen;i++ {
        MD[8+hlen+i]=SALT[i]
	}
	HMASK=SPhashit(MC_SHA2,sha,MD)
	k=0
    for i:=0;i<hlen;i++ {
		k|=(H[i]-HMASK[i])
	}
	if k!=0 {
		return false
	}
	return true
}

/* OAEP Message Encoding for Encryption */
func RSA_OAEP_ENCODE(sha int, m []byte, rng *RAND, p []byte, RFS int) []byte {
	olen := RFS - 1
	mlen := len(m)
	//var f [RFS]byte
	f := make([]byte,RFS)

	hlen := sha

	SEED := make([]byte, hlen)

	seedlen := hlen
	if mlen > olen-hlen-seedlen-1 {
		return nil
	}

	DBMASK := make([]byte, olen-seedlen)

	h := SPhashit(MC_SHA2,sha,p)
	//h := hashit(sha, p, -1)

	for i := 0; i < hlen; i++ {
		f[i] = h[i]
	}

	slen := olen - mlen - hlen - seedlen - 1

	for i := 0; i < slen; i++ {
		f[hlen+i] = 0
	}
	f[hlen+slen] = 1
	for i := 0; i < mlen; i++ {
		f[hlen+slen+1+i] = m[i]
	}

	for i := 0; i < seedlen; i++ {
		SEED[i] = rng.GetByte()
	}
	MGF1(sha, SEED, olen-seedlen, DBMASK)

	for i := 0; i < olen-seedlen; i++ {
		DBMASK[i] ^= f[i]
	}

	MGF1(sha, DBMASK, seedlen, f[:])

	for i := 0; i < seedlen; i++ {
		f[i] ^= SEED[i]
	}

	for i := 0; i < olen-seedlen; i++ {
		f[i+seedlen] = DBMASK[i]
	}

	/* pad to length RFS */
	d := 1
	for i := RFS - 1; i >= d; i-- {
		f[i] = f[i-d]
	}
	for i := d - 1; i >= 0; i-- {
		f[i] = 0
	}
	return f[:]
}

/* OAEP Message Decoding for Decryption */
func RSA_OAEP_DECODE(sha int, p []byte, f []byte, RFS int) []byte {
	olen := RFS - 1

	hlen := sha
	SEED := make([]byte, hlen)
	seedlen := hlen
	CHASH := make([]byte, hlen)

	if olen < seedlen+hlen+1 {
		return nil
	}
	DBMASK := make([]byte, olen-seedlen)
	for i := 0; i < olen-seedlen; i++ {
		DBMASK[i] = 0
	}

	if len(f) < RFS {
		d := RFS - len(f)
		for i := RFS - 1; i >= d; i-- {
			f[i] = f[i-d]
		}
		for i := d - 1; i >= 0; i-- {
			f[i] = 0
		}
	}

	h := SPhashit(MC_SHA2,sha,p)
	//h := hashit(sha, p, -1)
	for i := 0; i < hlen; i++ {
		CHASH[i] = h[i]
	}

	x := f[0]

	for i := seedlen; i < olen; i++ {
		DBMASK[i-seedlen] = f[i+1]
	}

	MGF1(sha, DBMASK, seedlen, SEED)
	for i := 0; i < seedlen; i++ {
		SEED[i] ^= f[i+1]
	}
	MGF1(sha, SEED, olen-seedlen, f)
	for i := 0; i < olen-seedlen; i++ {
		DBMASK[i] ^= f[i]
	}

	comp := true
	for i := 0; i < hlen; i++ {
		if CHASH[i] != DBMASK[i] {
			comp = false
		}
	}

	for i := 0; i < olen-seedlen-hlen; i++ {
		DBMASK[i] = DBMASK[i+hlen]
	}

	for i := 0; i < hlen; i++ {
		SEED[i] = 0
		CHASH[i] = 0
	}

	var k int
	for k = 0; ; k++ {
		if k >= olen-seedlen-hlen {
			return nil
		}
		if DBMASK[k] != 0 {
			break
		}
	}

	t := DBMASK[k]
	if !comp || x != 0 || t != 0x01 {
		for i := 0; i < olen-seedlen; i++ {
			DBMASK[i] = 0
		}
		return nil
	}

	var r = make([]byte, olen-seedlen-hlen-k-1)

	for i := 0; i < olen-seedlen-hlen-k-1; i++ {
		r[i] = DBMASK[i+k+1]
	}

	for i := 0; i < olen-seedlen; i++ {
		DBMASK[i] = 0
	}

	return r
}




/*


	MSG := []byte("abc")
	DST := []byte("P256_XMD:SHA-256_SSWU_RO_TESTGEN")

	OKM := core.XOF_Expand(core.SHA3_SHAKE128,48,DST,MSG)
	fmt.Printf("OKM= "); printBinary(OKM[:])

	OKM = core.XMD_Expand(core.MC_SHA2,32,48,DST,MSG)
	fmt.Printf("OKM= "); printBinary(OKM[:])



func main() {
	var ikm []byte
	var salt []byte
	var info []byte
	
	for i:=0;i<22;i++ {ikm=append(ikm,0x0b)}
	for i:=0;i<13;i++ {salt=append(salt,byte(i))}
	for i:=0;i<10;i++ {info=append(info,byte(0xf0+i))}

	prk:=core.HKDF_Extract(core.MC_SHA2,32,salt,ikm)
	fmt.Printf("PRK= ")
	for i := 0; i < len(prk); i++ {
		fmt.Printf("%02x", prk[i])
	}

	okm:=core.HKDF_Expand(core.MC_SHA2,32,42,prk,info)
	fmt.Printf("\nOKM= ")
	for i := 0; i < len(okm); i++ {
		fmt.Printf("%02x", okm[i])
	}
	
}
*/

