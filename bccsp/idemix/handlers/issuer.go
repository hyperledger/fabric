/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// issuerSecretKey contains the issuer secret key
// and implements the bccsp.Key interface
type issuerSecretKey struct {
	// sk is the idemix reference to the issuer key
	sk IssuerSecretKey
	// exportable if true, sk can be exported via the Bytes function
	exportable bool
}

func NewIssuerSecretKey(sk IssuerSecretKey, exportable bool) *issuerSecretKey {
	return &issuerSecretKey{sk: sk, exportable: exportable}
}

func (k *issuerSecretKey) Bytes() ([]byte, error) {
	if k.exportable {
		return k.sk.Bytes()
	}

	return nil, errors.New("not exportable")
}

func (k *issuerSecretKey) SKI() []byte {
	pk, err := k.PublicKey()
	if err != nil {
		return nil
	}

	return pk.SKI()
}

func (*issuerSecretKey) Symmetric() bool {
	return false
}

func (*issuerSecretKey) Private() bool {
	return true
}

func (k *issuerSecretKey) PublicKey() (bccsp.Key, error) {
	return &issuerPublicKey{k.sk.Public()}, nil
}

// issuerPublicKey contains the issuer public key
// and implements the bccsp.Key interface
type issuerPublicKey struct {
	pk IssuerPublicKey
}

func NewIssuerPublicKey(pk IssuerPublicKey) *issuerPublicKey {
	return &issuerPublicKey{pk}
}

func (k *issuerPublicKey) Bytes() ([]byte, error) {
	return k.pk.Bytes()
}

func (k *issuerPublicKey) SKI() []byte {
	return k.pk.Hash()
}

func (*issuerPublicKey) Symmetric() bool {
	return false
}

func (*issuerPublicKey) Private() bool {
	return false
}

func (k *issuerPublicKey) PublicKey() (bccsp.Key, error) {
	return k, nil
}

// IssuerKeyGen generates issuer secret keys.
type IssuerKeyGen struct {
	// exportable is a flag to allow an issuer secret key to be marked as exportable.
	// If a secret key is marked as exportable, its Bytes method will return the key's byte representation.
	Exportable bool
	// Issuer implements the underlying cryptographic algorithms
	Issuer Issuer
}

func (g *IssuerKeyGen) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	o, ok := opts.(*bccsp.IdemixIssuerKeyGenOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *bccsp.IdemixIssuerKeyGenOpts")
	}

	// Create a new key pair
	key, err := g.Issuer.NewKey(o.AttributeNames)
	if err != nil {
		return nil, err
	}

	return &issuerSecretKey{exportable: g.Exportable, sk: key}, nil
}

// IssuerPublicKeyImporter imports issuer public keys
type IssuerPublicKeyImporter struct {
	// Issuer implements the underlying cryptographic algorithms
	Issuer Issuer
}

func (i *IssuerPublicKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("invalid raw, expected byte array")
	}

	if len(der) == 0 {
		return nil, errors.New("invalid raw, it must not be nil")
	}

	o, ok := opts.(*bccsp.IdemixIssuerPublicKeyImportOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *bccsp.IdemixIssuerPublicKeyImportOpts")
	}

	pk, err := i.Issuer.NewPublicKeyFromBytes(raw.([]byte), o.AttributeNames)
	if err != nil {
		return nil, err
	}

	return &issuerPublicKey{pk}, nil
}
