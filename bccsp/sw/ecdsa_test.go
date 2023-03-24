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

	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/stretchr/testify/require"
)

func TestSignECDSABadParameter(t *testing.T) {
	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Induce an error on the underlying ecdsa algorithm
	curve := *elliptic.P256().Params()
	curve.N = big.NewInt(0)
	lowLevelKey.Curve = &curve

	_, err = signECDSA(lowLevelKey, []byte("hello world"), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "zero parameter")
}

func TestVerifyECDSA(t *testing.T) {
	t.Parallel()

	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	msg := []byte("hello world")
	sigma, err := signECDSA(lowLevelKey, msg, nil)
	require.NoError(t, err)

	valid, err := verifyECDSA(&lowLevelKey.PublicKey, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)

	_, err = verifyECDSA(&lowLevelKey.PublicKey, nil, msg, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed unmashalling signature [")

	R, S, err := utils.UnmarshalECDSASignature(sigma)
	require.NoError(t, err)
	S.Add(utils.GetCurveHalfOrdersAt(elliptic.P256()), big.NewInt(1))
	sigmaWrongS, err := utils.MarshalECDSASignature(R, S)
	require.NoError(t, err)
	_, err = verifyECDSA(&lowLevelKey.PublicKey, sigmaWrongS, msg, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid S. Must be smaller than half the order [")
}

func TestEcdsaSignerSign(t *testing.T) {
	t.Parallel()

	signer := &ecdsaSigner{}
	verifierPrivateKey := &ecdsaPrivateKeyVerifier{}
	verifierPublicKey := &ecdsaPublicKeyKeyVerifier{}

	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	k := &ecdsaPrivateKey{lowLevelKey}
	pk, err := k.PublicKey()
	require.NoError(t, err)

	// Sign
	msg := []byte("Hello World")
	sigma, err := signer.Sign(k, msg, nil)
	require.NoError(t, err)
	require.NotNil(t, sigma)

	// Verify
	valid, err := verifyECDSA(&lowLevelKey.PublicKey, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = verifierPrivateKey.Verify(k, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = verifierPublicKey.Verify(pk, sigma, msg, nil)
	require.NoError(t, err)
	require.True(t, valid)
}

func TestEcdsaPrivateKey(t *testing.T) {
	t.Parallel()

	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	k := &ecdsaPrivateKey{lowLevelKey}

	require.False(t, k.Symmetric())
	require.True(t, k.Private())

	_, err = k.Bytes()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Not supported.")

	k.privKey = nil
	ski := k.SKI()
	require.Nil(t, ski)

	k.privKey = lowLevelKey
	ski = k.SKI()
	raw := elliptic.Marshal(k.privKey.Curve, k.privKey.PublicKey.X, k.privKey.PublicKey.Y)
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	require.Equal(t, ski2, ski, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.NotNil(t, pk)
	ecdsaPK, ok := pk.(*ecdsaPublicKey)
	require.True(t, ok)
	require.Equal(t, &lowLevelKey.PublicKey, ecdsaPK.pubKey)
}

func TestEcdsaPublicKey(t *testing.T) {
	t.Parallel()

	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	k := &ecdsaPublicKey{&lowLevelKey.PublicKey}

	require.False(t, k.Symmetric())
	require.False(t, k.Private())

	k.pubKey = nil
	ski := k.SKI()
	require.Nil(t, ski)

	k.pubKey = &lowLevelKey.PublicKey
	ski = k.SKI()
	raw := elliptic.Marshal(k.pubKey.Curve, k.pubKey.X, k.pubKey.Y)
	hash := sha256.New()
	hash.Write(raw)
	ski2 := hash.Sum(nil)
	require.Equal(t, ski, ski2, "SKI is not computed in the right way.")

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.Equal(t, k, pk)

	bytes, err := k.Bytes()
	require.NoError(t, err)
	bytes2, err := x509.MarshalPKIXPublicKey(k.pubKey)
	require.NoError(t, err)
	require.Equal(t, bytes2, bytes, "bytes are not computed in the right way.")

	invalidCurve := &elliptic.CurveParams{Name: "P-Invalid"}
	invalidCurve.P = big.NewInt(1)
	invalidCurve.N = big.NewInt(1)
	invalidCurve.B = big.NewInt(1)
	invalidCurve.BitSize = 1024
	k.pubKey = &ecdsa.PublicKey{Curve: invalidCurve, X: big.NewInt(1), Y: big.NewInt(1)}

	defer func() {
		if r := recover(); r != nil {
			require.Contains(t, r, "crypto/elliptic: attempted operation on invalid point")
		}
	}()
	_, err = k.Bytes()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed marshalling key [")
}
