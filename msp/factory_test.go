/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/stretchr/testify/assert"
)

func TestNewInvalidOpts(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	i, err := New(nil, cryptoProvider)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid msp.NewOpts instance. It must be either *BCCSPNewOpts or *IdemixNewOpts. It was [<nil>]")
	assert.Nil(t, i)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: -1}}, cryptoProvider)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid *BCCSPNewOpts. Version not recognized [-1]")
	assert.Nil(t, i)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: -1}}, cryptoProvider)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [-1]")
	assert.Nil(t, i)
}

func TestNew(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	i, err := New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_0}}, cryptoProvider)
	assert.NoError(t, err)
	assert.NotNil(t, i)
	assert.Equal(t, MSPVersion(MSPv1_0), i.(*bccspmsp).version)
	assert.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).setupV1).Pointer()).Name(),
	)
	assert.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).validateIdentityOUsV1).Pointer()).Name(),
	)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_1}}, cryptoProvider)
	assert.NoError(t, err)
	assert.NotNil(t, i)
	assert.Equal(t, MSPVersion(MSPv1_1), i.(*bccspmsp).version)
	assert.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).setupV11).Pointer()).Name(),
	)
	assert.Equal(t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).validateIdentityOUsV11).Pointer()).Name(),
	)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_0}}, cryptoProvider)
	assert.Error(t, err)
	assert.Nil(t, i)
	assert.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [0]")

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_1}}, cryptoProvider)
	assert.NoError(t, err)
	assert.NotNil(t, i)
}
