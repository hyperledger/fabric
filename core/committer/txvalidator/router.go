/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	validatorv14 "github.com/hyperledger/fabric/core/committer/txvalidator/v14"
	validatorv20 "github.com/hyperledger/fabric/core/committer/txvalidator/v20"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/plugindispatcher"
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
	validator_v20 Validator
	validator_v14 Validator
}

func (v *routingValidator) Validate(block *common.Block) error {
	switch {
	case v.Capabilities().V2_0Validation():
		return v.validator_v20.Validate(block)
	default:
		return v.validator_v14.Validate(block)
	}
}

func NewTxValidator(
	chainID string,
	sem validatorv14.Semaphore,
	cr validatorv14.ChannelResources,
	lr plugindispatcher.LifecycleResources,
	cor plugindispatcher.CollectionResources,
	sccp sysccprovider.SystemChaincodeProvider,
	pm plugin.Mapper,
	cpmg policies.ChannelPolicyManagerGetter,
) *routingValidator {
	return &routingValidator{
		ChannelResources: cr,
		validator_v14:    validatorv14.NewTxValidator(chainID, sem, cr, sccp, pm),
		validator_v20:    validatorv20.NewTxValidator(chainID, sem, cr, cr.Ledger(), lr, cor, pm, cpmg),
	}
}
