/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"io/ioutil"

	"github.com/hyperledger/fabric/protos/utils"

	"github.com/spf13/cobra"
)

func signconfigtxCmd(cf *ChannelCmdFactory) *cobra.Command {
	signconfigtxCmd := &cobra.Command{
		Use:   "signconfigtx",
		Short: "Signs a configtx update.",
		Long:  "Signs the supplied configtx update file in place on the filesystem. Requires '-f'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sign(cmd, args, cf)
		},
	}
	flagList := []string{
		"file",
	}
	attachFlags(signconfigtxCmd, flagList)

	return signconfigtxCmd
}

func sign(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	if channelTxFile == "" {
		return InvalidCreateTx("No configtx file name supplied")
	}
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(EndorserNotRequired, PeerDeliverNotRequired, OrdererNotRequired)
		if err != nil {
			return err
		}
	}

	fileData, err := ioutil.ReadFile(channelTxFile)
	if err != nil {
		return ConfigTxFileNotFound(err.Error())
	}

	ctxEnv, err := utils.UnmarshalEnvelope(fileData)
	if err != nil {
		return err
	}

	sCtxEnv, err := sanityCheckAndSignConfigTx(ctxEnv)
	if err != nil {
		return err
	}

	sCtxEnvData := utils.MarshalOrPanic(sCtxEnv)

	return ioutil.WriteFile(channelTxFile, sCtxEnvData, 0660)
}
