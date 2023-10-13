/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package amcl

import (
	"crypto/hmac"
	"crypto/sha256"
	"math/big"
	"regexp"
	"strings"

	"github.com/IBM/mathlib/driver"
	"github.com/IBM/mathlib/driver/common"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
)

/*********************************************************************/

type fp256bnGt struct {
	FP256BN.FP12
}

func (a *fp256bnGt) Exp(x driver.Zr) driver.Gt {
	return &fp256bnGt{*a.FP12.Pow(bigToMiraclBIGCore(&x.(*common.BaseZr).Int))}
}

func (a *fp256bnGt) Equals(b driver.Gt) bool {
	return a.FP12.Equals(&b.(*fp256bnGt).FP12)
}

func (a *fp256bnGt) IsUnity() bool {
	return a.FP12.Isunity()
}

func (a *fp256bnGt) Inverse() {
	a.FP12.Inverse()
}

func (a *fp256bnGt) Mul(b driver.Gt) {
	a.FP12.Mul(&b.(*fp256bnGt).FP12)
}

func (b *fp256bnGt) ToString() string {
	return b.FP12.ToString()
}

func (b *fp256bnGt) Bytes() []byte {
	bytes := make([]byte, 12*int(FP256BN.MODBYTES))
	b.FP12.ToBytes(bytes)
	return bytes
}

/*********************************************************************/

func NewFp256bn() *Fp256bn {
	return &Fp256bn{common.CurveBase{Modulus: modulusBig}}
}

type Fp256bn struct {
	common.CurveBase
}

func (*Fp256bn) Pairing(a driver.G2, b driver.G1) driver.Gt {
	return &fp256bnGt{*FP256BN.Ate(&a.(*fp256bnG2).ECP2, &b.(*fp256bnG1).ECP)}
}

func (*Fp256bn) Pairing2(p2a, p2b driver.G2, p1a, p1b driver.G1) driver.Gt {
	return &fp256bnGt{*FP256BN.Ate2(&p2a.(*fp256bnG2).ECP2, &p1a.(*fp256bnG1).ECP, &p2b.(*fp256bnG2).ECP2, &p1b.(*fp256bnG1).ECP)}
}

func (*Fp256bn) FExp(e driver.Gt) driver.Gt {
	return &fp256bnGt{*FP256BN.Fexp(&e.(*fp256bnGt).FP12)}
}

func (*Fp256bn) GenG1() driver.G1 {
	return &fp256bnG1{*FP256BN.NewECPbigs(FP256BN.NewBIGints(FP256BN.CURVE_Gx), FP256BN.NewBIGints(FP256BN.CURVE_Gy))}
}

func (*Fp256bn) GenG2() driver.G2 {
	return &fp256bnG2{*FP256BN.NewECP2fp2s(
		FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pxa), FP256BN.NewBIGints(FP256BN.CURVE_Pxb)),
		FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pya), FP256BN.NewBIGints(FP256BN.CURVE_Pyb)))}
}

func (p *Fp256bn) GenGt() driver.Gt {
	return &fp256bnGt{*FP256BN.Fexp(FP256BN.Ate(&p.GenG2().(*fp256bnG2).ECP2, &p.GenG1().(*fp256bnG1).ECP))}
}

func (p *Fp256bn) CoordinateByteSize() int {
	return int(FP256BN.MODBYTES)
}

func (p *Fp256bn) G1ByteSize() int {
	return 2*int(FP256BN.MODBYTES) + 1
}

func (p *Fp256bn) CompressedG1ByteSize() int {
	return int(FP256BN.MODBYTES) + 1
}

func (p *Fp256bn) G2ByteSize() int {
	return 4 * int(FP256BN.MODBYTES)
}

func (p *Fp256bn) CompressedG2ByteSize() int {
	return 4 * int(FP256BN.MODBYTES)
}

func (p *Fp256bn) ScalarByteSize() int {
	return common.ScalarByteSize
}

func (p *Fp256bn) NewG1() driver.G1 {
	return &fp256bnG1{*FP256BN.NewECP()}
}

func (p *Fp256bn) NewG2() driver.G2 {
	return &fp256bnG2{*FP256BN.NewECP2()}
}

func bigToMiraclBIGCore(bi *big.Int) *FP256BN.BIG {
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

func (p *Fp256bn) NewG1FromBytes(b []byte) driver.G1 {
	return &fp256bnG1{*FP256BN.ECP_fromBytes(b)}
}

func (p *Fp256bn) NewG2FromBytes(b []byte) driver.G2 {
	return &fp256bnG2{*FP256BN.ECP2_fromBytes(b)}
}

func (p *Fp256bn) NewG1FromCompressed(b []byte) driver.G1 {
	return &fp256bnG1{*FP256BN.ECP_fromBytes(b)}
}

func (p *Fp256bn) NewG2FromCompressed(b []byte) driver.G2 {
	return &fp256bnG2{*FP256BN.ECP2_fromBytes(b)}
}

func (p *Fp256bn) NewGtFromBytes(b []byte) driver.Gt {
	return &fp256bnGt{*FP256BN.FP12_fromBytes(b)}
}

func (p *Fp256bn) HashToG1(data []byte) driver.G1 {
	return &fp256bnG1{*FP256BN.Bls_hash(string(data))}
}

func (p *Fp256bn) HashToG1WithDomain(data, domain []byte) driver.G1 {
	mac := hmac.New(sha256.New, domain)
	mac.Write(data)
	return &fp256bnG1{*FP256BN.Bls_hash(string(mac.Sum(nil)))}
}

/*********************************************************************/

type fp256bnG1 struct {
	FP256BN.ECP
}

func (e *fp256bnG1) Clone(a driver.G1) {
	e.ECP.Copy(&a.(*fp256bnG1).ECP)
}

func (e *fp256bnG1) Copy() driver.G1 {
	c := FP256BN.NewECP()
	c.Copy(&e.ECP)
	return &fp256bnG1{*c}
}

func (e *fp256bnG1) Add(a driver.G1) {
	e.ECP.Add(&a.(*fp256bnG1).ECP)
}

func (e *fp256bnG1) Mul(a driver.Zr) driver.G1 {
	return &fp256bnG1{*FP256BN.G1mul(&e.ECP, bigToMiraclBIGCore(&a.(*common.BaseZr).Int))}
}

func (e *fp256bnG1) Mul2(ee driver.Zr, Q driver.G1, f driver.Zr) driver.G1 {
	return &fp256bnG1{*e.ECP.Mul2(bigToMiraclBIGCore(&ee.(*common.BaseZr).Int), &Q.(*fp256bnG1).ECP, bigToMiraclBIGCore(&f.(*common.BaseZr).Int))}
}

func (e *fp256bnG1) Equals(a driver.G1) bool {
	return e.ECP.Equals(&a.(*fp256bnG1).ECP)
}

func (e *fp256bnG1) IsInfinity() bool {
	return e.ECP.Is_infinity()
}

func (e *fp256bnG1) Bytes() []byte {
	b := make([]byte, 2*int(FP256BN.MODBYTES)+1)
	e.ECP.ToBytes(b, false)
	return b
}

func (e *fp256bnG1) Compressed() []byte {
	b := make([]byte, int(FP256BN.MODBYTES)+1)
	e.ECP.ToBytes(b, true)
	return b
}

func (e *fp256bnG1) Sub(a driver.G1) {
	e.ECP.Sub(&a.(*fp256bnG1).ECP)
}

var g1StrRegexp *regexp.Regexp = regexp.MustCompile(`^\(([0-9a-f]+),([0-9a-f]+)\)$`)

func (b *fp256bnG1) String() string {
	rawstr := b.ECP.ToString()
	m := g1StrRegexp.FindAllStringSubmatch(rawstr, -1)
	return "(" + strings.TrimLeft(m[0][1], "0") + "," + strings.TrimLeft(m[0][2], "0") + ")"
}

func (e *fp256bnG1) Neg() {
	res := e.Mul(NewFp256bn().NewZrFromInt(-1))
	e.ECP = res.(*fp256bnG1).ECP
}

/*********************************************************************/

type fp256bnG2 struct {
	FP256BN.ECP2
}

func (e *fp256bnG2) Equals(a driver.G2) bool {
	return e.ECP2.Equals(&a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Clone(a driver.G2) {
	e.ECP2.Copy(&a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Copy() driver.G2 {
	c := FP256BN.NewECP2()
	c.Copy(&e.ECP2)
	return &fp256bnG2{*c}
}

func (e *fp256bnG2) Add(a driver.G2) {
	e.ECP2.Add(&a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Sub(a driver.G2) {
	e.ECP2.Sub(&a.(*fp256bnG2).ECP2)
}

func (e *fp256bnG2) Mul(a driver.Zr) driver.G2 {
	return &fp256bnG2{*e.ECP2.Mul(bigToMiraclBIGCore(&a.(*common.BaseZr).Int))}
}

func (e *fp256bnG2) Affine() {
	e.ECP2.Affine()
}

func (e *fp256bnG2) Bytes() []byte {
	b := make([]byte, 4*int(FP256BN.MODBYTES))
	e.ECP2.ToBytes(b)
	return b
}

func (e *fp256bnG2) Compressed() []byte {
	b := make([]byte, 4*int(FP256BN.MODBYTES))
	e.ECP2.ToBytes(b)
	return b
}

func (b *fp256bnG2) String() string {
	return b.ECP2.ToString()
}
