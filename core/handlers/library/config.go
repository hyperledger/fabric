/*
 Copyright IBM Corp All Rights Reserved.

 SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Config configures the factory methods
// and plugins for the registry
type Config struct {
	AuthFilters []*HandlerConfig `yaml:"authFilters"`
	Decorators  []*HandlerConfig `yaml:"decorators"`
	Endorsers   PluginMapping    `yaml:"endorsers"`
	Validators  PluginMapping    `yaml:"validators"`
}

// PluginMapping stores a map between chaincode id to plugin config
type PluginMapping map[string]*HandlerConfig

// HandlerConfig defines configuration for a plugin or compiled handler
type HandlerConfig struct {
	Name    string `yaml:"name"`
	Library string `yaml:"library"`
}

func LoadConfig() (Config, error) {
	var authFilters, decorators []*HandlerConfig
	if err := mapstructure.Decode(viper.Get("peer.handlers.authFilters"), &authFilters); err != nil {
		return Config{}, err
	}

	if err := mapstructure.Decode(viper.Get("peer.handlers.decorators"), &decorators); err != nil {
		return Config{}, err
	}

	endorsers, validators := make(PluginMapping), make(PluginMapping)
	e := viper.GetStringMap("peer.handlers.endorsers")
	for k := range e {
		name := viper.GetString("peer.handlers.endorsers." + k + ".name")
		library := viper.GetString("peer.handlers.endorsers." + k + ".library")
		endorsers[k] = &HandlerConfig{Name: name, Library: library}
	}

	v := viper.GetStringMap("peer.handlers.validators")
	for k := range v {
		name := viper.GetString("peer.handlers.validators." + k + ".name")
		library := viper.GetString("peer.handlers.validators." + k + ".library")
		validators[k] = &HandlerConfig{Name: name, Library: library}
	}

	return Config{
		AuthFilters: authFilters,
		Decorators:  decorators,
		Endorsers:   endorsers,
		Validators:  validators,
	}, nil
}
