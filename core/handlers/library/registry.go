/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"fmt"
	"os"
	"plugin"
	"reflect"
	"sync"

	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/decoration"
)

// Registry defines an object that looks up
// handlers by name
type Registry interface {
	// Lookup returns a handler with a given
	// registered name, or nil if does not exist
	Lookup(HandlerType) interface{}
}

// HandlerType defines custom handlers that can filter and mutate
// objects passing within the peer
type HandlerType int

const (
	// Auth handler - reject or forward proposals from clients
	Auth HandlerType = iota
	// Decoration handler - append or mutate the chaincode input
	// passed to the chaincode
	Decoration

	authPluginFactory      = "NewFilter"
	decoratorPluginFactory = "NewDecorator"
)

type registry struct {
	filters    []auth.Filter
	decorators []decoration.Decorator
}

var once sync.Once
var reg registry

// Config configures the factory methods
// and plugins for the registry
type Config struct {
	AuthFilters []*HandlerConfig `mapstructure:"authFilters" yaml:"authFilters"`
	Decorators  []*HandlerConfig `mapstructure:"decorators" yaml:"decorators"`
}

// HandlerConfig defines configuration for a plugin or compiled handler
type HandlerConfig struct {
	Name    string `mapstructure:"name" yaml:"name"`
	Library string `mapstructure:"library" yaml:"library"`
}

// InitRegistry creates the (only) instance
// of the registry
func InitRegistry(c Config) Registry {
	once.Do(func() {
		reg = registry{}
		reg.loadHandlers(c)
	})
	return &reg
}

// loadHandlers loads the configured handlers
func (r *registry) loadHandlers(c Config) {
	for _, config := range c.AuthFilters {
		r.evaluateModeAndLoad(config, Auth)
	}
	for _, config := range c.Decorators {
		r.evaluateModeAndLoad(config, Decoration)
	}
}

// evaluateModeAndLoad if a library path is provided, load the shared object
func (r *registry) evaluateModeAndLoad(c *HandlerConfig, handlerType HandlerType) {
	if c.Library != "" {
		r.loadPlugin(c.Library, handlerType)
	} else {
		r.loadCompiled(c.Name, handlerType)
	}
}

// loadCompiled loads a statically compiled handler
func (r *registry) loadCompiled(handlerFactory string, handlerType HandlerType) {
	registryMD := reflect.ValueOf(&HandlerLibrary{})

	o := registryMD.MethodByName(handlerFactory)
	if !o.IsValid() {
		panic(fmt.Errorf("Method %s isn't a method of HandlerLibrary", handlerFactory))
	}

	inst := o.Call(nil)[0].Interface()

	if handlerType == Auth {
		r.filters = append(r.filters, inst.(auth.Filter))
	} else if handlerType == Decoration {
		r.decorators = append(r.decorators, inst.(decoration.Decorator))
	}
}

// loadPlugin loads a pluggagle handler
func (r *registry) loadPlugin(pluginPath string, handlerType HandlerType) {
	if _, err := os.Stat(pluginPath); err != nil {
		panic(fmt.Errorf("Could not find plugin at path %s: %s", pluginPath, err))
	}

	p, err := plugin.Open(pluginPath)
	if err != nil {
		panic(fmt.Errorf("Error opening plugin at path %s: %s", pluginPath, err))
	}

	if handlerType == Auth {
		r.initAuthPlugin(p)
	} else if handlerType == Decoration {
		r.initDecoratorPlugin(p)
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

// panicWithLookupError panics when a handler constructor lookup fails
func panicWithLookupError(factory string, err error) {
	panic(fmt.Errorf("Filter must contain constructor with name %s. Error from lookup: %s",
		factory, err))
}

// panicWithDefinitionError panics when a handler constructor does not match
// the expected function definition
func panicWithDefinitionError(factory string) {
	panic(fmt.Errorf("Constructor method %s does not match expected definition",
		authPluginFactory))
}

// Lookup returns a list of handlers with the given
// given type, or nil if none exist
func (r *registry) Lookup(handlerType HandlerType) interface{} {
	if handlerType == Auth {
		return r.filters
	} else if handlerType == Decoration {
		return r.decorators
	}

	return nil
}
