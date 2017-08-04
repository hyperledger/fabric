/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"crypto/rand"
	"crypto/sha256"

	amcl "github.com/manudrijvers/amcl/go"
	"github.com/pkg/errors"
)

// GenG1 is a generator of Group G1
var GenG1 = amcl.NewECPbigs(
	amcl.NewBIGints(amcl.CURVE_Gx),
	amcl.NewBIGints(amcl.CURVE_Gy))

// GenG2 is a generator of Group G2
var GenG2 = amcl.NewECP2fp2s(
	amcl.NewFP2bigs(amcl.NewBIGints(amcl.CURVE_Pxa), amcl.NewBIGints(amcl.CURVE_Pxb)),
	amcl.NewFP2bigs(amcl.NewBIGints(amcl.CURVE_Pya), amcl.NewBIGints(amcl.CURVE_Pyb)))

// GroupOrder is the order of the groups
var GroupOrder = amcl.NewBIGints(amcl.CURVE_Order)

// FieldBytes is the bytelength of the group order
var FieldBytes = int(amcl.MODBYTES)

// RandModOrder returns a random element in 0, ..., GroupOrder-1
func RandModOrder(rng *amcl.RAND) *amcl.BIG {
	// curve order q
	q := amcl.NewBIGints(amcl.CURVE_Order)

	// Take random element in Zq
	return amcl.Randomnum(q, rng)
}

// HashModOrder hashes data into 0, ..., GroupOrder-1
func HashModOrder(data []byte) *amcl.BIG {
	digest := sha256.Sum256(data)
	digestBig := amcl.FromBytes(digest[:])
	digestBig.Mod(GroupOrder)
	return digestBig
}

func appendBytesG1(data []byte, index int, E *amcl.ECP) int {
	length := 2*FieldBytes + 1
	E.ToBytes(data[index : index+length])
	return index + length
}
func appendBytesG2(data []byte, index int, E *amcl.ECP2) int {
	length := 4 * FieldBytes
	E.ToBytes(data[index : index+length])
	return index + length
}
func appendBytesBig(data []byte, index int, B *amcl.BIG) int {
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
func MakeNym(sk *amcl.BIG, IPk *IssuerPublicKey, rng *amcl.RAND) (*amcl.ECP, *amcl.BIG) {
	RandNym := RandModOrder(rng)
	Nym := EcpFromProto(IPk.HSk).Mul2(sk, EcpFromProto(IPk.HRand), RandNym)
	return Nym, RandNym
}

// BigToBytes takes an *amcl.BIG and returns a []byte representation
func BigToBytes(big *amcl.BIG) []byte {
	ret := make([]byte, FieldBytes)
	big.ToBytes(ret)
	return ret
}

// EcpToProto converts a *amcl.ECP into the proto struct *ECP
func EcpToProto(p *amcl.ECP) *ECP {
	return &ECP{
		BigToBytes(p.GetX()),
		BigToBytes(p.GetY())}
}

// EcpFromProto converts a proto struct *ECP into an *amcl.ECP
func EcpFromProto(p *ECP) *amcl.ECP {
	return amcl.NewECPbigs(amcl.FromBytes(p.GetX()), amcl.FromBytes(p.GetY()))
}

// Ecp2ToProto converts a *amcl.ECP2 into the proto struct *ECP2
func Ecp2ToProto(p *amcl.ECP2) *ECP2 {
	return &ECP2{
		BigToBytes(p.GetX().GetA()),
		BigToBytes(p.GetX().GetB()),
		BigToBytes(p.GetY().GetA()),
		BigToBytes(p.GetY().GetB())}
}

// Ecp2FromProto converts a proto struct *ECP2 into an *amcl.ECP2
func Ecp2FromProto(p *ECP2) *amcl.ECP2 {
	return amcl.NewECP2fp2s(
		amcl.NewFP2bigs(amcl.FromBytes(p.GetXA()), amcl.FromBytes(p.GetXB())),
		amcl.NewFP2bigs(amcl.FromBytes(p.GetYA()), amcl.FromBytes(p.GetYB())))
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
