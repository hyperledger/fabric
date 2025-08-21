//go:build pkcs11
// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"os"
	"reflect"
	"strings"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/pkcs11"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const pkcs11Enabled = false

// FactoryOpts holds configuration information used to initialize factory implementations
type FactoryOpts struct {
	Default string             `json:"default" yaml:"Default"`
	SW      *SwOpts            `json:"SW,omitempty" yaml:"SW,omitempty"`
	PKCS11  *pkcs11.PKCS11Opts `json:"PKCS11,omitempty" yaml:"PKCS11"`
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

	// PKCS11-Based BCCSP
	if config.Default == "PKCS11" {
		if config.PKCS11 == nil {
			// Auto-populate PKCS11 configuration from environment variables
			config.PKCS11 = &pkcs11.PKCS11Opts{
				Library:        os.Getenv("CORE_PEER_BCCSP_PKCS11_LIBRARY"),
				Label:          os.Getenv("CORE_PEER_BCCSP_PKCS11_LABEL"),
				Pin:            os.Getenv("CORE_PEER_BCCSP_PKCS11_PIN"),
				Hash:           os.Getenv("CORE_PEER_BCCSP_PKCS11_HASH"),
				Security:       256,  // Default security level
				SoftwareVerify: true, // Default to software verification
			}

			// Set default values if not provided
			if config.PKCS11.Label == "" {
				config.PKCS11.Label = "ForFabric"
			}
			if config.PKCS11.Hash == "" {
				config.PKCS11.Hash = "SHA2"
			}
		}

		f := &PKCS11Factory{}
		var err error
		defaultBCCSP, err = initBCCSP(f, config)
		if err != nil {
			return errors.Wrapf(err, "Failed initializing PKCS11.BCCSP")
		}
	}

	// Software-Based BCCSP
	if config.Default == "SW" && config.SW != nil {
		f := &SWFactory{}
		var err error
		defaultBCCSP, err = initBCCSP(f, config)
		if err != nil {
			return errors.Wrap(err, "Failed initializing SW.BCCSP")
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
	case "PKCS11":
		f = &PKCS11Factory{}
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
		if f.Kind() != reflect.String {
			return data, nil
		}

		if t != reflect.TypeOf(pkcs11.KeyIDMapping{}) {
			return data, nil
		}

		res := pkcs11.KeyIDMapping{}
		raw := data.(string)
		if raw == "" {
			return res, nil
		}

		rec := strings.Fields(raw)
		if len(rec) != 2 {
			return res, nil
		}
		res.SKI = rec[0]
		res.ID = rec[1]

		return res, nil
	}
}
