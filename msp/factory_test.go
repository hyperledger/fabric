/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/stretchr/testify/require"
)

func TestNewInvalidOpts(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	i, err := New(nil, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid msp.NewOpts instance. It must be either *BCCSPNewOpts or *IdemixNewOpts. It was [<nil>]")
	require.Nil(t, i)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: -1}}, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid *BCCSPNewOpts. Version not recognized [-1]")
	require.Nil(t, i)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: -1}}, cryptoProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [-1]")
	require.Nil(t, i)
}

func TestNew(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	i, err := New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_0}}, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, i)
	require.Equal(t, MSPVersion(MSPv1_0), i.(*bccspmsp).version)
	require.Equal(
		t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).setupV1).Pointer()).Name(),
	)
	require.Equal(
		t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).validateIdentityOUsV1).Pointer()).Name(),
	)

	i, err = New(&BCCSPNewOpts{NewBaseOpts{Version: MSPv1_1}}, cryptoProvider)
	require.NoError(t, err)
	require.NotNil(t, i)
	require.Equal(t, MSPVersion(MSPv1_1), i.(*bccspmsp).version)
	require.Equal(
		t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalSetupFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).setupV11).Pointer()).Name(),
	)
	require.Equal(
		t,
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).internalValidateIdentityOusFunc).Pointer()).Name(),
		runtime.FuncForPC(reflect.ValueOf(i.(*bccspmsp).validateIdentityOUsV11).Pointer()).Name(),
	)

	i, err = New(&IdemixNewOpts{NewBaseOpts{Version: MSPv1_0}}, cryptoProvider)
	require.Error(t, err)
	require.Nil(t, i)
	require.Contains(t, err.Error(), "Invalid *IdemixNewOpts. Version not recognized [0]")

	for _, tc := range []struct {
		name    string
		version MSPVersion
		idemix  MSPVersion
	}{
		{"v1_1", MSPv1_1, MSPv1_1},
		{"v1_3", MSPv1_3, MSPv1_3},
		{"v1_4_3", MSPv1_4_3, MSPv1_3},
		{"v3_0", MSPv3_0, MSPv1_3},
	} {
		t.Run("Idemix/"+tc.name, func(t *testing.T) {
			i, err := New(&IdemixNewOpts{NewBaseOpts{Version: tc.version}}, cryptoProvider)
			require.NoError(t, err)
			require.NotNil(t, i)
			require.Equal(t, IDEMIX, i.GetType())
			require.Equal(t, tc.idemix, i.GetVersion())
		})
	}
}
