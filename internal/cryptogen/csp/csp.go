/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package csp

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// LoadPrivateKey loads a private key from a file in keystorePath.  It looks
// for a file ending in "_sk" and expects a PEM-encoded PKCS8 EC private key.
func LoadPrivateKey(keystorePath string) (crypto.PrivateKey, error) {
	var priv crypto.PrivateKey

	walkFunc := func(path string, info os.FileInfo, pathErr error) error {
		if !strings.HasSuffix(path, "_sk") {
			return nil
		}

		rawKey, err := os.ReadFile(path)
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

func parsePrivateKeyPEM(rawKey []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(rawKey)
	if block == nil {
		return nil, errors.New("bytes are not PEM encoded")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.WithMessage(err, "pem bytes are not PKCS8 encoded ")
	}

	_, isEcdsa := key.(*ecdsa.PrivateKey)
	_, isEd25519 := key.(ed25519.PrivateKey)
	if !isEcdsa && !isEd25519 {
		return nil, errors.New("pem bytes do not contain an ECDSA nor ed25519 private key")
	}

	return key, nil
}

// GeneratePrivateKey creates an ecdsa private key using a P-256 curve or an ed25519 key
// and stores it in keystorePath.
func GeneratePrivateKey(keystorePath string, keyAlg string) (crypto.PrivateKey, error) {
	var priv crypto.PrivateKey
	var err error

	switch keyAlg {
	case "ecdsa":
		priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ed25519":
		_, priv, err = ed25519.GenerateKey(rand.Reader)
	default:
		err = errors.WithMessagef(err, "Unsupported key algorithm: %s", keyAlg)
	}
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate private key")
	}

	pkcs8Encoded, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal private key")
	}

	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Encoded})

	keyFile := filepath.Join(keystorePath, "priv_sk")
	err = os.WriteFile(keyFile, pemEncoded, 0o600)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to save private key to file %s", keyFile)
	}

	return priv, err
}

/*
*
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

	// return marshaled signature
	return asn1.Marshal(sig)
}

/*
*
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

type ED25519Signer struct {
	PrivateKey ed25519.PrivateKey
}

// Public returns the ed25519.PublicKey associated with PrivateKey.
func (e *ED25519Signer) Public() crypto.PublicKey {
	return e.PrivateKey.Public()
}

// Sign signs the digest
func (e *ED25519Signer) Sign(rand io.Reader, msg []byte, opts crypto.SignerOpts) ([]byte, error) {
	sig := ed25519.Sign(e.PrivateKey, msg)

	return sig, nil
}
