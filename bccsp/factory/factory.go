/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"sync"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/pkg/errors"
)

var (
	// Default BCCSP
	DefaultBCCSP bccsp.BCCSP

	// when InitFactories has not been called yet (should only happen
	// in test cases), use this BCCSP temporarily
	bootBCCSP bccsp.BCCSP

	// BCCSP Factories
	bccspMap map[string]bccsp.BCCSP

	// factories' Sync on Initialization
	factoriesInitOnce sync.Once
	bootBCCSPInitOnce sync.Once

	// Factories' Initialization Error
	factoriesInitError error

	logger = flogging.MustGetLogger("bccsp")
)

// BCCSPFactory is used to get instances of the BCCSP interface.
// A Factory has name used to address it.
type BCCSPFactory interface {

	// Name returns the name of this factory
	Name() string

	// Get returns an instance of BCCSP using opts.
	Get(opts *FactoryOpts) (bccsp.BCCSP, error)
}

// GetDefault returns a non-ephemeral (long-term) BCCSP
func GetDefault() bccsp.BCCSP {
	if DefaultBCCSP == nil {
		logger.Debug("Before using BCCSP, please call InitFactories(). Falling back to bootBCCSP.")
		bootBCCSPInitOnce.Do(func() {
			var err error
			f := &SWFactory{}
			bootBCCSP, err = f.Get(&FactoryOpts{
				ProviderName: "SW",
				SwOpts: &SwOpts{
					HashFamily: "SHA2",
					SecLevel:   256,

					Ephemeral: true,
				}})
			if err != nil {
				panic("BCCSP Internal error, failed initialization with GetDefaultOpts!")
			}
		})
		return bootBCCSP
	}
	return DefaultBCCSP
}

func initBCCSP(f BCCSPFactory, config *FactoryOpts) error {
	csp, err := f.Get(config)
	if err != nil {
		return errors.Errorf("Could not initialize BCCSP %s [%s]", f.Name(), err)
	}

	logger.Debugf("Initialize BCCSP [%s]", f.Name())
	bccspMap[f.Name()] = csp
	return nil
}
