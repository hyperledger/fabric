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
	"strings"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/stretchr/testify/assert"
)

func TestKeyGenInvalidInputs(t *testing.T) {
	// Init a BCCSP instance with a key store that returns an error on store
	csp, err := New(256, "SHA2", &mocks.KeyStore{StoreKeyErr: errors.New("cannot store key")})
	assert.NoError(t, err)

	_, err = csp.KeyGen(nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Opts parameter. It must not be nil."))

	_, err = csp.KeyGen(&mocks.KeyGenOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'KeyGenOpts' provided ["))

	_, err = csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{})
	assert.Error(t, err, "Generation of a non-ephemeral key must fail. KeyStore is programmed to fail.")
	assert.True(t, strings.Contains(err.Error(), "cannot store key"), "Failure must be due to the KeyStore")
}

func TestKeyDerivInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.KeyDeriv(nil, &bccsp.ECDSAReRandKeyOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Key. It must not be nil."))

	_, err = csp.KeyDeriv(&mocks.MockKey{}, &bccsp.ECDSAReRandKeyOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Key type not recognized ["))
}

func TestKeyImportInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.KeyImport(nil, &bccsp.AES256ImportKeyOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid raw. Cannot be nil"))

	_, err = csp.KeyImport([]byte{0, 1, 2, 3, 4}, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Opts parameter. It must not be nil."))

	_, err = csp.KeyImport([]byte{0, 1, 2, 3, 4}, &mocks.KeyImportOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'KeyImportOptions' provided ["))
}

func TestGetKeyInvalidInputs(t *testing.T) {
	// Init a BCCSP instance with a key store that returns an error on get
	csp, err := New(256, "SHA2", &mocks.KeyStore{GetKeyErr: errors.New("cannot get key")})
	assert.NoError(t, err)

	_, err = csp.GetKey(nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "cannot get key"))

	// Init a BCCSP instance with a key store that returns a given key
	k := &mocks.MockKey{}
	csp, err = New(256, "SHA2", &mocks.KeyStore{GetKeyValue: k})
	assert.NoError(t, err)
	// No SKI is needed here
	k2, err := csp.GetKey(nil)
	assert.NoError(t, err)
	assert.Equal(t, k, k2, "Keys must be the same.")
}

func TestSignInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.Sign(nil, []byte{1, 2, 3, 5}, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Key. It must not be nil."))

	_, err = csp.Sign(&mocks.MockKey{}, nil, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid digest. Cannot be empty."))

	_, err = csp.Sign(&mocks.MockKey{}, []byte{1, 2, 3, 5}, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'SignKey' provided ["))
}

func TestVerifyInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.Verify(nil, []byte{1, 2, 3, 5}, []byte{1, 2, 3, 5}, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Key. It must not be nil."))

	_, err = csp.Verify(&mocks.MockKey{}, nil, []byte{1, 2, 3, 5}, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid signature. Cannot be empty."))

	_, err = csp.Verify(&mocks.MockKey{}, []byte{1, 2, 3, 5}, nil, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid digest. Cannot be empty."))

	_, err = csp.Verify(&mocks.MockKey{}, []byte{1, 2, 3, 5}, []byte{1, 2, 3, 5}, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'VerifyKey' provided ["))
}

func TestEncryptInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.Encrypt(nil, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Key. It must not be nil."))

	_, err = csp.Encrypt(&mocks.MockKey{}, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'EncryptKey' provided ["))
}

func TestDecryptInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.Decrypt(nil, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid Key. It must not be nil."))

	_, err = csp.Decrypt(&mocks.MockKey{}, []byte{1, 2, 3, 4}, &bccsp.AESCBCPKCS7ModeOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'DecryptKey' provided ["))
}

func TestHashInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.Hash(nil, nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid opts. It must not be nil."))

	_, err = csp.Hash(nil, &mocks.HashOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'HashOpt' provided ["))
}

func TestGetHashInvalidInputs(t *testing.T) {
	csp, err := New(256, "SHA2", &mocks.KeyStore{})
	assert.NoError(t, err)

	_, err = csp.GetHash(nil)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Invalid opts. It must not be nil."))

	_, err = csp.GetHash(&mocks.HashOpts{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "Unsupported 'HashOpt' provided ["))
}
