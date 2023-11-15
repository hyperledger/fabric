/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"crypto/ecdsa"
	"github.com/pkg/errors"

	"github.com/IBM/idemix/bccsp/types"
	bccsp "github.com/IBM/idemix/bccsp/types"
)

type Signer struct {
	SignatureScheme types.SignatureScheme
}

func (s *Signer) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	userSecretKey, ok := k.(*UserSecretKey)
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
	nymSk, ok := signerOpts.Nym.(*NymSecretKey)
	if !ok {
		return nil, errors.New("invalid nym key, expected *nymSecretKey")
	}

	sigma, meta, err := s.SignatureScheme.Sign(
		signerOpts.Credential,
		userSecretKey.Sk,
		nymSk.Pk,
		nymSk.Sk,
		ipk.pk,
		signerOpts.Attributes,
		digest,
		signerOpts.RhIndex,
		signerOpts.EidIndex,
		signerOpts.CRI,
		signerOpts.SigType,
		signerOpts.Metadata,
	)
	if err != nil {
		return nil, err
	}

	signerOpts.Metadata = meta

	return sigma, nil
}

type Verifier struct {
	SignatureScheme types.SignatureScheme
}

func (v *Verifier) AuditNymEid(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	issuerPublicKey, ok := k.(*issuerPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *issuerPublicKey")
	}

	signerOpts, ok := opts.(*bccsp.EidNymAuditOpts)
	if !ok {
		return false, errors.New("invalid options, expected *EidNymAuditOpts")
	}

	if len(signature) == 0 {
		return false, errors.New("invalid signature, it must not be empty")
	}

	err := v.SignatureScheme.AuditNymEid(
		issuerPublicKey.pk,
		signerOpts.EidIndex,
		signature,
		signerOpts.EnrollmentID,
		signerOpts.RNymEid,
		signerOpts.AuditVerificationType,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (v *Verifier) AuditNymRh(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	issuerPublicKey, ok := k.(*issuerPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *issuerPublicKey")
	}

	signerOpts, ok := opts.(*bccsp.RhNymAuditOpts)
	if !ok {
		return false, errors.New("invalid options, expected *RhNymAuditOpts")
	}

	if len(signature) == 0 {
		return false, errors.New("invalid signature, it must not be empty")
	}

	err := v.SignatureScheme.AuditNymRh(
		issuerPublicKey.pk,
		signerOpts.RhIndex,
		signature,
		signerOpts.RevocationHandle,
		signerOpts.RNymRh,
		signerOpts.AuditVerificationType,
	)
	if err != nil {
		return false, err
	}

	return true, nil
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

	var rPK *ecdsa.PublicKey
	if signerOpts.RevocationPublicKey != nil {
		revocationPK, ok := signerOpts.RevocationPublicKey.(*revocationPublicKey)
		if !ok {
			return false, errors.New("invalid options, expected *revocationPublicKey")
		}
		rPK = revocationPK.pubKey
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
		signerOpts.EidIndex,
		rPK,
		signerOpts.Epoch,
		signerOpts.VerificationType,
		signerOpts.Metadata,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}
