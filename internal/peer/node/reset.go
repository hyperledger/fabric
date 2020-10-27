/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/spf13/cobra"
)

func resetCmd() *cobra.Command {
	return nodeResetCmd
}

var nodeResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Resets the node.",
	Long: "Resets all channels to the genesis block. When the command is executed, the peer must be offline." +
		" When the peer starts after the reset, it will receive blocks starting with block number one from an orderer or another peer to rebuild the block store and state database." +
		" The command is not supported if the peer contains any channel that was bootstrapped from a snapshot.",
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ledgerConfig()
		return kvledger.ResetAllKVLedgers(config.RootFSPath)
	},
}
