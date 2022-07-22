/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package sw

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric-lib-go/bccsp"
)

type ed25519PrivateKey struct {
	privKey *ed25519.PrivateKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *ed25519PrivateKey) Bytes() ([]byte, error) {
	return nil, errors.New("Not supported.")
}

// SKI returns the subject key identifier of this key.
func (k *ed25519PrivateKey) SKI() []byte {
	if k.privKey == nil {
		return nil
	}

	// Marshall the public key
	raw := k.privKey.Public().(ed25519.PublicKey)

	// Hash it
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false if this key is asymmetric
func (k *ed25519PrivateKey) Symmetric() bool {
	return false
}

// Private returns true if this key is a private key,
// false otherwise.
func (k *ed25519PrivateKey) Private() bool {
	return true
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *ed25519PrivateKey) PublicKey() (bccsp.Key, error) {
	castedKey, ok := k.privKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("Error casting ed25519 public key")
	}
	return &ed25519PublicKey{&castedKey}, nil
}

type ed25519PublicKey struct {
	pubKey *ed25519.PublicKey
}

// Bytes converts this key to its byte representation,
// if this operation is allowed.
func (k *ed25519PublicKey) Bytes() (raw []byte, err error) {
	raw, err = x509.MarshalPKIXPublicKey(*k.pubKey)
	if err != nil {
		return nil, fmt.Errorf("Failed marshalling key [%s]", err)
	}
	return
}

// SKI returns the subject key identifier of this key.
func (k *ed25519PublicKey) SKI() []byte {
	if k.pubKey == nil {
		return nil
	}

	raw := *(k.pubKey)

	// Hash it
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

// Symmetric returns true if this key is a symmetric key,
// false if this key is asymmetric
func (k *ed25519PublicKey) Symmetric() bool {
	return false
}

// Private returns true if this key is a private key,
// false otherwise.
func (k *ed25519PublicKey) Private() bool {
	return false
}

// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
// This method returns an error in symmetric key schemes.
func (k *ed25519PublicKey) PublicKey() (bccsp.Key, error) {
	return k, nil
}
