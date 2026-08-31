// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

// Package bn254 efficient elliptic curve, pairing and hash to curve implementation for bn254. This curve appears in
// Ethereum pre-compiles as altbn128.
//
// bn254: A Barreto--Naerig curve with
//
//	seed x₀=4965661367192848881
//	𝔽r: r=21888242871839275222246405745257275088548364400416034343698204186575808495617 (36x₀⁴+36x₀³+18x₀²+6x₀+1)
//	𝔽p: p=21888242871839275222246405745257275088696311157297823662689037894645226208583 (36x₀⁴+36x₀³+24x₀²+6x₀+1)
//	(E/𝔽p): Y²=X³+3
//	(Eₜ/𝔽p²): Y² = X³+3/(u+9) (D-type twist)
//	r ∣ #E(Fp) and r ∣ #Eₜ(𝔽p²)
//
// Extension fields tower:
//
//	𝔽p²[u] = 𝔽p/u²+1
//	𝔽p⁶[v] = 𝔽p²/v³-9-u
//	𝔽p¹²[w] = 𝔽p⁶/w²-v
//
// optimal Ate loop size:
//
//	6x₀+2
//
// Security: estimated 103-bit level following [https://eprint.iacr.org/2019/885.pdf]
// (r is 254 bits and p¹² is 3044 bits)
//
// # Warning
//
// This code has been partially audited and is provided as-is. In particular, there is no security guarantees such as constant time implementation or side-channel attack resistance.
package bn254

import (
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fp"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/internal/fptower"
)

// ID bn254 ID
const ID = ecc.BN254

// aCurveCoeff is the a coefficients of the curve Y²=X³+ax+b
var aCurveCoeff fp.Element
var bCurveCoeff fp.Element

// twist
var twist fptower.E2

// bTwistCurveCoeff b coeff of the twist (defined over 𝔽p²) curve
var bTwistCurveCoeff fptower.E2

// generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
var g1Gen G1Jac
var g2Gen G2Jac

var g1GenAff G1Affine
var g2GenAff G2Affine

// point at infinity
var g1Infinity G1Jac
var g2Infinity G2Jac

// optimal Ate loop counter
var LoopCounter [66]int8

// Parameters useful for the GLV scalar multiplication. The third roots define the
// endomorphisms ϕ₁ and ϕ₂ for <G1Affine> and <G2Affine>. lambda is such that <r, ϕ-λ> lies above
// <r> in the ring Z[ϕ]. More concretely it's the associated eigenvalue
// of ϕ₁ (resp ϕ₂) restricted to <G1Affine> (resp <G2Affine>)
// see https://link.springer.com/content/pdf/10.1007/3-540-36492-7_3
var thirdRootOneG1 fp.Element
var thirdRootOneG2 fp.Element
var lambdaGLV big.Int

// glvBasis stores R-linearly independent vectors (a,b), (c,d)
// in ker((u,v) → u+vλ[r]), and their determinant
var glvBasis ecc.Lattice
var glsBasis ecc.Lattice4

// g1ScalarMulChoose and g2ScalarmulChoose indicate the bitlength of the scalar
// in scalar multiplication from which it is more efficient to use the GLV
// decomposition. It is computed from the GLV basis and considers the overhead
// for the GLV decomposition. It is heuristic and may change in the future.
var g1ScalarMulChoose, g2ScalarMulChoose int

// ψ o π o ψ⁻¹, where ψ:E → E' is the degree 6 iso defined over 𝔽p¹²
var endo struct {
	u fptower.E2
	v fptower.E2
}

// seed x₀ of the curve
var xGen big.Int

// expose the tower -- github.com/consensys/gnark uses it in a gnark circuit

// 𝔽p²
type E2 = fptower.E2

// 𝔽p⁶
type E6 = fptower.E6

// 𝔽p¹²
type E12 = fptower.E12

func init() {
	aCurveCoeff.SetUint64(0)
	bCurveCoeff.SetUint64(3)
	thirdRootOneG1.SetString("2203960485148121921418603742825762020974279258880205651966")
	thirdRootOneG2.Square(&thirdRootOneG1)
	// D-twist
	twist.A0.SetUint64(9)
	twist.A1.SetUint64(1)
	bTwistCurveCoeff.Inverse(&twist).MulByElement(&bTwistCurveCoeff, &bCurveCoeff)

	g1Gen.X.SetOne()
	g1Gen.Y.SetUint64(2)
	g1Gen.Z.SetOne()

	g2Gen.X.SetString("10857046999023057135944570762232829481370756359578518086990519993285655852781",
		"11559732032986387107991004021392285783925812861821192530917403151452391805634")
	g2Gen.Y.SetString("8495653923123431417604973247489272438418190587263600148770280649306958101930",
		"4082367875863433681332203403145435568316851327593401208105741076214120093531")
	g2Gen.Z.SetString("1",
		"0")

	g1GenAff.FromJacobian(&g1Gen)
	g2GenAff.FromJacobian(&g2Gen)

	// (X,Y,Z) = (1,1,0)
	g1Infinity.X.SetOne()
	g1Infinity.Y.SetOne()
	g2Infinity.X.SetOne()
	g2Infinity.Y.SetOne()

	lambdaGLV.SetString("4407920970296243842393367215006156084916469457145843978461", 10) // (36x₀³+18x₀²+6x₀+1)
	_r := fr.Modulus()
	ecc.PrecomputeLattice(_r, &lambdaGLV, &glvBasis)
	g1ScalarMulChoose = fr.Bits/16 + max(glvBasis.V1[0].BitLen(), glvBasis.V1[1].BitLen(), glvBasis.V2[0].BitLen(), glvBasis.V2[1].BitLen())
	g2ScalarMulChoose = fr.Bits/32 + max(glvBasis.V1[0].BitLen(), glvBasis.V1[1].BitLen(), glvBasis.V2[0].BitLen(), glvBasis.V2[1].BitLen())

	endo.u.A0.SetString("21575463638280843010398324269430826099269044274347216827212613867836435027261")
	endo.u.A1.SetString("10307601595873709700152284273816112264069230130616436755625194854815875713954")
	endo.v.A0.SetString("2821565182194536844548159561693502659359617185244120367078079554186484126554")
	endo.v.A1.SetString("3505843767911556378687030309984248845540243509899259641013678093033130930403")

	// 2-NAF decomposition of 6x₀+2 little endian
	optimaAteLoop, _ := new(big.Int).SetString("29793968203157093288", 10)
	ecc.NafDecomposition(optimaAteLoop, LoopCounter[:])

	xGen.SetString("4965661367192848881", 10)

	initGLSBasis()
}

func initGLSBasis() {
	// LLL-reduced basis (rows) from:
	//
	// 	 v1 = [r,                   0,          0,          0]
	// 	 v2 = [-lambdaGLV,   	    1,          0,          0]
	// 	 v3 = [-lambdaGLS,   	    0,          1,          0]
	// 	 v4 = [lambdaGLV*lambdaGLS, -lambdaGLS, -lambdaGLV, 1]
	//
	// to:
	//   v1 = [2x₀,    x₀,     x₀+1,   -x₀  ]
	//   v2 = [-x₀,    x₀,     x₀,     2x₀+1]
	//   v3 = [-x₀-1,  x₀,     -x₀,    -2x₀ ]
	//   v4 = [2x₀+1,  x₀+1,   -x₀,    x₀   ]
	setBasis := func(dst *big.Int, s string) {
		if _, ok := dst.SetString(s, 10); !ok {
			panic("invalid GLS basis constant")
		}
	}

	// v1 = (2x₀, x₀, x₀+1, -x₀)
	setBasis(&glsBasis.V[0][0], "9931322734385697762")
	setBasis(&glsBasis.V[0][1], "4965661367192848881")
	setBasis(&glsBasis.V[0][2], "4965661367192848882")
	setBasis(&glsBasis.V[0][3], "-4965661367192848881")

	// v2 = (-x₀, x₀, x₀, 2x₀+1)
	setBasis(&glsBasis.V[1][0], "-4965661367192848881")
	setBasis(&glsBasis.V[1][1], "4965661367192848881")
	setBasis(&glsBasis.V[1][2], "4965661367192848881")
	setBasis(&glsBasis.V[1][3], "9931322734385697763")

	// v3 = (-x₀-1, x₀, -x₀, -2x₀)
	setBasis(&glsBasis.V[2][0], "-4965661367192848882")
	setBasis(&glsBasis.V[2][1], "4965661367192848881")
	setBasis(&glsBasis.V[2][2], "-4965661367192848881")
	setBasis(&glsBasis.V[2][3], "-9931322734385697762")

	// v4 = (2x₀+1, x₀+1, -x₀, x₀)
	setBasis(&glsBasis.V[3][0], "9931322734385697763")
	setBasis(&glsBasis.V[3][1], "4965661367192848882")
	setBasis(&glsBasis.V[3][2], "-4965661367192848881")
	setBasis(&glsBasis.V[3][3], "4965661367192848881")

	ecc.PrecomputeLattice4(&glsBasis)
}

// Generators return the generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
func Generators() (g1Jac G1Jac, g2Jac G2Jac, g1Aff G1Affine, g2Aff G2Affine) {
	g1Aff = g1GenAff
	g2Aff = g2GenAff
	g1Jac = g1Gen
	g2Jac = g2Gen
	return
}

// CurveCoefficients returns the a, b coefficients of the curve equation.
func CurveCoefficients() (a, b fp.Element) {
	return aCurveCoeff, bCurveCoeff
}
