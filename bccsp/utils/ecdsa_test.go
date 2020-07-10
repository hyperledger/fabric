/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmarshalECDSASignature(t *testing.T) {
	_, _, err := UnmarshalECDSASignature(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmashalling signature [")

	_, _, err = UnmarshalECDSASignature([]byte{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmashalling signature [")

	_, _, err = UnmarshalECDSASignature([]byte{0})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed unmashalling signature [")

	sigma, err := MarshalECDSASignature(big.NewInt(-1), big.NewInt(1))
	require.NoError(t, err)
	_, _, err = UnmarshalECDSASignature(sigma)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature, R must be larger than zero")

	sigma, err = MarshalECDSASignature(big.NewInt(0), big.NewInt(1))
	require.NoError(t, err)
	_, _, err = UnmarshalECDSASignature(sigma)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature, R must be larger than zero")

	sigma, err = MarshalECDSASignature(big.NewInt(1), big.NewInt(0))
	require.NoError(t, err)
	_, _, err = UnmarshalECDSASignature(sigma)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature, S must be larger than zero")

	sigma, err = MarshalECDSASignature(big.NewInt(1), big.NewInt(-1))
	require.NoError(t, err)
	_, _, err = UnmarshalECDSASignature(sigma)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid signature, S must be larger than zero")

	sigma, err = MarshalECDSASignature(big.NewInt(1), big.NewInt(1))
	require.NoError(t, err)
	R, S, err := UnmarshalECDSASignature(sigma)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), R)
	require.Equal(t, big.NewInt(1), S)
}

func TestIsLowS(t *testing.T) {
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	lowS, err := IsLowS(&lowLevelKey.PublicKey, big.NewInt(0))
	require.NoError(t, err)
	require.True(t, lowS)

	s := new(big.Int)
	s = s.Set(GetCurveHalfOrdersAt(elliptic.P256()))

	lowS, err = IsLowS(&lowLevelKey.PublicKey, s)
	require.NoError(t, err)
	require.True(t, lowS)

	s = s.Add(s, big.NewInt(1))
	lowS, err = IsLowS(&lowLevelKey.PublicKey, s)
	require.NoError(t, err)
	require.False(t, lowS)
	s, err = ToLowS(&lowLevelKey.PublicKey, s)
	require.NoError(t, err)
	lowS, err = IsLowS(&lowLevelKey.PublicKey, s)
	require.NoError(t, err)
	require.True(t, lowS)
}

func TestSignatureToLowS(t *testing.T) {
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	s := new(big.Int)
	s = s.Set(GetCurveHalfOrdersAt(elliptic.P256()))
	s = s.Add(s, big.NewInt(1))

	lowS, err := IsLowS(&lowLevelKey.PublicKey, s)
	require.NoError(t, err)
	require.False(t, lowS)
	sigma, err := MarshalECDSASignature(big.NewInt(1), s)
	require.NoError(t, err)
	sigma2, err := SignatureToLowS(&lowLevelKey.PublicKey, sigma)
	require.NoError(t, err)
	_, s, err = UnmarshalECDSASignature(sigma2)
	require.NoError(t, err)
	lowS, err = IsLowS(&lowLevelKey.PublicKey, s)
	require.NoError(t, err)
	require.True(t, lowS)
}
