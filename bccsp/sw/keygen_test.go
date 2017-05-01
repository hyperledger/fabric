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
	"crypto/elliptic"
	"errors"
	"reflect"
	"testing"

	mocks2 "github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/hyperledger/fabric/bccsp/sw/mocks"
	"github.com/stretchr/testify/assert"
)

func TestKeyGen(t *testing.T) {
	expectedOpts := &mocks2.KeyGenOpts{EphemeralValue: true}
	expectetValue := &mocks2.MockKey{}
	expectedErr := errors.New("Expected Error")

	keyGenerators := make(map[reflect.Type]KeyGenerator)
	keyGenerators[reflect.TypeOf(&mocks2.KeyGenOpts{})] = &mocks.KeyGenerator{
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     expectedErr,
	}
	csp := impl{keyGenerators: keyGenerators}
	value, err := csp.KeyGen(expectedOpts)
	assert.Nil(t, value)
	assert.Contains(t, err.Error(), expectedErr.Error())

	keyGenerators = make(map[reflect.Type]KeyGenerator)
	keyGenerators[reflect.TypeOf(&mocks2.KeyGenOpts{})] = &mocks.KeyGenerator{
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     nil,
	}
	csp = impl{keyGenerators: keyGenerators}
	value, err = csp.KeyGen(expectedOpts)
	assert.Equal(t, expectetValue, value)
	assert.Nil(t, err)
}

func TestECDSAKeyGenerator(t *testing.T) {
	kg := &ecdsaKeyGenerator{curve: elliptic.P256()}

	k, err := kg.KeyGen(nil)
	assert.NoError(t, err)

	ecdsaK, ok := k.(*ecdsaPrivateKey)
	assert.True(t, ok)
	assert.NotNil(t, ecdsaK.privKey)
	assert.Equal(t, ecdsaK.privKey.Curve, elliptic.P256())
}

func TestRSAKeyGenerator(t *testing.T) {
	kg := &rsaKeyGenerator{length: 512}

	k, err := kg.KeyGen(nil)
	assert.NoError(t, err)

	rsaK, ok := k.(*rsaPrivateKey)
	assert.True(t, ok)
	assert.NotNil(t, rsaK.privKey)
	assert.Equal(t, rsaK.privKey.N.BitLen(), 512)
}

func TestAESKeyGenerator(t *testing.T) {
	kg := &aesKeyGenerator{length: 32}

	k, err := kg.KeyGen(nil)
	assert.NoError(t, err)

	aesK, ok := k.(*aesPrivateKey)
	assert.True(t, ok)
	assert.NotNil(t, aesK.privKey)
	assert.Equal(t, len(aesK.privKey), 32)
}

func TestAESKeyGeneratorInvalidInputs(t *testing.T) {
	kg := &aesKeyGenerator{length: -1}

	_, err := kg.KeyGen(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Len must be larger than 0")
}

func TestRSAKeyGeneratorInvalidInputs(t *testing.T) {
	kg := &rsaKeyGenerator{length: -1}

	_, err := kg.KeyGen(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed generating RSA -1 key")
}
