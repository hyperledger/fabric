/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"testing"

	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	"github.com/stretchr/testify/assert"
)

func TestRegistry(t *testing.T) {
	r := InitRegistry(Config{})
	assert.NotNil(t, r)
	authHandler := r.Lookup(AuthKey)
	assert.NotNil(t, authHandler)
	_, isAuthFilter := authHandler.(auth.Filter)
	assert.True(t, isAuthFilter)
	decorator := r.Lookup(DecoratorKey)
	assert.NotNil(t, decorator)
	_, isDecorator := decorator.(decoration.Decorator)
	assert.True(t, isDecorator)
}
