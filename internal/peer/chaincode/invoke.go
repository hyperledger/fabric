/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var chaincodeInvokeCmd *cobra.Command

// invokeCmd returns the cobra command for Chaincode Invoke
func invokeCmd(cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeInvokeCmd = &cobra.Command{
		Use:       "invoke",
		Short:     fmt.Sprintf("Invoke the specified %s.", chainFuncName),
		Long:      fmt.Sprintf("Invoke the specified %s. It will try to commit the endorsed transaction to the network.", chainFuncName),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodeInvoke(cmd, cf, cryptoProvider)
		},
	}
	flagList := []string{
		"name",
		"ctor",
		"isInit",
		"channelID",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
		"waitForEvent",
		"waitForEventTimeout",
	}
	attachFlags(chaincodeInvokeCmd, flagList)

	return chaincodeInvokeCmd
}

func chaincodeInvoke(cmd *cobra.Command, cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) error {
	if channelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, true, cryptoProvider)
		if err != nil {
			return err
		}
	}
	defer cf.BroadcastClient.Close()

	return chaincodeInvokeOrQuery(cmd, true, cf)
}
