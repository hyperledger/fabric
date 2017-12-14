/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sw

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"strings"
	"testing"

	"github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/stretchr/testify/assert"
)

func TestRSAPrivateKey(t *testing.T) {
	t.Parallel()

	lowLevelKey, err := rsa.GenerateKey(rand.Reader, 512)
	assert.NoError(t, err)
	k := &rsaPrivateKey{lowLevelKey}

	assert.False(t, k.Symmetric())
	assert.True(t, k.Private())

	_, err = k.Bytes()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Not supported.")

	k.privKey = nil
	ski := k.SKI()
	assert.Nil(t, ski)

	k.privKey = lowLevelKey
	ski = k.SKI()
	raw, _ := asn1.Marshal(rsaPublicKeyASN{N: k.privKey.N, E: k.privKey.E})
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	assert.Equal(t, ski2, ski, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	assert.NoError(t, err)
	assert.NotNil(t, pk)
	ecdsaPK, ok := pk.(*rsaPublicKey)
	assert.True(t, ok)
	assert.Equal(t, &lowLevelKey.PublicKey, ecdsaPK.pubKey)
}

func TestRSAPublicKey(t *testing.T) {
	t.Parallel()

	lowLevelKey, err := rsa.GenerateKey(rand.Reader, 512)
	assert.NoError(t, err)
	k := &rsaPublicKey{&lowLevelKey.PublicKey}

	assert.False(t, k.Symmetric())
	assert.False(t, k.Private())

	k.pubKey = nil
	ski := k.SKI()
	assert.Nil(t, ski)

	k.pubKey = &lowLevelKey.PublicKey
	ski = k.SKI()
	raw, _ := asn1.Marshal(rsaPublicKeyASN{N: k.pubKey.N, E: k.pubKey.E})
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	assert.Equal(t, ski, ski2, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	assert.NoError(t, err)
	assert.Equal(t, k, pk)

	bytes, err := k.Bytes()
	assert.NoError(t, err)
	bytes2, err := x509.MarshalPKIXPublicKey(k.pubKey)
	assert.Equal(t, bytes2, bytes, "bytes are not computed in the right way.")
}

func TestRSASignerSign(t *testing.T) {
	t.Parallel()

	signer := &rsaSigner{}
	verifierPrivateKey := &rsaPrivateKeyVerifier{}
	verifierPublicKey := &rsaPublicKeyKeyVerifier{}

	// Generate a key
	lowLevelKey, err := rsa.GenerateKey(rand.Reader, 1024)
	assert.NoError(t, err)
	k := &rsaPrivateKey{lowLevelKey}
	pk, err := k.PublicKey()
	assert.NoError(t, err)

	// Sign
	msg := []byte("Hello World!!!")

	_, err = signer.Sign(k, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid options. Must be different from nil.")

	_, err = signer.Sign(k, msg, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256})
	assert.Error(t, err)

	hf := sha256.New()
	hf.Write(msg)
	digest := hf.Sum(nil)
	sigma, err := signer.Sign(k, digest, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256})
	assert.NoError(t, err)

	opts := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
	// Verify against msg, must fail
	err = rsa.VerifyPSS(&lowLevelKey.PublicKey, crypto.SHA256, msg, sigma, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "crypto/rsa: verification error")

	// Verify against digest, must succeed
	err = rsa.VerifyPSS(&lowLevelKey.PublicKey, crypto.SHA256, digest, sigma, opts)
	assert.NoError(t, err)

	valid, err := verifierPrivateKey.Verify(k, sigma, msg, opts)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "crypto/rsa: verification error"))

	valid, err = verifierPrivateKey.Verify(k, sigma, digest, opts)
	assert.NoError(t, err)
	assert.True(t, valid)

	valid, err = verifierPublicKey.Verify(pk, sigma, msg, opts)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "crypto/rsa: verification error"))

	valid, err = verifierPublicKey.Verify(pk, sigma, digest, opts)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestRSAVerifiersInvalidInputs(t *testing.T) {
	t.Parallel()

	verifierPrivate := &rsaPrivateKeyVerifier{}
	_, err := verifierPrivate.Verify(nil, nil, nil, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid options. It must not be nil."))

	_, err = verifierPrivate.Verify(nil, nil, nil, &mocks.SignerOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Opts type not recognized ["))

	verifierPublic := &rsaPublicKeyKeyVerifier{}
	_, err = verifierPublic.Verify(nil, nil, nil, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid options. It must not be nil."))

	_, err = verifierPublic.Verify(nil, nil, nil, &mocks.SignerOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Opts type not recognized ["))
}
