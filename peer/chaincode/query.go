/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package chaincode

import (
	"fmt"

	"github.com/spf13/cobra"
)

var chaincodeQueryCmd *cobra.Command

// queryCmd returns the cobra command for Chaincode Query
func queryCmd(cf *ChaincodeCmdFactory) *cobra.Command {
	chaincodeQueryCmd = &cobra.Command{
		Use:       "query",
		Short:     fmt.Sprintf("Query using the specified %s.", chainFuncName),
		Long:      fmt.Sprintf("Get endorsed result of %s function call and print it. It won't generate transaction.", chainFuncName),
		ValidArgs: []string{"1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return chaincodeQuery(cmd, args, cf)
		},
	}
	flagList := []string{
		"ctor",
		"name",
		"tid",
		"channelID",
	}
	attachFlags(chaincodeQueryCmd, flagList)

	chaincodeQueryCmd.Flags().BoolVarP(&chaincodeQueryRaw, "raw", "r", false,
		"If true, output the query value as raw bytes, otherwise format as a printable string")
	chaincodeQueryCmd.Flags().BoolVarP(&chaincodeQueryHex, "hex", "x", false,
		"If true, output the query value byte array in hexadecimal. Incompatible with --raw")

	return chaincodeQueryCmd
}

func chaincodeQuery(cmd *cobra.Command, args []string, cf *ChaincodeCmdFactory) error {
	var err error
	if cf == nil {
		cf, err = InitCmdFactory(true, false)
		if err != nil {
			return err
		}
	}

	return chaincodeInvokeOrQuery(cmd, args, false, cf)
}
