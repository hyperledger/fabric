package bn254

import (
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fp"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/internal/fptower"
)

// E: y**2=x**3+3
// Etwist: y**2 = x**3+3*(u+9)**-1
// Tower: Fp->Fp2, u**2=-1 -> Fp12, v**6=9+u
// Generator (BN family): x=4965661367192848881
// optimal Ate loop: 6x+2
// trace of pi: x+1
// Fp: p=21888242871839275222246405745257275088696311157297823662689037894645226208583
// Fr: r=21888242871839275222246405745257275088548364400416034343698204186575808495617

// ID bn254 ID
const ID = ecc.BN254

// bCurveCoeff b coeff of the curve
var bCurveCoeff fp.Element

// twist
var twist fptower.E2

// bTwistCurveCoeff b coeff of the twist (defined over Fp2) curve
var bTwistCurveCoeff fptower.E2

// twoInv 1/2 mod p (needed for DoubleStep in Miller loop)
var twoInv fp.Element

// generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
var g1Gen G1Jac
var g2Gen G2Jac

var g1GenAff G1Affine
var g2GenAff G2Affine

// point at infinity
var g1Infinity G1Jac
var g2Infinity G2Jac

// optimal Ate loop counter
var loopCounter [66]int8

// Parameters useful for the GLV scalar multiplication. The third roots define the
//  endomorphisms phi1 and phi2 for <G1Affine> and <G2Affine>. lambda is such that <r, phi-lambda> lies above
// <r> in the ring Z[phi]. More concretely it's the associated eigenvalue
// of phi1 (resp phi2) restricted to <G1Affine> (resp <G2Affine>)
// cf https://www.cosic.esat.kuleuven.be/nessie/reports/phase2/GLV.pdf
var thirdRootOneG1 fp.Element
var thirdRootOneG2 fp.Element
var lambdaGLV big.Int

// glvBasis stores R-linearly independant vectors (a,b), (c,d)
// in ker((u,v)->u+vlambda[r]), and their determinant
var glvBasis ecc.Lattice

// psi o pi o psi**-1, where psi:E->E' is the degree 6 iso defined over Fp12
var endo struct {
	u fptower.E2
	v fptower.E2
}

// generator of the curve
var xGen big.Int

func init() {

	bCurveCoeff.SetUint64(3)
	twist.A0.SetUint64(9)
	twist.A1.SetUint64(1)
	bTwistCurveCoeff.Inverse(&twist).MulByElement(&bTwistCurveCoeff, &bCurveCoeff)

	twoInv.SetOne().Double(&twoInv).Inverse(&twoInv)

	g1Gen.X.SetString("1")
	g1Gen.Y.SetString("2")
	g1Gen.Z.SetString("1")

	g2Gen.X.SetString("10857046999023057135944570762232829481370756359578518086990519993285655852781",
		"11559732032986387107991004021392285783925812861821192530917403151452391805634")
	g2Gen.Y.SetString("8495653923123431417604973247489272438418190587263600148770280649306958101930",
		"4082367875863433681332203403145435568316851327593401208105741076214120093531")
	g2Gen.Z.SetString("1",
		"0")

	g1GenAff.FromJacobian(&g1Gen)
	g2GenAff.FromJacobian(&g2Gen)

	g1Infinity.X.SetOne()
	g1Infinity.Y.SetOne()
	g2Infinity.X.SetOne()
	g2Infinity.Y.SetOne()

	thirdRootOneG1.SetString("2203960485148121921418603742825762020974279258880205651966")
	thirdRootOneG2.Square(&thirdRootOneG1)
	lambdaGLV.SetString("4407920970296243842393367215006156084916469457145843978461", 10) // (36*x**3+18*x**2+6*x+1)
	_r := fr.Modulus()
	ecc.PrecomputeLattice(_r, &lambdaGLV, &glvBasis)

	endo.u.A0.SetString("21575463638280843010398324269430826099269044274347216827212613867836435027261")
	endo.u.A1.SetString("10307601595873709700152284273816112264069230130616436755625194854815875713954")
	endo.v.A0.SetString("2821565182194536844548159561693502659359617185244120367078079554186484126554")
	endo.v.A1.SetString("3505843767911556378687030309984248845540243509899259641013678093033130930403")

	// binary decomposition of 15132376222941642752 little endian
	optimaAteLoop, _ := new(big.Int).SetString("29793968203157093288", 10)
	ecc.NafDecomposition(optimaAteLoop, loopCounter[:])

	xGen.SetString("4965661367192848881", 10)

}

// Generators return the generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
func Generators() (g1Jac G1Jac, g2Jac G2Jac, g1Aff G1Affine, g2Aff G2Affine) {
	g1Aff = g1GenAff
	g2Aff = g2GenAff
	g1Jac = g1Gen
	g2Jac = g2Gen
	return
}
