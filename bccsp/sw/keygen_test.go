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
	"github.com/stretchr/testify/require"
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
	require.Nil(t, value)
	require.Contains(t, err.Error(), expectedErr.Error())

	keyGenerators = make(map[reflect.Type]KeyGenerator)
	keyGenerators[reflect.TypeOf(&mocks2.KeyGenOpts{})] = &mocks.KeyGenerator{
		OptsArg: expectedOpts,
		Value:   expectetValue,
		Err:     nil,
	}
	csp = CSP{KeyGenerators: keyGenerators}
	value, err = csp.KeyGen(expectedOpts)
	require.Equal(t, expectetValue, value)
	require.Nil(t, err)
}

func TestECDSAKeyGenerator(t *testing.T) {
	t.Parallel()

	kg := &ecdsaKeyGenerator{curve: elliptic.P256()}

	k, err := kg.KeyGen(nil)
	require.NoError(t, err)

	ecdsaK, ok := k.(*ecdsaPrivateKey)
	require.True(t, ok)
	require.NotNil(t, ecdsaK.privKey)
	require.Equal(t, ecdsaK.privKey.Curve, elliptic.P256())
}

func TestAESKeyGenerator(t *testing.T) {
	t.Parallel()

	kg := &aesKeyGenerator{length: 32}

	k, err := kg.KeyGen(nil)
	require.NoError(t, err)

	aesK, ok := k.(*aesPrivateKey)
	require.True(t, ok)
	require.NotNil(t, aesK.privKey)
	require.Equal(t, len(aesK.privKey), 32)
}

func TestAESKeyGeneratorInvalidInputs(t *testing.T) {
	t.Parallel()

	kg := &aesKeyGenerator{length: -1}

	_, err := kg.KeyGen(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Len must be larger than 0")
}
