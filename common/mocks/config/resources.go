/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/msp"
)

type Resources struct {
	// ConfigtxValidatorVal is returned as the result of ConfigtxValidator
	ConfigtxValidatorVal configtx.Validator

	// PolicyManagerVal is returned as the result of PolicyManager()
	PolicyManagerVal policies.Manager

	// ChannelConfigVal is returned as the result of ChannelConfig()
	ChannelConfigVal channelconfig.Channel

	// OrdererConfigVal is returned as the result of OrdererConfig()
	OrdererConfigVal channelconfig.Orderer

	// ApplicationConfigVal is returned as the result of ApplicationConfig()
	ApplicationConfigVal channelconfig.Application

	// ConsortiumsConfigVal is returned as the result of ConsortiumsConfig()
	ConsortiumsConfigVal channelconfig.Consortiums

	// MSPManagerVal is returned as the result of MSPManager()
	MSPManagerVal msp.MSPManager

	// ValidateNewErr is returned as the result of ValidateNew
	ValidateNewErr error
}

// ConfigtxMangaer returns ConfigtxValidatorVal
func (r *Resources) ConfigtxValidator() configtx.Validator {
	return r.ConfigtxValidatorVal
}

// Returns the PolicyManagerVal
func (r *Resources) PolicyManager() policies.Manager {
	return r.PolicyManagerVal
}

// Returns the ChannelConfigVal
func (r *Resources) ChannelConfig() channelconfig.Channel {
	return r.ChannelConfigVal
}

// Returns the OrdererConfigVal
func (r *Resources) OrdererConfig() (channelconfig.Orderer, bool) {
	return r.OrdererConfigVal, r.OrdererConfigVal != nil
}

// Returns the ApplicationConfigVal
func (r *Resources) ApplicationConfig() (channelconfig.Application, bool) {
	return r.ApplicationConfigVal, r.ApplicationConfigVal != nil
}

func (r *Resources) ConsortiumsConfig() (channelconfig.Consortiums, bool) {
	return r.ConsortiumsConfigVal, r.ConsortiumsConfigVal != nil
}

// Returns the MSPManagerVal
func (r *Resources) MSPManager() msp.MSPManager {
	return r.MSPManagerVal
}

// ValidateNew returns ValidateNewErr
func (r *Resources) ValidateNew(res channelconfig.Resources) error {
	return r.ValidateNewErr
}
