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
	defaultBCCSP       bccsp.BCCSP // default BCCSP
	factoriesInitOnce  sync.Once   // factories' Sync on Initialization
	factoriesInitError error       // Factories' Initialization Error

	// when InitFactories has not been called yet (should only happen
	// in test cases), use this BCCSP temporarily
	bootBCCSP         bccsp.BCCSP
	bootBCCSPInitOnce sync.Once

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
	if defaultBCCSP == nil {
		logger.Debug("Before using BCCSP, please call InitFactories(). Falling back to bootBCCSP.")
		bootBCCSPInitOnce.Do(func() {
			var err error
			bootBCCSP, err = (&SWFactory{}).Get(GetDefaultOpts())
			if err != nil {
				panic("BCCSP Internal error, failed initialization with GetDefaultOpts!")
			}
		})
		return bootBCCSP
	}
	return defaultBCCSP
}

func initBCCSP(f BCCSPFactory, config *FactoryOpts) (bccsp.BCCSP, error) {
	csp, err := f.Get(config)
	if err != nil {
		return nil, errors.Errorf("Could not initialize BCCSP %s [%s]", f.Name(), err)
	}

	return csp, nil
}
