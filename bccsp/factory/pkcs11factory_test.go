// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/stretchr/testify/assert"
)

func TestPKCS11FactoryName(t *testing.T) {
	f := &PKCS11Factory{}
	assert.Equal(t, f.Name(), PKCS11BasedFactoryName)
}

func TestPKCS11FactoryGetInvalidArgs(t *testing.T) {
	f := &PKCS11Factory{}

	_, err := f.Get(nil)
	assert.Error(t, err, "Invalid config. It must not be nil.")

	_, err = f.Get(&FactoryOpts{})
	assert.Error(t, err, "Invalid config. It must not be nil.")

	opts := &FactoryOpts{
		Pkcs11Opts: &pkcs11.PKCS11Opts{},
	}
	_, err = f.Get(opts)
	assert.Error(t, err, "CSP:500 - Failed initializing configuration at [0,]")
}

func TestPKCS11FactoryGet(t *testing.T) {
	f := &PKCS11Factory{}
	lib, pin, label := pkcs11.FindPKCS11Lib()

	opts := &FactoryOpts{
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			SecLevel:   256,
			HashFamily: "SHA2",
			Library:    lib,
			Pin:        pin,
			Label:      label,
		},
	}
	csp, err := f.Get(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	opts = &FactoryOpts{
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			SecLevel:   256,
			HashFamily: "SHA2",
			Library:    lib,
			Pin:        pin,
			Label:      label,
		},
	}
	csp, err = f.Get(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	opts = &FactoryOpts{
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			SecLevel:   256,
			HashFamily: "SHA2",
			Ephemeral:  true,
			Library:    lib,
			Pin:        pin,
			Label:      label,
		},
	}
	csp, err = f.Get(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)
}

func TestPKCS11FactoryGetEmptyKeyStorePath(t *testing.T) {
	f := &PKCS11Factory{}
	lib, pin, label := pkcs11.FindPKCS11Lib()

	opts := &FactoryOpts{
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			SecLevel:   256,
			HashFamily: "SHA2",
			Library:    lib,
			Pin:        pin,
			Label:      label,
		},
	}
	csp, err := f.Get(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)

	opts = &FactoryOpts{
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			SecLevel:   256,
			HashFamily: "SHA2",
			Library:    lib,
			Pin:        pin,
			Label:      label,
		},
	}
	csp, err = f.Get(opts)
	assert.NoError(t, err)
	assert.NotNil(t, csp)
}
