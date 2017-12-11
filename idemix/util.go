/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"crypto/rand"
	"crypto/sha256"

	"github.com/milagro-crypto/amcl/version3/go/amcl"
	"github.com/milagro-crypto/amcl/version3/go/amcl/FP256BN"
	"github.com/pkg/errors"
)

// GenG1 is a generator of Group G1
var GenG1 = FP256BN.NewECPbigs(
	FP256BN.NewBIGints(FP256BN.CURVE_Gx),
	FP256BN.NewBIGints(FP256BN.CURVE_Gy))

// GenG2 is a generator of Group G2
var GenG2 = FP256BN.NewECP2fp2s(
	FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pxa), FP256BN.NewBIGints(FP256BN.CURVE_Pxb)),
	FP256BN.NewFP2bigs(FP256BN.NewBIGints(FP256BN.CURVE_Pya), FP256BN.NewBIGints(FP256BN.CURVE_Pyb)))

// GroupOrder is the order of the groups
var GroupOrder = FP256BN.NewBIGints(FP256BN.CURVE_Order)

// FieldBytes is the bytelength of the group order
var FieldBytes = int(FP256BN.MODBYTES)

// RandModOrder returns a random element in 0, ..., GroupOrder-1
func RandModOrder(rng *amcl.RAND) *FP256BN.BIG {
	// curve order q
	q := FP256BN.NewBIGints(FP256BN.CURVE_Order)

	// Take random element in Zq
	return FP256BN.Randomnum(q, rng)
}

// HashModOrder hashes data into 0, ..., GroupOrder-1
func HashModOrder(data []byte) *FP256BN.BIG {
	digest := sha256.Sum256(data)
	digestBig := FP256BN.FromBytes(digest[:])
	digestBig.Mod(GroupOrder)
	return digestBig
}

func appendBytesG1(data []byte, index int, E *FP256BN.ECP) int {
	length := 2*FieldBytes + 1
	E.ToBytes(data[index : index+length])
	return index + length
}
func appendBytesG2(data []byte, index int, E *FP256BN.ECP2) int {
	length := 4 * FieldBytes
	E.ToBytes(data[index : index+length])
	return index + length
}
func appendBytesBig(data []byte, index int, B *FP256BN.BIG) int {
	length := FieldBytes
	B.ToBytes(data[index : index+length])
	return index + length
}
func appendBytesString(data []byte, index int, s string) int {
	bytes := []byte(s)
	copy(data[index:], bytes)
	return index + len(bytes)
}

// MakeNym creates a new unlinkable pseudonym
func MakeNym(sk *FP256BN.BIG, IPk *IssuerPublicKey, rng *amcl.RAND) (*FP256BN.ECP, *FP256BN.BIG) {
	RandNym := RandModOrder(rng)
	Nym := EcpFromProto(IPk.HSk).Mul2(sk, EcpFromProto(IPk.HRand), RandNym)
	return Nym, RandNym
}

// BigToBytes takes an *amcl.BIG and returns a []byte representation
func BigToBytes(big *FP256BN.BIG) []byte {
	ret := make([]byte, FieldBytes)
	big.ToBytes(ret)
	return ret
}

// EcpToProto converts a *amcl.ECP into the proto struct *ECP
func EcpToProto(p *FP256BN.ECP) *ECP {
	return &ECP{
		BigToBytes(p.GetX()),
		BigToBytes(p.GetY())}
}

// EcpFromProto converts a proto struct *ECP into an *amcl.ECP
func EcpFromProto(p *ECP) *FP256BN.ECP {
	return FP256BN.NewECPbigs(FP256BN.FromBytes(p.GetX()), FP256BN.FromBytes(p.GetY()))
}

// Ecp2ToProto converts a *amcl.ECP2 into the proto struct *ECP2
func Ecp2ToProto(p *FP256BN.ECP2) *ECP2 {
	return &ECP2{
		BigToBytes(p.GetX().GetA()),
		BigToBytes(p.GetX().GetB()),
		BigToBytes(p.GetY().GetA()),
		BigToBytes(p.GetY().GetB())}
}

// Ecp2FromProto converts a proto struct *ECP2 into an *amcl.ECP2
func Ecp2FromProto(p *ECP2) *FP256BN.ECP2 {
	return FP256BN.NewECP2fp2s(
		FP256BN.NewFP2bigs(FP256BN.FromBytes(p.GetXA()), FP256BN.FromBytes(p.GetXB())),
		FP256BN.NewFP2bigs(FP256BN.FromBytes(p.GetYA()), FP256BN.FromBytes(p.GetYB())))
}

// GetRand returns a new *amcl.RAND with a fresh seed
func GetRand() (*amcl.RAND, error) {
	seedLength := 32
	b := make([]byte, seedLength)
	_, err := rand.Read(b)
	if err != nil {
		return nil, errors.Wrap(err, "error getting randomness for seed")
	}
	rng := amcl.NewRAND()
	rng.Clean()
	rng.Seed(seedLength, b)
	return rng, nil
}

// Modadd takes input BIGs a, b, m, and returns a+b modulo m
func Modadd(a, b, m *FP256BN.BIG) *FP256BN.BIG {
	c := a.Plus(b)
	c.Mod(m)
	return c
}

// Modsub takes input BIGs a, b, m and returns a-b modulo m
func Modsub(a, b, m *FP256BN.BIG) *FP256BN.BIG {
	return Modadd(a, FP256BN.Modneg(b, m), m)
}
