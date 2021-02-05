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
* Implementation of the AES-GCM Encryption/Authentication
*
* Some restrictions..
* 1. Only for use with AES
* 2. Returned tag is always 128-bits. Truncate at your own risk.
* 3. The order of function calls must follow some rules
*
* Typical sequence of calls..
* 1. call GCM_init
* 2. call GCM_add_header any number of times, as long as length of header is multiple of 16 bytes (block size)
* 3. call GCM_add_header one last time with any length of header
* 4. call GCM_add_cipher any number of times, as long as length of cipher/plaintext is multiple of 16 bytes
* 5. call GCM_add_cipher one last time with any length of cipher/plaintext
* 6. call GCM_finish to extract the tag.
*
* See http://www.mindspring.com/~dmcgrew/gcm-nist-6.pdf
 */

package core

import (
	//	"fmt"
	"strconv"
)

const gcm_NB int = 4
const GCM_ACCEPTING_HEADER int = 0
const GCM_ACCEPTING_CIPHER int = 1
const GCM_NOT_ACCEPTING_MORE int = 2
const GCM_FINISHED int = 3
const GCM_ENCRYPTING int = 0
const GCM_DECRYPTING int = 1

type GCM struct {
	table   [128][4]uint32 /* 2k bytes */
	stateX  [16]byte
	Y_0     [16]byte
	counter int
	lenA    [2]uint32
	lenC    [2]uint32
	status  int
	a       *AES
}

func gcm_pack(b [4]byte) uint32 { /* pack bytes into a 32-bit Word */
	return ((uint32(b[0]) & 0xff) << 24) | ((uint32(b[1]) & 0xff) << 16) | ((uint32(b[2]) & 0xff) << 8) | (uint32(b[3]) & 0xff)
}

func gcm_unpack(a uint32) [4]byte { /* unpack bytes from a word */
	var b = [4]byte{byte((a >> 24) & 0xff), byte((a >> 16) & 0xff), byte((a >> 8) & 0xff), byte(a & 0xff)}
	return b
}

func (G *GCM) precompute(H []byte) {
	var b [4]byte
	j := 0
	for i := 0; i < gcm_NB; i++ {
		b[0] = H[j]
		b[1] = H[j+1]
		b[2] = H[j+2]
		b[3] = H[j+3]
		G.table[0][i] = gcm_pack(b)
		j += 4
	}
	for i := 1; i < 128; i++ {
		c := uint32(0)
		for j := 0; j < gcm_NB; j++ {
			G.table[i][j] = c | (G.table[i-1][j])>>1
			c = G.table[i-1][j] << 31
		}
		if c != 0 {
			G.table[i][0] ^= 0xE1000000
		} /* irreducible polynomial */
	}
}

func (G *GCM) gf2mul() { /* gf2m mul - Z=H*X mod 2^128 */
	var P [4]uint32

	for i := 0; i < 4; i++ {
		P[i] = 0
	}
	j := uint(8)
	m := 0
	for i := 0; i < 128; i++ {
		j--
		c := uint32((G.stateX[m] >> j) & 1)
		c = ^c + 1
		for k := 0; k < gcm_NB; k++ {
			P[k] ^= (G.table[i][k] & c)
		}
		if j == 0 {
			j = 8
			m++
			if m == 16 {
				break
			}
		}
	}
	j = 0
	for i := 0; i < gcm_NB; i++ {
		b := gcm_unpack(P[i])
		G.stateX[j] = b[0]
		G.stateX[j+1] = b[1]
		G.stateX[j+2] = b[2]
		G.stateX[j+3] = b[3]
		j += 4
	}
}

func (G *GCM) wrap() { /* Finish off GHASH */
	var F [4]uint32
	var L [16]byte

	/* convert lengths from bytes to bits */
	F[0] = (G.lenA[0] << 3) | (G.lenA[1]&0xE0000000)>>29
	F[1] = G.lenA[1] << 3
	F[2] = (G.lenC[0] << 3) | (G.lenC[1]&0xE0000000)>>29
	F[3] = G.lenC[1] << 3
	j := 0
	for i := 0; i < gcm_NB; i++ {
		b := gcm_unpack(F[i])
		L[j] = b[0]
		L[j+1] = b[1]
		L[j+2] = b[2]
		L[j+3] = b[3]
		j += 4
	}
	for i := 0; i < 16; i++ {
		G.stateX[i] ^= L[i]
	}
	G.gf2mul()
}

func (G *GCM) ghash(plain []byte, len int) bool {
	if G.status == GCM_ACCEPTING_HEADER {
		G.status = GCM_ACCEPTING_CIPHER
	}
	if G.status != GCM_ACCEPTING_CIPHER {
		return false
	}

	j := 0
	for j < len {
		for i := 0; i < 16 && j < len; i++ {
			G.stateX[i] ^= plain[j]
			j++
			G.lenC[1]++
			if G.lenC[1] == 0 {
				G.lenC[0]++
			}
		}
		G.gf2mul()
	}
	if len%16 != 0 {
		G.status = GCM_NOT_ACCEPTING_MORE
	}
	return true
}

/* Initialize GCM mode */
func (G *GCM) Init(nk int, key []byte, niv int, iv []byte) { /* iv size niv is usually 12 bytes (96 bits). AES key size nk can be 16,24 or 32 bytes */
	var H [16]byte

	for i := 0; i < 16; i++ {
		H[i] = 0
		G.stateX[i] = 0
	}

	G.a = new(AES)

	G.a.Init(AES_ECB, nk, key, iv)
	G.a.ecb_encrypt(H[:]) /* E(K,0) */
	G.precompute(H[:])

	G.lenA[0] = 0
	G.lenC[0] = 0
	G.lenA[1] = 0
	G.lenC[1] = 0
	if niv == 12 {
		for i := 0; i < 12; i++ {
			G.a.f[i] = iv[i]
		}
		b := gcm_unpack(uint32(1))
		G.a.f[12] = b[0]
		G.a.f[13] = b[1]
		G.a.f[14] = b[2]
		G.a.f[15] = b[3] /* initialise IV */
		for i := 0; i < 16; i++ {
			G.Y_0[i] = G.a.f[i]
		}
	} else {
		G.status = GCM_ACCEPTING_CIPHER
		G.ghash(iv, niv) /* GHASH(H,0,IV) */
		G.wrap()
		for i := 0; i < 16; i++ {
			G.a.f[i] = G.stateX[i]
			G.Y_0[i] = G.a.f[i]
			G.stateX[i] = 0
		}
		G.lenA[0] = 0
		G.lenC[0] = 0
		G.lenA[1] = 0
		G.lenC[1] = 0
	}
	G.status = GCM_ACCEPTING_HEADER
}

/* Add Header data - included but not encrypted */
func (G *GCM) Add_header(header []byte, len int) bool { /* Add some header. Won't be encrypted, but will be authenticated. len is length of header */
	if G.status != GCM_ACCEPTING_HEADER {
		return false
	}

	j := 0
	for j < len {
		for i := 0; i < 16 && j < len; i++ {
			G.stateX[i] ^= header[j]
			j++
			G.lenA[1]++
			if G.lenA[1] == 0 {
				G.lenA[0]++
			}
		}
		G.gf2mul()
	}
	if len%16 != 0 {
		G.status = GCM_ACCEPTING_CIPHER
	}

	return true
}

/* Add Plaintext - included and encrypted */
func (G *GCM) Add_plain(plain []byte, len int) []byte {
	var B [16]byte
	var b [4]byte

	cipher := make([]byte, len)
	var counter uint32 = 0
	if G.status == GCM_ACCEPTING_HEADER {
		G.status = GCM_ACCEPTING_CIPHER
	}
	if G.status != GCM_ACCEPTING_CIPHER {
		return nil
	}

	j := 0
	for j < len {

		b[0] = G.a.f[12]
		b[1] = G.a.f[13]
		b[2] = G.a.f[14]
		b[3] = G.a.f[15]
		counter = gcm_pack(b)
		counter++
		b = gcm_unpack(counter)
		G.a.f[12] = b[0]
		G.a.f[13] = b[1]
		G.a.f[14] = b[2]
		G.a.f[15] = b[3] /* increment counter */
		for i := 0; i < 16; i++ {
			B[i] = G.a.f[i]
		}
		G.a.ecb_encrypt(B[:]) /* encrypt it  */

		for i := 0; i < 16 && j < len; i++ {
			cipher[j] = (plain[j] ^ B[i])
			G.stateX[i] ^= cipher[j]
			j++
			G.lenC[1]++
			if G.lenC[1] == 0 {
				G.lenC[0]++
			}
		}
		G.gf2mul()
	}
	if len%16 != 0 {
		G.status = GCM_NOT_ACCEPTING_MORE
	}
	return cipher
}

/* Add Ciphertext - decrypts to plaintext */
func (G *GCM) Add_cipher(cipher []byte, len int) []byte {
	var B [16]byte
	var b [4]byte

	plain := make([]byte, len)
	var counter uint32 = 0

	if G.status == GCM_ACCEPTING_HEADER {
		G.status = GCM_ACCEPTING_CIPHER
	}
	if G.status != GCM_ACCEPTING_CIPHER {
		return nil
	}

	j := 0
	for j < len {
		b[0] = G.a.f[12]
		b[1] = G.a.f[13]
		b[2] = G.a.f[14]
		b[3] = G.a.f[15]
		counter = gcm_pack(b)
		counter++
		b = gcm_unpack(counter)
		G.a.f[12] = b[0]
		G.a.f[13] = b[1]
		G.a.f[14] = b[2]
		G.a.f[15] = b[3] /* increment counter */
		for i := 0; i < 16; i++ {
			B[i] = G.a.f[i]
		}
		G.a.ecb_encrypt(B[:]) /* encrypt it  */
		for i := 0; i < 16 && j < len; i++ {
			oc := cipher[j]
			plain[j] = (cipher[j] ^ B[i])
			G.stateX[i] ^= oc
			j++
			G.lenC[1]++
			if G.lenC[1] == 0 {
				G.lenC[0]++
			}
		}
		G.gf2mul()
	}
	if len%16 != 0 {
		G.status = GCM_NOT_ACCEPTING_MORE
	}
	return plain
}

/* Finish and extract Tag */
func (G *GCM) Finish(extract bool) []byte { /* Finish off GHASH and extract tag (MAC) */
	var tag []byte

	G.wrap()
	/* extract tag */
	if extract {
		G.a.ecb_encrypt(G.Y_0[:]) /* E(K,Y0) */
		for i := 0; i < 16; i++ {
			G.Y_0[i] ^= G.stateX[i]
		}
		for i := 0; i < 16; i++ {
			tag = append(tag,G.Y_0[i])
			G.Y_0[i] = 0
			G.stateX[i] = 0
		}
	}
	G.status = GCM_FINISHED
	G.a.End()
	return tag
}

func hex2bytes(s string) []byte {
	lgh := len(s)
	data := make([]byte, lgh/2)

	for i := 0; i < lgh; i += 2 {
		a, _ := strconv.ParseInt(s[i:i+2], 16, 32)
		data[i/2] = byte(a)
	}
	return data
}

func GCM_ENCRYPT(K []byte,IV []byte,H []byte,P []byte) ([]byte,[]byte){
	g:=new(GCM)
	g.Init(len(K),K,len(IV),IV)
	g.Add_header(H,len(H))
	C:=g.Add_plain(P,len(P))
	T:=g.Finish(true)
	return C,T
}

func GCM_DECRYPT(K []byte,IV []byte,H []byte,C []byte) ([]byte,[]byte){
	g:=new(GCM)
	g.Init(len(K),K,len(IV),IV)
	g.Add_header(H,len(H))
	P:=g.Add_cipher(C,len(C))
	T:=g.Finish(true)
	return P,T
}

/*
func main() {

	KT:="feffe9928665731c6d6a8f9467308308"
	MT:="d9313225f88406e5a55909c5aff5269a86a7a9531534f7da2e4c303d8a318a721c3c0c95956809532fcf0e2449a6b525b16aedf5aa0de657ba637b39"
	HT:="feedfacedeadbeeffeedfacedeadbeefabaddad2"

	NT:="9313225df88406e555909c5aff5269aa6a7a9538534f7da1e4c303d2a318a728c3c0c95156809539fcf0e2429a6b525416aedbf5a0de6a57a637b39b";
// Tag should be 619cc5aefffe0bfa462af43c1699d050

	g:=new(GCM)

	M:=hex2bytes(MT)
	H:=hex2bytes(HT)
	N:=hex2bytes(NT)
	K:=hex2bytes(KT)

	lenM:=len(M)
	lenH:=len(H)
	lenK:=len(K)
	lenIV:=len(N)

 	fmt.Printf("Plaintext=\n");
	for i:=0;i<lenM;i++ {fmt.Printf("%02x",M[i])}
	fmt.Printf("\n")

	g.Init(lenK,K,lenIV,N)
	g.Add_header(H,lenH)
	C:=g.Add_plain(M,lenM)
	T:=g.Finish(true)

	fmt.Printf("Ciphertext=\n")
	for i:=0;i<lenM;i++ {fmt.Printf("%02x",C[i])}
	fmt.Printf("\n")

	fmt.Printf("Tag=\n")
	for i:=0;i<16;i++ {fmt.Printf("%02x",T[i])}
	fmt.Printf("\n")

	g.Init(lenK,K,lenIV,N)
	g.Add_header(H,lenH)
	P:=g.Add_cipher(C,lenM)
	T=g.Finish(true)

 	fmt.Printf("Plaintext=\n");
	for i:=0;i<lenM;i++ {fmt.Printf("%02x",P[i])}
	fmt.Printf("\n")

	fmt.Printf("Tag=\n");
	for i:=0;i<16;i++ {fmt.Printf("%02x",T[i])}
	fmt.Printf("\n")
}
*/
