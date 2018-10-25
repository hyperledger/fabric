/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// revocationSecretKey contains the revocation secret key
// and implements the bccsp.Key interface
type revocationSecretKey struct {
	// sk is the idemix reference to the revocation key
	privKey *ecdsa.PrivateKey
	// exportable if true, sk can be exported via the Bytes function
	exportable bool
}

func NewRevocationSecretKey(sk *ecdsa.PrivateKey, exportable bool) *revocationSecretKey {
	return &revocationSecretKey{privKey: sk, exportable: exportable}
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *revocationSecretKey) Bytes() ([]byte, error) {
	if k.exportable {
		return k.privKey.D.Bytes(), nil
	}

	return nil, errors.New("not exportable")
}

// SKI returns the subject key identifier of this key.
func (k *revocationSecretKey) SKI() []byte {
	// Marshall the public key
	raw := elliptic.Marshal(k.privKey.Curve, k.privKey.PublicKey.X, k.privKey.PublicKey.Y)

	// Hash it
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false if this key is asymmetric
func (k *revocationSecretKey) Symmetric() bool {
	return false
}

// Private returns true if this key is a private key,
// false otherwise.
func (k *revocationSecretKey) Private() bool {
	return true
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *revocationSecretKey) PublicKey() (bccsp.Key, error) {
	return &revocationPublicKey{&k.privKey.PublicKey}, nil
}

type revocationPublicKey struct {
	pubKey *ecdsa.PublicKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *revocationPublicKey) Bytes() (raw []byte, err error) {
	raw, err = x509.MarshalPKIXPublicKey(k.pubKey)
	if err != nil {
		return nil, fmt.Errorf("Failed marshalling key [%s]", err)
	}
	return
}

// SKI returns the subject key identifier of this key.
func (k *revocationPublicKey) SKI() []byte {
	// Marshall the public key
	raw := elliptic.Marshal(k.pubKey.Curve, k.pubKey.X, k.pubKey.Y)

	// Hash it
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false if this key is asymmetric
func (k *revocationPublicKey) Symmetric() bool {
	return false
}

// Private returns true if this key is a private key,
// false otherwise.
func (k *revocationPublicKey) Private() bool {
	return false
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *revocationPublicKey) PublicKey() (bccsp.Key, error) {
	return k, nil
}

// RevocationKeyGen generates revocation secret keys.
type RevocationKeyGen struct {
	// exportable is a flag to allow an revocation secret key to be marked as exportable.
	// If a secret key is marked as exportable, its Bytes method will return the key's byte representation.
	Exportable bool
	// Revocation implements the underlying cryptographic algorithms
	Revocation Revocation
}

func (g *RevocationKeyGen) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	// Create a new key pair
	key, err := g.Revocation.NewKey()
	if err != nil {
		return nil, err
	}

	return &revocationSecretKey{exportable: g.Exportable, privKey: key}, nil
}
