/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinforetriever_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/valinforetriever"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/valinforetriever/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, unexpectedErr)
	require.NoError(t, validationErr)
	require.Equal(t, "new", plugin)
	require.Equal(t, []byte("new"), args)
	legacyResources.AssertNotCalled(t, "ValidationInfo")

	// get validation error from new source
	newResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	require.NoError(t, unexpectedErr)
	require.Error(t, validationErr)
	require.Contains(t, validationErr.Error(), "validation error")
	require.Equal(t, "", plugin)
	require.Equal(t, []byte(nil), args)
	legacyResources.AssertNotCalled(t, "ValidationInfo")

	// get unexpected error from new source
	newResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, uerr, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	require.Error(t, unexpectedErr)
	require.Error(t, validationErr)
	require.Contains(t, unexpectedErr.Error(), "unexpected error")
	require.Contains(t, validationErr.Error(), "validation error")
	require.Equal(t, "", plugin)
	require.Equal(t, []byte(nil), args)
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
	require.NoError(t, unexpectedErr)
	require.NoError(t, validationErr)
	require.Equal(t, "legacy", plugin)
	require.Equal(t, []byte("legacy"), args)

	// get validation error from legacy source
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	require.NoError(t, unexpectedErr)
	require.Error(t, validationErr)
	require.Contains(t, validationErr.Error(), "validation error")
	require.Equal(t, "", plugin)
	require.Equal(t, []byte(nil), args)

	// get unexpected error from legacy source
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("", nil, uerr, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	require.Error(t, unexpectedErr)
	require.NoError(t, validationErr)
	require.Contains(t, unexpectedErr.Error(), "unexpected error")
	require.Equal(t, "", plugin)
	require.Equal(t, []byte(nil), args)
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
	require.NoError(t, unexpectedErr)
	require.NoError(t, validationErr)
	require.Equal(t, "not vscc", plugin)
	require.Equal(t, protoutil.MarshalOrPanic(&common.SignaturePolicyEnvelope{Version: 1}), args)

	// no conversion if the policy is not a signature policy envelope
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("vscc", []byte("not a signature policy envelope"), nil, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	require.NoError(t, unexpectedErr)
	require.NoError(t, validationErr)
	require.Equal(t, "vscc", plugin)
	require.Equal(t, []byte("not a signature policy envelope"), args)

	// conversion if the policy is a signature policy envelope
	legacyResources.On("ValidationInfo", "channel", cc, nil).Return("vscc", goodSPE, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	require.NoError(t, unexpectedErr)
	require.NoError(t, validationErr)
	require.Equal(t, "vscc", plugin)
	require.Equal(t, protoutil.MarshalOrPanic(&peer.ApplicationPolicy{
		Type: &peer.ApplicationPolicy_SignaturePolicy{
			SignaturePolicy: &common.SignaturePolicyEnvelope{Version: 1},
		},
	}), args)
}
