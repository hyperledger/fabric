/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package token

import (
	"github.com/hyperledger/fabric/cmd/common"
	"github.com/pkg/errors"
)

type ListTokensCmd struct {
	*BaseCmd
	clientConfigPath *string

	stub   Stub
	parser ResponseParser
}

func NewListTokensCmd(stub Stub, parser ResponseParser) *ListTokensCmd {
	return &ListTokensCmd{BaseCmd: &BaseCmd{}, stub: stub, parser: parser}
}

// SetRecipient sets the recipient
func (cmd *ListTokensCmd) SetClientConfigPath(clientConfigPath *string) {
	cmd.clientConfigPath = clientConfigPath
}

func (cmd *ListTokensCmd) Execute(conf common.Config) error {
	if cmd.clientConfigPath == nil || len(*cmd.clientConfigPath) == 0 {
		return errors.New("no client config path specified")
	}
	clientConfigPath := *cmd.clientConfigPath
	channel, mspPath, mspID := cmd.BaseCmd.GetArgs()

	err := cmd.stub.Setup(clientConfigPath, channel, mspPath, mspID)
	if err != nil {
		return err
	}
	response, err := cmd.stub.ListTokens()
	if err != nil {
		return err
	}

	return cmd.parser.ParseResponse(response)
}
