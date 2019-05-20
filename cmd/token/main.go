/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/cmd/common"
	token "github.com/hyperledger/fabric/token/cmd"
)

func main() {
	factory.InitFactories(nil)
	cli := common.NewCLI("token", "Command line client for fabric token")
	token.AddCommands(cli)
	cli.Run(os.Args[1:])
}
