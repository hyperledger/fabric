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
	"sync"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/pkcs11"
)

const (
	// PKCS11BasedFactoryName is the name of the factory of the hsm-based BCCSP implementation
	PKCS11BasedFactoryName = "P11"
)

// PKCS11Factory is the factory of the HSM-based BCCSP.
type PKCS11Factory struct {
	initOnce sync.Once
	bccsp    bccsp.BCCSP
	err      error
}

// Name returns the name of this factory
func (f *PKCS11Factory) Name() string {
	return PKCS11BasedFactoryName
}

// Get returns an instance of BCCSP using Opts.
func (f *PKCS11Factory) Get(opts Opts) (bccsp.BCCSP, error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid opts. It must not be nil.")
	}

	if opts.FactoryName() != f.Name() {
		return nil, fmt.Errorf("Invalid Provider Name [%s]. Opts must refer to [%s].", opts.FactoryName(), f.Name())
	}

	pkcs11Opts, ok := opts.(*PKCS11Opts)
	if !ok {
		return nil, errors.New("Invalid opts. They must be of type PKCS11Opts.")
	}

	if !opts.Ephemeral() {
		f.initOnce.Do(func() {
			f.bccsp, f.err = pkcs11.New(pkcs11Opts.SecLevel, pkcs11Opts.HashFamily, pkcs11Opts.KeyStore)
			return
		})
		return f.bccsp, f.err
	}

	return pkcs11.New(pkcs11Opts.SecLevel, pkcs11Opts.HashFamily, pkcs11Opts.KeyStore)
}

// PKCS11Opts contains options for the P11Factory
type PKCS11Opts struct {
	Ephemeral_ bool
	SecLevel   int
	HashFamily string
	KeyStore   bccsp.KeyStore
}

// FactoryName returns the name of the provider
func (o *PKCS11Opts) FactoryName() string {
	return PKCS11BasedFactoryName
}

// Ephemeral returns true if the CSP has to be ephemeral, false otherwise
func (o *PKCS11Opts) Ephemeral() bool {
	return o.Ephemeral_
}
