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
package factory

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/stretchr/testify/assert"
)

func TestInitFactories(t *testing.T) {
	// Reset errors from previous negative test runs
	factoriesInitError = nil

	err := InitFactories(nil)
	assert.NoError(t, err)
}

func TestSetFactories(t *testing.T) {
	err := setFactories(nil)
	assert.NoError(t, err)

	err = setFactories(&FactoryOpts{})
	assert.NoError(t, err)
}

func TestSetFactoriesInvalidArgs(t *testing.T) {
	err := setFactories(&FactoryOpts{
		ProviderName: "SW",
		SwOpts:       &SwOpts{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed initializing SW.BCCSP")

	err = setFactories(&FactoryOpts{
		ProviderName: "PKCS11",
		Pkcs11Opts:   &pkcs11.PKCS11Opts{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed initializing PKCS11.BCCSP")
}

func TestGetBCCSPFromOpts(t *testing.T) {
	opts := GetDefaultOpts()
	opts.SwOpts.FileKeystore = &FileKeystoreOpts{KeyStorePath: os.TempDir()}
	opts.SwOpts.Ephemeral = false
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Could not find BCCSP, no 'BadName' provider")
	assert.Nil(t, csp)
}
