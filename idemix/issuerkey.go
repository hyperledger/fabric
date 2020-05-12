/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
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
// See http://eprint.iacr.org/2016/663.pdf Sec. 4.3, for references.
func NewIssuerKey(AttributeNames []string, rng *amcl.RAND) (*IssuerKey, error) {
	// validate inputs

	// check for duplicated attributes
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
	key.Isk = BigToBytes(ISk)

	// generate the corresponding public key
	key.Ipk = new(IssuerPublicKey)
	key.Ipk.AttributeNames = AttributeNames

	W := GenG2.Mul(ISk)
	key.Ipk.W = Ecp2ToProto(W)

	// generate bases that correspond to the attributes
	key.Ipk.HAttrs = make([]*ECP, len(AttributeNames))
	for i := 0; i < len(AttributeNames); i++ {
		key.Ipk.HAttrs[i] = EcpToProto(GenG1.Mul(RandModOrder(rng)))
	}

	// generate base for the secret key
	HSk := GenG1.Mul(RandModOrder(rng))
	key.Ipk.HSk = EcpToProto(HSk)

	// generate base for the randomness
	HRand := GenG1.Mul(RandModOrder(rng))
	key.Ipk.HRand = EcpToProto(HRand)

	BarG1 := GenG1.Mul(RandModOrder(rng))
	key.Ipk.BarG1 = EcpToProto(BarG1)

	BarG2 := BarG1.Mul(ISk)
	key.Ipk.BarG2 = EcpToProto(BarG2)

	// generate a zero-knowledge proof of knowledge (ZK PoK) of the secret key which
	// is in W and BarG2.

	// Sample the randomness needed for the proof
	r := RandModOrder(rng)

	// Step 1: First message (t-values)
	t1 := GenG2.Mul(r) // t1 = g_2^r, cover W
	t2 := BarG1.Mul(r) // t2 = (\bar g_1)^r, cover BarG2

	// Step 2: Compute the Fiat-Shamir hash, forming the challenge of the ZKP.
	proofData := make([]byte, 18*FieldBytes+3)
	index := 0
	index = appendBytesG2(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG2(proofData, index, GenG2)
	index = appendBytesG1(proofData, index, BarG1)
	index = appendBytesG2(proofData, index, W)
	index = appendBytesG1(proofData, index, BarG2)

	proofC := HashModOrder(proofData)
	key.Ipk.ProofC = BigToBytes(proofC)

	// Step 3: reply to the challenge message (s-values)
	proofS := Modadd(FP256BN.Modmul(proofC, ISk, GroupOrder), r, GroupOrder) // // s = r + C \cdot ISk
	key.Ipk.ProofS = BigToBytes(proofS)

	// Hash the public key
	serializedIPk, err := proto.Marshal(key.Ipk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal issuer public key")
	}
	key.Ipk.Hash = BigToBytes(HashModOrder(serializedIPk))

	// We are done
	return key, nil
}

// Check checks that this issuer public key is valid, i.e.
// that all components are present and a ZK proofs verifies
func (IPk *IssuerPublicKey) Check() error {
	// Unmarshall the public key
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

	// Check that the public key is well-formed
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

	// Verify Proof

	// Recompute challenge
	proofData := make([]byte, 18*FieldBytes+3)
	index := 0

	// Recompute t-values using s-values
	t1 := GenG2.Mul(ProofS)
	t1.Add(W.Mul(FP256BN.Modneg(ProofC, GroupOrder))) // t1 = g_2^s \cdot W^{-C}

	t2 := BarG1.Mul(ProofS)
	t2.Add(BarG2.Mul(FP256BN.Modneg(ProofC, GroupOrder))) // t2 = {\bar g_1}^s \cdot {\bar g_2}^C

	index = appendBytesG2(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG2(proofData, index, GenG2)
	index = appendBytesG1(proofData, index, BarG1)
	index = appendBytesG2(proofData, index, W)
	index = appendBytesG1(proofData, index, BarG2)

	// Verify that the challenge is the same
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
