// +build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitFactoriesWithMultipleProviders(t *testing.T) {
	err := initFactories(&FactoryOpts{
		ProviderName: "SW",
		SwOpts:       &SwOpts{},
		PluginOpts:   &PluginOpts{},
	})
	assert.EqualError(t, err, "Failed initializing BCCSP: Could not initialize BCCSP SW [Failed initializing configuration at [0,]: Hash Family not supported []]")

	err = initFactories(&FactoryOpts{
		ProviderName: "PLUGIN",
		SwOpts:       &SwOpts{},
		PluginOpts:   &PluginOpts{},
	})
	assert.EqualError(t, err, "Failed initializing PLUGIN.BCCSP: Could not initialize BCCSP PLUGIN [Invalid config: missing property 'Library']")
}
