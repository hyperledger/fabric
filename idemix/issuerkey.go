/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/golang/protobuf/proto"
	"github.com/milagro-crypto/amcl/version3/go/amcl"
	"github.com/milagro-crypto/amcl/version3/go/amcl/FP256BN"
	"github.com/pkg/errors"
)

// The Issuer secret ISk and public IPk keys are used to issue credentials and
// to verify signatures created using the credentials

// The Issuer Secret Key is a random exponent (generated randomly from Z*_p)

// The Issuer Public Key consists of several elliptic curve points (ECP),
// where index 1 corresponds to group G1 and 2 to group G2)
// HSk, HRand, BarG1, BarG2, an ECP array HAttrs, and an ECP2 W,
// and a proof of knowledge of the corresponding secret key

// NewIssuerKey creates a new issuer key pair taking an array of attribute names
// that will be contained in credentials certified by this issuer (a credential specification)
func NewIssuerKey(AttributeNames []string, rng *amcl.RAND) (*IssuerKey, error) {
	attributeNamesMap := map[string]bool{}
	for _, name := range AttributeNames {
		if attributeNamesMap[name] {
			return nil, errors.Errorf("attribute %s appears multiple times in AttributeNames", name)
		}
		attributeNamesMap[name] = true
	}

	key := new(IssuerKey)

	// generate issuer secret key
	ISk := RandModOrder(rng)
	key.ISk = BigToBytes(ISk)

	// generate the corresponding public key
	key.IPk = new(IssuerPublicKey)
	key.IPk.AttributeNames = AttributeNames

	W := GenG2.Mul(ISk)
	key.IPk.W = Ecp2ToProto(W)

	// generate bases that correspond to the attributes
	key.IPk.HAttrs = make([]*ECP, len(AttributeNames))
	for i := 0; i < len(AttributeNames); i++ {
		key.IPk.HAttrs[i] = EcpToProto(GenG1.Mul(RandModOrder(rng)))
	}

	HSk := GenG1.Mul(RandModOrder(rng))
	key.IPk.HSk = EcpToProto(HSk)

	HRand := GenG1.Mul(RandModOrder(rng))
	key.IPk.HRand = EcpToProto(HRand)

	BarG1 := GenG1.Mul(RandModOrder(rng))
	key.IPk.BarG1 = EcpToProto(BarG1)

	BarG2 := BarG1.Mul(ISk)
	key.IPk.BarG2 = EcpToProto(BarG2)

	// generate a zero-knowledge proof of knowledge (ZK PoK) of the secret key
	r := RandModOrder(rng)
	t1 := GenG2.Mul(r)
	t2 := BarG1.Mul(r)

	proofData := make([]byte, 18*FieldBytes+3)
	index := 0
	index = appendBytesG2(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG2(proofData, index, GenG2)
	index = appendBytesG1(proofData, index, BarG1)
	index = appendBytesG2(proofData, index, W)
	index = appendBytesG1(proofData, index, BarG2)

	proofC := HashModOrder(proofData)
	key.IPk.ProofC = BigToBytes(proofC)

	proofS := Modadd(FP256BN.Modmul(proofC, ISk, GroupOrder), r, GroupOrder)
	key.IPk.ProofS = BigToBytes(proofS)

	serializedIPk, err := proto.Marshal(key.IPk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal issuer public key")
	}
	key.IPk.Hash = BigToBytes(HashModOrder(serializedIPk))

	return key, nil
}

// Check checks that this issuer public key is valid, i.e.
// that all components are present and a ZK proofs verifies
func (IPk *IssuerPublicKey) Check() error {
	NumAttrs := len(IPk.GetAttributeNames())
	HSk := EcpFromProto(IPk.GetHSk())
	HRand := EcpFromProto(IPk.GetHRand())
	HAttrs := make([]*FP256BN.ECP, len(IPk.GetHAttrs()))
	for i := 0; i < len(IPk.GetHAttrs()); i++ {
		HAttrs[i] = EcpFromProto(IPk.GetHAttrs()[i])
	}
	BarG1 := EcpFromProto(IPk.GetBarG1())
	BarG2 := EcpFromProto(IPk.GetBarG2())
	W := Ecp2FromProto(IPk.GetW())
	ProofC := FP256BN.FromBytes(IPk.GetProofC())
	ProofS := FP256BN.FromBytes(IPk.GetProofS())

	if NumAttrs < 0 ||
		HSk == nil ||
		HRand == nil ||
		BarG1 == nil ||
		BarG1.Is_infinity() ||
		BarG2 == nil ||
		HAttrs == nil ||
		len(IPk.HAttrs) < NumAttrs {
		return errors.Errorf("some part of the public key is undefined")
	}
	for i := 0; i < NumAttrs; i++ {
		if IPk.HAttrs[i] == nil {
			return errors.Errorf("some part of the public key is undefined")
		}
	}

	// Check Proof
	proofData := make([]byte, 18*FieldBytes+3)
	index := 0

	t1 := GenG2.Mul(ProofS)
	t1.Add(W.Mul(FP256BN.Modneg(ProofC, GroupOrder)))
	t2 := BarG1.Mul(ProofS)
	t2.Add(BarG2.Mul(FP256BN.Modneg(ProofC, GroupOrder)))

	index = appendBytesG2(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG2(proofData, index, GenG2)
	index = appendBytesG1(proofData, index, BarG1)
	index = appendBytesG2(proofData, index, W)
	index = appendBytesG1(proofData, index, BarG2)

	if *ProofC != *HashModOrder(proofData) {
		return errors.Errorf("zero knowledge proof in public key invalid")
	}

	return IPk.SetHash()
}

// SetHash appends a hash of a serialized public key
func (IPk *IssuerPublicKey) SetHash() error {
	IPk.Hash = nil
	serializedIPk, err := proto.Marshal(IPk)
	if err != nil {
		return errors.Wrap(err, "Failed to marshal issuer public key")
	}
	IPk.Hash = BigToBytes(HashModOrder(serializedIPk))
	return nil
}
