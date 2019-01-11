/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	validatorv14 "github.com/hyperledger/fabric/core/committer/txvalidator/v14"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/protos/common"
)

// Validator defines API to validate transactions in a block
type Validator interface {
	// Validate returns an error if validation could not be performed successfully
	// In case of successful validation, the block is modified to reflect the validity
	// of the transactions it contains
	Validate(block *common.Block) error
}

type routingValidator struct {
	validatorv14.ChannelResources
	validator_v14 Validator
}

func (v *routingValidator) Validate(block *common.Block) error {
	// TODO: use v.support.Capabilities() to determine which validator we should call
	return v.validator_v14.Validate(block)
}

func NewTxValidator(chainID string, sem validatorv14.Semaphore, cr validatorv14.ChannelResources, sccp sysccprovider.SystemChaincodeProvider, pm plugin.Mapper) *routingValidator {
	return &routingValidator{
		ChannelResources: cr,
		validator_v14:    validatorv14.NewTxValidator(chainID, sem, cr, sccp, pm),
	}
}
