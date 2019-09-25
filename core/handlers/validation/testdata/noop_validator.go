/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger/fabric-protos-go/common"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
)

// NoOpValidator is used to test validation plugin infrastructure
type NoOpValidator struct {
}

// Validate valides the transactions with the given data
func (*NoOpValidator) Validate(_ *common.Block, _ string, _ int, _ int, _ ...validation.ContextDatum) error {
	return nil
}

// Init initializes the plugin with the given dependencies
func (*NoOpValidator) Init(dependencies ...validation.Dependency) error {
	return nil
}

// NoOpValidatorFactory creates new NoOpValidators
type NoOpValidatorFactory struct {
}

// New returns an instance of a NoOpValidator
func (*NoOpValidatorFactory) New() validation.Plugin {
	return &NoOpValidator{}
}

// NewPluginFactory is called by the validation plugin framework to obtain an instance
// of the factory
func NewPluginFactory() validation.PluginFactory {
	return &NoOpValidatorFactory{}
}
