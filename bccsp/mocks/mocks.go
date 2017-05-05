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

package mocks

import (
	"crypto"
	"errors"
	"hash"
	"reflect"

	"github.com/hyperledger/fabric/bccsp"
)

type MockBCCSP struct {
	SignArgKey    bccsp.Key
	SignDigestArg []byte
	SignOptsArg   bccsp.SignerOpts

	SignValue []byte
	SignErr   error

	VerifyValue bool
	VerifyErr   error
}

func (*MockBCCSP) KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error) {
	panic("Not yet implemented")
}

func (*MockBCCSP) KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error) {
	panic("Not yet implemented")
}

func (*MockBCCSP) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	panic("Not yet implemented")
}

func (*MockBCCSP) GetKey(ski []byte) (k bccsp.Key, err error) {
	panic("Not yet implemented")
}

func (*MockBCCSP) Hash(msg []byte, opts bccsp.HashOpts) (hash []byte, err error) {
	panic("Not yet implemented")
}

func (*MockBCCSP) GetHash(opts bccsp.HashOpts) (h hash.Hash, err error) {
	panic("Not yet implemented")
}

func (b *MockBCCSP) Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error) {
	if !reflect.DeepEqual(b.SignArgKey, k) {
		return nil, errors.New("invalid key")
	}
	if !reflect.DeepEqual(b.SignDigestArg, digest) {
		return nil, errors.New("invalid digest")
	}
	if !reflect.DeepEqual(b.SignOptsArg, opts) {
		return nil, errors.New("invalid opts")
	}

	return b.SignValue, b.SignErr
}

func (b *MockBCCSP) Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error) {
	return b.VerifyValue, b.VerifyErr
}

func (*MockBCCSP) Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error) {
	panic("Not yet implemented")
}

func (*MockBCCSP) Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error) {
	panic("Not yet implemented")
}

type MockKey struct {
	BytesValue []byte
	BytesErr   error
	Symm       bool
	PK         bccsp.Key
	PKErr      error
}

func (m *MockKey) Bytes() ([]byte, error) {
	return m.BytesValue, m.BytesErr
}

func (*MockKey) SKI() []byte {
	panic("Not yet implemented")
}

func (m *MockKey) Symmetric() bool {
	return m.Symm
}

func (*MockKey) Private() bool {
	panic("Not yet implemented")
}

func (m *MockKey) PublicKey() (bccsp.Key, error) {
	return m.PK, m.PKErr
}

type SignerOpts struct {
	HashFuncValue crypto.Hash
}

func (o *SignerOpts) HashFunc() crypto.Hash {
	return o.HashFuncValue
}

type KeyGenOpts struct {
	EphemeralValue bool
}

func (*KeyGenOpts) Algorithm() string {
	return "Mock KeyGenOpts"
}

func (o *KeyGenOpts) Ephemeral() bool {
	return o.EphemeralValue
}

type KeyStore struct {
	GetKeyValue bccsp.Key
	GetKeyErr   error
	StoreKeyErr error
}

func (*KeyStore) ReadOnly() bool {
	panic("Not yet implemented")
}

func (ks *KeyStore) GetKey(ski []byte) (k bccsp.Key, err error) {
	return ks.GetKeyValue, ks.GetKeyErr
}

func (ks *KeyStore) StoreKey(k bccsp.Key) (err error) {
	return ks.StoreKeyErr
}

type KeyImportOpts struct{}

func (*KeyImportOpts) Algorithm() string {
	return "Mock KeyImportOpts"
}

func (*KeyImportOpts) Ephemeral() bool {
	panic("Not yet implemented")
}

type EncrypterOpts struct{}

type HashOpts struct{}

func (HashOpts) Algorithm() string {
	return "Mock HashOpts"
}

type KeyDerivOpts struct {
	EphemeralValue bool
}

func (*KeyDerivOpts) Algorithm() string {
	return "Mock KeyDerivOpts"
}

func (o *KeyDerivOpts) Ephemeral() bool {
	return o.EphemeralValue
}
