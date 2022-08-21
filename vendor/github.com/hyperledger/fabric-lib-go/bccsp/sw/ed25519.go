/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package sw

import (
	"crypto/ed25519"

	"github.com/hyperledger/fabric-lib-go/bccsp"
)

func signED25519(k *ed25519.PrivateKey, msg []byte, opts bccsp.SignerOpts) ([]byte, error) {
	signature := ed25519.Sign(*k, msg)
	return signature, nil
}

func verifyED25519(k *ed25519.PublicKey, signature, msg []byte, opts bccsp.SignerOpts) (bool, error) {
	return ed25519.Verify(*k, msg, signature), nil
}

type ed25519Signer struct{}

func (s *ed25519Signer) Sign(k bccsp.Key, msg []byte, opts bccsp.SignerOpts) ([]byte, error) {
	return signED25519(k.(*ed25519PrivateKey).privKey, msg, opts)
}

type ed25519PrivateKeyVerifier struct{}

func (v *ed25519PrivateKeyVerifier) Verify(k bccsp.Key, signature, msg []byte, opts bccsp.SignerOpts) (bool, error) {
	castedKey, _ := (k.(*ed25519PrivateKey).privKey.Public()).(ed25519.PublicKey)
	return verifyED25519(&castedKey, signature, msg, opts)
}

type ed25519PublicKeyKeyVerifier struct{}

func (v *ed25519PublicKeyKeyVerifier) Verify(k bccsp.Key, signature, msg []byte, opts bccsp.SignerOpts) (bool, error) {
	return verifyED25519(k.(*ed25519PublicKey).pubKey, signature, msg, opts)
}
