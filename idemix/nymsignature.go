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

// NewSignature creates a new idemix pseudonym signature
func NewNymSignature(sk *FP256BN.BIG, Nym *FP256BN.ECP, RNym *FP256BN.BIG, ipk *IssuerPublicKey, msg []byte, rng *amcl.RAND) (*NymSignature, error) {
	// Validate inputs
	if sk == nil || Nym == nil || RNym == nil || ipk == nil || rng == nil {
		return nil, errors.Errorf("cannot create NymSignature: received nil input")
	}

	Nonce := RandModOrder(rng)

	HRand := EcpFromProto(ipk.HRand)
	HSk := EcpFromProto(ipk.HSk)

	// The rest of this function constructs the non-interactive zero knowledge proof proving that
	// the signer 'owns' this pseudonym, i.e., it knows the secret key and randomness on which it is based.
	// Recall that (Nym,RNym) is the output of MakeNym. Therefore, Nym = h_{sk}^sk \cdot h_r^r

	// Sample the randomness needed for the proof
	rSk := RandModOrder(rng)
	rRNym := RandModOrder(rng)

	// Step 1: First message (t-values)
	t := HSk.Mul2(rSk, HRand, rRNym) // t = h_{sk}^{r_sk} \cdot h_r^{r_{RNym}

	// Step 2: Compute the Fiat-Shamir hash, forming the challenge of the ZKP.
	// proofData will hold the data being hashed, it consists of:
	// - the signature label
	// - 2 elements of G1 each taking 2*FieldBytes+1 bytes
	// - one bigint (hash of the issuer public key) of length FieldBytes
	// - disclosed attributes
	// - message being signed
	proofData := make([]byte, len([]byte(signLabel))+2*(2*FieldBytes+1)+FieldBytes+len(msg))
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t)
	index = appendBytesG1(proofData, index, Nym)
	copy(proofData[index:], ipk.Hash)
	index = index + FieldBytes
	copy(proofData[index:], msg)
	c := HashModOrder(proofData)
	// combine the previous hash and the nonce and hash again to compute the final Fiat-Shamir value 'ProofC'
	index = 0
	proofData = proofData[:2*FieldBytes]
	index = appendBytesBig(proofData, index, c)
	index = appendBytesBig(proofData, index, Nonce)
	ProofC := HashModOrder(proofData)

	// Step 3: reply to the challenge message (s-values)
	ProofSSk := Modadd(rSk, FP256BN.Modmul(ProofC, sk, GroupOrder), GroupOrder)       // s_{sk} = r_{sk} + C \cdot sk
	ProofSRNym := Modadd(rRNym, FP256BN.Modmul(ProofC, RNym, GroupOrder), GroupOrder) // s_{RNym} = r_{RNym} + C \cdot RNym

	// The signature consists of the Fiat-Shamir hash (ProofC), the s-values (ProofSSk, ProofSRNym), and the nonce.
	return &NymSignature{
		ProofC:     BigToBytes(ProofC),
		ProofSSk:   BigToBytes(ProofSSk),
		ProofSRNym: BigToBytes(ProofSRNym),
		Nonce:      BigToBytes(Nonce)}, nil
}

// Ver verifies an idemix NymSignature
func (sig *NymSignature) Ver(nym *FP256BN.ECP, ipk *IssuerPublicKey, msg []byte) error {
	ProofC := FP256BN.FromBytes(sig.GetProofC())
	ProofSSk := FP256BN.FromBytes(sig.GetProofSSk())
	ProofSRNym := FP256BN.FromBytes(sig.GetProofSRNym())
	Nonce := FP256BN.FromBytes(sig.GetNonce())

	HRand := EcpFromProto(ipk.HRand)
	HSk := EcpFromProto(ipk.HSk)

	// Verify Proof

	// Recompute t-values using s-values
	t := HSk.Mul2(ProofSSk, HRand, ProofSRNym)
	t.Sub(nym.Mul(ProofC)) // t = h_{sk}^{s_{sk} \ cdot h_r^{s_{RNym}

	// Recompute challenge
	proofData := make([]byte, len([]byte(signLabel))+2*(2*FieldBytes+1)+FieldBytes+len(msg))
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t)
	index = appendBytesG1(proofData, index, nym)
	copy(proofData[index:], ipk.Hash)
	index = index + FieldBytes
	copy(proofData[index:], msg)
	c := HashModOrder(proofData)
	index = 0
	proofData = proofData[:2*FieldBytes]
	index = appendBytesBig(proofData, index, c)
	index = appendBytesBig(proofData, index, Nonce)

	if *ProofC != *HashModOrder(proofData) {
		return errors.Errorf("pseudonym signature invalid: zero-knowledge proof is invalid")
	}

	return nil
}
