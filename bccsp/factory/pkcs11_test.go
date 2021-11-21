//go:build pkcs11
// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/stretchr/testify/require"
)

func TestExportedInitFactories(t *testing.T) {
	// Reset errors from previous negative test runs
	factoriesInitError = initFactories(nil)

	err := InitFactories(nil)
	require.NoError(t, err)
}

func TestInitFactories(t *testing.T) {
	err := initFactories(nil)
	require.NoError(t, err)

	err = initFactories(&FactoryOpts{})
	require.NoError(t, err)
}

func TestInitFactoriesInvalidArgs(t *testing.T) {
	err := initFactories(&FactoryOpts{
		Default: "SW",
		SW:      &SwOpts{},
	})
	require.EqualError(t, err, "Failed initializing SW.BCCSP: Could not initialize BCCSP SW [Failed initializing configuration at [0,]: Hash Family not supported []]")

	err = initFactories(&FactoryOpts{
		Default: "PKCS11",
		PKCS11:  &pkcs11.PKCS11Opts{},
	})
	require.EqualError(t, err, "Failed initializing PKCS11.BCCSP: Could not initialize BCCSP PKCS11 [Failed initializing configuration: Security level not supported [0]]")
}

func TestGetBCCSPFromOpts(t *testing.T) {
	opts := GetDefaultOpts()
	opts.SW.FileKeystore = &FileKeystoreOpts{KeyStorePath: os.TempDir()}
	csp, err := GetBCCSPFromOpts(opts)
	require.NoError(t, err)
	require.NotNil(t, csp)

	lib, pin, label := pkcs11.FindPKCS11Lib()
	csp, err = GetBCCSPFromOpts(&FactoryOpts{
		Default: "PKCS11",
		PKCS11: &pkcs11.PKCS11Opts{
			Security: 256,
			Hash:     "SHA2",
			Library:  lib,
			Pin:      pin,
			Label:    label,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, csp)

	csp, err = GetBCCSPFromOpts(&FactoryOpts{
		Default: "BadName",
	})
	require.EqualError(t, err, "Could not find BCCSP, no 'BadName' provider")
	require.Nil(t, csp)
}
