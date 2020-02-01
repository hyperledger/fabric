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
 * Implementation of the Secure Hashing Algorithm (SHA-256)
 *
 * Generates a 256 bit message digest. It should be impossible to come
 * come up with two messages that hash to the same value ("collision free").
 *
 * For use with byte-oriented messages only.
 */

package amcl

//import "fmt"
const SHA256 int = 32

const hash256_H0 uint32 = 0x6A09E667
const hash256_H1 uint32 = 0xBB67AE85
const hash256_H2 uint32 = 0x3C6EF372
const hash256_H3 uint32 = 0xA54FF53A
const hash256_H4 uint32 = 0x510E527F
const hash256_H5 uint32 = 0x9B05688C
const hash256_H6 uint32 = 0x1F83D9AB
const hash256_H7 uint32 = 0x5BE0CD19

var hash256_K = [...]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2}

type HASH256 struct {
	length [2]uint32
	h      [8]uint32
	w      [64]uint32
}

/* functions */
func hash256_S(n uint32, x uint32) uint32 {
	return (((x) >> n) | ((x) << (32 - n)))
}

func hash256_R(n uint32, x uint32) uint32 {
	return ((x) >> n)
}

func hash256_Ch(x, y, z uint32) uint32 {
	return ((x & y) ^ (^(x) & z))
}

func hash256_Maj(x, y, z uint32) uint32 {
	return ((x & y) ^ (x & z) ^ (y & z))
}

func hash256_Sig0(x uint32) uint32 {
	return (hash256_S(2, x) ^ hash256_S(13, x) ^ hash256_S(22, x))
}

func hash256_Sig1(x uint32) uint32 {
	return (hash256_S(6, x) ^ hash256_S(11, x) ^ hash256_S(25, x))
}

func hash256_theta0(x uint32) uint32 {
	return (hash256_S(7, x) ^ hash256_S(18, x) ^ hash256_R(3, x))
}

func hash256_theta1(x uint32) uint32 {
	return (hash256_S(17, x) ^ hash256_S(19, x) ^ hash256_R(10, x))
}

func (H *HASH256) transform() { /* basic transformation step */
	for j := 16; j < 64; j++ {
		H.w[j] = hash256_theta1(H.w[j-2]) + H.w[j-7] + hash256_theta0(H.w[j-15]) + H.w[j-16]
	}
	a := H.h[0]
	b := H.h[1]
	c := H.h[2]
	d := H.h[3]
	e := H.h[4]
	f := H.h[5]
	g := H.h[6]
	hh := H.h[7]
	for j := 0; j < 64; j++ { /* 64 times - mush it up */
		t1 := hh + hash256_Sig1(e) + hash256_Ch(e, f, g) + hash256_K[j] + H.w[j]
		t2 := hash256_Sig0(a) + hash256_Maj(a, b, c)
		hh = g
		g = f
		f = e
		e = d + t1
		d = c
		c = b
		b = a
		a = t1 + t2
	}
	H.h[0] += a
	H.h[1] += b
	H.h[2] += c
	H.h[3] += d
	H.h[4] += e
	H.h[5] += f
	H.h[6] += g
	H.h[7] += hh
}

/* Initialise Hash function */
func (H *HASH256) Init() { /* initialise */
	for i := 0; i < 64; i++ {
		H.w[i] = 0
	}
	H.length[0] = 0
	H.length[1] = 0
	H.h[0] = hash256_H0
	H.h[1] = hash256_H1
	H.h[2] = hash256_H2
	H.h[3] = hash256_H3
	H.h[4] = hash256_H4
	H.h[5] = hash256_H5
	H.h[6] = hash256_H6
	H.h[7] = hash256_H7
}

func NewHASH256() *HASH256 {
	H := new(HASH256)
	H.Init()
	return H
}

/* process a single byte */
func (H *HASH256) Process(byt byte) { /* process the next message byte */
	cnt := (H.length[0] / 32) % 16

	H.w[cnt] <<= 8
	H.w[cnt] |= uint32(byt & 0xFF)
	H.length[0] += 8
	if H.length[0] == 0 {
		H.length[1]++
		H.length[0] = 0
	}
	if (H.length[0] % 512) == 0 {
		H.transform()
	}
}

/* process an array of bytes */
func (H *HASH256) Process_array(b []byte) {
	for i := 0; i < len(b); i++ {
		H.Process((b[i]))
	}
}

/* process a 32-bit integer */
func (H *HASH256) Process_num(n int32) {
	H.Process(byte((n >> 24) & 0xff))
	H.Process(byte((n >> 16) & 0xff))
	H.Process(byte((n >> 8) & 0xff))
	H.Process(byte(n & 0xff))
}

/* Generate 32-byte Hash */
func (H *HASH256) Hash() []byte { /* pad message and finish - supply digest */
	var digest [32]byte
	len0 := H.length[0]
	len1 := H.length[1]
	H.Process(0x80)
	for (H.length[0] % 512) != 448 {
		H.Process(0)
	}
	H.w[14] = len1
	H.w[15] = len0
	H.transform()
	for i := 0; i < 32; i++ { /* convert to bytes */
		digest[i] = byte((H.h[i/4] >> uint(8*(3-i%4))) & 0xff)
	}
	H.Init()
	return digest[0:32]
}

/* test program: should produce digest */

//248d6a61 d20638b8 e5c02693 0c3e6039 a33ce459 64ff2167 f6ecedd4 19db06c1
/*
func main() {

	test := []byte("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq")
	sh:=NewHASH256()

	for i:=0;i<len(test);i++ {
		sh.Process(test[i])
	}

	digest:=sh.Hash()
	for i:=0;i<32;i++ {fmt.Printf("%02x",digest[i])}

} */
