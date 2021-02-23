/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"

	"github.com/hyperledger/fabric/internal/ledger"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	resultFilename = "./result.json"
)

var (
	app = kingpin.New("ledger", "Ledger Utility Tool")

	compare       = app.Command("compare", "Compare two ledgers via their snapshots.")
	snapshotPath1 = compare.Arg("snapshotPath1", "File path to first ledger snapshot.").Required().String()
	snapshotPath2 = compare.Arg("snapshotPath2", "File path to second ledger snapshot.").Required().String()

	troubleshoot = app.Command("troubleshoot", "Identify potentially divergent transactions.")

	args = os.Args[1:]
)

func main() {
	kingpin.Version("0.0.1")

	command, err := app.Parse(args)
	if err != nil {
		kingpin.Fatalf("parsing arguments: %s. Try --help", err)
		return
	}

	switch command {

	case compare.FullCommand():

		count, err := ledger.Compare(*snapshotPath1, *snapshotPath2, resultFilename)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("\nSuccessfully compared snapshots. Result saved to %s. Total differences found: %v\n", resultFilename, count)

	case troubleshoot.FullCommand():

		fmt.Println("Command TBD")

	}
}
