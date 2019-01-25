/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/spf13/cobra"
)

func resetCmd() *cobra.Command {
	return nodeResetCmd
}

var nodeResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Resets the node.",
	Long:  `Resets all ledgers to genesis block`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("This will reset all ledgers to genesis blocks level. Press enter to continue...")
		reader := bufio.NewReader(os.Stdin)
		reader.ReadBytes('\n')
		rootFSPath := filepath.Join(config.GetPath("peer.fileSystemPath"), "ledgersData")
		return kvledger.ResetAllKVLedgers(rootFSPath)
	},
}
