/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	"reflect"

	"github.com/hyperledger/fabric/bccsp/idemix/bridge"

	"github.com/hyperledger/fabric/bccsp/idemix/handlers"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/pkg/errors"
)

type csp struct {
	bccsp.BCCSP
}

func New(keyStore bccsp.KeyStore) (*csp, error) {
	base, err := sw.New(keyStore)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating base bccsp")
	}

	csp := &csp{BCCSP: base}

	// key generators
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixIssuerKeyGenOpts{}), &handlers.IssuerKeyGen{Issuer: &bridge.Issuer{NewRand: bridge.NewRandOrPanic}})
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixUserSecretKeyGenOpts{}), &handlers.UserKeyGen{User: &bridge.User{NewRand: bridge.NewRandOrPanic}})
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixRevocationKeyGenOpts{}), &handlers.RevocationKeyGen{Revocation: &bridge.Revocation{}})

	// key derivers
	base.AddWrapper(reflect.TypeOf(handlers.NewUserSecretKey(nil, false)), &handlers.NymKeyDerivation{
		User: &bridge.User{NewRand: bridge.NewRandOrPanic},
	})

	// signers
	base.AddWrapper(reflect.TypeOf(handlers.NewUserSecretKey(nil, false)), &userSecreKeySignerMultiplexer{
		signer:                  &handlers.Signer{SignatureScheme: &bridge.SignatureScheme{NewRand: bridge.NewRandOrPanic}},
		nymSigner:               &handlers.NymSigner{NymSignatureScheme: &bridge.NymSignatureScheme{NewRand: bridge.NewRandOrPanic}},
		credentialRequestSigner: &handlers.CredentialRequestSigner{CredRequest: &bridge.CredRequest{NewRand: bridge.NewRandOrPanic}},
	})
	base.AddWrapper(reflect.TypeOf(handlers.NewIssuerSecretKey(nil, false)), &handlers.CredentialSigner{
		Credential: &bridge.Credential{NewRand: bridge.NewRandOrPanic},
	})
	base.AddWrapper(reflect.TypeOf(handlers.NewRevocationSecretKey(nil, false)), &handlers.CriSigner{
		Revocation: &bridge.Revocation{},
	})

	// verifiers
	base.AddWrapper(reflect.TypeOf(handlers.NewIssuerPublicKey(nil)), &issuerPublicKeyVerifierMultiplexer{
		verifier:                  &handlers.Verifier{SignatureScheme: &bridge.SignatureScheme{NewRand: bridge.NewRandOrPanic}},
		credentialRequestVerifier: &handlers.CredentialRequestVerifier{CredRequest: &bridge.CredRequest{NewRand: bridge.NewRandOrPanic}},
	})
	base.AddWrapper(reflect.TypeOf(handlers.NewNymPublicKey(nil)), &handlers.NymVerifier{
		NymSignatureScheme: &bridge.NymSignatureScheme{NewRand: bridge.NewRandOrPanic},
	})
	base.AddWrapper(reflect.TypeOf(handlers.NewUserSecretKey(nil, false)), &handlers.CredentialVerifier{
		Credential: &bridge.Credential{NewRand: bridge.NewRandOrPanic},
	})
	base.AddWrapper(reflect.TypeOf(handlers.NewRevocationPublicKey(nil)), &handlers.CriVerifier{
		Revocation: &bridge.Revocation{},
	})

	// importers
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixUserSecretKeyImportOpts{}), &handlers.UserKeyImporter{
		User: &bridge.User{},
	})
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixIssuerPublicKeyImportOpts{}), &handlers.IssuerPublicKeyImporter{
		Issuer: &bridge.Issuer{},
	})
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixNymPublicKeyImportOpts{}), &handlers.NymPublicKeyImporter{
		User: &bridge.User{},
	})
	base.AddWrapper(reflect.TypeOf(&bccsp.IdemixRevocationPublicKeyImportOpts{}), &handlers.RevocationPublicKeyImporter{})

	return csp, nil
}

type userSecreKeySignerMultiplexer struct {
	signer                  *handlers.Signer
	nymSigner               *handlers.NymSigner
	credentialRequestSigner *handlers.CredentialRequestSigner
}

func (s *userSecreKeySignerMultiplexer) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	switch opts.(type) {
	case *bccsp.IdemixSignerOpts:
		return s.signer.Sign(k, digest, opts)
	case *bccsp.IdemixNymSignerOpts:
		return s.nymSigner.Sign(k, digest, opts)
	case *bccsp.IdemixCredentialRequestSignerOpts:
		return s.credentialRequestSigner.Sign(k, digest, opts)
	default:
		return nil, errors.New("invalid opts, expected *bccsp.IdemixSignerOpt or *bccsp.IdemixNymSignerOpts or *bccsp.IdemixCredentialRequestSignerOpts")
	}
}

type issuerPublicKeyVerifierMultiplexer struct {
	verifier                  *handlers.Verifier
	credentialRequestVerifier *handlers.CredentialRequestVerifier
}

func (v *issuerPublicKeyVerifierMultiplexer) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	switch opts.(type) {
	case *bccsp.IdemixSignerOpts:
		return v.verifier.Verify(k, signature, digest, opts)
	case *bccsp.IdemixCredentialRequestSignerOpts:
		return v.credentialRequestVerifier.Verify(k, signature, digest, opts)
	default:
		return false, errors.New("invalid opts, expected *bccsp.IdemixSignerOpts or *bccsp.IdemixCredentialRequestSignerOpts")
	}
}
