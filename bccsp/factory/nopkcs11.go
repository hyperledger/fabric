// +build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package factory

import (
	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
)

const pkcs11Enabled = false

// FactoryOpts holds configuration information used to initialize factory implementations
type FactoryOpts struct {
	ProviderName string      `mapstructure:"default" json:"default" yaml:"Default"`
	SwOpts       *SwOpts     `mapstructure:"SW,omitempty" json:"SW,omitempty" yaml:"SwOpts"`
	PluginOpts   *PluginOpts `mapstructure:"PLUGIN,omitempty" json:"PLUGIN,omitempty" yaml:"PluginOpts"`
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

	if config.ProviderName == "" {
		config.ProviderName = "SW"
	}

	if config.SwOpts == nil {
		config.SwOpts = GetDefaultOpts().SwOpts
	}

	// Initialize factories map
	bccspMap = make(map[string]bccsp.BCCSP)

	// Software-Based BCCSP
	if config.ProviderName == "SW" && config.SwOpts != nil {
		f := &SWFactory{}
		err := initBCCSP(f, config)
		if err != nil {
			return errors.Wrapf(err, "Failed initializing BCCSP")
		}
	}

	// BCCSP Plugin
	if config.ProviderName == "PLUGIN" && config.PluginOpts != nil {
		f := &PluginFactory{}
		err := initBCCSP(f, config)
		if err != nil {
			return errors.Wrapf(err, "Failed initializing PLUGIN.BCCSP")
		}
	}

	var ok bool
	defaultBCCSP, ok = bccspMap[config.ProviderName]
	if !ok {
		return errors.Errorf("Could not find default `%s` BCCSP", config.ProviderName)
	}
	return nil
}

// GetBCCSPFromOpts returns a BCCSP created according to the options passed in input.
func GetBCCSPFromOpts(config *FactoryOpts) (bccsp.BCCSP, error) {
	var f BCCSPFactory
	switch config.ProviderName {
	case "SW":
		f = &SWFactory{}
	case "PLUGIN":
		f = &PluginFactory{}
	default:
		return nil, errors.Errorf("Could not find BCCSP, no '%s' provider", config.ProviderName)
	}

	csp, err := f.Get(config)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not initialize BCCSP %s", f.Name())
	}
	return csp, nil
}
