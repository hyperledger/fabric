/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"testing"

	"reflect"
	"runtime"

	"github.com/stretchr/testify/assert"
)

func TestNewInvalidOpts(t *testing.T) {
	i, err := New(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid msp.NewOpts instance. It must be either *BCCSPNewOpts or *IdemixNewOpts. It was [<nil>]")
	assert.Nil(t, i)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: -1}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid *BCCSPNewOpts. Version not recognized [-1]")
	assert.Nil(t, i)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: -1}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [-1]")
	assert.Nil(t, i)
}

func TestNew(t *testing.T) {
	i, err := New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_0}})
	assert.NoError(t, err)
	assert.NotNil(t, i)
	assert.Equal(t, MSPVersion(MSPv1_0), i.(*bccspmsp).version)
	assert.Equal(t, runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(), "github.com/hyperledger/fabric/msp.(*bccspmsp).(github.com/hyperledger/fabric/msp.setupV1)-fm")
	assert.Equal(t, runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(), "github.com/hyperledger/fabric/msp.(*bccspmsp).(github.com/hyperledger/fabric/msp.validateIdentityOUsV1)-fm")

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_1}})
	assert.NoError(t, err)
	assert.NotNil(t, i)
	assert.Equal(t, MSPVersion(MSPv1_1), i.(*bccspmsp).version)
	assert.Equal(t, runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(), "github.com/hyperledger/fabric/msp.(*bccspmsp).(github.com/hyperledger/fabric/msp.setupV11)-fm")
	assert.Equal(t, runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(), "github.com/hyperledger/fabric/msp.(*bccspmsp).(github.com/hyperledger/fabric/msp.validateIdentityOUsV11)-fm")

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_0}})
	assert.Error(t, err)
	assert.Nil(t, i)
	assert.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [0]")

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_1}})
	assert.NoError(t, err)
	assert.NotNil(t, i)
}
