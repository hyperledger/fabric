//go:build !pkcs11
// +build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"reflect"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const pkcs11Enabled = false

// FactoryOpts holds configuration information used to initialize factory implementations
type FactoryOpts struct {
	Default string  `json:"default" yaml:"Default"`
	SW      *SwOpts `json:"SW,omitempty" yaml:"SW,omitempty"`
}

// InitFactories must be called before using factory interfaces
// It is acceptable to call with config = nil, in which case
// some defaults will get used
// Error is returned only if defaultBCCSP cannot be found
func InitFactories(config *FactoryOpts) error {
	factoriesInitOnce.Do(func() {
		factoriesInitError = initFactories(config)
	})

	return factoriesInitError
}

func initFactories(config *FactoryOpts) error {
	// Take some precautions on default opts
	if config == nil {
		config = GetDefaultOpts()
	}

	if config.Default == "" {
		config.Default = "SW"
	}

	if config.SW == nil {
		config.SW = GetDefaultOpts().SW
	}

	// Software-Based BCCSP
	if config.Default == "SW" && config.SW != nil {
		f := &SWFactory{}
		var err error
		defaultBCCSP, err = initBCCSP(f, config)
		if err != nil {
			return errors.Wrapf(err, "Failed initializing BCCSP")
		}
	}

	if defaultBCCSP == nil {
		return errors.Errorf("Could not find default `%s` BCCSP", config.Default)
	}

	return nil
}

// GetBCCSPFromOpts returns a BCCSP created according to the options passed in input.
func GetBCCSPFromOpts(config *FactoryOpts) (bccsp.BCCSP, error) {
	var f BCCSPFactory
	switch config.Default {
	case "SW":
		f = &SWFactory{}
	default:
		return nil, errors.Errorf("Could not find BCCSP, no '%s' provider", config.Default)
	}

	csp, err := f.Get(config)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not initialize BCCSP %s", f.Name())
	}
	return csp, nil
}

// StringToKeyIds returns a DecodeHookFunc that converts
// strings to pkcs11.KeyIDMapping.
func StringToKeyIds() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		return data, nil
	}
}
