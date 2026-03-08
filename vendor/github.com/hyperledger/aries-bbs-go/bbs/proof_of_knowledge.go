/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bbs

import (
	"fmt"

	ml "github.com/IBM/mathlib"
)

// PoKOfSignature is Proof of Knowledge of a Signature that is used by the prover to construct PoKOfSignatureProof.
type PoKOfSignature struct {
	aPrime *ml.G1
	aBar   *ml.G1
	d      *ml.G1

	pokVC1   *ProverCommittedG1
	secrets1 []*ml.Zr

	PokVC2   *ProverCommittedG1
	secrets2 []*ml.Zr

	revealedMessages map[int]*SignatureMessage

	curve *ml.Curve
}

// NewPoKOfSignature creates a new PoKOfSignature.
func (bl *BBSLib) NewPoKOfSignature(signature *Signature, messages []*SignatureMessage, revealedIndexes []int,
	pubKey *PublicKeyWithGenerators) (*PoKOfSignature, error) {

	p := &PoKOfSignatureProvider{
		VC2SignatureProvider: &defaultVC2SignatureProvider{
			bl: bl,
		},
		VerifySig: true,
		Curve:     bl.curve,
		Bl:        bl,
	}

	return p.PoKOfSignature(signature, messages, revealedIndexes, pubKey)
}

type VC2SignatureProvider interface {
	New(*ml.G1, *ml.Zr, *PublicKeyWithGenerators, *ml.Zr, []*SignatureMessage, map[int]*SignatureMessage) (*ProverCommittedG1, []*ml.Zr)
}

type PoKOfSignatureProvider struct {
	VC2SignatureProvider

	VerifySig bool

	Curve *ml.Curve
	Bl    *BBSLib
}

func (p *PoKOfSignatureProvider) PoKOfSignature(signature *Signature, messages []*SignatureMessage, revealedIndexes []int,
	pubKey *PublicKeyWithGenerators) (*PoKOfSignature, error) {
	b := ComputeB(signature.S, messages, pubKey, p.Bl.curve)

	return p.PoKOfSignatureB(signature, messages, revealedIndexes, pubKey, b)
}

func (p *PoKOfSignatureProvider) PoKOfSignatureB(signature *Signature, messages []*SignatureMessage, revealedIndexes []int,
	pubKey *PublicKeyWithGenerators, b *ml.G1) (*PoKOfSignature, error) {

	if p.VerifySig {
		err := signature.Verify(messages, pubKey)
		if err != nil {
			return nil, fmt.Errorf("verify input signature: %w", err)
		}
	}

	r1, r2 := p.Bl.createRandSignatureFr(), p.Bl.createRandSignatureFr()
	aPrime := signature.A.Mul(FrToRepr(r1))

	aBarDenom := aPrime.Mul(FrToRepr(signature.E))

	aBar := b.Mul(FrToRepr(r1))
	aBar.Sub(aBarDenom)

	r2D := r2.Copy()
	r2D.Neg()

	commitmentBasesCount := 2
	cb := NewCommitmentBuilder(commitmentBasesCount)
	cb.Add(b, r1)
	cb.Add(pubKey.H0, r2D)

	d := cb.Build()
	r3 := r1.Copy()
	r3.InvModP(p.Bl.curve.GroupOrder)

	sPrime := r2.Mul(r3)
	sPrime.Neg()
	sPrime = sPrime.Plus(signature.S)

	pokVC1, secrets1 := p.Bl.newVC1Signature(aPrime, pubKey.H0, signature.E, r2)

	revealedMessages := make(map[int]*SignatureMessage, len(revealedIndexes))

	if len(messages) < len(revealedIndexes) {
		return nil, fmt.Errorf("invalid size: %d revealed indexes is larger than %d messages", len(revealedIndexes),
			len(messages))
	}

	for _, ind := range revealedIndexes {
		revealedMessages[messages[ind].Idx] = messages[ind]
	}

	pokVC2, secrets2 := p.VC2SignatureProvider.New(d, r3, pubKey, sPrime, messages, revealedMessages)

	return &PoKOfSignature{
		aPrime:           aPrime,
		aBar:             aBar,
		d:                d,
		pokVC1:           pokVC1,
		secrets1:         secrets1,
		PokVC2:           pokVC2,
		secrets2:         secrets2,
		revealedMessages: revealedMessages,
		curve:            p.Curve,
	}, nil
}

func (b *BBSLib) newVC1Signature(aPrime *ml.G1, h0 *ml.G1,
	e, r2 *ml.Zr) (*ProverCommittedG1, []*ml.Zr) {
	committing1 := b.NewProverCommittingG1()
	secrets1 := make([]*ml.Zr, 2)

	committing1.Commit(aPrime)

	sigE := e.Copy()
	sigE.Neg()
	secrets1[0] = sigE

	committing1.Commit(h0)

	secrets1[1] = r2
	pokVC1 := committing1.Finish()

	return pokVC1, secrets1
}

type defaultVC2SignatureProvider struct {
	bl *BBSLib
}

func (p *defaultVC2SignatureProvider) New(d *ml.G1, r3 *ml.Zr, pubKey *PublicKeyWithGenerators, sPrime *ml.Zr,
	messages []*SignatureMessage, revealedMessages map[int]*SignatureMessage) (*ProverCommittedG1, []*ml.Zr) {
	messagesCount := len(messages)
	committing2 := p.bl.NewProverCommittingG1()
	baseSecretsCount := 2
	secrets2 := make([]*ml.Zr, 0, baseSecretsCount+messagesCount)

	committing2.Commit(d)

	r3D := r3.Copy()
	r3D.Neg()

	secrets2 = append(secrets2, r3D)

	committing2.Commit(pubKey.H0)

	secrets2 = append(secrets2, sPrime)

	for _, msg := range messages {
		if _, ok := revealedMessages[msg.Idx]; ok {
			continue
		}

		committing2.Commit(pubKey.H[msg.Idx])

		sourceFR := msg.FR
		hiddenFRCopy := sourceFR.Copy()

		secrets2 = append(secrets2, hiddenFRCopy)
	}

	pokVC2 := committing2.Finish()

	return pokVC2, secrets2
}

// ToBytes converts PoKOfSignature to bytes.
func (pos *PoKOfSignature) ToBytes() []byte {
	challengeBytes := pos.aBar.Bytes()
	challengeBytes = append(challengeBytes, pos.pokVC1.ToBytes()...)
	challengeBytes = append(challengeBytes, pos.PokVC2.ToBytes()...)

	return challengeBytes
}

// GenerateProof generates PoKOfSignatureProof proof from PoKOfSignature signature.
func (pos *PoKOfSignature) GenerateProof(challengeHash *ml.Zr) *PoKOfSignatureProof {
	return &PoKOfSignatureProof{
		aPrime:   pos.aPrime,
		aBar:     pos.aBar,
		d:        pos.d,
		proofVC1: pos.pokVC1.GenerateProof(challengeHash, pos.secrets1),
		ProofVC2: pos.PokVC2.GenerateProof(challengeHash, pos.secrets2),
		curve:    pos.curve,
	}
}

// ProverCommittedG1 helps to generate a ProofG1.
type ProverCommittedG1 struct {
	Bases           []*ml.G1
	BlindingFactors []*ml.Zr
	Commitment      *ml.G1
}

// ToBytes converts ProverCommittedG1 to bytes.
func (g *ProverCommittedG1) ToBytes() []byte {
	bytes := make([]byte, 0)

	for _, base := range g.Bases {
		bytes = append(bytes, base.Bytes()...)
	}

	return append(bytes, g.Commitment.Bytes()...)
}

// GenerateProof generates proof ProofG1 for all secrets.
func (g *ProverCommittedG1) GenerateProof(challenge *ml.Zr, secrets []*ml.Zr) *ProofG1 {
	responses := make([]*ml.Zr, len(g.Bases))

	for i := range g.BlindingFactors {
		c := challenge.Mul(secrets[i])

		s := g.BlindingFactors[i].Minus(c)
		responses[i] = s
	}

	return &ProofG1{
		Commitment: g.Commitment,
		Responses:  responses,
	}
}

// ProverCommittingG1 is a proof of knowledge of messages in a vector commitment.
type ProverCommittingG1 struct {
	bases           []*ml.G1
	BlindingFactors []*ml.Zr
	b               *BBSLib
}

// NewProverCommittingG1 creates a new ProverCommittingG1.
func (bl *BBSLib) NewProverCommittingG1() *ProverCommittingG1 {
	return &ProverCommittingG1{
		bases:           make([]*ml.G1, 0),
		BlindingFactors: make([]*ml.Zr, 0),
		b:               bl,
	}
}

// Commit append a base point and randomly generated blinding factor.
func (pc *ProverCommittingG1) Commit(base *ml.G1) {
	pc.bases = append(pc.bases, base)
	r := pc.b.createRandSignatureFr()
	pc.BlindingFactors = append(pc.BlindingFactors, r)
}

// Finish helps to generate ProverCommittedG1 after commitment of all base points.
func (pc *ProverCommittingG1) Finish() *ProverCommittedG1 {
	commitment := sumOfG1Products(pc.bases, pc.BlindingFactors)

	return &ProverCommittedG1{
		Bases:           pc.bases,
		BlindingFactors: pc.BlindingFactors,
		Commitment:      commitment,
	}
}
