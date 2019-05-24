/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinforetriever_test

import (
	"testing"

	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/valinforetriever"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/valinforetriever/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestValidationInfoRetrieverFromNew(t *testing.T) {
	cc := "cc"
	newPlugin := "new"
	newArgs := []byte("new")
	uerr := errors.New("unexpected error")
	verr := errors.New("validation error")

	newResources := &mocks.LifecycleResources{}
	legacyResources := &mocks.LifecycleResources{}
	shim := valinforetriever.ValidationInfoRetrieveShim{
		Legacy: legacyResources,
		New:    newResources,
	}

	// successfully retrieve data from new source
	newResources.On("ValidationInfo", "channel", cc, nil).Return(newPlugin, newArgs, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr := shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "new", plugin)
	assert.Equal(t, []byte("new"), args)
	legacyResources.AssertNotCalled(t, "ValidationInfo")

	// get validation error from new source
	newResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.Error(t, validationErr)
	assert.Contains(t, validationErr.Error(), "validation error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)
	legacyResources.AssertNotCalled(t, "ValidationInfo")

	// get unexpected error from new source
	newResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, uerr, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.Error(t, unexpectedErr)
	assert.Error(t, validationErr)
	assert.Contains(t, unexpectedErr.Error(), "unexpected error")
	assert.Contains(t, validationErr.Error(), "validation error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)
	legacyResources.AssertNotCalled(t, "ValidationInfo")
}

func TestValidationInfoRetrieverFromLegacy(t *testing.T) {
	cc := "cc"
	legacyPlugin := "legacy"
	legacyArgs := []byte("legacy")
	uerr := errors.New("unexpected error")
	verr := errors.New("validation error")

	newResources := &mocks.LifecycleResources{}
	legacyResources := &mocks.LifecycleResources{}
	shim := valinforetriever.ValidationInfoRetrieveShim{
		Legacy: legacyResources,
		New:    newResources,
	}

	// new source always returns no data
	newResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, nil)

	// successfully retrieve data from legacy source
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return(legacyPlugin, legacyArgs, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr := shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "legacy", plugin)
	assert.Equal(t, []byte("legacy"), args)

	// get validation error from legacy source
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.Error(t, validationErr)
	assert.Contains(t, validationErr.Error(), "validation error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)

	// get unexpected error from legacy source
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, uerr, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.Error(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Contains(t, unexpectedErr.Error(), "unexpected error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)
}

func TestValidationInfoRetrieverFromLegacyWithConversion(t *testing.T) {
	cc := "cc"
	goodSPE := protoutil.MarshalOrPanic(&common.SignaturePolicyEnvelope{Version: 1})

	newResources := &mocks.LifecycleResources{}
	legacyResources := &mocks.LifecycleResources{}
	shim := valinforetriever.ValidationInfoRetrieveShim{
		Legacy: legacyResources,
		New:    newResources,
	}

	// new source always returns no data
	newResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, nil)

	// no conversion if the plugin is not vscc
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("not vscc", goodSPE, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr := shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "not vscc", plugin)
	assert.Equal(t, protoutil.MarshalOrPanic(&common.SignaturePolicyEnvelope{Version: 1}), args)

	// no conversion if the policy is not a signature policy envelope
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("vscc", []byte("not a signature policy envelope"), nil, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "vscc", plugin)
	assert.Equal(t, []byte("not a signature policy envelope"), args)

	// conversion if the policy is a signature policy envelope
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("vscc", goodSPE, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "vscc", plugin)
	assert.Equal(t, protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: &common.SignaturePolicyEnvelope{Version: 1},
		},
	}), args)
}
