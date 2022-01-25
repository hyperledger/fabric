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
	"github.com/stretchr/testify/assert"
)

func TestExportedInitFactories(t *testing.T) {
	// Reset errors from previous negative test runs
	factoriesInitError = initFactories(nil)

	err := InitFactories(nil)
	assert.NoError(t, err)
}

func TestInitFactories(t *testing.T) {
	err := initFactories(nil)
	assert.NoError(t, err)

	err = initFactories(&FactoryOpts{})
	assert.NoError(t, err)
}

func TestInitFactoriesInvalidArgs(t *testing.T) {
	err := initFactories(&FactoryOpts{
		ProviderName: "SW",
		SwOpts:       &SwOpts{},
	})
	assert.EqualError(t, err, "Failed initializing SW.BCCSP: Could not initialize BCCSP SW [Failed initializing configuration at [0,]: Hash Family not supported []]")

	err = initFactories(&FactoryOpts{
		ProviderName: "PKCS11",
		Pkcs11Opts:   &pkcs11.PKCS11Opts{},
	})
	assert.EqualError(t, err, "Failed initializing PKCS11.BCCSP: Could not initialize BCCSP PKCS11 [Failed initializing configuration: Hash Family not supported []]")
}

func TestGetBCCSPFromOpts(t *testing.T) {
	opts := GetDefaultOpts()
	opts.SwOpts.FileKeystore = &FileKeystoreOpts{KeyStorePath: os.TempDir()}
	csp, err := GetBCCSPFromOpts(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	lib, pin, label := pkcs11.FindPKCS11Lib()
	csp, err = GetBCCSPFromOpts(&FactoryOpts{
		ProviderName: "PKCS11",
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			SecLevel:   256,
			HashFamily: "SHA2",
			Ephemeral:  true,
			Library:    lib,
			Pin:        pin,
			Label:      label,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	csp, err = GetBCCSPFromOpts(&FactoryOpts{
		ProviderName: "BadName",
	})
	assert.EqualError(t, err, "Could not find BCCSP, no 'BadName' provider")
	assert.Nil(t, csp)
}
