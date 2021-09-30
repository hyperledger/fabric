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

/* ECDH/ECIES/ECDSA API Functions */

package FP256BN


import "github.com/hyperledger/fabric-amcl/core"

const INVALID_PUBLIC_KEY int = -2
const ERROR int = -3
//const INVALID int = -4
const EFS int = int(MODBYTES)
const EGS int = int(MODBYTES)

/* return true if S is in ranger 0 < S < order , else return false */
func ECDH_IN_RANGE(S []byte) bool {
	r := NewBIGints(CURVE_Order)
	s := FromBytes(S)
	if s.iszilch() {
		return false
	}
    if Comp(s,r)>=0 {
		return false
	}
	return true
}

/* Calculate a public/private EC GF(p) key pair W,S where W=S.G mod EC(p),
 * where S is the secret key and W is the public key
 * and G is fixed generator.
 * If RNG is NULL then the private key is provided externally in S
 * otherwise it is generated randomly internally */
func ECDH_KEY_PAIR_GENERATE(RNG *core.RAND, S []byte, W []byte) int {
	res := 0
	var s *BIG
	var G *ECP

	G = ECP_generator()

	r := NewBIGints(CURVE_Order)

	if RNG == nil {
		s = FromBytes(S)
		s.Mod(r)
	} else {
		s = Randtrunc(r, 16*AESKEY, RNG)
	}

	s.ToBytes(S)

	WP := G.mul(s)

	WP.ToBytes(W, false) // To use point compression on public keys, change to true

	return res
}

/* validate public key */
func ECDH_PUBLIC_KEY_VALIDATE(W []byte) int {
	WP := ECP_fromBytes(W)
	res := 0

	r := NewBIGints(CURVE_Order)

	if WP.Is_infinity() {
		res = INVALID_PUBLIC_KEY
	}
	if res == 0 {

		q := NewBIGints(Modulus)
		nb := q.nbits()
		k := NewBIGint(1)
		k.shl(uint((nb + 4) / 2))
		k.add(q)
		k.div(r)

		for k.parity() == 0 {
			k.shr(1)
			WP.dbl()
		}

		if !k.isunity() {
			WP = WP.mul(k)
		}
		if WP.Is_infinity() {
			res = INVALID_PUBLIC_KEY
		}

	}
	return res
}

/* IEEE-1363 Diffie-Hellman online calculation Z=S.WD */
// type = 0 is just x coordinate output
// type = 1 for standard compressed output
// type = 2 for standard uncompress output 04|x|y
func ECDH_ECPSVDP_DH(S []byte, WD []byte, Z []byte,typ int) int {
	res := 0

	s := FromBytes(S)

	W := ECP_fromBytes(WD)
	if W.Is_infinity() {
		res = ERROR
	}

	if res == 0 {
		r := NewBIGints(CURVE_Order)
		s.Mod(r)
		W = W.mul(s)
		if W.Is_infinity() {
			res = ERROR
		} else {
			if CURVETYPE != MONTGOMERY {
				if typ>0 {
					if typ==1 {
						W.ToBytes(Z,true)
					} else {
						W.ToBytes(Z,false)
					}
				} else {
					W.GetX().ToBytes(Z)
				}
				return res;
			} else {
				W.GetX().ToBytes(Z)
			}
		}
	}
	return res
}

/* IEEE ECDSA Signature, C and D are signature on F using private key S */
func ECDH_ECPSP_DSA(sha int, RNG *core.RAND, S []byte, F []byte, C []byte, D []byte) int {
	var T [EFS]byte

	B := core.GPhashit(core.MC_SHA2, sha, int(MODBYTES), 0, F, -1, nil )
	G := ECP_generator()

	r := NewBIGints(CURVE_Order)

	s := FromBytes(S)
	f := FromBytes(B[:])

	c := NewBIGint(0)
	d := NewBIGint(0)
	V := NewECP()

	for d.iszilch() {
		u := Randomnum(r, RNG)
		w := Randomnum(r, RNG) /* side channel masking */
		V.Copy(G)
		V = V.mul(u)
		vx := V.GetX()
		c.copy(vx)
		c.Mod(r)
		if c.iszilch() {
			continue
		}
		u.copy(Modmul(u, w, r))
		u.Invmodp(r)
		d.copy(Modmul(s, c, r))
		d.add(f)
		d.copy(Modmul(d, w, r))
		d.copy(Modmul(u, d, r))
	}

	c.ToBytes(T[:])
	for i := 0; i < EFS; i++ {
		C[i] = T[i]
	}
	d.ToBytes(T[:])
	for i := 0; i < EFS; i++ {
		D[i] = T[i]
	}
	return 0
}

/* IEEE1363 ECDSA Signature Verification. Signature C and D on F is verified using public key W */
func ECDH_ECPVP_DSA(sha int, W []byte, F []byte, C []byte, D []byte) int {
	res := 0

	B := core.GPhashit(core.MC_SHA2, sha, int(MODBYTES), 0, F, -1, nil )

	G := ECP_generator()
	r := NewBIGints(CURVE_Order)

	c := FromBytes(C)
	d := FromBytes(D)
	f := FromBytes(B[:])

	if c.iszilch() || Comp(c, r) >= 0 || d.iszilch() || Comp(d, r) >= 0 {
		res = ERROR
	}

	if res == 0 {
		d.Invmodp(r)
		f.copy(Modmul(f, d, r))
		h2 := Modmul(c, d, r)

		WP := ECP_fromBytes(W)
		if WP.Is_infinity() {
			res = ERROR
		} else {
			P := NewECP()
			P.Copy(WP)

			P = P.Mul2(h2, G, f)

			if P.Is_infinity() {
				res = ERROR
			} else {
				d = P.GetX()
				d.Mod(r)

				if Comp(d, c) != 0 {
					res = ERROR
				}
			}
		}
	}

	return res
}

/* IEEE1363 ECIES encryption. Encryption of plaintext M uses public key W and produces ciphertext V,C,T */
func ECDH_ECIES_ENCRYPT(sha int, P1 []byte, P2 []byte, RNG *core.RAND, W []byte, M []byte, V []byte, T []byte) []byte {
	var Z [EFS]byte
	var VZ [3*EFS + 1]byte
	var K1 [AESKEY]byte
	var K2 [AESKEY]byte
	var U [EGS]byte

	if ECDH_KEY_PAIR_GENERATE(RNG, U[:], V) != 0 {
		return nil
	}
	if ECDH_ECPSVDP_DH(U[:], W, Z[:], 0) != 0 {
		return nil
	}

	for i := 0; i < 2*EFS+1; i++ {
		VZ[i] = V[i]
	}
	for i := 0; i < EFS; i++ {
		VZ[2*EFS+1+i] = Z[i]
	}

	K := core.KDF2(core.MC_SHA2, sha, VZ[:], P1, 2*AESKEY)

	for i := 0; i < AESKEY; i++ {
		K1[i] = K[i]
		K2[i] = K[AESKEY+i]
	}

	C := core.AES_CBC_IV0_ENCRYPT(K1[:], M)

	L2 := core.InttoBytes(len(P2), 8)

	var AC []byte

	for i := 0; i < len(C); i++ {
		AC = append(AC, C[i])
	}
	for i := 0; i < len(P2); i++ {
		AC = append(AC, P2[i])
	}
	for i := 0; i < 8; i++ {
		AC = append(AC, L2[i])
	}

	core.HMAC(core.MC_SHA2, sha, T, len(T), K2[:], AC)

	return C
}

/* constant time n-byte compare */
func ncomp(T1 []byte, T2 []byte, n int) bool {
	res := 0
	for i := 0; i < n; i++ {
		res |= int(T1[i] ^ T2[i])
	}
	if res == 0 {
		return true
	}
	return false
}

/* IEEE1363 ECIES decryption. Decryption of ciphertext V,C,T using private key U outputs plaintext M */
func ECDH_ECIES_DECRYPT(sha int, P1 []byte, P2 []byte, V []byte, C []byte, T []byte, U []byte) []byte {
	var Z [EFS]byte
	var VZ [3*EFS + 1]byte
	var K1 [AESKEY]byte
	var K2 [AESKEY]byte

	var TAG []byte = T[:]

	if ECDH_ECPSVDP_DH(U, V, Z[:], 0) != 0 {
		return nil
	}

	for i := 0; i < 2*EFS+1; i++ {
		VZ[i] = V[i]
	}
	for i := 0; i < EFS; i++ {
		VZ[2*EFS+1+i] = Z[i]
	}

	K := core.KDF2(core.MC_SHA2, sha, VZ[:], P1, 2*AESKEY)

	for i := 0; i < AESKEY; i++ {
		K1[i] = K[i]
		K2[i] = K[AESKEY+i]
	}

	M := core.AES_CBC_IV0_DECRYPT(K1[:], C)

	if M == nil {
		return nil
	}

	L2 := core.InttoBytes(len(P2), 8)

	var AC []byte

	for i := 0; i < len(C); i++ {
		AC = append(AC, C[i])
	}
	for i := 0; i < len(P2); i++ {
		AC = append(AC, P2[i])
	}
	for i := 0; i < 8; i++ {
		AC = append(AC, L2[i])
	}

	core.HMAC(core.MC_SHA2, sha, TAG, len(TAG), K2[:],AC)

	if !ncomp(T, TAG, len(T)) {
		return nil
	}

	return M
}
