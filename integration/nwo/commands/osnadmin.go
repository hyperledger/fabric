/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type ChannelList struct {
	OrdererAddress string
	CAFile         string
	ClientCert     string
	ClientKey      string
	ChannelID      string
}

func (c ChannelList) SessionName() string {
	return "osnadmin-channel-list"
}

func (c ChannelList) Args() []string {
	args := []string{
		"channel", "list",
		"--no-status",
		"--orderer-address", c.OrdererAddress,
	}
	if c.CAFile != "" {
		args = append(args, "--ca-file", c.CAFile)
	}
	if c.ClientCert != "" {
		args = append(args, "--client-cert", c.ClientCert)
	}
	if c.ClientKey != "" {
		args = append(args, "--client-key", c.ClientKey)
	}
	if c.ChannelID != "" {
		args = append(args, "--channelID", c.ChannelID)
	}
	return args
}
