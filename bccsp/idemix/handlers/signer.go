/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

type Signer struct {
	SignatureScheme SignatureScheme
}

func (s *Signer) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	userSecretKey, ok := k.(*userSecretKey)
	if !ok {
		return nil, errors.New("invalid key, expected *userSecretKey")
	}

	signerOpts, ok := opts.(*bccsp.IdemixSignerOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *IdemixSignerOpts")
	}

	// Issuer public key
	if signerOpts.IssuerPK == nil {
		return nil, errors.New("invalid options, missing issuer public key")
	}
	ipk, ok := signerOpts.IssuerPK.(*issuerPublicKey)
	if !ok {
		return nil, errors.New("invalid issuer public key, expected *issuerPublicKey")
	}

	// Nym
	if signerOpts.Nym == nil {
		return nil, errors.New("invalid options, missing nym key")
	}
	nymSk, ok := signerOpts.Nym.(*nymSecretKey)
	if !ok {
		return nil, errors.New("invalid nym key, expected *nymSecretKey")
	}

	sigma, err := s.SignatureScheme.Sign(
		signerOpts.Credential,
		userSecretKey.sk,
		nymSk.pk, nymSk.sk,
		ipk.pk,
		signerOpts.Attributes,
		digest,
		signerOpts.RhIndex,
		signerOpts.CRI,
	)
	if err != nil {
		return nil, err
	}

	return sigma, nil
}

type Verifier struct {
	SignatureScheme SignatureScheme
}

func (v *Verifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	issuerPublicKey, ok := k.(*issuerPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *issuerPublicKey")
	}

	signerOpts, ok := opts.(*bccsp.IdemixSignerOpts)
	if !ok {
		return false, errors.New("invalid options, expected *IdemixSignerOpts")
	}

	rpk, ok := signerOpts.RevocationPublicKey.(*revocationPublicKey)
	if !ok {
		return false, errors.New("invalid options, expected *revocationPublicKey")
	}

	if len(signature) == 0 {
		return false, errors.New("invalid signature, it must not be empty")
	}

	err := v.SignatureScheme.Verify(
		issuerPublicKey.pk,
		signature,
		digest,
		signerOpts.Attributes,
		signerOpts.RhIndex,
		rpk.pubKey,
		signerOpts.Epoch,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}
