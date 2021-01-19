/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	// "crypto/ecdsa"
	"sort"

	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
	"github.com/tjfoc/gmsm/sm2"
)

var idemixLogger = flogging.MustGetLogger("idemix")

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
// We require the revocation handle to remain undisclosed (i.e., Disclosure[rhIndex] == 0).
// We use the zero-knowledge proof by http://eprint.iacr.org/2016/663.pdf, Sec. 4.5 to prove knowledge of a BBS+ signature
func NewSignature(cred *Credential, sk *FP256BN.BIG, Nym *FP256BN.ECP, RNym *FP256BN.BIG, ipk *IssuerPublicKey, Disclosure []byte, msg []byte, rhIndex int, cri *CredentialRevocationInformation, rng *amcl.RAND) (*Signature, error) {
	// Validate inputs
	if cred == nil || sk == nil || Nym == nil || RNym == nil || ipk == nil || rng == nil || cri == nil {
		return nil, errors.Errorf("cannot create idemix signature: received nil input")
	}

	if rhIndex < 0 || rhIndex >= len(ipk.AttributeNames) || len(Disclosure) != len(ipk.AttributeNames) {
		return nil, errors.Errorf("cannot create idemix signature: received invalid input")
	}

	if cri.RevocationAlg != int32(ALG_NO_REVOCATION) && Disclosure[rhIndex] == 1 {
		return nil, errors.Errorf("Attribute %d is disclosed but also used as revocation handle attribute, which should remain hidden.", rhIndex)
	}

	// locate the indices of the attributes to hide and sample randomness for them
	HiddenIndices := hiddenIndices(Disclosure)

	// Generate required randomness r_1, r_2
	r1 := RandModOrder(rng)
	r2 := RandModOrder(rng)
	// Set r_3 as \frac{1}{r_1}
	r3 := FP256BN.NewBIGcopy(r1)
	r3.Invmodp(GroupOrder)

	// Sample a nonce
	Nonce := RandModOrder(rng)

	// Parse credential
	A := EcpFromProto(cred.A)
	B := EcpFromProto(cred.B)

	// Randomize credential

	// Compute A' as A^{r_!}
	APrime := FP256BN.G1mul(A, r1)

	// Compute ABar as A'^{-e} b^{r1}
	ABar := FP256BN.G1mul(B, r1)
	ABar.Sub(FP256BN.G1mul(APrime, FP256BN.FromBytes(cred.E)))

	// Compute B' as b^{r1} / h_r^{r2}, where h_r is h_r
	BPrime := FP256BN.G1mul(B, r1)
	HRand := EcpFromProto(ipk.HRand)
	// Parse h_{sk} from ipk
	HSk := EcpFromProto(ipk.HSk)

	BPrime.Sub(FP256BN.G1mul(HRand, r2))

	S := FP256BN.FromBytes(cred.S)
	E := FP256BN.FromBytes(cred.E)

	// Compute s' as s - r_2 \cdot r_3
	sPrime := Modsub(S, FP256BN.Modmul(r2, r3, GroupOrder), GroupOrder)

	// The rest of this function constructs the non-interactive zero knowledge proof
	// that links the signature, the non-disclosed attributes and the nym.

	// Sample the randomness used to compute the commitment values (aka t-values) for the ZKP
	rSk := RandModOrder(rng)
	re := RandModOrder(rng)
	rR2 := RandModOrder(rng)
	rR3 := RandModOrder(rng)
	rSPrime := RandModOrder(rng)
	rRNym := RandModOrder(rng)

	rAttrs := make([]*FP256BN.BIG, len(HiddenIndices))
	for i := range HiddenIndices {
		rAttrs[i] = RandModOrder(rng)
	}

	// First compute the non-revocation proof.
	// The challenge of the ZKP needs to depend on it, as well.
	prover, err := getNonRevocationProver(RevocationAlgorithm(cri.RevocationAlg))
	if err != nil {
		return nil, err
	}
	nonRevokedProofHashData, err := prover.getFSContribution(
		FP256BN.FromBytes(cred.Attrs[rhIndex]),
		rAttrs[sort.SearchInts(HiddenIndices, rhIndex)],
		cri,
		rng,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute non-revoked proof")
	}

	// Step 1: First message (t-values)

	// t1 is related to knowledge of the credential (recall, it is a BBS+ signature)
	t1 := APrime.Mul2(re, HRand, rR2) // A'^{r_E} \cdot h_r^{r_{r2}}

	// t2: is related to knowledge of the non-disclosed attributes that signed  in (A,B,S,E)
	t2 := FP256BN.G1mul(HRand, rSPrime) // h_r^{r_{s'}}
	t2.Add(BPrime.Mul2(rR3, HSk, rSk))  // B'^{r_{r3}} \cdot h_{sk}^{r_{sk}}
	for i := 0; i < len(HiddenIndices)/2; i++ {
		t2.Add(
			// \cdot h_{2 \cdot i}^{r_{attrs,i}
			EcpFromProto(ipk.HAttrs[HiddenIndices[2*i]]).Mul2(
				rAttrs[2*i],
				EcpFromProto(ipk.HAttrs[HiddenIndices[2*i+1]]),
				rAttrs[2*i+1],
			),
		)
	}
	if len(HiddenIndices)%2 != 0 {
		t2.Add(FP256BN.G1mul(EcpFromProto(ipk.HAttrs[HiddenIndices[len(HiddenIndices)-1]]), rAttrs[len(HiddenIndices)-1]))
	}

	// t3 is related to the knowledge of the secrets behind the pseudonym, which is also signed in (A,B,S,E)
	t3 := HSk.Mul2(rSk, HRand, rRNym) // h_{sk}^{r_{sk}} \cdot h_r^{r_{rnym}}

	// Step 2: Compute the Fiat-Shamir hash, forming the challenge of the ZKP.

	// Compute the Fiat-Shamir hash, forming the challenge of the ZKP.
	// proofData is the data being hashed, it consists of:
	// the signature label
	// 7 elements of G1 each taking 2*FieldBytes+1 bytes
	// one bigint (hash of the issuer public key) of length FieldBytes
	// disclosed attributes
	// message being signed
	// the amount of bytes needed for the nonrevocation proof
	proofData := make([]byte, len([]byte(signLabel))+7*(2*FieldBytes+1)+FieldBytes+len(Disclosure)+len(msg)+ProofBytes[RevocationAlgorithm(cri.RevocationAlg)])
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG1(proofData, index, t3)
	index = appendBytesG1(proofData, index, APrime)
	index = appendBytesG1(proofData, index, ABar)
	index = appendBytesG1(proofData, index, BPrime)
	index = appendBytesG1(proofData, index, Nym)
	index = appendBytes(proofData, index, nonRevokedProofHashData)
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

	// Step 3: reply to the challenge message (s-values)
	ProofSSk := Modadd(rSk, FP256BN.Modmul(ProofC, sk, GroupOrder), GroupOrder)             // s_sk = rSK + C \cdot sk
	ProofSE := Modsub(re, FP256BN.Modmul(ProofC, E, GroupOrder), GroupOrder)                // s_e = re + C \cdot E
	ProofSR2 := Modadd(rR2, FP256BN.Modmul(ProofC, r2, GroupOrder), GroupOrder)             // s_r2 = rR2 + C \cdot r2
	ProofSR3 := Modsub(rR3, FP256BN.Modmul(ProofC, r3, GroupOrder), GroupOrder)             // s_r3 = rR3 + C \cdot r3
	ProofSSPrime := Modadd(rSPrime, FP256BN.Modmul(ProofC, sPrime, GroupOrder), GroupOrder) // s_S' = rSPrime + C \cdot sPrime
	ProofSRNym := Modadd(rRNym, FP256BN.Modmul(ProofC, RNym, GroupOrder), GroupOrder)       // s_RNym = rRNym + C \cdot RNym
	ProofSAttrs := make([][]byte, len(HiddenIndices))
	for i, j := range HiddenIndices {
		ProofSAttrs[i] = BigToBytes(
			// s_attrsi = rAttrsi + C \cdot cred.Attrs[j]
			Modadd(rAttrs[i], FP256BN.Modmul(ProofC, FP256BN.FromBytes(cred.Attrs[j]), GroupOrder), GroupOrder),
		)
	}

	// Compute the revocation part
	nonRevokedProof, err := prover.getNonRevokedProof(ProofC)
	if err != nil {
		return nil, err
	}

	// We are done. Return signature
	return &Signature{
			APrime:             EcpToProto(APrime),
			ABar:               EcpToProto(ABar),
			BPrime:             EcpToProto(BPrime),
			ProofC:             BigToBytes(ProofC),
			ProofSSk:           BigToBytes(ProofSSk),
			ProofSE:            BigToBytes(ProofSE),
			ProofSR2:           BigToBytes(ProofSR2),
			ProofSR3:           BigToBytes(ProofSR3),
			ProofSSPrime:       BigToBytes(ProofSSPrime),
			ProofSAttrs:        ProofSAttrs,
			Nonce:              BigToBytes(Nonce),
			Nym:                EcpToProto(Nym),
			ProofSRNym:         BigToBytes(ProofSRNym),
			RevocationEpochPk:  cri.EpochPk,
			RevocationPkSig:    cri.EpochPkSig,
			Epoch:              cri.Epoch,
			NonRevocationProof: nonRevokedProof},
		nil
}

// Ver verifies an idemix signature
// Disclosure steers which attributes it expects to be disclosed
// attributeValues contains the desired attribute values.
// This function will check that if attribute i is disclosed, the i-th attribute equals attributeValues[i].
func (sig *Signature) Ver(Disclosure []byte, ipk *IssuerPublicKey, msg []byte, attributeValues []*FP256BN.BIG, rhIndex int, revPk *sm2.PublicKey, epoch int) error {
	// Validate inputs
	if ipk == nil || revPk == nil {
		return errors.Errorf("cannot verify idemix signature: received nil input")
	}

	if rhIndex < 0 || rhIndex >= len(ipk.AttributeNames) || len(Disclosure) != len(ipk.AttributeNames) {
		return errors.Errorf("cannot verify idemix signature: received invalid input")
	}

	if sig.NonRevocationProof.RevocationAlg != int32(ALG_NO_REVOCATION) && Disclosure[rhIndex] == 1 {
		return errors.Errorf("Attribute %d is disclosed but is also used as revocation handle, which should remain hidden.", rhIndex)
	}

	HiddenIndices := hiddenIndices(Disclosure)

	// Parse signature
	APrime := EcpFromProto(sig.GetAPrime())
	ABar := EcpFromProto(sig.GetABar())
	BPrime := EcpFromProto(sig.GetBPrime())
	Nym := EcpFromProto(sig.GetNym())
	ProofC := FP256BN.FromBytes(sig.GetProofC())
	ProofSSk := FP256BN.FromBytes(sig.GetProofSSk())
	ProofSE := FP256BN.FromBytes(sig.GetProofSE())
	ProofSR2 := FP256BN.FromBytes(sig.GetProofSR2())
	ProofSR3 := FP256BN.FromBytes(sig.GetProofSR3())
	ProofSSPrime := FP256BN.FromBytes(sig.GetProofSSPrime())
	ProofSRNym := FP256BN.FromBytes(sig.GetProofSRNym())
	ProofSAttrs := make([]*FP256BN.BIG, len(sig.GetProofSAttrs()))

	if len(sig.ProofSAttrs) != len(HiddenIndices) {
		return errors.Errorf("signature invalid: incorrect amount of s-values for AttributeProofSpec")
	}
	for i, b := range sig.ProofSAttrs {
		ProofSAttrs[i] = FP256BN.FromBytes(b)
	}
	Nonce := FP256BN.FromBytes(sig.GetNonce())

	// Parse issuer public key
	W := Ecp2FromProto(ipk.W)
	HRand := EcpFromProto(ipk.HRand)
	HSk := EcpFromProto(ipk.HSk)

	// Verify signature
	if APrime.Is_infinity() {
		return errors.Errorf("signature invalid: APrime = 1")
	}
	temp1 := FP256BN.Ate(W, APrime)
	temp2 := FP256BN.Ate(GenG2, ABar)
	temp2.Inverse()
	temp1.Mul(temp2)
	if !FP256BN.Fexp(temp1).Isunity() {
		return errors.Errorf("signature invalid: APrime and ABar don't have the expected structure")
	}

	// Verify ZK proof

	// Recover t-values

	// Recompute t1
	t1 := APrime.Mul2(ProofSE, HRand, ProofSR2)
	temp := FP256BN.NewECP()
	temp.Copy(ABar)
	temp.Sub(BPrime)
	t1.Sub(FP256BN.G1mul(temp, ProofC))

	// Recompute t2
	t2 := FP256BN.G1mul(HRand, ProofSSPrime)
	t2.Add(BPrime.Mul2(ProofSR3, HSk, ProofSSk))
	for i := 0; i < len(HiddenIndices)/2; i++ {
		t2.Add(EcpFromProto(ipk.HAttrs[HiddenIndices[2*i]]).Mul2(ProofSAttrs[2*i], EcpFromProto(ipk.HAttrs[HiddenIndices[2*i+1]]), ProofSAttrs[2*i+1]))
	}
	if len(HiddenIndices)%2 != 0 {
		t2.Add(FP256BN.G1mul(EcpFromProto(ipk.HAttrs[HiddenIndices[len(HiddenIndices)-1]]), ProofSAttrs[len(HiddenIndices)-1]))
	}
	temp = FP256BN.NewECP()
	temp.Copy(GenG1)
	for index, disclose := range Disclosure {
		if disclose != 0 {
			temp.Add(FP256BN.G1mul(EcpFromProto(ipk.HAttrs[index]), attributeValues[index]))
		}
	}
	t2.Add(FP256BN.G1mul(temp, ProofC))

	// Recompute t3
	t3 := HSk.Mul2(ProofSSk, HRand, ProofSRNym)
	t3.Sub(Nym.Mul(ProofC))

	// add contribution from the non-revocation proof
	nonRevokedVer, err := getNonRevocationVerifier(RevocationAlgorithm(sig.NonRevocationProof.RevocationAlg))
	if err != nil {
		return err
	}

	i := sort.SearchInts(HiddenIndices, rhIndex)
	proofSRh := ProofSAttrs[i]
	nonRevokedProofBytes, err := nonRevokedVer.recomputeFSContribution(sig.NonRevocationProof, ProofC, Ecp2FromProto(sig.RevocationEpochPk), proofSRh)
	if err != nil {
		return err
	}

	// Recompute challenge
	// proofData is the data being hashed, it consists of:
	// the signature label
	// 7 elements of G1 each taking 2*FieldBytes+1 bytes
	// one bigint (hash of the issuer public key) of length FieldBytes
	// disclosed attributes
	// message that was signed
	proofData := make([]byte, len([]byte(signLabel))+7*(2*FieldBytes+1)+FieldBytes+len(Disclosure)+len(msg)+ProofBytes[RevocationAlgorithm(sig.NonRevocationProof.RevocationAlg)])
	index := 0
	index = appendBytesString(proofData, index, signLabel)
	index = appendBytesG1(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG1(proofData, index, t3)
	index = appendBytesG1(proofData, index, APrime)
	index = appendBytesG1(proofData, index, ABar)
	index = appendBytesG1(proofData, index, BPrime)
	index = appendBytesG1(proofData, index, Nym)
	index = appendBytes(proofData, index, nonRevokedProofBytes)
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

	if *ProofC != *HashModOrder(proofData) {
		// This debug line helps identify where the mismatch happened
		idemixLogger.Debugf("Signature Verification : \n"+
			"	[t1:%v]\n,"+
			"	[t2:%v]\n,"+
			"	[t3:%v]\n,"+
			"	[APrime:%v]\n,"+
			"	[ABar:%v]\n,"+
			"	[BPrime:%v]\n,"+
			"	[Nym:%v]\n,"+
			"	[nonRevokedProofBytes:%v]\n,"+
			"	[ipk.Hash:%v]\n,"+
			"	[Disclosure:%v]\n,"+
			"	[msg:%v]\n,",
			EcpToBytes(t1),
			EcpToBytes(t2),
			EcpToBytes(t3),
			EcpToBytes(APrime),
			EcpToBytes(ABar),
			EcpToBytes(BPrime),
			EcpToBytes(Nym),
			nonRevokedProofBytes,
			ipk.Hash,
			Disclosure,
			msg)
		return errors.Errorf("signature invalid: zero-knowledge proof is invalid")
	}

	// Signature is valid
	return nil
}
