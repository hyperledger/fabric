/*
Copyright IBM Corp, SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package library

import (
	"testing"

	"github.com/hyperledger/fabric/core/handlers/auth"
	"github.com/hyperledger/fabric/core/handlers/decoration"
	"github.com/stretchr/testify/require"
)

func TestInitRegistry(t *testing.T) {
	r := InitRegistry(Config{
		AuthFilters: []*HandlerConfig{{Name: "DefaultAuth"}},
		Decorators:  []*HandlerConfig{{Name: "DefaultDecorator"}},
	})
	require.NotNil(t, r)
	authHandlers := r.Lookup(Auth)
	require.NotNil(t, authHandlers)
	filters, isAuthFilters := authHandlers.([]auth.Filter)
	require.True(t, isAuthFilters)
	require.Len(t, filters, 1)

	decorationHandlers := r.Lookup(Decoration)
	require.NotNil(t, decorationHandlers)
	decorators, isDecorators := decorationHandlers.([]decoration.Decorator)
	require.True(t, isDecorators)
	require.Len(t, decorators, 1)
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
