/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"github.com/milagro-crypto/amcl/version3/go/amcl"
	"github.com/milagro-crypto/amcl/version3/go/amcl/FP256BN"
	"github.com/pkg/errors"
)

// credRequestLabel is the label used in zero-knowledge proof (ZKP) to identify that this ZKP is a credential request
const credRequestLabel = "credRequest"

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

// NewCredRequest creates a new Credential Request, the first message of the interactive credential issuance protocol (from user to issuer)
func NewCredRequest(sk *FP256BN.BIG, credS1 *FP256BN.BIG, IssuerNonce *FP256BN.BIG, ipk *IssuerPublicKey, rng *amcl.RAND) *CredRequest {
	HSk := EcpFromProto(ipk.HSk)
	HRand := EcpFromProto(ipk.HRand)
	Nym := HSk.Mul2(sk, HRand, credS1)

	// Create ZK Proof
	rSk := RandModOrder(rng)
	rRand := RandModOrder(rng)
	t := HSk.Mul2(rSk, HRand, rRand)

	// proofData is the data being hashed, it consists of:
	// the credential request label
	// 3 elements of G1 each taking 2*FieldBytes+1 bytes
	// hash of the issuer public key of length FieldBytes
	// issuer nonce of length FieldBytes
	proofData := make([]byte, len([]byte(credRequestLabel))+3*(2*FieldBytes+1)+2*FieldBytes)
	index := 0
	index = appendBytesString(proofData, index, credRequestLabel)
	index = appendBytesG1(proofData, index, t)
	index = appendBytesG1(proofData, index, HSk)
	index = appendBytesG1(proofData, index, Nym)
	index = appendBytesBig(proofData, index, IssuerNonce)
	copy(proofData[index:], ipk.Hash)

	proofC := HashModOrder(proofData)
	proofS1 := Modadd(FP256BN.Modmul(proofC, sk, GroupOrder), rSk, GroupOrder)
	proofS2 := Modadd(FP256BN.Modmul(proofC, credS1, GroupOrder), rRand, GroupOrder)

	return &CredRequest{EcpToProto(Nym), BigToBytes(IssuerNonce), BigToBytes(proofC), BigToBytes(proofS1), BigToBytes(proofS2)}
}

// Check cryptographically verifies the credential request
func (m *CredRequest) Check(ipk *IssuerPublicKey) error {
	Nym := EcpFromProto(m.GetNym())
	IssuerNonce := FP256BN.FromBytes(m.GetIssuerNonce())
	ProofC := FP256BN.FromBytes(m.GetProofC())
	ProofS1 := FP256BN.FromBytes(m.GetProofS1())
	ProofS2 := FP256BN.FromBytes(m.GetProofS2())

	HSk := EcpFromProto(ipk.HSk)
	HRand := EcpFromProto(ipk.HRand)

	if Nym == nil || IssuerNonce == nil || ProofC == nil || ProofS1 == nil || ProofS2 == nil {
		return errors.Errorf("one of the proof values is undefined")
	}

	t := HSk.Mul2(ProofS1, HRand, ProofS2)
	t.Sub(Nym.Mul(ProofC))

	// proofData is the data being hashed, it consists of:
	// the credential request label
	// 3 elements of G1 each taking 2*FieldBytes+1 bytes
	// hash of the issuer public key of length FieldBytes
	// issuer nonce of length FieldBytes
	proofData := make([]byte, len([]byte(credRequestLabel))+3*(2*FieldBytes+1)+2*FieldBytes)
	index := 0
	index = appendBytesString(proofData, index, credRequestLabel)
	index = appendBytesG1(proofData, index, t)
	index = appendBytesG1(proofData, index, HSk)
	index = appendBytesG1(proofData, index, Nym)
	index = appendBytesBig(proofData, index, IssuerNonce)
	copy(proofData[index:], ipk.Hash)

	if *ProofC != *HashModOrder(proofData) {
		return errors.Errorf("zero knowledge proof is invalid")
	}

	return nil
}
