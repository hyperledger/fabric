/*
Copyright IBM Corp. 2016 All Rights Reserved.

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
package signer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"testing"

	"github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/stretchr/testify/require"
)

func TestInitFailures(t *testing.T) {
	_, err := New(nil, &mocks.MockKey{})
	require.Error(t, err)

	_, err = New(&mocks.MockBCCSP{}, nil)
	require.Error(t, err)

	_, err = New(&mocks.MockBCCSP{}, &mocks.MockKey{Symm: true})
	require.Error(t, err)

	_, err = New(&mocks.MockBCCSP{}, &mocks.MockKey{PKErr: errors.New("No PK")})
	require.Error(t, err)
	require.Equal(t, "failed getting public key: No PK", err.Error())

	_, err = New(&mocks.MockBCCSP{}, &mocks.MockKey{PK: &mocks.MockKey{BytesErr: errors.New("No bytes")}})
	require.Error(t, err)
	require.Equal(t, "failed marshalling public key: No bytes", err.Error())

	_, err = New(&mocks.MockBCCSP{}, &mocks.MockKey{PK: &mocks.MockKey{BytesValue: []byte{0, 1, 2, 3}}})
	require.Error(t, err)
}

func TestInit(t *testing.T) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pkRaw, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	require.NoError(t, err)

	signer, err := New(&mocks.MockBCCSP{}, &mocks.MockKey{PK: &mocks.MockKey{BytesValue: pkRaw}})
	require.NoError(t, err)
	require.NotNil(t, signer)

	// Test public key
	R, S, err := ecdsa.Sign(rand.Reader, k, []byte{0, 1, 2, 3})
	require.NoError(t, err)

	require.True(t, ecdsa.Verify(signer.Public().(*ecdsa.PublicKey), []byte{0, 1, 2, 3}, R, S))
}

func TestPublic(t *testing.T) {
	pk := &mocks.MockKey{}
	signer := &bccspCryptoSigner{pk: pk}

	pk2 := signer.Public()
	require.NotNil(t, pk, pk2)
}

func TestSign(t *testing.T) {
	expectedSig := []byte{0, 1, 2, 3, 4}
	expectedKey := &mocks.MockKey{}
	expectedDigest := []byte{0, 1, 2, 3, 4, 5}
	expectedOpts := &mocks.SignerOpts{}

	signer := &bccspCryptoSigner{
		key: expectedKey,
		csp: &mocks.MockBCCSP{
			SignArgKey: expectedKey, SignDigestArg: expectedDigest, SignOptsArg: expectedOpts,
			SignValue: expectedSig,
		},
	}
	signature, err := signer.Sign(nil, expectedDigest, expectedOpts)
	require.NoError(t, err)
	require.Equal(t, expectedSig, signature)

	signer = &bccspCryptoSigner{
		key: expectedKey,
		csp: &mocks.MockBCCSP{
			SignArgKey: expectedKey, SignDigestArg: expectedDigest, SignOptsArg: expectedOpts,
			SignErr: errors.New("no signature"),
		},
	}
	_, err = signer.Sign(nil, expectedDigest, expectedOpts)
	require.Error(t, err)
	require.Equal(t, err.Error(), "no signature")

	signer = &bccspCryptoSigner{
		key: nil,
		csp: &mocks.MockBCCSP{SignArgKey: expectedKey, SignDigestArg: expectedDigest, SignOptsArg: expectedOpts},
	}
	_, err = signer.Sign(nil, expectedDigest, expectedOpts)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid key")

	signer = &bccspCryptoSigner{
		key: expectedKey,
		csp: &mocks.MockBCCSP{SignArgKey: expectedKey, SignDigestArg: expectedDigest, SignOptsArg: expectedOpts},
	}
	_, err = signer.Sign(nil, nil, expectedOpts)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid digest")

	signer = &bccspCryptoSigner{
		key: expectedKey,
		csp: &mocks.MockBCCSP{SignArgKey: expectedKey, SignDigestArg: expectedDigest, SignOptsArg: expectedOpts},
	}
	_, err = signer.Sign(nil, expectedDigest, nil)
	require.Error(t, err)
	require.Equal(t, err.Error(), "invalid opts")
}
