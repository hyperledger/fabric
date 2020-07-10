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
	"errors"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/mocks"
	mocks2 "github.com/hyperledger/fabric/bccsp/sw/mocks"
	"github.com/stretchr/testify/require"
)

func TestKeyGenInvalidInputs(t *testing.T) {
	// Init a BCCSP instance with a key store that returns an error on store
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{StoreKeyErr: errors.New("cannot store key")})
	require.NoError(t, err)

	_, err = csp.KeyGen(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Opts parameter. It must not be nil.")

	_, err = csp.KeyGen(&mocks.KeyGenOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'KeyGenOpts' provided [")

	_, err = csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{})
	require.Error(t, err, "Generation of a non-ephemeral key must fail. KeyStore is programmed to fail.")
	require.Contains(t, err.Error(), "cannot store key")
}

func TestKeyDerivInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{StoreKeyErr: errors.New("cannot store key")})
	require.NoError(t, err)

	_, err = csp.KeyDeriv(nil, &bccsp.ECDSAReRandKeyOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil.")

	_, err = csp.KeyDeriv(&mocks.MockKey{}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid opts. It must not be nil.")

	_, err = csp.KeyDeriv(&mocks.MockKey{}, &bccsp.ECDSAReRandKeyOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'Key' provided [")

	keyDerivers := make(map[reflect.Type]KeyDeriver)
	keyDerivers[reflect.TypeOf(&mocks.MockKey{})] = &mocks2.KeyDeriver{
		KeyArg:  &mocks.MockKey{},
		OptsArg: &mocks.KeyDerivOpts{EphemeralValue: false},
		Value:   nil,
		Err:     nil,
	}
	csp.(*CSP).KeyDerivers = keyDerivers
	_, err = csp.KeyDeriv(&mocks.MockKey{}, &mocks.KeyDerivOpts{EphemeralValue: false})
	require.Error(t, err, "KeyDerivation of a non-ephemeral key must fail. KeyStore is programmed to fail.")
	require.Contains(t, err.Error(), "cannot store key")
}

func TestKeyImportInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.KeyImport(nil, &bccsp.AES256ImportKeyOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid raw. It must not be nil.")

	_, err = csp.KeyImport([]byte{0, 1, 2, 3, 4}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid opts. It must not be nil.")

	_, err = csp.KeyImport([]byte{0, 1, 2, 3, 4}, &mocks.KeyImportOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'KeyImportOpts' provided [")
}

func TestGetKeyInvalidInputs(t *testing.T) {
	// Init a BCCSP instance with a key store that returns an error on get
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{GetKeyErr: errors.New("cannot get key")})
	require.NoError(t, err)

	_, err = csp.GetKey(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot get key")

	// Init a BCCSP instance with a key store that returns a given key
	k := &mocks.MockKey{}
	csp, err = NewWithParams(256, "SHA2", &mocks.KeyStore{GetKeyValue: k})
	require.NoError(t, err)
	// No SKI is needed here
	k2, err := csp.GetKey(nil)
	require.NoError(t, err)
	require.Equal(t, k, k2, "Keys must be the same.")
}

func TestSignInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.Sign(nil, []byte{1, 2, 3, 5}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil.")

	_, err = csp.Sign(&mocks.MockKey{}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid digest. Cannot be empty.")

	_, err = csp.Sign(&mocks.MockKey{}, []byte{1, 2, 3, 5}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'SignKey' provided [")
}

func TestVerifyInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.Verify(nil, []byte{1, 2, 3, 5}, []byte{1, 2, 3, 5}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil.")

	_, err = csp.Verify(&mocks.MockKey{}, nil, []byte{1, 2, 3, 5}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid signature. Cannot be empty.")

	_, err = csp.Verify(&mocks.MockKey{}, []byte{1, 2, 3, 5}, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid digest. Cannot be empty.")

	_, err = csp.Verify(&mocks.MockKey{}, []byte{1, 2, 3, 5}, []byte{1, 2, 3, 5}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'VerifyKey' provided [")
}

func TestEncryptInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.Encrypt(nil, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil.")

	_, err = csp.Encrypt(&mocks.MockKey{}, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'EncryptKey' provided [")
}

func TestDecryptInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.Decrypt(nil, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil.")

	_, err = csp.Decrypt(&mocks.MockKey{}, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'DecryptKey' provided [")
}

func TestHashInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.Hash(nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid opts. It must not be nil.")

	_, err = csp.Hash(nil, &mocks.HashOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'HashOpt' provided [")
}

func TestGetHashInvalidInputs(t *testing.T) {
	csp, err := NewWithParams(256, "SHA2", &mocks.KeyStore{})
	require.NoError(t, err)

	_, err = csp.GetHash(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid opts. It must not be nil.")

	_, err = csp.GetHash(&mocks.HashOpts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported 'HashOpt' provided [")
}
