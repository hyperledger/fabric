/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/channelconfig"
)

//go:generate mockery -dir . -name Validator -case underscore -output mocks

// Validator defines API to validate transactions in a block
type Validator interface {
	// Validate returns an error if validation could not be performed successfully
	// In case of successful validation, the block is modified to reflect the validity
	// of the transactions it contains
	Validate(block *common.Block) error
}

//go:generate mockery -dir . -name CapabilityProvider -case underscore -output mocks

// CapabilityProvider contains functions to retrieve capability information for a channel
type CapabilityProvider interface {
	// Capabilities defines the capabilities for the application portion of this channel
	Capabilities() channelconfig.ApplicationCapabilities
}

// ValidationRouter dynamically invokes the appropriate validator depending on the
// capabilities that are currently enabled in the channel.
type ValidationRouter struct {
	CapabilityProvider
	V20Validator Validator
	V14Validator Validator
}

// Validate returns an error if validation could not be performed successfully
// In case of successful validation, the block is modified to reflect the validity
// of the transactions it contains
func (v *ValidationRouter) Validate(block *common.Block) error {
	switch {
	case v.Capabilities().V2_0Validation():
		return v.V20Validator.Validate(block)
	default:
		return v.V14Validator.Validate(block)
	}
}
