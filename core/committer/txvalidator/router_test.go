/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator_test

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRouter(t *testing.T) {
	c14 := &mocks.ApplicationCapabilities{}
	c20 := &mocks.ApplicationCapabilities{}
	mcp := &mocks.CapabilityProvider{}
	mv14 := &mocks.Validator{}
	mv20 := &mocks.Validator{}

	c14.On("V2_0Validation").Return(false)
	c20.On("V2_0Validation").Return(true)

	r := &txvalidator.ValidationRouter{
		CapabilityProvider: mcp,
		V14Validator:       mv14,
		V20Validator:       mv20,
	}

	t.Run("v14 validator returns an error", func(t *testing.T) {
		mcp.On("Capabilities").Return(c14).Once()
		mv14.On("Validate", mock.Anything).Return(errors.New("uh uh")).Once()

		err := r.Validate(nil)
		require.EqualError(t, err, "uh uh")
	})

	t.Run("v20 validator returns an error", func(t *testing.T) {
		mcp.On("Capabilities").Return(c20).Once()
		mv20.On("Validate", mock.Anything).Return(errors.New("uh uh")).Once()

		err := r.Validate(nil)
		require.EqualError(t, err, "uh uh")
	})

	t.Run("v14 validator returns an error", func(t *testing.T) {
		mcp.On("Capabilities").Return(c14).Once()
		mv14.On("Validate", mock.Anything).Return(nil).Once()

		err := r.Validate(nil)
		require.NoError(t, err)
	})

	t.Run("v20 validator returns an error", func(t *testing.T) {
		mcp.On("Capabilities").Return(c20).Once()
		mv20.On("Validate", mock.Anything).Return(nil).Once()

		err := r.Validate(nil)
		require.NoError(t, err)
	})
}
