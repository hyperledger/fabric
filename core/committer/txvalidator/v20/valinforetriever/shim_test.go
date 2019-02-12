/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package valinforetriever_test

import (
	"testing"

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

	new := &mocks.LifecycleResources{}
	legacy := &mocks.LifecycleResources{}
	shim := valinforetriever.ValidationInfoRetrieveShim{
		Legacy: legacy,
		New:    new,
	}

	// successfully retrieve data from new source
	new.On("ValidationInfo", "channel", cc, nil).Return(newPlugin, newArgs, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr := shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "new", plugin)
	assert.Equal(t, []byte("new"), args)
	legacy.AssertNotCalled(t, "ValidationInfo")

	// get validation error from new source
	new.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.Error(t, validationErr)
	assert.Contains(t, validationErr.Error(), "validation error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)
	legacy.AssertNotCalled(t, "ValidationInfo")

	// get unexpected error from new source
	new.On("ValidationInfo", "channel", cc, nil).Return("", nil, uerr, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.Error(t, unexpectedErr)
	assert.Error(t, validationErr)
	assert.Contains(t, unexpectedErr.Error(), "unexpected error")
	assert.Contains(t, validationErr.Error(), "validation error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)
	legacy.AssertNotCalled(t, "ValidationInfo")
}

func TestValidationInfoRetrieverFromLegacy(t *testing.T) {
	cc := "cc"
	legacyPlugin := "legacy"
	legacyArgs := []byte("legacy")
	uerr := errors.New("unexpected error")
	verr := errors.New("validation error")

	new := &mocks.LifecycleResources{}
	legacy := &mocks.LifecycleResources{}
	shim := valinforetriever.ValidationInfoRetrieveShim{
		Legacy: legacy,
		New:    new,
	}

	// new source always returns no data
	new.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, nil)

	// successfully retrieve data from legacy source
	legacy.On("ValidationInfo", "channel", cc, nil).Return(legacyPlugin, legacyArgs, nil, nil).Once()
	plugin, args, unexpectedErr, validationErr := shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Equal(t, "legacy", plugin)
	assert.Equal(t, []byte("legacy"), args)

	// get validation error from legacy source
	legacy.On("ValidationInfo", "channel", cc, nil).Return("", nil, nil, verr).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.NoError(t, unexpectedErr)
	assert.Error(t, validationErr)
	assert.Contains(t, validationErr.Error(), "validation error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)

	// get unexpected error from legacy source
	legacy.On("ValidationInfo", "channel", cc, nil).Return("", nil, uerr, nil).Once()
	plugin, args, unexpectedErr, validationErr = shim.ValidationInfo("channel", "cc", nil)
	assert.Error(t, unexpectedErr)
	assert.NoError(t, validationErr)
	assert.Contains(t, unexpectedErr.Error(), "unexpected error")
	assert.Equal(t, "", plugin)
	assert.Equal(t, []byte(nil), args)
}
