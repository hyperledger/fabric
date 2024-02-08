/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package bbs12381g2pub contains BBS+ signing primitives and keys. Although it can be used directly, it is recommended
// to use BBS+ keys created by the kms along with the framework's Crypto service.
//
// The default local Crypto service is found at:
// "github.com/ale-linux/aries-framework-go/component/kmscrypto/crypto/tinkcrypto"
//
// While the remote Crypto service is found at:
// "github.com/ale-linux/aries-framework-go/component/kmscrypto/crypto/webkms"
package bbs12381g2pub

import (
	"errors"
	"fmt"
	"sort"

	ml "github.com/IBM/mathlib"
)

// nolint:gochecknoglobals
var curve = ml.Curves[ml.BLS12_381_BBS]

// BBSG2Pub defines BBS+ signature scheme where public key is a point in the field of G2.
// BBS+ signature scheme (as defined in https://eprint.iacr.org/2016/663.pdf, section 4.3).
type BBSG2Pub struct{}

// New creates a new BBSG2Pub.
func New() *BBSG2Pub {
	return &BBSG2Pub{}
}

// Number of bytes in scalar compressed form.
const frCompressedSize = 32

var (
	// nolint:gochecknoglobals
	// Signature length.
	bls12381SignatureLen = curve.CompressedG1ByteSize + 2*frCompressedSize

	// nolint:gochecknoglobals
	// Default BLS 12-381 public key length in G2 field.
	bls12381G2PublicKeyLen = curve.CompressedG2ByteSize

	// nolint:gochecknoglobals
	// Number of bytes in G1 X coordinate.
	g1CompressedSize = curve.CompressedG1ByteSize

	// nolint:gochecknoglobals
	// Number of bytes in G1 X and Y coordinates.
	g1UncompressedSize = curve.G1ByteSize

	// nolint:gochecknoglobals
	// Number of bytes in G2 X(a, b) and Y(a, b) coordinates.
	g2UncompressedSize = curve.G2ByteSize

	// nolint:gochecknoglobals
	// Number of bytes in scalar uncompressed form.
	frUncompressedSize = curve.ScalarByteSize
)

// Verify makes BLS BBS12-381 signature verification.
func (bbs *BBSG2Pub) Verify(messages [][]byte, sigBytes, pubKeyBytes []byte) error {
	signature, err := ParseSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	pubKey, err := UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	messagesCount := len(messages)

	publicKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return fmt.Errorf("build generators from public key: %w", err)
	}

	messagesFr := messagesToFr(messages)

	return signature.Verify(messagesFr, publicKeyWithGenerators)
}

// Sign signs the one or more messages using private key in compressed form.
func (bbs *BBSG2Pub) Sign(messages [][]byte, privKeyBytes []byte) ([]byte, error) {
	privKey, err := UnmarshalPrivateKey(privKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshal private key: %w", err)
	}

	if len(messages) == 0 {
		return nil, errors.New("messages are not defined")
	}

	return bbs.SignWithKey(messages, nil, privKey)
}

// VerifyProof verifies BBS+ signature proof for one ore more revealed messages.
func (bbs *BBSG2Pub) VerifyProof(messagesBytes [][]byte, proof, nonce, pubKeyBytes []byte) error {

	messages := messagesToFr(messagesBytes)

	return bbs.VerifyProofFr(messages, proof, nonce, pubKeyBytes)
}

// VerifyProof verifies BBS+ signature proof for one ore more revealed messages.
func (bbs *BBSG2Pub) VerifyProofFr(messages []*SignatureMessage, proof, nonce, pubKeyBytes []byte) error {
	payload, err := ParsePoKPayload(proof)
	if err != nil {
		return fmt.Errorf("parse signature proof: %w", err)
	}

	signatureProof, err := ParseSignatureProof(proof[payload.LenInBytes():])
	if err != nil {
		return fmt.Errorf("parse signature proof: %w", err)
	}

	pubKey, err := UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	publicKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(payload.messagesCount)
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
	proofNonce := ParseProofNonce(nonce)
	proofNonceBytes := proofNonce.ToBytes()
	challengeBytes = append(challengeBytes, proofNonceBytes...)
	proofChallenge := FrFromOKM(challengeBytes)

	return signatureProof.Verify(proofChallenge, publicKeyWithGenerators, revealedMessages, messages)
}

// DeriveProof derives a proof of BBS+ signature with some messages disclosed.
func (bbs *BBSG2Pub) DeriveProof(messages [][]byte, sigBytes, nonce, pubKeyBytes []byte,
	revealedIndexes []int) ([]byte, error) {

	return bbs.DeriveProofZr(messagesToFr(messages), sigBytes, nonce, pubKeyBytes, revealedIndexes)
}

// DeriveProof derives a proof of BBS+ signature with some messages disclosed.
func (bbs *BBSG2Pub) DeriveProofZr(messagesFr []*SignatureMessage, sigBytes, nonce, pubKeyBytes []byte,
	revealedIndexes []int) ([]byte, error) {

	if len(revealedIndexes) == 0 {
		return nil, errors.New("no message to reveal")
	}

	sort.Ints(revealedIndexes)

	messagesCount := len(messagesFr)

	pubKey, err := UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	publicKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return nil, fmt.Errorf("build generators from public key: %w", err)
	}

	signature, err := ParseSignature(sigBytes)
	if err != nil {
		return nil, fmt.Errorf("parse signature: %w", err)
	}

	pokSignature, err := NewPoKOfSignature(signature, messagesFr, revealedIndexes, publicKeyWithGenerators)
	if err != nil {
		return nil, fmt.Errorf("init proof of knowledge signature: %w", err)
	}

	challengeBytes := pokSignature.ToBytes()

	proofNonce := ParseProofNonce(nonce)
	proofNonceBytes := proofNonce.ToBytes()
	challengeBytes = append(challengeBytes, proofNonceBytes...)

	proofChallenge := FrFromOKM(challengeBytes)

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
func (bbs *BBSG2Pub) SignWithKey(messages [][]byte, commitment *ml.G1, privKey *PrivateKey) ([]byte, error) {

	messagesFr := make([]*SignatureMessage, 0, len(messages))

	for i, msg := range messages {
		if len(msg) == 0 {
			continue
		}

		messagesFr = append(messagesFr, ParseSignatureMessage(messages[i], i))
	}

	return bbs.SignWithKeyFr(messagesFr, len(messages), commitment, privKey)
}

// SignWithKey signs the one or more messages using BBS+ key pair.
func (bbs *BBSG2Pub) SignWithKeyFr(messagesFr []*SignatureMessage, messagesCount int, commitment *ml.G1, privKey *PrivateKey) ([]byte, error) {
	var err error

	pubKey := privKey.PublicKey()

	pubKeyWithGenerators, err := pubKey.ToPublicKeyWithGenerators(messagesCount)
	if err != nil {
		return nil, fmt.Errorf("build generators from public key: %w", err)
	}

	e, s := createRandSignatureFr(), createRandSignatureFr()
	exp := privKey.FR.Copy()
	exp = exp.Plus(e)
	exp.InvModP(curve.GroupOrder)

	b := computeB(s, messagesFr, pubKeyWithGenerators)
	if len(messagesFr) != messagesCount {
		b.Add(commitment)
	}

	sig := b.Mul(frToRepr(exp))

	signature := &Signature{
		A: sig,
		E: e,
		S: s,
	}

	return signature.ToBytes()
}

func computeB(s *ml.Zr, messages []*SignatureMessage, key *PublicKeyWithGenerators) *ml.G1 {
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

		g := b.Mul(frToRepr(s))
		if res == nil {
			res = g
		} else {
			res.Add(g)
		}
	}

	return res
}

func compareTwoPairings(p1 *ml.G1, q1 *ml.G2,
	p2 *ml.G1, q2 *ml.G2) bool {
	p := curve.Pairing2(q1, p1, q2, p2)
	p = curve.FExp(p)

	return p.IsUnity()
}

// ProofNonce is a nonce for Proof of Knowledge proof.
type ProofNonce struct {
	fr *ml.Zr
}

// ParseProofNonce creates a new ProofNonce from bytes.
func ParseProofNonce(proofNonceBytes []byte) *ProofNonce {
	return &ProofNonce{
		FrFromOKM(proofNonceBytes),
	}
}

// ToBytes converts ProofNonce into bytes.
func (pn *ProofNonce) ToBytes() []byte {
	return frToRepr(pn.fr).Bytes()
}
