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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalECDSASignature(t *testing.T) {
	_, _, err := unmarshalECDSASignature(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed unmashalling signature [")

	_, _, err = unmarshalECDSASignature([]byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed unmashalling signature [")

	_, _, err = unmarshalECDSASignature([]byte{0})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed unmashalling signature [")

	sigma, err := marshalECDSASignature(big.NewInt(-1), big.NewInt(1))
	assert.NoError(t, err)
	_, _, err = unmarshalECDSASignature(sigma)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signature. R must be larger than zero")

	sigma, err = marshalECDSASignature(big.NewInt(0), big.NewInt(1))
	assert.NoError(t, err)
	_, _, err = unmarshalECDSASignature(sigma)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signature. R must be larger than zero")

	sigma, err = marshalECDSASignature(big.NewInt(1), big.NewInt(0))
	assert.NoError(t, err)
	_, _, err = unmarshalECDSASignature(sigma)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signature. S must be larger than zero")

	sigma, err = marshalECDSASignature(big.NewInt(1), big.NewInt(-1))
	assert.NoError(t, err)
	_, _, err = unmarshalECDSASignature(sigma)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signature. S must be larger than zero")

	sigma, err = marshalECDSASignature(big.NewInt(1), big.NewInt(1))
	assert.NoError(t, err)
	R, S, err := unmarshalECDSASignature(sigma)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(1), R)
	assert.Equal(t, big.NewInt(1), S)
}

func TestSignECDSA(t *testing.T) {
	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)

	// Induce an error on the underlying ecdsa algorithm
	msg := []byte("hello world")
	oldN := lowLevelKey.Params().N
	defer func() { lowLevelKey.Params().N = oldN }()
	lowLevelKey.Params().N = big.NewInt(0)
	_, err = signECDSA(lowLevelKey, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "zero parameter")
	lowLevelKey.Params().N = oldN

	oldCurveHalfOrders := curveHalfOrders
	curveHalfOrders = nil
	defer func() { curveHalfOrders = oldCurveHalfOrders }()
	_, err = signECDSA(lowLevelKey, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Curve not recognized [")
	curveHalfOrders = oldCurveHalfOrders
}

func TestVerifyECDSA(t *testing.T) {
	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)

	msg := []byte("hello world")
	sigma, err := signECDSA(lowLevelKey, msg, nil)
	assert.NoError(t, err)

	valid, err := verifyECDSA(&lowLevelKey.PublicKey, sigma, msg, nil)
	assert.NoError(t, err)
	assert.True(t, valid)

	_, err = verifyECDSA(&lowLevelKey.PublicKey, nil, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed unmashalling signature [")

	oldCurveHalfOrders := curveHalfOrders
	curveHalfOrders = nil
	defer func() { curveHalfOrders = oldCurveHalfOrders }()
	_, err = verifyECDSA(&lowLevelKey.PublicKey, sigma, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Curve not recognized [")
	curveHalfOrders = oldCurveHalfOrders

	_, err = verifyECDSA(&lowLevelKey.PublicKey, nil, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed unmashalling signature [")

	R, S, err := unmarshalECDSASignature(sigma)
	assert.NoError(t, err)
	S.Add(curveHalfOrders[elliptic.P256()], big.NewInt(1))
	sigmaWrongS, err := marshalECDSASignature(R, S)
	assert.NoError(t, err)
	_, err = verifyECDSA(&lowLevelKey.PublicKey, sigmaWrongS, msg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid S. Must be smaller than half the order [")
}

func TestEcdsaSignerSign(t *testing.T) {
	signer := &ecdsaSigner{}

	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	k := &ecdsaPrivateKey{lowLevelKey}

	// Sign
	msg := []byte("Hello World")
	sigma, err := signer.Sign(k, msg, nil)
	assert.NoError(t, err)
	assert.NotNil(t, sigma)

	// Verify
	valid, err := verifyECDSA(&lowLevelKey.PublicKey, sigma, msg, nil)
	assert.NoError(t, err)
	assert.True(t, valid)
}

func TestEcdsaPrivateKey(t *testing.T) {
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	k := &ecdsaPrivateKey{lowLevelKey}

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
	raw := elliptic.Marshal(k.privKey.Curve, k.privKey.PublicKey.X, k.privKey.PublicKey.Y)
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	assert.Equal(t, ski2, ski, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	assert.NoError(t, err)
	assert.NotNil(t, pk)
	ecdsaPK, ok := pk.(*ecdsaPublicKey)
	assert.True(t, ok)
	assert.Equal(t, &lowLevelKey.PublicKey, ecdsaPK.pubKey)
}

func TestEcdsaPublicKey(t *testing.T) {
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	k := &ecdsaPublicKey{&lowLevelKey.PublicKey}

	assert.False(t, k.Symmetric())
	assert.False(t, k.Private())

	k.pubKey = nil
	ski := k.SKI()
	assert.Nil(t, ski)

	k.pubKey = &lowLevelKey.PublicKey
	ski = k.SKI()
	raw := elliptic.Marshal(k.pubKey.Curve, k.pubKey.X, k.pubKey.Y)
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

	invalidCurve := &elliptic.CurveParams{Name: "P-Invalid"}
	invalidCurve.BitSize = 1024
	k.pubKey = &ecdsa.PublicKey{Curve: invalidCurve, X: big.NewInt(1), Y: big.NewInt(1)}
	_, err = k.Bytes()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed marshalling key [")
}
