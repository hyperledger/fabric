//go:build !pkcs11
// +build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"reflect"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const pkcs11Enabled = false

// FactoryOpts holds configuration information used to initialize factory implementations
type FactoryOpts struct {
	Default string  `json:"default" yaml:"Default"`
	SW      *SwOpts `json:"SW,omitempty" yaml:"SW,omitempty"`
}

func initFactories(config *FactoryOpts) (res bccsp.BCCSP, err error) {
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
		res, err = initBCCSP(f, config)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed initializing BCCSP")
		}
	}

	if res == nil {
		return nil, errors.Errorf("Could not find default `%s` BCCSP", config.Default)
	}

	return res, nil
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
