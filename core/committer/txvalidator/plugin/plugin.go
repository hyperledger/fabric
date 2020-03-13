/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plugin

import validation "github.com/hyperledger/fabric/core/handlers/validation/api"

// Name defines the name of the plugin as it appears in the configuration
type Name string

// Mapper maps plugin names to their corresponding factory instance.
// Returns nil if the name isn't associated to any plugin.
type Mapper interface {
	FactoryByName(name Name) validation.PluginFactory
}

// MapBasedMapper maps plugin names to their corresponding factories
type MapBasedMapper map[string]validation.PluginFactory

// FactoryByName returns a plugin factory for the given plugin name, or nil if not found
func (m MapBasedMapper) FactoryByName(name Name) validation.PluginFactory {
	return m[string(name)]
}

// SerializedPolicy defines a marshaled policy
type SerializedPolicy []byte

// Bytes returns te bytes of the SerializedPolicy
func (sp SerializedPolicy) Bytes() []byte {
	return sp
}
