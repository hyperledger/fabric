//go:build !noplugin && cgo
// +build !noplugin,cgo

/*
 Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

 SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"fmt"
	"os"
	"plugin"

	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	endorsement "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
)

// loadPlugin loads a pluggable handler
func (r *registry) loadPlugin(pluginPath string, handlerType HandlerType, extraArgs ...string) {
	if _, err := os.Stat(pluginPath); err != nil {
		logger.Panicf(fmt.Sprintf("Could not find plugin at path %s: %s", pluginPath, err))
	}
	p, err := plugin.Open(pluginPath)
	if err != nil {
		logger.Panicf(fmt.Sprintf("Error opening plugin at path %s: %s", pluginPath, err))
	}

	if handlerType == Auth {
		r.initAuthPlugin(p)
	} else if handlerType == Decoration {
		r.initDecoratorPlugin(p)
	} else if handlerType == Endorsement {
		r.initEndorsementPlugin(p, extraArgs...)
	} else if handlerType == Validation {
		r.initValidationPlugin(p, extraArgs...)
	}
}

// initAuthPlugin constructs an auth filter from the given plugin
func (r *registry) initAuthPlugin(p *plugin.Plugin) {
	constructorSymbol, err := p.Lookup(authPluginFactory)
	if err != nil {
		panicWithLookupError(authPluginFactory, err)
	}
	constructor, ok := constructorSymbol.(func() auth.Filter)
	if !ok {
		panicWithDefinitionError(authPluginFactory)
	}

	filter := constructor()
	if filter != nil {
		r.filters = append(r.filters, filter)
	}
}

// initDecoratorPlugin constructs a decorator from the given plugin
func (r *registry) initDecoratorPlugin(p *plugin.Plugin) {
	constructorSymbol, err := p.Lookup(decoratorPluginFactory)
	if err != nil {
		panicWithLookupError(decoratorPluginFactory, err)
	}
	constructor, ok := constructorSymbol.(func() decoration.Decorator)
	if !ok {
		panicWithDefinitionError(decoratorPluginFactory)
	}
	decorator := constructor()
	if decorator != nil {
		r.decorators = append(r.decorators, constructor())
	}
}

func (r *registry) initEndorsementPlugin(p *plugin.Plugin, extraArgs ...string) {
	if len(extraArgs) != 1 {
		logger.Panicf("expected 1 argument in extraArgs")
	}
	factorySymbol, err := p.Lookup(pluginFactory)
	if err != nil {
		panicWithLookupError(pluginFactory, err)
	}

	constructor, ok := factorySymbol.(func() endorsement.PluginFactory)
	if !ok {
		panicWithDefinitionError(pluginFactory)
	}
	factory := constructor()
	if factory == nil {
		logger.Panicf("factory instance returned nil")
	}
	r.endorsers[extraArgs[0]] = factory
}

func (r *registry) initValidationPlugin(p *plugin.Plugin, extraArgs ...string) {
	if len(extraArgs) != 1 {
		logger.Panicf("expected 1 argument in extraArgs")
	}
	factorySymbol, err := p.Lookup(pluginFactory)
	if err != nil {
		panicWithLookupError(pluginFactory, err)
	}

	constructor, ok := factorySymbol.(func() validation.PluginFactory)
	if !ok {
		panicWithDefinitionError(pluginFactory)
	}
	factory := constructor()
	if factory == nil {
		logger.Panicf("factory instance returned nil")
	}
	r.validators[extraArgs[0]] = factory
}

// panicWithLookupError panics when a handler constructor lookup fails
func panicWithLookupError(factory string, err error) {
	logger.Panicf("Plugin must contain constructor with name %s. Error from lookup: %s", factory, err)
}

// panicWithDefinitionError panics when a handler constructor does not match
// the expected function definition
func panicWithDefinitionError(factory string) {
	logger.Panicf("Constructor method %s does not match expected definition", factory)
}
