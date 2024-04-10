/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type OutputBlock struct {
	ChannelID   string
	Profile     string
	ConfigPath  string
	OutputBlock string
}

func (o OutputBlock) SessionName() string {
	return "configtxgen-output-block"
}

func (o OutputBlock) Args() []string {
	return []string{
		"-channelID", o.ChannelID,
		"-profile", o.Profile,
		"-configPath", o.ConfigPath,
		"-outputBlock", o.OutputBlock,
	}
}

type CreateChannelTx struct {
	ChannelID             string
	Profile               string
	ConfigPath            string
	OutputCreateChannelTx string
	BaseProfile           string
}

func (c CreateChannelTx) SessionName() string {
	return "configtxgen-create-channel-tx"
}

func (c CreateChannelTx) Args() []string {
	return []string{
		"-channelID", c.ChannelID,
		"-profile", c.Profile,
		"-configPath", c.ConfigPath,
		"-outputCreateChannelTx", c.OutputCreateChannelTx,
		"-channelCreateTxBaseProfile", c.BaseProfile,
	}
}

type PrintOrg struct {
	ConfigPath string
	ChannelID  string
	PrintOrg   string
}

func (p PrintOrg) SessionName() string {
	return "configtxgen-print-org"
}

func (p PrintOrg) Args() []string {
	return []string{
		"-configPath", p.ConfigPath,
		"-channelID", p.ChannelID,
		"-printOrg", p.PrintOrg,
	}
}
