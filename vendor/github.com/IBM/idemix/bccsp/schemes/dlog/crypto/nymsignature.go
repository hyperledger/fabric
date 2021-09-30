/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"io"

	math "github.com/IBM/mathlib"
	"github.com/pkg/errors"
)

// NewSignature creates a new idemix pseudonym signature
func (i *Idemix) NewNymSignature(sk *math.Zr, Nym *math.G1, RNym *math.Zr, ipk *IssuerPublicKey, msg []byte, rng io.Reader, tr Translator) (*NymSignature, error) {
	return newNymSignature(sk, Nym, RNym, ipk, msg, rng, i.Curve, tr)
}

func newNymSignature(sk *math.Zr, Nym *math.G1, RNym *math.Zr, ipk *IssuerPublicKey, msg []byte, rng io.Reader, curve *math.Curve, tr Translator) (*NymSignature, error) {
	// Validate inputs
	if sk == nil || Nym == nil || RNym == nil || ipk == nil || rng == nil {
		return nil, errors.Errorf("cannot create NymSignature: received nil input")
	}

	Nonce := curve.NewRandomZr(rng)

	HRand, err := tr.G1FromProto(ipk.HRand)
	if err != nil {
		return nil, err
	}

	HSk, err := tr.G1FromProto(ipk.HSk)
	if err != nil {
		return nil, err
	}

	// The rest of this function constructs the non-interactive zero knowledge proof proving that
	// the signer 'owns' this pseudonym, i.e., it knows the secret key and randomness on which it is based.
	// Recall that (Nym,RNym) is the output of MakeNym. Therefore, Nym = h_{sk}^sk \cdot h_r^r

	// Sample the randomness needed for the proof
	rSk := curve.NewRandomZr(rng)
	rRNym := curve.NewRandomZr(rng)

	// Step 1: First message (t-values)
	t := HSk.Mul2(rSk, HRand, rRNym) // t = h_{sk}^{r_sk} \cdot h_r^{r_{RNym}

	// Step 2: Compute the Fiat-Shamir hash, forming the challenge of the ZKP.
	// proofData will hold the data being hashed, it consists of:
	// - the signature label
	// - 2 elements of G1 each taking 2*math.FieldBytes+1 bytes
	// - one bigint (hash of the issuer public key) of length math.FieldBytes
	// - disclosed attributes
	// - message being signed
	proofData := make([]byte, len([]byte(signLabel))+2*(2*curve.FieldBytes+1)+curve.FieldBytes+len(msg))
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t)
	index = appendBytesG1(proofData, index, Nym)
	copy(proofData[index:], ipk.Hash)
	index = index + curve.FieldBytes
	copy(proofData[index:], msg)
	c := curve.HashToZr(proofData)
	// combine the previous hash and the nonce and hash again to compute the final Fiat-Shamir value 'ProofC'
	index = 0
	proofData = proofData[:2*curve.FieldBytes]
	index = appendBytesBig(proofData, index, c)
	index = appendBytesBig(proofData, index, Nonce)
	ProofC := curve.HashToZr(proofData)

	// Step 3: reply to the challenge message (s-values)
	ProofSSk := curve.ModAdd(rSk, curve.ModMul(ProofC, sk, curve.GroupOrder), curve.GroupOrder)       // s_{sk} = r_{sk} + C \cdot sk
	ProofSRNym := curve.ModAdd(rRNym, curve.ModMul(ProofC, RNym, curve.GroupOrder), curve.GroupOrder) // s_{RNym} = r_{RNym} + C \cdot RNym

	// The signature consists of the Fiat-Shamir hash (ProofC), the s-values (ProofSSk, ProofSRNym), and the nonce.
	return &NymSignature{
		ProofC:     ProofC.Bytes(),
		ProofSSk:   ProofSSk.Bytes(),
		ProofSRNym: ProofSRNym.Bytes(),
		Nonce:      Nonce.Bytes()}, nil
}

// Ver verifies an idemix NymSignature
func (sig *NymSignature) Ver(nym *math.G1, ipk *IssuerPublicKey, msg []byte, curve *math.Curve, tr Translator) error {
	ProofC := curve.NewZrFromBytes(sig.GetProofC())
	ProofSSk := curve.NewZrFromBytes(sig.GetProofSSk())
	ProofSRNym := curve.NewZrFromBytes(sig.GetProofSRNym())
	Nonce := curve.NewZrFromBytes(sig.GetNonce())

	HRand, err := tr.G1FromProto(ipk.HRand)
	if err != nil {
		return err
	}

	HSk, err := tr.G1FromProto(ipk.HSk)
	if err != nil {
		return err
	}

	// Verify Proof

	// Recompute t-values using s-values
	t := HSk.Mul2(ProofSSk, HRand, ProofSRNym)
	t.Sub(nym.Mul(ProofC)) // t = h_{sk}^{s_{sk} \ cdot h_r^{s_{RNym}

	// Recompute challenge
	proofData := make([]byte, len([]byte(signLabel))+2*(2*curve.FieldBytes+1)+curve.FieldBytes+len(msg))
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t)
	index = appendBytesG1(proofData, index, nym)
	copy(proofData[index:], ipk.Hash)
	index = index + curve.FieldBytes
	copy(proofData[index:], msg)
	c := curve.HashToZr(proofData)
	index = 0
	proofData = proofData[:2*curve.FieldBytes]
	index = appendBytesBig(proofData, index, c)
	index = appendBytesBig(proofData, index, Nonce)

	if !ProofC.Equals(curve.HashToZr(proofData)) {
		return errors.Errorf("pseudonym signature invalid: zero-knowledge proof is invalid")
	}

	return nil
}
