/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	channelID   string
	blockNumber uint64
)

func rollbackCmd() *cobra.Command {
	nodeRollbackCmd.ResetFlags()
	flags := nodeRollbackCmd.Flags()
	flags.StringVarP(&channelID, "channelID", "c", common.UndefinedParamValue, "Channel to rollback.")
	flags.Uint64VarP(&blockNumber, "blockNumber", "b", 0, "Block number to which the channel needs to be rolled back to.")

	return nodeRollbackCmd
}

var nodeRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rolls back a channel.",
	Long:  `Rolls back a channel to a specified block number. When the command is executed, the peer must be offline. When the peer starts after the rollback, it will receive blocks, which got removed during the rollback, from an orderer or another peer to rebuild the block store and state database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if channelID == common.UndefinedParamValue {
			return errors.New("Must supply channel ID")
		}
		return kvledger.RollbackKVLedger(channelID, blockNumber)
	},
}
