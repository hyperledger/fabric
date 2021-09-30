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

/* MPIN 128-bit API Functions */

package FP256BN

import "github.com/hyperledger/fabric-amcl/core"


const MFS int = int(MODBYTES)
const MGS int = int(MODBYTES)
const BAD_PARAMS int = -11
const INVALID_POINT int = -14
const WRONG_ORDER int = -18
const BAD_PIN int = -19

/* Configure your PIN here */

const MAXPIN int32 = 10000 /* PIN less than this */
const PBLEN int32 = 14     /* Number of bits in PIN */

func MPIN_HASH_ID(sha int, ID []byte) []byte {
	return core.GPhashit(core.MC_SHA2,sha,int(MODBYTES),0,nil,-1,ID)
	//return mhashit(sha, 0, ID)
}

func roundup(a int,b int) int {
    return (((a)-1)/(b)+1)
}

func MPIN_ENCODE_TO_CURVE(DST []byte,ID []byte,HCID []byte) {
	q := NewBIGints(Modulus)
	k := q.Nbits()
	r := NewBIGints(CURVE_Order)
	m := r.Nbits();
	L:= roundup(k+roundup(m,2),8)
	var fd=make([]byte,L)
	OKM:=core.XMD_Expand(core.MC_SHA2,HASH_TYPE,L,DST,ID)
  
	for j:=0;j<L;j++ {
		fd[j]=OKM[j];
	}
	dx:=DBIG_fromBytes(fd)
	u:=NewFPbig(dx.Mod(q))
	P:=ECP_map2point(u)

	P.Cfp()
	P.Affine()	
	P.ToBytes(HCID, false)   
}

/* create random secret S */
func MPIN_RANDOM_GENERATE(rng *core.RAND, S []byte) int {
	r := NewBIGints(CURVE_Order)
	s := Randtrunc(r, 16*AESKEY, rng)
	s.ToBytes(S)
	return 0
}

func MPIN_EXTRACT_PIN(CID []byte, pin int, TOKEN []byte) int {
	P := ECP_fromBytes(TOKEN)
	if P.Is_infinity() {
		return INVALID_POINT
	}
	R := ECP_fromBytes(CID)
	if R.Is_infinity() {
		return INVALID_POINT
	}
	R = R.pinmul(int32(pin)%MAXPIN, PBLEN)
	P.Sub(R)
	P.ToBytes(TOKEN, false)
	return 0
}

/* Implement step 2 on client side of MPin protocol */
func MPIN_CLIENT_2(X []byte, Y []byte, SEC []byte) int {
	r := NewBIGints(CURVE_Order)
	P := ECP_fromBytes(SEC)
	if P.Is_infinity() {
		return INVALID_POINT
	}

	px := FromBytes(X)
	py := FromBytes(Y)
	px.add(py)
	px.Mod(r)

	P = G1mul(P, px)
	P.Neg()
	P.ToBytes(SEC, false)
	return 0
}

func MPIN_GET_CLIENT_SECRET(S []byte, IDHTC []byte, CST []byte) int {
	s := FromBytes(S)
	P := ECP_fromBytes(IDHTC)
	if P.Is_infinity() {
		return INVALID_POINT
	}
	G1mul(P, s).ToBytes(CST, false)
	return 0
}

/* Implement step 1 on client side of MPin protocol */
func MPIN_CLIENT_1(CID []byte, rng *core.RAND, X []byte, pin int, TOKEN []byte, SEC []byte, xID []byte) int {
	r := NewBIGints(CURVE_Order)
	var x *BIG
	if rng != nil {
		x = Randtrunc(r, 16*AESKEY, rng)
		x.ToBytes(X)
	} else {
		x = FromBytes(X)
	}

	P := ECP_fromBytes(CID)
	if P.Is_infinity() {
		return INVALID_POINT
	}

	T := ECP_fromBytes(TOKEN)
	if T.Is_infinity() {
		return INVALID_POINT
	}

	W := P.pinmul(int32(pin)%MAXPIN, PBLEN)
	T.Add(W)

	P = G1mul(P, x)
	P.ToBytes(xID, false)

	T.ToBytes(SEC, false)
	return 0
}

/* Extract Server Secret SST=S*Q where Q is fixed generator in G2 and S is master secret */
func MPIN_GET_SERVER_SECRET(S []byte, SST []byte) int {
	Q := ECP2_generator()
	s := FromBytes(S)
	Q = G2mul(Q, s)
	Q.ToBytes(SST,false)
	return 0
}

/* Implement step 2 of MPin protocol on server side */
func MPIN_SERVER(HID []byte, Y []byte, SST []byte, xID []byte, mSEC []byte) int {
	Q := ECP2_generator()

	sQ := ECP2_fromBytes(SST)
	if sQ.Is_infinity() {
		return INVALID_POINT
	}

	if xID == nil {
		return BAD_PARAMS
	}
	R := ECP_fromBytes(xID)
	if R.Is_infinity() {
		return INVALID_POINT
	}
	y := FromBytes(Y)
	if HID == nil {
		return BAD_PARAMS
	}
	P := ECP_fromBytes(HID)
	if P.Is_infinity() {
		return INVALID_POINT
	}

	P = G1mul(P, y)
	P.Add(R)
	R = ECP_fromBytes(mSEC)
	if R.Is_infinity() {
		return INVALID_POINT
	}

	var g *FP12
	g = Ate2(Q, R, sQ, P)
	g = Fexp(g)

	if !g.Isunity() {
		return BAD_PIN
	}
	return 0
}


