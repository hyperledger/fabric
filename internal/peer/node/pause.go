/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func pauseCmd() *cobra.Command {
	pauseChannelCmd.ResetFlags()
	flags := pauseChannelCmd.Flags()
	flags.StringVarP(&channelID, "channelID", "c", common.UndefinedParamValue, "Channel to pause.")

	return pauseChannelCmd
}

var pauseChannelCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pauses a channel on the peer.",
	Long:  `Pauses a channel on the peer. When the command is executed, the peer must be offline. When the peer starts after pause, it will not receive blocks for the paused channel.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if channelID == common.UndefinedParamValue {
			return errors.New("Must supply channel ID")
		}

		config := ledgerConfig()
		return kvledger.PauseChannel(config.RootFSPath, channelID)
	},
}
