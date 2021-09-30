/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"bytes"

	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/handlers"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// CredRequest encapsulates the idemix algorithms to produce (sign) a credential request
// and verify it. Recall that a credential request is produced by a user,
// and it is verified by the issuer at credential creation time.
type CredRequest struct {
	Translator idemix.Translator
	Idemix     *idemix.Idemix
}

// Sign produces an idemix credential request. It takes in input a user secret key and
// an issuer public key.
func (cr *CredRequest) Sign(sk *math.Zr, ipk handlers.IssuerPublicKey, nonce []byte) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}
	if len(nonce) != cr.Idemix.Curve.FieldBytes {
		return nil, errors.Errorf("invalid issuer nonce, expected length %d, got %d", cr.Idemix.Curve.FieldBytes, len(nonce))
	}

	credRequest, err := cr.Idemix.NewCredRequest(
		sk,
		nonce,
		iipk.PK,
		newRandOrPanic(cr.Idemix.Curve),
		cr.Translator,
	)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(credRequest)
}

// Verify checks that the passed credential request is valid with the respect to the passed
// issuer public key.
func (cr *CredRequest) Verify(credentialRequest []byte, ipk handlers.IssuerPublicKey, nonce []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	credRequest := &idemix.CredRequest{}
	err = proto.Unmarshal(credentialRequest, credRequest)
	if err != nil {
		return err
	}

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	err = credRequest.Check(iipk.PK, cr.Idemix.Curve, cr.Translator)
	if err != nil {
		return err
	}

	// Nonce checks
	if len(nonce) != cr.Idemix.Curve.FieldBytes {
		return errors.Errorf("invalid issuer nonce, expected length %d, got %d", cr.Idemix.Curve.FieldBytes, len(nonce))
	}
	if !bytes.Equal(nonce, credRequest.IssuerNonce) {
		return errors.Errorf("invalid nonce, expected [%v], got [%v]", nonce, credRequest.IssuerNonce)
	}

	return nil
}
