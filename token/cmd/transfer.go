/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package token

import (
	"time"

	"github.com/hyperledger/fabric/cmd/common"
	"github.com/pkg/errors"
)

type TransferCmd struct {
	*BaseCmd
	clientConfigPath *string
	tokenIDs         *string
	shares           *string

	stub   Stub
	loader Loader
	parser ResponseParser
}

func NewTransferCmd(stub Stub, loader Loader, parser ResponseParser) *TransferCmd {
	return &TransferCmd{BaseCmd: &BaseCmd{}, stub: stub, loader: loader, parser: parser}
}

// SetClientConfigPath sets the client config path
func (cmd *TransferCmd) SetClientConfigPath(clientConfigPath *string) {
	cmd.clientConfigPath = clientConfigPath
}

// SetTokenIDs sets the tokenIds
func (cmd *TransferCmd) SetTokenIDs(tokenIDs *string) {
	cmd.tokenIDs = tokenIDs
}

// SetShares sets the output shares
func (cmd *TransferCmd) SetShares(shares *string) {
	cmd.shares = shares
}

func (cmd *TransferCmd) Execute(conf common.Config) error {
	if cmd.clientConfigPath == nil || *cmd.clientConfigPath == "" {
		return errors.New("no client config path specified")
	}
	if cmd.tokenIDs == nil || *cmd.tokenIDs == "" {
		return errors.New("no token IDs specified")
	}
	if cmd.shares == nil || *cmd.shares == "" {
		return errors.New("no shares specified")
	}

	// Prepare inputs
	clientConfigPath := *cmd.clientConfigPath
	tokenIDsString := *cmd.tokenIDs
	sharesString := *cmd.shares
	channel, mspPath, mspID := cmd.BaseCmd.GetArgs()

	tokenIDs, err := cmd.loader.TokenIDs(tokenIDsString)
	if err != nil {
		return err
	}
	shares, err := cmd.loader.Shares(sharesString)
	if err != nil {
		return err
	}

	// Transfer
	err = cmd.stub.Setup(clientConfigPath, channel, mspPath, mspID)
	if err != nil {
		return err
	}
	response, err := cmd.stub.Transfer(tokenIDs, shares, 30*time.Second)
	if err != nil {
		return err
	}

	return cmd.parser.ParseResponse(response)
}
