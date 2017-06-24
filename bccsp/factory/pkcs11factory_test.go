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
			SecLevel:     256,
			HashFamily:   "SHA2",
			FileKeystore: &pkcs11.FileKeystoreOpts{KeyStorePath: os.TempDir()},
			Library:      lib,
			Pin:          pin,
			Label:        label,
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
