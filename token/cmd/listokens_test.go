/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"testing"

	"github.com/hyperledger/fabric/cmd/common"
	"github.com/hyperledger/fabric/token/cmd"
	"github.com/hyperledger/fabric/token/cmd/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestListTokensCmd(t *testing.T) {
	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	cmd := token.NewListTokensCmd(stub, parser)

	t.Run("no config supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(nil)
		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no client config path specified")
	})
}

func TestListTokensCmd_FailedSetup(t *testing.T) {
	clientConfigPath := "configuration"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	cmd := token.NewListTokensCmd(stub, parser)

	cmd.SetClientConfigPath(&clientConfigPath)

	stub.On("Setup", "configuration", "", "", "").Return(errors.New("failed setup"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "failed setup")
}

func TestListTokensCmd_FailedIssue(t *testing.T) {
	clientConfigPath := "configuration"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	cmd := token.NewListTokensCmd(stub, parser)

	cmd.SetClientConfigPath(&clientConfigPath)

	stub.On("Setup", "configuration", "", "", "").Return(nil)
	stub.On("ListTokens").Return(nil, errors.New("failed list tokens"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "failed list tokens")
}
