/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package amcl

import (
	"math/big"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	"github.com/hyperledger/fabric-amcl/core/FP256BN"
)

/*********************************************************************/

var modulusBig big.Int // q stored as big.Int
func init() {
	modulusBig.SetString("fffffffffffcf0cd46e5f25eee71a49e0cdc65fb1299921af62d536cd10b500d", 16)
}

/*********************************************************************/

type fp256bnMiraclGt struct {
	FP256BN.FP12
}

func (a *fp256bnMiraclGt) Exp(x driver.Zr) driver.Gt {
	return &fp256bnMiraclGt{*a.FP12.Pow(bigToMiraclBIG(&x.(*common.BaseZr).Int))}
}

func (a *fp256bnMiraclGt) Equals(b driver.Gt) bool {
	return a.FP12.Equals(&b.(*fp256bnMiraclGt).FP12)
}

func (a *fp256bnMiraclGt) IsUnity() bool {
	return a.FP12.Isunity()
}

func (a *fp256bnMiraclGt) Inverse() {
	a.FP12.Inverse()
}

func (a *fp256bnMiraclGt) Mul(b driver.Gt) {
	a.FP12.Mul(&b.(*fp256bnMiraclGt).FP12)
}

func (b *fp256bnMiraclGt) ToString() string {
	return b.FP12.ToString()
}

func (b *fp256bnMiraclGt) Bytes() []byte {
	bytes := make([]byte, 12*int(FP256BN.MODBYTES))
	b.FP12.ToBytes(bytes)
	return bytes
}

/*********************************************************************/

func NewFp256Miraclbn() *Fp256Miraclbn {
	return &Fp256Miraclbn{common.CurveBase{Modulus: modulusBig}}
}

type Fp256Miraclbn struct {
	common.CurveBase
}

func (*Fp256Miraclbn) Pairing(a driver.G2, b driver.G1) driver.Gt {
	return &fp256bnMiraclGt{*FP256BN.Ate(a.(*fp256bnMiraclG2).ECP2, &b.(*fp256bnMiraclG1).ECP)}
}

func (*Fp256Miraclbn) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	return &fp256bnMiraclGt{*FP256BN.Ate2(p2a.(*fp256bnMiraclG2).ECP2, &p1a.(*fp256bnMiraclG1).ECP, p2b.(*fp256bnMiraclG2).ECP2, &p1b.(*fp256bnMiraclG1).ECP)}
}

func (*Fp256Miraclbn) FExp(e driver.Gt) driver.Gt {
	return &fp256bnMiraclGt{*FP256BN.Fexp(&e.(*fp256bnMiraclGt).FP12)}
}

func (*Fp256Miraclbn) GenG1() driver.G1 {
	return &fp256bnMiraclG1{*FP256BN.NewECPbigs(FP256BN.NewBIGints(FP256BN.CURVE_Gx), FP256BN.NewBIGints(FP256BN.CURVE_Gy))}
}

func (*Fp256Miraclbn) GenG2() driver.G2 {
	return &fp256bnMiraclG2{FP256BN.NewECP2fp2s(
		FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pxa), FP256BN.NewBIGints(FP256BN.CURVE_Pxb)),
		FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pya), FP256BN.NewBIGints(FP256BN.CURVE_Pyb)))}
}

func (p *Fp256Miraclbn) GenGt() driver.Gt {
	return &fp256bnMiraclGt{*FP256BN.Fexp(FP256BN.Ate(p.GenG2().(*fp256bnMiraclG2).ECP2, &p.GenG1().(*fp256bnMiraclG1).ECP))}
}

func (p *Fp256Miraclbn) CoordinateByteSize() int {
	return int(FP256BN.MODBYTES)
}

func (p *Fp256Miraclbn) G1ByteSize() int {
	return 2*int(FP256BN.MODBYTES) + 1
}

func (p *Fp256Miraclbn) CompressedG1ByteSize() int {
	return int(FP256BN.MODBYTES) + 1
}

func (p *Fp256Miraclbn) G2ByteSize() int {
	return 4*int(FP256BN.MODBYTES) + 1
}

func (p *Fp256Miraclbn) CompressedG2ByteSize() int {
	return 2*int(FP256BN.MODBYTES) + 1
}

func (p *Fp256Miraclbn) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (p *Fp256Miraclbn) NewG1() driver.G1 {
	return &fp256bnMiraclG1{*FP256BN.NewECP()}
}

func (p *Fp256Miraclbn) NewG2() driver.G2 {
	return &fp256bnMiraclG2{FP256BN.NewECP2()}
}

func bigToMiraclBIG(bi *big.Int) *FP256BN.BIG {
	biCopy := bi

	if bi.Sign() < 0 || bi.Cmp(&modulusBig) > 0 {
		biCopy = new(big.Int).Set(bi)
		biCopy = biCopy.Mod(biCopy, &modulusBig)
		if biCopy.Sign() < 0 {
			biCopy = biCopy.Add(biCopy, &modulusBig)
		}
	}

	return FP256BN.FromBytes(common.BigToBytes(biCopy))
}

func (p *Fp256Miraclbn) NewG1FromBytes(b []byte) driver.G1 {
	return &fp256bnMiraclG1{*FP256BN.ECP_fromBytes(b)}
}

func (p *Fp256Miraclbn) NewG2FromBytes(b []byte) driver.G2 {
	return &fp256bnMiraclG2{FP256BN.ECP2_fromBytes(b)}
}

func (p *Fp256Miraclbn) NewG1FromCompressed(b []byte) driver.G1 {
	return &fp256bnMiraclG1{*FP256BN.ECP_fromBytes(b)}
}

func (p *Fp256Miraclbn) NewG2FromCompressed(b []byte) driver.G2 {
	return &fp256bnMiraclG2{FP256BN.ECP2_fromBytes(b)}
}

func (p *Fp256Miraclbn) NewGtFromBytes(b []byte) driver.Gt {
	return &fp256bnMiraclGt{*FP256BN.FP12_fromBytes(b)}
}

func (p *Fp256Miraclbn) HashToG1(data []byte) driver.G1 {
	return &fp256bnMiraclG1{*bls_hash_to_point_miracl(data, []byte{})}
}

func (p *Fp256Miraclbn) HashToG1WithDomain(data, domain []byte) driver.G1 {
	return &fp256bnMiraclG1{*bls_hash_to_point_miracl(data, domain)}
}

/*********************************************************************/

type fp256bnMiraclG1 struct {
	FP256BN.ECP
}

func (e *fp256bnMiraclG1) Clone(a driver.G1) {
	e.ECP.Copy(&a.(*fp256bnMiraclG1).ECP)
}

func (e *fp256bnMiraclG1) Copy() driver.G1 {
	c := FP256BN.NewECP()
	c.Copy(&e.ECP)
	return &fp256bnMiraclG1{*c}
}

func (e *fp256bnMiraclG1) Add(a driver.G1) {
	e.ECP.Add(&a.(*fp256bnMiraclG1).ECP)
}

func (e *fp256bnMiraclG1) Mul(a driver.Zr) driver.G1 {
	return &fp256bnMiraclG1{*FP256BN.G1mul(&e.ECP, bigToMiraclBIG(&a.(*common.BaseZr).Int))}
}

func (e *fp256bnMiraclG1) Mul2(ee driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	return &fp256bnMiraclG1{*e.ECP.Mul2(bigToMiraclBIG(&ee.(*common.BaseZr).Int), &Q.(*fp256bnMiraclG1).ECP, bigToMiraclBIG(&f.(*common.BaseZr).Int))}
}

func (e *fp256bnMiraclG1) Equals(a driver.G1) bool {
	return e.ECP.Equals(&a.(*fp256bnMiraclG1).ECP)
}

func (e *fp256bnMiraclG1) IsInfinity() bool {
	return e.ECP.Is_infinity()
}

func (e *fp256bnMiraclG1) Bytes() []byte {
	b := make([]byte, 2*int(FP256BN.MODBYTES)+1)
	e.ECP.ToBytes(b, false)
	return b
}

func (e *fp256bnMiraclG1) Compressed() []byte {
	b := make([]byte, int(FP256BN.MODBYTES)+1)
	e.ECP.ToBytes(b, true)
	return b
}

func (e *fp256bnMiraclG1) Sub(a driver.G1) {
	e.ECP.Sub(&a.(*fp256bnMiraclG1).ECP)
}

func (b *fp256bnMiraclG1) String() string {
	rawstr := b.ECP.ToString()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

func (e *fp256bnMiraclG1) Neg() {
	e.ECP.Neg()
}

/*********************************************************************/

type fp256bnMiraclG2 struct {
	*FP256BN.ECP2
}

func (e *fp256bnMiraclG2) Equals(a driver.G2) bool {
	return e.ECP2.Equals(a.(*fp256bnMiraclG2).ECP2)
}

func (e *fp256bnMiraclG2) Clone(a driver.G2) {
	e.ECP2.Copy(a.(*fp256bnMiraclG2).ECP2)
}

func (e *fp256bnMiraclG2) Copy() driver.G2 {
	c := FP256BN.NewECP2()
	c.Copy(e.ECP2)
	return &fp256bnMiraclG2{c}
}

func (e *fp256bnMiraclG2) Add(a driver.G2) {
	e.ECP2.Add(a.(*fp256bnMiraclG2).ECP2)
}

func (e *fp256bnMiraclG2) Sub(a driver.G2) {
	e.ECP2.Sub(a.(*fp256bnMiraclG2).ECP2)
}

func (e *fp256bnMiraclG2) Mul(a driver.Zr) driver.G2 {
	return &fp256bnMiraclG2{e.ECP2.Mul(bigToMiraclBIG(&a.(*common.BaseZr).Int))}
}

func (e *fp256bnMiraclG2) Affine() {
	e.ECP2.Affine()
}

func (e *fp256bnMiraclG2) Bytes() []byte {
	b := make([]byte, 4*int(FP256BN.MODBYTES)+1)
	e.ECP2.ToBytes(b, false)
	return b
}

func (e *fp256bnMiraclG2) Compressed() []byte {
	b := make([]byte, 2*int(FP256BN.MODBYTES)+1)
	e.ECP2.ToBytes(b, true)
	return b
}

func (b *fp256bnMiraclG2) String() string {
	return b.ECP2.ToString()
}
