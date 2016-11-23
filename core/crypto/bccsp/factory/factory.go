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
	"errors"
	"fmt"
	"sync"

	"os"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/sw"
)

var (
	// Default BCCSP
	defaultBCCSP bccsp.BCCSP

	// BCCSP Factories
	factories map[string]BCCSPFactory

	// factories' Sync on Initialization
	factoriesInitOnce sync.Once

	// Factories' Initialization Error
	factoriesInitError error
)

// BCCSPFactory is used to get instances of the BCCSP interface.
// A Factory has name used to address it.
type BCCSPFactory interface {

	// Name returns the name of this factory
	Name() string

	// Get returns an instance of BCCSP using opts.
	Get(opts Opts) (bccsp.BCCSP, error)
}

// Opts  contains options for instantiating BCCSPs.
type Opts interface {

	// FactoryName returns the name of the factory to be used
	FactoryName() string

	// Ephemeral returns true if the BCCSP has to be ephemeral, false otherwise
	Ephemeral() bool
}

// GetDefault returns a non-ephemeral (long-term) BCCSP
func GetDefault() (bccsp.BCCSP, error) {
	if err := initFactories(); err != nil {
		return nil, err
	}

	return defaultBCCSP, nil
}

// GetBCCSP returns a BCCSP created according to the options passed in input.
func GetBCCSP(opts Opts) (bccsp.BCCSP, error) {
	if err := initFactories(); err != nil {
		return nil, err
	}

	return getBCCSPInternal(opts)
}

func initFactories() error {
	factoriesInitOnce.Do(func() {
		// Initialize factories map
		if factoriesInitError = initFactoriesMap(); factoriesInitError != nil {
			return
		}

		// Create default non-ephemeral (long-term) BCCSP
		defaultBCCSP, factoriesInitError = createDefaultBCCSP()
		if factoriesInitError != nil {
			return
		}
	})
	return factoriesInitError
}

func initFactoriesMap() error {
	factories = make(map[string]BCCSPFactory)

	// Software-Based BCCSP
	f := &SWFactory{}
	factories[f.Name()] = f

	return nil
}

func createDefaultBCCSP() (bccsp.BCCSP, error) {
	return sw.NewDefaultSecurityLevel(os.TempDir())
}

func getBCCSPInternal(opts Opts) (bccsp.BCCSP, error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Cannot instantiate a factory with 'nil' Opts. A fully instantiated BCCSP Opts struct must be provided.")
	}

	f, ok := factories[opts.FactoryName()]
	if ok {
		return f.Get(opts)
	}

	return nil, fmt.Errorf("Factory [%s] does not exist.", opts.FactoryName())
}
