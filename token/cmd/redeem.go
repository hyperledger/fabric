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

type RedeemCmd struct {
	*BaseCmd
	clientConfigPath *string
	tokenIDs         *string
	quantity         *string

	stub   Stub
	loader Loader
	parser ResponseParser
}

func NewRedeemCmd(stub Stub, loader Loader, parser ResponseParser) *RedeemCmd {
	return &RedeemCmd{BaseCmd: &BaseCmd{}, stub: stub, loader: loader, parser: parser}
}

// SetClientConfigPath sets the client config path
func (cmd *RedeemCmd) SetClientConfigPath(clientConfigPath *string) {
	cmd.clientConfigPath = clientConfigPath
}

// SetTokenIDs sets the tokenIds
func (cmd *RedeemCmd) SetTokenIDs(tokenIDs *string) {
	cmd.tokenIDs = tokenIDs
}

// SetQuantity sets the quantity
func (cmd *RedeemCmd) SetQuantity(quantity *string) {
	cmd.quantity = quantity
}

func (cmd *RedeemCmd) Execute(conf common.Config) error {
	if cmd.clientConfigPath == nil || len(*cmd.clientConfigPath) == 0 {
		return errors.New("no client config path specified")
	}
	if cmd.tokenIDs == nil || len(*cmd.tokenIDs) == 0 {
		return errors.New("no token IDs specified")
	}
	if cmd.quantity == nil || len(*cmd.quantity) == 0 {
		return errors.New("no quantity specified")
	}

	// Prepare inputs
	clientConfigPath := *cmd.clientConfigPath
	tokenIDsString := *cmd.tokenIDs
	quantityValue := *cmd.quantity
	channel, mspPath, mspID := cmd.BaseCmd.GetArgs()

	tokenIDs, err := cmd.loader.TokenIDs(tokenIDsString)
	if err != nil {
		return err
	}

	// Redeem
	err = cmd.stub.Setup(clientConfigPath, channel, mspPath, mspID)
	if err != nil {
		return err
	}
	response, err := cmd.stub.Redeem(tokenIDs, quantityValue, 30*time.Second)
	if err != nil {
		return err
	}

	return cmd.parser.ParseResponse(response)
}
