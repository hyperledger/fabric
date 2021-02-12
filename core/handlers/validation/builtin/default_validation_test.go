/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package builtin

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	commonerrors "github.com/hyperledger/fabric/common/errors"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	. "github.com/hyperledger/fabric/core/handlers/validation/api"
	vmocks "github.com/hyperledger/fabric/core/handlers/validation/builtin/mocks"
	"github.com/hyperledger/fabric/core/handlers/validation/builtin/v12/mocks"
	v20mocks "github.com/hyperledger/fabric/core/handlers/validation/builtin/v20/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	factory := &DefaultValidationFactory{}
	defValidation := factory.New()

	identityDeserializer := &mocks.IdentityDeserializer{}
	capabilities := &mocks.Capabilities{}
	stateFetcher := &mocks.StateFetcher{}
	polEval := &mocks.PolicyEvaluator{}
	colRes := &v20mocks.CollectionResources{}

	require.Equal(t, "stateFetcher not passed in init", defValidation.Init(identityDeserializer, capabilities, polEval, colRes).Error())
	require.Equal(t, "identityDeserializer not passed in init", defValidation.Init(capabilities, stateFetcher, polEval, colRes).Error())
	require.Equal(t, "capabilities not passed in init", defValidation.Init(identityDeserializer, stateFetcher, polEval, colRes).Error())
	require.Equal(t, "policy fetcher not passed in init", defValidation.Init(identityDeserializer, capabilities, stateFetcher, colRes).Error())
	require.Equal(t, "collection resources not passed in init", defValidation.Init(identityDeserializer, capabilities, stateFetcher, polEval).Error())

	fullDeps := []Dependency{identityDeserializer, capabilities, stateFetcher, polEval, colRes}
	require.NoError(t, defValidation.Init(fullDeps...))
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

	capabilities.On("V2_0Validation").Return(false)
	capabilities.On("V1_3Validation").Return(false)
	capabilities.On("V1_2Validation").Return(true)

	// Scenario I: An error that isn't *commonerrors.ExecutionFailureError or *commonerrors.VSCCEndorsementPolicyError
	// should cause a panic
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bla bla")).Once()
	require.Panics(t, func() {
		validation.Validate(block, "", 0, 0, plugin.SerializedPolicy("policy"))
	})

	// Scenario II: Non execution errors are returned as is
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCEndorsementPolicyError{Err: errors.New("foo")}).Once()
	err := validation.Validate(block, "", 0, 0, plugin.SerializedPolicy("policy"))
	require.Equal(t, (&commonerrors.VSCCEndorsementPolicyError{Err: errors.New("foo")}).Error(), err.Error())

	// Scenario III: Execution errors are converted to the plugin error type
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&commonerrors.VSCCExecutionFailureError{Err: errors.New("bar")}).Once()
	err = validation.Validate(block, "", 0, 0, plugin.SerializedPolicy("policy"))
	require.Equal(t, &ExecutionFailureError{Reason: "bar"}, err)

	// Scenario IV: No errors are forwarded
	validator.On("Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	require.NoError(t, validation.Validate(block, "", 0, 0, plugin.SerializedPolicy("policy")))
}

func TestValidateBadInput(t *testing.T) {
	validator := &vmocks.TransactionValidator{}
	validation := &DefaultValidation{
		TxValidatorV1_2: validator,
	}

	// Scenario I: Nil block
	validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
	err := validation.Validate(nil, "", 0, 0, plugin.SerializedPolicy("policy"))
	require.Equal(t, "empty block", err.Error())

	block := &common.Block{
		Header: &common.BlockHeader{},
		Data: &common.BlockData{
			Data: [][]byte{{}},
		},
	}
	// Scenario II: Block with 1 transaction, but position is at 1 also
	validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
	err = validation.Validate(block, "", 1, 0, plugin.SerializedPolicy("policy"))
	require.Equal(t, "block has only 1 transactions, but requested tx at position 1", err.Error())

	// Scenario III: Block without header
	validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
	err = validation.Validate(&common.Block{
		Data: &common.BlockData{
			Data: [][]byte{{}},
		},
	}, "", 0, 0, plugin.SerializedPolicy("policy"))
	require.Equal(t, "no block header", err.Error())

	// Scenario IV: No serialized policy passed
	require.Panics(t, func() {
		validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
		err = validation.Validate(&common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{{}},
			},
		}, "", 0, 0)
	})

	// Scenario V: Policy passed isn't a serialized policy
	require.Panics(t, func() {
		validator.On("Validate", mock.Anything, mock.Anything).Return(nil).Once()
		err = validation.Validate(&common.Block{
			Header: &common.BlockHeader{},
			Data: &common.BlockData{
				Data: [][]byte{{}},
			},
		}, "", 0, 0, []byte("policy"))
	})
}
