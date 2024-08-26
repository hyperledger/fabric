/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bbs

import (
	"crypto/rand"

	ml "github.com/IBM/mathlib"
	"golang.org/x/crypto/blake2b"
)

func (b *BBSLib) parseFr(data []byte) *ml.Zr {
	return b.curve.NewZrFromBytes(data)
}

// nolint:gochecknoglobals
var f2192Bytes = []byte{
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
}

func f2192(curve *ml.Curve) *ml.Zr {
	return curve.NewZrFromBytes(f2192Bytes)
}

func FrFromOKM(message []byte, curve *ml.Curve) *ml.Zr {
	const (
		eightBytes = 8
		okmMiddle  = 24
	)

	// We pass a null key so error is impossible here.
	h, _ := blake2b.New384(nil) //nolint:errcheck

	// blake2b.digest() does not return an error.
	_, _ = h.Write(message)
	okm := h.Sum(nil)
	emptyEightBytes := make([]byte, eightBytes)

	elm := curve.NewZrFromBytes(append(emptyEightBytes, okm[:okmMiddle]...))
	elm = elm.Mul(f2192(curve))

	fr := curve.NewZrFromBytes(append(emptyEightBytes, okm[okmMiddle:]...))
	elm = elm.Plus(fr)

	return elm
}

func FrToRepr(fr *ml.Zr) *ml.Zr {
	return fr.Copy()
}

func MessagesToFr(messages [][]byte, curve *ml.Curve) []*SignatureMessage {
	messagesFr := make([]*SignatureMessage, len(messages))

	for i := range messages {
		messagesFr[i] = ParseSignatureMessage(messages[i], i, curve)
	}

	return messagesFr
}

func (b *BBSLib) createRandSignatureFr() *ml.Zr {
	return b.curve.NewRandomZr(rand.Reader)
}
