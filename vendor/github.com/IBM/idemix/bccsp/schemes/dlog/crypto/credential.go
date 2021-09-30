/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"io"

	amcl "github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
	"github.com/pkg/errors"
)

type Translator interface {
	G1ToProto(*math.G1) *amcl.ECP
	G1FromProto(*amcl.ECP) (*math.G1, error)
	G1FromRawBytes([]byte) (*math.G1, error)
	G2ToProto(*math.G2) *amcl.ECP2
	G2FromProto(*amcl.ECP2) (*math.G2, error)
}

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
func (i *Idemix) NewCredential(key *IssuerKey, m *CredRequest, attrs []*math.Zr, rng io.Reader, t Translator) (*Credential, error) {
	return newCredential(key, m, attrs, rng, t, i.Curve)
}

func newCredential(key *IssuerKey, m *CredRequest, attrs []*math.Zr, rng io.Reader, t Translator, curve *math.Curve) (*Credential, error) {
	// check the credential request that contains
	err := m.Check(key.Ipk, curve, t)
	if err != nil {
		return nil, err
	}

	if len(attrs) != len(key.Ipk.AttributeNames) {
		return nil, errors.Errorf("incorrect number of attribute values passed")
	}

	// Place a BBS+ signature on the user key and the attribute values
	// (For BBS+, see e.g. "Constant-Size Dynamic k-TAA" by Man Ho Au, Willy Susilo, Yi Mu)
	// or http://eprint.iacr.org/2016/663.pdf, Sec. 4.3.

	// For a credential, a BBS+ signature consists of the following three elements:
	// 1. E, random value in the proper group
	// 2. S, random value in the proper group
	// 3. A as B^Exp where B=g_1 \cdot h_r^s \cdot h_sk^sk \cdot \prod_{i=1}^L h_i^{m_i} and Exp = \frac{1}{e+x}
	// Notice that:
	// h_r is h_0 in http://eprint.iacr.org/2016/663.pdf, Sec. 4.3.

	// Pick randomness E and S
	E := curve.NewRandomZr(rng)
	S := curve.NewRandomZr(rng)

	// Set B as g_1 \cdot h_r^s \cdot h_sk^sk \cdot \prod_{i=1}^L h_i^{m_i} and Exp = \frac{1}{e+x}
	B := curve.NewG1()
	B.Clone(curve.GenG1) // g_1
	Nym, err := t.G1FromProto(m.Nym)
	if err != nil {
		return nil, err
	}
	B.Add(Nym) // in this case, recall Nym=h_sk^sk
	HRand, err := t.G1FromProto(key.Ipk.HRand)
	if err != nil {
		return nil, err
	}
	B.Add(HRand.Mul(S)) // h_r^s

	HAttrs := make([]*math.G1, len(key.Ipk.HAttrs))
	for i := range key.Ipk.HAttrs {
		var err error
		HAttrs[i], err = t.G1FromProto(key.Ipk.HAttrs[i])
		if err != nil {
			return nil, err
		}
	}

	// Append attributes
	// Use Mul2 instead of Mul as much as possible for efficiency reasones
	for i := 0; i < len(attrs)/2; i++ {
		B.Add(
			// Add two attributes in one shot
			HAttrs[2*i].Mul2(
				attrs[2*i],
				HAttrs[2*i+1],
				attrs[2*i+1],
			),
		)
	}
	// Check for residue in case len(attrs)%2 is odd
	if len(attrs)%2 != 0 {
		B.Add(HAttrs[len(attrs)-1].Mul(attrs[len(attrs)-1]))
	}

	// Set Exp as \frac{1}{e+x}
	Exp := curve.ModAdd(curve.NewZrFromBytes(key.GetIsk()), E, curve.GroupOrder)
	Exp.InvModP(curve.GroupOrder)
	// Finalise A as B^Exp
	A := B.Mul(Exp)
	// The signature is now generated.

	// Notice that here we release also B, this does not harm security cause
	// it can be compute publicly from the BBS+ signature itself.
	CredAttrs := make([][]byte, len(attrs))
	for index, attribute := range attrs {
		CredAttrs[index] = attribute.Bytes()
	}

	return &Credential{
		A:     t.G1ToProto(A),
		B:     t.G1ToProto(B),
		E:     E.Bytes(),
		S:     S.Bytes(),
		Attrs: CredAttrs}, nil
}

// Ver cryptographically verifies the credential by verifying the signature
// on the attribute values and user's secret key
func (cred *Credential) Ver(sk *math.Zr, ipk *IssuerPublicKey, curve *math.Curve, t Translator) error {
	// Validate Input

	// - parse the credential
	A, err := t.G1FromProto(cred.GetA())
	if err != nil {
		return err
	}
	B, err := t.G1FromProto(cred.GetB())
	if err != nil {
		return err
	}
	E := curve.NewZrFromBytes(cred.GetE())
	S := curve.NewZrFromBytes(cred.GetS())

	// - verify that all attribute values are present
	for i := 0; i < len(cred.GetAttrs()); i++ {
		if cred.Attrs[i] == nil {
			return errors.Errorf("credential has no value for attribute %s", ipk.AttributeNames[i])
		}
	}

	HSk, err := t.G1FromProto(ipk.HSk)
	if err != nil {
		return err
	}

	HRand, err := t.G1FromProto(ipk.HRand)
	if err != nil {
		return err
	}

	HAttrs := make([]*math.G1, len(ipk.HAttrs))
	for i := range ipk.HAttrs {
		var err error
		HAttrs[i], err = t.G1FromProto(ipk.HAttrs[i])
		if err != nil {
			return err
		}
	}

	// - verify cryptographic signature on the attributes and the user secret key
	BPrime := curve.NewG1()
	BPrime.Clone(curve.GenG1)
	BPrime.Add(HSk.Mul2(sk, HRand, S))
	for i := 0; i < len(cred.Attrs)/2; i++ {
		BPrime.Add(
			HAttrs[2*i].Mul2(
				curve.NewZrFromBytes(cred.Attrs[2*i]),
				HAttrs[2*i+1],
				curve.NewZrFromBytes(cred.Attrs[2*i+1]),
			),
		)
	}
	if len(cred.Attrs)%2 != 0 {
		BPrime.Add(HAttrs[len(cred.Attrs)-1].Mul(curve.NewZrFromBytes(cred.Attrs[len(cred.Attrs)-1])))
	}
	if !B.Equals(BPrime) {
		return errors.Errorf("b-value from credential does not match the attribute values")
	}

	W, err := t.G2FromProto(ipk.W)
	if err != nil {
		return err
	}

	// Verify BBS+ signature. Namely: e(w \cdot g_2^e, A) =? e(g_2, B)
	a := curve.GenG2.Mul(E)
	a.Add(W)
	a.Affine()

	left := curve.FExp(curve.Pairing(a, A))
	right := curve.FExp(curve.Pairing(curve.GenG2, B))

	if !left.Equals(right) {
		return errors.Errorf("credential is not cryptographically valid")
	}

	return nil
}
