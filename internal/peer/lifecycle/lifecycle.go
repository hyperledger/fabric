/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric/internal/peer/lifecycle/chaincode"
	"github.com/spf13/cobra"
)

// Cmd returns the cobra command for lifecycle
func Cmd(cryptoProvider bccsp.BCCSP, isNew bool) *cobra.Command {
	lifecycleCmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Perform _lifecycle operations.",
		Long:  "Perform _lifecycle operations.",
	}
	lifecycleCmd.AddCommand(chaincode.Cmd(cryptoProvider, isNew))

	if !isNew {
		lifecycleCmd.Short = "[DEPRECATED] Perform _lifecycle operations (use the \"peercli lifecycle\")."
		lifecycleCmd.Long = "[DEPRECATED] Perform _lifecycle operations. Instead of this command, use \"peercli lifecycle\"."
	}

	return lifecycleCmd
}
