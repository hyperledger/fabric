/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package token

import (
	"os"
	"time"

	cmdcommon "github.com/hyperledger/fabric/cmd/common"
	"github.com/hyperledger/fabric/protos/token"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	IssueCommand     = "issue"
	TransferCommand  = "transfer"
	ListTokensCommad = "list"
	RedeemCommand    = "redeem"
)

var (
	// responseParserWriter defines the stdout
	responseParserWriter = os.Stdout
)

//go:generate mockery -dir . -name CommandRegistrar -case underscore -output mocks/

// CommandRegistrar registers commands
type CommandRegistrar interface {
	// Command adds a new top-level command to the CLI
	Command(name, help string, onCommand cmdcommon.CLICommand) *kingpin.CmdClause
}

//go:generate mockery -dir . -name ResponseParser -case underscore -output mocks/

// ResponseParser parses responses sent from the server
type ResponseParser interface {
	// ParseResponse parses the response and uses the given output when emitting data
	ParseResponse(response StubResponse) error
}

type StubResponse interface {
}

//go:generate mockery -dir . -name Stub -case underscore -output mocks/

// Stub is a client for the token service
type Stub interface {
	// Setup the stub
	Setup(configFilePath, channel, mspPath, mspID string) error

	// Issue is the function that the client calls to introduce tokens into the system.
	Issue(tokensToIssue []*token.Token, waitTimeout time.Duration) (StubResponse, error)

	// Transfer is the function that the client calls to transfer his tokens.
	Transfer(tokenIDs []*token.TokenId, shares []*token.RecipientShare, waitTimeout time.Duration) (StubResponse, error)

	// Redeem allows the redemption of the tokens in the input tokenIDs
	Redeem(tokenIDs []*token.TokenId, quantity string, waitTimeout time.Duration) (StubResponse, error)

	// ListTokens allows the client to submit a list request to a prover peer service;
	ListTokens() (StubResponse, error)
}

//go:generate mockery -dir . -name Loader -case underscore -output mocks/

// Loader converts string to token objects to be used with the Stub
type Loader interface {
	// TokenOwner converts a string to a token owner
	TokenOwner(s string) (*token.TokenOwner, error)

	// TokenIDs converts a string to a slice of token ids
	TokenIDs(s string) ([]*token.TokenId, error)

	// Shares converts a string to a slice of RecipientShare
	Shares(s string) ([]*token.RecipientShare, error)
}

// BaseCmd contains shared command arguments
type BaseCmd struct {
	channel *string
	mspPath *string
	mspID   *string
}

// SetChannel sets the channel
func (cmd *BaseCmd) SetChannel(channel *string) {
	cmd.channel = channel
}

// SetMSPPath sets the mspPath
func (cmd *BaseCmd) SetMSPPath(mspPath *string) {
	cmd.mspPath = mspPath
}

// SetMSPId sets the mspID
func (cmd *BaseCmd) SetMSPId(mspID *string) {
	cmd.mspID = mspID
}

// GetArgs returns the arguments
func (cmd *BaseCmd) GetArgs() (string, string, string) {
	var channel string
	if cmd.channel == nil {
		channel = ""
	} else {
		channel = *cmd.channel
	}
	var mspPath string
	if cmd.mspPath == nil {
		mspPath = ""
	} else {
		mspPath = *cmd.mspPath
	}
	var mspID string
	if cmd.mspID == nil {
		mspID = ""
	} else {
		mspID = *cmd.mspID
	}
	return channel, mspPath, mspID
}

func addBaseFlags(cli *kingpin.CmdClause, baseCmd *BaseCmd) {
	channel := cli.Flag("channel", "Overrides channel configuration").String()
	mspPath := cli.Flag("mspPath", "Overrides msp path configuration").String()
	mspID := cli.Flag("mspId", "Overrides msp id configuration").String()

	baseCmd.SetChannel(channel)
	baseCmd.SetMSPPath(mspPath)
	baseCmd.SetMSPId(mspID)
}

// AddCommands registers the discovery commands to the given CommandRegistrar
func AddCommands(cli CommandRegistrar) {
	// Import
	issueCmd := NewIssueCmd(&TokenClientStub{}, &JsonLoader{}, &OperationResponseParser{responseParserWriter})
	importCli := cli.Command(IssueCommand, "Import token command", issueCmd.Execute)
	addBaseFlags(importCli, issueCmd.BaseCmd)
	configPath := importCli.Flag("config", "Sets the client configuration path").String()
	ttype := importCli.Flag("type", "Sets the token type to issue").String()
	quantity := importCli.Flag("quantity", "Sets the quantity of tokens to issue").String()
	recipient := importCli.Flag("recipient", "Sets the recipient of tokens to issue").String()

	issueCmd.SetClientConfigPath(configPath)
	issueCmd.SetType(ttype)
	issueCmd.SetQuantity(quantity)
	issueCmd.SetRecipient(recipient)

	// List Tokens
	listTokensCmd := NewListTokensCmd(&TokenClientStub{}, &UnspentTokenResponseParser{responseParserWriter})
	listTokensCli := cli.Command(ListTokensCommad, "List tokens command", listTokensCmd.Execute)
	addBaseFlags(listTokensCli, listTokensCmd.BaseCmd)
	configPath = listTokensCli.Flag("config", "Sets the client configuration path").String()
	listTokensCmd.SetClientConfigPath(configPath)

	// Transfer
	transferCmd := NewTransferCmd(&TokenClientStub{}, &JsonLoader{}, &OperationResponseParser{responseParserWriter})
	transferCli := cli.Command(TransferCommand, "Transfer tokens command", transferCmd.Execute)
	addBaseFlags(transferCli, transferCmd.BaseCmd)
	configPath = transferCli.Flag("config", "Sets the client configuration path").String()
	tokenIDs := transferCli.Flag("tokenIDs", "Sets the token IDs to transfer").String()
	shares := transferCli.Flag("shares", "Sets the shares of the recipients").String()
	transferCmd.SetClientConfigPath(configPath)
	transferCmd.SetTokenIDs(tokenIDs)
	transferCmd.SetShares(shares)

	// Redeem
	redeemCmd := NewRedeemCmd(&TokenClientStub{}, &JsonLoader{}, &OperationResponseParser{responseParserWriter})
	redeemCli := cli.Command(RedeemCommand, "Redeem tokens command", redeemCmd.Execute)
	addBaseFlags(redeemCli, redeemCmd.BaseCmd)
	configPath = redeemCli.Flag("config", "Sets the client configuration path").String()
	tokenIDs = redeemCli.Flag("tokenIDs", "Sets the token IDs to redeem").String()
	quantity = redeemCli.Flag("quantity", "Sets the quantity of tokens to redeem").String()
	redeemCmd.SetClientConfigPath(configPath)
	redeemCmd.SetTokenIDs(tokenIDs)
	redeemCmd.SetQuantity(quantity)
}
