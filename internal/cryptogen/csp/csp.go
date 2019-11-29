/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package csp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// LoadPrivateKey loads a private key from a file in keystorePath.  It looks
// for a file ending in "_sk" and expects a PEM-encoded PKCS8 EC private key.
func LoadPrivateKey(keystorePath string) (*ecdsa.PrivateKey, error) {
	var priv *ecdsa.PrivateKey

	walkFunc := func(path string, info os.FileInfo, pathErr error) error {

		if !strings.HasSuffix(path, "_sk") {
			return nil
		}

		rawKey, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		priv, err = parsePrivateKeyPEM(rawKey)
		if err != nil {
			return errors.WithMessage(err, path)
		}

		return nil
	}

	err := filepath.Walk(keystorePath, walkFunc)
	if err != nil {
		return nil, err
	}

	return priv, err
}

func parsePrivateKeyPEM(rawKey []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(rawKey)
	if block == nil {
		return nil, errors.New("bytes are not PEM encoded")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.WithMessage(err, "pem bytes are not PKCS8 encoded ")
	}

	priv, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("pem bytes do not contain an EC private key")
	}
	return priv, nil
}

// GeneratePrivateKey creates an EC private key using a P-256 curve and stores
// it in keystorePath.
func GeneratePrivateKey(keystorePath string) (*ecdsa.PrivateKey, error) {

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate private key")
	}

	pkcs8Encoded, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal private key")
	}

	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Encoded})

	keyFile := filepath.Join(keystorePath, "priv_sk")
	err = ioutil.WriteFile(keyFile, pemEncoded, 0600)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to save private key to file %s", keyFile)
	}

	return priv, err
}

/**
ECDSA signer implements the crypto.Signer interface for ECDSA keys.  The
Sign method ensures signatures are created with Low S values since Fabric
normalizes all signatures to Low S.
See https://github.com/bitcoin/bips/blob/master/bip-0146.mediawiki#low_s
for more detail.
*/
type ECDSASigner struct {
	PrivateKey *ecdsa.PrivateKey
}

// Public returns the ecdsa.PublicKey associated with PrivateKey.
func (e *ECDSASigner) Public() crypto.PublicKey {
	return &e.PrivateKey.PublicKey
}

// Sign signs the digest and ensures that signatures use the Low S value.
func (e *ECDSASigner) Sign(rand io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand, e.PrivateKey, digest)
	if err != nil {
		return nil, err
	}

	// ensure Low S signatures
	sig := toLowS(
		e.PrivateKey.PublicKey,
		ECDSASignature{
			R: r,
			S: s,
		},
	)

	// return marshaled aignature
	return asn1.Marshal(sig)
}

/**
When using ECDSA, both (r,s) and (r, -s mod n) are valid signatures.  In order
to protect against signature malleability attacks, Fabric normalizes all
signatures to a canonical form where s is at most half the order of the curve.
In order to make signatures compliant with what Fabric expects, toLowS creates
signatures in this canonical form.
*/
func toLowS(key ecdsa.PublicKey, sig ECDSASignature) ECDSASignature {
	// calculate half order of the curve
	halfOrder := new(big.Int).Div(key.Curve.Params().N, big.NewInt(2))
	// check if s is greater than half order of curve
	if sig.S.Cmp(halfOrder) == 1 {
		// Set s to N - s so that s will be less than or equal to half order
		sig.S.Sub(key.Params().N, sig.S)
	}
	return sig
}

type ECDSASignature struct {
	R, S *big.Int
}
