// +build !pkcs11

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitFactoriesWithMultipleProviders(t *testing.T) {
	// testing SW Provider and ensuring other providers are not initialized
	factoriesInitError = nil

	err := initFactories(&FactoryOpts{
		ProviderName: "SW",
		SwOpts:       &SwOpts{},
		PluginOpts:   &PluginOpts{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed initializing BCCSP")
	assert.NotContains(t, err.Error(), "Failed initializing PLUGIN.BCCSP")

	// testing PLUGIN Provider and ensuring other providers are not initialized
	factoriesInitError = nil

	err = initFactories(&FactoryOpts{
		ProviderName: "PLUGIN",
		SwOpts:       &SwOpts{},
		PluginOpts:   &PluginOpts{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed initializing PLUGIN.BCCSP")
	assert.NotContains(t, err.Error(), "Failed initializing BCCSP")

}
