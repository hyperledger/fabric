/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package config

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

// Config encapsulates config (channel or resource) tree
type Config interface {
	// ConfigProto returns the current config
	ConfigProto() *cb.Config

	// ProposeConfigUpdate attempts to validate a new configtx against the current config state
	ProposeConfigUpdate(configtx *cb.Envelope) (*cb.ConfigEnvelope, error)
}

// Manager provides access to the resource config
type Manager interface {
	// GetChannelConfig defines methods that are related to channel configuration
	GetChannelConfig(channel string) Config
}
