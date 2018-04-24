/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"
	"os"
	"plugin"
	"sync"

	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/pkg/errors"
)

const (
	sccFactoryMethod = "New"
)

// PluginConfig SCC plugin configuration
type PluginConfig struct {
	Enabled           bool   `mapstructure:"enabled" yaml:"enabled"`
	Name              string `mapstructure:"name" yaml:"name"`
	Path              string `mapstructure:"path" yaml:"path"`
	InvokableExternal bool   `mapstructure:"invokableExternal" yaml:"invokableExternal"`
	InvokableCC2CC    bool   `mapstructure:"invokableCC2CC" yaml:"invokableCC2CC"`
}

var once sync.Once
var sccPlugins []*SystemChaincode

// loadSysCCs reads system chaincode plugin configuration and loads them
func loadSysCCs(p *Provider) []*SystemChaincode {
	once.Do(func() {
		var config []*PluginConfig
		err := viperutil.EnhancedExactUnmarshalKey("chaincode.systemPlugins", &config)
		if err != nil {
			panic(errors.WithMessage(err, "could not load YAML config"))
		}
		loadSysCCsWithConfig(config)
	})
	return sccPlugins
}

func loadSysCCsWithConfig(configs []*PluginConfig) {
	for _, conf := range configs {
		plugin := loadPlugin(conf.Path)
		chaincode := &SystemChaincode{
			Enabled:           conf.Enabled,
			Name:              conf.Name,
			Path:              conf.Path,
			Chaincode:         *plugin,
			InvokableExternal: conf.InvokableExternal,
			InvokableCC2CC:    conf.InvokableCC2CC,
		}
		sccPlugins = append(sccPlugins, chaincode)
		sysccLogger.Infof("Successfully loaded SCC %s from path %s", chaincode.Name, chaincode.Path)
	}
}

func loadPlugin(path string) *shim.Chaincode {
	if _, err := os.Stat(path); err != nil {
		panic(fmt.Errorf("Could not find plugin at path %s: %s", path, err))
	}

	p, err := plugin.Open(path)
	if err != nil {
		panic(fmt.Errorf("Error opening plugin at path %s: %s", path, err))
	}

	sccFactorySymbol, err := p.Lookup(sccFactoryMethod)
	if err != nil {
		panic(fmt.Errorf(
			"Could not find symbol %s. Plugin must export this method", sccFactoryMethod))
	}

	sccFactory, ok := sccFactorySymbol.(func() shim.Chaincode)
	if !ok {
		panic(fmt.Errorf("Function %s does not match expected definition func() shim.Chaincode",
			sccFactoryMethod))
	}

	scc := sccFactory()

	return &scc
}
