/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

// Validator is a mock implementation of configtx.Validator
type Validator struct {
	// ChainIDVal is returned as the result of ChainID()
	ChainIDVal string

	// SequenceVal is returned as the result of Sequence()
	SequenceVal uint64

	// ApplyVal is returned by Apply
	ApplyVal error

	// AppliedConfigUpdateEnvelope is set by Apply
	AppliedConfigUpdateEnvelope *cb.ConfigEnvelope

	// ValidateVal is returned by Validate
	ValidateVal error

	// ProposeConfigUpdateError is returned as the error value for ProposeConfigUpdate
	ProposeConfigUpdateError error

	// ProposeConfigUpdateVal is returns as the value for ProposeConfigUpdate
	ProposeConfigUpdateVal *cb.ConfigEnvelope

	// ConfigProtoVal is returned as the value for ConfigProtoVal()
	ConfigProtoVal *cb.Config
}

// ConfigProto returns the ConfigProtoVal
func (cm *Validator) ConfigProto() *cb.Config {
	return cm.ConfigProtoVal
}

// ConsensusType returns the ConsensusTypeVal
func (cm *Validator) ChainID() string {
	return cm.ChainIDVal
}

// BatchSize returns the BatchSizeVal
func (cm *Validator) Sequence() uint64 {
	return cm.SequenceVal
}

// ProposeConfigUpdate
func (cm *Validator) ProposeConfigUpdate(update *cb.Envelope) (*cb.ConfigEnvelope, error) {
	return cm.ProposeConfigUpdateVal, cm.ProposeConfigUpdateError
}

// Apply returns ApplyVal
func (cm *Validator) Apply(configEnv *cb.ConfigEnvelope) error {
	cm.AppliedConfigUpdateEnvelope = configEnv
	return cm.ApplyVal
}

// Validate returns ValidateVal
func (cm *Validator) Validate(configEnv *cb.ConfigEnvelope) error {
	return cm.ValidateVal
}
