/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"crypto/sha256"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// nymSecretKey contains the nym secret key
type nymSecretKey struct {
	// SKI of this key
	ski []byte
	// sk is the idemix reference to the nym secret
	sk Big
	// pk is the idemix reference to the nym public part
	pk Ecp
	// exportable if true, sk can be exported via the Bytes function
	exportable bool
}

func computeSKI(serialise func() ([]byte, error)) ([]byte, error) {
	raw, err := serialise()
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil), nil

}

func NewNymSecretKey(sk Big, pk Ecp, exportable bool) (*nymSecretKey, error) {
	ski, err := computeSKI(sk.Bytes)
	if err != nil {
		return nil, err
	}

	return &nymSecretKey{ski: ski, sk: sk, pk: pk, exportable: exportable}, nil
}

func (k *nymSecretKey) Bytes() ([]byte, error) {
	if k.exportable {
		return k.sk.Bytes()
	}

	return nil, errors.New("not supported")
}

func (k *nymSecretKey) SKI() []byte {
	c := make([]byte, len(k.ski))
	copy(c, k.ski)
	return c
}

func (*nymSecretKey) Symmetric() bool {
	return false
}

func (*nymSecretKey) Private() bool {
	return true
}

func (k *nymSecretKey) PublicKey() (bccsp.Key, error) {
	ski, err := computeSKI(k.pk.Bytes)
	if err != nil {
		return nil, err
	}
	return &nymPublicKey{ski: ski, pk: k.pk}, nil
}

type nymPublicKey struct {
	// SKI of this key
	ski []byte
	// pk is the idemix reference to the nym public part
	pk Ecp
}

func NewNymPublicKey(pk Ecp) *nymPublicKey {
	return &nymPublicKey{pk: pk}
}

func (k *nymPublicKey) Bytes() ([]byte, error) {
	return k.pk.Bytes()
}

func (k *nymPublicKey) SKI() []byte {
	c := make([]byte, len(k.ski))
	copy(c, k.ski)
	return c
}

func (*nymPublicKey) Symmetric() bool {
	return false
}

func (*nymPublicKey) Private() bool {
	return false
}

func (k *nymPublicKey) PublicKey() (bccsp.Key, error) {
	return k, nil
}

// NymKeyDerivation derives nyms
type NymKeyDerivation struct {
	// Exportable is a flag to allow an issuer secret key to be marked as Exportable.
	// If a secret key is marked as Exportable, its Bytes method will return the key's byte representation.
	Exportable bool
	// User implements the underlying cryptographic algorithms
	User User
}

func (kd *NymKeyDerivation) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	userSecretKey, ok := k.(*userSecretKey)
	if !ok {
		return nil, errors.New("invalid key, expected *userSecretKey")
	}
	nymKeyDerivationOpts, ok := opts.(*bccsp.IdemixNymKeyDerivationOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *IdemixNymKeyDerivationOpts")
	}
	if nymKeyDerivationOpts.IssuerPK == nil {
		return nil, errors.New("invalid options, missing issuer public key")
	}
	issuerPK, ok := nymKeyDerivationOpts.IssuerPK.(*issuerPublicKey)
	if !ok {
		return nil, errors.New("invalid options, expected IssuerPK as *issuerPublicKey")
	}

	Nym, RandNym, err := kd.User.MakeNym(userSecretKey.sk, issuerPK.pk)
	if err != nil {
		return nil, err
	}

	return NewNymSecretKey(RandNym, Nym, kd.Exportable)
}

// NymPublicKeyImporter imports nym public keys
type NymPublicKeyImporter struct {
	// User implements the underlying cryptographic algorithms
	User User
}

func (i *NymPublicKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	bytes, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("invalid raw, expected byte array")
	}

	if len(bytes) == 0 {
		return nil, errors.New("invalid raw, it must not be nil")
	}

	pk, err := i.User.NewPublicNymFromBytes(bytes)
	if err != nil {
		return nil, err
	}

	return &nymPublicKey{pk: pk}, nil
}
