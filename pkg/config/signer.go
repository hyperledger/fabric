/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"io"
	"math/big"
)

// SigningIdentity is an MSP Identity that can be used to sign configuration
// updates.
type SigningIdentity struct {
	Certificate *x509.Certificate
	PrivateKey  crypto.PrivateKey
	MSPID       string
}

type ecdsaSignature struct {
	R, S *big.Int
}

// Sign performs ECDSA sign with SigningIdentity's private key on given digest.
// It ensures signatures are created with Low S values since Fabric normalizes
// all signatures to Low S.
// See https://github.com/bitcoin/bips/blob/master/bip-0146.mediawiki#low_s
// for more detail.
func (s *SigningIdentity) Sign(reader io.Reader, digest []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	switch pk := s.PrivateKey.(type) {
	case *ecdsa.PrivateKey:
		rr, ss, err := ecdsa.Sign(reader, pk, digest)
		if err != nil {
			return nil, err
		}

		// ensure Low S signatures
		sig := toLowS(
			pk.PublicKey,
			ecdsaSignature{
				R: rr,
				S: ss,
			},
		)

		return asn1.Marshal(sig)
	default:
		return nil, fmt.Errorf("signing with private key of type %T not supported", pk)
	}
}

// toLows normalizes all signatures to a canonical form where s is at most
// half the order of the curve. By doing so, it compliant with what Fabric
// expected as well as protect against signature malleability attacks.
func toLowS(key ecdsa.PublicKey, sig ecdsaSignature) ecdsaSignature {
	// calculate half order of the curve
	halfOrder := new(big.Int).Div(key.Curve.Params().N, big.NewInt(2))
	// check if s is greater than half order of curve
	if sig.S.Cmp(halfOrder) == 1 {
		// Set s to N - s so that s will be less than or equal to half order
		sig.S.Sub(key.Params().N, sig.S)
	}

	return sig
}
