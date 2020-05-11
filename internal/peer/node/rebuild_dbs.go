/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/spf13/cobra"
)

func rebuildDBsCmd() *cobra.Command {
	return nodeRebuildCmd
}

var nodeRebuildCmd = &cobra.Command{
	Use:   "rebuild-dbs",
	Short: "Rebuilds databases.",
	Long:  "Drops the databases for all the channels and rebuilds them upon peer restart. When the command is executed, the peer must be offline.",
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ledgerConfig()
		return kvledger.RebuildDBs(config)
	},
}
