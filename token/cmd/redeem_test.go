/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token_test

import (
	"testing"

	"github.com/hyperledger/fabric/cmd/common"
	ptoken "github.com/hyperledger/fabric/protos/token"
	token "github.com/hyperledger/fabric/token/cmd"
	"github.com/hyperledger/fabric/token/cmd/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRedeemCmd(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDs := "token_ids"
	var quantity = ToHex(100)

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewRedeemCmd(stub, loader, parser)

	t.Run("no config supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(nil)
		cmd.SetTokenIDs(&tokenIDs)
		cmd.SetQuantity(&quantity)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no client config path specified")
	})

	t.Run("no token ids supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetTokenIDs(nil)
		cmd.SetQuantity(&quantity)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no token IDs specified")
	})

	t.Run("no quantity supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetTokenIDs(&tokenIDs)
		cmd.SetQuantity(nil)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no quantity specified")
	})

}

func TestTestRedeemCmd_InvalidQuantities(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDs := "token_ids"
	var quantity = ToHex(1)

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewRedeemCmd(stub, loader, parser)
	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetTokenIDs(&tokenIDs)
	cmd.SetQuantity(&quantity)

	loader.On("TokenIDs", "token_ids").Return(nil, errors.New("invalid token ids"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, "invalid token ids", err.Error())
}

func TestTestRedeemCmd_FailedStubSetup(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDs := "token_ids"
	var quantity = ToHex(1)

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewRedeemCmd(stub, loader, parser)
	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetTokenIDs(&tokenIDs)
	cmd.SetQuantity(&quantity)

	loader.On("TokenIDs", "token_ids").Return([]*ptoken.TokenId{}, nil)
	stub.On("Setup", "configuration", "", "", "").Return(errors.New("failed setup"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, "failed setup", err.Error())
}

func TestTestRedeemCmd_FailedStubRedeem(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDsString := "token_ids"
	quantity := ToHex(10)

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewRedeemCmd(stub, loader, parser)
	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetTokenIDs(&tokenIDsString)
	cmd.SetQuantity(&quantity)

	tokenIDs := []*ptoken.TokenId{{TxId: "1", Index: 1}, {TxId: "2", Index: 1}}
	loader.On("TokenIDs", "token_ids").Return(tokenIDs, nil)
	stub.On("Setup", "configuration", "", "", "").Return(nil)
	stub.On("Redeem", tokenIDs, quantity, mock.Anything).Return(nil, errors.New("failed redeem"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, "failed redeem", err.Error())
}
