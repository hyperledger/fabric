/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type TokenIssue struct {
	Config    string
	Channel   string
	MspPath   string
	MspID     string
	Type      string
	Quantity  string
	Recipient string
}

func (p TokenIssue) SessionName() string {
	return "token-issue"
}

func (p TokenIssue) Args() []string {
	return []string{
		"issue",
		"--config", p.Config,
		"--channel", p.Channel,
		"--mspPath", p.MspPath,
		"--mspId", p.MspID,
		"--type", p.Type,
		"--quantity", p.Quantity,
		"--recipient", p.Recipient,
	}
}

type TokenList struct {
	Config  string
	Channel string
	MspPath string
	MspID   string
}

func (p TokenList) SessionName() string {
	return "token-list"
}

func (p TokenList) Args() []string {
	return []string{
		"list",
		"--config", p.Config,
		"--channel", p.Channel,
		"--mspPath", p.MspPath,
		"--mspId", p.MspID,
	}
}

type TokenTransfer struct {
	Config   string
	Channel  string
	MspPath  string
	MspID    string
	TokenIDs string
	Shares   string
}

func (p TokenTransfer) SessionName() string {
	return "token-transfer"
}

func (p TokenTransfer) Args() []string {
	return []string{
		"transfer",
		"--config", p.Config,
		"--channel", p.Channel,
		"--mspPath", p.MspPath,
		"--mspId", p.MspID,
		"--tokenIDs", p.TokenIDs,
		"--shares", p.Shares,
	}
}

type TokenRedeem struct {
	Config   string
	Channel  string
	MspPath  string
	MspID    string
	TokenIDs string
	Quantity string
}

func (p TokenRedeem) SessionName() string {
	return "token-redeem"
}

func (p TokenRedeem) Args() []string {
	return []string{
		"redeem",
		"--config", p.Config,
		"--channel", p.Channel,
		"--mspPath", p.MspPath,
		"--mspId", p.MspID,
		"--tokenIDs", p.TokenIDs,
		"--quantity", p.Quantity,
	}
}
