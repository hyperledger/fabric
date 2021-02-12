/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sw

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

type rsaPublicKeyASN struct {
	N *big.Int
	E int
}

func TestRSAPublicKey(t *testing.T) {
	lowLevelKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	k := &rsaPublicKey{&lowLevelKey.PublicKey}

	require.False(t, k.Symmetric())
	require.False(t, k.Private())

	k.pubKey = nil
	ski := k.SKI()
	require.Nil(t, ski)

	k.pubKey = &lowLevelKey.PublicKey
	ski = k.SKI()
	raw, err := asn1.Marshal(rsaPublicKeyASN{N: k.pubKey.N, E: k.pubKey.E})
	require.NoError(t, err, "asn1 marshal failed")
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

	_, err = (&rsaPublicKey{}).Bytes()
	require.EqualError(t, err, "Failed marshalling key. Key is nil.")
}
