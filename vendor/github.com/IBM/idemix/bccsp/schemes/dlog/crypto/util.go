/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"io"

	amcl "github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
)

func appendBytes(data []byte, index int, bytesToAdd []byte) int {
	copy(data[index:], bytesToAdd)
	return index + len(bytesToAdd)
}
func appendBytesG1(data []byte, index int, E *math.G1) int {
	return appendBytes(data, index, E.Bytes())
}
func appendBytesG2(data []byte, index int, E *math.G2) int {
	return appendBytes(data, index, E.Bytes())
}
func appendBytesBig(data []byte, index int, B *math.Zr) int {
	return appendBytes(data, index, B.Bytes())
}
func appendBytesString(data []byte, index int, s string) int {
	bytes := []byte(s)
	copy(data[index:], bytes)
	return index + len(bytes)
}

// MakeNym creates a new unlinkable pseudonym
func (i *Idemix) MakeNym(sk *math.Zr, IPk *IssuerPublicKey, rng io.Reader, t Translator) (*math.G1, *math.Zr, error) {
	return makeNym(sk, IPk, rng, i.Curve, t)
}

func makeNym(sk *math.Zr, IPk *IssuerPublicKey, rng io.Reader, curve *math.Curve, t Translator) (*math.G1, *math.Zr, error) {
	// Construct a commitment to the sk
	// Nym = h_{sk}^sk \cdot h_r^r
	RandNym := curve.NewRandomZr(rng)
	HSk, err := t.G1FromProto(IPk.HSk)
	if err != nil {
		return nil, nil, err
	}
	HRand, err := t.G1FromProto(IPk.HRand)
	if err != nil {
		return nil, nil, err
	}
	Nym := HSk.Mul2(sk, HRand, RandNym)
	return Nym, RandNym, nil
}

func (i *Idemix) MakeNymFromBytes(raw []byte) (*math.G1, *math.Zr, error) {
	return makeNymFromBytes(i.Curve, raw, i.Translator)
}

func makeNymFromBytes(curve *math.Curve, raw []byte, translator Translator) (*math.G1, *math.Zr, error) {
	RandNym := curve.NewZrFromBytes(raw[:curve.FieldBytes])
	pk, err := translator.G1FromProto(&amcl.ECP{
		X: raw[curve.FieldBytes : 2*curve.FieldBytes],
		Y: raw[2*curve.FieldBytes:],
	})
	if err != nil {
		return nil, nil, err
	}

	return pk, RandNym, nil
}
