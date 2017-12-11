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

/* Test and benchmark elliptic curve and RSA functions */

package main

import "fmt"
import "amcl"
import "amcl/BN254"
import "amcl/ED25519"
import "amcl/RSA2048"

import "time"

const MIN_TIME int=10
const MIN_ITERS int=10

func main() {
	var RAW [100]byte

	rng:=amcl.NewRAND()

	rng.Clean();
	for i:=0;i<100;i++ {RAW[i]=byte(i)}

	rng.Seed(100,RAW[:])

	fmt.Printf("Testing/Timing ED25519 ECC\n")

	if ED25519.CURVETYPE==ED25519.WEIERSTRASS {
		fmt.Printf("Weierstrass parameterization\n")
	}		
	if ED25519.CURVETYPE==ED25519.EDWARDS {
		fmt.Printf("Edwards parameterization\n")
	}
	if ED25519.CURVETYPE==ED25519.MONTGOMERY {
		fmt.Printf("Montgomery parameterization\n")
	}

	if ED25519.MODTYPE==ED25519.PSEUDO_MERSENNE {
		fmt.Printf("Pseudo-Mersenne Modulus\n")
	}
	if ED25519.MODTYPE==ED25519.MONTGOMERY_FRIENDLY {
		fmt.Printf("Montgomery friendly Modulus\n")
	}
	if ED25519.MODTYPE==ED25519.GENERALISED_MERSENNE {
		fmt.Printf("Generalised-Mersenne Modulus\n")
	}
	if ED25519.MODTYPE==ED25519.NOT_SPECIAL {
		fmt.Printf("Not special Modulus\n")
	}

	fmt.Printf("Modulus size %d bits\n",ED25519.MODBITS)
	fmt.Printf("%d bit build\n",ED25519.CHUNK)

	var es *ED25519.BIG
	var EG *ED25519.ECP

	gx:=ED25519.NewBIGints(ED25519.CURVE_Gx)
	if ED25519.CURVETYPE!=ED25519.MONTGOMERY {
		gy:=ED25519.NewBIGints(ED25519.CURVE_Gy)
		EG=ED25519.NewECPbigs(gx,gy)
	} else {
		EG=ED25519.NewECPbig(gx)
	}

	er:=ED25519.NewBIGints(ED25519.CURVE_Order)
	es=ED25519.Randomnum(er,rng)

	WP:=EG.Mul(er)
	if !WP.Is_infinity() {
		fmt.Printf("FAILURE - rG!=O\n")
		return
	}

	start := time.Now()
	iterations:=0
	elapsed:=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		WP=EG.Mul(es)
		iterations++
		elapsed=time.Since(start)
	} 
	dur:=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("EC  mul - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)



	fmt.Printf("\nTesting/Timing BN254 Pairings\n")

	if BN254.CURVE_PAIRING_TYPE==BN254.BN {
		fmt.Printf("BN Pairing-Friendly Curve\n")
	}
	if BN254.CURVE_PAIRING_TYPE==BN254.BLS {
		fmt.Printf("BLS Pairing-Friendly Curve\n")
	}

	fmt.Printf("Modulus size %d bits\n",BN254.MODBITS)
	fmt.Printf("%d bit build\n",BN254.CHUNK)

	G:=BN254.NewECPbigs(BN254.NewBIGints(BN254.CURVE_Gx),BN254.NewBIGints(BN254.CURVE_Gy))
	r:=BN254.NewBIGints(BN254.CURVE_Order)
	s:=BN254.Randomnum(r,rng)

	P:=BN254.G1mul(G,r)

	if !P.Is_infinity() {
		fmt.Printf("FAILURE - rP!=O\n");
		return;
	}

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		P=BN254.G1mul(G,s)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("G1 mul              - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	Q:=BN254.NewECP2fp2s(BN254.NewFP2bigs(BN254.NewBIGints(BN254.CURVE_Pxa),BN254.NewBIGints(BN254.CURVE_Pxb)),BN254.NewFP2bigs(BN254.NewBIGints(BN254.CURVE_Pya),BN254.NewBIGints(BN254.CURVE_Pyb)))
	W:=BN254.G2mul(Q,r)

	if !W.Is_infinity() {
		fmt.Printf("FAILURE - rQ!=O\n");
		return;
	}

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		W=BN254.G2mul(Q,s)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("G2 mul              - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	w:=BN254.Ate(Q,P)
	w=BN254.Fexp(w)

	g:=BN254.GTpow(w,r)

	if !g.Isunity() {
		fmt.Printf("FAILURE - g^r!=1\n");
		return;
	}

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		g=BN254.GTpow(w,s)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("GT pow              - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		_=w.Compow(s,r)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("GT pow (compressed) - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		w=BN254.Ate(Q,P)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("PAIRing ATE         - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		g=BN254.Fexp(w)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("PAIRing FEXP        - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	P.Copy(G)
	Q.Copy(W)

	P=BN254.G1mul(P,s)

	g=BN254.Ate(Q,P)
	g=BN254.Fexp(g)

	P.Copy(G)
	Q=BN254.G2mul(Q,s)

	w=BN254.Ate(Q,P)
	w=BN254.Fexp(w)

	if !g.Equals(w) {
		fmt.Printf("FAILURE - e(sQ,p)!=e(Q,sP) \n")
		return
	}

	Q.Copy(W);
	g=BN254.Ate(Q,P)
	g=BN254.Fexp(g)
	g=BN254.GTpow(g,s)

	if !g.Equals(w) {
		fmt.Printf("FAILURE - e(sQ,p)!=e(Q,P)^s \n")
		return
	}



	fmt.Printf("\nTesting/Timing 2048-bit RSA\n")

	pub:=RSA2048.New_public_key(RSA2048.FFLEN)
	priv:=RSA2048.New_private_key(RSA2048.HFLEN)

	var PT [RSA2048.RFS]byte
	var M [RSA2048.RFS]byte
	var CT [RSA2048.RFS]byte

	fmt.Printf("Generating 2048-bit RSA public/private key pair\n");

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		RSA2048.RSA_KEY_PAIR(rng,65537,priv,pub)
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("RSA gen - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	for i:=0;i<RSA2048.RFS;i++ {M[i]=byte(i%128)};

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		RSA2048.RSA_ENCRYPT(pub,M[:],CT[:])
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("RSA enc - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	start = time.Now()
	iterations=0
	elapsed=time.Since(start)
	for (int(elapsed/time.Second))<MIN_TIME || iterations<MIN_ITERS {
		RSA2048.RSA_DECRYPT(priv,CT[:],PT[:])
		iterations++
		elapsed=time.Since(start)
	} 
	dur=float64(elapsed/time.Millisecond)/float64(iterations)
	fmt.Printf("RSA dec - %8d iterations  ",iterations)
	fmt.Printf(" %8.2f ms per iteration\n",dur)

	for i:=0;i<RSA2048.RFS;i++ {
		if (PT[i]!=M[i]) {
			fmt.Printf("FAILURE - RSA decryption\n")
			return
		}
	}

	fmt.Printf("All tests pass\n") 
}
