/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/spf13/cobra"
)

func updateCmd(cf *ChannelCmdFactory) *cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Send a configtx update.",
		Long:  "Signs and sends the supplied configtx update file to the channel. Requires '-f', '-o', '-c'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return update(cmd, args, cf)
		},
	}
	flagList := []string{
		"channelID",
		"file",
	}
	attachFlags(updateCmd, flagList)

	return updateCmd
}

func update(cmd *cobra.Command, args []string, cf *ChannelCmdFactory) error {
	// the global chainID filled by the "-c" command
	if channelID == common.UndefinedParamValue {
		return errors.New("Must supply channel ID")
	}

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

	ctxEnv, err := protoutil.UnmarshalEnvelope(fileData)
	if err != nil {
		return err
	}

	sCtxEnv, err := sanityCheckAndSignConfigTx(ctxEnv, cf.Signer)
	if err != nil {
		return err
	}

	var broadcastClient common.BroadcastClient
	broadcastClient, err = cf.BroadcastFactory()
	if err != nil {
		return fmt.Errorf("Error getting broadcast client: %s", err)
	}

	defer broadcastClient.Close()
	err = broadcastClient.Send(sCtxEnv)
	if err != nil {
		return err
	}

	logger.Info("Successfully submitted channel update")
	return nil
}
