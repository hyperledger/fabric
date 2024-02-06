/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package aries

import (
	"fmt"

	"github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type CredRequest struct {
	Curve *math.Curve
}

// Sign creates a new Credential Request, the first message of the interactive credential issuance protocol
// (from user to issuer)
func (c *CredRequest) Blind(sk *math.Zr, key types.IssuerPublicKey, nonce []byte) ([]byte, []byte, error) {
	ipk, ok := key.(*IssuerPublicKey)
	if !ok {
		return nil, nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	zrs := make([]*math.Zr, ipk.N+1)
	zrs[UserSecretKeyIndex] = sk

	blindedMsg, err := BlindMessagesZr(zrs, ipk.PK, 1, nonce, c.Curve)
	if err != nil {
		return nil, nil, fmt.Errorf("BlindMessagesZr failed [%w]", err)
	}

	return blindedMsg.Bytes(), blindedMsg.S.Bytes(), nil
}

// Verify verifies the credential request
func (c *CredRequest) BlindVerify(credRequest []byte, key types.IssuerPublicKey, nonce []byte) error {
	ipk, ok := key.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	bitmap := make([]bool, ipk.N+1)
	bitmap[UserSecretKeyIndex] = true

	blindedMsg, err := ParseBlindedMessages(credRequest, c.Curve)
	if err != nil {
		return fmt.Errorf("ParseBlindedMessages failed [%w]", err)
	}

	return VerifyBlinding(bitmap, blindedMsg.C, blindedMsg.PoK, ipk.PK, nonce)
}

// Unblind takes a blinded signature and a blinding and produces a standard signature
func (c *CredRequest) Unblind(signature, blinding []byte) ([]byte, error) {
	S := c.Curve.NewZrFromBytes(blinding)

	credential := &Credential{}
	err := proto.Unmarshal(signature, credential)
	if err != nil {
		return nil, fmt.Errorf("proto.Unmarshal failed [%w]", err)
	}

	sig, err := UnblindSign(credential.Cred, S, c.Curve)
	if err != nil {
		return nil, fmt.Errorf("bls.UnblindSign failed [%w]", err)
	}

	credential.Cred = sig

	return proto.Marshal(credential)
}
