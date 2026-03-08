/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package aries

import (
	"crypto/rand"
	"errors"
	"fmt"

	math "github.com/IBM/mathlib"
	ml "github.com/IBM/mathlib"
	"github.com/hyperledger/aries-bbs-go/bbs"
)

// BlindedMessages represents a set of messages prepared
// (blinded) to be submitted to a signer for a blind signature.
type BlindedMessages struct {
	PK  *bbs.PublicKeyWithGenerators
	S   *ml.Zr
	C   *ml.G1
	PoK *POKOfBlindedMessages
}

func (b *BlindedMessages) Bytes() []byte {
	bytes := make([]byte, 0)

	bytes = append(bytes, b.C.Compressed()...)
	bytes = append(bytes, b.PoK.C.Compressed()...)
	bytes = append(bytes, b.PoK.ProofC.ToBytes()...)

	return bytes
}

func ParseBlindedMessages(bytes []byte, curve *ml.Curve) (*BlindedMessages, error) {
	offset := 0

	C, err := curve.NewG1FromCompressed(bytes[offset : offset+curve.CompressedG1ByteSize])
	if err != nil {
		return nil, fmt.Errorf("parse G1 point (C): %w", err)
	}

	offset += curve.CompressedG1ByteSize

	PoKC, err := curve.NewG1FromCompressed(bytes[offset : offset+curve.CompressedG1ByteSize])
	if err != nil {
		return nil, fmt.Errorf("parse G1 point (PoKC): %w", err)
	}

	offset += curve.CompressedG1ByteSize

	proof, err := bbs.NewBBSLib(curve).ParseProofG1(bytes[offset:])
	if err != nil {
		return nil, fmt.Errorf("parse G1 proof: %w", err)
	}

	return &BlindedMessages{
		C: C,
		PoK: &POKOfBlindedMessages{
			C:      PoKC,
			ProofC: proof,
		},
	}, nil
}

// POKOfBlindedMessages is the zero-knowledge proof that the
// requester knows the messages they have submitted for blind
// signature in the form of a Pedersen commitment.
type POKOfBlindedMessages struct {
	C      *ml.G1
	ProofC *bbs.ProofG1
}

// VerifyProof verifies the correctness of the zero knowledge
// proof against the supplied commitment, challenge and public key.
func (b *POKOfBlindedMessages) VerifyProof(messages []bool, commitment *ml.G1, challenge *ml.Zr, PK *bbs.PublicKey) error {
	pubKeyWithGenerators, err := PK.ToPublicKeyWithGenerators(len(messages))
	if err != nil {
		return fmt.Errorf("build generators from public key: %w", err)
	}

	bases := []*ml.G1{pubKeyWithGenerators.H0}

	for i, in := range messages {
		if !in {
			continue
		}

		bases = append(bases, pubKeyWithGenerators.H[i])
	}

	err = b.ProofC.Verify(bases, commitment, challenge)
	if err != nil {
		return errors.New("invalid proof")
	}

	return nil
}

// VerifyBlinding verifies that `msgCommit` is a valid
// commitment of a set of messages against the appropriate bases.
func VerifyBlinding(messageBitmap []bool, msgCommit *ml.G1, bmProof *POKOfBlindedMessages, PK *bbs.PublicKey, nonce []byte, curve *math.Curve) error {
	challengeBytes := msgCommit.Bytes()
	challengeBytes = append(challengeBytes, bmProof.C.Bytes()...)
	challengeBytes = append(challengeBytes, nonce...)

	return bmProof.VerifyProof(messageBitmap, msgCommit, bbs.FrFromOKM(challengeBytes, curve), PK)
}

// BlindMessages constructs a commitment to a set of messages
// that need to be blinded before signing, and generates the
// corresponding ZKP.
func BlindMessages(messages [][]byte, PK *bbs.PublicKey, blindedMsgCount int, nonce []byte, curve *ml.Curve) (*BlindedMessages, error) {
	zrs := make([]*ml.Zr, len(messages))

	for i, msg := range messages {
		if len(msg) == 0 {
			continue
		}

		zrs[i] = bbs.FrFromOKM(msg, curve)
	}

	return BlindMessagesZr(zrs, PK, blindedMsgCount, nonce, curve)
}

// BlindMessagesZr constructs a commitment to a set of messages
// that need to be blinded before signing, and generates the
// corresponding ZKP.
func BlindMessagesZr(zrs []*ml.Zr, PK *bbs.PublicKey, blindedMsgCount int, nonce []byte, curve *ml.Curve) (*BlindedMessages, error) {
	pubKeyWithGenerators, err := PK.ToPublicKeyWithGenerators(len(zrs))
	if err != nil {
		return nil, fmt.Errorf("build generators from public key: %w", err)
	}

	commit := bbs.NewBBSLib(curve).NewProverCommittingG1()
	cb := bbs.NewCommitmentBuilder(blindedMsgCount + 1)
	secrets := make([]*ml.Zr, 0, blindedMsgCount+1)

	s := curve.NewRandomZr(rand.Reader)

	commit.Commit(pubKeyWithGenerators.H0)
	cb.Add(pubKeyWithGenerators.H0, s)
	secrets = append(secrets, s)

	for i, zr := range zrs {
		if zr == nil {
			continue
		}

		commit.Commit(pubKeyWithGenerators.H[i])
		cb.Add(pubKeyWithGenerators.H[i], zr)
		secrets = append(secrets, zr)
	}

	C := cb.Build()
	U := commit.Finish()

	challengeBytes := C.Bytes()
	challengeBytes = append(challengeBytes, U.Commitment.Bytes()...)
	challengeBytes = append(challengeBytes, nonce...)

	return &BlindedMessages{
		PK: pubKeyWithGenerators,
		S:  s,
		C:  C,
		PoK: &POKOfBlindedMessages{
			C:      U.Commitment,
			ProofC: U.GenerateProof(bbs.FrFromOKM(challengeBytes, curve), secrets),
		},
	}, nil
}

// BlindSign signs disclosed and blinded messages using private key in compressed form.
func BlindSign(messages []*bbs.SignatureMessage, msgCount int, commitment *ml.G1, privKeyBytes []byte, curve *math.Curve) ([]byte, error) {
	bl := bbs.NewBBSLib(curve)

	privKey, err := bl.UnmarshalPrivateKey(privKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshal private key: %w", err)
	}

	pkwg, err := privKey.PublicKey().ToPublicKeyWithGenerators(msgCount)
	if err != nil {
		return nil, fmt.Errorf("unmarshal PublicKey: %w", err)
	}

	if len(messages) == 0 {
		return nil, errors.New("messages are not defined")
	}

	// signer adds its component
	cb := bbs.NewCommitmentBuilder(len(messages) + 2)
	for _, msg := range messages {
		cb.Add(pkwg.H[msg.Idx], msg.FR)
	}
	cb.Add(commitment, curve.NewZrFromInt(1))
	cb.Add(curve.GenG1, curve.NewZrFromInt(1))
	comm := cb.Build()

	return bbs.New(curve).SignWithKeyB(comm, msgCount, privKey)
}

// UnblindSign converts a signature over some blind messages into a standard signature.
func UnblindSign(sigBytes []byte, S *ml.Zr, curve *ml.Curve) ([]byte, error) {
	signature, err := bbs.NewBBSLib(curve).ParseSignature(sigBytes)
	if err != nil {
		return nil, fmt.Errorf("parse signature: %w", err)
	}

	signature.S = curve.ModAdd(signature.S, S, curve.GroupOrder)

	return signature.ToBytes()
}
