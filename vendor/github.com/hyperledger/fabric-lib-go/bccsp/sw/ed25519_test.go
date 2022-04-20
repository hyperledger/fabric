/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sw

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyED25519(t *testing.T) {
	t.Parallel()

	// Generate a key
	_, lowLevelKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	msg := []byte("hello world")
	sigma, err := signED25519(&lowLevelKey, msg, nil)
	require.NoError(t, err)

	castedKey, _ := lowLevelKey.Public().(ed25519.PublicKey)
	valid, err := verifyED25519(&castedKey, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestEd25519SignerSign(t *testing.T) {
	t.Parallel()

	signer := &ed25519Signer{}
	verifierPrivateKey := &ed25519PrivateKeyVerifier{}
	verifierPublicKey := &ed25519PublicKeyKeyVerifier{}

	// Generate a key
	_, lowLevelKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	k := &ed25519PrivateKey{&lowLevelKey}
	pk, err := k.PublicKey()
	require.NoError(t, err)

	// Sign
	msg := []byte("Hello World")
	sigma, err := signer.Sign(k, msg, nil)
	require.NoError(t, err)
	require.NotNil(t, sigma)

	// Verify
	castedKey, _ := lowLevelKey.Public().(ed25519.PublicKey)
	valid, err := verifyED25519(&castedKey, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = verifierPrivateKey.Verify(k, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = verifierPublicKey.Verify(pk, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestEd25519PrivateKey(t *testing.T) {
	t.Parallel()

	_, lowLevelKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	k := &ed25519PrivateKey{&lowLevelKey}

	require.False(t, k.Symmetric())
	require.True(t, k.Private())

	_, err = k.Bytes()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Not supported.")

	k.privKey = nil
	ski := k.SKI()
	require.Nil(t, ski)

	k.privKey = &lowLevelKey
	ski = k.SKI()
	raw := k.privKey.Public().(ed25519.PublicKey)
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	require.Equal(t, ski2, ski, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.NotNil(t, pk)
	ed25519PK, ok := pk.(*ed25519PublicKey)
	require.True(t, ok)
	castedKey, _ := lowLevelKey.Public().(ed25519.PublicKey)
	require.Equal(t, &castedKey, ed25519PK.pubKey)
}

func TestEd25519PublicKey(t *testing.T) {
	t.Parallel()

	_, lowLevelKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	castedKey, _ := lowLevelKey.Public().(ed25519.PublicKey)
	k := &ed25519PublicKey{&castedKey}

	require.False(t, k.Symmetric())
	require.False(t, k.Private())

	k.pubKey = nil
	ski := k.SKI()
	require.Nil(t, ski)

	k.pubKey = &castedKey
	ski = k.SKI()
	raw := *(k.pubKey)
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	require.Equal(t, ski, ski2, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.Equal(t, k, pk)

	bytes, err := k.Bytes()
	require.NoError(t, err)
	bytes2, err := x509.MarshalPKIXPublicKey(*k.pubKey)
	require.NoError(t, err)
	require.Equal(t, bytes2, bytes, "bytes are not computed in the right way.")
}
