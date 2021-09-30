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
 * Implementation of the Secure Hashing Algorithm (SHA-384)
 *
 * Generates a 384 bit message digest. It should be impossible to come
 * come up with two messages that hash to the same value ("collision free").
 *
 * For use with byte-oriented messages only.
 */

//package main

package core

//import "fmt"

const SHA3_HASH224 int = 28
const SHA3_HASH256 int = 32
const SHA3_HASH384 int = 48
const SHA3_HASH512 int = 64
const SHA3_SHAKE128 int = 16
const SHA3_SHAKE256 int = 32

const sha3_ROUNDS int = 24

var sha3_RC = [24]uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808A, 0x8000000080008000,
	0x000000000000808B, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
	0x000000000000008A, 0x0000000000000088, 0x0000000080008009, 0x000000008000000A,
	0x000000008000808B, 0x800000000000008B, 0x8000000000008089, 0x8000000000008003,
	0x8000000000008002, 0x8000000000000080, 0x000000000000800A, 0x800000008000000A,
	0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008}

type SHA3 struct {
	length uint64
	rate   int
	len    int
	s      [5][5]uint64
}

/* functions */

func sha3_ROTL(x uint64, n uint64) uint64 {
	return (((x) << n) | ((x) >> (64 - n)))
}

func (H *SHA3) transform() { /* basic transformation step */

	var c [5]uint64
	var d [5]uint64
	var b [5][5]uint64

	for k := 0; k < sha3_ROUNDS; k++ {
		c[0] = H.s[0][0] ^ H.s[0][1] ^ H.s[0][2] ^ H.s[0][3] ^ H.s[0][4]
		c[1] = H.s[1][0] ^ H.s[1][1] ^ H.s[1][2] ^ H.s[1][3] ^ H.s[1][4]
		c[2] = H.s[2][0] ^ H.s[2][1] ^ H.s[2][2] ^ H.s[2][3] ^ H.s[2][4]
		c[3] = H.s[3][0] ^ H.s[3][1] ^ H.s[3][2] ^ H.s[3][3] ^ H.s[3][4]
		c[4] = H.s[4][0] ^ H.s[4][1] ^ H.s[4][2] ^ H.s[4][3] ^ H.s[4][4]

		d[0] = c[4] ^ sha3_ROTL(c[1], 1)
		d[1] = c[0] ^ sha3_ROTL(c[2], 1)
		d[2] = c[1] ^ sha3_ROTL(c[3], 1)
		d[3] = c[2] ^ sha3_ROTL(c[4], 1)
		d[4] = c[3] ^ sha3_ROTL(c[0], 1)

		for i := 0; i < 5; i++ {
			for j := 0; j < 5; j++ {
				H.s[i][j] ^= d[i]
			}
		}

		b[0][0] = H.s[0][0]
		b[1][3] = sha3_ROTL(H.s[0][1], 36)
		b[2][1] = sha3_ROTL(H.s[0][2], 3)
		b[3][4] = sha3_ROTL(H.s[0][3], 41)
		b[4][2] = sha3_ROTL(H.s[0][4], 18)

		b[0][2] = sha3_ROTL(H.s[1][0], 1)
		b[1][0] = sha3_ROTL(H.s[1][1], 44)
		b[2][3] = sha3_ROTL(H.s[1][2], 10)
		b[3][1] = sha3_ROTL(H.s[1][3], 45)
		b[4][4] = sha3_ROTL(H.s[1][4], 2)

		b[0][4] = sha3_ROTL(H.s[2][0], 62)
		b[1][2] = sha3_ROTL(H.s[2][1], 6)
		b[2][0] = sha3_ROTL(H.s[2][2], 43)
		b[3][3] = sha3_ROTL(H.s[2][3], 15)
		b[4][1] = sha3_ROTL(H.s[2][4], 61)

		b[0][1] = sha3_ROTL(H.s[3][0], 28)
		b[1][4] = sha3_ROTL(H.s[3][1], 55)
		b[2][2] = sha3_ROTL(H.s[3][2], 25)
		b[3][0] = sha3_ROTL(H.s[3][3], 21)
		b[4][3] = sha3_ROTL(H.s[3][4], 56)

		b[0][3] = sha3_ROTL(H.s[4][0], 27)
		b[1][1] = sha3_ROTL(H.s[4][1], 20)
		b[2][4] = sha3_ROTL(H.s[4][2], 39)
		b[3][2] = sha3_ROTL(H.s[4][3], 8)
		b[4][0] = sha3_ROTL(H.s[4][4], 14)

		for i := 0; i < 5; i++ {
			for j := 0; j < 5; j++ {
				H.s[i][j] = b[i][j] ^ (^b[(i+1)%5][j] & b[(i+2)%5][j])
			}
		}

		H.s[0][0] ^= sha3_RC[k]
	}
}

/* Initialise Hash function */
func (H *SHA3) Init(olen int) {
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			H.s[i][j] = 0
		}
	}
	H.length = 0
	H.len = olen
	H.rate = 200 - 2*olen
}

func NewSHA3(olen int) *SHA3 {
	H := new(SHA3)
	H.Init(olen)
	return H
}

func NewSHA3copy(HC *SHA3) *SHA3 {
	H := new(SHA3)
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			H.s[i][j] = HC.s[i][j]
		}
	}
	H.length=HC.length
	H.len=HC.len
	H.rate=HC.rate
	return H
}

/* process a single byte */
func (H *SHA3) Process(byt byte) { /* process the next message byte */
	cnt := int(H.length % uint64(H.rate))
	b := cnt % 8
	cnt /= 8
	i := cnt % 5
	j := cnt / 5
	H.s[i][j] ^= uint64(byt&0xff) << uint(8*b)
	H.length++
	if int(H.length%uint64(H.rate)) == 0 {
		H.transform()
	}
}

/* process an array of bytes */
func (H *SHA3) Process_array(b []byte) {
	for i := 0; i < len(b); i++ {
		H.Process((b[i]))
	}
}

/* process a 32-bit integer */
func (H *SHA3) Process_num(n int32) {
	H.Process(byte((n >> 24) & 0xff))
	H.Process(byte((n >> 16) & 0xff))
	H.Process(byte((n >> 8) & 0xff))
	H.Process(byte(n & 0xff))
}


/* squeeze the sponge */
func (H *SHA3) Squeeze(buff []byte, olen int) {
	//	olen:=len(buff)
	done := false
	m := 0
	/* extract by columns */
	for {
		for j := 0; j < 5; j++ {
			for i := 0; i < 5; i++ {
				el := H.s[i][j]
				for k := 0; k < 8; k++ {
					buff[m] = byte(el & 0xff)
					m++
					if m >= olen || (m%H.rate) == 0 {
						done = true
						break
					}
					el >>= 8
				}
				if done {
					break
				}
			}
			if done {
				break
			}
		}
		if m >= olen {
			break
		}
		done = false
		H.transform()

	}
}

/* Generate Hash */
func (H *SHA3) Hash()  []byte  { /* generate a SHA3 hash of appropriate size */
	var digest [64]byte
	q := H.rate - int(H.length%uint64(H.rate))
	if q == 1 {
		H.Process(0x86)
	} else {
		H.Process(0x06)
		for int(H.length%uint64(H.rate)) != (H.rate - 1) {
			H.Process(0x00)
		}
		H.Process(0x80)
	}
	H.Squeeze(digest[:], H.len)
	return digest[0:H.len]
}

func (H *SHA3) Continuing_Hash() [] byte {
	sh := NewSHA3copy(H)
	return sh.Hash()
}

func (H *SHA3) Shake(hash []byte, olen int) { /* generate a SHA3 hash of appropriate size */
	q := H.rate - int(H.length%uint64(H.rate))
	if q == 1 {
		H.Process(0x9f)
	} else {
		H.Process(0x1f)
		for int(H.length%uint64(H.rate)) != H.rate-1 {
			H.Process(0x00)
		}
		H.Process(0x80)
	}
	H.Squeeze(hash, olen)
}

func (H *SHA3) Continuing_Shake(hash []byte, olen int) {
	sh := NewSHA3copy(H)
	sh.Shake(hash,olen)
}

/* test program: should produce digest */
//916f6061fe879741ca6469b43971dfdb28b1a32dc36cb3254e812be27aad1d18
//afebb2ef542e6579c50cad06d2e578f9f8dd6881d7dc824d26360feebf18a4fa73e3261122948efcfd492e74e82e2189ed0fb440d187f382270cb455f21dd185
//98be04516c04cc73593fef3ed0352ea9f6443942d6950e29a372a681c3deaf4535423709b02843948684e029010badcc0acd8303fc85fdad3eabf4f78cae165635f57afd28810fc2

/*
func main() {

	test := []byte("abcdefghbcdefghicdefghijdefghijkefghijklfghijklmghijklmnhijklmnoijklmnopjklmnopqklmnopqrlmnopqrsmnopqrstnopqrstu")
	var digest [172]byte

	sh:=NewSHA3(SHA3_HASH256)
	for i:=0;i<len(test);i++ {
		sh.Process(test[i])
	}
	sh.Hash(digest[:])
	for i:=0;i<32;i++ {fmt.Printf("%02x",digest[i])}
	fmt.Printf("\n");

	sh=NewSHA3(SHA3_HASH512)
	for i:=0;i<len(test);i++ {
		sh.Process(test[i])
	}
	sh.Hash(digest[:])
	for i:=0;i<64;i++ {fmt.Printf("%02x",digest[i])}
	fmt.Printf("\n");

	sh=NewSHA3(SHA3_SHAKE256)
	for i:=0;i<len(test);i++ {
		sh.Process(test[i])
	}
	sh.Shake(digest[:],72)
	for i:=0;i<72;i++ {fmt.Printf("%02x",digest[i])}
	fmt.Printf("\n");

} */
