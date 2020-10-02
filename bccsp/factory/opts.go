/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package factory

import "github.com/hyperledger/fabric/bccsp/pkcs11"

// FactoryOpts holds configuration information used to initialize factory implementations
type FactoryOpts struct {
	ProviderName string             `mapstructure:"default" json:"default" yaml:"Default"`
	SwOpts       *SwOpts            `mapstructure:"SW,omitempty" json:"SW,omitempty" yaml:"SW,omitempty"`
	PluginOpts   *PluginOpts        `mapstructure:"PLUGIN,omitempty" json:"PLUGIN,omitempty" yaml:"PluginOpts"`
	Pkcs11Opts   *pkcs11.PKCS11Opts `mapstructure:"PKCS11,omitempty" json:"PKCS11,omitempty" yaml:"PKCS11"`
}

// GetDefaultOpts offers a default implementation for Opts
// returns a new instance every time
func GetDefaultOpts() *FactoryOpts {
	return &FactoryOpts{
		ProviderName: "SW",
		SwOpts: &SwOpts{
			HashFamily: "SHA2",
			SecLevel:   256,

			Ephemeral: true,
		},
		Pkcs11Opts: &pkcs11.PKCS11Opts{
			HashFamily: "SHA2",
			SecLevel:   256,
		},
	}
}

// FactoryName returns the name of the provider
func (o *FactoryOpts) FactoryName() string {
	return o.ProviderName
}
