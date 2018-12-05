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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"reflect"
	"testing"

	mocks2 "github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/hyperledger/fabric/bccsp/sw/mocks"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/stretchr/testify/assert"
)

func TestKeyImport(t *testing.T) {
	t.Parallel()

	expectedRaw := []byte{1, 2, 3}
	expectedOpts := &mocks2.KeyDerivOpts{EphemeralValue: true}
	expectetValue := &mocks2.MockKey{BytesValue: []byte{1, 2, 3, 4, 5}}
	expectedErr := errors.New("Expected Error")

	keyImporters := make(map[reflect.Type]KeyImporter)
	keyImporters[reflect.TypeOf(&mocks2.KeyDerivOpts{})] = &mocks.KeyImporter{
		RawArg:  expectedRaw,
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     expectedErr,
	}
	csp := CSP{KeyImporters: keyImporters}
	value, err := csp.KeyImport(expectedRaw, expectedOpts)
	assert.Nil(t, value)
	assert.Contains(t, err.Error(), expectedErr.Error())

	keyImporters = make(map[reflect.Type]KeyImporter)
	keyImporters[reflect.TypeOf(&mocks2.KeyDerivOpts{})] = &mocks.KeyImporter{
		RawArg:  expectedRaw,
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     nil,
	}
	csp = CSP{KeyImporters: keyImporters}
	value, err = csp.KeyImport(expectedRaw, expectedOpts)
	assert.Equal(t, expectetValue, value)
	assert.Nil(t, err)
}

func TestAES256ImportKeyOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := aes256ImportKeyOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport([]byte(nil), &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. It must not be nil.")

	_, err = ki.KeyImport([]byte{0}, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Key Length [")
}

func TestHMACImportKeyOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := hmacImportKeyOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport([]byte(nil), &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. It must not be nil.")
}

func TestECDSAPKIXPublicKeyImportOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := ecdsaPKIXPublicKeyImportOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport([]byte(nil), &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw. It must not be nil.")

	_, err = ki.KeyImport([]byte{0}, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed converting PKIX to ECDSA public key [")

	k, err := rsa.GenerateKey(rand.Reader, 512)
	assert.NoError(t, err)
	raw, err := utils.PublicKeyToDER(&k.PublicKey)
	assert.NoError(t, err)
	_, err = ki.KeyImport(raw, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed casting to ECDSA public key. Invalid raw material.")
}

func TestECDSAPrivateKeyImportOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := ecdsaPrivateKeyImportOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected byte array.")

	_, err = ki.KeyImport([]byte(nil), &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw. It must not be nil.")

	_, err = ki.KeyImport([]byte{0}, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed converting PKIX to ECDSA public key")

	k, err := rsa.GenerateKey(rand.Reader, 512)
	assert.NoError(t, err)
	raw := x509.MarshalPKCS1PrivateKey(k)
	_, err = ki.KeyImport(raw, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed casting to ECDSA private key. Invalid raw material.")
}

func TestECDSAGoPublicKeyImportOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := ecdsaGoPublicKeyImportOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected *ecdsa.PublicKey.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected *ecdsa.PublicKey.")
}

func TestRSAGoPublicKeyImportOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := rsaGoPublicKeyImportOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected *rsa.PublicKey.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected *rsa.PublicKey.")
}

func TestX509PublicKeyImportOptsKeyImporter(t *testing.T) {
	t.Parallel()

	ki := x509PublicKeyImportOptsKeyImporter{}

	_, err := ki.KeyImport("Hello World", &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected *x509.Certificate.")

	_, err = ki.KeyImport(nil, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid raw material. Expected *x509.Certificate.")

	cert := &x509.Certificate{}
	cert.PublicKey = "Hello world"
	_, err = ki.KeyImport(cert, &mocks2.KeyImportOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Certificate's public key type not recognized. Supported keys: [ECDSA, RSA]")
}
