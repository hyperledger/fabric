/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	endorsement2 "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
)

var logger = flogging.MustGetLogger("core.handlers")

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
	Endorsement
	Validation

	authPluginFactory      = "NewFilter"
	decoratorPluginFactory = "NewDecorator"
	pluginFactory          = "NewPluginFactory"
)

type registry struct {
	filters    []auth.Filter
	decorators []decoration.Decorator
	endorsers  map[string]endorsement2.PluginFactory
	validators map[string]validation.PluginFactory
}

var (
	once sync.Once
	reg  registry
)

// InitRegistry creates the (only) instance
// of the registry
func InitRegistry(c Config) Registry {
	once.Do(func() {
		reg = registry{
			endorsers:  make(map[string]endorsement2.PluginFactory),
			validators: make(map[string]validation.PluginFactory),
		}
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

	for chaincodeID, config := range c.Endorsers {
		r.evaluateModeAndLoad(config, Endorsement, chaincodeID)
	}

	for chaincodeID, config := range c.Validators {
		r.evaluateModeAndLoad(config, Validation, chaincodeID)
	}
}

// evaluateModeAndLoad if a library path is provided, load the shared object
func (r *registry) evaluateModeAndLoad(c *HandlerConfig, handlerType HandlerType, extraArgs ...string) {
	if c.Library != "" {
		r.loadPlugin(c.Library, handlerType, extraArgs...)
	} else {
		r.loadCompiled(c.Name, handlerType, extraArgs...)
	}
}

// loadCompiled loads a statically compiled handler
func (r *registry) loadCompiled(handlerFactory string, handlerType HandlerType, extraArgs ...string) {
	registryMD := reflect.ValueOf(&HandlerLibrary{})

	o := registryMD.MethodByName(handlerFactory)
	if !o.IsValid() {
		logger.Panicf(fmt.Sprintf("Method %s isn't a method of HandlerLibrary", handlerFactory))
	}

	inst := o.Call(nil)[0].Interface()

	if handlerType == Auth {
		r.filters = append(r.filters, inst.(auth.Filter))
	} else if handlerType == Decoration {
		r.decorators = append(r.decorators, inst.(decoration.Decorator))
	} else if handlerType == Endorsement {
		if len(extraArgs) != 1 {
			logger.Panicf("expected 1 argument in extraArgs")
		}
		r.endorsers[extraArgs[0]] = inst.(endorsement2.PluginFactory)
	} else if handlerType == Validation {
		if len(extraArgs) != 1 {
			logger.Panicf("expected 1 argument in extraArgs")
		}
		r.validators[extraArgs[0]] = inst.(validation.PluginFactory)
	}
}

// Lookup returns a list of handlers with the given
// type, or nil if none exist
func (r *registry) Lookup(handlerType HandlerType) interface{} {
	if handlerType == Auth {
		return r.filters
	} else if handlerType == Decoration {
		return r.decorators
	} else if handlerType == Endorsement {
		return r.endorsers
	} else if handlerType == Validation {
		return r.validators
	}

	return nil
}
