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

func TestTransferCmd(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDs := "token_ids"
	shares := "shares"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewTransferCmd(stub, loader, parser)

	t.Run("no config supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(nil)
		cmd.SetTokenIDs(&tokenIDs)
		cmd.SetShares(&shares)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no client config path specified")
	})

	t.Run("no token ids supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetTokenIDs(nil)
		cmd.SetShares(&shares)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no token IDs specified")
	})

	t.Run("no quantity supplied", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetTokenIDs(&tokenIDs)
		cmd.SetShares(nil)

		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "no shares specified")
	})

	t.Run("invalid TokenIDs", func(t *testing.T) {
		cmd.SetClientConfigPath(&clientConfigPath)
		cmd.SetTokenIDs(&tokenIDs)
		cmd.SetShares(&shares)

		loader.On("TokenIDs", "token_ids").Return(nil, errors.New("invalid token ids"))
		err := cmd.Execute(common.Config{})
		assert.Equal(t, err.Error(), "transfer: failed loading token ids [token_ids]: invalid token ids")
	})

}

func TestTestTransferCmd_InvalidShares(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDs := "token_ids"
	shares := "shares"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewTransferCmd(stub, loader, parser)
	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetTokenIDs(&tokenIDs)
	cmd.SetShares(&shares)

	loader.On("TokenIDs", "token_ids").Return([]*ptoken.TokenId{{TxId: "0"}}, nil)
	loader.On("Shares", "shares").Return(nil, errors.New("invalid shares"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "transfer: failed loading shares [shares]: invalid shares")
}

func TestTestTransferCmd_FailedStubSetup(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDs := "token_ids"
	shares := "shares"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewTransferCmd(stub, loader, parser)
	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetTokenIDs(&tokenIDs)
	cmd.SetShares(&shares)

	loader.On("TokenIDs", "token_ids").Return([]*ptoken.TokenId{{TxId: "0"}}, nil)
	loader.On("Shares", "shares").Return([]*ptoken.RecipientShare{{Quantity: "10"}}, nil)
	stub.On("Setup", "configuration", "", "", "").Return(errors.New("failed setup"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "transfer: failed invoking setup [][][]: failed setup")
}

func TestTestTransferCmd_FailedStubTransfer(t *testing.T) {
	clientConfigPath := "configuration"
	tokenIDsString := "token_ids"
	sharesString := "shares"

	stub := &mocks.Stub{}
	parser := &mocks.ResponseParser{}
	loader := &mocks.Loader{}
	cmd := token.NewTransferCmd(stub, loader, parser)
	cmd.SetClientConfigPath(&clientConfigPath)
	cmd.SetTokenIDs(&tokenIDsString)
	cmd.SetShares(&sharesString)

	tokenIDs := []*ptoken.TokenId{{TxId: "1", Index: 1}, {TxId: "2", Index: 1}}
	shares := []*ptoken.RecipientShare{
		{
			Recipient: &ptoken.TokenOwner{Raw: []byte("bob")},
			Quantity:  ToHex(100),
		},
	}

	loader.On("TokenIDs", "token_ids").Return(tokenIDs, nil)
	loader.On("Shares", "shares").Return(shares, nil)
	stub.On("Setup", "configuration", "", "", "").Return(nil)
	stub.On("Transfer", tokenIDs, shares, mock.Anything).Return(nil, errors.New("failed transfer"))
	err := cmd.Execute(common.Config{})
	assert.Equal(t, err.Error(), "transfer: failed invoking transfer [][][][token_ids][shares]: failed transfer")
}
