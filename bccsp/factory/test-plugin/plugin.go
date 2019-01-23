/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package main

import (
	"hash"

	"github.com/hyperledger/fabric/bccsp"
)

type impl struct{}

// New returns a new instance of the BCCSP implementation
func New(config map[string]interface{}) (bccsp.BCCSP, error) {
	return &impl{}, nil
}

// KeyGen generates a key using opts.
func (csp *impl) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	return nil, nil
}

// KeyDeriv derives a key from k using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	return nil, nil
}

// KeyImport imports a key from its raw representation using opts.
// The opts argument should be appropriate for the primitive used.
func (csp *impl) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	return nil, nil
}

// GetKey returns the key this CSP associates to
// the Subject Key Identifier ski.
func (csp *impl) GetKey(ski []byte) (k bccsp.Key, err error) {
	return nil, nil
}

// Hash hashes messages msg using options opts.
// If opts is nil, the default hash function will be used.
func (csp *impl) Hash(msg []byte, opts bccsp.HashOpts) (hash []byte, err error) {
	return nil, nil
}

// GetHash returns and instance of hash.Hash using options opts.
// If opts is nil, the default hash function will be returned.
func (csp *impl) GetHash(opts bccsp.HashOpts) (h hash.Hash, err error) {
	return nil, nil
}

// Sign signs digest using key k.
// The opts argument should be appropriate for the algorithm used.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest).
func (csp *impl) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	return nil, nil
}

// Verify verifies signature against key k and digest
// The opts argument should be appropriate for the algorithm used.
func (csp *impl) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	return true, nil
}

// Encrypt encrypts plaintext using key k.
// The opts argument should be appropriate for the algorithm used.
func (csp *impl) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {
	return nil, nil
}

// Decrypt decrypts ciphertext using key k.
// The opts argument should be appropriate for the algorithm used.
func (csp *impl) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {
	return nil, nil
}
