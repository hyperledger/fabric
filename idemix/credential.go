/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

// Identity Mixer Credential is a list of attributes certified (signed) by the issuer
// A credential also contains a user secret key blindly signed by the issuer
// Without the secret key the credential cannot be used

// Credential issuance is an interactive protocol between a user and an issuer
// The issuer takes its secret and public keys and user attribute values as input
// The user takes the issuer public key and user secret as input
// The issuance protocol consists of the following steps:
// 1) The issuer sends a random nonce to the user
// 2) The user creates a Credential Request using the public key of the issuer, user secret, and the nonce as input
//    The request consists of a commitment to the user secret (can be seen as a public key) and a zero-knowledge proof
//     of knowledge of the user secret key
//    The user sends the credential request to the issuer
// 3) The issuer verifies the credential request by verifying the zero-knowledge proof
//    If the request is valid, the issuer issues a credential to the user by signing the commitment to the secret key
//    together with the attribute values and sends the credential back to the user
// 4) The user verifies the issuer's signature and stores the credential that consists of
//    the signature value, a randomness used to create the signature, the user secret, and the attribute values

// NewCredential issues a new credential, which is the last step of the interactive issuance protocol
// All attribute values are added by the issuer at this step and then signed together with a commitment to
// the user's secret key from a credential request
func NewCredential(key *IssuerKey, m *CredRequest, attrs []*FP256BN.BIG, rng *amcl.RAND) (*Credential, error) {
	// check the credential request that contains
	err := m.Check(key.Ipk)
	if err != nil {
		return nil, err
	}

	if len(attrs) != len(key.Ipk.AttributeNames) {
		return nil, errors.Errorf("incorrect number of attribute values passed")
	}

	// Place a BBS+ signature on the user key and the attribute values
	// (For BBS+, see e.g. "Constant-Size Dynamic k-TAA" by Man Ho Au, Willy Susilo, Yi Mu)
	E := RandModOrder(rng)
	S := RandModOrder(rng)

	B := FP256BN.NewECP()
	B.Copy(GenG1)
	Nym := EcpFromProto(m.Nym)
	B.Add(Nym)
	B.Add(EcpFromProto(key.Ipk.HRand).Mul(S))

	// Use Mul2 instead of Mul as much as possible
	for i := 0; i < len(attrs)/2; i++ {
		B.Add(EcpFromProto(key.Ipk.HAttrs[2*i]).Mul2(attrs[2*i], EcpFromProto(key.Ipk.HAttrs[2*i+1]), attrs[2*i+1]))
	}
	if len(attrs)%2 != 0 {
		B.Add(EcpFromProto(key.Ipk.HAttrs[len(attrs)-1]).Mul(attrs[len(attrs)-1]))
	}

	Exp := Modadd(FP256BN.FromBytes(key.GetIsk()), E, GroupOrder)
	Exp.Invmodp(GroupOrder)
	A := B.Mul(Exp)

	CredAttrs := make([][]byte, len(attrs))
	for index, attribute := range attrs {
		CredAttrs[index] = BigToBytes(attribute)
	}

	return &Credential{
		A:     EcpToProto(A),
		B:     EcpToProto(B),
		E:     BigToBytes(E),
		S:     BigToBytes(S),
		Attrs: CredAttrs}, nil
}

// Ver cryptographically verifies the credential by verifying the signature
// on the attribute values and user's secret key
func (cred *Credential) Ver(sk *FP256BN.BIG, ipk *IssuerPublicKey) error {

	// parse the credential
	A := EcpFromProto(cred.GetA())
	B := EcpFromProto(cred.GetB())
	E := FP256BN.FromBytes(cred.GetE())
	S := FP256BN.FromBytes(cred.GetS())

	// verify that all attribute values are present
	for i := 0; i < len(cred.GetAttrs()); i++ {
		if cred.Attrs[i] == nil {
			return errors.Errorf("credential has no value for attribute %s", ipk.AttributeNames[i])
		}
	}

	// verify cryptographic signature on the attributes and the user secret key
	BPrime := FP256BN.NewECP()
	BPrime.Copy(GenG1)
	BPrime.Add(EcpFromProto(ipk.HSk).Mul2(sk, EcpFromProto(ipk.HRand), S))
	for i := 0; i < len(cred.Attrs)/2; i++ {
		BPrime.Add(EcpFromProto(ipk.HAttrs[2*i]).Mul2(FP256BN.FromBytes(cred.Attrs[2*i]), EcpFromProto(ipk.HAttrs[2*i+1]), FP256BN.FromBytes(cred.Attrs[2*i+1])))
	}
	if len(cred.Attrs)%2 != 0 {
		BPrime.Add(EcpFromProto(ipk.HAttrs[len(cred.Attrs)-1]).Mul(FP256BN.FromBytes(cred.Attrs[len(cred.Attrs)-1])))
	}
	if !B.Equals(BPrime) {
		return errors.Errorf("b-value from credential does not match the attribute values")
	}

	a := GenG2.Mul(E)
	a.Add(Ecp2FromProto(ipk.W))
	a.Affine()

	if !FP256BN.Fexp(FP256BN.Ate(a, A)).Equals(FP256BN.Fexp(FP256BN.Ate(GenG2, B))) {
		return errors.Errorf("credential is not cryptographically valid")
	}
	return nil
}
