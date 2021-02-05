/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"io"

	amcl "github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
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
func (i *Idemix) NewIssuerKey(AttributeNames []string, rng io.Reader, t Translator) (*IssuerKey, error) {
	return newIssuerKey(AttributeNames, rng, i.Curve, t)
}

func newIssuerKey(AttributeNames []string, rng io.Reader, curve *math.Curve, t Translator) (*IssuerKey, error) {
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
	ISk := curve.NewRandomZr(rng)
	key.Isk = ISk.Bytes()

	// generate the corresponding public key
	key.Ipk = new(IssuerPublicKey)
	key.Ipk.AttributeNames = AttributeNames

	W := curve.GenG2.Mul(ISk)
	key.Ipk.W = t.G2ToProto(W)

	// generate bases that correspond to the attributes
	key.Ipk.HAttrs = make([]*amcl.ECP, len(AttributeNames))
	for i := 0; i < len(AttributeNames); i++ {
		key.Ipk.HAttrs[i] = t.G1ToProto(curve.GenG1.Mul(curve.NewRandomZr(rng)))
	}

	// generate base for the secret key
	HSk := curve.GenG1.Mul(curve.NewRandomZr(rng))
	key.Ipk.HSk = t.G1ToProto(HSk)

	// generate base for the randomness
	HRand := curve.GenG1.Mul(curve.NewRandomZr(rng))
	key.Ipk.HRand = t.G1ToProto(HRand)

	BarG1 := curve.GenG1.Mul(curve.NewRandomZr(rng))
	key.Ipk.BarG1 = t.G1ToProto(BarG1)

	BarG2 := BarG1.Mul(ISk)
	key.Ipk.BarG2 = t.G1ToProto(BarG2)

	// generate a zero-knowledge proof of knowledge (ZK PoK) of the secret key which
	// is in W and BarG2.

	// Sample the randomness needed for the proof
	r := curve.NewRandomZr(rng)

	// Step 1: First message (t-values)
	t1 := curve.GenG2.Mul(r) // t1 = g_2^r, cover W
	t2 := BarG1.Mul(r)       // t2 = (\bar g_1)^r, cover BarG2

	// Step 2: Compute the Fiat-Shamir hash, forming the challenge of the ZKP.
	proofData := make([]byte, 18*curve.FieldBytes+3)
	index := 0
	index = appendBytesG2(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG2(proofData, index, curve.GenG2)
	index = appendBytesG1(proofData, index, BarG1)
	index = appendBytesG2(proofData, index, W)
	index = appendBytesG1(proofData, index, BarG2)

	proofC := curve.HashToZr(proofData)
	key.Ipk.ProofC = proofC.Bytes()

	// Step 3: reply to the challenge message (s-values)
	proofS := curve.ModAdd(curve.ModMul(proofC, ISk, curve.GroupOrder), r, curve.GroupOrder) // // s = r + C \cdot ISk
	key.Ipk.ProofS = proofS.Bytes()

	// Hash the public key
	serializedIPk, err := proto.Marshal(key.Ipk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal issuer public key")
	}
	key.Ipk.Hash = curve.HashToZr(serializedIPk).Bytes()

	// We are done
	return key, nil
}

func (i *Idemix) NewIssuerKeyFromBytes(raw []byte) (*IssuerKey, error) {
	return newIssuerKeyFromBytes(raw)
}

func newIssuerKeyFromBytes(raw []byte) (*IssuerKey, error) {
	ik := &IssuerKey{}
	if err := proto.Unmarshal(raw, ik); err != nil {
		return nil, err
	}

	//raw, err :=proto.Marshal(ik.Ipk.W)
	//if err != nil {
	//	panic(err)
	//}
	//fmt.Printf("IPKW : [%v]", ik.Ipk.W.Xa)

	return ik, nil
}

// Check checks that this issuer public key is valid, i.e.
// that all components are present and a ZK proofs verifies
func (IPk *IssuerPublicKey) Check(curve *math.Curve, t Translator) error {
	// Unmarshall the public key
	NumAttrs := len(IPk.GetAttributeNames())
	HSk, err := t.G1FromProto(IPk.GetHSk())
	if err != nil {
		return err
	}

	HRand, err := t.G1FromProto(IPk.GetHRand())
	if err != nil {
		return err
	}

	HAttrs := make([]*math.G1, len(IPk.GetHAttrs()))
	for i := 0; i < len(IPk.GetHAttrs()); i++ {
		HAttrs[i], err = t.G1FromProto(IPk.GetHAttrs()[i])
		if err != nil {
			return err
		}
	}
	BarG1, err := t.G1FromProto(IPk.GetBarG1())
	if err != nil {
		return err
	}

	BarG2, err := t.G1FromProto(IPk.GetBarG2())
	if err != nil {
		return err
	}

	W, err := t.G2FromProto(IPk.GetW())
	if err != nil {
		return err
	}

	ProofC := curve.NewZrFromBytes(IPk.GetProofC())
	ProofS := curve.NewZrFromBytes(IPk.GetProofS())

	// Check that the public key is well-formed
	if NumAttrs < 0 ||
		HSk == nil ||
		HRand == nil ||
		BarG1 == nil ||
		BarG1.IsInfinity() ||
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
	proofData := make([]byte, 18*curve.FieldBytes+3)
	index := 0

	// Recompute t-values using s-values
	t1 := curve.GenG2.Mul(ProofS)
	t1.Add(W.Mul(curve.ModNeg(ProofC, curve.GroupOrder))) // t1 = g_2^s \cdot W^{-C}

	t2 := BarG1.Mul(ProofS)
	t2.Add(BarG2.Mul(curve.ModNeg(ProofC, curve.GroupOrder))) // t2 = {\bar g_1}^s \cdot {\bar g_2}^C

	index = appendBytesG2(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG2(proofData, index, curve.GenG2)
	index = appendBytesG1(proofData, index, BarG1)
	index = appendBytesG2(proofData, index, W)
	index = appendBytesG1(proofData, index, BarG2)

	// Verify that the challenge is the same
	if !ProofC.Equals(curve.HashToZr(proofData)) {
		return errors.Errorf("zero knowledge proof in public key invalid")
	}

	return IPk.SetHash(curve)
}

// SetHash appends a hash of a serialized public key
func (IPk *IssuerPublicKey) SetHash(curve *math.Curve) error {
	IPk.Hash = nil
	serializedIPk, err := proto.Marshal(IPk)
	if err != nil {
		return errors.Wrap(err, "Failed to marshal issuer public key")
	}
	IPk.Hash = curve.HashToZr(serializedIPk).Bytes()
	return nil
}
