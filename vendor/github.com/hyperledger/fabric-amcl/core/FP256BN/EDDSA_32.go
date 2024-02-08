//go:build 386 || arm

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
 *	 http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/* EDDSA API Functions */

package FP256BN

import "github.com/hyperledger/fabric-amcl/core"

const EDDSA_INVALID_PUBLIC_KEY int = -2

//const EDDSA_ERROR int = -3

// Transform a point multiplier to rfc7748 form
func rfc7748(r *BIG) {
	lg := 0
	t := NewBIGint(1)
	c := CURVE_Cof_I
	for c != 1 {
		lg++
		c /= 2
	}
	n := uint(8*EGS - lg + 1)
	r.mod2m(n)
	t.shl(n)
	r.add(t)
	c = r.lastbits(lg)
	r.dec(c)
}

// reverse first n bytes of buff - for little endian
func eddsa_reverse(n int, buff []byte) {
	for i := 0; i < n/2; i++ {
		ch := buff[i]
		buff[i] = buff[n-i-1]
		buff[n-i-1] = ch
	}
}

// dom - domain function
func dom(pp string, ph bool, cl byte) []byte {
	PP := []byte(pp)
	n := len(PP)
	dom := make([]byte, n+2)
	for i := 0; i < n; i++ {
		dom[i] = PP[i]
	}
	if ph {
		dom[n] = 1
	} else {
		dom[n] = 0
	}
	dom[n+1] = cl
	return dom
}

func h(S []byte) []byte {
	n := len(S)
	if AESKEY <= 16 { // for ed25519?
		sh := core.NewHASH512()
		for i := 0; i < n; i++ {
			sh.Process(S[i])
		}
		return sh.Hash()
	} else { // for ed448?
		digest := make([]byte, 2*n)
		sh := core.NewSHA3(core.SHA3_SHAKE256)
		for i := 0; i < n; i++ {
			sh.Process(S[i])
		}
		sh.Shake(digest[:], 2*n)
		return digest
	}
}

func h2(ph bool, ctx []byte, R []byte, Q []byte, M []byte) *DBIG {
	b := len(Q)
	cl := 0
	if ctx != nil {
		cl = len(ctx)
	}
	if AESKEY <= 16 { // Ed25519??
		sh := core.NewHASH512()
		if ph || cl > 0 { // if not prehash and no context, omit dom2()
			domain := dom("SigFP256BN no FP256BN collisions", ph, byte(cl))
			for i := 0; i < len(domain); i++ {
				sh.Process(domain[i])
			}
			for i := 0; i < int(cl); i++ {
				sh.Process(ctx[i])
			}
		}
		for i := 0; i < b; i++ {
			sh.Process(R[i])
		}
		for i := 0; i < b; i++ {
			sh.Process(Q[i])
		}
		for i := 0; i < len(M); i++ {
			sh.Process(M[i])
		}
		h := sh.Hash()
		eddsa_reverse(64, h)
		return DBIG_fromBytes(h)
	} else { // for ed448?
		domain := dom("SigFP256BN", ph, byte(cl))
		h := make([]byte, 2*b)
		sh := core.NewSHA3(core.SHA3_SHAKE256)
		for i := 0; i < len(domain); i++ {
			sh.Process(domain[i])
		}
		for i := 0; i < cl; i++ {
			sh.Process(ctx[i])
		}
		for i := 0; i < b; i++ {
			sh.Process(R[i])
		}
		for i := 0; i < b; i++ {
			sh.Process(Q[i])
		}
		for i := 0; i < len(M); i++ {
			sh.Process(M[i])
		}
		sh.Shake(h, 2*b)
		eddsa_reverse(2*b, h)
		return DBIG_fromBytes(h)
	}
}

func getR(ph bool, b int, digest []byte, ctx []byte, M []byte) *DBIG {
	cl := 0
	if ctx != nil {
		cl = len(ctx)
	}
	if AESKEY <= 16 { // Ed25519??
		sh := core.NewHASH512()
		if ph || cl > 0 { // if not prehash and no context, omit dom2()
			domain := dom("SigFP256BN no FP256BN collisions", ph, byte(cl))
			for i := 0; i < len(domain); i++ {
				sh.Process(domain[i])
			}
			for i := 0; i < cl; i++ {
				sh.Process(ctx[i])
			}
		}
		for i := b; i < 2*b; i++ {
			sh.Process(digest[i])
		}
		for i := 0; i < len(M); i++ {
			sh.Process(M[i])
		}
		h := sh.Hash()
		eddsa_reverse(64, h)
		return DBIG_fromBytes(h)
	} else { // for ed448?
		domain := dom("SigFP256BN", ph, byte(cl))
		sh := core.NewSHA3(core.SHA3_SHAKE256)
		h := make([]byte, 2*b)
		for i := 0; i < len(domain); i++ {
			sh.Process(domain[i])
		}
		for i := 0; i < cl; i++ {
			sh.Process(ctx[i])
		}
		for i := b; i < 2*b; i++ {
			sh.Process(digest[i])
		}
		for i := 0; i < len(M); i++ {
			sh.Process(M[i])
		}
		sh.Shake(h, 2*b)
		eddsa_reverse(2*b, h)
		return DBIG_fromBytes(h)
	}
}

// encode integer (little endian)
func encode_int(x *BIG, w []byte) int {
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index

	w[0] = 0
	x.tobytearray(w, index)
	eddsa_reverse(b, w)
	return b
}

// encode point
func encode(P *ECP, w []byte) {
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index

	x := P.GetX()
	y := P.GetY()
	encode_int(y, w)
	w[b-1] |= byte(x.parity() << 7)
}

// get sign
func getsign(x []byte) int {
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index
	if (x[b-1] & 0x80) != 0 {
		return 1
	} else {
		return 0
	}
}

// decode integer (little endian)
func decode_int(strip_sign bool, ei []byte) *BIG {
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index

	r := make([]byte, b)
	for i := 0; i < b; i++ {
		r[i] = ei[i]
	}
	eddsa_reverse(b, r)
	if strip_sign {
		r[0] &= 0x7f
	}
	return BIG_frombytearray(r, index)
}

// decode compressed point
func decode(W []byte) *ECP {
	sign := getsign(W) // lsb of x
	y := decode_int(true, W)
	one := NewFPint(1)
	hint := NewFP()
	x := NewFPbig(y)
	x.sqr()
	d := NewFPcopy(x)
	x.sub(one)
	x.norm()
	t := NewFPbig(NewBIGints(CURVE_B))
	d.mul(t)
	if CURVE_A == 1 {
		d.sub(one)
	}
	if CURVE_A == -1 {
		d.add(one)
	}
	d.norm()
	// inverse square root trick for sqrt(x/d)
	t.copy(x)
	t.sqr()
	x.mul(t)
	x.mul(d)
	if x.qr(hint) != 1 {
		return NewECP()
	}
	d.copy(x.sqrt(hint))
	x.inverse(hint)
	x.mul(d)
	x.mul(t)
	x.reduce()
	if x.redc().parity() != sign {
		x.neg()
	}
	x.norm()
	return NewECPbigs(x.redc(), y)
}

/* Calculate a public/private EC GF(p) key pair. Q=D.G mod EC(p),
 * where D is the secret key and Q is the public key
 * and G is fixed generator.
 * RNG is a cryptographically strong RNG
 * If RNG==NULL, D is provided externally
 */
func KEY_PAIR_GENERATE(RNG *core.RAND, D []byte, Q []byte) int {
	res := 0
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index

	G := ECP_generator()

	if RNG != nil {
		for i := 0; i < b; i++ {
			D[i] = byte(RNG.GetByte())
		}
	}
	digest := h(D)

	// reverse bytes for little endian
	eddsa_reverse(b, digest)
	s := BIG_frombytearray(digest, index)
	rfc7748(s)
	G.Copy(G.mul(s))
	encode(G, Q)
	return res
}

// Generate a signature using key pair (D,Q) on message M
// Set ph=true if message has already been pre-hashed
// if ph=false, then context should be NULL for ed25519. However RFC8032 mode ed25519ctx is supported by supplying a non-NULL or non-empty context
func SIGNATURE(ph bool, D []byte, ctx []byte, M []byte, SIG []byte) int {
	digest := h(D) // hash of private key
	res := 0
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index
	S := make([]byte, b)
	Q := make([]byte, b)
	KEY_PAIR_GENERATE(nil, D, Q[:])

	q := NewBIGints(CURVE_Order)
	if len(D) != len(Q) || len(D) != b {
		res = EDDSA_INVALID_PUBLIC_KEY
	}
	if res == 0 {
		dr := getR(ph, b, digest, ctx, M)
		sr := dr.Mod(q)
		R := ECP_generator().mul(sr)
		encode(R, S[:])
		for i := 0; i < b; i++ {
			SIG[i] = S[i]
		}
		// reverse bytes for little endian
		eddsa_reverse(b, digest)
		s := BIG_frombytearray(digest, index)
		RFC7748(s)
		dr = h2(ph, ctx, SIG, Q, M)
		sd := dr.Mod(q)
		encode_int(Modadd(sr, Modmul(s, sd, q), q), S)
		for i := 0; i < b; i++ {
			SIG[b+i] = S[i]
		}
	}
	return res
}

func VERIFY(ph bool, Q []byte, ctx []byte, M []byte, SIG []byte) bool {
	lg := 0
	index := 0
	if 8*MODBYTES == MODBITS {
		index = 1 // extra byte needed for compression
	}
	b := int(MODBYTES) + index
	S := make([]byte, b)
	c := CURVE_Cof_I
	for c != 1 {
		lg++
		c /= 2
	}
	q := NewBIGints(CURVE_Order)
	R := decode(SIG)
	if R.Is_infinity() {
		return false
	}
	for i := 0; i < b; i++ {
		S[i] = SIG[b+i]
	}
	t := decode_int(false, S)
	if Comp(t, q) >= 0 {
		return false
	}
	du := h2(ph, ctx, SIG, Q, M)
	su := du.Mod(q)

	G := ECP_generator()
	QD := decode(Q)
	if QD.Is_infinity() {
		return false
	}
	QD.Neg()
	for i := 0; i < lg; i++ { // use cofactor 2^c
		G.dbl()
		QD.dbl()
		R.dbl()
	}

	if !G.Mul2(t, QD, su).Equals(R) {
		return false
	}
	return true
}
