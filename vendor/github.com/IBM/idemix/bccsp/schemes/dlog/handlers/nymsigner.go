/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	bccsp "github.com/IBM/idemix/bccsp/schemes"
	"github.com/pkg/errors"
)

type NymSigner struct {
	NymSignatureScheme NymSignatureScheme
}

func (s *NymSigner) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	userSecretKey, ok := k.(*UserSecretKey)
	if !ok {
		return nil, errors.New("invalid key, expected *userSecretKey")
	}

	signerOpts, ok := opts.(*bccsp.IdemixNymSignerOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *IdemixNymSignerOpts")
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
	nymSk, ok := signerOpts.Nym.(*NymSecretKey)
	if !ok {
		return nil, errors.New("invalid nym key, expected *nymSecretKey")
	}

	sigma, err := s.NymSignatureScheme.Sign(
		userSecretKey.Sk,
		nymSk.Pk, nymSk.Sk,
		ipk.pk,
		digest)
	if err != nil {
		return nil, err
	}

	return sigma, nil
}

type NymVerifier struct {
	NymSignatureScheme NymSignatureScheme
}

func (v *NymVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	nymPublicKey, ok := k.(*nymPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *nymPublicKey")
	}

	signerOpts, ok := opts.(*bccsp.IdemixNymSignerOpts)
	if !ok {
		return false, errors.New("invalid options, expected *IdemixNymSignerOpts")
	}

	if signerOpts.IssuerPK == nil {
		return false, errors.New("invalid options, missing issuer public key")
	}
	ipk, ok := signerOpts.IssuerPK.(*issuerPublicKey)
	if !ok {
		return false, errors.New("invalid issuer public key, expected *issuerPublicKey")
	}

	if len(signature) == 0 {
		return false, errors.New("invalid signature, it must not be empty")
	}

	err := v.NymSignatureScheme.Verify(
		ipk.pk,
		nymPublicKey.pk,
		signature,
		digest)
	if err != nil {
		return false, err
	}

	return true, nil
}
