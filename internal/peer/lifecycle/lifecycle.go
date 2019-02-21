/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/spf13/cobra"
)

// Cmd returns the cobra command for lifecycle
func Cmd() *cobra.Command {
	lifecycleCmd.AddCommand(chaincode.Cmd(nil))

	return lifecycleCmd
}

var lifecycleCmd = &cobra.Command{
	Use:   "lifecycle",
	Short: "Perform _lifecycle operations",
	Long:  "Perform _lifecycle operations",
}
