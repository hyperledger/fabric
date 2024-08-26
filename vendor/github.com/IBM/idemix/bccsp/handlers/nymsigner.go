/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"fmt"

	"github.com/IBM/idemix/bccsp/types"
	"github.com/pkg/errors"
)

type NymSigner struct {
	NymSignatureScheme          types.NymSignatureScheme
	SmartcardNymSignatureScheme types.SmartcardNymSignatureScheme
}

func (s *NymSigner) Sign(k types.Key, digest []byte, opts types.SignerOpts) ([]byte, error) {
	signerOpts, ok := opts.(*types.IdemixNymSignerOpts)
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

	// handle the smartcard case
	if signerOpts.IsSmartcard {
		if s.SmartcardNymSignatureScheme == nil {
			return nil, fmt.Errorf("smartcard mode is unsupported")
		}

		if signerOpts.Smartcard == nil {
			return nil, fmt.Errorf("no s/w smartcard supplied in opts")
		}

		sigma, nym, rNym, err := s.SmartcardNymSignatureScheme.Sign(signerOpts.Smartcard, ipk.pk, digest)
		if err != nil {
			return nil, err
		}

		signerOpts.NymG1 = nym
		signerOpts.RNym = rNym

		return sigma, nil
	}

	userSecretKey, ok := k.(*UserSecretKey)
	if !ok {
		return nil, errors.New("invalid key, expected *userSecretKey")
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
	NymSignatureScheme          types.NymSignatureScheme
	SmartcardNymSignatureScheme types.SmartcardNymSignatureScheme
}

func (v *NymVerifier) Verify(k types.Key, signature, digest []byte, opts types.SignerOpts) (bool, error) {
	nymPublicKey, ok := k.(*nymPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *nymPublicKey")
	}

	signerOpts, ok := opts.(*types.IdemixNymSignerOpts)
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

	// handle the smartcard case
	if signerOpts.IsSmartcard {
		if v.SmartcardNymSignatureScheme == nil {
			return false, fmt.Errorf("smartcard mode is unsupported")
		}
		if signerOpts.NymEid == nil {
			return false, fmt.Errorf("nym eid missing")
		}

		err := v.SmartcardNymSignatureScheme.Verify(
			ipk.pk,
			signerOpts.NymEid,
			signature,
			digest)
		if err != nil {
			return false, err
		}

		return true, nil
	}

	err := v.NymSignatureScheme.Verify(
		ipk.pk,
		nymPublicKey.pk,
		signature,
		digest,
		signerOpts.SKIndex)
	if err != nil {
		return false, err
	}

	return true, nil
}
