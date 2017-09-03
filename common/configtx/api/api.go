/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"github.com/hyperledger/fabric/common/policies"
	cb "github.com/hyperledger/fabric/protos/common"
)

// Manager provides a mechanism to query and update config
type Manager interface {
	// Validate attempts to apply a configtx to become the new config
	Validate(configEnv *cb.ConfigEnvelope) error

	// Validate attempts to validate a new configtx against the current config state
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)

	// ChainID retrieves the chain ID associated with this manager
	ChainID() string

	// ConfigEnvelope returns the current config envelope
	ConfigEnvelope() *cb.ConfigEnvelope

	// Sequence returns the current sequence number of the config
	Sequence() uint64
}

// Proposer contains the references necessary to appropriately unmarshal
// a cb.ConfigGroup
type Proposer interface {
	// RootGroupKey is the string to use to namespace the root group
	RootGroupKey() string

	// PolicyManager() returns the policy manager for considering config changes
	PolicyManager() policies.Manager
}
