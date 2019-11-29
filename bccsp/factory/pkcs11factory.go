// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/pkg/errors"
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
	if config == nil || config.Pkcs11Opts == nil {
		return nil, errors.New("Invalid config. It must not be nil.")
	}

	p11Opts := config.Pkcs11Opts
	ks := sw.NewDummyKeyStore()

	return pkcs11.New(*p11Opts, ks)
}
