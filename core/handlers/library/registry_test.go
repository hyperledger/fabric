/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"testing"

	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	"github.com/stretchr/testify/assert"
)

func TestInitRegistry(t *testing.T) {
	r := InitRegistry(Config{
		AuthFilters: []*HandlerConfig{{Name: "DefaultAuth"}},
		Decorators:  []*HandlerConfig{{Name: "DefaultDecorator"}},
	})
	assert.NotNil(t, r)
	authHandlers := r.Lookup(Auth)
	assert.NotNil(t, authHandlers)
	filters, isAuthFilters := authHandlers.([]auth.Filter)
	assert.True(t, isAuthFilters)
	assert.Len(t, filters, 1)

	decorationHandlers := r.Lookup(Decoration)
	assert.NotNil(t, decorationHandlers)
	decorators, isDecorators := decorationHandlers.([]decoration.Decorator)
	assert.True(t, isDecorators)
	assert.Len(t, decorators, 1)
}

func TestLoadCompiledInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic with invalid factory method")
		}
	}()

	testReg := registry{}
	testReg.loadCompiled("InvalidFactory", Auth)
}
