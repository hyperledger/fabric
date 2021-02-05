/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"sort"

	opts "github.com/IBM/idemix/bccsp/schemes"
	math "github.com/IBM/mathlib"
	"github.com/pkg/errors"
)

// signLabel is the label used in zero-knowledge proof (ZKP) to identify that this ZKP is a signature of knowledge
const signLabel = "sign"
const signWithEidNymLabel = "signWithEidNym"

// A signature that is produced using an Identity Mixer credential is a so-called signature of knowledge
// (for details see C.P.Schnorr "Efficient Identification and Signatures for Smart Cards")
// An Identity Mixer signature is a signature of knowledge that signs a message and proves (in zero-knowledge)
// the knowledge of the user secret (and possibly attributes) signed inside a credential
// that was issued by a certain issuer (referred to with the issuer public key)
// The signature is verified using the message being signed and the public key of the issuer
// Some of the attributes from the credential can be selectively disclosed or different statements can be proven about
// credential attributes without disclosing them in the clear
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
func (i *Idemix) NewSignature(
	cred *Credential,
	sk *math.Zr,
	Nym *math.G1,
	RNym *math.Zr,
	ipk *IssuerPublicKey,
	Disclosure []byte,
	msg []byte,
	rhIndex, eidIndex int,
	cri *CredentialRevocationInformation,
	rng io.Reader,
	tr Translator,
	sigType opts.SignatureType,
) (*Signature, *opts.IdemixSignerMetadata, error) {
	switch sigType {
	case opts.Standard:
		return newSignature(cred, sk, Nym, RNym, ipk, Disclosure, msg, rhIndex, cri, rng, i.Curve, tr)
	case opts.EidNym:
		return newSignatureWithEIDNym(cred, sk, Nym, RNym, ipk, Disclosure, msg, rhIndex, eidIndex, cri, rng, i.Curve, tr)
	}

	panic(fmt.Sprintf("programming error, requested signature type %d", sigType))
}

func newSignatureWithEIDNym(
	cred *Credential,
	sk *math.Zr,
	Nym *math.G1,
	RNym *math.Zr,
	ipk *IssuerPublicKey,
	Disclosure []byte,
	msg []byte,
	rhIndex, eidIndex int,
	cri *CredentialRevocationInformation,
	rng io.Reader,
	curve *math.Curve,
	tr Translator,
) (*Signature, *opts.IdemixSignerMetadata, error) {
	if Disclosure[eidIndex] != 0 {
		return nil, nil, errors.Errorf("cannot create idemix signature: disclosure of enrollment ID requested for NewSignatureWithEIDNym")
	}

	t1, t2, t3, APrime, ABar, BPrime, nonRevokedProofHashData, E, Nonce, rSk, rSPrime, rR2, rR3, r2, r3, re, sPrime, rRNym, rAttrs, prover, HiddenIndices, err := prepare(cred, sk, Nym, RNym, ipk, Disclosure, msg, rhIndex, cri, rng, curve, tr)
	if err != nil {
		return nil, nil, err
	}

	return finalise(
		cred,
		sk,
		Nym,
		RNym,
		ipk,
		Disclosure,
		msg,
		rhIndex, eidIndex,
		cri,
		rng,
		curve,
		tr,
		t1, t2, t3,
		APrime, ABar, BPrime,
		nonRevokedProofHashData,
		E,
		Nonce,
		rSk, rSPrime, rR2, rR3, r2, r3, re, sPrime, rRNym,
		rAttrs,
		prover,
		HiddenIndices,
		opts.EidNym,
	)
}

func newSignature(
	cred *Credential,
	sk *math.Zr,
	Nym *math.G1,
	RNym *math.Zr,
	ipk *IssuerPublicKey,
	Disclosure []byte,
	msg []byte,
	rhIndex int,
	cri *CredentialRevocationInformation,
	rng io.Reader,
	curve *math.Curve,
	tr Translator,
) (*Signature, *opts.IdemixSignerMetadata, error) {
	t1, t2, t3, APrime, ABar, BPrime, nonRevokedProofHashData, E, Nonce, rSk, rSPrime, rR2, rR3, r2, r3, re, sPrime, rRNym, rAttrs, prover, HiddenIndices, err := prepare(cred, sk, Nym, RNym, ipk, Disclosure, msg, rhIndex, cri, rng, curve, tr)
	if err != nil {
		return nil, nil, err
	}

	return finalise(
		cred,
		sk,
		Nym,
		RNym,
		ipk,
		Disclosure,
		msg,
		rhIndex, -1,
		cri,
		rng,
		curve,
		tr,
		t1, t2, t3,
		APrime, ABar, BPrime,
		nonRevokedProofHashData,
		E,
		Nonce,
		rSk, rSPrime, rR2, rR3, r2, r3, re, sPrime, rRNym,
		rAttrs,
		prover,
		HiddenIndices,
		opts.Standard,
	)
}

func prepare(
	cred *Credential,
	sk *math.Zr,
	Nym *math.G1,
	RNym *math.Zr,
	ipk *IssuerPublicKey,
	Disclosure []byte,
	msg []byte,
	rhIndex int,
	cri *CredentialRevocationInformation,
	rng io.Reader,
	curve *math.Curve,
	tr Translator,
) (*math.G1, *math.G1, *math.G1, *math.G1, *math.G1, *math.G1,
	[]byte,
	*math.Zr,
	*math.Zr,
	*math.Zr, *math.Zr, *math.Zr, *math.Zr, *math.Zr, *math.Zr, *math.Zr, *math.Zr, *math.Zr,
	[]*math.Zr,
	nonRevokedProver,
	[]int, error,
) {
	// Validate inputs
	if cred == nil || sk == nil || Nym == nil || RNym == nil || ipk == nil || rng == nil || cri == nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, errors.Errorf("cannot create idemix signature: received nil input")
	}

	if rhIndex < 0 || rhIndex >= len(ipk.AttributeNames) || len(Disclosure) != len(ipk.AttributeNames) {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, errors.Errorf("cannot create idemix signature: received invalid input")
	}

	if cri.RevocationAlg != int32(ALG_NO_REVOCATION) && Disclosure[rhIndex] == 1 {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, errors.Errorf("Attribute %d is disclosed but also used as revocation handle attribute, which should remain hidden.", rhIndex)
	}

	// locate the indices of the attributes to hide and sample randomness for them
	HiddenIndices := hiddenIndices(Disclosure)

	// Generate required randomness r_1, r_2
	r1 := curve.NewRandomZr(rng)
	r2 := curve.NewRandomZr(rng)
	// Set r_3 as \frac{1}{r_1}
	r3 := r1.Copy()
	r3.InvModP(curve.GroupOrder)

	// Sample a nonce
	Nonce := curve.NewRandomZr(rng)

	// Parse credential
	A, err := tr.G1FromProto(cred.A)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	B, err := tr.G1FromProto(cred.B)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	// Randomize credential

	// Compute A' as A^{r_1}
	APrime := A.Mul(r1)
	// logger.Printf("Signature Generation : \n"+
	// 	"	[APrime:%v]\n",
	// 	APrime.Bytes(),
	// )

	// Compute ABar as A'^{-e} b^{r1}
	ABar := B.Mul(r1)
	ABar.Sub(APrime.Mul(curve.NewZrFromBytes(cred.E)))
	// logger.Printf("Signature Generation : \n"+
	// 	"	[ABar:%v]\n",
	// 	ABar.Bytes(),
	// )

	// Compute B' as b^{r1} / h_r^{r2}, where h_r is h_r
	BPrime := B.Mul(r1)
	HRand, err := tr.G1FromProto(ipk.HRand)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	// Parse h_{sk} from ipk
	HSk, err := tr.G1FromProto(ipk.HSk)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	BPrime.Sub(HRand.Mul(r2))

	S := curve.NewZrFromBytes(cred.S)
	E := curve.NewZrFromBytes(cred.E)

	// Compute s' as s - r_2 \cdot r_3
	sPrime := curve.ModSub(S, curve.ModMul(r2, r3, curve.GroupOrder), curve.GroupOrder)

	// The rest of this function constructs the non-interactive zero knowledge proof
	// that links the signature, the non-disclosed attributes and the nym.

	// Sample the randomness used to compute the commitment values (aka t-values) for the ZKP
	rSk := curve.NewRandomZr(rng)
	re := curve.NewRandomZr(rng)
	rR2 := curve.NewRandomZr(rng)
	rR3 := curve.NewRandomZr(rng)
	rSPrime := curve.NewRandomZr(rng)
	rRNym := curve.NewRandomZr(rng)

	rAttrs := make([]*math.Zr, len(HiddenIndices))
	for i := range HiddenIndices {
		rAttrs[i] = curve.NewRandomZr(rng)
	}

	// First compute the non-revocation proof.
	// The challenge of the ZKP needs to depend on it, as well.
	prover, err := getNonRevocationProver(RevocationAlgorithm(cri.RevocationAlg))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
	}
	nonRevokedProofHashData, err := prover.getFSContribution(
		curve.NewZrFromBytes(cred.Attrs[rhIndex]),
		rAttrs[sort.SearchInts(HiddenIndices, rhIndex)],
		cri,
		rng,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, errors.Wrap(err, "failed to compute non-revoked proof")
	}

	// Step 1: First message (t-values)

	HAttrs := make([]*math.G1, len(ipk.HAttrs))
	for i := range ipk.HAttrs {
		var err error
		HAttrs[i], err = tr.G1FromProto(ipk.HAttrs[i])
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
		}
	}

	// t1 is related to knowledge of the credential (recall, it is a BBS+ signature)
	t1 := APrime.Mul2(re, HRand, rR2) // A'^{r_E} \cdot h_r^{r_{r2}}

	// t2: is related to knowledge of the non-disclosed attributes that signed  in (A,B,S,E)
	t2 := HRand.Mul(rSPrime)           // h_r^{r_{s'}}
	t2.Add(BPrime.Mul2(rR3, HSk, rSk)) // B'^{r_{r3}} \cdot h_{sk}^{r_{sk}}
	for i := 0; i < len(HiddenIndices)/2; i++ {
		t2.Add(
			// \cdot h_{2 \cdot i}^{r_{attrs,i}
			HAttrs[HiddenIndices[2*i]].Mul2(
				rAttrs[2*i],
				HAttrs[HiddenIndices[2*i+1]],
				rAttrs[2*i+1],
			),
		)
	}
	if len(HiddenIndices)%2 != 0 {
		t2.Add(HAttrs[HiddenIndices[len(HiddenIndices)-1]].Mul(rAttrs[len(HiddenIndices)-1]))
	}

	// t3 is related to the knowledge of the secrets behind the pseudonym, which is also signed in (A,B,S,E)
	t3 := HSk.Mul2(rSk, HRand, rRNym) // h_{sk}^{r_{sk}} \cdot h_r^{r_{rnym}}

	return t1, t2, t3,
		APrime, ABar, BPrime,
		nonRevokedProofHashData,
		E,
		Nonce,
		rSk, rSPrime, rR2, rR3, r2, r3, re, sPrime, rRNym,
		rAttrs,
		prover,
		HiddenIndices, nil
}

func finalise(
	cred *Credential,
	sk *math.Zr,
	Nym *math.G1,
	RNym *math.Zr,
	ipk *IssuerPublicKey,
	Disclosure []byte,
	msg []byte,
	rhIndex, eidIndex int,
	cri *CredentialRevocationInformation,
	rng io.Reader,
	curve *math.Curve,
	tr Translator,
	t1, t2, t3 *math.G1,
	APrime, ABar, BPrime *math.G1,
	nonRevokedProofHashData []byte,
	E *math.Zr,
	Nonce *math.Zr,
	rSk, rSPrime, rR2, rR3, r2, r3, re, sPrime, rRNym *math.Zr,
	rAttrs []*math.Zr,
	prover nonRevokedProver,
	HiddenIndices []int,
	sigType opts.SignatureType,
) (*Signature, *opts.IdemixSignerMetadata, error) {

	var Nym_eid *math.G1
	var t4 *math.G1
	var r_r_eid, r_eid *math.Zr
	if sigType == opts.EidNym {
		r_a_eid := rAttrs[sort.SearchInts(HiddenIndices, eidIndex)]
		H_a_eid, err := tr.G1FromProto(ipk.HAttrs[eidIndex])
		if err != nil {
			return nil, nil, err
		}

		a_eid := curve.NewZrFromBytes(cred.Attrs[eidIndex])
		HRand, err := tr.G1FromProto(ipk.HRand)
		if err != nil {
			return nil, nil, err
		}

		// Generate new required randomness r_eid and r_r_eid
		r_eid = curve.NewRandomZr(rng)
		r_r_eid = curve.NewRandomZr(rng)

		// Nym_eid is a hiding and binding commitment to the enrollment id
		Nym_eid = H_a_eid.Mul2(a_eid, HRand, r_eid) // H_{a_{eid}}^{a_{eid}} \cdot H_{r}^{r_{eid}}

		// t4 is the new t-value for the eid nym computed above
		t4 = H_a_eid.Mul2(r_a_eid, HRand, r_r_eid) // H_{a_{eid}}^{r_{a_{2}}} \cdot H_{r}^{r_{r_{eid}}}
	}

	// Step 2: Compute the Fiat-Shamir hash, forming the challenge of the ZKP.

	// Compute the Fiat-Shamir hash, forming the challenge of the ZKP.
	// proofData is the data being hashed, it consists of:
	// the signature label
	// 7 elements of G1 each taking 2*math.FieldBytes+1 bytes
	// one bigint (hash of the issuer public key) of length math.FieldBytes
	// disclosed attributes
	// message being signed
	// the amount of bytes needed for the nonrevocation proof
	pdl := len([]byte(signLabel)) + 7*(2*curve.FieldBytes+1) + curve.FieldBytes + len(Disclosure) + len(msg) + ProofBytes[RevocationAlgorithm(cri.RevocationAlg)]
	if sigType == opts.EidNym {
		pdl = len([]byte(signWithEidNymLabel)) + 9*(2*curve.FieldBytes+1) + curve.FieldBytes + len(Disclosure) + len(msg) + ProofBytes[RevocationAlgorithm(cri.RevocationAlg)]
	}
	proofData := make([]byte, pdl)
	index := 0
	switch sigType {
	case opts.Standard:
		index = appendBytesString(proofData, index, signLabel)
	case opts.EidNym:
		index = appendBytesString(proofData, index, signWithEidNymLabel)
	default:
		panic("programming error")
	}
	index = appendBytesG1(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG1(proofData, index, t3)
	index = appendBytesG1(proofData, index, APrime)
	index = appendBytesG1(proofData, index, ABar)
	index = appendBytesG1(proofData, index, BPrime)
	index = appendBytesG1(proofData, index, Nym)
	if sigType == opts.EidNym {
		index = appendBytesG1(proofData, index, Nym_eid)
		index = appendBytesG1(proofData, index, t4)
	}
	index = appendBytes(proofData, index, nonRevokedProofHashData)
	copy(proofData[index:], ipk.Hash)
	index = index + curve.FieldBytes
	copy(proofData[index:], Disclosure)
	index = index + len(Disclosure)
	copy(proofData[index:], msg)
	c := curve.HashToZr(proofData)

	// add the previous hash and the nonce and hash again to compute a second hash (C value)
	index = 0
	proofData = proofData[:2*curve.FieldBytes]
	index = appendBytesBig(proofData, index, c)
	appendBytesBig(proofData, index, Nonce)
	ProofC := curve.HashToZr(proofData)

	// Step 3: reply to the challenge message (s-values)
	ProofSSk := curve.ModAdd(rSk, curve.ModMul(ProofC, sk, curve.GroupOrder), curve.GroupOrder)             // s_sk = rSK + C \cdot sk
	ProofSE := curve.ModSub(re, curve.ModMul(ProofC, E, curve.GroupOrder), curve.GroupOrder)                // s_e = re + C \cdot E
	ProofSR2 := curve.ModAdd(rR2, curve.ModMul(ProofC, r2, curve.GroupOrder), curve.GroupOrder)             // s_r2 = rR2 + C \cdot r2
	ProofSR3 := curve.ModSub(rR3, curve.ModMul(ProofC, r3, curve.GroupOrder), curve.GroupOrder)             // s_r3 = rR3 + C \cdot r3
	ProofSSPrime := curve.ModAdd(rSPrime, curve.ModMul(ProofC, sPrime, curve.GroupOrder), curve.GroupOrder) // s_S' = rSPrime + C \cdot sPrime
	ProofSRNym := curve.ModAdd(rRNym, curve.ModMul(ProofC, RNym, curve.GroupOrder), curve.GroupOrder)       // s_RNym = rRNym + C \cdot RNym
	ProofSAttrs := make([][]byte, len(HiddenIndices))
	for i, j := range HiddenIndices {
		ProofSAttrs[i] =
			// s_attrsi = rAttrsi + C \cdot cred.Attrs[j]
			curve.ModAdd(rAttrs[i], curve.ModMul(ProofC, curve.NewZrFromBytes(cred.Attrs[j]), curve.GroupOrder), curve.GroupOrder).Bytes()
	}

	// Compute the revocation part
	nonRevokedProof, err := prover.getNonRevokedProof(ProofC)
	if err != nil {
		return nil, nil, err
	}

	// logger.Printf("Signature Generation : \n"+
	// 	"	[t1:%v]\n,"+
	// 	"	[t2:%v]\n,"+
	// 	"	[t3:%v]\n,"+
	// 	"	[APrime:%v]\n,"+
	// 	"	[ABar:%v]\n,"+
	// 	"	[BPrime:%v]\n,"+
	// 	"	[Nym:%v]\n,"+
	// 	"	[nonRevokedProofBytes:%v]\n,"+
	// 	"	[ipk.Hash:%v]\n,"+
	// 	"	[Disclosure:%v]\n,"+
	// 	"	[msg:%v]\n,"+
	// 	"	[ProofData:%v]\n,"+
	// 	"	[ProofC:%v]\n"+
	// 	"	[HSk:%v]\n,"+
	// 	"	[ProofSSK:%v]\n,"+
	// 	"	[HRand:%v]\n,"+
	// 	"	[ProofSRNym:%v]\n",
	// 	t1.Bytes(),
	// 	t2.Bytes(),
	// 	t3.Bytes(),
	// 	APrime.Bytes(),
	// 	ABar.Bytes(),
	// 	BPrime.Bytes(),
	// 	Nym.Bytes(),
	// 	nil,
	// 	ipk.Hash,
	// 	Disclosure,
	// 	msg,
	// 	proofData,
	// 	ProofC.Bytes(),
	// 	HSk.Bytes(),
	// 	ProofSSk.Bytes(),
	// 	HRand.Bytes(),
	// 	ProofSRNym.Bytes(),
	// )

	// We are done. Return signature
	sig := &Signature{
		APrime:             tr.G1ToProto(APrime),
		ABar:               tr.G1ToProto(ABar),
		BPrime:             tr.G1ToProto(BPrime),
		ProofC:             ProofC.Bytes(),
		ProofSSk:           ProofSSk.Bytes(),
		ProofSE:            ProofSE.Bytes(),
		ProofSR2:           ProofSR2.Bytes(),
		ProofSR3:           ProofSR3.Bytes(),
		ProofSSPrime:       ProofSSPrime.Bytes(),
		ProofSAttrs:        ProofSAttrs,
		Nonce:              Nonce.Bytes(),
		Nym:                tr.G1ToProto(Nym),
		ProofSRNym:         ProofSRNym.Bytes(),
		RevocationEpochPk:  cri.EpochPk,
		RevocationPkSig:    cri.EpochPkSig,
		Epoch:              cri.Epoch,
		NonRevocationProof: nonRevokedProof,
	}

	if sigType == opts.EidNym {
		ProofSEid := curve.ModAdd(r_r_eid, curve.ModMul(ProofC, r_eid, curve.GroupOrder), curve.GroupOrder) // s_{r{eid}} = r_r_eid + C \cdot r_eid
		sig.EidNym = &EIDNym{
			Nym:       tr.G1ToProto(Nym_eid),
			ProofSEid: ProofSEid.Bytes(),
		}
	}

	var m *opts.IdemixSignerMetadata
	if sigType == opts.EidNym {
		m = &opts.IdemixSignerMetadata{
			NymEIDAuditData: &opts.NymEIDAuditData{
				RNymEid: r_eid,
				EID:     curve.NewZrFromBytes(cred.Attrs[eidIndex]),
			},
		}
	}

	return sig, m, nil
}

func (sig *Signature) AuditNymEid(
	ipk *IssuerPublicKey,
	eidAttr *math.Zr,
	eidIndex int,
	RNymEid *math.Zr,
	curve *math.Curve,
	t Translator,
) error {
	// Validate inputs
	if ipk == nil {
		return errors.Errorf("cannot verify idemix signature: received nil input")
	}

	if sig.EidNym == nil || sig.EidNym.Nym == nil {
		return errors.Errorf("no EidNym provided")
	}

	if len(ipk.HAttrs) <= eidIndex {
		return errors.Errorf("could not access H_a_eid in array")
	}

	H_a_eid, err := t.G1FromProto(ipk.HAttrs[eidIndex])
	if err != nil {
		return errors.Wrap(err, "could not deserialize H_a_eid")
	}

	HRand, err := t.G1FromProto(ipk.HRand)
	if err != nil {
		return errors.Wrap(err, "could not deserialize HRand")
	}

	EidNym, err := t.G1FromProto(sig.EidNym.Nym)
	if err != nil {
		return errors.Wrap(err, "could not deserialize EidNym")
	}

	Nym_eid := H_a_eid.Mul2(eidAttr, HRand, RNymEid)

	if !Nym_eid.Equals(EidNym) {
		return errors.New("eid nym does not match")
	}

	return nil
}

// Ver verifies an idemix signature
// Disclosure steers which attributes it expects to be disclosed
// attributeValues contains the desired attribute values.
// This function will check that if attribute i is disclosed, the i-th attribute equals attributeValues[i].
func (sig *Signature) Ver(
	Disclosure []byte,
	ipk *IssuerPublicKey,
	msg []byte,
	attributeValues []*math.Zr,
	rhIndex, eidIndex int,
	revPk *ecdsa.PublicKey,
	epoch int,
	curve *math.Curve,
	t Translator,
	verType opts.VerificationType,
	meta *opts.IdemixSignerMetadata,
) error {
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

	if verType == opts.ExpectEidNym &&
		(sig.EidNym == nil || sig.EidNym.Nym == nil || sig.EidNym.ProofSEid == nil) {
		return errors.Errorf("no EidNym provided but ExpectEidNym required")
	}

	if verType == opts.ExpectStandard && sig.EidNym != nil {
		return errors.Errorf("EidNym available but ExpectStandard required")
	}

	verifyEIDNym := (verType == opts.BestEffort && sig.EidNym != nil) || verType == opts.ExpectEidNym

	HiddenIndices := hiddenIndices(Disclosure)

	// Parse signature
	APrime, err := t.G1FromProto(sig.GetAPrime())
	if err != nil {
		return err
	}
	//logger.Printf("Signature Verification : \n"+
	//	"	[APrime:%v]\n",
	//	APrime.Bytes(),
	//)
	ABar, err := t.G1FromProto(sig.GetABar())
	if err != nil {
		return err
	}
	//logger.Printf("Signature Verification : \n"+
	//	"	[ABar:%v]\n",
	//	ABar.Bytes(),
	//)
	BPrime, err := t.G1FromProto(sig.GetBPrime())
	if err != nil {
		return err
	}
	Nym, err := t.G1FromProto(sig.GetNym())
	if err != nil {
		return err
	}
	ProofC := curve.NewZrFromBytes(sig.GetProofC())
	ProofSSk := curve.NewZrFromBytes(sig.GetProofSSk())
	ProofSE := curve.NewZrFromBytes(sig.GetProofSE())
	ProofSR2 := curve.NewZrFromBytes(sig.GetProofSR2())
	ProofSR3 := curve.NewZrFromBytes(sig.GetProofSR3())
	ProofSSPrime := curve.NewZrFromBytes(sig.GetProofSSPrime())
	ProofSRNym := curve.NewZrFromBytes(sig.GetProofSRNym())
	ProofSAttrs := make([]*math.Zr, len(sig.GetProofSAttrs()))

	if len(sig.ProofSAttrs) != len(HiddenIndices) {
		return errors.Errorf("signature invalid: incorrect amount of s-values for AttributeProofSpec")
	}
	for i, b := range sig.ProofSAttrs {
		ProofSAttrs[i] = curve.NewZrFromBytes(b)
	}
	Nonce := curve.NewZrFromBytes(sig.GetNonce())

	// Parse issuer public key
	W, err := t.G2FromProto(ipk.W)
	if err != nil {
		return err
	}
	HRand, err := t.G1FromProto(ipk.HRand)
	if err != nil {
		return err
	}
	HSk, err := t.G1FromProto(ipk.HSk)
	if err != nil {
		return err
	}
	//logger.Printf("Signature Verification : \n"+
	//	"	[W:%v]\n",
	//	W.Bytes(),
	//)

	// Verify signature
	if APrime.IsInfinity() {
		return errors.Errorf("signature invalid: APrime = 1")
	}
	temp1 := curve.Pairing(W, APrime)
	temp2 := curve.Pairing(curve.GenG2, ABar)
	temp2.Inverse()
	temp1.Mul(temp2)
	if !curve.FExp(temp1).IsUnity() {
		return errors.Errorf("signature invalid: APrime and ABar don't have the expected structure")
	}

	// Verify ZK proof

	// Recover t-values

	HAttrs := make([]*math.G1, len(ipk.HAttrs))
	for i := range ipk.HAttrs {
		var err error
		HAttrs[i], err = t.G1FromProto(ipk.HAttrs[i])
		if err != nil {
			return err
		}
	}

	// Recompute t1
	t1 := APrime.Mul2(ProofSE, HRand, ProofSR2)
	temp := curve.NewG1()
	temp.Clone(ABar)
	temp.Sub(BPrime)
	t1.Sub(temp.Mul(ProofC))

	// Recompute t2
	t2 := HRand.Mul(ProofSSPrime)
	t2.Add(BPrime.Mul2(ProofSR3, HSk, ProofSSk))
	for i := 0; i < len(HiddenIndices)/2; i++ {
		t2.Add(HAttrs[HiddenIndices[2*i]].Mul2(ProofSAttrs[2*i], HAttrs[HiddenIndices[2*i+1]], ProofSAttrs[2*i+1]))
	}
	if len(HiddenIndices)%2 != 0 {
		t2.Add(HAttrs[HiddenIndices[len(HiddenIndices)-1]].Mul(ProofSAttrs[len(HiddenIndices)-1]))
	}
	temp = curve.NewG1()
	temp.Clone(curve.GenG1)
	for index, disclose := range Disclosure {
		if disclose != 0 {
			temp.Add(HAttrs[index].Mul(attributeValues[index]))
		}
	}
	t2.Add(temp.Mul(ProofC))

	// Recompute t3
	t3 := HSk.Mul2(ProofSSk, HRand, ProofSRNym)
	t3.Sub(Nym.Mul(ProofC))

	var t4 *math.G1
	if verifyEIDNym {
		H_a_eid, err := t.G1FromProto(ipk.HAttrs[eidIndex])
		if err != nil {
			return err
		}

		t4 = H_a_eid.Mul2(ProofSAttrs[sort.SearchInts(HiddenIndices, eidIndex)], HRand, curve.NewZrFromBytes(sig.EidNym.ProofSEid))
		EidNym, err := t.G1FromProto(sig.EidNym.Nym)
		if err != nil {
			return err
		}
		t4.Sub(EidNym.Mul(ProofC))
	}

	// add contribution from the non-revocation proof
	nonRevokedVer, err := getNonRevocationVerifier(RevocationAlgorithm(sig.NonRevocationProof.RevocationAlg))
	if err != nil {
		return err
	}

	i := sort.SearchInts(HiddenIndices, rhIndex)
	proofSRh := ProofSAttrs[i]
	RevocationEpochPk, err := t.G2FromProto(sig.RevocationEpochPk)
	if err != nil {
		return err
	}

	nonRevokedProofBytes, err := nonRevokedVer.recomputeFSContribution(sig.NonRevocationProof, ProofC, RevocationEpochPk, proofSRh)
	if err != nil {
		return err
	}

	// Recompute challenge
	// proofData is the data being hashed, it consists of:
	// the signature label
	// 7 elements of G1 each taking 2*math.FieldBytes+1 bytes
	// one bigint (hash of the issuer public key) of length math.FieldBytes
	// disclosed attributes
	// message that was signed
	pdl := len([]byte(signLabel)) + 7*(2*curve.FieldBytes+1) + curve.FieldBytes + len(Disclosure) + len(msg) + ProofBytes[RevocationAlgorithm(sig.NonRevocationProof.RevocationAlg)]
	if verifyEIDNym {
		pdl = len([]byte(signWithEidNymLabel)) + 9*(2*curve.FieldBytes+1) + curve.FieldBytes + len(Disclosure) + len(msg) + ProofBytes[RevocationAlgorithm(sig.NonRevocationProof.RevocationAlg)]
	}
	proofData := make([]byte, pdl)
	index := 0
	if verifyEIDNym {
		index = appendBytesString(proofData, index, signWithEidNymLabel)
	} else {
		index = appendBytesString(proofData, index, signLabel)
	}
	index = appendBytesG1(proofData, index, t1)
	index = appendBytesG1(proofData, index, t2)
	index = appendBytesG1(proofData, index, t3)
	index = appendBytesG1(proofData, index, APrime)
	index = appendBytesG1(proofData, index, ABar)
	index = appendBytesG1(proofData, index, BPrime)
	index = appendBytesG1(proofData, index, Nym)
	if verifyEIDNym {
		Nym, err := t.G1FromProto(sig.EidNym.Nym)
		if err != nil {
			return err
		}

		index = appendBytesG1(proofData, index, Nym)
		index = appendBytesG1(proofData, index, t4)
	}
	index = appendBytes(proofData, index, nonRevokedProofBytes)
	copy(proofData[index:], ipk.Hash)
	index = index + curve.FieldBytes
	copy(proofData[index:], Disclosure)
	index = index + len(Disclosure)
	copy(proofData[index:], msg)

	c := curve.HashToZr(proofData)
	index = 0
	proofData = proofData[:2*curve.FieldBytes]
	index = appendBytesBig(proofData, index, c)
	appendBytesBig(proofData, index, Nonce)

	// audit eid nym if data provided and verification requested
	if verifyEIDNym && meta != nil && meta.NymEIDAuditData != nil {
		H_a_eid, err := t.G1FromProto(ipk.HAttrs[eidIndex])
		if err != nil {
			return err
		}

		EidNym, err := t.G1FromProto(sig.EidNym.Nym)
		if err != nil {
			return err
		}

		Nym_eid := H_a_eid.Mul2(meta.NymEIDAuditData.EID, HRand, meta.NymEIDAuditData.RNymEid)
		if !Nym_eid.Equals(EidNym) {
			return errors.Errorf("signature invalid: nym eid validation failed")
		}
	}

	recomputedProofC := curve.HashToZr(proofData)
	if !ProofC.Equals(recomputedProofC) {
		// This debug line helps identify where the mismatch happened
		logger.Printf("Signature Verification : \n"+
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
			"	[msg:%v]\n,"+
			"	[proofdata:%v]\n,"+
			"	[ProofC:%v]\n,"+
			"	[recomputedProofC:%v]\n,"+
			"	[HSk:%v]\n,"+
			"	[ProofSSK:%v]\n,"+
			"	[HRand:%v]\n,"+
			"	[ProofSRNym:%v]\n",
			t1.Bytes(),
			t2.Bytes(),
			t3.Bytes(),
			APrime.Bytes(),
			ABar.Bytes(),
			BPrime.Bytes(),
			Nym.Bytes(),
			nonRevokedProofBytes,
			ipk.Hash,
			Disclosure,
			msg,
			proofData,
			ProofC.Bytes(),
			recomputedProofC.Bytes(),
			HSk.Bytes(),
			ProofSSk.Bytes(),
			HRand.Bytes(),
			ProofSRNym.Bytes(),
		)
		return errors.Errorf("signature invalid: zero-knowledge proof is invalid")
	}

	// Signature is valid
	return nil
}
