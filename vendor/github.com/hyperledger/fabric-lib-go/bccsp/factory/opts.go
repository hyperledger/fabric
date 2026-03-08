/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

// GetDefaultOpts offers a default implementation for Opts
// returns a new instance every time
func GetDefaultOpts() *FactoryOpts {
	return &FactoryOpts{
		Default: "SW",
		SW: &SwOpts{
			Hash:     "SHA2",
			Security: 256,
		},
	}
}

// FactoryName returns the name of the provider
func (o *FactoryOpts) FactoryName() string {
	return o.Default
}
