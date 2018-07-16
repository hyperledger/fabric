/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"testing"

	"github.com/hyperledger/fabric/core/handlers/validation/token"
	"github.com/stretchr/testify/assert"
)

func TestValidationFactory_New(t *testing.T) {
	factory := &token.ValidationFactory{}
	plugin := factory.New()
	assert.NotNil(t, plugin)
}

func TestValidation_Validate(t *testing.T) {
	factory := &token.ValidationFactory{}
	plugin := factory.New()

	err := plugin.Init()
	assert.NoError(t, err)

	// Validate returns nil, no matter what!
	err = plugin.Validate(nil, "", 0, 0, nil)
	assert.NoError(t, err)
}
