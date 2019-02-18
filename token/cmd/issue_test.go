/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"testing"

	"github.com/hyperledger/fabric/cmd/common"
	ptoken "github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/token/cmd"
	"github.com/hyperledger/fabric/token/cmd/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestIssueCmd(t *testing.T) {
	clientConfigPath := "configuration"
	recipient := "alice"
	var quantity string = ToHex(100)
	ttype := "pineapple"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewIssueCmd(stub, loader, parser)

	t.Run("no config supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(nil)
		cmd.SetRecipient(&recipient)
		cmd.SetQuantity(&quantity)
		cmd.SetType(&ttype)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no client config path specified")
	})

	t.Run("no recipient supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetRecipient(nil)
		cmd.SetQuantity(&quantity)
		cmd.SetType(&ttype)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no recipient specified")
	})

	t.Run("no quantity supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetRecipient(&recipient)
		cmd.SetQuantity(nil)
		cmd.SetType(&ttype)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no quantity specified")
	})

	t.Run("no type supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetRecipient(&recipient)
		cmd.SetQuantity(&quantity)
		cmd.SetType(nil)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no type specified")
	})

	t.Run("invalid recipient", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetRecipient(&recipient)
		cmd.SetQuantity(&quantity)
		cmd.SetType(&ttype)

		loader.On("TokenOwner", "alice").Return(nil, errors.New("invalid recipient"))
		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "invalid recipient")
	})
}

func TestImportCmd_FailedSetup(t *testing.T) {
	clientConfigPath := "configuration"
	recipient := "alice"
	var quantity string = ToHex(100)
	ttype := "pineapple"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewIssueCmd(stub, loader, parser)

	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetRecipient(&recipient)
	cmd.SetQuantity(&quantity)
	cmd.SetType(&ttype)

	loader.On("TokenOwner", "alice").Return(&ptoken.TokenOwner{Raw: []byte{0, 1, 2, 3, 4}}, nil)
	stub.On("Setup", "configuration", "", "", "").Return(errors.New("failed setup"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "failed setup")
}

func TestImportCmd_FailedIssue(t *testing.T) {
	clientConfigPath := "configuration"
	recipient := "alice"
	var quantity string = ToHex(100)
	ttype := "pineapple"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewIssueCmd(stub, loader, parser)

	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetRecipient(&recipient)
	cmd.SetQuantity(&quantity)
	cmd.SetType(&ttype)

	loader.On("TokenOwner", "alice").Return(&ptoken.TokenOwner{Raw: []byte{0, 1, 2, 3, 4}}, nil)
	stub.On("Setup", "configuration", "", "", "").Return(nil)
	stub.On("Issue", mock.Anything, mock.Anything).Return(nil, errors.New("failed issue"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "failed issue")
}
