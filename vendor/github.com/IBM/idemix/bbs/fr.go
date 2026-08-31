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

var (
	//nolint:gochecknoglobals
	f2192Bytes = []byte{
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
	}
	//nolint:gochecknoglobals
	f2192Cache = func() map[*ml.Curve]*ml.Zr {
		m := make(map[*ml.Curve]*ml.Zr, len(ml.Curves))
		for _, c := range ml.Curves {
			m[c] = c.NewZrFromBytes(f2192Bytes)
		}

		return m
	}()
)

func f2192(curve *ml.Curve) *ml.Zr {
	if cached, ok := f2192Cache[curve]; ok {
		return cached
	}

	return curve.NewZrFromBytes(f2192Bytes)
}

func FrFromOKM(message []byte, curve *ml.Curve) *ml.Zr {
	const (
		eightBytes = 8
		okmMiddle  = 24
	)

	// We pass a null key so error is impossible here.
	h, _ := blake2b.New384(nil)

	// blake2b.digest() does not return an error.
	_, _ = h.Write(message)
	okm := h.Sum(nil)

	buf := make([]byte, eightBytes+okmMiddle)
	// buf has leading 8 zeros
	copy(buf[eightBytes:], okm[:okmMiddle])

	elm := curve.NewZrFromBytes(buf)
	elm = elm.Mul(f2192(curve))

	buf2 := make([]byte, eightBytes+okmMiddle)
	copy(buf2[eightBytes:], okm[okmMiddle:])
	fr := curve.NewZrFromBytes(buf2)
	elm = elm.Plus(fr)

	return elm
}

func FrToRepr(fr *ml.Zr) *ml.Zr {
	return fr
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
