/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package bbs contains BBS+ signing primitives and keys.
package bbs

import (
	"errors"
	"fmt"
	"sort"

	ml "github.com/IBM/mathlib"
)

type BBSLib struct {
	curve                  *ml.Curve
	bls12381SignatureLen   int
	bls12381G2PublicKeyLen int
	g1CompressedSize       int
	g1UncompressedSize     int
	g2UncompressedSize     int
	frUncompressedSize     int
}

func NewBBSLib(curve *ml.Curve) *BBSLib {
	return &BBSLib{
		curve: curve,

		// Signature length.
		bls12381SignatureLen: curve.CompressedG1ByteSize + 2*frCompressedSize,

		// Default BLS 12-381 public key length in G2 field.
		bls12381G2PublicKeyLen: curve.CompressedG2ByteSize,

		// Number of bytes in G1 X coordinate.
		g1CompressedSize: curve.CompressedG1ByteSize,

		// Number of bytes in G1 X and Y coordinates.
		g1UncompressedSize: curve.G1ByteSize,

		// Number of bytes in G2 X(a, b) and Y(a, b) coordinates.
		g2UncompressedSize: curve.G2ByteSize,

		// Number of bytes in scalar uncompressed form.
		frUncompressedSize: curve.ScalarByteSize,
	}
}

// BBSG2Pub defines BBS+ signature scheme where public key is a point in the field of G2.
// BBS+ signature scheme (as defined in https://eprint.iacr.org/2016/663.pdf, section 4.3).
type BBSG2Pub struct {
	curve *ml.Curve
	lib   *BBSLib
}

// New creates a new BBSG2Pub.
func New(curve *ml.Curve) *BBSG2Pub {
	return &BBSG2Pub{
		curve: curve,
		lib:   NewBBSLib(curve),
	}
}

// Number of bytes in scalar compressed form.
const frCompressedSize = 32

// Verify makes BLS BBS12-381 signature verification.
func (bbs *BBSG2Pub) Verify(messages [][]byte, sigBytes, pubKeyBytes []byte) error {
	signature, err := bbs.lib.ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	pubKey, err := bbs.lib.UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	messagesCount := len(messages)

	publicKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return fmt.Errorf("build generators from public key: %w", err)
	}

	messagesFr := MessagesToFr(messages, bbs.curve)

	return signature.Verify(messagesFr, publicKeyWithGenerators)
}

// Sign signs the one or more messages using private key in compressed form.
func (bbs *BBSG2Pub) Sign(messages [][]byte, privKeyBytes []byte) ([]byte, error) {
	privKey, err := bbs.lib.UnmarshalPrivateKey(privKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshal private key: %w", err)
	}

	if len(messages) == 0 {
		return nil, errors.New("messages are not defined")
	}

	return bbs.SignWithKey(messages, privKey)
}

// VerifyProof verifies BBS+ signature proof for one ore more revealed messages.
func (bbs *BBSG2Pub) VerifyProof(messagesBytes [][]byte, proof, nonce, pubKeyBytes []byte) error {

	messages := MessagesToFr(messagesBytes, bbs.curve)

	return bbs.VerifyProofFr(messages, proof, nonce, pubKeyBytes)
}

// VerifyProofFr verifies BBS+ signature proof for one ore more revealed messages.
// The messages are supplied as scalars and not bytes.
func (bbs *BBSG2Pub) VerifyProofFr(messages []*SignatureMessage, proof, nonce, pubKeyBytes []byte) error {
	payload, err := ParsePoKPayload(proof)
	if err != nil {
		return fmt.Errorf("parse signature proof: %w", err)
	}

	signatureProof, err := bbs.lib.ParseSignatureProof(proof[payload.LenInBytes():])
	if err != nil {
		return fmt.Errorf("parse signature proof: %w", err)
	}

	pubKey, err := bbs.lib.UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	publicKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(payload.MessagesCount)
	if err != nil {
		return fmt.Errorf("build generators from public key: %w", err)
	}

	if len(payload.Revealed) > len(messages) {
		return fmt.Errorf("payload revealed bigger from messages")
	}

	revealedMessages := make(map[int]*SignatureMessage)
	for i := range payload.Revealed {
		revealedMessages[payload.Revealed[i]] = messages[i]
	}

	challengeBytes := signatureProof.GetBytesForChallenge(revealedMessages, publicKeyWithGenerators)
	proofNonce := ParseProofNonce(nonce, bbs.curve)
	proofNonceBytes := proofNonce.ToBytes()
	challengeBytes = append(challengeBytes, proofNonceBytes...)
	proofChallenge := FrFromOKM(challengeBytes, bbs.curve)

	return signatureProof.Verify(proofChallenge, publicKeyWithGenerators, revealedMessages, messages)
}

// DeriveProof derives a proof of BBS+ signature with some messages disclosed.
func (bbs *BBSG2Pub) DeriveProof(messages [][]byte, sigBytes, nonce, pubKeyBytes []byte,
	revealedIndexes []int) ([]byte, error) {

	return bbs.DeriveProofZr(MessagesToFr(messages, bbs.curve), sigBytes, nonce, pubKeyBytes, revealedIndexes)
}

// DeriveProofZr derives a proof of BBS+ signature with some messages disclosed.
// The messages are supplied as scalars and not bytes.
func (bbs *BBSG2Pub) DeriveProofZr(messagesFr []*SignatureMessage, sigBytes, nonce, pubKeyBytes []byte,
	revealedIndexes []int) ([]byte, error) {

	if len(revealedIndexes) == 0 {
		return nil, errors.New("no message to reveal")
	}

	sort.Ints(revealedIndexes)

	messagesCount := len(messagesFr)

	pubKey, err := bbs.lib.UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	publicKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return nil, fmt.Errorf("build generators from public key: %w", err)
	}

	signature, err := bbs.lib.ParseSignature(sigBytes)
	if err != nil {
		return nil, fmt.Errorf("parse signature: %w", err)
	}

	pokSignature, err := bbs.lib.NewPoKOfSignature(signature, messagesFr, revealedIndexes, publicKeyWithGenerators)
	if err != nil {
		return nil, fmt.Errorf("init proof of knowledge signature: %w", err)
	}

	challengeBytes := pokSignature.ToBytes()

	proofNonce := ParseProofNonce(nonce, bbs.curve)
	proofNonceBytes := proofNonce.ToBytes()
	challengeBytes = append(challengeBytes, proofNonceBytes...)

	proofChallenge := FrFromOKM(challengeBytes, bbs.curve)

	proof := pokSignature.GenerateProof(proofChallenge)

	payload := NewPoKPayload(messagesCount, revealedIndexes)

	payloadBytes, err := payload.ToBytes()
	if err != nil {
		return nil, fmt.Errorf("derive proof: paylod to bytes: %w", err)
	}

	signatureProofBytes := append(payloadBytes, proof.ToBytes()...)

	return signatureProofBytes, nil
}

// SignWithKey signs the one or more messages using BBS+ key pair.
func (bbs *BBSG2Pub) SignWithKey(messages [][]byte, privKey *PrivateKey) ([]byte, error) {
	messagesFr := make([]*SignatureMessage, len(messages))
	for i := range messages {
		messagesFr[i] = ParseSignatureMessage(messages[i], i, bbs.curve)
	}

	return bbs.SignWithKeyFr(messagesFr, len(messages), privKey)
}

// SignWithKeyFr signs the one or more messages using BBS+ key pair.
// The messages are supplied as scalars and not bytes.
func (bbs *BBSG2Pub) SignWithKeyFr(messagesFr []*SignatureMessage, messagesCount int, privKey *PrivateKey) ([]byte, error) {
	var err error

	pubKey := privKey.PublicKey()

	pubKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return nil, fmt.Errorf("build generators from public key: %w", err)
	}

	const basesOffset = 1

	cb := NewCommitmentBuilder(len(messagesFr) + basesOffset)

	cb.Add(bbs.curve.GenG1, bbs.curve.NewZrFromInt(1))

	for i := 0; i < len(messagesFr); i++ {
		cb.Add(pubKeyWithGenerators.H[messagesFr[i].Idx], messagesFr[i].FR)
	}

	return bbs.SignWithKeyB(cb.Build(), len(messagesFr), privKey)
}

// SignWithKeyB signs the one or more messages using BBS+ key pair.
// Messages are already committed in the element `b`, which the caller
// is supposed to have constructed properly. This call enables a clean
// construction of blind signing protocols, where `b` is constructed
// jointly by requester and signer.
func (bbs *BBSG2Pub) SignWithKeyB(b *ml.G1, messagesCount int, privKey *PrivateKey) ([]byte, error) {
	var err error

	pubKey := privKey.PublicKey()

	pubKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return nil, fmt.Errorf("build generators from public key: %w", err)
	}

	e, s := bbs.lib.createRandSignatureFr(), bbs.lib.createRandSignatureFr()
	exp := privKey.FR.Copy()
	exp = exp.Plus(e)
	exp.InvModP(bbs.curve.GroupOrder)

	b = b.Copy()
	b.Add(pubKeyWithGenerators.H0.Mul(s))

	sig := b.Mul(FrToRepr(exp))

	signature := &Signature{
		A:     sig,
		E:     e,
		S:     s,
		curve: bbs.curve,
	}

	return signature.ToBytes()
}

func ComputeB(
	s *ml.Zr,
	messages []*SignatureMessage,
	key *PublicKeyWithGenerators,
	curve *ml.Curve,
) *ml.G1 {
	const basesOffset = 2

	cb := NewCommitmentBuilder(len(messages) + basesOffset)

	cb.Add(curve.GenG1, curve.NewZrFromInt(1))
	cb.Add(key.H0, s)

	for i := 0; i < len(messages); i++ {
		cb.Add(key.H[messages[i].Idx], messages[i].FR)
	}

	return cb.Build()
}

type commitmentBuilder struct {
	bases   []*ml.G1
	scalars []*ml.Zr
}

func NewCommitmentBuilder(expectedSize int) *commitmentBuilder {
	return &commitmentBuilder{
		bases:   make([]*ml.G1, 0, expectedSize),
		scalars: make([]*ml.Zr, 0, expectedSize),
	}
}

func (cb *commitmentBuilder) Add(base *ml.G1, scalar *ml.Zr) {
	cb.bases = append(cb.bases, base)
	cb.scalars = append(cb.scalars, scalar)
}

func (cb *commitmentBuilder) Build() *ml.G1 {
	return sumOfG1Products(cb.bases, cb.scalars)
}

func sumOfG1Products(bases []*ml.G1, scalars []*ml.Zr) *ml.G1 {
	var res *ml.G1

	for i := 0; i < len(bases); i++ {
		b := bases[i]
		s := scalars[i]

		g := b.Mul(FrToRepr(s))
		if res == nil {
			res = g
		} else {
			res.Add(g)
		}
	}

	return res
}

func compareTwoPairings(p1 *ml.G1, q1 *ml.G2,
	p2 *ml.G1, q2 *ml.G2, curve *ml.Curve) bool {
	p := curve.Pairing2(q1, p1, q2, p2)
	p = curve.FExp(p)

	return p.IsUnity()
}

// ProofNonce is a nonce for Proof of Knowledge proof.
type ProofNonce struct {
	fr *ml.Zr
}

// ParseProofNonce creates a new ProofNonce from bytes.
func ParseProofNonce(proofNonceBytes []byte, curve *ml.Curve) *ProofNonce {
	return &ProofNonce{
		FrFromOKM(proofNonceBytes, curve),
	}
}

// ToBytes converts ProofNonce into bytes.
func (pn *ProofNonce) ToBytes() []byte {
	return FrToRepr(pn.fr).Bytes()
}
