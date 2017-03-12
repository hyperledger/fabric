/*
Copyright IBM Corp. 2016 All Rights Reserved.

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
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/hyperledger/fabric/bccsp/sw"
)

const (
	// PKCS11BasedFactoryName is the name of the factory of the hsm-based BCCSP implementation
	PKCS11BasedFactoryName = "PKCS11"
)

// PKCS11Factory is the factory of the HSM-based BCCSP.
type PKCS11Factory struct{}

// Name returns the name of this factory
func (f *PKCS11Factory) Name() string {
	return PKCS11BasedFactoryName
}

// Get returns an instance of BCCSP using Opts.
func (f *PKCS11Factory) Get(config *FactoryOpts) (bccsp.BCCSP, error) {
	// Validate arguments
	if config == nil || config.SwOpts == nil {
		return nil, errors.New("Invalid config. It must not be nil.")
	}

	p11Opts := config.Pkcs11Opts

	//TODO: PKCS11 does not need a keystore, but we have not migrated all of PKCS11 BCCSP to PKCS11 yet
	var ks bccsp.KeyStore
	if p11Opts.Ephemeral == true {
		ks = sw.NewDummyKeyStore()
	} else if p11Opts.FileKeystore != nil {
		fks, err := sw.NewFileBasedKeyStore(nil, p11Opts.FileKeystore.KeyStorePath, false)
		if err != nil {
			return nil, fmt.Errorf("Failed to initialize software key store: %s", err)
		}
		ks = fks
	} else {
		// Default to DummyKeystore
		ks = sw.NewDummyKeyStore()
	}
	err := pkcs11.InitPKCS11(p11Opts.Library, p11Opts.Pin, p11Opts.Label)
	if err != nil {
		return nil, fmt.Errorf("Failed initializing PKCS11 library %s %s [%s]",
			p11Opts.Library, p11Opts.Label, err)
	}
	return pkcs11.New(p11Opts.SecLevel, p11Opts.HashFamily, ks)
}

// PKCS11Opts contains options for the P11Factory
type PKCS11Opts struct {
	// Default algorithms when not specified (Deprecated?)
	SecLevel   int    `mapstructure:"security" json:"security"`
	HashFamily string `mapstructure:"hash" json:"hash"`

	// Keystore options
	Ephemeral     bool               `mapstructure:"tempkeys,omitempty" json:"tempkeys,omitempty"`
	FileKeystore  *FileKeystoreOpts  `mapstructure:"filekeystore,omitempty" json:"filekeystore,omitempty"`
	DummyKeystore *DummyKeystoreOpts `mapstructure:"dummykeystore,omitempty" json:"dummykeystore,omitempty"`

	// PKCS11 options
	Library    string `mapstructure:"library" json:"library"`
	Label      string `mapstructure:"label" json:"label"`
	Pin        string `mapstructure:"pin" json:"pin"`
	Sensitive  bool   `mapstructure:"sensitivekeys,omitempty" json:"sensitivekeys,omitempty"`
	SoftVerify bool   `mapstructure:"softwareverify,omitempty" json:"softwareverify,omitempty"`
}
