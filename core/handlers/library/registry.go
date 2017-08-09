/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"fmt"
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
	Lookup(string) interface{}
}

const (
	AuthKey      = "Auth"
	DecoratorKey = "Decorator"

	defaultAuthFactory      = "DefaultAuth"
	defaultDecoratorFactory = "DefaultDecorator"
)

type registry map[string]interface{}

var once sync.Once
var reg registry

// Config configures the factory methods
// for the registry
type Config struct {
	AuthFilterFactory string
	DecoratorFactory  string
}

// InitRegistry creates the (only) instance
// of the registry
func InitRegistry(c Config) Registry {
	once.Do(func() {
		reg = make(map[string]interface{})
		reg.load(c)
	})
	return reg
}

func (r registry) load(c Config) {
	registryMD := reflect.ValueOf(&HandlerLibrary{})

	if c.AuthFilterFactory == "" {
		c.AuthFilterFactory = defaultAuthFactory
	}

	if c.DecoratorFactory == "" {
		c.DecoratorFactory = defaultDecoratorFactory
	}

	o := registryMD.MethodByName(c.AuthFilterFactory)
	if !o.IsValid() {
		panic(fmt.Errorf("Method %s isn't a method of HandlerLibrary", c.AuthFilterFactory))
	}

	inst := o.Call(nil)[0].Interface()
	r[AuthKey] = inst.(auth.Filter)

	o = registryMD.MethodByName(c.DecoratorFactory)
	if !o.IsValid() {
		panic(fmt.Errorf("Method %s isn't a method of HandlerLibrary", c.DecoratorFactory))
	}

	inst = o.Call(nil)[0].Interface()
	r[DecoratorKey] = inst.(decoration.Decorator)
}

// Lookup returns a handler with a given
// registered name, or nil if exists
func (r registry) Lookup(name string) interface{} {
	return r[name]
}
