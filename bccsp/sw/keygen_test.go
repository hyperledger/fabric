/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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
	t.Parallel()

	expectedOpts := &mocks2.KeyGenOpts{EphemeralValue: true}
	expectetValue := &mocks2.MockKey{}
	expectedErr := errors.New("Expected Error")

	keyGenerators := make(map[reflect.Type]KeyGenerator)
	keyGenerators[reflect.TypeOf(&mocks2.KeyGenOpts{})] = &mocks.KeyGenerator{
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     expectedErr,
	}
	csp := CSP{KeyGenerators: keyGenerators}
	value, err := csp.KeyGen(expectedOpts)
	assert.Nil(t, value)
	assert.Contains(t, err.Error(), expectedErr.Error())

	keyGenerators = make(map[reflect.Type]KeyGenerator)
	keyGenerators[reflect.TypeOf(&mocks2.KeyGenOpts{})] = &mocks.KeyGenerator{
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     nil,
	}
	csp = CSP{KeyGenerators: keyGenerators}
	value, err = csp.KeyGen(expectedOpts)
	assert.Equal(t, expectetValue, value)
	assert.Nil(t, err)
}

func TestECDSAKeyGenerator(t *testing.T) {
	t.Parallel()

	kg := &ecdsaKeyGenerator{curve: elliptic.P256()}

	k, err := kg.KeyGen(nil)
	assert.NoError(t, err)

	ecdsaK, ok := k.(*ecdsaPrivateKey)
	assert.True(t, ok)
	assert.NotNil(t, ecdsaK.privKey)
	assert.Equal(t, ecdsaK.privKey.Curve, elliptic.P256())
}

func TestAESKeyGenerator(t *testing.T) {
	t.Parallel()

	kg := &aesKeyGenerator{length: 32}

	k, err := kg.KeyGen(nil)
	assert.NoError(t, err)

	aesK, ok := k.(*aesPrivateKey)
	assert.True(t, ok)
	assert.NotNil(t, aesK.privKey)
	assert.Equal(t, len(aesK.privKey), 32)
}

func TestAESKeyGeneratorInvalidInputs(t *testing.T) {
	t.Parallel()

	kg := &aesKeyGenerator{length: -1}

	_, err := kg.KeyGen(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Len must be larger than 0")
}
