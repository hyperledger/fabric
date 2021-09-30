/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
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

func NewRevocationPublicKey(pubKey *ecdsa.PublicKey) *revocationPublicKey {
	return &revocationPublicKey{pubKey: pubKey}
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

func (g *RevocationKeyGen) KeyGen(opts bccsp.KeyGenOpts) (bccsp.Key, error) {
	// Create a new key pair
	key, err := g.Revocation.NewKey()
	if err != nil {
		return nil, err
	}

	return &revocationSecretKey{exportable: g.Exportable, privKey: key}, nil
}

// RevocationPublicKeyImporter imports revocation public key
type RevocationPublicKeyImporter struct {
}

func (i *RevocationPublicKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("invalid raw, expected byte array")
	}

	if len(der) == 0 {
		return nil, errors.New("invalid raw, it must not be nil")
	}

	blockPub, _ := pem.Decode(raw.([]byte))
	if blockPub == nil {
		return nil, errors.New("Failed to decode revocation ECDSA public key")
	}
	revocationPk, err := x509.ParsePKIXPublicKey(blockPub.Bytes)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to parse revocation ECDSA public key bytes")
	}
	ecdsaPublicKey, isECDSA := revocationPk.(*ecdsa.PublicKey)
	if !isECDSA {
		return nil, errors.Errorf("key is of type %v, not of type ECDSA", reflect.TypeOf(revocationPk))
	}

	return &revocationPublicKey{ecdsaPublicKey}, nil
}

// RevocationKeyImporter imports revocation key
type RevocationKeyImporter struct {
	// exportable is a flag to allow an revocation secret key to be marked as exportable.
	// If a secret key is marked as exportable, its Bytes method will return the key's byte representation.
	Exportable bool
	// Revocation implements the underlying cryptographic algorithms
	Revocation Revocation
}

func (i *RevocationKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("invalid raw, expected byte array")
	}

	if len(der) == 0 {
		return nil, errors.New("invalid raw, it must not be nil")
	}

	key, err := i.Revocation.NewKeyFromBytes(raw.([]byte))
	if err != nil {
		return nil, err
	}

	return &revocationSecretKey{
		privKey:    key,
		exportable: i.Exportable,
	}, nil
}

type CriSigner struct {
	Revocation Revocation
}

func (s *CriSigner) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) ([]byte, error) {
	revocationSecretKey, ok := k.(*revocationSecretKey)
	if !ok {
		return nil, errors.New("invalid key, expected *revocationSecretKey")
	}
	criOpts, ok := opts.(*bccsp.IdemixCRISignerOpts)
	if !ok {
		return nil, errors.New("invalid options, expected *IdemixCRISignerOpts")
	}

	return s.Revocation.Sign(
		revocationSecretKey.privKey,
		criOpts.UnrevokedHandles,
		criOpts.Epoch,
		criOpts.RevocationAlgorithm,
	)
}

type CriVerifier struct {
	Revocation Revocation
}

func (v *CriVerifier) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (bool, error) {
	revocationPublicKey, ok := k.(*revocationPublicKey)
	if !ok {
		return false, errors.New("invalid key, expected *revocationPublicKey")
	}
	criOpts, ok := opts.(*bccsp.IdemixCRISignerOpts)
	if !ok {
		return false, errors.New("invalid options, expected *IdemixCRISignerOpts")
	}
	if len(signature) == 0 {
		return false, errors.New("invalid signature, it must not be empty")
	}

	err := v.Revocation.Verify(
		revocationPublicKey.pubKey,
		signature,
		criOpts.Epoch,
		criOpts.RevocationAlgorithm,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}
