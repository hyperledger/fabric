/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// CredentialRequestSigner produces credential requests
type CredentialRequestSigner struct {
	// CredRequest implements the underlying cryptographic algorithms
	CredRequest CredRequest
}

func (c *CredentialRequestSigner) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	userSecretKey, ok := k.(*userSecretKey)
	if !ok {
		return nil, errors.New("invalid key, expected *userSecretKey")
	}
	credentialRequestSignerOpts, ok := opts.(*bccsp.IdemixCredentialRequestSignerOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *IdemixCredentialRequestSignerOpts")
	}
	if credentialRequestSignerOpts.IssuerPK == nil {
		return nil, errors.New("invalid options, missing issuer public key")
	}
	issuerPK, ok := credentialRequestSignerOpts.IssuerPK.(*issuerPublicKey)
	if !ok {
		return nil, errors.New("invalid options, expected IssuerPK as *issuerPublicKey")
	}
	if len(digest) != 0 {
		return nil, errors.New("invalid digest, it must be empty")
	}

	return c.CredRequest.Sign(userSecretKey.sk, issuerPK.pk)
}

// CredentialRequestVerifier verifies credential requests
type CredentialRequestVerifier struct {
	// CredRequest implements the underlying cryptographic algorithms
	CredRequest CredRequest
}

func (c *CredentialRequestVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	issuerPublicKey, ok := k.(*issuerPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *issuerPublicKey")
	}
	if len(digest) != 0 {
		return false, errors.New("invalid digest, it must be empty")
	}

	err := c.CredRequest.Verify(signature, issuerPublicKey.pk)
	if err != nil {
		return false, err
	}

	return true, nil
}
