/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bbs12381g2pub

import (
	"errors"
	"fmt"

	ml "github.com/IBM/mathlib"
)

// Signature defines BLS signature.
type Signature struct {
	A *ml.G1
	E *ml.Zr
	S *ml.Zr
}

// ParseSignature parses a Signature from bytes.
func ParseSignature(sigBytes []byte) (*Signature, error) {
	if len(sigBytes) != bls12381SignatureLen {
		return nil, errors.New("invalid size of signature")
	}

	pointG1, err := curve.NewG1FromCompressed(sigBytes[:g1CompressedSize])
	if err != nil {
		return nil, fmt.Errorf("deserialize G1 compressed signature: %w", err)
	}

	e := parseFr(sigBytes[g1CompressedSize : g1CompressedSize+frCompressedSize])
	s := parseFr(sigBytes[g1CompressedSize+frCompressedSize:])

	return &Signature{
		A: pointG1,
		E: e,
		S: s,
	}, nil
}

// ToBytes converts signature to bytes using compression of G1 point and E, S FR points.
func (s *Signature) ToBytes() ([]byte, error) {
	bytes := make([]byte, bls12381SignatureLen)

	copy(bytes, s.A.Compressed())
	copy(bytes[g1CompressedSize:g1CompressedSize+frCompressedSize], s.E.Bytes())
	copy(bytes[g1CompressedSize+frCompressedSize:], s.S.Bytes())

	return bytes, nil
}

// Verify is used for signature verification.
func (s *Signature) Verify(messages []*SignatureMessage, pubKey *PublicKeyWithGenerators) error {
	p1 := s.A

	q1 := curve.GenG2.Mul(frToRepr(s.E))
	q1.Add(pubKey.w)

	p2 := computeB(s.S, messages, pubKey)
	p2.Neg()

	if compareTwoPairings(p1, q1, p2, curve.GenG2) {
		return nil
	}

	return errors.New("invalid BLS12-381 signature")
}
