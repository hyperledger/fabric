// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"testing"

	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/stretchr/testify/require"
)

func TestPKCS11FactoryName(t *testing.T) {
	f := &PKCS11Factory{}
	require.Equal(t, f.Name(), PKCS11BasedFactoryName)
}

func TestPKCS11FactoryGetInvalidArgs(t *testing.T) {
	f := &PKCS11Factory{}

	_, err := f.Get(nil)
	require.Error(t, err, "Invalid config. It must not be nil.")

	_, err = f.Get(&FactoryOpts{})
	require.Error(t, err, "Invalid config. It must not be nil.")

	opts := &FactoryOpts{
		PKCS11: &pkcs11.PKCS11Opts{},
	}
	_, err = f.Get(opts)
	require.Error(t, err, "CSP:500 - Failed initializing configuration at [0,]")
}

func TestPKCS11FactoryGet(t *testing.T) {
	f := &PKCS11Factory{}
	lib, pin, label := pkcs11.FindPKCS11Lib()

	opts := &FactoryOpts{
		PKCS11: &pkcs11.PKCS11Opts{
			Security: 256,
			Hash:     "SHA2",
			Library:  lib,
			Pin:      pin,
			Label:    label,
		},
	}
	csp, err := f.Get(opts)
	require.NoError(t, err)
	require.NotNil(t, csp)

	opts = &FactoryOpts{
		PKCS11: &pkcs11.PKCS11Opts{
			Security: 256,
			Hash:     "SHA2",
			Library:  lib,
			Pin:      pin,
			Label:    label,
		},
	}
	csp, err = f.Get(opts)
	require.NoError(t, err)
	require.NotNil(t, csp)

	opts = &FactoryOpts{
		PKCS11: &pkcs11.PKCS11Opts{
			Security: 256,
			Hash:     "SHA2",
			Library:  lib,
			Pin:      pin,
			Label:    label,
		},
	}
	csp, err = f.Get(opts)
	require.NoError(t, err)
	require.NotNil(t, csp)
}

func TestPKCS11FactoryGetEmptyKeyStorePath(t *testing.T) {
	f := &PKCS11Factory{}
	lib, pin, label := pkcs11.FindPKCS11Lib()

	opts := &FactoryOpts{
		PKCS11: &pkcs11.PKCS11Opts{
			Security: 256,
			Hash:     "SHA2",
			Library:  lib,
			Pin:      pin,
			Label:    label,
		},
	}
	csp, err := f.Get(opts)
	require.NoError(t, err)
	require.NotNil(t, csp)

	opts = &FactoryOpts{
		PKCS11: &pkcs11.PKCS11Opts{
			Security: 256,
			Hash:     "SHA2",
			Library:  lib,
			Pin:      pin,
			Label:    label,
		},
	}
	csp, err = f.Get(opts)
	require.NoError(t, err)
	require.NotNil(t, csp)
}
