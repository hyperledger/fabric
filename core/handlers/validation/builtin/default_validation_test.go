/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package builtin

import (
	"testing"

	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	. "github.com/hyperledger/fabric/core/handlers/validation/api"
	vmocks "github.com/hyperledger/fabric/core/handlers/validation/builtin/mocks"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin/v12/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestInit(t *testing.T) {
	factory := &DefaultValidationFactory{}
	defValidation := factory.New()

	identityDeserializer := &mocks.IdentityDeserializer{}
	capabilities := &mocks.Capabilities{}
	stateFetcher := &mocks.StateFetcher{}
	polEval := &mocks.PolicyEvaluator{}

	assert.Equal(t, "stateFetcher not passed in init", defValidation.Init(identityDeserializer, capabilities, polEval).Error())
	assert.Equal(t, "identityDeserializer not passed in init", defValidation.Init(capabilities, stateFetcher, polEval).Error())
	assert.Equal(t, "capabilities not passed in init", defValidation.Init(identityDeserializer, stateFetcher, polEval).Error())
	assert.Equal(t, "policy fetcher not passed in init", defValidation.Init(identityDeserializer, capabilities, stateFetcher).Error())

	fullDeps := []Dependency{identityDeserializer, capabilities, stateFetcher, polEval}
	assert.NoError(t, defValidation.Init(fullDeps...))
}

func TestErrorConversion(t *testing.T) {
	validator := &vmocks.TransactionValidator{}
	capabilities := &mocks.Capabilities{}
	validation := &DefaultValidation{
		TxValidatorV1_2: validator,
		Capabilities:    capabilities,
	}
	block := &common.Block{
		Header: &common.BlockHeader{},
		Data: &common.BlockData{
			Data: [][]byte{{}},
		},
	}

	capabilities.On("V1_3Validation").Return(false)
	capabilities.On("V1_2Validation").Return(true)

	// Scenario I: An error that isn't *commonerrors.ExecutionFailureError or *commonerrors.VSCCEndorsementPolicyError
	// should cause a panic
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bla bla")).Once()
	assert.Panics(t, func() {
		validation.Validate(block, "", 0, 0, txvalidator.SerializedPolicy("policy"))
	})

	// Scenario II: Non execution errors are returned as is
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCEndorsementPolicyError{Err: errors.New("foo")}).Once()
	err := validation.Validate(block, "", 0, 0, txvalidator.SerializedPolicy("policy"))
	assert.Equal(t, (&commonerrors.VSCCEndorsementPolicyError{Err: errors.New("foo")}).Error(), err.Error())

	// Scenario III: Execution errors are converted to the plugin error type
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCExecutionFailureError{Err: errors.New("bar")}).Once()
	err = validation.Validate(block, "", 0, 0, txvalidator.SerializedPolicy("policy"))
	assert.Equal(t, &ExecutionFailureError{Reason: "bar"}, err)

	// Scenario IV: No errors are forwarded
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	assert.NoError(t, validation.Validate(block, "", 0, 0, txvalidator.SerializedPolicy("policy")))
}

func TestValidateBadInput(t *testing.T) {
	validator := &vmocks.TransactionValidator{}
	validation := &DefaultValidation{
		TxValidatorV1_2: validator,
	}

	// Scenario I: Nil block
	validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
	err := validation.Validate(nil, "", 0, 0, txvalidator.SerializedPolicy("policy"))
	assert.Equal(t, "empty block", err.Error())

	block := &common.Block{
		Header: &common.BlockHeader{},
		Data: &common.BlockData{
			Data: [][]byte{{}},
		},
	}
	// Scenario II: Block with 1 transaction, but position is at 1 also
	validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
	err = validation.Validate(block, "", 1, 0, txvalidator.SerializedPolicy("policy"))
	assert.Equal(t, "block has only 1 transactions, but requested tx at position 1", err.Error())

	// Scenario III: Block without header
	validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
	err = validation.Validate(&common.Block{
		Data: &common.BlockData{
			Data: [][]byte{{}},
		},
	}, "", 0, 0, txvalidator.SerializedPolicy("policy"))
	assert.Equal(t, "no block header", err.Error())

	// Scenario IV: No serialized policy passed
	assert.Panics(t, func() {
		validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
		err = validation.Validate(&common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{{}},
			},
		}, "", 0, 0)
	})

	// Scenario V: Policy passed isn't a serialized policy
	assert.Panics(t, func() {
		validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
		err = validation.Validate(&common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{{}},
			},
		}, "", 0, 0, []byte("policy"))
	})

}
