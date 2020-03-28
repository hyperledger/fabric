/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signer

import (
	"crypto"
	"crypto/x509"
	"io"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

// bccspCryptoSigner is the BCCSP-based implementation of a crypto.Signer
type bccspCryptoSigner struct {
	csp bccsp.BCCSP
	key bccsp.Key
	pk  interface{}
}

// New returns a new BCCSP-based crypto.Signer
// for the given BCCSP instance and key.
func New(csp bccsp.BCCSP, key bccsp.Key) (crypto.Signer, error) {
	// Validate arguments
	if csp == nil {
		return nil, errors.New("bccsp instance must be different from nil.")
	}
	if key == nil {
		return nil, errors.New("key must be different from nil.")
	}
	if key.Symmetric() {
		return nil, errors.New("key must be asymmetric.")
	}

	// Marshall the bccsp public key as a crypto.PublicKey
	pub, err := key.PublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "failed getting public key")
	}

	raw, err := pub.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling public key")
	}

	pk, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling der to public key")
	}

	return &bccspCryptoSigner{csp, key, pk}, nil
}

// Public returns the public key corresponding to the opaque,
// private key.
func (s *bccspCryptoSigner) Public() crypto.PublicKey {
	return s.pk
}

// Sign signs digest with the private key, possibly using entropy from rand.
// For an (EC)DSA key, it should be a DER-serialised, ASN.1 signature
// structure.
//
// Hash implements the SignerOpts interface and, in most cases, one can
// simply pass in the hash function used as opts. Sign may also attempt
// to type assert opts to other types in order to obtain algorithm
// specific values. See the documentation in each package for details.
//
// Note that when a signature of a hash of a larger message is needed,
// the caller is responsible for hashing the larger message and passing
// the hash (as digest) and the hash function (as opts) to Sign.
func (s *bccspCryptoSigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	return s.csp.Sign(s.key, digest, opts)
}
