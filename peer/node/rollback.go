/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"bufio"
	"fmt"
	"os"

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
	flags.StringVarP(&channelID, "channelID", "c", common.UndefinedParamValue, "Rollback the ledger associated with the channel ID.")
	flags.Uint64VarP(&blockNumber, "blockNumber", "b", 0, "Rollback the ledger to the block number.")

	return nodeRollbackCmd
}

var nodeRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "rollback a ledger.",
	Long:  `rollback a ledger to a specified block number. Requires '-c', '-b'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if channelID == common.UndefinedParamValue {
			return errors.New("Must supply channel ID")
		}

		fmt.Printf("This will rollback channel [%s] to the block number [%d]. Press enter to continue...",
			channelID, blockNumber)
		reader := bufio.NewReader(os.Stdin)
		reader.ReadBytes('\n')
		return kvledger.RollbackKVLedger(channelID, blockNumber)
	},
}
