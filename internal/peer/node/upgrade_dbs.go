/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/spf13/cobra"
)

func upgradeDBsCmd() *cobra.Command {
	return nodeUpgradeDBsCmd
}

var nodeUpgradeDBsCmd = &cobra.Command{
	Use:   "upgrade-dbs",
	Short: "Upgrades databases.",
	Long: "Upgrades databases by directly updating the database format or dropping the databases." +
		" Dropped databases will be rebuilt with new format upon peer restart. When the command is executed, the peer must be offline.",
	RunE: func(cmd *cobra.Command, args []string) error {
		config := ledgerConfig()
		return kvledger.UpgradeDBs(config)
	},
}
