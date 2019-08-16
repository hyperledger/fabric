/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/spf13/cobra"
)

var chaincodeQueryCmd *cobra.Command

// queryCmd returns the cobra command for Chaincode Query
func queryCmd(cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) *cobra.Command {
	chaincodeQueryCmd = &cobra.Command{
		Use:       "query",
		Short:     fmt.Sprintf("Query using the specified %s.", chainFuncName),
		Long:      fmt.Sprintf("Get endorsed result of %s function call and print it. It won't generate transaction.", chainFuncName),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodeQuery(cmd, cf, cryptoProvider)
		},
	}
	flagList := []string{
		"ctor",
		"name",
		"channelID",
		"peerAddresses",
		"tlsRootCertFiles",
		"connectionProfile",
	}
	attachFlags(chaincodeQueryCmd, flagList)

	chaincodeQueryCmd.Flags().BoolVarP(&chaincodeQueryRaw, "raw", "r", false,
		"If true, output the query value as raw bytes, otherwise format as a printable string")
	chaincodeQueryCmd.Flags().BoolVarP(&chaincodeQueryHex, "hex", "x", false,
		"If true, output the query value byte array in hexadecimal. Incompatible with --raw")

	return chaincodeQueryCmd
}

func chaincodeQuery(cmd *cobra.Command, cf *ChaincodeCmdFactory, cryptoProvider bccsp.BCCSP) error {
	if channelID == "" {
		return errors.New("The required parameter 'channelID' is empty. Rerun the command with -C flag")
	}
	// Parsing of the command line is done so silence cmd usage
	cmd.SilenceUsage = true

	var err error
	if cf == nil {
		cf, err = InitCmdFactory(cmd.Name(), true, false, cryptoProvider)
		if err != nil {
			return err
		}
	}

	return chaincodeInvokeOrQuery(cmd, false, cf)
}
