/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"sync"
	"sync/atomic"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/pkg/errors"
)

var (
	defaultBCCSP       atomic.Pointer[bccsp.BCCSP] // default BCCSP
	factoriesInitError atomic.Pointer[error]       // Factories' Initialization Error
	factoriesInitOnce  sync.Once                   // Factories' Sync on Initialization

	// when InitFactories has not been called yet (should only happen
	// in test cases), use this BCCSP temporarily
	bootBCCSP         atomic.Pointer[bccsp.BCCSP]
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

// InitFactories must be called before using factory interfaces
// It is acceptable to call with config = nil, in which case
// some defaults will get used.
// Error is returned only if defaultBCCSP cannot be found.
func InitFactories(config *FactoryOpts) error {
	factoriesInitOnce.Do(func() {
		res, err := initFactories(config)
		if err != nil {
			factoriesInitError.Store(&err)
		}
		if res != nil {
			defaultBCCSP.Store(&res)
		}
	})
	errPtr := factoriesInitError.Load()
	if errPtr != nil {
		return *errPtr
	}
	return nil
}

// GetDefault returns a non-ephemeral (long-term) BCCSP
func GetDefault() bccsp.BCCSP {
	res := defaultBCCSP.Load()
	if res != nil {
		return *res
	}

	bootBCCSPInitOnce.Do(func() {
		logger.Debug("Before using BCCSP, please call InitFactories(). Falling back to bootBCCSP.")
		newBootBCCSP, err := (&SWFactory{}).Get(GetDefaultOpts())
		if err != nil {
			panic("BCCSP Internal error, failed initialization with GetDefaultOpts!")
		}
		bootBCCSP.Store(&newBootBCCSP)
	})

	res = bootBCCSP.Load()
	if res != nil {
		return *res
	}
	// This should never happen, but if it does, panic is better than returning nil.
	panic("BCCSP Internal error, both defaultBCCSP and bootBCCSP are nil")
}

func initBCCSP(f BCCSPFactory, config *FactoryOpts) (bccsp.BCCSP, error) {
	csp, err := f.Get(config)
	if err != nil {
		return nil, errors.Errorf("Could not initialize BCCSP %s [%s]", f.Name(), err)
	}

	return csp, nil
}
