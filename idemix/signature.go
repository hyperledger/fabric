/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	amcl "github.com/manudrijvers/amcl/go"
	"github.com/pkg/errors"
)

// signLabel is the label used in zero-knowledge proof (ZKP) to identify that this ZKP is a signature of knowledge
const signLabel = "sign"

// A signature that is produced using an Identity Mixer credential is a so-called signature of knowledge
// (for details see C.P.Schnorr "Efficient Identification and Signatures for Smart Cards")
// An Identity Mixer signature is a signature of knowledge that signs a message and proves (in zero-knowledge)
// the knowledge of the user secret (and possibly attributes) signed inside a credential
// that was issued by a certain issuer (referred to with the issuer public key)
// The signature is verified using the message being signed and the public key of the issuer
// Some of the attributes from the credential can be selectvely disclosed or different statements can be proven about
// credential atrributes without diclosing them in the clear
// The difference between a standard signature using X.509 certificates and an Identity Mixer signature is
// the advanced privacy features provided by Identity Mixer (due to zero-knowledge proofs):
//  - Unlinkability of the signatures produced with the same credential
//  - Selective attribute disclosure and predicates over attributes

// Make a slice of all the attribute indices that will not be disclosed
func hiddenIndices(Disclosure []byte) []int {
	HiddenIndices := make([]int, 0)
	for index, disclose := range Disclosure {
		if disclose == 0 {
			HiddenIndices = append(HiddenIndices, index)
		}
	}
	return HiddenIndices
}

// NewSignature creates a new idemix signature (Schnorr-type signature)
// The []byte Disclosure steers which attributes are disclosed:
// if Disclosure[i] == 0 then attribute i remains hidden and otherwise it is disclosed.
// We use the zero-knowledge proof by http://eprint.iacr.org/2016/663.pdf to prove knowledge of a BBS+ signature
func NewSignature(cred *Credential, sk *amcl.BIG, Nym *amcl.ECP, RNym *amcl.BIG, ipk *IssuerPublicKey, Disclosure []byte, msg []byte, rng *amcl.RAND) *Signature {
	HiddenIndices := hiddenIndices(Disclosure)

	// Start sig
	r1 := RandModOrder(rng)
	r2 := RandModOrder(rng)
	r3 := amcl.NewBIGcopy(r1)
	r3.Invmodp(GroupOrder)

	Nonce := RandModOrder(rng)

	A := EcpFromProto(cred.A)
	B := EcpFromProto(cred.B)

	APrime := amcl.G1mul(A, r1) // A' = A^{r1}
	ABar := amcl.G1mul(B, r1)
	ABar.Sub(amcl.G1mul(APrime, amcl.FromBytes(cred.E))) // barA = A'^{-e} b^{r1}

	BPrime := amcl.G1mul(B, r1)
	HRand := EcpFromProto(ipk.HRand)
	HSk := EcpFromProto(ipk.HSk)

	BPrime.Sub(amcl.G1mul(HRand, r2)) // b' = b^{r1} h_r^{-r2}

	S := amcl.FromBytes(cred.S)
	E := amcl.FromBytes(cred.E)
	sPrime := amcl.Modsub(S, amcl.Modmul(r2, r3, GroupOrder), GroupOrder)

	// Construct ZK proof
	rSk := RandModOrder(rng)
	re := RandModOrder(rng)
	rR2 := RandModOrder(rng)
	rR3 := RandModOrder(rng)
	rSPrime := RandModOrder(rng)
	rRNym := RandModOrder(rng)
	rAttrs := make([]*amcl.BIG, len(HiddenIndices))
	for i := range HiddenIndices {
		rAttrs[i] = RandModOrder(rng)
	}

	t1 := APrime.Mul2(re, HRand, rR2)
	t2 := amcl.G1mul(HRand, rSPrime)
	t2.Add(BPrime.Mul2(rR3, HSk, rSk))

	for i := 0; i < len(HiddenIndices)/2; i++ {
		t2.Add(EcpFromProto(ipk.HAttrs[HiddenIndices[2*i]]).Mul2(rAttrs[2*i], EcpFromProto(ipk.HAttrs[HiddenIndices[2*i+1]]), rAttrs[2*i+1]))
	}
	if len(HiddenIndices)%2 != 0 {
		t2.Add(amcl.G1mul(EcpFromProto(ipk.HAttrs[HiddenIndices[len(HiddenIndices)-1]]), rAttrs[len(HiddenIndices)-1]))
	}

	t3 := HSk.Mul2(rSk, HRand, rRNym)

	// proofData is the data being hashed, it consists of:
	// the signature label
	// 7 elements of G1 each taking 2*FieldBytes+1 bytes
	// one bigint (hash of the issuer public key) of length FieldBytes
	// disclosed attributes
	// message being signed
	proofData := make([]byte, len([]byte(signLabel))+7*(2*FieldBytes+1)+FieldBytes+len(Disclosure)+len(msg))
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG1(proofData, index, t3)
	index = appendBytesG1(proofData, index, APrime)
	index = appendBytesG1(proofData, index, ABar)
	index = appendBytesG1(proofData, index, BPrime)
	index = appendBytesG1(proofData, index, Nym)
	copy(proofData[index:], ipk.Hash)
	index = index + FieldBytes
	copy(proofData[index:], Disclosure)
	index = index + len(Disclosure)
	copy(proofData[index:], msg)
	c := HashModOrder(proofData)

	// add the previous hash and the nonce and hash again to compute a second hash (C value)
	index = 0
	proofData = proofData[:2*FieldBytes]
	index = appendBytesBig(proofData, index, c)
	index = appendBytesBig(proofData, index, Nonce)
	ProofC := HashModOrder(proofData)
	ProofSSk := amcl.Modadd(rSk, amcl.Modmul(ProofC, sk, GroupOrder), GroupOrder)
	ProofSE := amcl.Modsub(re, amcl.Modmul(ProofC, E, GroupOrder), GroupOrder)
	ProofSR2 := amcl.Modadd(rR2, amcl.Modmul(ProofC, r2, GroupOrder), GroupOrder)
	ProofSR3 := amcl.Modsub(rR3, amcl.Modmul(ProofC, r3, GroupOrder), GroupOrder)
	ProofSSPrime := amcl.Modadd(rSPrime, amcl.Modmul(ProofC, sPrime, GroupOrder), GroupOrder)
	ProofSRNym := amcl.Modadd(rRNym, amcl.Modmul(ProofC, RNym, GroupOrder), GroupOrder)

	ProofSAttrs := make([][]byte, len(HiddenIndices))
	for i, j := range HiddenIndices {
		ProofSAttrs[i] = BigToBytes(amcl.Modadd(rAttrs[i], amcl.Modmul(ProofC, amcl.FromBytes(cred.Attrs[j]), GroupOrder), GroupOrder))
	}

	return &Signature{
		EcpToProto(APrime),
		EcpToProto(ABar),
		EcpToProto(BPrime),
		BigToBytes(ProofC),
		BigToBytes(ProofSSk),
		BigToBytes(ProofSE),
		BigToBytes(ProofSR2),
		BigToBytes(ProofSR3),
		BigToBytes(ProofSSPrime),
		ProofSAttrs,
		BigToBytes(Nonce),
		EcpToProto(Nym),
		BigToBytes(ProofSRNym)}
}

// Ver verifies an idemix signature
// Disclosure steers which attributes it expects to be disclosed
// attributeValues[i] contains the desired attribute value for the i-th undisclosed attribute in Disclosure
func (sig *Signature) Ver(Disclosure []byte, ipk *IssuerPublicKey, msg []byte, attributeValues []*amcl.BIG) error {
	HiddenIndices := hiddenIndices(Disclosure)

	APrime := EcpFromProto(sig.GetAPrime())
	ABar := EcpFromProto(sig.GetABar())
	BPrime := EcpFromProto(sig.GetBPrime())
	Nym := EcpFromProto(sig.GetNym())
	ProofC := amcl.FromBytes(sig.GetProofC())
	ProofSSk := amcl.FromBytes(sig.GetProofSSk())
	ProofSE := amcl.FromBytes(sig.GetProofSE())
	ProofSR2 := amcl.FromBytes(sig.GetProofSR2())
	ProofSR3 := amcl.FromBytes(sig.GetProofSR3())
	ProofSSPrime := amcl.FromBytes(sig.GetProofSSPrime())
	ProofSRNym := amcl.FromBytes(sig.GetProofSRNym())
	ProofSAttrs := make([]*amcl.BIG, len(sig.GetProofSAttrs()))

	if len(sig.ProofSAttrs) != len(HiddenIndices) {
		return errors.Errorf("signature invalid: incorrect amount of s-values for AttributeProofSpec")
	}
	for i, b := range sig.ProofSAttrs {
		ProofSAttrs[i] = amcl.FromBytes(b)
	}

	Nonce := amcl.FromBytes(sig.GetNonce())

	W := Ecp2FromProto(ipk.W)
	HRand := EcpFromProto(ipk.HRand)
	HSk := EcpFromProto(ipk.HSk)

	if APrime.Is_infinity() {
		return errors.Errorf("signature invalid: APrime = 1")
	}
	temp1 := amcl.Ate(W, APrime)
	temp2 := amcl.Ate(GenG2, ABar)
	temp2.Inverse()
	temp1.Mul(temp2)
	if !amcl.Fexp(temp1).Isunity() {
		return errors.Errorf("signature invalid: APrime and ABar don't have the expected structure")
	}

	t1 := APrime.Mul2(ProofSE, HRand, ProofSR2)
	temp := amcl.NewECP()
	temp.Copy(ABar)
	temp.Sub(BPrime)
	t1.Sub(amcl.G1mul(temp, ProofC))

	t2 := amcl.G1mul(HRand, ProofSSPrime)
	t2.Add(BPrime.Mul2(ProofSR3, HSk, ProofSSk))

	for i := 0; i < len(HiddenIndices)/2; i++ {
		t2.Add(EcpFromProto(ipk.HAttrs[HiddenIndices[2*i]]).Mul2(ProofSAttrs[2*i], EcpFromProto(ipk.HAttrs[HiddenIndices[2*i+1]]), ProofSAttrs[2*i+1]))
	}
	if len(HiddenIndices)%2 != 0 {
		t2.Add(amcl.G1mul(EcpFromProto(ipk.HAttrs[HiddenIndices[len(HiddenIndices)-1]]), ProofSAttrs[len(HiddenIndices)-1]))
	}

	temp = amcl.NewECP()
	temp.Copy(GenG1)

	for index, disclose := range Disclosure {
		if disclose != 0 {
			temp.Add(amcl.G1mul(EcpFromProto(ipk.HAttrs[index]), attributeValues[index]))
		}
	}
	t2.Add(amcl.G1mul(temp, ProofC))

	t3 := HSk.Mul2(ProofSSk, HRand, ProofSRNym)
	t3.Sub(Nym.Mul(ProofC))

	// proofData is the data being hashed, it consists of:
	// the signature label
	// 7 elements of G1 each taking 2*FieldBytes+1 bytes
	// one bigint (hash of the issuer public key) of length FieldBytes
	// disclosed attributes
	// message that was signed
	proofData := make([]byte, len([]byte(signLabel))+7*(2*FieldBytes+1)+FieldBytes+len(Disclosure)+len(msg))
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG1(proofData, index, t3)
	index = appendBytesG1(proofData, index, APrime)
	index = appendBytesG1(proofData, index, ABar)
	index = appendBytesG1(proofData, index, BPrime)
	index = appendBytesG1(proofData, index, Nym)
	copy(proofData[index:], ipk.Hash)
	index = index + FieldBytes
	copy(proofData[index:], Disclosure)
	index = index + len(Disclosure)
	copy(proofData[index:], msg)

	c := HashModOrder(proofData)
	index = 0
	proofData = proofData[:2*FieldBytes]
	index = appendBytesBig(proofData, index, c)
	index = appendBytesBig(proofData, index, Nonce)
	if !ProofC.Equals(HashModOrder(proofData)) {
		return errors.Errorf("signature invalid: zero-knowledge proof is invalid")
	}

	return nil
}
