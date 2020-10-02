/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import "github.com/hyperledger/fabric/bccsp/pkcs11"

// FactoryOpts holds configuration information used to initialize factory implementations
type FactoryOpts struct {
	Default string             `json:"default" yaml:"Default"`
	SW      *SwOpts            `json:"SW,omitempty" yaml:"SW,omitempty"`
	PKCS11  *pkcs11.PKCS11Opts `json:"PKCS11,omitempty" yaml:"PKCS11"`
}

// GetDefaultOpts offers a default implementation for Opts
// returns a new instance every time
func GetDefaultOpts() *FactoryOpts {
	return &FactoryOpts{
		Default: "SW",
		SW: &SwOpts{
			Hash:     "SHA2",
			Security: 256,
		},
		PKCS11: &pkcs11.PKCS11Opts{
			Hash:     "SHA2",
			Security: 256,
		},
	}
}

// FactoryName returns the name of the provider
func (o *FactoryOpts) FactoryName() string {
	return o.Default
}
