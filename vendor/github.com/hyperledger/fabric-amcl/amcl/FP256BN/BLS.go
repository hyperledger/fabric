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

/* Boneh-Lynn-Shacham  API Functions */

package FP256BN

import "github.com/hyperledger/fabric-amcl/amcl"



const BFS int = int(MODBYTES)
const BGS int = int(MODBYTES)
const BLS_OK int = 0
const BLS_FAIL int = -1

/* hash a message to an ECP point, using SHA3 */

func Bls_hash(m string) *ECP {
	sh := amcl.NewSHA3(amcl.SHA3_SHAKE256)
	var hm [BFS]byte
	t := []byte(m)
	for i := 0; i < len(t); i++ {
		sh.Process(t[i])
	}
	sh.Shake(hm[:], BFS)
	P := ECP_mapit(hm[:])
	return P
}

/* generate key pair, private key S, public key W */

func KeyPairGenerate(rng *amcl.RAND, S []byte, W []byte) int {
	G := ECP2_generator()
	q := NewBIGints(CURVE_Order)
	s := Randomnum(q, rng)
	s.ToBytes(S)
	G = G2mul(G, s)
	G.ToBytes(W)
	return BLS_OK
}

/* Sign message m using private key S to produce signature SIG */

func Sign(SIG []byte, m string, S []byte) int {
	D := Bls_hash(m)
	s := FromBytes(S)
	D = G1mul(D, s)
	D.ToBytes(SIG, true)
	return BLS_OK
}

/* Verify signature given message m, the signature SIG, and the public key W */

func Verify(SIG []byte, m string, W []byte) int {
	HM := Bls_hash(m)
	D := ECP_fromBytes(SIG)
	G := ECP2_generator()
	PK := ECP2_fromBytes(W)
	D.neg()

	// Use new multi-pairing mechanism
	r := initmp()
	another(r, G, D)
	another(r, PK, HM)
	v := miller(r)

	//.. or alternatively
	//	v := Ate2(G, D, PK, HM)

	v = Fexp(v)
	if v.Isunity() {
		return BLS_OK
	} else {
		return BLS_FAIL
	}
}
