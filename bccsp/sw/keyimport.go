/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sw

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"reflect"

	"github.com/hyperledger/fabric/bccsp"

	oqs "github.com/hyperledger/fabric/pq-crypto"
)

type aes256ImportKeyOptsKeyImporter struct{}

func (*aes256ImportKeyOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	aesRaw, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected byte array.")
	}

	if aesRaw == nil {
		return nil, errors.New("Invalid raw material. It must not be nil.")
	}

	if len(aesRaw) != 32 {
		return nil, fmt.Errorf("Invalid Key Length [%d]. Must be 32 bytes", len(aesRaw))
	}

	return &aesPrivateKey{aesRaw, false}, nil
}

type hmacImportKeyOptsKeyImporter struct{}

func (*hmacImportKeyOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	aesRaw, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected byte array.")
	}

	if len(aesRaw) == 0 {
		return nil, errors.New("Invalid raw material. It must not be nil.")
	}

	return &aesPrivateKey{aesRaw, false}, nil
}

type ecdsaPKIXPublicKeyImportOptsKeyImporter struct{}

func (*ecdsaPKIXPublicKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected byte array.")
	}

	if len(der) == 0 {
		return nil, errors.New("Invalid raw. It must not be nil.")
	}

	lowLevelKey, err := derToPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("Failed converting PKIX to ECDSA public key [%s]", err)
	}

	ecdsaPK, ok := lowLevelKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("Failed casting to ECDSA public key. Invalid raw material.")
	}

	return &ecdsaPublicKey{ecdsaPK}, nil
}

type ecdsaPrivateKeyImportOptsKeyImporter struct{}

func (*ecdsaPrivateKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("[ECDSADERPrivateKeyImportOpts] Invalid raw material. Expected byte array.")
	}

	if len(der) == 0 {
		return nil, errors.New("[ECDSADERPrivateKeyImportOpts] Invalid raw. It must not be nil.")
	}

	lowLevelKey, err := derToPrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("Failed converting PKIX to ECDSA public key [%s]", err)
	}

	ecdsaSK, ok := lowLevelKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("Failed casting to ECDSA private key. Invalid raw material.")
	}

	return &ecdsaPrivateKey{ecdsaSK}, nil
}

type ecdsaGoPublicKeyImportOptsKeyImporter struct{}

func (*ecdsaGoPublicKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	lowLevelKey, ok := raw.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected *ecdsa.PublicKey.")
	}

	return &ecdsaPublicKey{lowLevelKey}, nil
}

type x509PublicKeyImportOptsKeyImporter struct {
	bccsp *CSP
}

func (ki *x509PublicKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	x509Cert, ok := raw.(*x509.Certificate)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected *x509.Certificate.")
	}

	pk := x509Cert.PublicKey

	switch pk := pk.(type) {
	case *ecdsa.PublicKey:
		return ki.bccsp.KeyImporters[reflect.TypeOf(&bccsp.ECDSAGoPublicKeyImportOpts{})].KeyImport(
			pk,
			&bccsp.ECDSAGoPublicKeyImportOpts{Temporary: opts.Ephemeral()})
	case *rsa.PublicKey:
		// This path only exists to support environments that use RSA certificate
		// authorities to issue ECDSA certificates.
		return &rsaPublicKey{pubKey: pk}, nil
	default:
		return nil, errors.New("Certificate's public key type not recognized. Supported keys: [ECDSA, RSA]")
	}
}

type x509AltPublicKeyImportOptsKeyImporter struct {
	bccsp *CSP
}

func (ki *x509AltPublicKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	x509Cert, ok := raw.(*x509.Certificate)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected *x509.Certificate.")
	}

	pub, err := oqs.ParseSubjectAltPublicKeyInfoExtension(x509Cert.Extensions)
	if err != nil {
		return nil, errors.New("Unable to parse X509 alternate public key extension")
	}
	// Perhaps there was no alternate key provided. This is valid (the cert is purely classical), but we won't know
	// this until now, when we first try to parse the cert extensions. In this case, key is nil.
	if pub == nil {
		// Ideally, we would just return (nil, nil) from this case. However, no other implementation of KeyImport can
		// fail to find a key (in a non-error situation). As such, csp.KeyImport (which called this function via
		// reflection) next proceeds to store the key it found, which requires a non-nil Key.
		// Enter the black magic. In order to prevent KeyImport from taking further action with this key, we modify its
		// options to specify it as "temporary" -- don't save it, don't do anything, just return it.
		// This lets the user's KeyImport call return without error, and fits the backwards-compatible scheme used
		// throughout for hybridization: if the object is classical, the quantum key will be nil.
		opts.(*bccsp.X509AltPublicKeyImportOpts).Temporary = true
		return nil, nil

	}
	return ki.bccsp.KeyImporters[reflect.TypeOf(&bccsp.OQSGoPublicKeyImportOpts{})].KeyImport(
		pub,
		&bccsp.OQSGoPublicKeyImportOpts{Temporary: opts.Ephemeral()})
}

type oqsGoPublicKeyImportOptsKeyImporter struct{}

func (*oqsGoPublicKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	lowLevelKey, ok := raw.(*oqs.PublicKey)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected *oqs.PublicKey.")
	}

	return &oqsPublicKey{lowLevelKey}, nil
}

type oqsPKIXPublicKeyImportOptsKeyImporter struct{}

func (*oqsPKIXPublicKeyImportOptsKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (bccsp.Key, error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("Invalid raw material. Expected byte array.")
	}

	if len(der) == 0 {
		return nil, errors.New("Invalid raw. It must not be nil.")
	}

	lowLevelKey, err := oqs.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("Failed converting PKIX to OQS public key [%s]", err)
	}

	oqsPK, ok := lowLevelKey.(*oqs.PublicKey)
	if !ok {
		return nil, errors.New("Failed casting to OQS public key. Invalid raw material.")
	}

	return &oqsPublicKey{oqsPK}, nil
}
