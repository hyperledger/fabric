/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bbs12381g2pub

import (
	"crypto/rand"

	ml "github.com/IBM/mathlib"
	"golang.org/x/crypto/blake2b"
)

func parseFr(data []byte) *ml.Zr {
	return curve.NewZrFromBytes(data)
}

// nolint:gochecknoglobals
var f2192Bytes = []byte{
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
}

func f2192() *ml.Zr {
	return curve.NewZrFromBytes(f2192Bytes)
}

func FrFromOKM(message []byte) *ml.Zr {
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
	elm = elm.Mul(f2192())

	fr := curve.NewZrFromBytes(append(emptyEightBytes, okm[okmMiddle:]...))
	elm = elm.Plus(fr)

	return elm
}

func frToRepr(fr *ml.Zr) *ml.Zr {
	return fr.Copy()
}

func messagesToFr(messages [][]byte) []*SignatureMessage {
	messagesFr := make([]*SignatureMessage, len(messages))

	for i := range messages {
		messagesFr[i] = ParseSignatureMessage(messages[i], i)
	}

	return messagesFr
}

func createRandSignatureFr() *ml.Zr {
	return curve.NewRandomZr(rand.Reader)
}
