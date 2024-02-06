/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package aries

import (
	"fmt"
	"io"

	"github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/ale-linux/aries-framework-go/component/kmscrypto/crypto/primitive/bbs12381g2pub"
	"github.com/golang/protobuf/proto"
)

const nymSigLabel = "nym-sig"

type NymSigner struct {
	Curve *math.Curve
	Rng   io.Reader
}

// Sign creates a new idemix pseudonym signature
func (s *NymSigner) Sign(
	sk *math.Zr,
	Nym *math.G1,
	RNym *math.Zr,
	key types.IssuerPublicKey,
	digest []byte,
) ([]byte, error) {
	ipk, ok := key.(*IssuerPublicKey)
	if !ok {
		return nil, fmt.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	Nonce := s.Curve.NewRandomZr(s.Rng)

	commit := bbs12381g2pub.NewProverCommittingG1()
	commit.Commit(ipk.PKwG.H0)
	commit.Commit(ipk.PKwG.H[0])
	commitNym := commit.Finish()

	challengeBytes := []byte(nymSigLabel)
	challengeBytes = append(challengeBytes, Nym.Bytes()...)
	challengeBytes = append(challengeBytes, commitNym.ToBytes()...)
	challengeBytes = append(challengeBytes, digest...)

	proofChallenge := bbs12381g2pub.FrFromOKM(challengeBytes)

	challengeBytes = proofChallenge.Bytes()
	challengeBytes = append(challengeBytes, Nonce.Bytes()...)
	proofChallenge = bbs12381g2pub.FrFromOKM(challengeBytes)

	proof := commitNym.GenerateProof(proofChallenge, []*math.Zr{RNym, sk})

	sig := &NymSignature{
		MainSignature: proof.ToBytes(),
		Nonce:         Nonce.Bytes(),
	}

	return proto.Marshal(sig)
}

// Verify verifies an idemix NymSignature
func (s *NymSigner) Verify(
	key types.IssuerPublicKey,
	Nym *math.G1,
	sigBytes, digest []byte,
) error {
	ipk, ok := key.(*IssuerPublicKey)
	if !ok {
		return fmt.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	sig := &NymSignature{}
	err := proto.Unmarshal(sigBytes, sig)
	if err != nil {
		return fmt.Errorf("error unmarshalling signature: [%w]", err)
	}

	nymProof, err := bbs12381g2pub.ParseProofG1(sig.MainSignature)
	if err != nil {
		return fmt.Errorf("parse nym proof: %w", err)
	}

	challengeBytes := []byte(nymSigLabel)
	challengeBytes = append(challengeBytes, Nym.Bytes()...)
	challengeBytes = append(challengeBytes, ipk.PKwG.H0.Bytes()...)
	challengeBytes = append(challengeBytes, ipk.PKwG.H[0].Bytes()...)
	challengeBytes = append(challengeBytes, nymProof.Commitment.Bytes()...)
	challengeBytes = append(challengeBytes, digest...)

	proofChallenge := bbs12381g2pub.FrFromOKM(challengeBytes)

	challengeBytes = proofChallenge.Bytes()
	challengeBytes = append(challengeBytes, sig.Nonce...)
	proofChallenge = bbs12381g2pub.FrFromOKM(challengeBytes)

	return nymProof.Verify([]*math.G1{ipk.PKwG.H0, ipk.PKwG.H[0]}, Nym, proofChallenge)
}
