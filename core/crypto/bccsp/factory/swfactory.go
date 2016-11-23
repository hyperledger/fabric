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

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/sw"
)

const (
	// SoftwareBasedFactoryName is the name of the factory of the software-based BCCSP implementation
	SoftwareBasedFactoryName = "SW"
)

// SWFactory is the factory of the software-based BCCSP.
type SWFactory struct {
	initOnce sync.Once
	bccsp    bccsp.BCCSP
	err      error
}

// Name returns the name of this factory
func (f *SWFactory) Name() string {
	return SoftwareBasedFactoryName
}

// Get returns an instance of BCCSP using Opts.
func (f *SWFactory) Get(opts Opts) (bccsp.BCCSP, error) {
	// Validate arguments
	if opts == nil {
		return nil, errors.New("Invalid opts. It must not be nil.")
	}

	if opts.FactoryName() != f.Name() {
		return nil, fmt.Errorf("Invalid Provider Name [%s]. Opts must refer to [%s].", opts.FactoryName(), f.Name())
	}

	swOpts, ok := opts.(*SwOpts)
	if !ok {
		return nil, errors.New("Invalid opts. They must be of type SwOpts.")
	}

	if !opts.Ephemeral() {
		f.initOnce.Do(func() {
			f.bccsp, f.err = sw.New(swOpts.SecLevel, swOpts.HashFamily, swOpts.KeyStore)
			return
		})
		return f.bccsp, f.err
	}

	return sw.New(swOpts.SecLevel, swOpts.HashFamily, swOpts.KeyStore)
}

// SwOpts contains options for the SWFactory
type SwOpts struct {
	Ephemeral_ bool
	SecLevel   int
	HashFamily string
	KeyStore   bccsp.KeyStore
}

// FactoryName returns the name of the provider
func (o *SwOpts) FactoryName() string {
	return SoftwareBasedFactoryName
}

// Ephemeral returns true if the CSP has to be ephemeral, false otherwise
func (o *SwOpts) Ephemeral() bool {
	return o.Ephemeral_
}
